package goscript

import (
	"fmt"
	"go/token"
	"go/types"
	"reflect"
	"strings"
	"sync/atomic"
	"time"

	"github.com/linkxzhou/SimpleClaw/goscript/internal/value"
	"github.com/linkxzhou/SimpleClaw/utils"

	"golang.org/x/tools/go/ssa"
)

// debugMode 启用详细调试输出。
var debugMode = false

// callExternal 调用外部（原生 Go）函数。
// 处理普通参数和可变参数的转换，将解释器的 value.Value 转为 reflect.Value 后调用。
func callExternal(fn reflect.Value, args []value.Value) value.Value {
	fnType := fn.Type()
	numIn := fnType.NumIn()
	if fnType.IsVariadic() {
		numIn--
	}

	// 准备普通参数
	in := make([]reflect.Value, numIn)
	for i := 0; i < numIn; i++ {
		in[i] = args[i].RValue().Convert(fnType.In(i))
	}

	// 展开可变参数
	if fnType.IsVariadic() {
		variadicType := fnType.In(numIn).Elem()
		variadicArgs := args[len(args)-1]
		for i := 0; i < variadicArgs.Len(); i++ {
			in = append(in, variadicArgs.Index(i).RValue().Convert(variadicType))
		}
	}

	out := fn.Call(in)
	return value.Package(out)
}

// callSSA 调用一个 SSA 函数，传入参数和闭包捕获的自由变量。
// 创建子栈帧，初始化局部变量、绑定参数和闭包变量，然后执行函数体。
func callSSA(caller *execFrame, fn *ssa.Function, args []value.Value, env []*value.Value) value.Value {
	frame := caller.newChild(fn)
	frame.env = make(map[ssa.Value]*value.Value)

	if len(fn.Blocks) > 0 {
		frame.block = fn.Blocks[0]
	}

	// 初始化局部变量为零值
	frame.locals = make([]value.Value, len(fn.Locals))
	for i, local := range fn.Locals {
		frame.locals[i] = zero(deref(local.Type()))
		frame.env[local] = &frame.locals[i]
	}

	// 绑定函数参数
	for i, param := range fn.Params {
		frame.env[param] = &args[i]
	}

	// 绑定闭包捕获的自由变量
	for i, freeVar := range fn.FreeVars {
		frame.env[freeVar] = env[i]
	}

	if frame.block != nil {
		runFrame(frame)
	}

	// 释放局部变量引用
	for i := range fn.Locals {
		frame.locals[i] = nil
	}

	return frame.result
}

// callBuiltin 处理内置函数调用（append、copy、close、delete、print、len、cap、panic、recover）。
func callBuiltin(caller *execFrame, pos token.Pos, fn *ssa.Builtin, args []value.Value) value.Value {
	switch fn.Name() {
	case "append":
		if args[1].RValue().IsNil() {
			return args[0]
		}
		elems := make([]reflect.Value, args[1].Elem().Len())
		for i := range elems {
			elems[i] = args[1].RValue().Index(i)
		}
		return value.RValue{Value: reflect.Append(args[0].RValue(), elems...)}

	case "copy":
		n := reflect.Copy(args[0].RValue(), args[1].RValue())
		return value.ValueOf(n)

	case "close":
		args[0].RValue().Close()
		return nil

	case "delete":
		args[0].RValue().SetMapIndex(args[1].RValue(), reflect.Value{})
		return nil

	case "print", "println":
		parts := make([]string, len(args))
		for i, arg := range args {
			parts[i] = fmt.Sprint(arg.Interface())
		}
		position := caller.program.pkg.Prog.Fset.Position(pos)

		if debugMode {
			buf := &caller.ctx.output
			buf.WriteString(fmt.Sprintf("[%s %s:%d] %s\n",
				time.Now().Format("15:04:05"),
				position.Filename,
				position.Line,
				strings.Join(parts, " "),
			))
		}

		logMsg := fmt.Sprint(position.Filename, position.Line, strings.Join(parts, " "))
		utils.LogInfo(caller.traceID, "|", logMsg)
		return nil

	case "len":
		return value.ValueOf(args[0].Len())

	case "cap":
		return value.ValueOf(args[0].Cap())

	case "panic":
		panic(args[0].Interface())

	case "recover":
		if caller.caller.panicking {
			caller.caller.panicking = false
			return value.ValueOf(caller.caller.panicValue)
		}
		return value.ValueOf(recover())
	}

	panic("unknown built-in: " + fn.Name())
}

// deref 解引用指针类型，返回其指向的元素类型。
// 如果不是指针类型则直接返回原类型。
func deref(typ types.Type) types.Type {
	if p, ok := typ.Underlying().(*types.Pointer); ok {
		return p.Elem()
	}
	return typ
}

// zero 返回给定类型的零值（以指针包装形式：reflect.New 返回 *T）。
func zero(t types.Type) value.Value {
	v := reflect.New(typeConvert(t))
	return value.RValue{Value: v}
}

// runFrame 在栈帧中执行程序。
// 逐基本块、逐指令执行，处理正常返回和 panic 恢复。
func runFrame(frame *execFrame) {
	var instr ssa.Instruction

	defer func() {
		if frame.block == nil {
			return // 正常返回
		}

		// panic 恢复：记录 panic 信息并执行 defer，然后跳转到 recover 块
		frame.panicking = true
		pos := frame.program.pkg.Prog.Fset.Position(instr.Pos())
		frame.panicValue = fmt.Errorf("panic at %s: %v", pos.String(), recover())
		frame.runDefers()
		frame.block = frame.fn.Recover
	}()

	for {
		for _, instr = range frame.block.Instrs {
			action := executeInstr(frame, instr)

			// 非调试模式下检查上下文是否已取消（超时或手动取消）
			if !debugMode {
				if err := frame.ctx.Err(); err != nil {
					panic(err)
				}
			}

			switch action {
			case instrReturn:
				return
			case instrNext:
				// 继续执行下一条指令
			case instrJump:
				break
			}
		}
	}
}

// instrAction 表示执行一条指令后的动作。
type instrAction int

const (
	instrNext   instrAction = iota // 继续执行下一条指令
	instrReturn                    // 从函数返回
	instrJump                      // 跳转到另一个基本块
)

// executeInstr 执行单条 SSA 指令，根据指令类型分派到对应的处理函数。
func executeInstr(frame *execFrame, instr ssa.Instruction) instrAction {
	action := instrNext

	switch i := instr.(type) {
	case *ssa.DebugRef:
		// 调试引用，空操作
	case *ssa.Alloc:
		action = execAlloc(frame, i)
	case *ssa.UnOp:
		action = execUnOp(frame, i)
	case *ssa.BinOp:
		action = execBinOp(frame, i)
	case *ssa.MakeInterface:
		action = execMakeInterface(frame, i)
	case *ssa.Return:
		action = execReturn(frame, i)
	case *ssa.IndexAddr:
		action = execIndexAddr(frame, i)
	case *ssa.Field:
		action = execField(frame, i)
	case *ssa.FieldAddr:
		action = execFieldAddr(frame, i)
	case *ssa.Store:
		action = execStore(frame, i)
	case *ssa.Slice:
		action = execSlice(frame, i)
	case *ssa.Call:
		action = execCallInstr(frame, i)
	case *ssa.MakeSlice:
		action = execMakeSlice(frame, i)
	case *ssa.MakeMap:
		action = execMakeMap(frame, i)
	case *ssa.MapUpdate:
		action = execMapUpdate(frame, i)
	case *ssa.Lookup:
		action = execLookup(frame, i)
	case *ssa.Extract:
		action = execExtract(frame, i)
	case *ssa.If:
		action = execIf(frame, i)
	case *ssa.Jump:
		action = execJump(frame, i)
	case *ssa.Phi:
		action = execPhi(frame, i)
	case *ssa.Convert:
		action = execConvert(frame, i)
	case *ssa.Range:
		action = execRange(frame, i)
	case *ssa.Next:
		action = execNext(frame, i)
	case *ssa.ChangeType:
		action = execChangeType(frame, i)
	case *ssa.ChangeInterface:
		action = execChangeInterface(frame, i)
	case *ssa.MakeClosure:
		action = execMakeClosure(frame, i)
	case *ssa.Defer:
		action = execDefer(frame, i)
	case *ssa.RunDefers:
		action = execRunDefers(frame, i)
	case *ssa.MakeChan:
		action = execMakeChan(frame, i)
	case *ssa.Send:
		action = execSend(frame, i)
	case *ssa.TypeAssert:
		action = execTypeAssert(frame, i)
	case *ssa.Go:
		action = execGo(frame, i)
	case *ssa.Panic:
		action = execPanic(frame, i)
	case *ssa.Select:
		action = execSelect(frame, i)
	default:
		panic(fmt.Sprintf("unexpected instruction: %T", instr))
	}

	// 调试模式下输出每条指令的执行信息
	if debugMode {
		pos := frame.program.pkg.Prog.Fset.Position(instr.Pos())
		utils.LogDebugf("exec %s: \t%s \t%T", pos, instr.String(), instr)
		if val, ok := instr.(ssa.Value); ok {
			v := *frame.env[val]
			if v != nil && v.IsValid() {
				utils.LogDebugf("\t\t\t%#v", v.Interface())
			}
		}
	}

	return action
}

// execGo 处理 goroutine 创建指令（go f()）。
func execGo(frame *execFrame, instr *ssa.Go) instrAction {
	goCallFunc(frame, instr.Common())
	return instrNext
}

// goCallFunc 启动一个新的 goroutine 执行函数调用。
// 使用 atomic 计数器跟踪活跃 goroutine 数量，并在 goroutine 内捕获 panic。
func goCallFunc(frame *execFrame, call *ssa.CallCommon) {
	// 处理外部类型上的方法调用
	if call.Signature().Recv() != nil {
		recv := frame.get(call.Args[0])
		if recv.RValue().NumMethod() > 0 {
			args := buildArgs(frame, call.Args[1:])
			go callExternal(recv.RValue().MethodByName(call.Value.Name()), args)
			return
		}
	}

	args := buildArgs(frame, call.Args)

	atomic.AddInt32(&frame.ctx.goroutines, 1)

	go func(caller *execFrame, fn ssa.Value, args []value.Value) {
		defer func() {
			if r := recover(); r != nil {
				caller.ctx.output.WriteString(fmt.Sprintf("goroutine panic: %v", r))
			}
			atomic.AddInt32(&caller.ctx.goroutines, -1)
		}()
		callFunc(caller, call.Pos(), fn, args)
	}(frame, call.Value, args)
}

// execCall 处理函数/方法调用操作。
// 按调用类型分三种情况：普通函数调用、接口方法调用、具体类型方法调用。
func execCall(frame *execFrame, call *ssa.CallCommon) value.Value {
	// 普通函数调用（无接收器）
	if call.Signature().Recv() == nil {
		args := buildArgs(frame, call.Args)
		return callFunc(frame, call.Pos(), call.Value, args)
	}

	// 接口方法调用：通过 reflect 的 MethodByName 分派
	if call.IsInvoke() {
		recv := frame.get(call.Value)
		args := buildArgs(frame, call.Args)
		return callExternal(recv.RValue().MethodByName(call.Method.Name()), args)
	}

	// 具体类型方法调用：如果接收器有方法则用 reflect 调用，否则作为 SSA 函数调用
	args := buildArgs(frame, call.Args)
	if args[0].Type().NumMethod() == 0 {
		return callFunc(frame, call.Pos(), call.Value, args)
	}
	return callExternal(args[0].RValue().MethodByName(call.Value.Name()), args[1:])
}

// buildArgs 将 SSA 值列表转换为运行时值列表。
func buildArgs(frame *execFrame, values []ssa.Value) []value.Value {
	if len(values) == 0 {
		return nil
	}
	args := make([]value.Value, len(values))
	for i, arg := range values {
		args[i] = frame.get(arg)
	}
	return args
}

// callFunc 根据函数类型分派调用。
// 支持 SSA 函数、内置函数、外部值（已注册的原生包函数）、闭包、reflect 函数等。
func callFunc(caller *execFrame, pos token.Pos, fn interface{}, args []value.Value) value.Value {
	switch f := fn.(type) {
	case *ssa.Function:
		if f == nil {
			panic("call of nil function")
		}
		return callSSA(caller, f, args, nil)
	case *ssa.Builtin:
		return callBuiltin(caller, pos, f, args)
	case *value.ExternalValue:
		return callExternal(f.Object.Value, args)
	case ssa.Value:
		ptr := caller.env[f]
		callable := (*ptr).Interface()
		return callFunc(caller, pos, callable, args)
	default:
		return callExternal(reflect.ValueOf(fn), args)
	}
}
