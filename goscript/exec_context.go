package goscript

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleClaw/goscript/internal/value"
	"golang.org/x/tools/go/ssa"
)

// defaultTimeout 是脚本执行的默认超时时间。
const defaultTimeout = 60 * time.Second

// ExecContext 表示一次函数执行的上下文环境。
// 包含超时控制、print 输出捕获、goroutine 计数等运行时状态。
type ExecContext struct {
	context.Context
	output     strings.Builder    // 捕获 print/println 的输出内容
	goroutines int32              // 当前活跃的 goroutine 数量
	cancel     context.CancelFunc // 取消函数，用于超时或手动取消
}

// Output 返回执行过程中通过内置 print/println 输出的内容。
func (c *ExecContext) Output() string {
	return c.output.String()
}

// newExecContext 创建一个带有默认超时的执行上下文。
func newExecContext() *ExecContext {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	return &ExecContext{
		Context: ctx,
		cancel:  cancel,
	}
}

// execFrame 表示函数执行时的栈帧。
// 每次函数调用都会创建一个新的栈帧，保存返回地址、参数、局部变量等信息。
type execFrame struct {
	program    *Program                  // 所属的编译后程序
	caller     *execFrame                // 调用方栈帧（用于 panic/recover 传播）
	fn         *ssa.Function             // 当前执行的 SSA 函数
	block      *ssa.BasicBlock           // 当前基本块
	prevBlock  *ssa.BasicBlock           // 前一个基本块（用于 Phi 节点选择分支）
	env        map[ssa.Value]*value.Value // SSA 值到运行时值的映射表
	locals     []value.Value             // 局部变量数组
	defers     []*ssa.Defer              // defer 调用栈
	result     value.Value               // 函数返回值
	panicking  bool                      // 是否处于 panic 状态
	panicValue interface{}               // panic 的值
	traceID    string                    // 追踪标识符
	ctx        *ExecContext              // 执行上下文
}

// makeFunc 创建函数值或闭包。
// bindings 包含从外层作用域捕获的变量（自由变量）。
func (f *execFrame) makeFunc(fn *ssa.Function, bindings []ssa.Value) value.Value {
	// 捕获绑定的自由变量
	env := make([]*value.Value, len(bindings))
	for i, binding := range bindings {
		env[i] = f.env[binding]
	}

	// 构建函数签名的参数类型
	inTypes := make([]reflect.Type, len(fn.Params))
	for i, param := range fn.Params {
		inTypes[i] = typeConvert(param.Type())
	}

	// 构建函数签名的返回类型
	outTypes := make([]reflect.Type, 0)
	results := fn.Signature.Results()
	for i := 0; i < results.Len(); i++ {
		outTypes = append(outTypes, typeConvert(results.At(i).Type()))
	}

	// 通过 reflect.MakeFunc 创建可调用的函数值
	funcType := reflect.FuncOf(inTypes, outTypes, fn.Signature.Variadic())
	callable := func(in []reflect.Value) (results []reflect.Value) {
		args := make([]value.Value, len(in))
		for i, arg := range in {
			args[i] = value.RValue{Value: arg}
		}
		ret := callSSA(f, fn, args, env)
		if ret != nil {
			return value.Unpackage(ret)
		}
		return nil
	}

	return value.RValue{Value: reflect.MakeFunc(funcType, callable)}
}

// get 根据 SSA 值查找对应的运行时值。
// 处理常量、全局变量、外部值、函数等不同类型的 SSA 值。
func (f *execFrame) get(key ssa.Value) value.Value {
	switch k := key.(type) {
	case nil:
		return nil
	case *ssa.Const:
		return evalConst(k)
	case *ssa.Global:
		if ptr, ok := f.program.globals[k]; ok {
			v := (*ptr).Interface()
			return value.ValueOf(&v)
		}
	case *value.ExternalValue:
		return k.ToValue()
	case *ssa.Function:
		return f.makeFunc(k, nil)
	}

	if ptr, ok := f.env[key]; ok {
		return *ptr
	}
	panic(fmt.Sprintf("get: no value for %T: %v", key, key.Name()))
}

// set 将运行时值存储到栈帧的环境映射中。
func (f *execFrame) set(instr ssa.Value, v value.Value) {
	f.env[instr] = &v
}

// newChild 创建子栈帧，用于函数调用。
// 子栈帧继承程序引用、执行上下文和追踪 ID，但拥有独立的环境。
func (f *execFrame) newChild(fn *ssa.Function) *execFrame {
	return &execFrame{
		program: f.program,
		ctx:     f.ctx,
		caller:  f,
		fn:      fn,
		traceID: f.traceID,
	}
}

// runDefers 按 LIFO（后进先出）顺序执行所有 defer 函数。
// 如果执行后仍处于 panic 状态，则继续向上传播 panic。
func (f *execFrame) runDefers() {
	for i := len(f.defers) - 1; i >= 0; i-- {
		f.runDefer(f.defers[i])
	}
	f.defers = nil
	if f.panicking {
		panic(f.panicValue)
	}
}

// runDefer 执行单个 defer 函数，带有 panic 恢复保护。
// 如果 defer 函数本身 panic，会捕获并记录到栈帧中。
func (f *execFrame) runDefer(d *ssa.Defer) {
	var ok bool
	defer func() {
		if !ok {
			f.panicking = true
			f.panicValue = recover()
		}
	}()
	execCall(f, d.Common())
	ok = true
}
