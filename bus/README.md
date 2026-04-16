# Bus - 消息总线

异步消息队列，解耦聊天渠道（Telegram、WhatsApp、CLI 等）与 Agent 核心。渠道将消息推入入站队列，Agent 处理后将响应推入出站队列，通过订阅者模式按渠道分发。

## 架构

```
Channel A ─┐                          ┌─→ Handler A (Telegram)
Channel B ─┼─→ [入站队列] → Agent → [出站队列] → DispatchOutbound ─┼─→ Handler B (WhatsApp)
Channel C ─┘                          └─→ Handler C (CLI)
```

## 目录结构

```
bus/
├── events.go      消息类型定义（InboundMessage、OutboundMessage、MediaItem）
├── queue.go       消息总线核心实现（队列、订阅、分发）
├── errors.go      错误定义（ErrQueueFull）
└── bus_test.go    单元测试（28 个测试用例，含并发安全和 FIFO 顺序）
```

## 消息类型

### InboundMessage - 入站消息

从聊天渠道接收到的消息，包含来源渠道、发送者、聊天 ID、文本内容、媒体附件和元数据。

```go
InboundMessage{
    Channel:   "telegram",
    SenderID:  "user1",
    ChatID:    "12345",
    Content:   "你好",
    Timestamp: time.Now(),
    Media:     []MediaItem{{Type: "image", URL: "https://..."}},
    Metadata:  map[string]string{"lang": "zh"},
}
```

`SessionKey()` 方法返回 `channel:chatID` 格式的唯一会话键。

### OutboundMessage - 出站消息

Agent 要发送到聊天渠道的响应，支持回复引用和媒体附件。

```go
OutboundMessage{
    Channel: "telegram",
    ChatID:  "12345",
    Content: "你好！有什么需要帮助的？",
    ReplyTo: "msg-001",
}
```

### MediaItem - 媒体附件

```go
MediaItem{
    Type:     "image",    // "image" | "audio" | "video" | "file"
    URL:      "https://example.com/img.png",
    Name:     "photo.png",
    MimeType: "image/png",
}
```

## 核心 API

### 创建

```go
bus := bus.NewMessageBus(256, logger) // bufSize<=0 时默认 256
```

### 发布 / 消费

```go
// 渠道 → Agent
bus.PublishInbound(msg)                    // 非阻塞，队列满时返回 ErrQueueFull
msg, err := bus.ConsumeInbound(ctx)       // 阻塞，直到有消息或 ctx 取消

// Agent → 渠道
bus.PublishOutbound(msg)                   // 非阻塞，队列满时返回 ErrQueueFull
msg, err := bus.ConsumeOutbound(ctx)      // 阻塞，直到有消息或 ctx 取消
```

### 订阅 / 分发

```go
// 注册渠道处理器（支持同一渠道多个 handler）
bus.SubscribeOutbound("telegram", func(msg OutboundMessage) error {
    return sendToTelegram(msg)
})

// 启动分发循环（阻塞，从出站队列消费并分发给对应渠道的订阅者）
go bus.DispatchOutbound(ctx)

// 停止分发
bus.Stop()
```

### 队列监控

```go
bus.InboundSize()   // 入站队列待处理消息数
bus.OutboundSize()  // 出站队列待处理消息数
```

## 设计特点

- **非阻塞发布**：队列满时立即返回 `ErrQueueFull`，不会阻塞生产者
- **FIFO 顺序**：基于 Go channel，保证消息严格先进先出
- **并发安全**：订阅者列表使用 `sync.RWMutex` 保护，队列操作基于 channel 天然安全
- **优雅停止**：支持 `context.Cancel` 和 `Stop()` 两种方式终止分发循环
- **错误隔离**：单个 handler 报错不影响其他 handler 执行，错误通过 logger 记录

## 插件中间件 (`middleware.go`)

`MiddlewareChain` 为工具调用提供可插拔的前/后处理管道：

```
ToolCall → Middleware[0].Before → Middleware[1].Before → ... → Execute → Middleware[N].After → ... → Result
```

```go
type Middleware interface {
    Name() string
    Before(ctx *MiddlewareContext) error    // 返回 error 则中止调用链
    After(ctx *MiddlewareContext) error
}
```

- 支持全局中间件和按工具名过滤
- FIFO 顺序执行 Before，LIFO 顺序执行 After
- 中间件通过 `MiddlewareContext.Set()/Get()` 传递上下文数据

### 内置中间件

| 中间件 | 说明 |
|--------|------|
| `AuditMiddleware` | 审计日志，记录所有工具调用（名称、参数、耗时、结果） |
| `FilterMiddleware` | 敏感词过滤，在工具返回结果中替换指定关键词 |

## 事件系统 (`events.go`)

`EventBus` 提供全局事件发布/订阅机制：

```go
eb := bus.NewEventBus()
eb.Subscribe(bus.EventToolCallStart, func(e bus.Event) {
    log.Printf("Tool %s called", e.Data["tool"])
})
eb.Publish(bus.EventToolCallStart, map[string]interface{}{"tool": "exec"})
```

### 10 种事件类型

| 事件 | 触发时机 |
|------|---------|
| `message.inbound` | 收到入站消息 |
| `message.outbound` | 发送出站消息 |
| `tool.call.start` | 工具调用开始 |
| `tool.call.end` | 工具调用完成 |
| `tool.call.error` | 工具调用出错 |
| `agent.loop.start` | ReAct 循环开始 |
| `agent.loop.end` | ReAct 循环结束 |
| `llm.request` | LLM 请求发送 |
| `llm.response` | LLM 响应接收 |
| `system.error` | 系统级错误 |

- 异步分发（独立 goroutine），不阻塞发布者
- 支持按事件类型订阅
- 通过 `Close()` 优雅停止
