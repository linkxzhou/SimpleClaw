// 中间件接口：管道模式消息变换链。
// InboundMiddleware 在消息到达 Agent 前执行；
// OutboundMiddleware 在消息发给用户前执行；
// 返回 nil 表示丢弃该消息。

package bus

import "sync"

// InboundMiddleware 入站中间件接口。
type InboundMiddleware interface {
	Name() string
	ProcessInbound(msg *InboundMessage) *InboundMessage
}

// OutboundMiddleware 出站中间件接口。
type OutboundMiddleware interface {
	Name() string
	ProcessOutbound(msg *OutboundMessage) *OutboundMessage
}

// MiddlewareChain 管道式执行中间件链。
type MiddlewareChain struct {
	inbound  []InboundMiddleware
	outbound []OutboundMiddleware
	mu       sync.RWMutex
}

// NewMiddlewareChain 创建空中间件链。
func NewMiddlewareChain() *MiddlewareChain {
	return &MiddlewareChain{}
}

// AddInbound 添加入站中间件。
func (mc *MiddlewareChain) AddInbound(mw InboundMiddleware) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.inbound = append(mc.inbound, mw)
}

// AddOutbound 添加出站中间件。
func (mc *MiddlewareChain) AddOutbound(mw OutboundMiddleware) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.outbound = append(mc.outbound, mw)
}

// RunInbound 依次执行入站中间件。返回 nil 表示消息被丢弃。
func (mc *MiddlewareChain) RunInbound(msg *InboundMessage) *InboundMessage {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	current := msg
	for _, mw := range mc.inbound {
		current = mw.ProcessInbound(current)
		if current == nil {
			return nil
		}
	}
	return current
}

// RunOutbound 依次执行出站中间件。返回 nil 表示消息被丢弃。
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

// InboundCount 返回入站中间件数量。
func (mc *MiddlewareChain) InboundCount() int {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return len(mc.inbound)
}

// OutboundCount 返回出站中间件数量。
func (mc *MiddlewareChain) OutboundCount() int {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return len(mc.outbound)
}
