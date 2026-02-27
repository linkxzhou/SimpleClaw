# goscript

基于 SSA（静态单赋值）的 Go 脚本解释器。将 Go 源码编译为 SSA 中间表示，然后在运行时逐指令解释执行。

## 架构

```
                         Go Source Code
                              │
                    ┌─────────▼─────────┐
                    │   parser.ParseFile │  解析为 AST
                    └─────────┬─────────┘
                              │
                    ┌─────────▼─────────┐
                    │    autoImport()    │  自动补充 import
                    └─────────┬─────────┘
                              │
                    ┌─────────▼─────────┐
                    │  ssautil.BuildPkg  │  类型检查 + 构建 SSA
                    └─────────┬─────────┘
                              │
                    ┌─────────▼─────────┐
                    │     Program       │  可执行的编译产物
                    │  ├── globals      │  全局变量映射
                    │  ├── pkg (SSA)    │  SSA 包
                    │  └── importedPkgs │  已导入包列表
                    └─────────┬─────────┘
                              │
                    ┌─────────▼─────────┐
                    │   Run / Execute   │  逐指令解释执行
                    │  ├── execFrame    │  栈帧（局部变量、defer）
                    │  ├── ExecContext  │  执行上下文（超时、输出）
                    │  └── callSSA()    │  SSA 函数调用
                    └───────────────────┘
```

## 目录结构

```
goscript/
├── program.go          # 核心：Compile、Run、Program、autoImport
├── exec_context.go     # 执行上下文（ExecContext）和栈帧（execFrame）
├── call.go             # 函数调用处理（SSA/内置/外部/闭包）
├── instr.go            # SSA 指令执行（40+ 种指令分派）
├── ops.go              # 运算求值（一元、常量、二元）
├── binop.go            # 二元运算符实现（16 种运算）
├── types.go            # 类型转换（go/types → reflect.Type）
├── internal/
│   ├── importer/       # 包导入器（实现 go/types.Importer 接口）
│   │   ├── importer.go #   reflect.Type ↔ types.Type 双向映射
│   │   └── types.go    #   类型注册数据结构
│   └── value/          # 运行时值表示
│       └── value.go    #   Value 接口、RValue、ExternalValue
├── register/           # 外部包注册 API
│   ├── register.go     #   AddPackage 等注册函数
│   └── keywords.go     #   代码补全关键字生成
├── packages/           # 标准库适配（init() 自动注册）
│   ├── fmt.go          #   fmt 包
│   ├── math.go         #   math 包
│   ├── strings.go      #   strings 包
│   ├── time.go         #   time 包
│   └── tool/           #   代码生成工具
└── testdata/           # 测试用例源码
```

## 快速开始

### 一步编译执行

```go
result, err := goscript.Run("", code, "Add", 3, 5)
```

### 分步编译和执行

```go
program, err := goscript.Compile("", "test", code)
result, err := program.Run("", "Add", 3, 5)
```

### 带上下文执行（获取 print 输出）

```go
result, ctx, err := program.RunWithContext("", "Add", 3, 5)
output := ctx.Output() // 获取 print/println 输出
```

## 核心 API

| 函数/方法 | 说明 |
|-----------|------|
| `Run(traceID, source, funcName, args...)` | 一步编译执行 |
| `Compile(traceID, pkgName, source, deps...)` | 编译源码为 Program |
| `ParseFunctions(source, exportedOnly)` | 从源码提取函数名列表 |
| `program.Run(traceID, funcName, args...)` | 执行已编译程序中的函数 |
| `program.RunWithContext(traceID, funcName, args...)` | 执行并返回上下文 |
| `program.SetGlobal(name, value)` | 设置全局变量 |
| `program.GetGlobal(name)` | 获取全局变量类型 |
| `program.SSAPackage()` | 获取底层 SSA 包（用于跨包依赖） |

## 包注册

### 自动导入

源码中使用已注册包名时，自动补充 import 语句。

### 注册标准库

```go
import _ "github.com/linkxzhou/SimpleClaw/goscript/packages"
```

已注册标准库：`fmt`、`math`、`strings`、`time`

### 注册自定义包

```go
register.AddPackage(
    "mypkg", "mypkg",
    register.NewFunction("Hello", func() string { return "hi" }, ""),
    register.NewConst("Version", "1.0.0", "Package version"),
)
```

## SSA 指令执行

解释器支持完整的 SSA 指令集：

| 类别 | 指令 |
|------|------|
| 内存 | Alloc、Store、IndexAddr、FieldAddr |
| 运算 | UnOp、BinOp、Convert、ChangeType |
| 控制流 | If、Jump、Phi、Return、Panic |
| 函数 | Call、Defer、RunDefers、Go |
| 数据结构 | MakeSlice、MakeMap、MapUpdate、Lookup、Slice |
| 类型 | MakeInterface、ChangeInterface、TypeAssert |
| 并发 | MakeChan、Send、Select |
| 闭包 | MakeClosure |

## 类型桥接

解释器的核心挑战是桥接 Go 的两套类型系统：

- **编译期** — `go/types`（AST 类型检查产生）
- **运行时** — `reflect`（值操作需要）

`types.go` 中的 `typeConvert()` 递归地将 `go/types.Type` 转换为 `reflect.Type`，
覆盖 Basic、Array、Slice、Map、Struct、Pointer、Chan、Interface 等所有类型。

## 安全特性

- **超时控制** — 默认 10 秒执行超时，通过 `context.WithTimeout` 实现
- **Panic 恢复** — `RunWithContext` 通过 `defer/recover` 捕获运行时 panic
- **沙箱执行** — 脚本只能调用已注册的外部包函数
