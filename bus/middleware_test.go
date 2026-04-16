package bus

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// MiddlewareChain 测试
// ---------------------------------------------------------------------------

type passthrough struct{ name string }

func (p *passthrough) Name() string { return p.name }
func (p *passthrough) ProcessInbound(msg *InboundMessage) *InboundMessage {
	return msg
}
func (p *passthrough) ProcessOutbound(msg *OutboundMessage) *OutboundMessage {
	return msg
}

type dropper struct{}

func (d *dropper) Name() string                                          { return "dropper" }
func (d *dropper) ProcessInbound(msg *InboundMessage) *InboundMessage    { return nil }
func (d *dropper) ProcessOutbound(msg *OutboundMessage) *OutboundMessage { return nil }

type modifier struct{}

func (m *modifier) Name() string { return "modifier" }
func (m *modifier) ProcessInbound(msg *InboundMessage) *InboundMessage {
	msg.Content = msg.Content + " [modified]"
	return msg
}

func TestMiddlewareChainPassthrough(t *testing.T) {
	mc := NewMiddlewareChain()
	mc.AddInbound(&passthrough{name: "p1"})
	mc.AddInbound(&passthrough{name: "p2"})

	msg := &InboundMessage{Content: "hello"}
	result := mc.RunInbound(msg)
	if result == nil {
		t.Fatal("透传中间件不应丢弃消息")
	}
	if result.Content != "hello" {
		t.Errorf("内容不应改变，实际 %q", result.Content)
	}
}

func TestMiddlewareChainDrop(t *testing.T) {
	mc := NewMiddlewareChain()
	mc.AddInbound(&passthrough{name: "p1"})
	mc.AddInbound(&dropper{})
	mc.AddInbound(&passthrough{name: "p3"})

	msg := &InboundMessage{Content: "hello"}
	result := mc.RunInbound(msg)
	if result != nil {
		t.Error("dropper 后应返回 nil")
	}
}

func TestMiddlewareChainModify(t *testing.T) {
	mc := NewMiddlewareChain()
	mc.AddInbound(&modifier{})

	msg := &InboundMessage{Content: "hello"}
	result := mc.RunInbound(msg)
	if result.Content != "hello [modified]" {
		t.Errorf("期望 'hello [modified]'，实际 %q", result.Content)
	}
}

func TestMiddlewareChainOutbound(t *testing.T) {
	mc := NewMiddlewareChain()
	mc.AddOutbound(&passthrough{name: "p1"})

	msg := &OutboundMessage{Content: "reply"}
	result := mc.RunOutbound(msg)
	if result == nil || result.Content != "reply" {
		t.Error("出站透传异常")
	}
}

func TestMiddlewareChainOutboundDrop(t *testing.T) {
	mc := NewMiddlewareChain()
	mc.AddOutbound(&dropper{})

	msg := &OutboundMessage{Content: "reply"}
	result := mc.RunOutbound(msg)
	if result != nil {
		t.Error("出站 dropper 应返回 nil")
	}
}

func TestMiddlewareChainEmpty(t *testing.T) {
	mc := NewMiddlewareChain()
	msg := &InboundMessage{Content: "hello"}
	result := mc.RunInbound(msg)
	if result == nil || result.Content != "hello" {
		t.Error("空链应透传")
	}
}

func TestMiddlewareChainCount(t *testing.T) {
	mc := NewMiddlewareChain()
	mc.AddInbound(&passthrough{name: "a"})
	mc.AddInbound(&passthrough{name: "b"})
	mc.AddOutbound(&passthrough{name: "c"})

	if mc.InboundCount() != 2 {
		t.Errorf("入站数应为 2，实际 %d", mc.InboundCount())
	}
	if mc.OutboundCount() != 1 {
		t.Errorf("出站数应为 1，实际 %d", mc.OutboundCount())
	}
}

// ---------------------------------------------------------------------------
// EventBus 测试
// ---------------------------------------------------------------------------

func TestEventBusOn(t *testing.T) {
	eb := NewEventBus()
	var called int32
	eb.On(EventToolCall, func(e Event) {
		atomic.AddInt32(&called, 1)
	})
	eb.Emit(EventToolCall, nil)
	if atomic.LoadInt32(&called) != 1 {
		t.Error("处理器应被调用一次")
	}
}

func TestEventBusOnAll(t *testing.T) {
	eb := NewEventBus()
	var count int32
	eb.OnAll(func(e Event) {
		atomic.AddInt32(&count, 1)
	})
	eb.Emit(EventToolCall, nil)
	eb.Emit(EventError, nil)
	eb.Emit(EventMessageInbound, nil)
	if atomic.LoadInt32(&count) != 3 {
		t.Errorf("通配符应被调用 3 次，实际 %d", atomic.LoadInt32(&count))
	}
}

func TestEventBusPanicRecovery(t *testing.T) {
	eb := NewEventBus()
	eb.On(EventToolCall, func(e Event) {
		panic("intentional panic")
	})
	var afterPanic int32
	eb.On(EventToolCall, func(e Event) {
		atomic.AddInt32(&afterPanic, 1)
	})

	// 不应 panic
	eb.Emit(EventToolCall, nil)
	if atomic.LoadInt32(&afterPanic) != 1 {
		t.Error("panic 后的处理器仍应被调用")
	}
}

func TestEventBusNoHandler(t *testing.T) {
	eb := NewEventBus()
	// 无处理器不应 panic
	eb.Emit(EventToolCall, map[string]interface{}{"key": "value"})
}

func TestEventBusHandlerCount(t *testing.T) {
	eb := NewEventBus()
	eb.On(EventToolCall, func(e Event) {})
	eb.On(EventToolCall, func(e Event) {})
	eb.On(EventError, func(e Event) {})
	eb.OnAll(func(e Event) {})

	if eb.HandlerCount(EventToolCall) != 2 {
		t.Errorf("ToolCall 处理器数应为 2，实际 %d", eb.HandlerCount(EventToolCall))
	}
	if eb.HandlerCount(EventError) != 1 {
		t.Errorf("Error 处理器数应为 1，实际 %d", eb.HandlerCount(EventError))
	}
	if eb.WildcardCount() != 1 {
		t.Errorf("通配符数应为 1，实际 %d", eb.WildcardCount())
	}
}

func TestEventBusData(t *testing.T) {
	eb := NewEventBus()
	var received Event
	eb.On(EventToolCall, func(e Event) {
		received = e
	})
	eb.Emit(EventToolCall, map[string]interface{}{"tool": "exec"})

	if received.Type != EventToolCall {
		t.Errorf("事件类型应为 tool:call，实际 %q", received.Type)
	}
	if received.Data["tool"] != "exec" {
		t.Errorf("Data[tool] 应为 exec，实际 %v", received.Data["tool"])
	}
}

// ---------------------------------------------------------------------------
// MessageBus + Middleware 集成测试
// ---------------------------------------------------------------------------

func TestMessageBusMiddlewareInbound(t *testing.T) {
	mb := NewMessageBus(10, nil)
	mb.Middleware().AddInbound(&modifier{})

	mb.PublishInbound(InboundMessage{Content: "test"})

	msg, _ := mb.ConsumeInbound(mwTestCtx())
	if msg.Content != "test [modified]" {
		t.Errorf("中间件应修改内容，实际 %q", msg.Content)
	}
}

func TestMessageBusMiddlewareDropInbound(t *testing.T) {
	mb := NewMessageBus(10, nil)
	mb.Middleware().AddInbound(&dropper{})

	err := mb.PublishInbound(InboundMessage{Content: "test"})
	if err != nil {
		t.Errorf("丢弃消息不应报错: %v", err)
	}
	if mb.InboundSize() != 0 {
		t.Error("被丢弃的消息不应入队")
	}
}

func TestMessageBusMiddlewareDropOutbound(t *testing.T) {
	mb := NewMessageBus(10, nil)
	mb.Middleware().AddOutbound(&dropper{})

	err := mb.PublishOutbound(OutboundMessage{Content: "test"})
	if err != nil {
		t.Errorf("丢弃消息不应报错: %v", err)
	}
	if mb.OutboundSize() != 0 {
		t.Error("被丢弃的消息不应入队")
	}
}

func TestMessageBusEvents(t *testing.T) {
	mb := NewMessageBus(10, nil)
	var inboundCount, outboundCount int32

	mb.Events().On(EventMessageInbound, func(e Event) {
		atomic.AddInt32(&inboundCount, 1)
	})
	mb.Events().On(EventMessageOutbound, func(e Event) {
		atomic.AddInt32(&outboundCount, 1)
	})

	mb.PublishInbound(InboundMessage{Content: "in"})
	mb.PublishOutbound(OutboundMessage{Content: "out"})

	if atomic.LoadInt32(&inboundCount) != 1 {
		t.Error("入站事件应触发一次")
	}
	if atomic.LoadInt32(&outboundCount) != 1 {
		t.Error("出站事件应触发一次")
	}
}

func mwTestCtx() context.Context {
	ctx, _ := context.WithTimeout(context.Background(), time.Second)
	return ctx
}

// ---------------------------------------------------------------------------
// ContentFilterMiddleware 测试
// ---------------------------------------------------------------------------

func TestContentFilterMiddleware(t *testing.T) {
	filter := NewContentFilterMiddleware([]string{`(?i)spam`, `badword`})

	msg1 := &InboundMessage{Content: "hello world"}
	if filter.ProcessInbound(msg1) == nil {
		t.Error("正常消息不应被过滤")
	}

	msg2 := &InboundMessage{Content: "This is SPAM message"}
	if filter.ProcessInbound(msg2) != nil {
		t.Error("包含 spam 的消息应被过滤")
	}

	msg3 := &InboundMessage{Content: "contains badword here"}
	if filter.ProcessInbound(msg3) != nil {
		t.Error("包含 badword 的消息应被过滤")
	}
}
