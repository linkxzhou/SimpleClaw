// Package importer 提供外部包导入和类型注册功能。
// 它是 goscript 解释器与原生 Go 包之间的桥梁，
// 负责将原生 Go 类型和对象注册到类型检查系统中。
package importer

import (
	"go/ast"
	"reflect"
)

// ObjectKind 描述外部对象的种类。
type ObjectKind int

const (
	// ObjectUnknown 未知对象类型。
	ObjectUnknown ObjectKind = iota
	// ObjectVar 变量。
	ObjectVar
	// ObjectConst 常量。
	ObjectConst
	// ObjectType 类型定义。
	ObjectType
	// ObjectFunc 函数。
	ObjectFunc
	// ObjectBuiltinFunc 内置函数。
	ObjectBuiltinFunc
)

// ExternalPackage 表示一个可以被脚本导入的外部包。
type ExternalPackage struct {
	Path    string            // 包的导入路径
	Name    string            // 包名
	Objects []*ExternalObject // 包中导出的对象列表
}

// ExternalObject 表示外部包中的一个导出对象（函数、类型、变量或常量）。
type ExternalObject struct {
	Name  string        // 对象名称
	Kind  ObjectKind    // 对象种类
	Value reflect.Value // 对象的运行时值
	Type  reflect.Type  // 对象的运行时类型
	Doc   string        // 文档注释
}

// 包注册表：按路径和按名称索引。
var (
	packagesByPath = make(map[string]*ExternalPackage)    // 按完整导入路径索引
	packagesByName = make(map[string]*ast.ImportSpec)     // 按短包名索引（用于自动导入）
)

// GetPackageByName 根据短包名返回对应的 import 声明。
// 用于源码解析时的自动导入补全。
func GetPackageByName(name string) *ast.ImportSpec {
	return packagesByName[name]
}

// GetAllPackages 返回所有已注册的外部包。
// 用于代码补全等场景，遍历所有可用的包和对象。
func GetAllPackages() map[string]*ExternalPackage {
	return packagesByPath
}
