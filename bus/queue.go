// 消息总线核心实现：异步消息队列，解耦聊天渠道与 Agent 核心。
// 渠道将消息推入入站队列，Agent 处理后将响应推入出站队列。
// 支持订阅者模式，可注册出站消息处理器按渠道分发。

package bus

import (
	"context"
	"log/slog"
)

// MessageBus 是异步消息队列，解耦聊天渠道与 Agent 核心。
type MessageBus struct {
	inbound  chan InboundMessage  // 入站消息队列
	outbound chan OutboundMessage // 出站消息队列

	middleware *MiddlewareChain // 中间件链
	events     *EventBus        // 事件总线

	done   chan struct{} // 停止信号
	logger *slog.Logger // 日志记录器
}

// NewMessageBus 创建一个新的消息总线。
// bufSize 指定队列缓冲大小，<= 0 时默认为 256。
func NewMessageBus(bufSize int, logger *slog.Logger) *MessageBus {
	if bufSize <= 0 {
		bufSize = 256
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &MessageBus{
		inbound:    make(chan InboundMessage, bufSize),
		outbound:   make(chan OutboundMessage, bufSize),
		middleware:  NewMiddlewareChain(),
		events:     NewEventBus(),
		done:       make(chan struct{}),
		logger:     logger,
	}
}

// Middleware 返回中间件链（用于外部注册中间件）。
func (b *MessageBus) Middleware() *MiddlewareChain { return b.middleware }

// Events 返回事件总线（用于外部注册事件处理器）。
func (b *MessageBus) Events() *EventBus { return b.events }

// PublishInbound 将来自渠道的消息发布到入站队列。
// 先经过中间件链处理，被中间件丢弃则不入队。
// 队列满时丢弃消息并返回 ErrQueueFull。
func (b *MessageBus) PublishInbound(msg InboundMessage) error {
	// 运行入站中间件
	processed := b.middleware.RunInbound(&msg)
	if processed == nil {
		return nil // 消息被中间件丢弃
	}

	// 发射事件
	b.events.Emit(EventMessageInbound, map[string]interface{}{
		"channel": processed.Channel, "sender": processed.SenderID,
	})

	select {
	case b.inbound <- *processed:
		return nil
	default:
		b.logger.Warn("inbound queue full, dropping message",
			"channel", processed.Channel, "chat_id", processed.ChatID)
		return ErrQueueFull
	}
}

// ConsumeInbound 阻塞等待入站消息，直到消息可用或上下文取消。
func (b *MessageBus) ConsumeInbound(ctx context.Context) (InboundMessage, error) {
	select {
	case msg := <-b.inbound:
		return msg, nil
	case <-ctx.Done():
		return InboundMessage{}, ctx.Err()
	}
}

// PublishOutbound 将 Agent 的响应发布到出站队列。
// 先经过出站中间件链处理，被中间件丢弃则不入队。
// 队列满时丢弃消息并返回 ErrQueueFull。
func (b *MessageBus) PublishOutbound(msg OutboundMessage) error {
	// 运行出站中间件
	processed := b.middleware.RunOutbound(&msg)
	if processed == nil {
		return nil
	}

	// 发射事件
	b.events.Emit(EventMessageOutbound, map[string]interface{}{
		"channel": processed.Channel, "content_length": len(processed.Content),
	})

	select {
	case b.outbound <- *processed:
		return nil
	default:
		b.logger.Warn("outbound queue full, dropping message",
			"channel", processed.Channel, "chat_id", processed.ChatID)
		return ErrQueueFull
	}
}

// ConsumeOutbound 阻塞等待出站消息，直到消息可用或上下文取消。
func (b *MessageBus) ConsumeOutbound(ctx context.Context) (OutboundMessage, error) {
	select {
	case msg := <-b.outbound:
		return msg, nil
	case <-ctx.Done():
		return OutboundMessage{}, ctx.Err()
	}
}

// Stop 停止消息总线。
func (b *MessageBus) Stop() {
	select {
	case b.done <- struct{}{}:
	default:
	}
}

// InboundSize 返回入站队列中待处理的消息数。
func (b *MessageBus) InboundSize() int {
	return len(b.inbound)
}

// OutboundSize 返回出站队列中待处理的消息数。
func (b *MessageBus) OutboundSize() int {
	return len(b.outbound)
}
