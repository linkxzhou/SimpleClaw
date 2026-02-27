# Agent - AI Agent 框架

基于 ReAct（Reasoning + Acting）模式的 AI Agent 框架，采用事件驱动的消息总线架构，支持多通道接入、工具调用循环、持久化记忆、渐进式技能加载和后台子 Agent 异步执行。

## 架构概览

```
用户消息 → Channel (Telegram / WhatsApp / CLI)
         → MessageBus (消息总线)
         → Agent (核心引擎)
             ├─ ContextBuilder (组装 prompt: identity + bootstrap + memory + skills)
             ├─ Session (对话历史管理，内存缓存，上限 50 条)
             ├─ LLMProvider (调用 LLM API，支持 tool calling)
             ├─ ToolRegistry (工具注册与执行)
             │   ├─ read_file / write_file / edit_file / list_dir
             │   ├─ exec (Shell，~30 条安全规则)
             │   ├─ web_search / web_fetch
             │   ├─ message (发送消息到指定 channel)
             │   ├─ spawn (创建后台子 Agent)
             │   ├─ go_run (动态执行 Go 代码)
             │   └─ go_agent (Go 动态子代理)
             ├─ MemoryStore (长期记忆 + 每日笔记)
             ├─ SkillsLoader (技能发现与加载)
             └─ SubagentManager (后台任务 → 结果通过 MessageBus 回报)
         → MessageBus → Channel → 用户
```

## 目录结构

```
agent/
├── types.go          核心类型定义（消息、LLM 接口、Tool 接口、Session）
├── agent.go          Agent 主循环（消息处理、ReAct 循环、system 消息路由）
├── context.go        上下文构建器（system prompt 组装、消息列表管理）
├── memory.go         记忆系统（长期记忆、每日笔记、近期记忆）
├── skills.go         技能加载器（workspace/builtin 技能、依赖检查、渐进式加载）
├── subagent.go       子 Agent 管理器（独立工具集、聚焦 prompt、结果回报）
└── tools/
    ├── registry.go   工具注册中心（注册/注销/查找/执行）
    ├── fs.go         文件工具（ReadFile、WriteFile、EditFile、ListDir）
    ├── shell.go      Shell 执行工具（安全防护、超时控制、输出截断）
    ├── web.go        Web 工具（Brave Search API 搜索、URL 内容获取）
    ├── message.go    消息发送工具（动态 channel/chatID 上下文）
    ├── spawn.go      子 Agent 生成工具
    ├── gorun.go      Go 代码动态执行工具（goscript SSA 解释器）
    └── goagent.go    Go 动态子代理工具（生成 Go 代码作为子代理执行）
```

> **注意**：路径验证工具 `ValidatePath` 已移至 `utils` 包，供全局使用。

## 核心接口

### MessageBus - 消息总线

```go
type MessageBus interface {
    ConsumeInbound(ctx context.Context) (InboundMessage, error)
    PublishOutbound(msg OutboundMessage) error
    PublishInbound(msg InboundMessage) error
}
```

解耦 Agent 与具体通道（Telegram、WhatsApp、CLI 等），所有消息通过总线流转。`PublishInbound` 用于子 Agent 将结果回注到主 Agent。

### LLMProvider - LLM 提供商

```go
type LLMProvider interface {
    Chat(ctx context.Context, messages []Message, tools []ToolDef, model string) (*LLMResponse, error)
    GetDefaultModel() string
}
```

抽象 LLM 调用，支持 OpenAI function calling 格式的工具调用。`LLMResponse` 包含文本内容和可选的 `ToolCalls`。

### Tool - 工具接口

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() map[string]interface{}
    Execute(ctx context.Context, params map[string]interface{}) (string, error)
}
```

所有工具实现此接口，`Parameters()` 返回 JSON Schema 格式的参数定义，`Execute()` 接收解析后的参数 map 并返回字符串结果。

## 使用方式

### 基本使用

```go
agent := agent.NewAgent(agent.AgentConfig{
    Bus:       myMessageBus,     // 实现 MessageBus 接口
    Provider:  myLLMProvider,    // 实现 LLMProvider 接口
    Workspace: "/path/to/workspace",
    Model:     "gpt-4",         // 可选，默认取 provider.GetDefaultModel()
})

// 启动主循环（阻塞，监听 MessageBus）
ctx, cancel := context.WithCancel(context.Background())
defer cancel()
agent.Run(ctx)
```

### CLI 直接模式

```go
response, err := agent.ProcessDirect(ctx, "帮我分析一下当前目录的代码结构")
fmt.Println(response)
```

### 注册自定义工具

```go
agent.RegisterTool(&MyCustomTool{})
```

## 模块详细设计

### Agent 主循环 (`agent.go`)

Agent 的核心处理流程：

1. **消息消费**：从 `MessageBus` 阻塞获取入站消息
2. **消息路由**：区分普通消息和 system 消息（子 Agent 回报）
3. **Session 管理**：按 `channel:chatID` 维护独立会话，内存缓存，上限 50 条历史
4. **上下文构建**：通过 `ContextBuilder` 组装 system prompt + 历史 + 当前消息
5. **ReAct 循环**：最多迭代 `maxIterations`（默认 20）次
   - 调用 `LLMProvider.Chat()` 获取响应
   - 若有 `ToolCalls`：执行工具，将结果追加到消息历史，继续循环
   - 若无 `ToolCalls`：返回文本作为最终回复
6. **响应发送**：通过 `MessageBus.PublishOutbound()` 发回用户

**System 消息处理**：子 Agent 完成后将结果以 `channel=system` 的 `InboundMessage` 发回。主 Agent 解析 `ChatID`（格式 `originChannel:originChatID`）路由回原始会话。

### 上下文构建器 (`context.go`)

`ContextBuilder` 负责组装完整的 LLM 提示，按以下顺序拼接 system prompt：

| 顺序 | 内容 | 说明 |
|------|------|------|
| 1 | Identity | Agent 身份描述、当前时间、workspace 路径 |
| 2 | Bootstrap Files | 从 workspace 加载 `AGENTS.md`、`SOUL.md`、`USER.md`、`TOOLS.md`、`IDENTITY.md` |
| 3 | Memory | 长期记忆 (`MEMORY.md`) + 今日笔记 (`YYYY-MM-DD.md`) |
| 4 | Active Skills | 标记为 `always` 的技能完整内容 |
| 5 | Skills Summary | 所有可用技能的 XML 摘要（渐进式加载） |

`BuildMessages()` 方法将 system prompt、历史消息和当前用户消息组装为完整的消息列表。支持 base64 编码的图片附件。

### 记忆系统 (`memory.go`)

`MemoryStore` 提供两层持久化记忆：

| 类型 | 文件 | 说明 |
|------|------|------|
| 长期记忆 | `memory/MEMORY.md` | Agent 的持久化知识库，可读写 |
| 每日笔记 | `memory/YYYY-MM-DD.md` | 按日期组织的笔记，支持追加 |

主要方法：

- `ReadLongTerm()` / `WriteLongTerm()` - 长期记忆读写
- `ReadToday()` / `AppendToday()` - 今日笔记读取和追加
- `GetRecentMemories(days)` - 获取最近 N 天的记忆
- `ListMemoryFiles()` - 列出所有记忆文件（最新在前）
- `GetMemoryContext()` - 生成用于 system prompt 的记忆上下文

### 技能系统 (`skills.go`)

`SkillsLoader` 管理 Agent 的可扩展技能：

**技能结构**：每个技能是 `skills/{name}/SKILL.md` 文件，支持 YAML frontmatter 元数据。

**两个来源**（workspace 优先）：
- `{workspace}/skills/` - 用户自定义技能
- builtin skills 目录 - 内置技能

**渐进式加载**：
- `always` 标记的技能：完整内容注入 system prompt
- 其他技能：仅在 system prompt 中展示 XML 摘要，Agent 需要时通过 `read_file` 工具加载

**依赖检查**：检查技能所需的 CLI 工具（`bins`）和环境变量（`env`）是否可用。

### 子 Agent 管理器 (`subagent.go`)

`SubagentManager` 管理后台异步任务：

**设计特点**：
- 每个子 Agent 以独立 goroutine 运行
- 拥有独立的 `ToolRegistry`（无 `message` 和 `spawn` 工具，防止递归）
- 使用聚焦型 system prompt，仅包含任务描述和规则
- 最多 15 次工具迭代
- 完成后通过 `MessageBus.PublishInbound()` 以 system 消息方式回报结果

**生命周期**：
1. 主 Agent 调用 `spawn` 工具 → `SubagentManager.Spawn()`
2. 创建带 cancel 的 goroutine，存入 `runningTasks` map
3. 子 Agent 独立运行 ReAct 循环
4. 完成后调用 `announceResult()` 将结果注入 MessageBus
5. 主 Agent 收到 system 消息后路由回原始 channel

### 工具注册中心 (`tools/registry.go`)

`Registry` 提供线程安全的工具管理：

- `Register(t)` / `Unregister(name)` - 注册/注销
- `Get(name)` / `Has(name)` - 查找
- `Execute(ctx, name, params)` - 执行并自动处理错误
- `GetDefinitions()` - 生成 OpenAI function calling 格式的工具定义列表

### 内置工具

| 工具 | 文件 | 说明 |
|------|------|------|
| `read_file` | `tools/fs.go` | 读取文件内容，支持绝对路径 |
| `write_file` | `tools/fs.go` | 写入文件，自动创建父目录 |
| `edit_file` | `tools/fs.go` | 精确文本替换编辑，要求唯一匹配 |
| `list_dir` | `tools/fs.go` | 列出目录内容，区分文件和目录 |
| `exec` | `tools/shell.go` | 执行 Shell 命令，~30 条安全拒绝规则、可配置白名单、超时（默认 60s）、输出截断（10000 字符） |
| `web_search` | `tools/web.go` | 通过 Brave Search API 搜索，返回标题、URL、摘要 |
| `web_fetch` | `tools/web.go` | 获取 URL 内容，支持 JSON/HTML/Raw 格式，HTML 自动转文本（50000 字符上限） |
| `message` | `tools/message.go` | 向指定 channel:chatID 发送消息，支持动态上下文切换 |
| `spawn` | `tools/spawn.go` | 创建后台子 Agent 执行复杂任务 |
| `go_run` | `tools/gorun.go` | 通过 goscript SSA 解释器动态执行 Go 源代码，无需外部编译器，沙盒环境（超时 30s） |
| `go_agent` | `tools/goagent.go` | 生成 Go 代码作为动态子代理执行，定义 `Run(input)` 函数实现自举计算（超时 60s） |

### Go 代码执行工具

SimpleClaw 的核心差异化能力 —— Agent 可以编写并即时执行 Go 代码：

**`go_run`**：通用 Go 代码执行器
- Agent 生成 Go 源代码 → goscript 编译为 SSA → 解释执行
- 支持调用指定函数（默认 `main`）
- 可用包：`fmt`、`math`、`strings`、`time`

**`go_agent`**：动态子代理模式
- 代码必须定义 `Run(input map[string]interface{}) string` 函数
- 支持通过 `input` 参数传递结构化数据
- 自动校验 `Run` 函数存在性
- 捕获打印输出和返回值

两者均在沙盒中运行，支持超时控制和输出截断。

## 数据流

### 普通消息处理

```
InboundMessage{channel:"telegram", chatID:"12345", content:"你好"}
  → Agent.processMessage()
    → getOrCreateSession("telegram:12345")
    → updateToolContexts("telegram", "12345")
    → ContextBuilder.BuildMessages(history, "你好", nil)
    → agentLoop():
        → LLMProvider.Chat(messages, toolDefs, model)
        ← LLMResponse{Content: "你好！有什么需要帮助的？"}
  → OutboundMessage{channel:"telegram", chatID:"12345", content:"你好！..."}
```

### 工具调用流

```
agentLoop() 迭代 1:
  → LLMProvider.Chat(...)
  ← LLMResponse{ToolCalls: [{name:"read_file", args:{path:"/tmp/a.txt"}}]}
  → tools.Execute("read_file", {path:"/tmp/a.txt"})
  ← "file content..."
  → messages 追加 assistant(tool_calls) + tool(result)

agentLoop() 迭代 2:
  → LLMProvider.Chat(...)  (包含工具结果)
  ← LLMResponse{Content: "文件内容是..."}
  → 返回最终回复
```

### Go 代码执行流

```
agentLoop():
  → LLMProvider.Chat(...)
  ← LLMResponse{ToolCalls: [{name:"go_agent", args:{code:"...", input:{...}, task:"数据分析"}}]}
  → GoAgentTool.Execute()
    → goscript.Compile() → program.RunWithContext("Run", input)
    ← "[Go Agent: 数据分析]\n\nResult: ..."
  → messages 追加 assistant(tool_calls) + tool(result)
  → 继续 ReAct 循环
```

### 子 Agent 异步流

```
agentLoop():
  → LLMProvider.Chat(...)
  ← LLMResponse{ToolCalls: [{name:"spawn", args:{task:"分析代码"}}]}
  → SpawnTool.Execute() → SubagentManager.Spawn()
    → goroutine: runSubagent()
      → 独立 ReAct 循环（最多 15 次）
      → announceResult() → MessageBus.PublishInbound(system message)
  ← "Subagent [分析代码] started (id: abc123)"

（后续）
  → Agent.processMessage(InboundMessage{channel:"system"})
    → processSystemMessage() → 路由回原始 channel
```

## 消息格式

所有消息遵循 OpenAI function calling 格式：

```go
// System prompt
Message{Role: "system", Content: "..."}

// 用户消息
Message{Role: "user", Content: "..."}

// Assistant 回复（含工具调用）
Message{Role: "assistant", Content: "...", ToolCalls: [...]}

// 工具结果
Message{Role: "tool", ToolCallID: "call_xxx", Name: "read_file", Content: "..."}
```

## Workspace 约定

```
{workspace}/
├── AGENTS.md           # Agent 行为引导（可选）
├── SOUL.md             # Agent 人格定义（可选）
├── USER.md             # 用户偏好描述（可选）
├── TOOLS.md            # 工具使用指南（可选）
├── IDENTITY.md         # 身份定义（可选）
├── memory/
│   ├── MEMORY.md       # 长期记忆
│   └── 2026-02-03.md   # 每日笔记
└── skills/
    └── my-skill/
        └── SKILL.md    # 技能定义（YAML frontmatter + Markdown）
```
