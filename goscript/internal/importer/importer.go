package importer

// 包导入器：实现 go/types.Importer 接口，将外部注册的原生 Go 包
// 转换为 go/types 类型系统中的 types.Package，使类型检查器能够识别这些包。
// 同时管理 reflect.Type ↔ types.Type 的双向映射缓存。

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"reflect"
	"strings"
	"sync"

	"github.com/linkxzhou/SimpleClaw/utils"

	"golang.org/x/tools/go/ssa"
)

// externalTypes 存储 types.Type → reflect.Type 的映射。
// 使用 sync.Map 保证并发安全。
var externalTypes = sync.Map{}

// RegisterPackage 注册一个外部包及其导出对象。
// 同时注册到按路径索引和按名称索引的两个注册表中。
func RegisterPackage(path, name string, objects ...*ExternalObject) {
	packagesByPath[path] = &ExternalPackage{
		Path:    path,
		Name:    name,
		Objects: objects,
	}
	packagesByName[name] = &ast.ImportSpec{
		Path: &ast.BasicLit{
			Kind:  token.STRING,
			Value: fmt.Sprintf(`"%s"`, path),
		},
	}
}

// GetExternalType 根据 types.Type 查找对应的 reflect.Type。
// 用于 typeConvert 中优先查找已注册的外部类型。
func GetExternalType(t types.Type) reflect.Type {
	if v, ok := externalTypes.Load(t.String()); ok {
		return v.(reflect.Type)
	}
	return nil
}

// SetExternalType 注册 types.Type → reflect.Type 的映射关系。
func SetExternalType(t types.Type, rType reflect.Type) {
	externalTypes.Store(t.String(), rType)
}

// Importer 实现 go/types.Importer 接口，用于包导入和类型解析。
// 维护类型缓存、SSA 包引用和外部对象注册表。
type Importer struct {
	typeCache       map[reflect.Type]types.Type      // reflect.Type → types.Type 缓存
	ssaPackages     map[string]*ssa.Package           // 已编译的 SSA 包（脚本内部包）
	packageCache    map[string]*types.Package         // 已解析的 types.Package 缓存
	externalObjects map[string]*ExternalObject        // 按全名索引的外部对象
}

// NewImporter 创建新的导入器实例。
// ssaPkgs 参数用于注册脚本内部已编译的 SSA 包（支持跨包导入）。
func NewImporter(ssaPkgs ...*ssa.Package) *Importer {
	// 初始化基本类型缓存：reflect.Type → types.Type
	imp := &Importer{
		typeCache: map[reflect.Type]types.Type{
			reflect.TypeOf(func(bool) {}).In(0):       types.Typ[types.Bool],
			reflect.TypeOf(func(int) {}).In(0):        types.Typ[types.Int],
			reflect.TypeOf(func(int8) {}).In(0):       types.Typ[types.Int8],
			reflect.TypeOf(func(int16) {}).In(0):      types.Typ[types.Int16],
			reflect.TypeOf(func(int32) {}).In(0):      types.Typ[types.Int32],
			reflect.TypeOf(func(int64) {}).In(0):      types.Typ[types.Int64],
			reflect.TypeOf(func(uint) {}).In(0):       types.Typ[types.Uint],
			reflect.TypeOf(func(uint8) {}).In(0):      types.Typ[types.Uint8],
			reflect.TypeOf(func(uint16) {}).In(0):     types.Typ[types.Uint16],
			reflect.TypeOf(func(uint32) {}).In(0):     types.Typ[types.Uint32],
			reflect.TypeOf(func(uint64) {}).In(0):     types.Typ[types.Uint64],
			reflect.TypeOf(func(uintptr) {}).In(0):    types.Typ[types.Uintptr],
			reflect.TypeOf(func(float32) {}).In(0):    types.Typ[types.Float32],
			reflect.TypeOf(func(float64) {}).In(0):    types.Typ[types.Float64],
			reflect.TypeOf(func(complex64) {}).In(0):  types.Typ[types.Complex64],
			reflect.TypeOf(func(complex128) {}).In(0): types.Typ[types.Complex128],
			reflect.TypeOf(func(string) {}).In(0):     types.Typ[types.String],
		},
		externalObjects: make(map[string]*ExternalObject),
		ssaPackages:     make(map[string]*ssa.Package),
		packageCache:    make(map[string]*types.Package),
	}

	// 注册脚本内部 SSA 包
	for _, pkg := range ssaPkgs {
		imp.ssaPackages[pkg.Pkg.Name()] = pkg
	}

	return imp
}

// Import 实现 types.Importer 接口。
// 优先查找 SSA 包（脚本内部包），然后查找缓存，最后构建新的 types.Package。
func (imp *Importer) Import(path string) (*types.Package, error) {
	// 优先查找已编译的脚本内部包
	if pkg := imp.ssaPackages[path]; pkg != nil {
		return pkg.Pkg, nil
	}
	// 查找已解析的缓存
	if pkg := imp.packageCache[path]; pkg != nil {
		return pkg, nil
	}

	// 构建新的 types.Package 并设置导入依赖
	pkg := imp.Package(path)
	importList := make([]*types.Package, 0)
	for _, importPkg := range imp.packageCache {
		importList = append(importList, importPkg)
	}
	pkg.SetImports(importList)
	return pkg, nil
}

// newObject 将 ExternalObject 转换为 types.Object 并注册到包作用域。
// 根据对象种类（类型、变量、常量、函数）创建不同的 types.Object。
func (imp *Importer) newObject(pkg *types.Package, obj *ExternalObject) types.Object {
	name := obj.Name

	switch obj.Kind {
	case ObjectType:
		typ := imp.typeOf(obj.Type, pkg)
		return types.NewTypeName(token.NoPos, pkg, name, typ)

	case ObjectVar:
		typ := imp.typeOf(obj.Type, pkg)
		object := types.NewVar(token.NoPos, pkg, name, typ)
		pkg.Scope().Insert(object)
		return object

	case ObjectConst:
		// 根据值的 reflect.Kind 创建对应的 constant.Value
		v := obj.Value
		var constVal constant.Value

		switch obj.Type.Kind() {
		case reflect.Bool:
			constVal = constant.MakeBool(v.Bool())
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			constVal = constant.MakeInt64(v.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			constVal = constant.MakeUint64(v.Uint())
		case reflect.Float32, reflect.Float64:
			constVal = constant.MakeFloat64(v.Float())
		case reflect.String:
			constVal = constant.MakeString(v.String())
		}

		object := types.NewConst(token.NoPos, pkg, name, imp.typeOf(obj.Type, pkg), constVal)
		pkg.Scope().Insert(object)
		return object

	case ObjectFunc, ObjectBuiltinFunc:
		typ := imp.typeOf(obj.Type, pkg)
		object := types.NewFunc(token.NoPos, pkg, name, typ.(*types.Signature))
		pkg.Scope().Insert(object)
		return object
	}

	return nil
}

// pathToName 从完整导入路径中提取包名（最后一段）。
func pathToName(path string) string {
	if i := strings.LastIndexAny(path, "/"); i > 0 {
		return path[i+1:]
	}
	return path
}

// Package 获取或创建指定路径对应的 types.Package。
// 如果该路径已注册为外部包，则创建包含所有导出对象的 types.Package；
// 否则创建一个空的 types.Package。
func (imp *Importer) Package(path string) *types.Package {
	const vendorPath = "/vendor/"
	if path == "" {
		return nil
	}

	// 去除 vendor 路径前缀
	if index := strings.LastIndex(path, vendorPath); index >= 0 {
		path = path[index+len(vendorPath):]
	}

	pkg := imp.packageCache[path]
	if pkg == nil {
		if extPkg := packagesByPath[path]; extPkg != nil {
			// 已注册的外部包：创建 types.Package 并注册所有导出对象
			pkg = types.NewPackage(path, extPkg.Name)
			imp.packageCache[path] = pkg

			for _, obj := range extPkg.Objects {
				fullName := fmt.Sprintf("%s.%s", pkg.Path(), obj.Name)
				imp.externalObjects[fullName] = obj
				imp.newObject(pkg, obj)
			}
		} else {
			// 未注册的包：创建空的 types.Package
			pkg = types.NewPackage(path, pathToName(path))
			imp.packageCache[path] = pkg
		}
		pkg.MarkComplete()
	}
	return pkg
}

// funcSignature 从 reflect.Type 函数类型创建 types.Signature。
// recv 非 nil 时表示方法接收器，此时跳过第一个参数（接收器参数）。
func (imp *Importer) funcSignature(fn reflect.Type, recv *types.Var, pkg *types.Package) *types.Signature {
	// 构建输入参数列表
	numIn := fn.NumIn()
	in := make([]*types.Var, numIn)
	for i := 0; i < numIn; i++ {
		param := fn.In(i)
		in[i] = types.NewParam(token.NoPos, imp.Package(param.PkgPath()), param.Name(), imp.typeOf(param, pkg))
	}

	// 如果是方法，跳过第一个参数（接收器）
	if recv != nil {
		in = in[1:]
	}

	// 构建输出参数列表
	numOut := fn.NumOut()
	out := make([]*types.Var, numOut)
	for i := 0; i < numOut; i++ {
		param := fn.Out(i)
		out[i] = types.NewParam(token.NoPos, imp.Package(param.PkgPath()), param.Name(), imp.typeOf(param, pkg))
	}

	return types.NewSignature(recv, types.NewTuple(in...), types.NewTuple(out...), fn.IsVariadic())
}

// builtinKindMap 将 reflect.Kind 映射到 types.BasicKind。
// 用于 typeOf 中将运行时类型信息转换回编译期类型表示。
var builtinKindMap = map[reflect.Kind]types.BasicKind{
	reflect.Bool:          types.Bool,
	reflect.Int:           types.Int,
	reflect.Int8:          types.Int8,
	reflect.Int16:         types.Int16,
	reflect.Int32:         types.Int32,
	reflect.Int64:         types.Int64,
	reflect.Uint:          types.Uint,
	reflect.Uint8:         types.Uint8,
	reflect.Uint16:        types.Uint16,
	reflect.Uint32:        types.Uint32,
	reflect.Uint64:        types.Uint64,
	reflect.Uintptr:       types.Uintptr,
	reflect.Float32:       types.Float32,
	reflect.Float64:       types.Float64,
	reflect.Complex64:     types.Complex64,
	reflect.Complex128:    types.Complex128,
	reflect.String:        types.String,
	reflect.UnsafePointer: types.UnsafePointer,
}

// typeOf 将 reflect.Type（运行时类型）转换为 types.Type（编译期类型）。
// 这是 typeConvert 的反向操作，用于将外部包的 Go 类型注册到类型检查系统中。
// 对于命名类型，会递归处理方法集并注册外部类型映射。
func (imp *Importer) typeOf(t reflect.Type, _ *types.Package) types.Type {
	// 查找缓存
	if cached := imp.typeCache[t]; cached != nil {
		return cached
	}

	pkg := imp.Package(t.PkgPath())
	var namedType *types.Named

	// 命名类型：创建或获取 Named 类型，注册方法集
	if t.Name() != "" {
		namedType = imp.parseNamedType(t)
		imp.addMethods(t, namedType, pkg)
		SetExternalType(namedType, t)
	}

	var ttype types.Type

	switch t.Kind() {
	case reflect.Array:
		ttype = types.NewArray(imp.typeOf(t.Elem(), pkg), int64(t.Len()))

	case reflect.Chan:
		var dir types.ChanDir
		switch t.ChanDir() {
		case reflect.RecvDir:
			dir = types.RecvOnly
		case reflect.SendDir:
			dir = types.SendOnly
		case reflect.BothDir:
			dir = types.SendRecv
		}
		ttype = types.NewChan(dir, imp.typeOf(t.Elem(), pkg))

	case reflect.Func:
		ttype = imp.funcSignature(t, nil, pkg)

	case reflect.Interface:
		// 接口类型：收集所有方法签名
		methods := make([]*types.Func, t.NumMethod())
		for i := range methods {
			m := t.Method(i)
			methods[i] = types.NewFunc(token.NoPos, pkg, m.Name, imp.funcSignature(m.Type, nil, pkg))
		}
		ttype = types.NewInterface(methods, nil).Complete()

	case reflect.Map:
		ttype = types.NewMap(imp.typeOf(t.Key(), pkg), imp.typeOf(t.Elem(), pkg))

	case reflect.Ptr:
		ttype = types.NewPointer(imp.typeOf(t.Elem(), pkg))

	case reflect.Slice:
		ttype = types.NewSlice(imp.typeOf(t.Elem(), pkg))

	case reflect.Struct:
		// 结构体类型：只处理导出字段
		fields := make([]*types.Var, 0)
		tags := make([]string, 0)
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if !ast.IsExported(field.Name) {
				continue
			}
			fields = append(fields, types.NewVar(token.NoPos, imp.Package(field.PkgPath), field.Name, imp.typeOf(field.Type, pkg)))
			tags = append(tags, string(field.Tag))
		}
		ttype = types.NewStruct(fields, tags)

	case reflect.UnsafePointer:
		ttype = types.Typ[types.UnsafePointer]

	default:
		// 基本类型通过 builtinKindMap 映射
		if basicKind, ok := builtinKindMap[t.Kind()]; ok {
			ttype = types.Typ[basicKind]
		} else {
			ttype = types.Typ[types.Invalid]
			utils.LogError(t.Kind(), " ", t.String(), " not supported")
		}
	}

	// 命名类型设置底层类型后返回
	if t.Name() != "" {
		namedType.SetUnderlying(ttype)
		return namedType
	}

	imp.typeCache[t] = ttype
	return ttype
}

// addMethods 为命名类型添加方法集。
// 分别处理值接收器方法和指针接收器方法。
func (imp *Importer) addMethods(t reflect.Type, namedType *types.Named, pkg *types.Package) {
	// 添加值接收器方法
	if t.Kind() != reflect.Interface && t.NumMethod() > 0 {
		recv := types.NewParam(token.NoPos, pkg, "t", namedType)
		for i := 0; i < t.NumMethod(); i++ {
			m := t.Method(i)
			sig := imp.funcSignature(m.Type, recv, pkg)
			fn := types.NewFunc(token.NoPos, pkg, m.Name, sig)
			namedType.AddMethod(fn)
		}
	}

	// 添加指针接收器方法
	ptrType := reflect.PtrTo(t)
	if ptrType.NumMethod() > 0 {
		recv := types.NewParam(token.NoPos, pkg, "t", types.NewPointer(namedType))
		for i := 0; i < ptrType.NumMethod(); i++ {
			m := ptrType.Method(i)
			sig := imp.funcSignature(m.Type, recv, pkg)
			fn := types.NewFunc(token.NoPos, pkg, m.Name, sig)
			namedType.AddMethod(fn)
		}
	}
}

// SSAPackage 根据名称返回已注册的 SSA 包。
func (imp *Importer) SSAPackage(name string) *ssa.Package {
	return imp.ssaPackages[name]
}

// ExternalObject 根据全限定名返回外部对象。
// 全限定名格式为 "包路径.对象名"。
func (imp *Importer) ExternalObject(name string) *ExternalObject {
	return imp.externalObjects[name]
}

// parseNamedType 创建或获取命名类型。
// 如果该类型已在包作用域中注册，则返回已有的 Named 类型；
// 否则创建新的 Named 类型并插入作用域。
func (imp *Importer) parseNamedType(t reflect.Type) *types.Named {
	pkg := imp.Package(t.PkgPath())
	name := t.Name()
	var named *types.Named

	if pkg != nil {
		scope := pkg.Scope()
		obj := scope.Lookup(name)
		if obj == nil {
			// 首次遇到：创建新的 Named 类型
			typeName := types.NewTypeName(token.NoPos, pkg, name, nil)
			named = types.NewNamed(typeName, nil, nil)
			scope.Insert(typeName)
		} else {
			// 已存在：复用
			named = obj.Type().(*types.Named)
		}
	} else {
		typeName := types.NewTypeName(token.NoPos, pkg, name, nil)
		named = types.NewNamed(typeName, nil, nil)
	}

	imp.typeCache[t] = named
	return named
}
