package goscript

// 二元运算符实现。
// 每个函数根据操作数的 reflect.Kind 分派到对应的数值类型运算。

import (
	"reflect"

	"github.com/linkxzhou/SimpleClaw/goscript/internal/value"
)

// binaryAdd 执行加法运算（+）。
// 支持字符串拼接、有符号整数、浮点数和无符号整数。
func binaryAdd(x, y value.Value) interface{} {
	switch x.Kind() {
	case reflect.String:
		return x.String() + y.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return x.Int() + y.Int()
	case reflect.Float32, reflect.Float64:
		return x.Float() + y.Float()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return x.Uint() + y.Uint()
	}
	return nil
}

// binarySub 执行减法运算（-）。
func binarySub(x, y value.Value) interface{} {
	switch x.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return x.Int() - y.Int()
	case reflect.Float32, reflect.Float64:
		return x.Float() - y.Float()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return x.Uint() - y.Uint()
	}
	return nil
}

// binaryMul 执行乘法运算（*）。
func binaryMul(x, y value.Value) interface{} {
	switch x.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return x.Int() * y.Int()
	case reflect.Float32, reflect.Float64:
		return x.Float() * y.Float()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return x.Uint() * y.Uint()
	}
	return nil
}

// binaryQuo 执行除法运算（/）。
func binaryQuo(x, y value.Value) interface{} {
	switch x.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return x.Int() / y.Int()
	case reflect.Float32, reflect.Float64:
		return x.Float() / y.Float()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return x.Uint() / y.Uint()
	}
	return nil
}

// binaryRem 执行取余运算（%）。仅适用于整数类型。
func binaryRem(x, y value.Value) interface{} {
	switch x.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return x.Int() % y.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return x.Uint() % y.Uint()
	}
	return nil
}

// binaryAnd 执行按位与运算（&）。
func binaryAnd(x, y value.Value) interface{} {
	switch x.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return x.Int() & y.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return x.Uint() & y.Uint()
	}
	return nil
}

// binaryOr 执行按位或运算（|）。
func binaryOr(x, y value.Value) interface{} {
	switch x.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return x.Int() | y.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return x.Uint() | y.Uint()
	}
	return nil
}

// binaryXor 执行按位异或运算（^）。
func binaryXor(x, y value.Value) interface{} {
	switch x.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return x.Int() ^ y.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return x.Uint() ^ y.Uint()
	}
	return nil
}

// binaryAndNot 执行位清除运算（&^）。将 x 中 y 为 1 的位清零。
func binaryAndNot(x, y value.Value) interface{} {
	switch x.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return x.Int() &^ y.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return x.Uint() &^ y.Uint()
	}
	return nil
}

// binaryShl 执行左移运算（<<）。右操作数始终为无符号整数。
func binaryShl(x, y value.Value) interface{} {
	switch x.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return x.Int() << y.Uint()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return x.Uint() << y.Uint()
	}
	return nil
}

// binaryShr 执行右移运算（>>）。右操作数始终为无符号整数。
func binaryShr(x, y value.Value) interface{} {
	switch x.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return x.Int() >> y.Uint()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return x.Uint() >> y.Uint()
	}
	return nil
}

// binaryLss 执行小于比较（<）。支持字符串、整数、浮点数和无符号整数。
func binaryLss(x, y value.Value) interface{} {
	switch x.Kind() {
	case reflect.String:
		return x.String() < y.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return x.Int() < y.Int()
	case reflect.Float32, reflect.Float64:
		return x.Float() < y.Float()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return x.Uint() < y.Uint()
	}
	return nil
}

// binaryLeq 执行小于等于比较（<=）。
func binaryLeq(x, y value.Value) interface{} {
	switch x.Kind() {
	case reflect.String:
		return x.String() <= y.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return x.Int() <= y.Int()
	case reflect.Float32, reflect.Float64:
		return x.Float() <= y.Float()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return x.Uint() <= y.Uint()
	}
	return nil
}

// binaryEql 执行相等比较（==）。
// 特殊处理 nil 值：两个 nil 相等，nil 与非 nil 不等。
func binaryEql(x, y value.Value) interface{} {
	if x.IsNil() || y.IsNil() {
		return x.IsNil() && y.IsNil()
	}
	return x.Interface() == y.Interface()
}

// binaryNeq 执行不等比较（!=）。
// 特殊处理 nil 值：两个 nil 相等（返回 false），nil 与非 nil 不等（返回 true）。
func binaryNeq(x, y value.Value) interface{} {
	if x.IsNil() || y.IsNil() {
		return x.IsNil() != y.IsNil()
	}
	return x.Interface() != y.Interface()
}

// binaryGtr 执行大于比较（>）。
func binaryGtr(x, y value.Value) interface{} {
	switch x.Kind() {
	case reflect.String:
		return x.String() > y.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return x.Int() > y.Int()
	case reflect.Float32, reflect.Float64:
		return x.Float() > y.Float()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return x.Uint() > y.Uint()
	}
	return nil
}

// binaryGeq 执行大于等于比较（>=）。
func binaryGeq(x, y value.Value) interface{} {
	switch x.Kind() {
	case reflect.String:
		return x.String() >= y.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return x.Int() >= y.Int()
	case reflect.Float32, reflect.Float64:
		return x.Float() >= y.Float()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return x.Uint() >= y.Uint()
	}
	return nil
}
