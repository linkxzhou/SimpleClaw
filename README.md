# SimpleClaw

SimpleClaw 是一个用 Go 实现的模块化、事件驱动的 AI Agent 框架。核心架构围绕**消息总线（MessageBus）**展开，实现 Agent 核心逻辑与外部通信渠道的完全解耦。

## 架构总览

```
┌─────────────────────────────────────────────────────────────────┐
│                        SimpleClaw 架构                              │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────┐  ┌──────────┐              ┌──────────────────┐   │
│  │ Telegram │  │ WhatsApp │  ...         │   CLI / Direct   │   │
│  └────┬─────┘  └────┬─────┘              └────────┬─────────┘   │
│       │              │                             │              │
│       └──────┬───────┘                             │              │
│              ▼                                     │              │
│  ┌───────────────────────┐  ┌──────────────────┐  │              │
│  │    Channel Manager    │  │  Routing (路由)   │  │              │
│  └───────────┬───────────┘  └──────────────────┘  │              │
│              ▼                                     ▼              │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │                    MessageBus (bus/)                      │    │
│  │          inbound chan ◄──────── outbound chan             │    │
│  └─────────────┬───────────────────────┬───────────────────┘    │
│                │                       ▲                         │
│                ▼                       │                         │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │                     Agent (agent/)                        │    │
│  │  ┌──────────────┐  ┌──────────────┐  ┌───────────────┐  │    │
│  │  │ContextBuilder│  │ MemoryStore  │  │ SkillsLoader  │  │    │
│  │  └──────┬───────┘  └──────────────┘  └───────────────┘  │    │
│  │         │                                                 │    │
│  │         ▼          ┌──────────────┐                       │    │
│  │  ┌──────────────┐  │SubagentMgr   │                       │    │
│  │  │  AgentLoop   │──│  (spawn)     │                       │    │
│  │  │  (ReAct)     │  └──────────────┘                       │    │
│  │  └──────┬───────┘                                         │    │
│  │         │                                                 │    │
│  │         ▼                                                 │    │
│  │  ┌──────────────────────────────────────────────────┐    │    │
│  │  │              Tool Registry (agent/tools/)         │    │    │
│  │  │  read_file │ write_file │ edit_file │ list_dir   │    │    │
│  │  │  exec      │ web_search │ web_fetch              │    │    │
│  │  │  message   │ spawn      │ go_run   │ go_agent   │    │    │
│  │  └──────────────────────────────────────────────────┘    │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                  │
│  ┌───────────┐  ┌───────────┐  ┌──────────┐  ┌──────────────┐  │
│  │ Providers │  │  Memory   │  │   Cron   │  │   Health     │  │
│  │(LLM 容错) │  │(会话/状态) │  │(定时任务) │  │(健康+心跳)   │  │
│  └───────────┘  └───────────┘  └──────────┘  └──────────────┘  │
│                                                                  │
│  ┌───────────┐  ┌──────────────────────────┐                     │
│  │  Config   │  │  Utils (helpers + log)   │                     │
│  │  (JSON)   │  │  slog + lumberjack       │                     │
│  └───────────┘  └──────────────────────────┘                     │
│                                                                  │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │              GoScript (goscript/)                         │    │
│  │         SSA-based Go 脚本解释器                            │    │
│  └─────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────┘
```

## 核心模块

### 1. Agent 引擎 (`agent/`)

系统的核心，基于 **ReAct（Reasoning + Acting）** 模式运行：

- **`Agent`** — 主入口，消费 MessageBus 消息，分发 goroutine 处理，支持 `Run(ctx)` 长运行和 `ProcessDirect()` CLI 直接调用
- **`ContextBuilder`** — 组装 LLM 的 System Prompt，整合身份文件（`AGENTS.md`/`SOUL.md`/`IDENTITY.md`）、记忆（`memory.MemoryStore`）、技能摘要
- **`SkillsLoader`** — 渐进式技能加载：always 技能完整加载，其余展示 XML 摘要，按需加载
- **`SubagentManager`** — 后台子 Agent 并发管理，受限工具集（无 message/spawn），结果通过系统消息回传
- **类型别名** — `types.go` 通过 Go 类型别名（`type MediaItem = bus.MediaItem`）统一 agent 与 bus 包类型，消除适配器转换开销

**内置工具（11 个）：**

| 工具 | 功能 |
|------|------|
| `read_file` | 读取文件内容 |
| `write_file` | 写入文件（自动创建目录） |
| `edit_file` | 精确文本替换 |
| `list_dir` | 列出目录结构 |
| `exec` | 执行 Shell 命令（超时/输出截断） |
| `web_search` | Brave Search API 搜索 |
| `web_fetch` | HTTP 抓取网页（HTML→文本转换） |
| `message` | 向聊天频道发送消息 |
| `spawn` | 创建子 Agent 执行异步任务 |
| **`go_run`** | **动态执行 Go 代码**（goscript SSA 解释器沙箱，可用包列表从 `pkgs.ImportPkgs` 动态读取） |
| **`go_agent`** | **生成 Go 动态子 Agent**（Go 自举：Agent 用 Go 写 Go Agent，共享 `goExec` 执行引擎） |

### 2. 消息总线 (`bus/`)

Agent 与外部通信的解耦层：

- **`InboundMessage`** — 入站消息（channel/sender_id/chat_id/content/media/metadata）
- **`OutboundMessage`** — 出站消息（channel/chat_id/content/reply_to/media）
- **`MessageBus`** — 带缓冲的 channel 队列，支持 `PublishInbound`/`ConsumeInbound`/`PublishOutbound`/`ConsumeOutbound`
- 支持 `SubscribeOutbound` 注册回调 + `DispatchOutbound` 后台分发

### 3. 聊天渠道 (`channels/`)

外部平台的适配器：

- **`Channel`** 接口 — `Name()`、`Start()`、`Stop()`、`Send()`、`IsRunning()`
- **`BaseChannel`** — 公共逻辑：权限校验（`AllowFrom` 白名单，支持 `|` 分隔复合 ID）、消息转发
- **`TelegramChannel`** — Telegram Bot 集成框架，包含 Markdown→Telegram HTML 转换
- **`WhatsAppChannel`** — WhatsApp Node.js Bridge 集成框架，WebSocket JSON 协议
- **`Manager`** — 管理多通道生命周期、出站消息路由、状态查询

### 4. 配置管理 (`config/`)

JSON 配置文件，支持两级路径优先级：
1. **本地目录** `.simpleclaw/config.json`（项目级，优先）
2. **用户目录** `~/.simpleclaw/config.json`（全局，兜底）

`GetConfigPath()` 先检查当前工作目录下的 `.simpleclaw/config.json`，存在则返回其绝对路径，否则回退到 `~/.simpleclaw/config.json`。

```go
Config {
    Agents    // 默认模型、workspace、max_tokens、temperature
    Channels  // Telegram（token/allowFrom）、WhatsApp（bridgeUrl）
    Providers // Anthropic/OpenAI/OpenRouter/Groq/Zhipu/VLLM/Gemini
    Gateway   // host/port
    Tools     // Web Search API key
}
```

- `GetAPIKey()` — 按优先级自动选择 API Key（OpenRouter > Anthropic > OpenAI > ...）
- `GetEndpoints()` — 从所有 8 家厂商配置 + `agents.defaults.apiBase` 构建端点映射
- `GetAPIKeys()` — 根据模型前缀自动匹配厂商 API Key
- `Load(path)` / `Save(cfg, path)` — JSON 加载/保存

### 5. LLM 提供商 (`providers/`)

统一的 LLM 调用层，支持多厂商容错切换：

- **`LLMProvider`** 接口 — `Chat(ctx, ChatRequest) (*LLMResponse, error)` + `GetDefaultModel()`
- **`OpenAIProvider`** — 兼容 OpenAI/OpenRouter/vLLM/Zhipu/DeepSeek/Groq/Gemini/Moonshot/Qwen 等 OpenAI 格式 API
  - 自动解析 tool_calls、处理 JSON 参数、统计 token 用量
- **`ProviderFactory`** — 根据 `vendor/model` 格式自动创建 Provider，内置 12 家厂商默认端点
- **`FallbackChain`** — 多 Provider 容错执行链，按优先级逐个尝试，自动跳过冷却中的 Provider
- **`CooldownTracker`** — 智能冷却退避：标准错误指数退避（1min→5min→25min→60min cap），计费错误长退避（5h→24h cap）
- **`ErrorClassifier`** — 6 种错误分类（auth/rate_limit/billing/timeout/format/overloaded），基于 HTTP 状态码 + 正则模式匹配
- **`GroqTranscriptionProvider`** — Groq Whisper API 语音转文字

### 6. 持久化存储 (`memory/`)

统一的会话、状态和记忆管理：

- **`Session`** — 对话会话，消息历史（role/content/timestamp），`GetHistory(maxMessages)` 截取最近 N 条
- **`SessionManager`** — 会话生命周期管理：内存缓存 + JSONL 磁盘持久化，`GetOrCreate` / `Save` / `Delete` / `ListSessions`
  - 存储格式：第一行 metadata（创建/更新时间），后续每行一条消息
- **`StateManager`** — 系统状态原子持久化（temp file + rename），追踪最近活跃的 Channel/ChatID
- **`MemoryStore`** — 长期记忆（`MEMORY.md`）+ 每日笔记（`YYYY-MM-DD.md`）管理，支持记忆上下文生成

### 7. 定时任务 (`cron/`)

完整的定时任务调度系统：

- **三种调度模式**：`at`（一次性定时）、`every`（固定间隔）、`cron`（cron 表达式）
- **Job CRUD**：`AddJob` / `RemoveJob` / `EnableJob` / `RunJob` / `ListJobs`
- **持久化**：JSON 文件存储，支持服务重启后恢复
- **一次性任务**：执行后自动禁用或删除（`DeleteAfterRun`）

### 8. 健康与心跳 (`health/`)

HTTP 健康检查 + 周期性 Agent 唤醒：

- **`Server`** — HTTP 健康检查服务器：`/health`（存活探针，始终 200）、`/ready`（就绪探针，检查依赖状态），支持 `HandleFunc()` 挂载外部路由
- **`HeartbeatService`** — 周期性心跳服务，默认每 30 分钟读取 `HEARTBEAT.md`
  - 空内容自动跳过（智能检测：跳过标题、注释、空 checkbox）
  - `TriggerNow()` 支持手动触发

### 9. 消息路由 (`routing/`)

多 Agent 消息路由解析：

- **`RouteResolver`** — 7 级优先级级联路由：peer > guild > team > account > channel_wildcard > channel_exact > default
- **`BindingConfig`** — 路由绑定规则（agent/channel/peer/guild/team/account）
- 支持通配符渠道匹配（`*`）

### 10. 工具函数 (`utils/`)

通用辅助函数 + 日志系统：

- **路径管理**：`EnsureDir` / `GetDataPath` / `GetWorkspacePath` / `GetSessionsPath` / `GetMemoryPath` / `GetSkillsPath` / `ValidatePath` / `ExpandHome`
- **时间**：`TodayDate` / `Timestamp`
- **字符串**：`TruncateString` / `SafeFilename`
- **Session Key**：`ParseSessionKey` / `BuildSessionKey`
- **日志系统**（基于 `log/slog` + lumberjack）：
  - 双输出：控制台（TextHandler）+ 文件（JSONHandler + 自动轮转）
  - `slog.LevelVar` 支持运行时动态调整日志级别
  - 统一接口：`LogDebug/LogInfo/LogWarn/LogError/LogFatal` + 格式化版本

### 11. Go 脚本解释器 (`goscript/`)

基于 SSA（Static Single Assignment）的 Go 脚本解释器：

- 解析 Go 源码 → SSA IR → 逐指令解释执行
- 支持：变量、函数、方法、闭包、接口、流程控制（if/for/switch/select）
- 内置标准库绑定：`fmt`、`math`、`strings`、`time`（由 `packages/tool/pkgs/pkgs.go` 统一管理）
- 可扩展：通过 `register.AddPackage()` 注册自定义包
- `go_run` / `go_agent` 工具的可用包列表从 `pkgs.ImportPkgs` 动态读取，增删包后自动更新工具描述

## Go 自举：核心设计亮点

SimpleClaw 通过 `goscript` 解释器实现了 **Go 自举（self-bootstrapping）** 能力：

```
┌─────────────┐    生成 Go 代码    ┌──────────────┐    SSA 解释执行    ┌──────────┐
│  Agent/LLM  │ ─────────────────► │  go_run /     │ ─────────────────► │  结果回传  │
│  (推理决策)  │ ◄───────────────── │  go_agent     │ ◄───────────────── │  到 Agent │
└─────────────┘    结果反馈循环     └──────────────┘    goscript 沙箱    └──────────┘
```

**两个关键工具：**

- **`go_run`** — 通用 Go 代码执行器。Agent 生成任意 Go 代码片段，立即在沙箱中运行并获取结果。无需编译器、无需写文件。
- **`go_agent`** — Go 动态子 Agent 生成器。Agent 编写带有 `Run(input map[string]interface{}) string` 接口的 Go 代码，实现参数化的可复用计算逻辑。

**这意味着：**

1. **Agent 可以自己写算法** — 遇到复杂计算（排序、数学、数据处理），Agent 不再只能调 Shell，而是直接用 Go 写出高效逻辑并执行
2. **Go 写 Go** — Agent 可以用 Go 代码生成更多的 Go 工具代码，实现能力的递归扩展
3. **安全沙箱** — 在 goscript 解释器中运行，只能访问 `pkgs.ImportPkgs` 中注册的标准库，不会执行任意系统调用
4. **零依赖执行** — 不需要系统安装 Go 编译器，goscript 内建完成编译 + 执行

## 目录结构

```text
SimpleClaw/
├── cmd/                    # CLI 入口
│   ├── main.go             # 命令路由
│   ├── adapter.go          # bus/providers/memory ↔ agent 类型适配器
│   ├── onboard.go          # 初始化配置和工作区
│   ├── gateway.go          # 启动完整网关服务
│   ├── agent.go            # 单消息 / 交互模式
│   ├── cron.go             # 定时任务管理
│   └── status.go           # 系统状态 / 渠道管理
├── agent/                  # Agent 引擎
│   ├── agent.go            # 主循环 (AgentLoop, ReAct)
│   ├── context.go          # 上下文构建器
│   ├── skills.go           # 技能加载器
│   ├── subagent.go         # 子 Agent 管理
│   ├── types.go            # 核心类型定义（含 bus 类型别名）
│   └── tools/              # 内置工具集
│       ├── registry.go     # 工具注册表
│       ├── fs.go           # 文件操作 (read/write/edit/list)
│       ├── shell.go        # Shell 命令执行（安全防护）
│       ├── web.go          # 网页搜索/抓取
│       ├── message.go      # 消息发送工具
│       ├── spawn.go        # 子 Agent 生成工具
│       ├── gocode.go       # Go 动态执行（go_run + go_agent 统一引擎）
│       └── *_test.go       # 工具测试（7 个文件，141 个用例）
├── bus/                    # 消息总线
│   ├── events.go           # InboundMessage / OutboundMessage
│   ├── queue.go            # MessageBus 实现
│   └── errors.go           # 错误定义
├── channels/               # 聊天渠道适配器
│   ├── base.go             # Channel 接口 + BaseChannel
│   ├── manager.go          # 多通道管理器
│   ├── telegram.go         # Telegram 通道
│   └── whatsapp.go         # WhatsApp 通道
├── config/                 # 配置管理
│   ├── schema.go           # 配置 Schema + GetEndpoints/GetAPIKeys
│   └── loader.go           # JSON 加载/保存
├── providers/              # LLM 提供商（含容错）
│   ├── base.go             # LLMProvider 接口 + LLMResponse
│   ├── openai.go           # OpenAI 兼容实现
│   ├── factory.go          # ProviderFactory（多厂商自动创建）
│   ├── fallback.go         # FallbackChain（容错执行链）
│   ├── cooldown.go         # CooldownTracker（智能冷却退避）
│   ├── errors.go           # ErrorClassifier（错误分类）
│   └── transcription.go    # Groq 语音转文字
├── memory/                 # 持久化存储
│   ├── session.go          # Session 数据结构
│   ├── manager.go          # SessionManager（JSONL 持久化）
│   ├── state.go            # StateManager（原子状态持久化）
│   └── store.go            # MemoryStore（长期记忆 + 每日笔记）
├── routing/                # 消息路由
│   └── route.go            # RouteResolver（7 级优先级路由）
├── cron/                   # 定时任务
│   ├── types.go            # Job/Schedule/Payload 类型
│   └── service.go          # 调度服务
├── health/                 # 健康检查 + 心跳
│   ├── server.go           # HTTP 健康检查（/health + /ready + 外部路由）
│   └── heartbeat.go        # 周期唤醒服务
├── utils/                  # 工具函数 + 日志
│   ├── helpers.go          # 通用辅助（路径/时间/字符串/SessionKey）
│   └── log.go              # 日志系统（slog + lumberjack 双输出）
├── goscript/               # Go 脚本解释器
│   ├── program.go          # 编译/运行入口
│   ├── exec_context.go     # 执行上下文
│   ├── instr.go            # SSA 指令执行
│   ├── call.go             # 函数调用引擎
│   ├── binop.go            # 二元运算
│   ├── ops.go              # 一元运算
│   ├── types.go            # 类型映射
│   ├── register/           # 包注册 API
│   ├── packages/           # 标准库绑定 (fmt/math/strings/time)
│   └── internal/           # 内部实现 (importer/value)
├── thirdparty/             # 第三方参考项目
│   ├── nanobot-main/       # NanoBot (Python)
│   ├── picoclaw-main/      # PicoClaw (Go)
│   ├── tinyagi-main/       # TinyAGI (TypeScript/Node.js)
│   └── zeroclaw-master/    # ZeroClaw (Rust)
├── go.mod
└── go.sum
```

## 数据流

### 正常消息处理

```
用户 (Telegram/WhatsApp/...)
  │
  ▼
Channel.Start() ──监听外部事件──► BaseChannel.HandleMessage()
  │                                      │
  │                              权限校验 (AllowFrom)
  │                                      │
  ▼                                      ▼
                              MessageBus.PublishInbound()
                                         │
                                         ▼
                              Agent.Run() ──消费 inbound──►
                                         │
                              ┌───────────┴───────────┐
                              │  1. Session 获取/创建   │
                              │  2. ContextBuilder     │
                              │     构建 System Prompt  │
                              │  3. Agent Loop (ReAct) │
                              │     LLM → Tool → LLM  │
                              │     (最多 20 轮)        │
                              └───────────┬───────────┘
                                         │
                                         ▼
                              MessageBus.PublishOutbound()
                                         │
                                         ▼
                              Manager.dispatchOutbound()
                                         │
                                         ▼
                              Channel.Send() ──► 用户
```

### 子 Agent 异步流

```
Agent Loop ──spawn 工具──► SubagentManager.Spawn()
                                    │
                          goroutine │ (独立 ReAct 循环, 最多 15 轮)
                                    │ (受限工具: 无 message/spawn)
                                    │
                                    ▼
                          announceResult()
                                    │
                          MessageBus.PublishInbound(channel="system")
                                    │
                                    ▼
                          Agent.processSystemMessage()
                                    │
                          解析 origin channel:chatID
                                    │
                                    ▼
                          MessageBus.PublishOutbound() ──► 原始用户
```

## 快速开始

### 环境要求

- Go 1.21+

### 安装

```bash
go build -o simpleclaw ./cmd/
```

### 首次初始化

```bash
simpleclaw onboard
```

执行后会：
1. 创建配置文件 `~/.simpleclaw/config.json`（全局）
2. 创建工作区 `~/.simpleclaw/workspace/`
3. 生成模板文件（`AGENTS.md`、`SOUL.md`、`USER.md`、`HEARTBEAT.md`、`memory/MEMORY.md`）

> 也可在项目目录下创建 `.simpleclaw/config.json` 覆盖全局配置（本地优先）。

初始化完成后，在配置文件中填入你的 API Key：

```bash
# 推荐 OpenRouter（聚合多家 LLM）
# 获取 Key: https://openrouter.ai/keys
```

### 配置文件

配置文件按优先级查找：
1. **本地目录** `.simpleclaw/config.json`（当前工作目录下，适用于项目级配置）
2. **用户目录** `~/.simpleclaw/config.json`（全局配置，兜底）

配置文件格式：

```json
{
  "agents": {
    "defaults": {
      "workspace": "~/.simpleclaw/workspace",
      "model": "anthropic/claude-opus-4-5",
      "maxTokens": 8192,
      "temperature": 0.7,
      "maxToolIterations": 20,
      "apiBase": "",
      "apiKey": ""
    }
  },
  "channels": {
    "telegram": { "enabled": false, "token": "", "allowFrom": [] },
    "whatsapp": { "enabled": false, "bridgeUrl": "ws://localhost:3001" }
  },
  "providers": {
    "anthropic": { "apiKey": "" },
    "openai": { "apiKey": "" },
    "openrouter": { "apiKey": "" }
  },
  "gateway": { "host": "0.0.0.0", "port": 18790 },
  "tools": { "web": { "search": { "apiKey": "", "maxResults": 5 } } }
}
```

API Key 优先级：OpenRouter > Anthropic > OpenAI > Gemini > Zhipu > Groq > VLLM

## CLI 使用

```
simpleclaw <command> [options]
```

### `simpleclaw agent` — 与 Agent 对话

**单消息模式：**

```bash
simpleclaw agent -m "今天天气怎么样？"
simpleclaw agent -m "帮我写一个快速排序" -s "cli:coding"
```

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-m, --message` | 发送的消息内容 | 无（省略则进入交互模式） |
| `-s, --session` | Session ID | `cli:default` |

**交互模式（REPL）：**

```bash
simpleclaw agent
```

```
🤖 Interactive mode (type 'exit' or Ctrl+C to quit)

You: 你好，你能做什么？

🤖 我是 SimpleClaw，一个 AI Agent。我可以：
  - 读写文件
  - 执行 Shell 命令
  - 搜索网页
  - 动态生成并执行 Go 代码
  ...

You: 帮我用 Go 写一个斐波那契函数并运行
...
You: exit
Goodbye!
```

### `simpleclaw gateway` — 启动完整网关

启动全部服务：Agent + Channels + Cron + Heartbeat + Health + API。

```bash
simpleclaw gateway              # 正常启动
simpleclaw gateway -v           # 详细日志模式 (debug level)
```

| 参数 | 说明 |
|------|------|
| `-v, --verbose` | 启用 debug 级别日志 |

启动后：
- Agent 监听 MessageBus，处理所有入站消息
- Channel Manager 启动已配置的渠道（Telegram/WhatsApp）
- Cron Service 加载并执行定时任务
- Heartbeat 每 30 分钟检查 `HEARTBEAT.md`
- Health Server 提供 HTTP 健康检查端点
- `Ctrl+C` 优雅关闭所有服务

### `simpleclaw cron` — 管理定时任务

**列出任务：**

```bash
simpleclaw cron list             # 列出已启用的任务
simpleclaw cron list --all       # 包含已禁用的任务
```

输出示例：

```
ID           Name                 Schedule           Status     Next Run
------------ -------------------- ------------------ ---------- --------------------
a1b2c3d4     daily report         every 3600s        enabled    2026-02-08 18:00
e5f6g7h8     morning check        0 9 * * *          enabled    2026-02-09 09:00
```

**添加任务：**

```bash
# 每小时执行
simpleclaw cron add -n "hourly check" -m "检查系统状态" --every 3600

# Cron 表达式（每天早上 9 点）
simpleclaw cron add -n "morning" -m "早上好，今天有什么安排？" --cron "0 9 * * *"

# 一次性定时
simpleclaw cron add -n "reminder" -m "开会提醒" --at "2026-02-08T14:00:00"

# 执行结果投递到 WhatsApp
simpleclaw cron add -n "report" -m "生成日报" --every 86400 -d --to "user123" --channel whatsapp
```

| 参数 | 说明 |
|------|------|
| `-n, --name` | 任务名称（必填） |
| `-m, --message` | Agent 执行的消息（必填） |
| `--every <秒>` | 固定间隔（秒） |
| `--cron <表达式>` | Cron 表达式 |
| `--at <时间>` | 一次性定时（RFC3339 或 `2006-01-02T15:04:05`） |
| `-d, --deliver` | 投递结果到渠道 |
| `--to <接收者>` | 投递目标 |
| `--channel <渠道>` | 投递渠道（telegram/whatsapp） |

**管理任务：**

```bash
simpleclaw cron remove <job_id>           # 删除任务
simpleclaw cron enable <job_id>           # 启用任务
simpleclaw cron enable <job_id> --disable # 禁用任务
simpleclaw cron run <job_id>              # 手动执行
simpleclaw cron run <job_id> --force      # 强制执行（即使已禁用）
```

### `simpleclaw status` — 查看系统状态

```bash
simpleclaw status
```

输出示例：

```
🤖 SimpleClaw Status

Config:    /Users/you/.simpleclaw/config.json ✓
Workspace: /Users/you/.simpleclaw/workspace ✓
Model:     anthropic/claude-opus-4-5

Providers:
  OpenRouter  ✓ (configured)
  Anthropic   not set
  OpenAI      not set
  Gemini      not set
  Zhipu       not set
  Groq        not set
  vLLM/Local  not set

Channels:
  Telegram   disabled
  WhatsApp   disabled
```

### `simpleclaw channels` — 渠道管理

```bash
simpleclaw channels status     # 查看渠道状态（等同 simpleclaw status）
```

### 其他

```bash
simpleclaw --version           # 显示版本号
simpleclaw --help              # 显示帮助
```

## 编程接口

SimpleClaw 的每个模块都可以作为 Go 库独立使用：

```go
package main

import (
    "context"

    "github.com/linkxzhou/SimpleClaw/agent"
    "github.com/linkxzhou/SimpleClaw/bus"
    "github.com/linkxzhou/SimpleClaw/config"
    "github.com/linkxzhou/SimpleClaw/memory"
    "github.com/linkxzhou/SimpleClaw/providers"
)

func main() {
    // 1. 加载配置
    cfg, _ := config.Load("")

    // 2. 创建消息总线
    mb := bus.NewMessageBus(256, nil)

    // 3. 创建 LLM Provider（支持容错链）
    factory := providers.NewProviderFactory(providers.FactoryConfig{
        APIKeys: map[string]string{"openrouter": cfg.GetAPIKey()},
    })
    classifier := providers.NewErrorClassifier()
    cooldown := providers.NewCooldownTracker()
    chain := providers.NewFallbackChain(classifier, cooldown, factory, nil)

    // 4. 创建会话管理器
    sessDir, _ := utils.GetSessionsPath()
    sessMgr, _ := memory.NewSessionManager(sessDir, nil)

    // 5. 创建并启动 Agent
    //    注意：bus/providers/memory 的类型需要适配为 agent 内部接口
    //    参见 cmd/adapter.go 中的 BusAdapter / ProviderAdapter / SessionStoreAdapter
    a := agent.NewAgent(agent.AgentConfig{
        Bus:       busAdapter,      // 实现 agent.MessageBus 接口
        Provider:  providerAdapter, // 实现 agent.LLMProvider 接口
        Workspace: cfg.WorkspacePath(),
    })

    ctx := context.Background()
    go a.Run(ctx)

    // 6. CLI 直接模式
    response, _ := a.ProcessDirect(ctx, "Hello, what can you do?")
    println(response)
}
```

## Workspace 约定

```text
~/.simpleclaw/
├── config.json             # 全局配置
├── sessions/               # 会话持久化 (JSONL)
└── workspace/              # Agent 工作区
    ├── AGENTS.md            # Agent 行为规则
    ├── SOUL.md              # 人格定义
    ├── IDENTITY.md          # 身份信息
    ├── TOOLS.md             # 工具使用指南
    ├── USER.md              # 用户偏好
    ├── HEARTBEAT.md         # 心跳任务清单
    ├── memory/              # 记忆存储
    │   ├── long_term.md     # 长期记忆
    │   └── 2026-02-08.md    # 每日笔记
    └── skills/              # 用户自定义技能
        └── my-skill/
            └── SKILL.md
```

## 测试

项目包含完整的单元测试覆盖，18 个测试文件分布在各模块中：

```bash
go test ./...                    # 运行全部测试
go test ./agent/tools/ -v        # 运行工具测试（141 个用例）
go test ./config/ -v             # 运行配置测试
go test ./memory/ -v             # 运行存储测试
go test ./utils/ -v              # 运行工具函数测试
```

| 模块 | 测试文件 | 覆盖内容 |
|------|---------|---------|
| `agent/tools/` | 7 个 | 11 个工具的全部功能：文件操作、Shell 安全防护、Web 搜索/抓取、消息发送、子 Agent、Go 动态执行（gocode 统一引擎） |
| `bus/` | 1 个 | MessageBus 队列、订阅分发 |
| `channels/` | 1 个 | Channel 接口、权限校验 |
| `config/` | 1 个 | 配置加载/保存、端点解析 |
| `cron/` | 1 个 | 定时任务调度 |
| `goscript/` | 2 个 | SSA 编译/执行、标准库绑定 |
| `health/` | 1 个 | 健康检查服务 |
| `memory/` | 1 个 | 会话管理、状态持久化 |
| `providers/` | 1 个 | Provider 工厂、容错链、错误分类 |
| `utils/` | 2 个 | 路径管理、日志系统 |

## 参考

- https://github.com/charmbracelet/crush
- https://github.com/ghuntley/how-to-build-a-coding-agent
- https://github.com/looplj/axonhub
- https://github.com/opencode-ai/opencode
- https://github.com/smallnest/goclaw

---

## 第三方参考项目分析 (`thirdparty/`)

`thirdparty/` 目录收录了 4 个同类型的轻量级个人 AI Agent 开源项目，作为 SimpleClaw 的设计参考。以下是各项目的功能分析、通用点和差异点汇总。

### 项目概览

| 项目 | 语言 | 定位 | 版本 | 许可证 |
|------|------|------|------|--------|
| **NanoBot** | Python 3.11+ | 超轻量个人 AI Agent（受 OpenClaw 启发） | v0.1.5 | MIT |
| **PicoClaw** | Go 1.25+ | 超低资源 AI 助手（受 NanoBot 启发，$10 硬件可运行） | v0.2.4 | MIT |
| **TinyAGI** | TypeScript/Node.js 18+ | 多 Agent 多团队协作 AI 助手 | Latest | MIT |
| **ZeroClaw** | Rust (Edition 2024) | 零开销个人 AI 助手（<5MB RAM） | v0.6.9 | MIT/Apache-2.0 |

### 资源占用对比

| 指标 | NanoBot (Python) | PicoClaw (Go) | TinyAGI (Node.js) | ZeroClaw (Rust) |
|------|-----------------|---------------|-------------------|----------------|
| **RAM** | >100MB | <10MB* | >200MB (估) | <5MB |
| **启动时间 (0.8GHz)** | >30s | <1s | >10s (估) | <10ms |
| **二进制大小** | N/A (脚本) | ~8MB | N/A (脚本) | ~8.8MB |
| **最低硬件成本** | ~$50 Linux SBC | $10 开发板 | 标准 PC | $10 任意硬件 |

> *PicoClaw 近期因快速合并 PR，实际可能 10-20MB。

---

### 各项目功能详解

#### 1. NanoBot (`nanobot-main/`) — Python 超轻量 Agent

**核心定位**：以最少代码实现 OpenClaw 的核心 Agent 功能，面向教育和研究。

**核心模块**：

| 模块 | 功能 |
|------|------|
| `agent/` | Agent 循环（ReAct）、上下文构建、记忆管理、子 Agent、技能加载、生命周期 Hook |
| `agent/tools/` | 工具集：文件系统、Shell、Web 搜索/抓取、MCP、Cron、消息、沙箱、Spawn |
| `providers/` | LLM 提供商：Anthropic（原生）、OpenAI 兼容、Azure、GitHub Copilot、Codex |
| `channels/` | 聊天渠道：Telegram、Discord、Slack、WeChat、WhatsApp、DingTalk、Feishu、Matrix、QQ、Email、WeCom |
| `bus/` | 事件总线 + 消息队列 |
| `session/` | 会话管理器 |
| `cron/` | 定时任务调度 |
| `heartbeat/` | 心跳服务 |
| `memory/` | Dream 两阶段记忆系统 |
| `skills/` | 技能系统（ClawHub、Cron、GitHub、Memory、Weather 等） |
| `security/` | 网络安全 |
| `cli/` | 命令行界面 + 交互式引导 |
| `api/` | OpenAI 兼容 API 服务 |
| `templates/` | Jinja2 响应模板 |

**独特特性**：
- Dream 两阶段记忆系统（记忆整理 + 知识提炼）
- Jinja2 响应模板引擎
- Python SDK 外观模式（Facade）
- OpenAI 兼容 API 接口
- Bridge 模块（TypeScript，用于 WhatsApp 等 Node.js 依赖的渠道）
- 可通过 `pip install nanobot-ai` 一键安装

---

#### 2. PicoClaw (`picoclaw-main/`) — Go 极致低资源 Agent

**核心定位**：用 Go 从零重写 NanoBot，目标是在 $10 硬件（RISC-V/ARM/MIPS）上运行。

**核心模块**：

| 模块 | 功能 |
|------|------|
| `pkg/agent/` | Agent 引擎：循环、上下文、SubTurn、Hooks、Steering、EventBus、记忆 |
| `pkg/providers/` | LLM 提供商：OpenAI 兼容、Anthropic、AWS Bedrock、Azure、Copilot |
| `pkg/channels/` | 聊天渠道：Telegram、Discord、Slack、WeChat、WhatsApp、DingTalk、Feishu、Matrix、IRC、QQ、WeCom、MaixCAM、VK、Teams Webhook、Line、OneBot |
| `pkg/bus/` | 事件总线 |
| `pkg/config/` | TOML 配置（含安全策略、迁移） |
| `pkg/memory/` | JSONL 记忆存储 |
| `pkg/session/` | 会话管理 |
| `pkg/cron/` | 定时任务 |
| `pkg/heartbeat/` | 心跳服务 |
| `pkg/mcp/` | MCP 协议支持 |
| `pkg/routing/` | 智能模型路由 |
| `pkg/tools/` | 工具集 |
| `pkg/skills/` | 技能系统 |
| `pkg/devices/` | 硬件设备集成 |
| `pkg/auth/` | OAuth 认证 |
| `pkg/gateway/` | HTTP 网关 |
| `pkg/health/` | 健康检查 |
| `pkg/isolation/` | 沙箱隔离 |
| `pkg/media/` | 多媒体处理 |
| `pkg/audio/` | 语音识别/合成（ASR/TTS） |
| `pkg/tokenizer/` | Token 计数 |
| `web/` | Web UI Launcher |

**独特特性**：
- 跨架构单二进制：x86_64、ARM64、MIPS、RISC-V、LoongArch
- 系统托盘 UI（Windows & Linux）
- Web UI Launcher（浏览器配置界面）
- 视觉管线（Vision Pipeline）：图片自动 base64 编码
- 智能模型路由：简单查询走轻量模型，节省 API 费用
- 硬件设备集成（MaixCAM 等）
- Android APK 支持
- 95% 核心代码由 AI Agent 生成

---

#### 3. TinyAGI (`tinyagi-main/`) — 多 Agent 多团队协作

**核心定位**：多 Agent、多团队、多渠道的 24/7 AI 助手，强调团队协作和任务管理。

**核心模块**：

| 模块 | 功能 |
|------|------|
| `packages/core/` | 核心引擎：Agent、配置、调用、记忆、队列、路由、调度、插件 |
| `packages/channels/` | 聊天渠道：Discord、Telegram、WhatsApp |
| `packages/cli/` | CLI 工具：Agent/Team/Channel/Provider/Schedule 管理 |
| `packages/server/` | HTTP API 服务：RESTful 路由（Agent、消息、队列、任务、团队、聊天室） |
| `packages/teams/` | 团队协作：聊天室、对话、路由 |
| `packages/main/` | 主入口：渠道管理、心跳 |
| `packages/visualizer/` | TUI 可视化：团队链路可视化、聊天室查看器 |
| `tinyoffice/` | Web 门户（Next.js）：仪表盘、聊天、Agent/Team 管理、看板任务、日志 |

**独特特性**：
- **多 Agent 多团队**：Agent 可组成团队，通过链式执行和扇出协作
- **团队聊天室**：每个团队有持久化异步聊天室，实时 CLI 查看器
- **TinyOffice Web 门户**：浏览器端仪表盘、聊天、看板任务管理、组织架构图
- **SQLite 队列**：原子事务、重试逻辑、死信管理
- **插件系统**：消息钩子和事件监听器扩展
- **TUI 仪表盘**：实时团队可视化和聊天室查看
- **自定义 Provider**：支持任意 OpenAI/Anthropic 兼容端点
- **Sender Pairing**：发送者配对认证机制

---

#### 4. ZeroClaw (`zeroclaw-master/`) — Rust 零开销 Agent

**核心定位**：100% Rust 实现，零开销、零妥协的个人 AI 助手，功能最全面。

**核心模块**：

| 模块 | 功能 |
|------|------|
| `src/agent/` | Agent 引擎：循环、分类器、上下文压缩、调度器、评估器、历史修剪、人格、提示构建、思维链 |
| `src/providers/` | LLM 提供商（20+ 后端） |
| `src/channels/` | **30+ 聊天渠道**：WhatsApp、Telegram、Slack、Discord、Signal、iMessage、Matrix、IRC、Email、Bluesky、Nostr、Mattermost、Nextcloud Talk、DingTalk、Lark、QQ、Reddit、LinkedIn、Twitter、MQTT、WeChat Work、VoiceCall 等 |
| `src/gateway/` | HTTP/WS/SSE 网关：API、配对、WebAuthn、Canvas、TLS |
| `src/memory/` | 高级记忆系统：嵌入向量、知识图谱、Qdrant、衰减、整合、审计、快照、响应缓存 |
| `src/tools/` | **70+ 工具**：Shell、文件、浏览器、Git、Web、MCP、Jira、Notion、Google Workspace、Microsoft 365 |
| `src/security/` | 安全系统 |
| `src/trust/` | 信任模型 |
| `src/approval/` | 审批网关 |
| `src/cron/` | 定时任务（调度器 + 持久化存储） |
| `src/hooks/` | 生命周期钩子 |
| `src/skills/` + `src/skillforge/` | 技能平台 + 技能锻造 |
| `src/sop/` | 标准操作流程（SOP）：事件驱动工作流自动化 |
| `src/hardware/` | 硬件外设：ESP32、STM32、Arduino、Raspberry Pi GPIO |
| `src/rag/` | RAG 检索增强生成 |
| `src/tunnel/` | 隧道支持：Cloudflare、Tailscale、ngrok、OpenVPN |
| `src/nodes/` | 节点管理 |
| `src/routines/` | 自主例程 |
| `src/cost/` | 成本追踪 |
| `src/observability/` | 可观测性 |
| `src/plugins/` | 插件系统 |
| `src/integrations/` | 集成注册表 |
| `src/tui/` | 终端 UI |
| `src/verifiable_intent/` | 可验证意图 |
| `crates/robot-kit/` | 机器人套件（驱动、情感、语音、视觉、安全） |
| `web/` | React 19 + Vite Web 仪表盘 |
| `firmware/` | 固件目标（ESP32、STM32、Arduino） |

**独特特性**：
- **极致性能**：<5MB RAM、<10ms 启动、单二进制无运行时依赖
- **30+ 聊天渠道**（最全面）
- **70+ 工具**（最丰富）
- **高级记忆系统**：嵌入向量、知识图谱、Qdrant 向量数据库、记忆衰减、整合、审计
- **RAG 检索增强生成**
- **SOP 标准操作流程**：事件驱动工作流自动化（MQTT、Webhook、Cron、外设触发）
- **硬件外设集成**：ESP32、STM32、Arduino、Raspberry Pi + 固件编译
- **机器人套件**（robot-kit crate）：驱动、情感表达、语音、视觉、安全
- **安全体系**：Landlock（Linux）、Bubblewrap 沙箱、129+ 安全测试
- **隧道支持**：Cloudflare、Tailscale、ngrok、OpenVPN
- **OpenClaw 迁移工具**：`zeroclaw migrate openclaw`
- **可验证意图**（Verifiable Intent）
- **React 19 Web 仪表盘**

---

### 通用点（四个项目共有）

| 维度 | 共同特征 |
|------|---------|
| **Agent 循环** | 均基于 ReAct（Reasoning + Acting）模式，LLM → Tool → LLM 迭代循环 |
| **工具系统** | 均内置文件操作、Shell 执行、Web 搜索/抓取等基础工具 |
| **MCP 支持** | 均支持 Model Context Protocol，可连接外部 MCP Server 扩展能力 |
| **多渠道通信** | 均支持 Telegram、Discord、WhatsApp 等主流聊天平台 |
| **多 LLM 提供商** | 均支持 Anthropic、OpenAI 及 OpenAI 兼容 API（OpenRouter 等） |
| **定时任务** | 均内置 Cron 调度系统，支持定时/周期/一次性任务 |
| **心跳机制** | 均有 Heartbeat 服务，周期性唤醒 Agent |
| **会话管理** | 均支持持久化会话，跨重启恢复对话上下文 |
| **记忆系统** | 均有长期记忆存储机制 |
| **技能系统** | 均支持可扩展的技能/插件机制 |
| **CLI 界面** | 均提供命令行交互界面 |
| **Docker 支持** | 均提供 Dockerfile 和 Docker Compose 配置 |
| **MIT 许可** | 均采用 MIT 许可证（ZeroClaw 额外提供 Apache-2.0） |
| **安全意识** | 均有工作区隔离、路径安全校验等基础安全措施 |
| **事件总线** | 均采用事件驱动/消息总线架构解耦 Agent 与渠道 |

### 差异点对比

| 维度 | NanoBot (Python) | PicoClaw (Go) | TinyAGI (Node.js) | ZeroClaw (Rust) |
|------|-----------------|---------------|-------------------|----------------|
| **设计哲学** | 最少代码、教育研究 | 极致低资源、嵌入式 | 多 Agent 团队协作 | 零开销、功能最全 |
| **渠道数量** | ~12 个 | ~16 个 | 3 个 | **30+ 个** |
| **工具数量** | ~12 个 | ~15 个（估） | ~10 个（估） | **70+ 个** |
| **记忆系统** | Dream 两阶段 | JSONL 存储 | SQLite 持久化 | **向量嵌入 + 知识图谱 + Qdrant** |
| **多 Agent** | 子 Agent（Spawn） | 子 Agent（SubTurn） | **多 Agent + 多团队 + 链式/扇出** | Hands 自主 Agent 群 |
| **Web UI** | 无 | Web UI Launcher | **TinyOffice（Next.js 全功能门户）** | React 19 仪表盘 |
| **硬件支持** | 无 | MaixCAM、Android | 无 | **ESP32/STM32/Arduino/RPi + 固件** |
| **安全沙箱** | 基础沙箱 | 隔离模块 | 无专门沙箱 | **Landlock + Bubblewrap + 129 测试** |
| **配置格式** | YAML/JSON | TOML | JSON | TOML |
| **包管理** | PyPI (`pip install`) | 预编译二进制 | npm (`curl \| bash`) | Homebrew/Cargo/预编译 |
| **模型路由** | 无 | **智能路由**（按复杂度选模型） | 自定义 Provider | 模型路由 + Failover |
| **语音能力** | 无 | ASR/TTS | 无 | 语音转录 + TTS |
| **RAG** | 无 | 无 | 无 | **检索增强生成** |
| **SOP 工作流** | 无 | 无 | 无 | **事件驱动 SOP** |
| **隧道/远程** | 无 | 无 | 无 | **Cloudflare/Tailscale/ngrok** |
| **迁移工具** | 无 | 无 | 无 | **OpenClaw 迁移** |
| **跨平台** | 依赖 Python 运行时 | **RISC-V/ARM/MIPS/x86 单二进制** | 依赖 Node.js 运行时 | ARM/x86 单二进制 |
| **独特亮点** | Jinja2 模板、Python SDK、Bridge | AI 自举生成代码、$10 硬件 | 团队聊天室、看板任务、TUI 可视化 | 机器人套件、可验证意图、知识图谱 |

### 值得借鉴的具体方案（源码级分析）

以下从四个项目中提炼出 **具体可落地** 的设计方案，按优先级排序。每个方案附带源码出处和关键实现细节。

---

#### 方案 1：Dream 两阶段记忆整合（来自 NanoBot）

**源码**：`nanobot/agent/memory.py` → `Dream` 类

**核心思路**：定时 Cron 任务驱动的两阶段记忆处理，将对话历史自动整理为结构化知识。

**Phase 1 — 分析阶段**（纯 LLM 调用）：
```python
# 读取 history.jsonl 中未处理的条目（基于 cursor 游标增量读取）
entries = store.read_unprocessed_history(since_cursor=last_cursor)
# 连同 MEMORY.md / SOUL.md / USER.md 当前内容一起送入 LLM
# LLM 输出：哪些是新事实、哪些是偏好变化、哪些过时需要删除
analysis = await provider.chat(system=dream_phase1_prompt, user=history_text + file_context)
```

**Phase 2 — 编辑阶段**（AgentRunner + 工具调用）：
```python
# 给 LLM 注册 read_file + edit_file 两个工具，限定只能操作 workspace 内文件
# LLM 根据 Phase 1 分析结果，对 MEMORY.md / SOUL.md / USER.md 做增量编辑
result = await runner.run(AgentRunSpec(
    tools=[ReadFileTool(workspace), EditFileTool(workspace)],
    initial_messages=[system_prompt, phase1_analysis],
    max_iterations=10,
))
# 编辑完成后 Git 自动提交，记录变更日志
store.git.auto_commit(f"dream: {timestamp}, {len(changelog)} change(s)")
```

**借鉴价值**：SimpleClaw 当前的 `MemoryStore` 只做文件读写，缺乏自动整理能力。可以在 `cron/` 中增加 Dream 任务，利用现有的 `goscript` 或直接 LLM 调用，定期将 `memory/` 目录下的每日笔记整合到 `long_term.md` 中。游标机制（`.dream_cursor`）保证增量处理，不重复分析。

---

#### 方案 2：SubTurn 子回合机制（来自 PicoClaw）

**源码**：`pkg/agent/subturn.go`

**核心思路**：在 Agent 主循环内嵌套独立的子 Agent 回合，支持同步/异步两种模式，带深度限制和并发控制。

**关键设计**：
```go
type SubTurnConfig struct {
    Model           string        // 子回合可用不同模型（省钱）
    Async           bool          // true=后台运行，结果通过 channel 回传
    Critical        bool          // true=父结束后子继续运行
    Timeout         time.Duration // 独立超时（默认 5min）
    MaxContextRunes int           // 软截断（模型上下文窗口 * 0.75）
    InitialTokenBudget *atomic.Int64 // 共享 token 预算（团队级限额）
}
```

**并发安全**：
```go
// 信号量控制并发数（默认 5 个子回合）
select {
case parentTS.concurrencySem <- struct{}{}: // 获取槽位
case <-timeoutCtx.Done(): return nil, ErrConcurrencyTimeout
}
// 子回合使用独立 context（非继承父 context），拥有自己的超时
childCtx, cancel := context.WithTimeout(context.Background(), timeout)
// 临时会话存储（ephemeral），子回合不污染父会话历史
ephemeralStore := newEphemeralSession(nil)
agent := *baseAgent // 浅拷贝 + 独立工具注册表
```

**借鉴价值**：SimpleClaw 的 `SubagentManager` 已有子 Agent 概念，但 PicoClaw 的 SubTurn 更精细：① 支持同步阻塞返回结果（不必都走消息总线）；② 共享 token 预算原子计数器，团队级费用控制；③ `Critical` 标记让重要子任务在父结束后继续运行；④ 软截断 `MaxContextRunes` 避免硬错误。

---

#### 方案 3：多 Agent 团队协作 + 扇出执行（来自 TinyAGI）

**源码**：`packages/teams/src/routing.ts` + `packages/main/src/index.ts`

**核心思路**：Agent 通过 DSL 标签在 LLM 响应中触发跨 Agent 协作，形成 leader → worker 扇出拓扑。

**Bracket Tag DSL**：
```
[@coder: 请实现这个功能]        → 路由到 coder agent
[@coder,reviewer: 请审查代码]    → 同时路由到两个 agent（扇出）
[#engineering: 大家注意，API 已更新] → 广播到整个团队聊天室
```

**Promise Chain 串行保证**：
```typescript
const agentChains = new Map<string, Promise<void>>();
// 同一 agent 的消息串行处理，不同 agent 并行
const newChain = currentChain.catch(() => {}).then(async () => {
    for (const msg of messages) await processMessage(msg);
});
```

**SQLite 队列原子 Claim**：
```typescript
// IMMEDIATE 事务级别，防并发消费
function claimAllPendingMessages(agentId) {
    return db.transaction(() => {
        const rows = db.prepare('SELECT * FROM messages WHERE status=? AND agent=?').all('pending', agentId);
        db.prepare('UPDATE messages SET status=? WHERE id IN (...)').run('queued', ...ids);
        return rows;
    }).immediate(); // ← 关键：事务开始即获取写锁
}
// 消息状态机：pending → queued → processing → completed | dead（重试 5 次后进死信）
```

**借鉴价值**：SimpleClaw 可以在 `SubagentManager` 基础上扩展：① Agent 响应中解析 `[@agent: message]` 标签实现协作路由，比 `spawn` 工具更自然；② 用 Go 的 channel + sync.Map 实现类似 Promise Chain 的串行保证；③ 死信队列 + 重试机制增强消息可靠性。

---

#### 方案 4：LLM 成本追踪 + 预算网关（来自 ZeroClaw）

**源码**：`src/cost/tracker.rs`

**核心思路**：每次 LLM 调用前检查预算，调用后记录费用，支持日/月限额 + 预警阈值。

**关键实现**：
```rust
pub struct CostTracker {
    config: CostConfig,                    // daily_limit_usd, monthly_limit_usd, warn_at_percent
    storage: Arc<Mutex<CostStorage>>,      // JSONL 持久化
    session_costs: Arc<Mutex<Vec<CostRecord>>>, // 会话级内存快照
}

// 调用前检查
pub fn check_budget(&self, estimated_cost_usd: f64) -> Result<BudgetCheck> {
    let (daily_cost, monthly_cost) = storage.get_aggregated_costs()?;
    if daily_cost + estimated_cost_usd > self.config.daily_limit_usd {
        return Ok(BudgetCheck::Exceeded { period: UsagePeriod::Day, ... });
    }
    // 预警阈值（如 80%）
    if projected >= daily_limit * warn_threshold {
        return Ok(BudgetCheck::Warning { ... });
    }
    Ok(BudgetCheck::Allowed)
}

// 调用后记录
pub fn record_usage(&self, usage: TokenUsage) -> Result<()> {
    let record = CostRecord::new(&self.session_id, usage); // 含 model, tokens, cost_usd, timestamp
    storage.add_record(record.clone())?;  // JSONL 追加写入
    session_costs.push(record);           // 内存更新
}
```

**借鉴价值**：SimpleClaw 的 `providers/` 已统计 token 用量，但缺乏费用追踪和预算控制。可在 `FallbackChain` 中嵌入 CostTracker：① 每次 `Chat()` 前 `check_budget()`，超限返回友好错误而非继续调用；② 用 JSONL 按日存储，支持 `simpleclaw status` 显示当日/当月费用；③ 预警阈值避免账单惊喜。

---

#### 方案 5：记忆衰减 + 分类存储（来自 ZeroClaw）

**源码**：`src/memory/decay.rs` + `src/memory/consolidation.rs` + `src/memory/traits.rs`

**指数衰减公式**：
```rust
// score * 2^(-age_days / half_life_days)
// 默认半衰期 7 天：7 天后分数降至 50%，14 天降至 25%
// Core 类别记忆永不衰减（"evergreen"）
pub fn apply_time_decay(entries: &mut [MemoryEntry], half_life_days: f64) {
    for entry in entries.iter_mut() {
        if entry.category == MemoryCategory::Core { continue; } // 核心记忆豁免
        let age_days = now.signed_duration_since(ts).num_seconds() as f64 / 86400.0;
        let decay_factor = (-age_days / half_life * LN_2).exp();
        entry.score = Some(score * decay_factor);
    }
}
```

**记忆分类**：
```rust
pub enum MemoryCategory {
    Core,         // 核心事实（永不衰减）
    Daily,        // 每日对话摘要
    Conversation, // 对话上下文
    Procedural,   // 操作步骤
}
```

**两阶段整合**（每轮对话后触发）：
```rust
pub struct ConsolidationResult {
    pub history_entry: String,        // 当日摘要 → Daily 类别
    pub memory_update: Option<String>, // 新事实 → Core 类别
    pub facts: Vec<String>,           // 原子事实提取
    pub trend: Option<String>,        // 趋势/模式识别
}
```

**借鉴价值**：SimpleClaw 的 `MemoryStore` 当前是平面文件存储，缺乏优先级排序。可以：① 给 `MEMORY.md` 中的条目增加时间戳和分类标签；② 在 `ContextBuilder` 中按衰减分数排序，上下文窗口有限时优先保留高分条目；③ 核心事实（用户名、偏好等）标记为 Core 永不丢弃。

---

#### 方案 6：审批网关 + 自治级别（来自 ZeroClaw）

**源码**：`src/approval/mod.rs`

**核心思路**：工具调用前的拦截层，按自治级别和工具类型决定是否需要人工确认。

**三级自治**：
```rust
enum AutonomyLevel {
    Full,       // 全部自动执行，不弹确认
    Supervised, // 默认需确认，auto_approve 列表豁免
    ReadOnly,   // 只允许读操作，写操作全部拒绝
}
```

**审批决策三选一**：
```rust
enum ApprovalResponse {
    Yes,    // 本次允许
    No,     // 本次拒绝
    Always, // 允许并加入会话级白名单（后续同工具不再询问）
}
```

**非交互模式**（渠道驱动，无人值守）：
```rust
// CLI 模式：弹 stdin 提示等待确认
// Channel 模式：需要确认的工具自动拒绝（非静默放行）
pub fn for_non_interactive(config) -> Self {
    Self { non_interactive: true, ... } // auto_approve 的工具仍然放行
}
```

**审计日志**：每次审批决策记录 `{timestamp, tool_name, arguments_summary, decision, channel}`。

**借鉴价值**：SimpleClaw 的 `exec` 工具有基础的安全防护（Shell 命令黑名单），但缺乏系统级的审批机制。可以在 `agent/tools/registry.go` 中增加 `ApprovalMiddleware`，对 `exec`、`write_file`、`edit_file` 等写操作在 CLI 交互模式下弹确认，在 Telegram/WhatsApp 渠道下按 `auto_approve` 白名单放行。

---

#### 方案 7：SOP 标准操作流程（来自 ZeroClaw）

**源码**：`src/sop/types.rs` + `src/sop/mod.rs`

**核心思路**：用 TOML + Markdown 定义可复用的工作流，支持事件触发 + 多种执行模式。

**SOP 定义结构**：
```toml
# workspace/sops/deploy-pipeline/SOP.toml
[sop]
name = "deploy-pipeline"
description = "标准部署流水线"
priority = "high"
execution_mode = "supervised"
cooldown_secs = 300
max_concurrent = 1

[[triggers]]
type = "webhook"
path = "/deploy"

[[triggers]]
type = "cron"
expression = "0 2 * * *"

[[triggers]]
type = "mqtt"
topic = "ci/build-success"
condition = "branch == 'main'"
```

**五种执行模式**：
```rust
enum SopExecutionMode {
    Auto,          // 全自动
    Supervised,    // 开始前确认，然后自动
    StepByStep,    // 每步确认
    PriorityBased, // Critical/High 自动，Normal/Low 需确认
    Deterministic, // 无 LLM，步骤输出直接管道到下一步输入
}
```

**步骤定义**（SOP.md，Markdown 解析）：
```markdown
## Step 1: 拉取最新代码
tools: exec
执行 `git pull origin main`

## Step 2: 运行测试 [checkpoint]
tools: exec
执行 `go test ./...`，检查是否全部通过

## Step 3: 构建部署
tools: exec, write_file
构建二进制并部署到生产环境
```

**借鉴价值**：SimpleClaw 的 `cron/` 只能执行单条消息，缺乏多步骤编排。SOP 可以作为 `skills/` 的增强：① 在 `workspace/sops/` 目录下定义流程；② 解析 Markdown 步骤 + TOML 元数据；③ Checkpoint 步骤在关键操作前暂停等待确认；④ `Deterministic` 模式不走 LLM，直接工具链式执行，节省 API 费用。

---

#### 方案 8：智能上下文截断（来自 PicoClaw + NanoBot）

**源码**：PicoClaw `subturn.go` 的 `MaxContextRunes` + NanoBot `memory.py` 的 `maybe_consolidate_by_tokens`

**PicoClaw 软截断**：
```go
// MaxContextRunes = 0 → 自动计算：模型上下文窗口 * 0.75
// 超过时截断旧消息，保留 system 消息和最近上下文
// 先软截断，再靠 provider 硬错误兜底
```

**NanoBot 循环归档**：
```python
async def maybe_consolidate_by_tokens(self, session):
    budget = context_window - max_completion - SAFETY_BUFFER
    target = budget // 2  # 目标：压缩到预算的一半
    while estimated > target:
        boundary = pick_consolidation_boundary(session, excess_tokens)
        chunk = session.messages[last_consolidated:boundary]
        await archive(chunk)  # LLM 摘要后写入 history.jsonl
        session.last_consolidated = boundary
        # 重新估算，直到 fit 或无安全边界可切
```

**关键细节**：`pick_consolidation_boundary()` 寻找"安全切割点"——避免在工具调用对（tool_use → tool_result）中间切割，保持语义完整性。

**借鉴价值**：SimpleClaw 的 `Session.GetHistory(maxMessages)` 只按消息条数截取，不考虑 token 数。可以：① 在 `ContextBuilder` 中估算 token 总量（简单按字符数 * 系数即可）；② 超过阈值时触发归档，将旧消息摘要后写入 `memory/`；③ 保留最近 N 条完整消息 + 历史摘要的混合策略。

---

#### 方案 9：插件系统双机制 — 管道 + 事件（来自 TinyAGI）

**源码**：`packages/core/src/plugins.ts`

**管道模式**（消息变换链）：
```typescript
// 消息依次经过所有插件的 transformOutgoing 处理
async function runOutgoingHooks(message, ctx) {
    let text = message;
    for (const plugin of loadedPlugins) {
        if (plugin.hooks?.transformOutgoing) {
            const result = await plugin.hooks.transformOutgoing(text, ctx);
            text = typeof result === 'string' ? result : result.text;
        }
    }
    return { text };
}
// 处理流水线：incoming hooks → Agent 处理 → outgoing hooks → 响应
```

**事件监听模式**（解耦观察）：
```typescript
// 插件通过 ctx.on() 注册事件处理器
function activate(ctx: PluginContext) {
    ctx.on('agent:invoke', (event) => { /* 记录日志 */ });
    ctx.on('agent:mention', (event) => { /* 追踪协作 */ });
    ctx.on('*', (event) => { /* 通配符：监听所有事件 */ });
}
// 事件不阻塞主流程，错误被吞掉
function emitEvent(type, data) {
    for (const listener of eventListeners) {
        try { listener(type, data); } catch { /* 不中断 */ }
    }
}
```

**自动发现**：插件放在 `~/.tinyagi/plugins/<name>/index.ts`，启动时自动扫描加载。

**借鉴价值**：SimpleClaw 的 `SkillsLoader` 已有技能加载，但缺乏消息拦截能力。可以在 `bus/` 中增加中间件链：① `InboundMiddleware` 在消息进入 Agent 前做预处理（敏感词过滤、翻译等）；② `OutboundMiddleware` 在响应发出前做后处理（格式化、审计等）；③ Go 接口实现比 JS 更类型安全。

---

#### 方案 10：Sender Pairing 配对认证（来自 TinyAGI）

**源码**：`packages/core/src/pairing.ts`

**流程**：
```
新用户发消息 → 检查 approved 列表
  ├── 已认证 → 放行
  └── 未认证 → 生成 8 位配对码（去除易混淆字符 I/O/0/1）
                → 返回 "请联系管理员提供配对码: ABCD1234"
                → 管理员执行 approve <code> 完成配对
                → 原子写入（write tmp → rename）防数据损坏
```

**借鉴价值**：SimpleClaw 的 `AllowFrom` 白名单是硬编码的 ID 列表，新用户需要改配置重启。配对码机制更灵活：① 新用户自助申请；② 管理员通过 CLI 或渠道消息审批；③ JSON 文件持久化，无需重启。

---

### SimpleClaw 独有优势

| 特性 | 说明 |
|------|------|
| **goscript SSA 解释器** | 四个参考项目均无类似能力。Agent 可以动态生成并执行 Go 代码，在沙箱中安全运行，无需系统安装 Go 编译器 |
| **Go 自举** | `go_run` + `go_agent` 实现能力递归扩展，Agent 用 Go 写 Go |
| **类型别名零开销** | `types.go` 通过 Go 类型别名消除 bus ↔ agent 类型转换，其他项目多用适配器模式 |