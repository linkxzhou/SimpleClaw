// 渠道管理器：管理所有聊天渠道并协调消息路由。

package channels

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/linkxzhou/SimpleClaw/bus"
	"github.com/linkxzhou/SimpleClaw/config"
)

// Manager 管理聊天渠道并协调消息路由。
type Manager struct {
	config   *config.Config
	bus      *bus.MessageBus
	channels map[string]Channel
	logger   *slog.Logger
	mu       sync.Mutex
	cancel   context.CancelFunc
}

// NewManager 创建渠道管理器，根据配置自动初始化已启用的渠道。
func NewManager(cfg *config.Config, b *bus.MessageBus, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	m := &Manager{
		config:   cfg,
		bus:      b,
		channels: make(map[string]Channel),
		logger:   logger,
	}
	m.initChannels()
	return m
}

func (m *Manager) initChannels() {
	if m.config.Channels.Telegram.Enabled {
		ch := NewTelegramChannel(
			m.config.Channels.Telegram.Token,
			m.config.Channels.Telegram.AllowFrom,
			m.bus,
			m.logger,
		)
		m.channels["telegram"] = ch
		m.logger.Info("telegram channel initialized")
	}

	if m.config.Channels.Discord.Enabled {
		m.logger.Warn("discord channel not yet implemented, skipping")
	}

	if m.config.Channels.WhatsApp.Enabled {
		ch := NewWhatsAppChannel(
			m.config.Channels.WhatsApp.BridgeURL,
			m.config.Channels.WhatsApp.AllowFrom,
			m.bus,
			m.logger,
		)
		m.channels["whatsapp"] = ch
		m.logger.Info("whatsapp channel initialized")
	}

	if m.config.Channels.Feishu.Enabled {
		m.logger.Warn("feishu channel not yet implemented, skipping")
	}
}

// Register 注册一个自定义渠道。
func (m *Manager) Register(ch Channel) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels[ch.Name()] = ch
}

// StartAll 启动所有渠道和出站消息分发器。
func (m *Manager) StartAll(ctx context.Context) error {
	if len(m.channels) == 0 {
		m.logger.Warn("no channels enabled")
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel

	go m.dispatchOutbound(ctx)

	var wg sync.WaitGroup
	errCh := make(chan error, len(m.channels))

	for name, ch := range m.channels {
		wg.Add(1)
		go func(name string, ch Channel) {
			defer wg.Done()
			m.logger.Info("starting channel", "name", name)
			if err := ch.Start(ctx); err != nil {
				m.logger.Error("channel start error", "name", name, "error", err)
				errCh <- fmt.Errorf("channel %s: %w", name, err)
			}
		}(name, ch)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		return err
	}
	return nil
}

// StopAll 停止所有渠道和分发器。
func (m *Manager) StopAll() {
	m.logger.Info("stopping all channels")
	if m.cancel != nil {
		m.cancel()
	}
	for name, ch := range m.channels {
		if err := ch.Stop(); err != nil {
			m.logger.Error("stop channel error", "name", name, "error", err)
		}
	}
}

func (m *Manager) dispatchOutbound(ctx context.Context) {
	for {
		msg, err := m.bus.ConsumeOutbound(ctx)
		if err != nil {
			return
		}
		ch, ok := m.channels[msg.Channel]
		if !ok {
			m.logger.Warn("unknown channel", "channel", msg.Channel)
			continue
		}
		if err := ch.Send(msg); err != nil {
			m.logger.Error("send error", "channel", msg.Channel, "error", err)
		}
	}
}

// GetChannel 按名称获取渠道。
func (m *Manager) GetChannel(name string) (Channel, bool) {
	ch, ok := m.channels[name]
	return ch, ok
}

// EnabledChannels 返回已启用渠道名称列表。
func (m *Manager) EnabledChannels() []string {
	names := make([]string, 0, len(m.channels))
	for name := range m.channels {
		names = append(names, name)
	}
	return names
}

// ChannelStatus 渠道状态。
type ChannelStatus struct {
	Enabled bool `json:"enabled"`
	Running bool `json:"running"`
}

// GetStatus 返回所有渠道状态。
func (m *Manager) GetStatus() map[string]ChannelStatus {
	status := make(map[string]ChannelStatus, len(m.channels))
	for name, ch := range m.channels {
		status[name] = ChannelStatus{Enabled: true, Running: ch.IsRunning()}
	}
	return status
}
