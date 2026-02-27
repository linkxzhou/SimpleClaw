package goscript

// 类型转换：将 go/types 的静态类型系统映射到 reflect 的运行时类型系统。
// 这是 SSA 解释器的核心桥接层，使编译期类型信息可在运行时使用。

import (
	"go/types"
	"reflect"

	"github.com/linkxzhou/SimpleClaw/goscript/internal/importer"
	"github.com/linkxzhou/SimpleClaw/goscript/internal/value"
)

// builtinTypeMap 将 Go 的 types.BasicKind 映射到对应的 reflect.Type。
// 包含所有基本类型和无类型常量的映射关系。
var builtinTypeMap = map[types.BasicKind]reflect.Type{
	types.Bool:       reflect.TypeOf(true),
	types.Int:        reflect.TypeOf(int(0)),
	types.Int8:       reflect.TypeOf(int8(0)),
	types.Int16:      reflect.TypeOf(int16(0)),
	types.Int32:      reflect.TypeOf(int32(0)),
	types.Int64:      reflect.TypeOf(int64(0)),
	types.Uint:       reflect.TypeOf(uint(0)),
	types.Uint8:      reflect.TypeOf(uint8(0)),
	types.Uint16:     reflect.TypeOf(uint16(0)),
	types.Uint32:     reflect.TypeOf(uint32(0)),
	types.Uint64:     reflect.TypeOf(uint64(0)),
	types.Uintptr:    reflect.TypeOf(uintptr(0)),
	types.Float32:    reflect.TypeOf(float32(0)),
	types.Float64:    reflect.TypeOf(float64(0)),
	types.Complex64:  reflect.TypeOf(complex64(0)),
	types.Complex128: reflect.TypeOf(complex128(0)),
	types.String:     reflect.TypeOf(""),

	// 无类型常量的默认映射
	types.UntypedBool:    reflect.TypeOf(true),
	types.UntypedInt:     reflect.TypeOf(int(0)),
	types.UntypedRune:    reflect.TypeOf(rune(0)),
	types.UntypedFloat:   reflect.TypeOf(float64(0)),
	types.UntypedComplex: reflect.TypeOf(complex128(0)),
	types.UntypedString:  reflect.TypeOf(""),
}

// typeConvert 将 go/types.Type（编译期类型）转换为 reflect.Type（运行时类型）。
// 优先查找外部注册类型，然后根据底层类型递归构建 reflect.Type。
func typeConvert(typ types.Type) reflect.Type {
	// 优先检查是否为外部注册类型
	if rType := importer.GetExternalType(typ); rType != nil {
		return rType
	}

	var rType reflect.Type

	switch t := typ.Underlying().(type) {
	case *types.Array:
		rType = reflect.ArrayOf(int(t.Len()), typeConvert(t.Elem()))

	case *types.Basic:
		if rt := builtinTypeMap[t.Kind()]; rt != nil {
			rType = rt
		} else {
			panic(t.Kind())
		}

	case *types.Chan:
		// 转换 channel 方向
		var dir reflect.ChanDir
		switch t.Dir() {
		case types.RecvOnly:
			dir = reflect.RecvDir
		case types.SendOnly:
			dir = reflect.SendDir
		case types.SendRecv:
			dir = reflect.BothDir
		}
		rType = reflect.ChanOf(dir, typeConvert(t.Elem()))

	case *types.Interface:
		// 接口类型统一映射为 interface{}
		rType = reflect.TypeOf(func(interface{}) {}).In(0)

	case *types.Map:
		rType = reflect.MapOf(typeConvert(t.Key()), typeConvert(t.Elem()))

	case *types.Pointer:
		rType = reflect.PtrTo(typeConvert(t.Elem()))

	case *types.Slice:
		rType = reflect.SliceOf(typeConvert(t.Elem()))

	case *types.Struct:
		// 逐字段构建运行时结构体类型
		fields := make([]reflect.StructField, t.NumFields())
		for i := range fields {
			field := t.Field(i)
			fields[i] = reflect.StructField{
				Name:      field.Name(),
				Type:      typeConvert(t.Field(i).Type()),
				Tag:       reflect.StructTag(t.Tag(i)),
				Offset:    0,
				Index:     []int{i},
				Anonymous: field.Anonymous(),
			}
		}
		rType = reflect.StructOf(fields)

	default:
		// 兜底处理：映射为 interface{}
		rType = reflect.TypeOf(func(interface{}) {}).In(0)
	}

	return rType
}

// convert 将 Go 原生值转换为指定 SSA 类型对应的 Value。
// nil 值返回该类型的零值，非 nil 值通过 reflect.Convert 进行类型转换。
func convert(v interface{}, typ types.Type) value.Value {
	rtype := typeConvert(typ)
	if v == nil {
		return value.RValue{Value: reflect.Zero(rtype)}
	}
	return value.RValue{Value: reflect.ValueOf(v).Convert(rtype)}
}
