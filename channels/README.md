# channels

聊天渠道管理包，提供统一的多渠道消息收发抽象和渠道生命周期管理。

## 架构

```
            ┌─────────────────────────────────────────┐
            │            Manager (渠道管理器)            │
            │                                         │
            │  initChannels()    根据配置初始化渠道      │
            │  StartAll()        启动所有渠道 + 出站分发  │
            │  StopAll()         停止所有渠道            │
            │  dispatchOutbound  消费出站队列 → 路由到渠道 │
            └────────────┬───────────┬────────────────┘
                         │           │
              ┌──────────▼──┐   ┌───▼──────────┐
              │  Telegram   │   │   WhatsApp   │
              │  Channel    │   │   Channel    │
              │             │   │              │
              │ Long Polling│   │ HTTP Polling │
              │ Bot API     │   │ via Bridge   │
              └──────┬──────┘   └──────┬───────┘
                     │                 │
                     ▼                 ▼
              Telegram API      Node.js Bridge
                               (baileys)
```

## 目录结构

```
channels/
├── base.go             # Channel 接口定义 + BaseChannel 通用实现
├── manager.go          # Manager 渠道管理器（初始化、启停、出站分发）
├── telegram.go         # Telegram Bot 渠道（Long Polling + Markdown→HTML）
├── whatsapp.go         # WhatsApp 渠道（通过 WebSocket/HTTP Bridge）
├── pairing.go          # Sender 配对认证（配对码生成/审批/撤销、原子文件写入）
└── channels_test.go    # 测试
```

## 核心接口

```go
// Channel 是所有聊天渠道必须实现的接口
type Channel interface {
    Name() string
    Start(ctx context.Context) error
    Stop() error
    Send(msg bus.OutboundMessage) error
    IsRunning() bool
}
```

## BaseChannel

所有渠道的公共基类，提供：

- **名称管理** — `Name()` 返回渠道标识
- **运行状态** — `IsRunning()` 原子读取运行标志
- **白名单过滤** — `IsAllowed(senderID)` 支持管道分隔的复合 ID
- **消息转发** — `HandleMessage(msg)` 发布到消息总线入站队列

## 渠道实现

### Telegram

| 特性 | 说明 |
|------|------|
| 接收方式 | Long Polling（`getUpdates`，30s 超时） |
| 发送格式 | Markdown → Telegram HTML 自动转换 |
| 消息分片 | 超过 4096 字符自动在换行处分片 |
| 降级处理 | HTML 解析失败时回退为纯文本 |
| 白名单 | 基于 Telegram user ID 过滤 |

### WhatsApp

| 特性 | 说明 |
|------|------|
| 接收方式 | 通过 Node.js Bridge 的 HTTP Polling |
| Bridge 协议 | JSON 消息（message/qr/status/error） |
| 自动重连 | 断开后 5 秒自动重连 |
| QR 认证 | 支持 Bridge 推送的 QR 码认证事件 |

## Manager API

| 方法 | 说明 |
|------|------|
| `NewManager(cfg, bus, logger)` | 创建管理器，根据配置自动初始化已启用渠道 |
| `Register(ch)` | 注册自定义渠道 |
| `StartAll(ctx)` | 启动所有渠道和出站消息分发器 |
| `StopAll()` | 停止所有渠道 |
| `GetChannel(name)` | 按名称获取渠道 |
| `EnabledChannels()` | 返回已启用渠道名称列表 |
| `GetStatus()` | 返回所有渠道的启用/运行状态 |

## 消息流

```
入站: Telegram/WhatsApp API → Channel.pollLoop → BaseChannel.HandleMessage → bus.PublishInbound
出站: bus.ConsumeOutbound → Manager.dispatchOutbound → Channel.Send → Telegram/WhatsApp API
```
对码 → 新 Sender 提交配对码 → Owner 审批 → 绑定生效
```

### 核心功能

| 方法 | 说明 |
|------|------|
| `GeneratePairingCode(ownerID)` | 生成 6 位配对码（10 分钟有效） |
| `SubmitPairing(code, senderID, channel)` | 新 Sender 提交配对请求 |
| `ApprovePairing(requestID)` | Owner 审批通过 |
| `RevokePairing(senderID)` | 撤销已绑定的 Sender |
| `IsPaired(senderID)` | 检查 Sender 是否已配对 |

### 存储

- JSON 文件持久化（`~/.simpleclaw/pairing.json`）
- 原子写入（temp file + rename），防止中断导致数据损坏
- 过期配对码自动清理
