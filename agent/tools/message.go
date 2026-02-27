package tools

import (
	"context"
	"fmt"
	"sync"
)

// OutboundMessage 发送的消息（本地定义，避免循环导入）
type OutboundMessage struct {
	Channel string
	ChatID  string
	Content string
}

// SendCallback 消息发送回调
type SendCallback func(msg OutboundMessage) error

// MessageTool 消息发送工具
type MessageTool struct {
	mu             sync.Mutex
	sendCallback   SendCallback
	defaultChannel string
	defaultChatID  string
}

// NewMessageTool 创建消息发送工具
func NewMessageTool(callback SendCallback) *MessageTool {
	return &MessageTool{sendCallback: callback}
}

// SetContext 设置当前消息上下文
func (t *MessageTool) SetContext(channel, chatID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.defaultChannel = channel
	t.defaultChatID = chatID
}

func (t *MessageTool) Name() string        { return "message" }
func (t *MessageTool) Description() string {
	return "Send a message to the user. Use this when you want to communicate something."
}
func (t *MessageTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The message content to send",
			},
			"channel": map[string]interface{}{
				"type":        "string",
				"description": "Optional: target channel (telegram, discord, etc.)",
			},
			"chat_id": map[string]interface{}{
				"type":        "string",
				"description": "Optional: target chat/user ID",
			},
		},
		"required": []string{"content"},
	}
}

func (t *MessageTool) Execute(_ context.Context, params map[string]interface{}) (string, error) {
	content, _ := params["content"].(string)
	if content == "" {
		return "", fmt.Errorf("content is required")
	}

	channel, _ := params["channel"].(string)
	chatID, _ := params["chat_id"].(string)

	t.mu.Lock()
	if channel == "" {
		channel = t.defaultChannel
	}
	if chatID == "" {
		chatID = t.defaultChatID
	}
	t.mu.Unlock()

	if channel == "" || chatID == "" {
		return "Error: No target channel/chat specified", nil
	}

	if t.sendCallback == nil {
		return "Error: Message sending not configured", nil
	}

	msg := OutboundMessage{
		Channel: channel,
		ChatID:  chatID,
		Content: content,
	}

	if err := t.sendCallback(msg); err != nil {
		return fmt.Sprintf("Error sending message: %s", err.Error()), nil
	}
	return fmt.Sprintf("Message sent to %s:%s", channel, chatID), nil
}
