// Package memory 提供 SimpleClaw 的持久化存储功能。
// 包含会话管理、系统状态追踪和记忆存储（长期记忆 + 每日笔记）。
package memory

import (
	"time"
)

// Session 表示一个对话会话。
type Session struct {
	Key       string                   `json:"key"`                // 会话键（格式：channel:chat_id）
	Messages  []map[string]interface{} `json:"messages"`           // 消息历史
	CreatedAt time.Time                `json:"created_at"`         // 创建时间
	UpdatedAt time.Time                `json:"updated_at"`         // 最后更新时间
	Metadata  map[string]interface{}   `json:"metadata,omitempty"` // 额外元数据（可选）
}

// NewSession 创建一个新的空会话。
func NewSession(key string) *Session {
	now := time.Now()
	return &Session{
		Key:       key,
		Messages:  make([]map[string]interface{}, 0),
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  make(map[string]interface{}),
	}
}

// AddMessage 向会话中添加一条消息。
// extra 中的键值对会合并到消息 map 中。
func (s *Session) AddMessage(role, content string, extra map[string]interface{}) {
	msg := map[string]interface{}{
		"role":      role,
		"content":   content,
		"timestamp": time.Now().Format(time.RFC3339),
	}
	for k, v := range extra {
		msg[k] = v
	}
	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = time.Now()
}

// GetHistory 返回最近的消息历史（LLM 格式，仅包含 role 和 content）。
// maxMessages <= 0 时默认返回最近 50 条。
func (s *Session) GetHistory(maxMessages int) []map[string]interface{} {
	if maxMessages <= 0 {
		maxMessages = 50
	}
	msgs := s.Messages
	if len(msgs) > maxMessages {
		msgs = msgs[len(msgs)-maxMessages:]
	}
	result := make([]map[string]interface{}, len(msgs))
	for i, m := range msgs {
		result[i] = map[string]interface{}{
			"role":    m["role"],
			"content": m["content"],
		}
	}
	return result
}

// Clear 清空会话中的所有消息。
func (s *Session) Clear() {
	s.Messages = make([]map[string]interface{}, 0)
	s.UpdatedAt = time.Now()
}
