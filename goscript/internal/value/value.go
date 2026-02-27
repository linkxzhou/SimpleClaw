// Package value 提供解释器运行时的值表示。
// 定义了 Value 接口及其实现（RValue、ExternalValue），
// 以及值的打包/解包和外部值包装功能。
package value

import (
	"reflect"
	"strings"

	"github.com/linkxzhou/SimpleClaw/goscript/internal/importer"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// Value 表示解释器中的运行时值。
// 所有解释器内部的值操作都通过此接口进行，
// 屏蔽了 reflect.Value 的底层细节。
type Value interface {
	Elem() Value                // 解引用指针或接口
	Interface() interface{}     // 转换为 Go 原生值
	String() string             // 字符串表示
	Int() int64                 // 有符号整数值
	Uint() uint64               // 无符号整数值
	Float() float64             // 浮点数值
	Index(i int) Value          // 数组/切片元素访问
	MapIndex(v Value) Value     // map 键值访问
	Set(Value)                  // 设置值
	Len() int                   // 长度
	Cap() int                   // 容量
	Type() reflect.Type         // 运行时类型
	IsValid() bool              // 是否有效
	IsNil() bool                // 是否为 nil
	Bool() bool                 // 布尔值
	Field(i int) Value          // 结构体字段访问
	Next() Value                // 迭代器下一个元素
	Kind() reflect.Kind         // 值的种类
	RValue() reflect.Value      // 获取底层 reflect.Value
}

// ExternalValue 表示来自已导入外部包的变量。
// 它包装了 SSA 值和对应的外部对象引用，
// 使解释器能够读写外部包的全局变量。
type ExternalValue struct {
	ssa.Value                           // 嵌入 SSA 值（用于类型信息）
	Object *importer.ExternalObject     // 对应的外部对象
}

// Store 更新外部变量的值。
func (v *ExternalValue) Store(val Value) {
	v.Object.Value.Elem().Set(val.RValue())
}

// ToValue 将外部变量转换为 Value 接口。
func (v *ExternalValue) ToValue() Value {
	return RValue{v.Object.Value}
}

// Interface 返回外部对象本身。
func (v *ExternalValue) Interface() interface{} {
	return v.Object
}

// RValue 包装 reflect.Value 以实现 Value 接口。
// 这是解释器中最常用的值类型，大多数运算结果都以 RValue 表示。
type RValue struct {
	reflect.Value
}

// RValue 返回底层的 reflect.Value。
func (v RValue) RValue() reflect.Value {
	return v.Value
}

// Next 实现 Value 接口（RValue 不支持迭代）。
func (v RValue) Next() Value {
	panic("Next not implemented for RValue")
}

// Field 返回结构体的第 i 个字段。
func (v RValue) Field(i int) Value {
	return RValue{v.Value.Field(i)}
}

// MapIndex 根据键返回 map 中的值。
func (v RValue) MapIndex(key Value) Value {
	return RValue{v.Value.MapIndex(key.RValue())}
}

// Set 设置值。
func (v RValue) Set(val Value) {
	v.Value.Set(val.RValue())
}

// Index 返回数组/切片的第 i 个元素。
func (v RValue) Index(i int) Value {
	return RValue{v.Value.Index(i)}
}

// Elem 解引用指针和接口。
// 对于非指针/非接口类型，返回自身。
func (v RValue) Elem() Value {
	if v.Value.Kind() == reflect.Ptr || v.Value.Kind() == reflect.Interface {
		return RValue{v.Value.Elem()}
	}
	return v
}

// IsNil 检查值是否为 nil。
// 只有 channel、函数、map、指针、unsafe pointer、接口和切片类型才可能为 nil。
func (v RValue) IsNil() bool {
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Ptr, reflect.UnsafePointer, reflect.Interface, reflect.Slice:
		return v.Value.IsNil()
	default:
		return false
	}
}

// MapIter 表示 map 迭代器，用于 for range 遍历 map。
type MapIter struct {
	I int              // 当前迭代位置
	Value              // 被迭代的 map 值
	Keys []reflect.Value // map 的所有键
}

// Next 返回 map 迭代器的下一个键值对。
// 返回值为 [ok, key, value] 三元组，ok 为 false 时表示迭代结束。
func (m *MapIter) Next() Value {
	result := make([]Value, 3)

	if m.I < len(m.Keys) {
		key := RValue{m.Keys[m.I]}
		result[0] = ValueOf(true)
		result[1] = key
		result[2] = m.MapIndex(key)
		m.I++
	} else {
		result[0] = ValueOf(false)
	}

	return ValueOf(result)
}

// ValueOf 将任意 Go 值包装为 Value。
func ValueOf(v interface{}) Value {
	return RValue{reflect.ValueOf(v)}
}

// Package 将多个 reflect.Value 打包为单个 Value。
// 用于处理多返回值函数的结果：
//   - 0 个值返回 nil
//   - 1 个值直接包装为 RValue
//   - 多个值包装为 []Value 切片
func Package(values []reflect.Value) Value {
	switch len(values) {
	case 0:
		return nil
	case 1:
		return RValue{values[0]}
	default:
		result := make([]Value, len(values))
		for i, v := range values {
			result[i] = RValue{v}
		}
		return ValueOf(result)
	}
}

// Unpackage 从 Value 中提取 reflect.Value 切片。
// 是 Package 的逆操作，用于将打包的多返回值拆分为独立的 reflect.Value。
func Unpackage(val Value) []reflect.Value {
	if val == nil {
		return nil
	}

	if arr, ok := val.Interface().([]Value); ok {
		result := make([]reflect.Value, len(arr))
		for i, v := range arr {
			result[i] = v.RValue()
		}
		return result
	}

	return []reflect.Value{val.RValue()}
}

// ExternalValueWrap 遍历 SSA 包中所有函数的所有指令操作数，
// 将引用外部包对象的 SSA 值替换为 ExternalValue 包装。
// 这使得解释器在运行时能识别并正确处理对外部包的引用。
func ExternalValueWrap(imp *importer.Importer, pkg *ssa.Package) {
	for fn := range ssautil.AllFunctions(pkg.Prog) {
		for _, block := range fn.Blocks {
			for _, instr := range block.Instrs {
				for _, operand := range instr.Operands(nil) {
					wrapExternalValue(imp, operand)
				}
			}
		}
	}
}

// wrapExternalValue 检查单个 SSA 操作数是否引用外部对象。
// 如果是，则将其替换为 ExternalValue；
// 如果引用的是脚本内部的 SSA 包成员，则替换为对应的 SSA 成员值。
func wrapExternalValue(imp *importer.Importer, v *ssa.Value) {
	if *v == nil {
		return
	}

	// 解析操作数名称：去除前缀 *&，提取 "包名.成员名"
	name := strings.TrimLeft((*v).String(), "*&")
	dotIndex := strings.IndexRune(name, '.')
	if dotIndex < 0 {
		return
	}

	pkgName := name[:dotIndex]
	memberName := name[dotIndex+1:]

	// 优先检查是否为脚本内部 SSA 包的成员
	if pkg := imp.SSAPackage(pkgName); pkg != nil {
		if member, ok := pkg.Members[memberName].(ssa.Value); ok {
			*v = member
			return
		}
	}

	// 检查是否为已注册的外部对象
	if external := imp.ExternalObject(name); external != nil {
		*v = &ExternalValue{
			Value:  *v,
			Object: external,
		}
	}
}
