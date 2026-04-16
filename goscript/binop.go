package goscript

// 二元运算符实现。
// 通过 isSignedInt/isUnsignedInt/isFloat 辅助函数统一分派，减少重复的 reflect.Kind 枚举。

import (
	"reflect"

	"github.com/linkxzhou/SimpleClaw/goscript/internal/value"
)

// Kind 分类辅助函数，避免每个运算符函数重复枚举所有 Kind。

func isSignedInt(k reflect.Kind) bool {
	return k >= reflect.Int && k <= reflect.Int64
}

func isUnsignedInt(k reflect.Kind) bool {
	return k >= reflect.Uint && k <= reflect.Uintptr
}

func isFloat(k reflect.Kind) bool {
	return k == reflect.Float32 || k == reflect.Float64
}

// 算术运算：通用模式 — 按 kind 分类执行对应运算。

func binaryAdd(x, y value.Value) interface{} {
	k := x.Kind()
	switch {
	case k == reflect.String:
		return x.String() + y.String()
	case isSignedInt(k):
		return x.Int() + y.Int()
	case isFloat(k):
		return x.Float() + y.Float()
	case isUnsignedInt(k):
		return x.Uint() + y.Uint()
	}
	return nil
}

func binarySub(x, y value.Value) interface{} {
	k := x.Kind()
	switch {
	case isSignedInt(k):
		return x.Int() - y.Int()
	case isFloat(k):
		return x.Float() - y.Float()
	case isUnsignedInt(k):
		return x.Uint() - y.Uint()
	}
	return nil
}

func binaryMul(x, y value.Value) interface{} {
	k := x.Kind()
	switch {
	case isSignedInt(k):
		return x.Int() * y.Int()
	case isFloat(k):
		return x.Float() * y.Float()
	case isUnsignedInt(k):
		return x.Uint() * y.Uint()
	}
	return nil
}

func binaryQuo(x, y value.Value) interface{} {
	k := x.Kind()
	switch {
	case isSignedInt(k):
		return x.Int() / y.Int()
	case isFloat(k):
		return x.Float() / y.Float()
	case isUnsignedInt(k):
		return x.Uint() / y.Uint()
	}
	return nil
}

func binaryRem(x, y value.Value) interface{} {
	k := x.Kind()
	switch {
	case isSignedInt(k):
		return x.Int() % y.Int()
	case isUnsignedInt(k):
		return x.Uint() % y.Uint()
	}
	return nil
}

// 位运算：仅适用于整数类型。

func binaryAnd(x, y value.Value) interface{} {
	k := x.Kind()
	switch {
	case isSignedInt(k):
		return x.Int() & y.Int()
	case isUnsignedInt(k):
		return x.Uint() & y.Uint()
	}
	return nil
}

func binaryOr(x, y value.Value) interface{} {
	k := x.Kind()
	switch {
	case isSignedInt(k):
		return x.Int() | y.Int()
	case isUnsignedInt(k):
		return x.Uint() | y.Uint()
	}
	return nil
}

func binaryXor(x, y value.Value) interface{} {
	k := x.Kind()
	switch {
	case isSignedInt(k):
		return x.Int() ^ y.Int()
	case isUnsignedInt(k):
		return x.Uint() ^ y.Uint()
	}
	return nil
}

func binaryAndNot(x, y value.Value) interface{} {
	k := x.Kind()
	switch {
	case isSignedInt(k):
		return x.Int() &^ y.Int()
	case isUnsignedInt(k):
		return x.Uint() &^ y.Uint()
	}
	return nil
}

// 移位运算：右操作数始终为无符号整数。

func binaryShl(x, y value.Value) interface{} {
	k := x.Kind()
	switch {
	case isSignedInt(k):
		return x.Int() << y.Uint()
	case isUnsignedInt(k):
		return x.Uint() << y.Uint()
	}
	return nil
}

func binaryShr(x, y value.Value) interface{} {
	k := x.Kind()
	switch {
	case isSignedInt(k):
		return x.Int() >> y.Uint()
	case isUnsignedInt(k):
		return x.Uint() >> y.Uint()
	}
	return nil
}

// 比较运算：支持字符串、整数、浮点数和无符号整数。

func binaryLss(x, y value.Value) interface{} {
	k := x.Kind()
	switch {
	case k == reflect.String:
		return x.String() < y.String()
	case isSignedInt(k):
		return x.Int() < y.Int()
	case isFloat(k):
		return x.Float() < y.Float()
	case isUnsignedInt(k):
		return x.Uint() < y.Uint()
	}
	return nil
}

func binaryLeq(x, y value.Value) interface{} {
	k := x.Kind()
	switch {
	case k == reflect.String:
		return x.String() <= y.String()
	case isSignedInt(k):
		return x.Int() <= y.Int()
	case isFloat(k):
		return x.Float() <= y.Float()
	case isUnsignedInt(k):
		return x.Uint() <= y.Uint()
	}
	return nil
}

func binaryEql(x, y value.Value) interface{} {
	if x.IsNil() || y.IsNil() {
		return x.IsNil() && y.IsNil()
	}
	return x.Interface() == y.Interface()
}

func binaryNeq(x, y value.Value) interface{} {
	if x.IsNil() || y.IsNil() {
		return x.IsNil() != y.IsNil()
	}
	return x.Interface() != y.Interface()
}

func binaryGtr(x, y value.Value) interface{} {
	k := x.Kind()
	switch {
	case k == reflect.String:
		return x.String() > y.String()
	case isSignedInt(k):
		return x.Int() > y.Int()
	case isFloat(k):
		return x.Float() > y.Float()
	case isUnsignedInt(k):
		return x.Uint() > y.Uint()
	}
	return nil
}

func binaryGeq(x, y value.Value) interface{} {
	k := x.Kind()
	switch {
	case k == reflect.String:
		return x.String() >= y.String()
	case isSignedInt(k):
		return x.Int() >= y.Int()
	case isFloat(k):
		return x.Float() >= y.Float()
	case isUnsignedInt(k):
		return x.Uint() >= y.Uint()
	}
	return nil
}
