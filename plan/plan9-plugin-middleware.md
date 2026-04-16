# Plan 9: 插件系统双机制 — 管道 + 事件

## 来源

TinyAGI — `packages/core/src/plugins.ts`

## 目标

为 SimpleClaw 的消息总线增加中间件扩展能力，支持管道模式（消息变换链）和事件观察者模式（解耦监听），让第三方功能（审计、过滤、翻译、格式化）可以非侵入式地接入。

## 现状分析

- `bus/queue.go` 的 `MessageBus` 是纯透传：Publish → channel → Consume，无拦截点
- `SkillsLoader` 只加载技能摘要进 system prompt，不参与消息处理流程
- 添加消息预/后处理只能修改 `agent/agent.go` 核心代码，耦合度高
- 没有统一的事件系统，各模块间的状态同步靠直接方法调用

## 技术方案

### 阶段一：中间件接口定义（0.5 周）

#### 1.1 管道中间件（消息变换）

```go
// bus/middleware.go (新文件)

// InboundMiddleware 在消息到达 Agent 前执行
// 返回修改后的消息；返回 nil 表示丢弃该消息
type InboundMiddleware interface {
    Name() string
    ProcessInbound(msg *InboundMessage) *InboundMessage
}

// OutboundMiddleware 在消息发给用户前执行
type OutboundMiddleware interface {
    Name() string
    ProcessOutbound(msg *OutboundMessage) *OutboundMessage
}

// MiddlewareChain 管道式执行中间件链
type MiddlewareChain struct {
    inbound  []InboundMiddleware
    outbound []OutboundMiddleware
    mu       sync.RWMutex
}

func (mc *MiddlewareChain) AddInbound(mw InboundMiddleware)
func (mc *MiddlewareChain) AddOutbound(mw OutboundMiddleware)

func (mc *MiddlewareChain) RunInbound(msg *InboundMessage) *InboundMessage {
    mc.mu.RLock()
    defer mc.mu.RUnlock()
    current := msg
    for _, mw := range mc.inbound {
        current = mw.ProcessInbound(current)
        if current == nil {
            return nil // 消息被丢弃
        }
    }
    return current
}

func (mc *MiddlewareChain) RunOutbound(msg *OutboundMessage) *OutboundMessage {
    mc.mu.RLock()
    defer mc.mu.RUnlock()
    current := msg
    for _, mw := range mc.outbound {
        current = mw.ProcessOutbound(current)
        if current == nil {
            return nil
        }
    }
    return current
}
```

### 阶段二：事件系统（1 周）

#### 2.1 事件总线

```go
// bus/events_system.go (新文件)
type EventType string
const (
    EventMessageInbound  EventType = "message:inbound"
    EventMessageOutbound EventType = "message:outbound"
    EventAgentInvoke     EventType = "agent:invoke"
    EventAgentResponse   EventType = "agent:response"
    EventToolCall        EventType = "tool:call"
    EventToolResult      EventType = "tool:result"
    EventToolDenied      EventType = "tool:denied"
    EventSessionCreated  EventType = "session:created"
    EventCronJobRun      EventType = "cron:job_run"
    EventError           EventType = "error"
)

type Event struct {
    Type      EventType              `json:"type"`
    Timestamp time.Time              `json:"timestamp"`
    Data      map[string]interface{} `json:"data"`
}

type EventHandler func(event Event)

type EventBus struct {
    handlers  map[EventType][]EventHandler
    wildcards []EventHandler // "*" 通配符监听
    mu        sync.RWMutex
}

func NewEventBus() *EventBus

func (eb *EventBus) On(eventType EventType, handler EventHandler) {
    eb.mu.Lock()
    defer eb.mu.Unlock()
    eb.handlers[eventType] = append(eb.handlers[eventType], handler)
}

func (eb *EventBus) OnAll(handler EventHandler) {
    eb.mu.Lock()
    defer eb.mu.Unlock()
    eb.wildcards = append(eb.wildcards, handler)
}

// Emit 异步触发事件，错误不中断主流程
func (eb *EventBus) Emit(eventType EventType, data map[string]interface{}) {
    event := Event{Type: eventType, Timestamp: time.Now(), Data: data}
    eb.mu.RLock()
    handlers := eb.handlers[eventType]
    wildcards := eb.wildcards
    eb.mu.RUnlock()

    for _, h := range handlers {
        func() {
            defer func() { recover() }() // 不让插件 panic 崩掉主流程
            h(event)
        }()
    }
    for _, h := range wildcards {
        func() {
            defer func() { recover() }()
            h(event)
        }()
    }
}
```

### 阶段三：集成到 MessageBus + Agent（1 周）

#### 3.1 MessageBus 嵌入中间件

```go
// bus/queue.go — 修改 PublishInbound / PublishOutbound
func (mb *MessageBus) PublishInbound(msg InboundMessage) error {
    // 运行入站中间件链
    processed := mb.middleware.RunInbound(&msg)
    if processed == nil {
        return nil // 消息被中间件丢弃
    }
    // 发射事件
    mb.events.Emit(EventMessageInbound, map[string]interface{}{
        "channel": processed.Channel, "sender": processed.SenderID,
    })
    // 入队
    mb.inbound <- *processed
    return nil
}

func (mb *MessageBus) PublishOutbound(msg OutboundMessage) error {
    processed := mb.middleware.RunOutbound(&msg)
    if processed == nil {
        return nil
    }
    mb.events.Emit(EventMessageOutbound, map[string]interface{}{
        "channel": processed.Channel, "content_length": len(processed.Content),
    })
    mb.outbound <- *processed
    return nil
}
```

#### 3.2 Agent 中发射工具事件

```go
// agent/agent.go — 工具调用前后
func (a *Agent) executeTool(ctx context.Context, name string, args map[string]interface{}) (string, error) {
    a.events.Emit(EventToolCall, map[string]interface{}{"tool": name, "args": args})

    result, err := a.tools.Execute(ctx, name, args)
    if err != nil {
        a.events.Emit(EventError, map[string]interface{}{"tool": name, "error": err.Error()})
        return "", err
    }

    a.events.Emit(EventToolResult, map[string]interface{}{"tool": name, "result_length": len(result)})
    return result, nil
}
```

### 阶段四：内置中间件示例（0.5 周）

#### 4.1 日志审计中间件

```go
// bus/middleware_audit.go (新文件)
type AuditMiddleware struct {
    logFile string
}

func (a *AuditMiddleware) Name() string { return "audit" }

func (a *AuditMiddleware) ProcessInbound(msg *InboundMessage) *InboundMessage {
    // 记录入站消息到审计日志
    log.Printf("[AUDIT] inbound: channel=%s sender=%s len=%d", msg.Channel, msg.SenderID, len(msg.Content))
    return msg // 透传不修改
}
```

#### 4.2 敏感词过滤中间件

```go
// bus/middleware_filter.go (新文件)
type ContentFilterMiddleware struct {
    blockedPatterns []*regexp.Regexp
}

func (f *ContentFilterMiddleware) Name() string { return "content_filter" }

func (f *ContentFilterMiddleware) ProcessInbound(msg *InboundMessage) *InboundMessage {
    for _, pattern := range f.blockedPatterns {
        if pattern.MatchString(msg.Content) {
            return nil // 丢弃匹配的消息
        }
    }
    return msg
}
```

#### 4.3 响应格式化中间件

```go
// bus/middleware_format.go (新文件)
type MarkdownToHTMLMiddleware struct{}

func (m *MarkdownToHTMLMiddleware) Name() string { return "md_to_html" }

func (m *MarkdownToHTMLMiddleware) ProcessOutbound(msg *OutboundMessage) *OutboundMessage {
    if msg.Channel == "telegram" {
        msg.Content = markdownToTelegramHTML(msg.Content)
    }
    return msg
}
```

## 涉及文件

| 操作 | 文件 | 说明 |
|------|------|------|
| 新建 | `bus/middleware.go` | 中间件接口 + MiddlewareChain |
| 新建 | `bus/events_system.go` | EventBus 事件系统 |
| 新建 | `bus/middleware_audit.go` | 审计中间件（示例） |
| 新建 | `bus/middleware_filter.go` | 敏感词过滤（示例） |
| 新建 | `bus/middleware_test.go` | 单元测试 |
| 修改 | `bus/queue.go` | 嵌入中间件链 + 事件发射 |
| 修改 | `agent/agent.go` | 工具调用前后发射事件 |
| 修改 | `cmd/gateway.go` | 注册内置中间件 |

## 验收标准

- [ ] InboundMiddleware 能拦截并修改入站消息
- [ ] OutboundMiddleware 能拦截并修改出站消息
- [ ] 中间件返回 nil 时消息被丢弃
- [ ] 中间件 panic 不崩溃主流程（recover 保护）
- [ ] EventBus 支持按类型订阅和通配符订阅
- [ ] 审计中间件正确记录所有消息
- [ ] 敏感词中间件正确过滤消息

## 工期估算

**总计：3 周（1 人）**

| 阶段 | 工期 |
|------|------|
| 中间件接口定义 | 0.5 周 |
| 事件系统 | 1 周 |
| 集成到 MessageBus + Agent | 1 周 |
| 内置中间件示例 | 0.5 周 |
