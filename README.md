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

### 9. 前端 API 服务 (`cmd/api.go`)

为 Vue 3 前端提供统一的 HTTP API 接口，复用 Health Server 端口：

- **`POST /api/rpc`** — 统一 JSON-RPC 入口，支持 22 个方法（配置读写、会话管理、定时任务 CRUD、系统状态等）
- **`GET /api/events`** — SSE 事件流，实时推送 Agent 响应到前端
- **`GET /api/health`** — 前端专用健康检查
- 所有 RPC 方法通过 `{ "method": "...", "params": {...} }` 格式调用

### 10. 前端界面 (`frontend/`)

Vue 3 + Ant Design Vue 构建的 Web UI，PC/移动端自适应：

- **聊天界面** — 支持 Markdown 渲染、代码高亮、思考过程折叠展示
  - **头像系统**：用户蓝色圆形 + 字母 "U"，AI 紫色渐变圆形 + SVG 机器人图标
  - **时间戳**：今天显示 `HH:MM`，其他日期显示 `Mon DD HH:MM`
  - **嵌入式输入框**：发送按钮位于输入框右下角，最少 4 行，支持图片/文件附件预览
  - **Gateway 状态监听**：`watch(gatewayStore.status.state)` 确保网关初始化后自动加载会话和历史
  - **SSE 流式响应**：通过 `/api/events` 实时显示 Agent 回复
- **主题切换** — 深色/浅色模式，基于 CSS 变量全局切换
- **移动端适配** — 侧边栏变底部 TabBar，响应式断点布局
- **国际化** — 中/英/日三语支持

### 11. 消息路由 (`routing/`)

多 Agent 消息路由解析：

- **`RouteResolver`** — 7 级优先级级联路由：peer > guild > team > account > channel_wildcard > channel_exact > default
- **`BindingConfig`** — 路由绑定规则（agent/channel/peer/guild/team/account）
- 支持通配符渠道匹配（`*`）

### 11. 工具函数 (`utils/`)

通用辅助函数 + 日志系统：

- **路径管理**：`EnsureDir` / `GetDataPath` / `GetWorkspacePath` / `GetSessionsPath` / `GetMemoryPath` / `GetSkillsPath` / `ValidatePath` / `ExpandHome`
- **时间**：`TodayDate` / `Timestamp`
- **字符串**：`TruncateString` / `SafeFilename`
- **Session Key**：`ParseSessionKey` / `BuildSessionKey`
- **日志系统**（基于 `log/slog` + lumberjack）：
  - 双输出：控制台（TextHandler）+ 文件（JSONHandler + 自动轮转）
  - `slog.LevelVar` 支持运行时动态调整日志级别
  - 统一接口：`LogDebug/LogInfo/LogWarn/LogError/LogFatal` + 格式化版本

### 12. Go 脚本解释器 (`goscript/`)

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
│   ├── api.go              # 前端 API 服务（RPC + SSE + Health）
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
├── frontend/               # Vue 3 前端（PC + 移动端自适应）
│   ├── src/                # 源码（layouts/pages/stores/i18n）
│   │   ├── components/     # ChatMessage（头像 + 时间戳 + Markdown 渲染）
│   │   ├── pages/          # Chat（嵌入式输入框 + 图片/文件附件 + SSE 流）
│   │   └── stores/         # chatStore（Gateway 状态监听自动加载）
│   └── package.json        # 依赖管理
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
- API Server 提供前端 RPC + SSE 接口（复用 Health Server 端口）
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