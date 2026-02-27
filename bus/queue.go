// 消息总线核心实现：异步消息队列，解耦聊天渠道与 Agent 核心。
// 渠道将消息推入入站队列，Agent 处理后将响应推入出站队列。
// 支持订阅者模式，可注册出站消息处理器按渠道分发。

package bus

import (
	"context"
	"log/slog"
	"sync"
)

// OutboundHandler 是出站消息的回调处理函数。
type OutboundHandler func(msg OutboundMessage) error

// MessageBus 是异步消息队列，解耦聊天渠道与 Agent 核心。
type MessageBus struct {
	inbound  chan InboundMessage  // 入站消息队列
	outbound chan OutboundMessage // 出站消息队列

	mu          sync.RWMutex                    // 保护 subscribers 的并发访问
	subscribers map[string][]OutboundHandler     // 出站消息订阅者（按渠道名索引）

	done    chan struct{}  // 停止信号
	running bool          // 分发器运行状态
	logger  *slog.Logger  // 日志记录器
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
		inbound:     make(chan InboundMessage, bufSize),
		outbound:    make(chan OutboundMessage, bufSize),
		subscribers: make(map[string][]OutboundHandler),
		done:        make(chan struct{}),
		logger:      logger,
	}
}

// PublishInbound 将来自渠道的消息发布到入站队列。
// 队列满时丢弃消息并返回 ErrQueueFull。
func (b *MessageBus) PublishInbound(msg InboundMessage) error {
	select {
	case b.inbound <- msg:
		return nil
	default:
		b.logger.Warn("inbound queue full, dropping message",
			"channel", msg.Channel, "chat_id", msg.ChatID)
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
// 队列满时丢弃消息并返回 ErrQueueFull。
func (b *MessageBus) PublishOutbound(msg OutboundMessage) error {
	select {
	case b.outbound <- msg:
		return nil
	default:
		b.logger.Warn("outbound queue full, dropping message",
			"channel", msg.Channel, "chat_id", msg.ChatID)
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

// SubscribeOutbound 为指定渠道注册出站消息处理器。
func (b *MessageBus) SubscribeOutbound(channel string, handler OutboundHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers[channel] = append(b.subscribers[channel], handler)
}

// DispatchOutbound 启动出站消息分发循环。
// 从出站队列消费消息并分发给对应渠道的订阅者。
// 阻塞直到 Stop() 被调用或上下文取消。
func (b *MessageBus) DispatchOutbound(ctx context.Context) {
	b.running = true
	defer func() { b.running = false }()

	for {
		select {
		case msg := <-b.outbound:
			b.mu.RLock()
			handlers := b.subscribers[msg.Channel]
			b.mu.RUnlock()

			for _, h := range handlers {
				if err := h(msg); err != nil {
					b.logger.Error("dispatch outbound error",
						"channel", msg.Channel, "error", err)
				}
			}
		case <-ctx.Done():
			return
		case <-b.done:
			return
		}
	}
}

// Stop 停止分发循环。
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
