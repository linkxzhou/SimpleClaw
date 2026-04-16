# Plan 11: Adaptive Code Generation — Agent 自适应代码生成与执行

## 目标

让 Agent 在对话过程中**自动识别**需要计算/数据处理的场景，**主动生成** Go 代码并通过 goscript 执行，将结果无缝融入对话流。从"用户显式调用 `go_run`/`go_agent`"进化为"Agent 自主决策何时写代码、写什么代码、如何利用结果"。

## 现状分析

### 已有基础（强）

| 组件 | 能力 | 状态 |
|------|------|------|
| `goscript` | SSA 解释器，支持 36+ 标准库包 | ✅ 成熟 |
| `go_run` 工具 | Agent 调用 → 编译执行 → 返回结果 | ✅ 可用 |
| `go_agent` 工具 | `Run(input)` 模式，支持参数传入 | ✅ 可用 |
| ReAct 循环 | 工具调用 → 结果 → 继续推理 | ✅ 成熟 |
| SubTurn | 嵌套执行，独立超时和 token 预算 | ✅ 可用 |

### 当前痛点

1. **被动触发** — Agent 必须"意识到"要用 `go_run`/`go_agent`，实际上它经常选择用自然语言硬算
2. **无代码复用** — 每次生成的代码都是一次性的，相同逻辑反复生成浪费 token
3. **无错误学习** — 代码编译/运行失败后，Agent 缺乏结构化的错误修复策略
4. **无能力扩展** — Agent 不能把成功的代码片段注册为新工具，动态扩展自己的能力
5. **Prompt 引导弱** — 当前 `go_run`/`go_agent` 的 Description 只是"可以执行"，缺少"应该在什么场景用"的策略引导

## 技术方案

### 阶段一：GoCodegen 策略引导 + 自动修复（核心）

让 Agent 在合适场景自动选择生成代码，并在失败时自动修复。

#### 1.1 GoCodegen Advisor（代码生成顾问）

在 system prompt 中注入代码生成策略，引导 Agent 主动使用 `go_run`/`go_agent`：

```go
// agent/go_codegen.go (新文件)
package agent

// GoCodegenAdvisor 生成代码决策策略 prompt
type GoCodegenAdvisor struct {
    AvailablePackages string // 从 goscript 动态读取
}

// BuildPrompt 返回注入 system prompt 的代码生成策略
func (a *GoCodegenAdvisor) BuildPrompt() string {
    // 返回策略文本，描述：
    // 1. 哪些场景应该写代码（数学计算、数据转换、文本处理、排序/搜索、正则匹配...）
    // 2. 何时不需要写代码（简单问答、闲聊、文件操作...）
    // 3. 代码模板和最佳实践
    // 4. 可用包清单（动态生成）
}
```

策略内容包括场景识别规则：

| 场景 | 建议 | 理由 |
|------|------|------|
| 数学计算（超过 2 步） | **写代码** | 自然语言计算容易出错 |
| 数据排序/过滤/聚合 | **写代码** | 精确且可验证 |
| 正则匹配/文本处理 | **写代码** | `regexp`/`strings` 包精确处理 |
| JSON/XML 结构转换 | **写代码** | `json`/`xml` 包类型安全 |
| 日期/时间计算 | **写代码** | `time` 包避免人工错误 |
| 加解密/编码 | **写代码** | `crypto`/`encoding` 包精确 |
| 简单问答/闲聊 | 自然语言 | 无需代码 |
| 文件读写操作 | 用已有工具 | `read_file`/`write_file` 更直接 |

#### 1.2 Auto-Fix Loop（自动修复循环）

当 `go_run`/`go_agent` 返回编译错误或运行时错误时，自动进入修复循环：

```go
// agent/tools/gocode.go 中增强 goExec

// CodeFixConfig 代码修复配置
type CodeFixConfig struct {
    MaxRetries    int           // 最大重试次数（默认 3）
    FixTimeout    time.Duration // 单次修复超时
}

// goExecWithFix 带自动修复的代码执行
// 执行流程：
// 1. 编译执行
// 2. 成功 → 返回结果
// 3. 失败 → 分析错误类型（编译错误/运行时错误/超时）
// 4. 生成结构化错误报告（错误类型 + 行号 + 建议修复方向）
// 5. 返回带修复建议的错误信息，Agent 在 ReAct 循环中自然重试
```

关键点：**不在 goExec 内部重试**（那样要嵌套调用 LLM，复杂度爆炸），而是返回**结构化错误**让 Agent 在 ReAct 循环中自然修复：

```
[Compile Error] Line 12: undefined: strconv
Available packages: fmt, math, strings, strconv, ...
Hint: Did you forget to import "strconv"? The interpreter auto-imports known packages.

[Runtime Error] Line 8: index out of range [5] with length 3
Context: data := []int{1, 2, 3}; result := data[5]
Hint: Check array bounds before accessing.
```

#### 1.3 增强 ContextBuilder 集成

```go
// agent/context.go — BuildSystemPrompt 中新增一节

func (cb *ContextBuilder) buildGoCodegenSection() string {
    advisor := &GoCodegenAdvisor{
        AvailablePackages: getAvailablePackages(), // 从 goscript 动态读取
    }
    return advisor.BuildPrompt()
}
```

### 阶段二：代码片段缓存与复用

避免相同逻辑反复生成，节省 token。

#### 2.1 Snippet Store（片段存储）

```go
// agent/snippets.go (新文件)

// Snippet 可复用的代码片段
type Snippet struct {
    ID          string    `json:"id"`
    Name        string    `json:"name"`        // 人可读名称
    Description string    `json:"description"` // 用途描述
    Code        string    `json:"code"`        // Go 源代码
    FuncName    string    `json:"func_name"`   // 入口函数名
    Tags        []string  `json:"tags"`        // 标签（用于搜索）
    UseCount    int       `json:"use_count"`   // 使用次数
    LastUsed    time.Time `json:"last_used"`   // 最后使用时间
    CreatedAt   time.Time `json:"created_at"`  // 创建时间
}

// SnippetStore 管理代码片段
type SnippetStore struct {
    dir      string              // ~/.simpleclaw/snippets/
    snippets map[string]*Snippet // ID → Snippet
    mu       sync.RWMutex
}

func (s *SnippetStore) Save(snippet *Snippet) error    // 持久化到 JSON 文件
func (s *SnippetStore) Search(query string) []*Snippet  // 按名称/标签/描述搜索
func (s *SnippetStore) Get(id string) (*Snippet, bool)
func (s *SnippetStore) Delete(id string) error
func (s *SnippetStore) ListByUsage(limit int) []*Snippet // 按使用频率排序
```

#### 2.2 新增工具：`go_snippet`

```go
// agent/tools/gocode.go 新增

// GoSnippetTool 管理和执行保存的代码片段
type GoSnippetTool struct {
    Store     *SnippetStore
    Timeout   time.Duration
    MaxOutput int
}

// 操作模式：
// - action:"save"   — 保存当前代码为片段（name + description + code + tags）
// - action:"list"   — 列出已保存的片段（支持搜索）
// - action:"run"    — 按 ID/Name 执行已保存的片段
// - action:"delete" — 删除片段
```

#### 2.3 Snippet 注入 System Prompt

在 ContextBuilder 中注入已保存片段的摘要，Agent 可以选择复用：

```
## Available Code Snippets
- `csv_parser` — Parse CSV data and return structured JSON (used 5 times)
- `date_calc` — Calculate business days between two dates (used 3 times)
- `json_diff` — Compare two JSON objects and return differences (used 2 times)
Use go_snippet(action:"run", name:"csv_parser", input:{...}) to execute saved snippets.
```

### 阶段三：动态工具注册

将成功且常用的代码片段自动升级为一等工具。

#### 3.1 DynamicTool（动态工具）

```go
// agent/tools/dynamic.go (新文件)

// DynamicTool 由 goscript 代码驱动的动态工具
type DynamicTool struct {
    name        string
    description string
    paramSchema map[string]interface{}
    code        string                  // Go 源代码
    funcName    string                  // 入口函数（默认 "Run"）
    program     *goscript.Program       // 预编译（避免重复编译）
    timeout     time.Duration
    maxOutput   int
}

// 实现 tools.Tool 接口
func (t *DynamicTool) Name() string                      { return t.name }
func (t *DynamicTool) Description() string               { return t.description }
func (t *DynamicTool) Parameters() map[string]interface{} { return t.paramSchema }
func (t *DynamicTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
    // 将 params 传入预编译的 goscript Program 执行
}
```

#### 3.2 DynamicToolManager（动态工具管理器）

```go
// agent/tools/dynamic.go

// DynamicToolManager 管理动态注册的工具
type DynamicToolManager struct {
    store    *SnippetStore
    registry *Registry
    dir      string // ~/.simpleclaw/dynamic_tools/
}

// PromoteSnippet 将代码片段升级为工具
// 1. 从 SnippetStore 获取代码
// 2. 预编译（Compile 一次缓存 Program）
// 3. 生成参数 Schema（从 Run 函数签名推断或手动指定）
// 4. 注册到 Registry
func (m *DynamicToolManager) PromoteSnippet(snippetID, toolName, description string, paramSchema map[string]interface{}) error

// LoadPersistedTools 启动时加载之前注册的动态工具
func (m *DynamicToolManager) LoadPersistedTools() error

// DemoteTool 将动态工具降级回片段
func (m *DynamicToolManager) DemoteTool(toolName string) error
```

#### 3.3 新增工具：`go_promote`

```go
// 操作模式：
// - action:"promote" — 将 snippet 升级为工具（需指定 name, description, param_schema）
// - action:"demote"  — 将动态工具降级回 snippet
// - action:"list"    — 列出所有动态工具
```

### 阶段四：Prelude 库（可选增强）

为 goscript 提供面向 Agent 的高级工具函数库。

#### 4.1 Agent Prelude 包

```go
// goscript/packages/agentlib.go (新文件)
// 注册 "agentlib" 包，提供 Agent 常用的高级函数

func init() {
    register.AddPackage("agentlib", "agentlib",
        // 数据处理
        register.NewFunction("ParseCSV", parseCSV, "Parse CSV string to [][]string"),
        register.NewFunction("ToJSON", toJSON, "Marshal value to JSON string"),
        register.NewFunction("FromJSON", fromJSON, "Unmarshal JSON string to map"),
        
        // 文本处理
        register.NewFunction("ExtractURLs", extractURLs, "Extract all URLs from text"),
        register.NewFunction("WordCount", wordCount, "Count words in text"),
        register.NewFunction("Summarize", summarize, "Truncate text to N sentences"),
        
        // 数学/统计
        register.NewFunction("Mean", mean, "Calculate mean of float64 slice"),
        register.NewFunction("Median", median, "Calculate median"),
        register.NewFunction("StdDev", stddev, "Calculate standard deviation"),
        
        // 日期
        register.NewFunction("BusinessDays", businessDays, "Count business days between dates"),
        register.NewFunction("ParseDate", parseDate, "Flexible date string parser"),
    )
}
```

## 实现文件清单

| 阶段 | 文件 | 说明 | 工作量 |
|------|------|------|--------|
| 一 | `agent/go_codegen.go` | GoCodegen Advisor（策略 prompt 生成） | S |
| 一 | `agent/tools/gocode.go` ← 修改 | 结构化错误报告 + 修复建议 | S |
| 一 | `agent/context.go` ← 修改 | 集成 GoCodegen Advisor | XS |
| 二 | `agent/snippets.go` | SnippetStore（片段存储 + 搜索） | M |
| 二 | `agent/tools/gocode.go` ← 修改 | 新增 GoSnippetTool | S |
| 二 | `agent/context.go` ← 修改 | 注入片段摘要 | XS |
| 三 | `agent/tools/dynamic.go` | DynamicTool + DynamicToolManager | M |
| 三 | `agent/tools/gocode.go` ← 修改 | 新增 GoPromoteTool | S |
| 三 | `agent/tools/registry.go` ← 修改 | 支持动态注册/注销 | XS |
| 四 | `goscript/packages/agentlib.go` | Agent Prelude 标准库 | M |
| - | `*_test.go` | 各阶段测试 | M |

## 数据流

### 阶段一：自适应代码生成

```
User: "帮我算一下 2025-01-01 到 2025-12-31 之间有多少个工作日"
  → Agent 看到 system prompt 中的 GoCodegen 策略
  → 识别为"日期计算"场景 → 选择 go_agent
  → 生成代码:
      func Run(input map[string]interface{}) string {
          start, _ := time.Parse("2006-01-02", "2025-01-01")
          end, _ := time.Parse("2006-01-02", "2025-12-31")
          days := 0
          for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
              if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
                  days++
              }
          }
          return fmt.Sprintf("2025年共有 %d 个工作日", days)
      }
  → goscript 执行 → "2025年共有 261 个工作日"
  → Agent 返回结果
```

### 阶段一：自动修复

```
Agent → go_run(code: "...strconv.Atoi(s)...") 
  → [Compile Error] Line 5: undefined: strconv
    Available packages: ..., strconv, ...
    Hint: Auto-import should handle this. Check if package name is correct.
  → Agent 分析错误，修正代码
  → go_run(code: "package main\nimport \"strconv\"\n...") 
  → 成功
```

### 阶段二：片段复用

```
Agent → go_snippet(action:"save", name:"workdays", code:"...", tags:["date","calendar"])
  → Saved snippet 'workdays' (id: abc123)

（下次类似问题）
Agent → 看到 system prompt 中 "Available Snippets: workdays — ..."
  → go_snippet(action:"run", name:"workdays", input:{"start":"2026-01-01","end":"2026-12-31"})
  → 直接复用，不需要重新生成代码
```

### 阶段三：工具升级

```
Agent → go_promote(action:"promote", snippet:"workdays", name:"count_workdays", 
         description:"Count business days between two dates")
  → Promoted 'workdays' → tool 'count_workdays'

（后续对话）
Agent → 在 tool list 中看到 count_workdays
  → count_workdays(start:"2026-01-01", end:"2026-06-30")
  → 直接作为工具调用，无需感知底层是 goscript
```

## 优先级建议

- **阶段一必做**（投入小收益大）：go_codegen 策略 prompt + 结构化错误 = Agent 主动且正确地写代码
- **阶段二推荐**（中等投入）：片段复用 = token 节省 + 一致性
- **阶段三可选**（投入较大）：动态工具 = Agent 能力自进化（酷但复杂度高）
- **阶段四可选**（独立模块）：Prelude 库 = 降低 Agent 生成代码的复杂度

## 与现有架构的兼容性

| 依赖 | 交互方式 | 影响 |
|------|---------|------|
| `goscript` | 只读使用，无需修改 | ✅ 零侵入 |
| `go_run`/`go_agent` | 增强错误输出，向后兼容 | ✅ 兼容 |
| `ContextBuilder` | 新增一节 prompt | ✅ 追加式 |
| `tools.Registry` | 已支持动态 Register/Unregister | ✅ 就绪 |
| `SubTurn` | 可选：代码执行作为子回合 | ✅ 可选集成 |
| 审批网关 | 动态工具自动继承审批策略 | ✅ 透明 |

## 风险与缓解

| 风险 | 缓解 |
|------|------|
| Agent 过度使用代码（简单问题也写代码） | 策略 prompt 明确"何时不需要写代码"的边界 |
| 代码修复死循环 | goExec 只返回结构化错误，ReAct 循环的 maxIterations 天然限制重试 |
| Snippet 存储膨胀 | 按使用频率 + 最后使用时间自动清理低频片段 |
| 动态工具安全性 | 继承审批网关（Plan6），write 操作需确认 |
| goscript 沙盒逃逸 | 只能调用已注册包，无 os/exec，timeout 保护 |
