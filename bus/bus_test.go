package bus

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// NewMessageBus 测试
// ---------------------------------------------------------------------------

func TestNewMessageBusDefaults(t *testing.T) {
	b := NewMessageBus(0, nil)
	if b == nil {
		t.Fatal("不应返回 nil")
	}
	if b.logger == nil {
		t.Error("logger 不应为 nil")
	}
	// bufSize <= 0 应回退到 256
	if cap(b.inbound) != 256 {
		t.Errorf("期望 inbound 容量 256，实际 %d", cap(b.inbound))
	}
	if cap(b.outbound) != 256 {
		t.Errorf("期望 outbound 容量 256，实际 %d", cap(b.outbound))
	}
}

func TestNewMessageBusNegativeBufSize(t *testing.T) {
	b := NewMessageBus(-10, nil)
	if cap(b.inbound) != 256 {
		t.Errorf("负数 bufSize 应回退到 256，实际 %d", cap(b.inbound))
	}
}

func TestNewMessageBusCustomBufSize(t *testing.T) {
	b := NewMessageBus(32, nil)
	if cap(b.inbound) != 32 {
		t.Errorf("期望 inbound 容量 32，实际 %d", cap(b.inbound))
	}
	if cap(b.outbound) != 32 {
		t.Errorf("期望 outbound 容量 32，实际 %d", cap(b.outbound))
	}
}

// ---------------------------------------------------------------------------
// InboundMessage 测试
// ---------------------------------------------------------------------------

func TestSessionKey(t *testing.T) {
	msg := InboundMessage{Channel: "telegram", ChatID: "12345"}
	if key := msg.SessionKey(); key != "telegram:12345" {
		t.Errorf("期望 'telegram:12345'，实际 %q", key)
	}
}

func TestSessionKeyEmptyFields(t *testing.T) {
	msg := InboundMessage{}
	if key := msg.SessionKey(); key != ":" {
		t.Errorf("期望 ':'，实际 %q", key)
	}
}

// ---------------------------------------------------------------------------
// ErrQueueFull 测试
// ---------------------------------------------------------------------------

func TestErrQueueFull(t *testing.T) {
	if ErrQueueFull == nil {
		t.Fatal("ErrQueueFull 不应为 nil")
	}
	if ErrQueueFull.Error() != "bus: queue full" {
		t.Errorf("期望 'bus: queue full'，实际 %q", ErrQueueFull.Error())
	}
}

// ---------------------------------------------------------------------------
// PublishInbound / ConsumeInbound 测试
// ---------------------------------------------------------------------------

func TestPublishAndConsumeInbound(t *testing.T) {
	b := NewMessageBus(8, nil)
	msg := InboundMessage{
		Channel:  "telegram",
		SenderID: "user1",
		ChatID:   "chat1",
		Content:  "hello",
	}

	if err := b.PublishInbound(msg); err != nil {
		t.Fatalf("PublishInbound 失败: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	got, err := b.ConsumeInbound(ctx)
	if err != nil {
		t.Fatalf("ConsumeInbound 失败: %v", err)
	}
	if got.Channel != "telegram" {
		t.Errorf("期望 channel=telegram，实际 %q", got.Channel)
	}
	if got.Content != "hello" {
		t.Errorf("期望 content=hello，实际 %q", got.Content)
	}
	if got.SenderID != "user1" {
		t.Errorf("期望 senderID=user1，实际 %q", got.SenderID)
	}
}

func TestPublishInboundQueueFull(t *testing.T) {
	b := NewMessageBus(1, nil)
	msg := InboundMessage{Channel: "test", ChatID: "1"}

	// 第一条应成功
	if err := b.PublishInbound(msg); err != nil {
		t.Fatalf("第一条消息不应失败: %v", err)
	}
	// 第二条应返回 ErrQueueFull
	if err := b.PublishInbound(msg); !errors.Is(err, ErrQueueFull) {
		t.Errorf("期望 ErrQueueFull，实际 %v", err)
	}
}

func TestConsumeInboundContextCancel(t *testing.T) {
	b := NewMessageBus(8, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, err := b.ConsumeInbound(ctx)
	if err == nil {
		t.Error("context 取消后应返回错误")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("期望 context.Canceled，实际 %v", err)
	}
}

func TestConsumeInboundTimeout(t *testing.T) {
	b := NewMessageBus(8, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := b.ConsumeInbound(ctx)
	if err == nil {
		t.Error("超时后应返回错误")
	}
}

// ---------------------------------------------------------------------------
// PublishOutbound / ConsumeOutbound 测试
// ---------------------------------------------------------------------------

func TestPublishAndConsumeOutbound(t *testing.T) {
	b := NewMessageBus(8, nil)
	msg := OutboundMessage{
		Channel: "whatsapp",
		ChatID:  "chat2",
		Content: "response",
		ReplyTo: "msg-001",
	}

	if err := b.PublishOutbound(msg); err != nil {
		t.Fatalf("PublishOutbound 失败: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	got, err := b.ConsumeOutbound(ctx)
	if err != nil {
		t.Fatalf("ConsumeOutbound 失败: %v", err)
	}
	if got.Channel != "whatsapp" {
		t.Errorf("期望 channel=whatsapp，实际 %q", got.Channel)
	}
	if got.Content != "response" {
		t.Errorf("期望 content=response，实际 %q", got.Content)
	}
	if got.ReplyTo != "msg-001" {
		t.Errorf("期望 replyTo=msg-001，实际 %q", got.ReplyTo)
	}
}

func TestPublishOutboundQueueFull(t *testing.T) {
	b := NewMessageBus(1, nil)
	msg := OutboundMessage{Channel: "test", ChatID: "1"}

	if err := b.PublishOutbound(msg); err != nil {
		t.Fatalf("第一条消息不应失败: %v", err)
	}
	if err := b.PublishOutbound(msg); !errors.Is(err, ErrQueueFull) {
		t.Errorf("期望 ErrQueueFull，实际 %v", err)
	}
}

func TestConsumeOutboundContextCancel(t *testing.T) {
	b := NewMessageBus(8, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := b.ConsumeOutbound(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("期望 context.Canceled，实际 %v", err)
	}
}

// ---------------------------------------------------------------------------
// InboundSize / OutboundSize 测试
// ---------------------------------------------------------------------------

func TestInboundSize(t *testing.T) {
	b := NewMessageBus(8, nil)
	if b.InboundSize() != 0 {
		t.Errorf("初始应为 0，实际 %d", b.InboundSize())
	}
	b.PublishInbound(InboundMessage{Channel: "test"})
	if b.InboundSize() != 1 {
		t.Errorf("发布 1 条后应为 1，实际 %d", b.InboundSize())
	}
	b.PublishInbound(InboundMessage{Channel: "test"})
	if b.InboundSize() != 2 {
		t.Errorf("发布 2 条后应为 2，实际 %d", b.InboundSize())
	}
}

func TestOutboundSize(t *testing.T) {
	b := NewMessageBus(8, nil)
	if b.OutboundSize() != 0 {
		t.Errorf("初始应为 0，实际 %d", b.OutboundSize())
	}
	b.PublishOutbound(OutboundMessage{Channel: "test"})
	if b.OutboundSize() != 1 {
		t.Errorf("发布 1 条后应为 1，实际 %d", b.OutboundSize())
	}
}

func TestSizeAfterConsume(t *testing.T) {
	b := NewMessageBus(8, nil)
	b.PublishInbound(InboundMessage{Channel: "test"})
	if b.InboundSize() != 1 {
		t.Fatal("发布后应为 1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	b.ConsumeInbound(ctx)

	if b.InboundSize() != 0 {
		t.Errorf("消费后应为 0，实际 %d", b.InboundSize())
	}
}

// ---------------------------------------------------------------------------
// Stop 测试
// ---------------------------------------------------------------------------

func TestStop(t *testing.T) {
	b := NewMessageBus(8, nil)
	// Stop 不应阻塞或 panic
	b.Stop()
}

// ---------------------------------------------------------------------------
// 消息字段完整性测试
// ---------------------------------------------------------------------------

func TestInboundMessageFields(t *testing.T) {
	now := time.Now()
	msg := InboundMessage{
		Channel:   "telegram",
		SenderID:  "user1",
		ChatID:    "chat1",
		Content:   "hello",
		Timestamp: now,
		Media:     []MediaItem{{Type: "image", URL: "https://example.com/img.png"}},
		Metadata:  map[string]string{"key": "value"},
	}

	if msg.Channel != "telegram" {
		t.Error("Channel 不匹配")
	}
	if msg.SenderID != "user1" {
		t.Error("SenderID 不匹配")
	}
	if msg.ChatID != "chat1" {
		t.Error("ChatID 不匹配")
	}
	if msg.Content != "hello" {
		t.Error("Content 不匹配")
	}
	if !msg.Timestamp.Equal(now) {
		t.Error("Timestamp 不匹配")
	}
	if len(msg.Media) != 1 || msg.Media[0].URL != "https://example.com/img.png" {
		t.Error("Media 不匹配")
	}
	if msg.Metadata["key"] != "value" {
		t.Error("Metadata 不匹配")
	}
}

func TestOutboundMessageFields(t *testing.T) {
	msg := OutboundMessage{
		Channel:  "whatsapp",
		ChatID:   "chat2",
		Content:  "response",
		ReplyTo:  "msg-001",
		Media:    []MediaItem{{Type: "file", URL: "file.pdf"}},
		Metadata: map[string]string{"type": "reply"},
	}

	if msg.Channel != "whatsapp" {
		t.Error("Channel 不匹配")
	}
	if msg.ReplyTo != "msg-001" {
		t.Error("ReplyTo 不匹配")
	}
	if msg.Metadata["type"] != "reply" {
		t.Error("Metadata 不匹配")
	}
}

// ---------------------------------------------------------------------------
// 并发安全测试
// ---------------------------------------------------------------------------

func TestConcurrentPublishInbound(t *testing.T) {
	b := NewMessageBus(1000, nil)
	var wg sync.WaitGroup
	n := 100

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			b.PublishInbound(InboundMessage{
				Channel: "test",
				ChatID:  "chat",
				Content: "msg",
			})
		}(i)
	}
	wg.Wait()

	if size := b.InboundSize(); size != n {
		t.Errorf("期望入站队列大小 %d，实际 %d", n, size)
	}
}

func TestConcurrentPublishOutbound(t *testing.T) {
	b := NewMessageBus(1000, nil)
	var wg sync.WaitGroup
	n := 100

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.PublishOutbound(OutboundMessage{Channel: "test", Content: "msg"})
		}()
	}
	wg.Wait()

	if size := b.OutboundSize(); size != n {
		t.Errorf("期望出站队列大小 %d，实际 %d", n, size)
	}
}

func TestConcurrentPublishAndConsume(t *testing.T) {
	b := NewMessageBus(256, nil)
	n := 100
	var consumed int32

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 消费者
	go func() {
		for {
			_, err := b.ConsumeInbound(ctx)
			if err != nil {
				return
			}
			atomic.AddInt32(&consumed, 1)
		}
	}()

	// 生产者
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.PublishInbound(InboundMessage{Channel: "test", Content: "msg"})
		}()
	}
	wg.Wait()

	// 等待消费完
	time.Sleep(100 * time.Millisecond)
	cancel()

	if c := atomic.LoadInt32(&consumed); c != int32(n) {
		t.Errorf("期望消费 %d 条，实际 %d", n, c)
	}
}

// ---------------------------------------------------------------------------
// FIFO 顺序测试
// ---------------------------------------------------------------------------

func TestInboundFIFO(t *testing.T) {
	b := NewMessageBus(16, nil)
	for i := 0; i < 5; i++ {
		b.PublishInbound(InboundMessage{Content: string(rune('a' + i))})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	for i := 0; i < 5; i++ {
		msg, err := b.ConsumeInbound(ctx)
		if err != nil {
			t.Fatalf("消费第 %d 条失败: %v", i, err)
		}
		expected := string(rune('a' + i))
		if msg.Content != expected {
			t.Errorf("第 %d 条期望 %q，实际 %q", i, expected, msg.Content)
		}
	}
}

func TestOutboundFIFO(t *testing.T) {
	b := NewMessageBus(16, nil)
	for i := 0; i < 5; i++ {
		b.PublishOutbound(OutboundMessage{Content: string(rune('a' + i))})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	for i := 0; i < 5; i++ {
		msg, err := b.ConsumeOutbound(ctx)
		if err != nil {
			t.Fatalf("消费第 %d 条失败: %v", i, err)
		}
		expected := string(rune('a' + i))
		if msg.Content != expected {
			t.Errorf("第 %d 条期望 %q，实际 %q", i, expected, msg.Content)
		}
	}
}
