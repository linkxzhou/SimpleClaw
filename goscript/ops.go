package goscript

// 运算求值：一元运算、常量求值、二元运算。
// 所有求值结果都通过 convert 转换为目标 SSA 类型对应的 reflect 值。

import (
	"fmt"
	"go/constant"
	"go/token"
	"go/types"
	"reflect"

	"github.com/linkxzhou/SimpleClaw/goscript/internal/value"
	"golang.org/x/tools/go/ssa"
)

// evalUnary 求值一元表达式。
// 支持的运算符：
//   - MUL（*）：解引用指针
//   - SUB（-）：取负
//   - XOR（^）：按位取反
//   - NOT（!）：逻辑非
//
// 对于 channel 类型，执行接收操作（<-ch），支持 CommaOk 模式。
func evalUnary(instr *ssa.UnOp, x value.Value) value.Value {
	// 解引用指针
	if instr.Op == token.MUL {
		return value.ValueOf(x.Elem().Interface())
	}

	var result interface{}
	switch x.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch instr.Op {
		case token.SUB:
			result = -x.Int()
		case token.XOR:
			result = ^x.Int()
		default:
			panic(fmt.Sprintf("invalid unary op %s %T", instr.Op, x))
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		switch instr.Op {
		case token.SUB:
			result = -x.Uint()
		case token.XOR:
			result = ^x.Uint()
		default:
			panic(fmt.Sprintf("invalid unary op %s %T", instr.Op, x))
		}

	case reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		switch instr.Op {
		case token.SUB:
			result = -x.Float()
		default:
			panic(fmt.Sprintf("invalid unary op %s %T", instr.Op, x))
		}

	case reflect.Bool:
		switch instr.Op {
		case token.NOT:
			result = !x.Bool()
		default:
			panic(fmt.Sprintf("invalid unary op %s %T", instr.Op, x))
		}

	case reflect.Chan:
		// 从 channel 接收值
		v, ok := x.RValue().Recv()
		if !ok {
			v = reflect.Zero(x.Type().Elem())
		}
		// CommaOk 模式返回 (值, 是否成功) 元组
		if instr.CommaOk {
			return value.ValueOf([]value.Value{value.RValue{Value: v}, value.ValueOf(ok)})
		}
		return value.RValue{Value: v}
	}

	return convert(result, instr.Type())
}

// evalConst 求值 SSA 常量表达式。
// 将 SSA 的 go/constant 常量值转换为解释器内部的 Value 表示。
func evalConst(c *ssa.Const) value.Value {
	if c.IsNil() {
		return zero(c.Type()).Elem() // 类型化的 nil
	}

	// 非基本类型（如结构体、数组）返回零值
	if _, ok := c.Type().Underlying().(*types.Basic); !ok {
		return zero(c.Type()).Elem()
	}

	var val interface{}
	t := c.Type().Underlying().(*types.Basic)

	switch t.Kind() {
	case types.Bool, types.UntypedBool:
		val = constant.BoolVal(c.Value)
	case types.Int, types.UntypedInt, types.Int8, types.Int16, types.Int32, types.UntypedRune, types.Int64:
		val = c.Int64()
	case types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
		val = c.Uint64()
	case types.Float32, types.Float64, types.UntypedFloat:
		val = c.Float64()
	case types.Complex64, types.Complex128, types.UntypedComplex:
		val = c.Complex128()
	case types.String, types.UntypedString:
		if c.Value.Kind() == constant.String {
			val = constant.StringVal(c.Value)
		} else {
			// rune 字面量转换为字符串
			val = string(rune(c.Int64()))
		}
	default:
		panic(fmt.Sprintf("evalConst: %s", c))
	}

	return convert(val, c.Type())
}

// evalBinary 求值二元表达式。
// 根据运算符分派到 binop.go 中对应的具体运算函数，
// 最后将结果转换为指令声明的目标类型。
func evalBinary(instr *ssa.BinOp, x, y value.Value) value.Value {
	var result interface{}

	switch instr.Op {
	case token.ADD:
		result = binaryAdd(x, y)
	case token.SUB:
		result = binarySub(x, y)
	case token.MUL:
		result = binaryMul(x, y)
	case token.QUO:
		result = binaryQuo(x, y)
	case token.REM:
		result = binaryRem(x, y)
	case token.AND:
		result = binaryAnd(x, y)
	case token.OR:
		result = binaryOr(x, y)
	case token.XOR:
		result = binaryXor(x, y)
	case token.AND_NOT:
		result = binaryAndNot(x, y)
	case token.SHL:
		result = binaryShl(x, y)
	case token.SHR:
		result = binaryShr(x, y)
	case token.LSS:
		result = binaryLss(x, y)
	case token.LEQ:
		result = binaryLeq(x, y)
	case token.EQL:
		result = binaryEql(x, y)
	case token.NEQ:
		result = binaryNeq(x, y)
	case token.GTR:
		result = binaryGtr(x, y)
	case token.GEQ:
		result = binaryGeq(x, y)
	}

	return convert(result, instr.Type())
}
