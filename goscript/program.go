// Package goscript 提供基于 SSA（静态单赋值）的 Go 脚本解释器。
// 将 Go 源码编译为 SSA 中间表示，然后在运行时逐指令解释执行。
package goscript

import (
	"errors"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"go/types"
	"os"

	"github.com/linkxzhou/SimpleClaw/goscript/internal/importer"
	"github.com/linkxzhou/SimpleClaw/goscript/internal/value"
	_ "github.com/linkxzhou/SimpleClaw/goscript/packages"
	"github.com/linkxzhou/SimpleClaw/utils"

	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// Program 表示一个已编译的 Go 程序，可以直接执行。
type Program struct {
	pkg          *ssa.Package               // 主 SSA 包
	globals      map[ssa.Value]*value.Value // 全局变量地址映射
	importedPkgs []string                   // 已导入的包路径列表
}

// ParseFunctions 从源码中提取函数名列表。
// 当 exportedOnly 为 false 时返回所有函数；为 true 时仅返回导出函数（排除 init）。
func ParseFunctions(source string, exportedOnly bool) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", source, parser.AllErrors)
	if err != nil {
		utils.LogError("ParseFile failed for source")
		return nil, err
	}

	var functions []string
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Recv != nil {
			continue // 跳过非函数声明和方法
		}

		name := funcDecl.Name.Name
		if exportedOnly {
			if funcDecl.Name.IsExported() && name != "init" {
				functions = append(functions, name)
			}
		} else {
			functions = append(functions, name)
		}
	}
	return functions, nil
}

// Run 一步完成编译和执行：编译源码后直接调用指定函数。
func Run(traceID, source, funcName string, args ...interface{}) (interface{}, error) {
	prog, err := Compile(traceID, "main", source)
	if err != nil {
		return nil, err
	}
	return prog.Run(traceID, funcName, args...)
}

// autoImport 自动为未解析的标识符添加 import 语句。
// 扫描 AST 中的 Unresolved 列表，如果是已注册的外部包则自动补充导入。
func autoImport(file *ast.File) []string {
	var importedPkgs []string
	imported := make(map[string]bool)

	// 记录已有的 import
	for _, imp := range file.Imports {
		imported[imp.Path.Value] = true
		importedPkgs = append(importedPkgs, imp.Path.Value)
	}

	// 为未解析的标识符补充 import
	for _, unresolved := range file.Unresolved {
		if doc.IsPredeclared(unresolved.Name) {
			continue
		}
		importSpec := importer.GetPackageByName(unresolved.Name)
		if importSpec == nil || imported[importSpec.Path.Value] {
			continue
		}
		imported[importSpec.Path.Value] = true
		file.Imports = append(file.Imports, importSpec)
		file.Decls = append(file.Decls, &ast.GenDecl{
			Specs: []ast.Spec{importSpec},
		})
	}
	return importedPkgs
}

// Compile 将 Go 源码编译为可执行的 Program。
// 流程：解析源码 → 自动补充 import → 类型检查 → 构建 SSA → 包装外部值 → 初始化全局变量 → 执行 init 函数。
// dependencies 参数用于导入其他已编译的 SSA 包（跨包依赖）。
func Compile(traceID, pkgName, source string, dependencies ...*ssa.Package) (*Program, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, pkgName+".go", source, parser.AllErrors)
	if err != nil {
		return nil, err
	}

	importedPkgs := autoImport(file)
	pkg := types.NewPackage(file.Name.Name, file.Name.Name)

	pkgImporter := importer.NewImporter(dependencies...)
	mode := ssa.SanityCheckFunctions | ssa.BareInits
	ssaPkg, _, err := ssautil.BuildPackage(
		&types.Config{Importer: pkgImporter}, fset, pkg, []*ast.File{file}, mode)
	if err != nil {
		return nil, err
	}

	prog := &Program{
		pkg:          ssaPkg,
		globals:      make(map[ssa.Value]*value.Value),
		importedPkgs: importedPkgs,
	}

	// 将外部包的值包装为 ExternalValue，以便 SSA 指令执行时能正确引用
	value.ExternalValueWrap(pkgImporter, ssaPkg)
	prog.initGlobals()

	// 执行 init 函数（先执行依赖包的 init，再执行本包的 init）
	ctx := newExecContext()
	frame := &execFrame{
		program: prog,
		ctx:     ctx,
		traceID: traceID,
	}

	if initFn := ssaPkg.Func("init"); initFn != nil {
		// 先初始化依赖包
		for _, dep := range dependencies {
			if depInit := dep.Func("init"); depInit != nil {
				callSSA(frame, depInit, nil, nil)
			}
		}
		callSSA(frame, initFn, nil, nil)
	}
	ctx.cancel()

	return prog, nil
}

// Run 按函数名执行，传入参数并返回结果。
func (p *Program) Run(traceID, funcName string, args ...interface{}) (interface{}, error) {
	result, _, err := p.RunWithContext(traceID, funcName, args...)
	return result, err
}

// RunWithContext 执行指定函数，同时返回执行上下文（可获取 print 输出等信息）。
// 内部通过 defer+recover 捕获 panic，确保不会因运行时错误导致宿主程序崩溃。
func (p *Program) RunWithContext(traceID, funcName string, args ...interface{}) (interface{}, *ExecContext, error) {
	var err error
	var result interface{}

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic recovered: %v", r)
		}
	}()

	ctx := newExecContext()
	fn := p.pkg.Func(funcName)
	if fn == nil {
		ctx.cancel()
		return nil, nil, errors.New("function not found: " + funcName)
	}

	if debugMode {
		_, _ = fn.WriteTo(os.Stdout)
	}

	// 将 Go 原生参数转换为解释器内部的 value.Value 类型
	argValues := make([]value.Value, len(args))
	for i, arg := range args {
		argValues[i] = value.ValueOf(arg)
	}

	frame := &execFrame{
		program: p,
		ctx:     ctx,
		traceID: traceID,
	}

	ret := callSSA(frame, fn, argValues, nil)
	if frame.panicValue != nil {
		err = fmt.Errorf("runtime error: %v", frame.panicValue)
	}
	if ret != nil {
		result = ret.Interface()
	}
	ctx.cancel()

	return result, ctx, err
}

// initGlobals 将所有全局变量初始化为其类型的零值。
func (p *Program) initGlobals() {
	for _, member := range p.pkg.Members {
		if g, ok := member.(*ssa.Global); ok {
			zeroVal := zero(g.Type().(*types.Pointer).Elem()).Elem()
			p.globals[g] = &zeroVal
		}
	}
}

// SetGlobal 设置全局变量的值。
func (p *Program) SetGlobal(name string, val interface{}) error {
	member := p.pkg.Members[name]
	if g, ok := member.(*ssa.Global); ok {
		v := value.ValueOf(val)
		p.globals[g] = &v
		return nil
	}
	return fmt.Errorf("global variable not found: %s", name)
}

// GetGlobal 获取全局变量的类型信息。
func (p *Program) GetGlobal(name string) (interface{}, error) {
	if member, ok := p.pkg.Members[name]; ok {
		return member.Type().(*types.Pointer).Elem(), nil
	}
	return nil, fmt.Errorf("global variable not found: %s", name)
}

// SSAPackage 返回底层 SSA 包，用于导入到其他 Program 中实现跨包调用。
func (p *Program) SSAPackage() *ssa.Package {
	return p.pkg
}
