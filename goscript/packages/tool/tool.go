// 代码生成工具：自动扫描 Go 标准库包，生成 goscript 包注册代码。
// 使用 go/importer 解析标准库包的导出对象（函数、类型、常量、变量），
// 然后为每个包生成一个 Go 源文件，其中的 init() 函数调用 register.AddPackage
// 注册所有导出对象。
//
// 用法：go generate 或 go run tool.go
package main

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/importer"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/linkxzhou/SimpleClaw/goscript/packages/tool/pkgs"
	"github.com/linkxzhou/SimpleClaw/utils"
)

var sourceDir string

func init() {
	_, filename, _, _ := runtime.Caller(0)
	sourceDir = filepath.Dir(filename)
}

//go:generate go run tool.go
func main() {
	for path, vlist := range pkgs.ImportPkgs {
		utils.LogDebug("path: ", path, ", vlist: ", vlist)
		err := packageImport(path, vlist)
		if err != nil {
			utils.LogDebug(path, err.Error())
			continue
		}
	}
}

// objectDecl 根据 types.Object 的具体类型（TypeName/Const/Var/Func），
// 生成对应的 register.NewType/NewConst/NewVar/NewFunction 调用代码。
func objectDecl(object types.Object) string {
	name := fmt.Sprintf("%s.%s", object.Pkg().Name(), object.Name())
	pkgPath := trimVendor(object.Pkg().Path())

	switch object.(type) {
	case *types.TypeName:
		if pkgPath == "sync/atomic" && object.Name() == "Pointer" {
			return fmt.Sprintf(`register.NewType("%s", reflect.TypeOf(func(%s[any]){}).In(0), "%s")`, object.Name(), name, "")
		}
		return fmt.Sprintf(`register.NewType("%s", reflect.TypeOf(func(%s){}).In(0), "%s")`, object.Name(), name, "")
	case *types.Const:
		if pkgPath == "math" && (object.Name() == "MaxUint" || object.Name() == "MaxUint64") {
			name = fmt.Sprintf("uint64(%s)", name)
		}
		return fmt.Sprintf(`register.NewConst("%s", %s, "%s")`, object.Name(), name, "")
	case *types.Var:
		switch object.Type().Underlying().(type) {
		case *types.Interface:
			return fmt.Sprintf(`register.NewVar("%s", &%s, reflect.TypeOf(func (%s){}).In(0), "%s")`, object.Name(), name, trimVendor(object.Type().String()), "")
		default:
			return fmt.Sprintf(`register.NewVar("%s", &%s, reflect.TypeOf(%s), "%s")`, object.Name(), name, name, "")
		}

	case *types.Func:
		if pkgPath == "sync" && object.Name() == "OnceValue" {
			return fmt.Sprintf(`register.NewFunction("%s", %s, "%s")`, object.Name(), "onceValue", "")
		}
		if pkgPath == "sync" && object.Name() == "OnceValues" {
			return fmt.Sprintf(`register.NewFunction("%s", %s, "%s")`, object.Name(), "onceValues", "")
		}
		return fmt.Sprintf(`register.NewFunction("%s", %s, "%s")`, object.Name(), name, "")
	}
	return ""
}

func trimVendor(src string) string {
	if i := strings.LastIndex(src, `vendor/`); i >= 0 {
		return src[i+7:]
	}
	return src
}

func packageImport(path string, vlist []string) error {
	pkg, err := importer.ForCompiler(token.NewFileSet(), "source", nil).Import(path)
	if err != nil {
		return err
	}

	builder := strings.Builder{}
	pkgPath := trimVendor(pkg.Path())
	utils.LogDebug("pkg.Path(): ", pkg.Path(), ", pkgPath: ", pkgPath)
	preImports := strings.Builder{}
	for _, v := range vlist {
		preImports.WriteString(`"`)
		preImports.WriteString(v)
		preImports.WriteString(`"` + "\n")
	}
	extraDecls := ""
	if pkgPath == "sync" {
		extraDecls = `
func onceValue(f func() any) func() any {
	return sync.OnceValue(f)
}

func onceValues(f func() (any, any)) func() (any, any) {
	return sync.OnceValues(f)
}
`
	}
	builder.WriteString(fmt.Sprintf(`package imports
import (
	%s
	"%s"
	"reflect"

	"github.com/linkxzhou/SimpleClaw/goscript/register"
)

var _ = reflect.Int

%s

func init() {
	register.AddPackage("%s", "%s",`+"\n", preImports.String(), path, extraDecls, path, pkg.Name()))
	scope := pkg.Scope()
	for _, declName := range pkg.Scope().Names() {
		if ast.IsExported(declName) {
			obj := scope.Lookup(declName)
			builder.WriteString(strings.Replace(objectDecl(obj), path, pkg.Name(), 1) + ",\n")
		}
	}
	builder.WriteString(`)
}`)

	src := builder.String()
	code, err := format.Source([]byte(src))
	if err != nil {
		code = []byte(src)
		println(path, err.Error())
	}

	utils.LogDebug("pkg.Name(): ", pkg.Name())
	filename := fmt.Sprintf("%s%c%s.go", filepath.Dir(sourceDir), os.PathSeparator, pkg.Name())
	return os.WriteFile(filename, code, 0666)
}
