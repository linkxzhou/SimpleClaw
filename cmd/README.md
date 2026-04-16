# cmd

SimpleClaw 的 CLI 入口包，使用纯 `os.Args` 实现命令行解析，不依赖第三方 CLI 库。

## 目录结构

```
cmd/
├── main.go        # 程序入口，子命令分派
├── onboard.go     # onboard 子命令：初始化配置和工作区
├── gateway.go     # gateway 子命令：启动完整网关服务
├── agent.go       # agent 子命令：单消息模式或交互式 REPL
├── cron.go        # cron 子命令：定时任务管理（list/add/remove/enable/run）
├── sop.go         # sop 子命令：SOP 标准操作流程管理（list/run）
├── pairing.go     # pairing 子命令：Sender 配对认证（generate/approve/revoke/list）
├── status.go      # status/channels 子命令：显示系统状态
└── adapter.go     # 类型适配器：桥接 bus/providers/session 与 agent 内部类型
```

## 子命令

| 命令 | 说明 | 示例 |
|------|------|------|
| `onboard` | 初始化配置文件和工作区模板 | `simpleclaw onboard` |
| `gateway` | 启动完整网关（Agent + Channels + Cron + Heartbeat + Health） | `simpleclaw gateway` |
| `agent` | 与 Agent 交互 | `simpleclaw agent -m "Hello!"` |
| `cron` | 管理定时任务 | `simpleclaw cron list` |
| `channels` | 管理聊天渠道 | `simpleclaw channels status` |
| `status` | 显示系统状态 | `simpleclaw status` |

## Gateway 架构

`gateway` 是 SimpleClaw 的核心运行模式，启动并协调所有服务组件：

```
simpleclaw gateway
    │
    ├── Config        加载 ~/.simpleclaw/config.json
    ├── MessageBus    创建消息总线（容量 256）
    ├── Provider      创建 LLM Provider 容错链
    ├── Agent         创建 AI Agent（消费入站 → 调用 LLM → 发布出站）
    ├── Cron          定时任务调度（任务触发 → Agent 处理 → 可选投递）
    ├── Heartbeat     心跳服务（周期性唤醒 Agent）
    ├── Channels      渠道管理器（Telegram/WhatsApp 收发）
    └── Health        HTTP 健康检查端点
```

## 适配器（adapter.go）

由于 `agent` 包定义了自己的接口类型，而 `bus`/`providers`/`session` 包有各自独立的类型，
`adapter.go` 提供以下适配器桥接二者：

| 适配器 | 接口 | 作用 |
|--------|------|------|
| `BusAdapter` | `agent.MessageBus` | 将 `*bus.MessageBus` 适配为 Agent 消息总线 |
| `ProviderAdapter` | `agent.LLMProvider` | 将单个 `*providers.OpenAIProvider` 适配为 Agent LLM |
| `FallbackProviderAdapter` | `agent.LLMProvider` | 将 `*providers.FallbackChain` 适配为带容错的 Agent LLM |
| `SessionStoreAdapter` | `agent.SessionStore` | 将 `*session.Manager` 适配为 Agent 会话存储 |

## Agent 交互模式

```bash
# 单消息模式
simpleclaw agent -m "What's the weather?" -s "session1"

# 交互式 REPL
simpleclaw agent
You: Hello!
🤖 Hi there! How can I help you?
You: exit
Goodbye!
```

## Cron 管理

```bash
simpleclaw cron list                                              # 列出任务
simpleclaw cron add -n "report" -m "Generate report" --every 3600 # 添加周期任务
simpleclaw cron add -n "alarm" -m "Wake up" --cron "0 9 * * *"   # 添加 cron 任务
simpleclaw cron remove <job_id>                                   # 删除任务
simpleclaw cron enable <job_id> [--disable]                       # 启用/禁用
simpleclaw cron run <job_id> [-f]                                 # 手动执行
```

## SOP 管理

```bash
simpleclaw sop list                           # 列出所有 SOP
simpleclaw sop run <name>                     # 执行 SOP（默认 auto 模式）
simpleclaw sop run <name> --step-by-step      # 逐步确认模式
```

## Sender 配对认证

```bash
simpleclaw pairing generate                   # 生成 6 位配对码（10 分钟有效）
simpleclaw pairing approve <request_id>       # 审批配对请求
simpleclaw pairing revoke <sender_id>         # 撤销已绑定的 Sender
simpleclaw pairing list                       # 列出所有配对状态
```
