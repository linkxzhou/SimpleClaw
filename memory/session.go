// Package memory 提供 SimpleClaw 的持久化存储功能。
// 包含会话管理、系统状态追踪和记忆存储（长期记忆 + 每日笔记）。
package memory

import (
	"time"
)

// SessionMessage 表示会话中的一条消息（强类型，替代 map[string]interface{}）。
type SessionMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp,omitempty"`
}

// Session 表示一个对话会话。
type Session struct {
	Key       string                 `json:"key"`                // 会话键（格式：channel:chat_id）
	Messages  []SessionMessage       `json:"messages"`           // 消息历史
	CreatedAt time.Time              `json:"created_at"`         // 创建时间
	UpdatedAt time.Time              `json:"updated_at"`         // 最后更新时间
	Metadata  map[string]interface{} `json:"metadata,omitempty"` // 额外元数据（可选）
}

// NewSession 创建一个新的空会话。
func NewSession(key string) *Session {
	now := time.Now()
	return &Session{
		Key:       key,
		Messages:  make([]SessionMessage, 0),
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  make(map[string]interface{}),
	}
}

// AddMessage 向会话中添加一条消息。
func (s *Session) AddMessage(role, content string) {
	s.Messages = append(s.Messages, SessionMessage{
		Role:      role,
		Content:   content,
		Timestamp: time.Now().Format(time.RFC3339),
	})
	s.UpdatedAt = time.Now()
}

// GetHistory 返回最近的消息历史（仅 role + content）。
// maxMessages <= 0 时默认返回最近 50 条。
func (s *Session) GetHistory(maxMessages int) []SessionMessage {
	if maxMessages <= 0 {
		maxMessages = 50
	}
	msgs := s.Messages
	if len(msgs) > maxMessages {
		msgs = msgs[len(msgs)-maxMessages:]
	}
	result := make([]SessionMessage, len(msgs))
	for i, m := range msgs {
		result[i] = SessionMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}
	return result
}

// Clear 清空会话中的所有消息。
func (s *Session) Clear() {
	s.Messages = make([]SessionMessage, 0)
	s.UpdatedAt = time.Now()
}
