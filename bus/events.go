// 事件系统：解耦的观察者模式，支持按类型订阅和通配符订阅。
// 事件处理器 panic 不会崩溃主流程（recover 保护）。

package bus

import (
	"sync"
	"time"
)

// EventType 事件类型。
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

// Event 一个事件。
type Event struct {
	Type      EventType              `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// EventHandler 事件处理函数。
type EventHandler func(event Event)

// EventBus 事件总线。
type EventBus struct {
	handlers  map[EventType][]EventHandler
	wildcards []EventHandler // "*" 通配符监听
	mu        sync.RWMutex
}

// NewEventBus 创建事件总线。
func NewEventBus() *EventBus {
	return &EventBus{
		handlers: make(map[EventType][]EventHandler),
	}
}

// On 注册指定事件类型的处理器。
func (eb *EventBus) On(eventType EventType, handler EventHandler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.handlers[eventType] = append(eb.handlers[eventType], handler)
}

// OnAll 注册通配符处理器（接收所有事件）。
func (eb *EventBus) OnAll(handler EventHandler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.wildcards = append(eb.wildcards, handler)
}

// Emit 触发事件。处理器 panic 被 recover 保护，不影响主流程。
func (eb *EventBus) Emit(eventType EventType, data map[string]interface{}) {
	event := Event{Type: eventType, Timestamp: time.Now(), Data: data}

	eb.mu.RLock()
	handlers := make([]EventHandler, len(eb.handlers[eventType]))
	copy(handlers, eb.handlers[eventType])
	wildcards := make([]EventHandler, len(eb.wildcards))
	copy(wildcards, eb.wildcards)
	eb.mu.RUnlock()

	for _, h := range handlers {
		func() {
			defer func() { recover() }()
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

// HandlerCount 返回指定事件类型的处理器数量。
func (eb *EventBus) HandlerCount(eventType EventType) int {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	return len(eb.handlers[eventType])
}

// WildcardCount 返回通配符处理器数量。
func (eb *EventBus) WildcardCount() int {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	return len(eb.wildcards)
}
