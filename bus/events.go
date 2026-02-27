// Package bus 提供消息总线，实现渠道与 Agent 之间的解耦通信。
// 渠道通过入站队列发送消息给 Agent，Agent 通过出站队列将响应发回渠道。
package bus

import (
	"time"
)

// MediaItem 表示一个媒体附件。
type MediaItem struct {
	Type     string `json:"type"`               // "image", "audio", "video", "file"
	URL      string `json:"url"`                // 远程 URL 或本地路径
	Name     string `json:"name,omitempty"`     // 文件名
	MimeType string `json:"mime_type,omitempty"` // MIME 类型
}

// InboundMessage 表示从聊天渠道接收到的消息。
type InboundMessage struct {
	Channel   string            `json:"channel"`
	SenderID  string            `json:"sender_id"`
	ChatID    string            `json:"chat_id"`
	Content   string            `json:"content"`
	Timestamp time.Time         `json:"timestamp"`
	Media     []MediaItem       `json:"media,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// SessionKey 返回此消息的唯一会话键（格式：channel:chat_id）。
func (m *InboundMessage) SessionKey() string {
	return m.Channel + ":" + m.ChatID
}

// OutboundMessage 表示要发送到聊天渠道的消息。
type OutboundMessage struct {
	Channel  string            `json:"channel"`
	ChatID   string            `json:"chat_id"`
	Content  string            `json:"content"`
	ReplyTo  string            `json:"reply_to,omitempty"`
	Media    []MediaItem       `json:"media,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}
