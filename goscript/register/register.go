// Package register 提供注册外部包到 goscript 解释器的便捷函数。
// 脚本中可通过 import 语句使用已注册的包。
// 注册必须在编译脚本代码之前完成。
package register

import (
	"reflect"

	"github.com/linkxzhou/SimpleClaw/goscript/internal/importer"
)

// AddPackage 注册一个外部包及其导出对象。
// 必须在解析使用该包的脚本代码之前调用。
func AddPackage(path, name string, objects ...*importer.ExternalObject) {
	importer.RegisterPackage(path, name, objects...)
}

// NewFunction 创建一个函数对象用于注册。
// fn 必须是一个 Go 函数值。
func NewFunction(name string, fn interface{}, doc string) *importer.ExternalObject {
	return &importer.ExternalObject{
		Name:  name,
		Kind:  importer.ObjectFunc,
		Value: reflect.ValueOf(fn),
		Type:  reflect.TypeOf(fn),
		Doc:   doc,
	}
}

// NewVar 创建一个变量对象用于注册。
// addr 必须是变量的指针（&v），typ 是变量的类型。
func NewVar(name string, addr interface{}, typ reflect.Type, doc string) *importer.ExternalObject {
	return &importer.ExternalObject{
		Name:  name,
		Kind:  importer.ObjectVar,
		Value: reflect.ValueOf(addr),
		Type:  typ,
		Doc:   doc,
	}
}

// NewConst 创建一个常量对象用于注册。
// val 是常量的值，类型从值自动推断。
func NewConst(name string, val interface{}, doc string) *importer.ExternalObject {
	return &importer.ExternalObject{
		Name:  name,
		Kind:  importer.ObjectConst,
		Value: reflect.ValueOf(val),
		Type:  reflect.TypeOf(val),
		Doc:   doc,
	}
}

// NewType 创建一个类型对象用于注册。
// typ 通常通过 reflect.TypeOf(func(T){}).In(0) 获取。
func NewType(name string, typ reflect.Type, doc string) *importer.ExternalObject {
	return &importer.ExternalObject{
		Name: name,
		Kind: importer.ObjectType,
		Type: typ,
		Doc:  doc,
	}
}
