package goscript

import (
	"fmt"
	"go/types"
	"reflect"

	"github.com/linkxzhou/SimpleClaw/goscript/internal/value"
	"golang.org/x/tools/go/ssa"
)

// execAlloc 处理内存分配指令。
// Heap 分配在堆上创建新的 value.Value 指针；栈分配则复用已有的 env 槽位。
func execAlloc(frame *execFrame, instr *ssa.Alloc) instrAction {
	var addr *value.Value
	if instr.Heap {
		addr = new(value.Value)
		frame.env[instr] = addr
	} else {
		addr = frame.env[instr]
	}
	*addr = zero(deref(instr.Type()))
	return instrNext
}

// execUnOp 处理一元运算指令（如取负、按位取反、解引用等）。
func execUnOp(frame *execFrame, instr *ssa.UnOp) instrAction {
	v := evalUnary(instr, frame.get(instr.X))
	frame.set(instr, v)
	return instrNext
}

// execBinOp 处理二元运算指令（如加减乘除、比较、位运算等）。
func execBinOp(frame *execFrame, instr *ssa.BinOp) instrAction {
	v := evalBinary(instr, frame.get(instr.X), frame.get(instr.Y))
	frame.set(instr, v)
	return instrNext
}

// execMakeInterface 处理接口创建指令。
// 将具体类型值包装为 interface{} 值。
func execMakeInterface(frame *execFrame, instr *ssa.MakeInterface) instrAction {
	v := frame.get(instr.X)
	frame.set(instr, v)
	return instrNext
}

// execReturn 处理函数返回指令。
// 支持无返回值、单返回值和多返回值（打包为 []value.Value）三种情况。
func execReturn(frame *execFrame, instr *ssa.Return) instrAction {
	switch len(instr.Results) {
	case 0:
		// 无返回值
	case 1:
		frame.result = frame.get(instr.Results[0])
	default:
		// 将多个返回值打包
		results := make([]value.Value, len(instr.Results))
		for i, r := range instr.Results {
			results[i] = frame.get(r)
		}
		frame.result = value.ValueOf(results)
	}
	frame.block = nil
	return instrReturn
}

// execIndexAddr 处理数组/切片的索引取地址操作（&a[i]）。
func execIndexAddr(frame *execFrame, instr *ssa.IndexAddr) instrAction {
	x := frame.get(instr.X)
	idx := int(frame.get(instr.Index).Int())
	frame.set(instr, value.RValue{Value: x.Elem().RValue().Index(idx).Addr()})
	return instrNext
}

// execField 处理结构体字段读取操作（s.Field）。
func execField(frame *execFrame, instr *ssa.Field) instrAction {
	x := frame.get(instr.X)
	frame.set(instr, x.Field(instr.Field))
	return instrNext
}

// execFieldAddr 处理结构体字段取地址操作（&s.Field）。
func execFieldAddr(frame *execFrame, instr *ssa.FieldAddr) instrAction {
	x := frame.get(instr.X).Elem()
	v := x.RValue().Field(instr.Field).Addr()
	frame.set(instr, value.RValue{Value: v})
	return instrNext
}

// execStore 处理内存存储指令。
// 根据目标地址类型分别处理：局部变量、全局变量、外部变量、数组元素、通用指针。
func execStore(frame *execFrame, instr *ssa.Store) instrAction {
	switch addr := instr.Addr.(type) {
	case *ssa.Alloc, *ssa.FreeVar:
		// 局部变量存储
		v := frame.get(instr.Val)
		(*frame.env[addr]).Elem().Set(v)
	case *ssa.Global:
		// 全局变量存储
		v := frame.get(instr.Val)
		frame.program.globals[addr] = &v
	case *value.ExternalValue:
		// 外部包变量存储
		v := frame.get(instr.Val)
		addr.Store(v)
	case *ssa.IndexAddr:
		// 数组/切片元素存储
		index := int(frame.get(addr.Index).Int())
		x := frame.get(addr.X).Elem()
		val := frame.get(instr.Val)
		x.Index(index).Set(val)
	default:
		// 通过指针存储
		v := *frame.env[addr]
		v.Elem().Set(frame.get(instr.Val))
	}
	return instrNext
}

// execSlice 处理切片操作（s[lo:hi] 和 s[lo:hi:max]）。
func execSlice(frame *execFrame, instr *ssa.Slice) instrAction {
	x := frame.get(instr.X).Elem()
	lo, hi := 0, x.Len()

	if low := frame.get(instr.Low); low != nil {
		lo = int(low.Int())
	}
	if high := frame.get(instr.High); high != nil {
		hi = int(high.Int())
	}

	if max := frame.get(instr.Max); max != nil {
		frame.set(instr, value.RValue{Value: x.RValue().Slice3(lo, hi, int(max.Int()))})
	} else {
		frame.set(instr, value.RValue{Value: x.RValue().Slice(lo, hi)})
	}
	return instrNext
}

// execCallInstr 处理函数调用指令。
func execCallInstr(frame *execFrame, instr *ssa.Call) instrAction {
	if v := execCall(frame, instr.Common()); v != nil {
		frame.env[instr] = &v
	}
	return instrNext
}

// execMakeSlice 处理切片创建指令（make([]T, len, cap)）。
func execMakeSlice(frame *execFrame, instr *ssa.MakeSlice) instrAction {
	length := int(frame.get(instr.Len).Int())
	capacity := int(frame.get(instr.Cap).Int())
	frame.set(instr, value.RValue{Value: reflect.MakeSlice(typeConvert(instr.Type()), length, capacity)})
	return instrNext
}

// execMakeMap 处理 map 创建指令（make(map[K]V)）。
func execMakeMap(frame *execFrame, instr *ssa.MakeMap) instrAction {
	frame.set(instr, value.RValue{Value: reflect.MakeMap(typeConvert(instr.Type()))})
	return instrNext
}

// execMapUpdate 处理 map 更新操作（m[key] = value）。
func execMapUpdate(frame *execFrame, instr *ssa.MapUpdate) instrAction {
	m := frame.get(instr.Map)
	key := frame.get(instr.Key)
	val := frame.get(instr.Value)
	m.Elem().RValue().SetMapIndex(key.RValue(), val.RValue())
	return instrNext
}

// execLookup 处理 map 查找和字符串索引操作。
// 对于 map：返回值和 ok 布尔值（CommaOk 模式）。
// 对于字符串：返回指定位置的字节。
func execLookup(frame *execFrame, instr *ssa.Lookup) instrAction {
	x := frame.get(instr.X)
	index := frame.get(instr.Index)

	if x.Type().Kind() == reflect.Map {
		v := x.MapIndex(index)
		ok := true
		if !v.IsValid() {
			v = value.RValue{Value: reflect.Zero(x.Type().Elem())}
			ok = false
		}
		if instr.CommaOk {
			v = value.ValueOf([]value.Value{v, value.ValueOf(ok)})
		}
		frame.set(instr, v)
	} else {
		// 字符串索引
		frame.set(instr, x.Index(int(index.Int())))
	}
	return instrNext
}

// execExtract 处理元组提取操作。
// 用于从多返回值中提取单个值（如 v, ok := m[key] 中的 v 或 ok）。
func execExtract(frame *execFrame, instr *ssa.Extract) instrAction {
	frame.set(instr, frame.get(instr.Tuple).Index(instr.Index).Interface().(value.Value))
	return instrNext
}

// execIf 处理条件分支指令。
// 根据条件值跳转到 true 分支（Succs[0]）或 false 分支（Succs[1]）。
func execIf(frame *execFrame, instr *ssa.If) instrAction {
	succ := 1
	if frame.get(instr.Cond).Bool() {
		succ = 0
	}
	frame.prevBlock, frame.block = frame.block, frame.block.Succs[succ]
	return instrJump
}

// execJump 处理无条件跳转指令。
func execJump(frame *execFrame, instr *ssa.Jump) instrAction {
	frame.prevBlock, frame.block = frame.block, frame.block.Succs[0]
	return instrJump
}

// execPhi 处理 Phi 节点指令。
// Phi 节点是 SSA 的核心概念：在控制流汇合点，根据来源基本块选择对应的值。
func execPhi(frame *execFrame, instr *ssa.Phi) instrAction {
	for i, pred := range instr.Block().Preds {
		if frame.prevBlock == pred {
			frame.set(instr, frame.get(instr.Edges[i]))
			break
		}
	}
	return instrNext
}

// execConvert 处理类型转换指令（如 int(x)、string(x) 等显式转换）。
func execConvert(frame *execFrame, instr *ssa.Convert) instrAction {
	frame.set(instr, convert(frame.get(instr.X).Interface(), instr.Type()))
	return instrNext
}

// execRange 处理 range 迭代初始化指令。
// 为 map 创建一个迭代器，记录所有 key 和当前位置。
func execRange(frame *execFrame, instr *ssa.Range) instrAction {
	v := frame.get(instr.X)
	frame.set(instr, &value.MapIter{
		I:     0,
		Value: v,
		Keys:  v.RValue().MapKeys(),
	})
	return instrNext
}

// execNext 处理 range 迭代步进指令。
// 返回下一个迭代元素（key-value 对）。
func execNext(frame *execFrame, instr *ssa.Next) instrAction {
	frame.set(instr, frame.get(instr.Iter).Next())
	return instrNext
}

// execChangeType 处理类型变更指令（底层类型相同，如 type MyInt int 与 int 之间的转换）。
func execChangeType(frame *execFrame, instr *ssa.ChangeType) instrAction {
	frame.set(instr, frame.get(instr.X))
	return instrNext
}

// execChangeInterface 处理接口转换指令（将一个接口类型转换为另一个兼容的接口类型）。
func execChangeInterface(frame *execFrame, instr *ssa.ChangeInterface) instrAction {
	frame.set(instr, frame.get(instr.X))
	return instrNext
}

// execMakeClosure 处理闭包创建指令。
// 将函数与其捕获的自由变量绑定，生成闭包值。
func execMakeClosure(frame *execFrame, instr *ssa.MakeClosure) instrAction {
	closure := frame.makeFunc(instr.Fn.(*ssa.Function), instr.Bindings)
	frame.set(instr, closure)
	return instrNext
}

// execDefer 处理 defer 语句。
// 将 defer 调用压入栈帧的 defer 栈，在函数返回时按 LIFO 顺序执行。
func execDefer(frame *execFrame, instr *ssa.Defer) instrAction {
	frame.defers = append(frame.defers, instr)
	return instrNext
}

// execRunDefers 处理 defer 函数的批量执行指令。
func execRunDefers(frame *execFrame, instr *ssa.RunDefers) instrAction {
	frame.runDefers()
	return instrNext
}

// execMakeChan 处理 channel 创建指令（make(chan T, size)）。
func execMakeChan(frame *execFrame, instr *ssa.MakeChan) instrAction {
	frame.set(instr, value.RValue{Value: reflect.MakeChan(typeConvert(instr.Type()), int(frame.get(instr.Size).Int()))})
	return instrNext
}

// execSend 处理 channel 发送操作（ch <- x）。
func execSend(frame *execFrame, instr *ssa.Send) instrAction {
	frame.get(instr.Chan).RValue().Send(frame.get(instr.X).RValue())
	return instrNext
}

// execTypeAssert 处理类型断言操作（x.(T) 和 x.(T) 的 CommaOk 形式）。
// 支持四种组合：CommaOk/非CommaOk × 可赋值/不可赋值。
func execTypeAssert(frame *execFrame, instr *ssa.TypeAssert) instrAction {
	v := frame.get(instr.X)
	for v.Kind() == reflect.Interface {
		v = v.Elem()
	}
	destType := typeConvert(instr.AssertedType)

	var assignable bool
	if v.Kind() == reflect.Invalid {
		assignable = false
	} else {
		assignable = v.Type().AssignableTo(destType)
	}

	switch {
	case instr.CommaOk && assignable:
		frame.set(instr, value.ValueOf([]value.Value{v, value.ValueOf(true)}))
	case instr.CommaOk && !assignable:
		frame.set(instr, value.ValueOf([]value.Value{value.RValue{Value: reflect.Zero(destType)}, value.ValueOf(false)}))
	case !instr.CommaOk && assignable:
		frame.set(instr, v)
	case !instr.CommaOk && !assignable:
		if v.Kind() == reflect.Invalid {
			panic(fmt.Errorf("interface conversion: interface is nil, not %s", destType.String()))
		} else {
			panic(fmt.Errorf("interface conversion: interface is %s, not %s", v.Type().String(), destType.String()))
		}
	}
	return instrNext
}

// execPanic 处理 panic 指令。
func execPanic(frame *execFrame, instr *ssa.Panic) instrAction {
	panic(frame.get(instr.X).Interface())
}

// execSelect 处理 select 语句。
// 使用 reflect.Select 在多个 channel 操作中选择一个就绪的分支。
// 支持 blocking（无 default）和 non-blocking（有 default）两种模式。
func execSelect(frame *execFrame, instr *ssa.Select) instrAction {
	cases := make([]reflect.SelectCase, 0, len(instr.States)+1)

	// 非阻塞模式添加 default case
	if !instr.Blocking {
		cases = append(cases, reflect.SelectCase{
			Dir: reflect.SelectDefault,
		})
	}

	// 构建 select case 列表
	for _, state := range instr.States {
		var dir reflect.SelectDir
		if state.Dir == types.RecvOnly {
			dir = reflect.SelectRecv
		} else {
			dir = reflect.SelectSend
		}
		var send reflect.Value
		if state.Send != nil {
			send = frame.get(state.Send).RValue()
		}
		chanValue := frame.get(state.Chan).RValue()
		cases = append(cases, reflect.SelectCase{
			Dir:  dir,
			Chan: chanValue,
			Send: send,
		})
	}

	chosen, recv, recvOk := reflect.Select(cases)
	if !instr.Blocking {
		chosen-- // default case 的索引应为 -1
	}

	// 组装结果：[chosen_index, recvOk, recv_values...]
	result := []value.Value{value.ValueOf(chosen), value.ValueOf(recvOk)}
	for i, st := range instr.States {
		if st.Dir == types.RecvOnly {
			var v value.Value
			if i == chosen && recvOk {
				v = value.RValue{Value: recv}
			} else {
				v = zero(st.Chan.Type().Underlying().(*types.Chan).Elem())
			}
			result = append(result, v)
		}
	}
	frame.set(instr, value.ValueOf(result))
	return instrNext
}
