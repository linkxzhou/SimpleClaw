// Package channels 提供 SimpleClaw 的聊天渠道实现。
// 包含 Channel 接口定义、BaseChannel 基础实现。
package channels

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/linkxzhou/SimpleClaw/bus"
)

// Channel 是所有聊天渠道实现必须满足的接口。
type Channel interface {
	Name() string
	Start(ctx context.Context) error
	Stop() error
	Send(msg bus.OutboundMessage) error
	IsRunning() bool
}

// BaseChannel 为渠道实现提供通用功能。
type BaseChannel struct {
	name      string
	bus       *bus.MessageBus
	allowList []string
	running   atomic.Bool
}

func (b *BaseChannel) Name() string     { return b.name }
func (b *BaseChannel) IsRunning() bool   { return b.running.Load() }

// IsAllowed 检查发送者是否被允许。空白名单允许所有人。
func (b *BaseChannel) IsAllowed(senderID string) bool {
	if len(b.allowList) == 0 {
		return true
	}
	for _, a := range b.allowList {
		if a == senderID {
			return true
		}
	}
	// 支持管道分隔的复合 ID
	if strings.Contains(senderID, "|") {
		for _, part := range strings.Split(senderID, "|") {
			if part == "" {
				continue
			}
			for _, a := range b.allowList {
				if a == part {
					return true
				}
			}
		}
	}
	return false
}

// HandleMessage 将消息转发到消息总线。
func (b *BaseChannel) HandleMessage(msg bus.InboundMessage) error {
	return b.bus.PublishInbound(msg)
}
