// WhatsApp 渠道实现（通过 WebSocket 连接到 Node.js Bridge）。
// Bridge 基于 @whiskeysockets/baileys 库，提供 WebSocket 接口。
// 支持自动重连、QR 码认证、消息收发。

package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleClaw/bus"
)

// WhatsAppChannel 通过 WebSocket Bridge 实现 WhatsApp 渠道。
type WhatsAppChannel struct {
	BaseChannel
	bridgeURL string
	cancel    context.CancelFunc
	logger    *slog.Logger
	mu        sync.Mutex
	conn      wsConn // WebSocket 连接接口
}

// wsConn WebSocket 连接接口（便于测试）。
type wsConn interface {
	ReadMessage() (int, []byte, error)
	WriteMessage(messageType int, data []byte) error
	Close() error
}

// NewWhatsAppChannel 创建 WhatsApp 渠道。
func NewWhatsAppChannel(bridgeURL string, allowList []string, msgBus *bus.MessageBus, logger *slog.Logger) *WhatsAppChannel {
	if logger == nil {
		logger = slog.Default()
	}
	return &WhatsAppChannel{
		BaseChannel: BaseChannel{
			name:      "whatsapp",
			bus:       msgBus,
			allowList: allowList,
		},
		bridgeURL: bridgeURL,
		logger:    logger,
	}
}

func (w *WhatsAppChannel) Start(ctx context.Context) error {
	if w.bridgeURL == "" {
		return fmt.Errorf("whatsapp bridge URL not configured")
	}
	ctx, w.cancel = context.WithCancel(ctx)
	w.running.Store(true)
	go w.connectLoop(ctx)
	w.logger.Info("whatsapp channel started", "bridge", w.bridgeURL)
	return nil
}

func (w *WhatsAppChannel) Stop() error {
	if w.cancel != nil {
		w.cancel()
	}
	w.running.Store(false)
	w.mu.Lock()
	if w.conn != nil {
		_ = w.conn.Close()
	}
	w.mu.Unlock()
	w.logger.Info("whatsapp channel stopped")
	return nil
}

func (w *WhatsAppChannel) Send(msg bus.OutboundMessage) error {
	w.mu.Lock()
	conn := w.conn
	w.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("whatsapp: not connected to bridge")
	}

	bridgeMsg := bridgeOutMessage{
		Type:    "message",
		ChatID:  msg.ChatID,
		Content: msg.Content,
	}
	data, _ := json.Marshal(bridgeMsg)
	return conn.WriteMessage(1, data) // 1 = TextMessage
}

// connectLoop 自动重连的连接循环。
func (w *WhatsAppChannel) connectLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if err := w.connect(ctx); err != nil {
				if ctx.Err() != nil {
					return
				}
				w.logger.Error("whatsapp connect failed", "error", err)
			}
			// 断开后等待 5 秒重连
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}
}

// connect 建立 WebSocket 连接并读取消息。
func (w *WhatsAppChannel) connect(ctx context.Context) error {
	// 使用标准库做基础 WebSocket 握手
	// 生产环境应使用 gorilla/websocket，这里提供一个 HTTP 降级方案
	w.logger.Info("whatsapp connecting to bridge", "url", w.bridgeURL)

	// 构造 HTTP 请求（polling 模式降级方案）
	// 注意：完整的 WebSocket 需要引入 gorilla/websocket 依赖
	// 这里实现一个简单的 HTTP long-polling 替代方案
	client := &http.Client{Timeout: 30 * time.Second}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 尝试从 bridge 获取消息
		pollURL := w.bridgeURL + "/poll"
		resp, err := client.Get(pollURL)
		if err != nil {
			return err
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return fmt.Errorf("bridge returned status %d", resp.StatusCode)
		}

		var msgs []bridgeInMessage
		if err := json.NewDecoder(resp.Body).Decode(&msgs); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()

		for _, msg := range msgs {
			w.handleBridgeMessage(msg)
		}
	}
}

// handleBridgeMessage 处理 Bridge 的消息。
func (w *WhatsAppChannel) handleBridgeMessage(msg bridgeInMessage) {
	switch msg.Type {
	case "message":
		senderID := msg.From
		if !w.IsAllowed(senderID) {
			return
		}
		w.HandleMessage(bus.InboundMessage{
			Channel:   "whatsapp",
			SenderID:  senderID,
			ChatID:    msg.ChatID,
			Content:   msg.Content,
			Timestamp: time.Now(),
		})
	case "qr":
		w.logger.Info("WhatsApp QR code received - scan to authenticate")
	case "status":
		w.logger.Info("WhatsApp status update", "status", msg.Status)
	case "error":
		w.logger.Error("WhatsApp bridge error", "error", msg.Content)
	}
}

// === Bridge 消息类型 ===

type bridgeInMessage struct {
	Type    string `json:"type"`    // "message", "qr", "status", "error"
	From    string `json:"from"`
	ChatID  string `json:"chat_id"`
	Content string `json:"content"`
	Status  string `json:"status"`
}

type bridgeOutMessage struct {
	Type    string `json:"type"`
	ChatID  string `json:"chat_id"`
	Content string `json:"content"`
}
