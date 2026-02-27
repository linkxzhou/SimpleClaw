# SimpleClaw (SimpleClaw) 重构优化方案 — 整体架构设计

## 1. 背景与目标

### 1.1 当前状态

SimpleClaw (SimpleClaw) 是一个纯 Go 实现的轻量级个人 AI Agent 框架，核心功能已基本完备：
- Agent ReAct 循环引擎（最多 20 次工具迭代）
- 11 个内置工具（含独创的 `go_run` / `go_agent` GoScript 动态执行）
- 消息总线 + 事件驱动架构
- 7 个 LLM Provider 支持（通用 OpenAI 兼容 API）
- Telegram / WhatsApp 通道（框架已搭建，API 集成 TODO）
- 定时任务 / 心跳服务 / 会话持久化 / 记忆系统 / 技能系统

### 1.2 参考项目分析

| 维度 | PicoClaw (Go) | Nanobot (Python) | SimpleClaw (Go) |
|------|--------------|-------------------|------------------|
| 依赖数 | ~20 直接依赖 | ~10 (litellm等) | 3 直接依赖 |
| LLM Provider | 17+ 厂商, 原生多SDK | 7 厂商, LiteLLM统一 | 7 厂商, OpenAI兼容 |
| 容错 | FallbackChain + Cooldown + ErrorClassifier | 无 | 无 |
| 路由 | 7 级优先级多 Agent 路由 | 单 Agent | 单 Agent |
| Channel | 13 个（全部实现） | 2 个（Telegram + WhatsApp） | 2 个（框架级 TODO） |
| 安全 | 40+ Shell 拒绝规则 + 工作区沙箱 | 基础超时 | 基础超时 |
| 认证 | OAuth/PKCE + 多Provider | 无 | 无 |
| 技能 | 三级加载 + 远程市场 | 双级加载 | 双级加载 |
| 特色 | 设备集成 + 语音转录 | LiteLLM统一 + Typer CLI | GoScript 解释器 |

### 1.3 优化目标

**核心原则：保持极简依赖（Go 哲学），同时吸收 PicoClaw 的生产级能力和 Nanobot 的优雅设计**

1. **可靠性**：Provider 容错 + 错误分类 + 冷却退避
2. **安全性**：Shell 命令安全模型 + 工作区沙箱
3. **扩展性**：多 Agent 路由 + 技能市场 + 设备集成
4. **完整性**：完成 Channel 实现 + Cron 表达式 + Session 集成
5. **独特性**：保留并增强 GoScript 解释器（核心差异化）

---

## 2. 目标架构

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           SimpleClaw Gateway                            │
│                                                                         │
│  ┌─────────────┐  ┌─────────────┐  ┌──────────┐  ┌──────────────────┐  │
│  │ HealthServer│  │ CronService │  │Heartbeat │  │  DeviceService   │  │
│  │ /health     │  │ at/every/   │  │ 30min    │  │  USB Monitor     │  │
│  │ /ready      │  │ cron expr   │  │ HEARTBEAT│  │  (Linux)         │  │
│  └─────────────┘  └──────┬──────┘  └────┬─────┘  └────────┬─────────┘  │
│                          │              │                  │            │
│  ┌───────────────────────┴──────────────┴──────────────────┘            │
│  │                                                                      │
│  │                    ┌──────────────────────┐                          │
│  │                    │     MessageBus        │                          │
│  │                    │  inbound (chan 256)   │                          │
│  │                    │  outbound (chan 256)  │                          │
│  │                    └──────────┬───────────┘                          │
│  │                               │                                      │
│  │  ┌────────────────────────────┼────────────────────────────┐        │
│  │  │                    AgentLoop                             │        │
│  │  │                                                          │        │
│  │  │  ┌──────────────┐  ┌───────────────┐  ┌──────────────┐ │        │
│  │  │  │RouteResolver │  │ AgentRegistry │  │  StateManager│ │        │
│  │  │  │ 7-level      │  │ multi-agent   │  │  atomic save │ │        │
│  │  │  └──────┬───────┘  └───────┬───────┘  └──────────────┘ │        │
│  │  │         │                  │                             │        │
│  │  │         └──────────────────┘                             │        │
│  │  │                    │                                     │        │
│  │  │         ┌──────────▼──────────┐                         │        │
│  │  │         │   AgentInstance      │ (per agent)            │        │
│  │  │         │                      │                         │        │
│  │  │         │  ContextBuilder ─────┤── Memory + Skills       │        │
│  │  │         │  ToolRegistry ───────┤── 13+ 内置工具          │        │
│  │  │         │  SessionManager ─────┤── JSONL 持久化          │        │
│  │  │         │  SubagentManager ────┤── 后台/同步子 Agent     │        │
│  │  │         └──────────┬──────────┘                         │        │
│  │  │                    │                                     │        │
│  │  │         ┌──────────▼──────────┐                         │        │
│  │  │         │   Provider Layer     │                         │        │
│  │  │         │                      │                         │        │
│  │  │         │  FallbackChain ──────┤── 多候选容错             │        │
│  │  │         │  CooldownTracker ────┤── 指数退避冷却          │        │
│  │  │         │  ErrorClassifier ────┤── 40+ 模式分类          │        │
│  │  │         │  OpenAICompat ───────┤── 通用 HTTP Provider    │        │
│  │  │         └─────────────────────┘                         │        │
│  │  └─────────────────────────────────────────────────────────┘        │
│  │                                                                      │
│  │  ┌────────────────────────────────────────────────────────┐         │
│  │  │                  Channel Layer                          │         │
│  │  │                                                         │         │
│  │  │  Telegram │ Discord │ QQ │ DingTalk │ Slack │ Feishu  │         │
│  │  │  WhatsApp │ WeCom   │ LINE │ OneBot │ CLI             │         │
│  │  └────────────────────────────────────────────────────────┘         │
│  └──────────────────────────────────────────────────────────────────────┘
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 3. 模块划分与包结构

```
SimpleClaw/
├── cmd/                        # CLI 入口
│   ├── main.go                 # 命令路由
│   ├── gateway.go              # 网关模式（长驻服务）
│   ├── agent.go                # CLI 交互模式
│   ├── onboard.go              # 初始化向导
│   ├── cron.go                 # 定时任务管理
│   ├── status.go               # 状态查看
│   ├── auth.go                 # [新增] 认证管理
│   └── skills.go               # [新增] 技能管理
│
├── config/                     # 配置系统
│   ├── schema.go               # [增强] 配置结构（新增 Bindings/ModelList/Auth/Devices）
│   ├── loader.go               # 配置加载
│   └── defaults.go             # [新增] 默认配置常量
│
├── agent/                      # Agent 核心引擎
│   ├── loop.go                 # [重构] 主循环（集成路由 + 多Agent）
│   ├── instance.go             # [新增] Agent 实例（独立工具集/会话/模型）
│   ├── registry.go             # [新增] Agent 注册表
│   ├── context.go              # 上下文构建器
│   ├── memory.go               # 记忆系统
│   ├── skills.go               # 技能加载器
│   ├── subagent.go             # 子 Agent 管理器
│   ├── types.go                # 类型定义
│   └── tools/                  # 工具实现
│       ├── registry.go         # 工具注册表
│       ├── fs.go               # 文件系统工具
│       ├── shell.go            # [增强] Shell 工具（安全模型）
│       ├── web.go              # Web 搜索/抓取
│       ├── message.go          # 消息发送
│       ├── spawn.go            # 异步子 Agent
│       ├── gorun.go            # Go 动态执行
│       ├── goagent.go          # Go 子 Agent
│       ├── cron_tool.go        # [新增] 定时任务工具
│       └── skills_tool.go      # [新增] 技能搜索/安装工具
│
├── bus/                        # 消息总线
│   ├── queue.go                # Go channel 异步队列
│   ├── events.go               # 消息类型
│   └── errors.go               # 错误定义
│
├── channels/                   # 聊天通道
│   ├── base.go                 # 基础通道
│   ├── manager.go              # [增强] 通道管理器
│   ├── telegram.go             # [完成] Telegram Bot 实现
│   ├── whatsapp.go             # [完成] WhatsApp WebSocket 实现
│   ├── discord.go              # [新增] Discord Bot
│   ├── dingtalk.go             # [新增] 钉钉 Bot
│   ├── feishu.go               # [新增] 飞书 Bot
│   └── wecom.go                # [新增] 企业微信
│
├── providers/                  # LLM 提供商
│   ├── base.go                 # 接口定义
│   ├── openai.go               # OpenAI 兼容 Provider
│   ├── transcription.go        # 语音转录
│   ├── fallback.go             # [新增] FallbackChain 容错链
│   ├── cooldown.go             # [新增] CooldownTracker 冷却追踪
│   ├── errors.go               # [新增] ErrorClassifier 错误分类
│   └── factory.go              # [新增] Provider 工厂
│
├── routing/                    # [新增] 消息路由
│   └── route.go                # 7 级优先级路由解析
│
├── session/                    # 会话管理
│   ├── manager.go              # JSONL 持久化管理器
│   └── session.go              # 会话数据结构
│
├── cron/                       # 定时任务
│   ├── service.go              # [增强] 完整 cron 表达式支持
│   └── types.go                # 任务类型
│
├── heartbeat/                  # 心跳服务
│   └── service.go              # 周期心跳
│
├── health/                     # [新增] 健康检查
│   └── server.go               # HTTP /health + /ready
│
├── state/                      # [新增] 状态管理
│   └── state.go                # 原子持久化（LastChannel/ChatID）
│
├── skills/                     # [新增] 技能管理
│   ├── loader.go               # 三级加载器（workspace > global > builtin）
│   └── registry.go             # 远程技能市场
│
├── auth/                       # [新增] 认证系统
│   ├── oauth.go                # OAuth 2.0 + PKCE
│   ├── store.go                # 凭证存储
│   └── token.go                # JWT 解析
│
├── devices/                    # [新增] 设备集成
│   ├── service.go              # 设备事件服务
│   ├── source.go               # EventSource 接口
│   └── usb/                    # USB 监控
│       ├── monitor_linux.go    # Linux udevadm
│       └── monitor_stub.go     # 非 Linux 空实现
│
├── voice/                      # [新增] 语音处理
│   └── transcriber.go          # Groq Whisper 转录
│
├── goscript/                   # GoScript 解释器（保留增强）
│   ├── program.go              # 编译 + 执行
│   ├── exec_context.go         # 执行上下文
│   ├── types.go                # 类型桥接
│   └── ...                     # 其他文件
│
├── log/                        # 日志系统
│   └── log.go                  # slog + lumberjack
│
├── utils/                      # 工具函数
│   └── helpers.go              # 通用辅助
│
├── workspace/                  # [新增] 嵌入工作区模板
│   ├── AGENT.md                # Agent 指令
│   ├── IDENTITY.md             # 身份描述
│   ├── SOUL.md                 # 性格设定
│   ├── USER.md                 # 用户画像
│   ├── memory/MEMORY.md        # 记忆模板
│   └── skills/                 # 内置技能
│       ├── weather/SKILL.md
│       ├── github/SKILL.md
│       └── skill-creator/SKILL.md
│
├── go.mod
├── go.sum
├── Makefile                    # [新增] 构建脚本
├── Dockerfile                  # [新增] 容器化
└── docker-compose.yml          # [新增] 编排
```

---

## 4. 核心设计决策

### 4.1 保持极简依赖原则

**新增依赖（精选）**：

| 依赖 | 用途 | 必要性 |
|------|------|--------|
| `gorilla/websocket` | WhatsApp WebSocket + OneBot | Channel 通信必需 |
| `gronx` | Cron 表达式解析 | 轻量无依赖 cron 库 |
| 各 Channel SDK | Telegram/Discord/DingTalk/Feishu | 按需引入，可选编译 |

**拒绝引入**：
- 不引入 Cobra/Viper（保持 `os.Args` 直接路由，零依赖 CLI）
- 不引入 OAuth 库（自实现轻量 PKCE 流，<200 行）
- 不引入 ORM/数据库（保持 JSON/JSONL 文件存储）

### 4.2 adapter 模式保留

当前 `cmd/adapter.go` 的类型桥接设计是正确的，它解耦了 `bus` 和 `agent` 包的类型定义。重构中将：
- 保持 `BusAdapter` 和 `ProviderAdapter` 模式
- 新增 `RoutingAdapter`（路由层到 Agent 实例的桥接）
- 新增 `SessionAdapter`（统一 Agent 内存 Session 和 JSONL 持久化）

### 4.3 GoScript 增强方向

GoScript 是 SimpleClaw 的**核心差异化特性**，计划增强：
- 新增标准库注册：`os`、`io`、`net/http`、`encoding/json`
- 安全沙箱：限制文件系统访问范围、网络访问白名单
- 执行资源限制：内存上限、goroutine 数量上限
- Go Agent 增强：支持传入上下文变量、支持返回结构化结果

### 4.4 消息总线优化

当前 `bus` 包设计良好，仅需微调：
- 添加 `Media` 字段到 `InboundMessage`（支持图片/音频/文件）
- 添加 `Metadata` 字段（携带平台特定数据）
- 添加 `ReplyTo` 字段到 `OutboundMessage`（支持回复特定消息）
- 队列满时的策略可配置（丢弃/阻塞/报错）

---

## 5. 数据流

### 5.1 消息处理主流程

```
1. 用户发送消息到 Channel (Telegram/Discord/...)
2. Channel.HandleMessage() 构造 InboundMessage
3. MessageBus.PublishInbound(msg) → inbound channel
4. AgentLoop.Run() 消费 inbound message
5. RouteResolver.Resolve(channel, accountID, chatID) → AgentID + SessionKey
6. AgentRegistry.Get(agentID) → AgentInstance
7. SessionManager.GetOrCreate(sessionKey) → Session (历史消息)
8. ContextBuilder.BuildMessages(session, userMsg) → [system, ...history, user]
9. FallbackChain.Execute() → LLM Provider.Chat(messages, tools)
10. 有 tool_calls → ToolRegistry.Execute() → 结果反馈 → 重复步骤 9
11. 无 tool_calls → 最终回复
12. Session.AddMessages(assistant reply)
13. SessionManager.Save(session)
14. MessageBus.PublishOutbound(OutboundMessage)
15. ChannelManager.dispatch() → Channel.Send() → 用户
```

### 5.2 子 Agent 流程

```
1. 主 Agent 调用 spawn tool (task description)
2. SubagentManager.Spawn() 创建后台 goroutine
3. 子 Agent 独立 ToolRegistry（无 message/spawn，防递归）
4. 子 Agent 执行 ToolLoop（最多 15 次迭代）
5. 完成后 → MessageBus.PublishInbound(channel="system", content=result)
6. AgentLoop 接收 system message → 路由到主 Agent → 解码原始 channel:chatID
7. 主 Agent 处理结果 → 回复到原始通道
```

### 5.3 GoScript 动态执行流程

```
1. Agent 决定执行 Go 代码（go_run 工具）
2. GoRun Tool 接收源代码字符串
3. goscript.Compile(source) → AST → 类型检查 → SSA
4. goscript.RunWithContext(ctx, timeout) → 捕获输出
5. 返回执行结果（stdout + 返回值）给 Agent
6. Agent 基于结果继续推理
```
