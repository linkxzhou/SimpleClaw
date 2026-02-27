package agent

import (
	"context"
	"encoding/json"

	"github.com/linkxzhou/SimpleClaw/bus"
)

// 直接复用 bus 包的消息类型，避免镜像定义和适配器转换。
type (
	MediaItem       = bus.MediaItem
	InboundMessage  = bus.InboundMessage
	OutboundMessage = bus.OutboundMessage
)

// ============ 消息总线 ============

// MessageBus 定义消息总线接口
type MessageBus interface {
	ConsumeInbound(ctx context.Context) (InboundMessage, error)
	PublishOutbound(msg OutboundMessage) error
	PublishInbound(msg InboundMessage) error
}

// ============ LLM Provider ============

// LLMProvider 定义 LLM 提供商接口。
type LLMProvider interface {
	Chat(ctx context.Context, messages []Message, tools []ToolDef, model string) (*LLMResponse, error)
	GetDefaultModel() string
}

// LLMResponse LLM 的响应
type LLMResponse struct {
	Content      string     `json:"content"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	HasToolCalls bool       `json:"has_tool_calls"`
	FinishReason string     `json:"finish_reason,omitempty"`
}

// ============ 消息 ============

// Message 代表 LLM 聊天消息
type Message struct {
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	ToolCalls  []ToolCallEntry `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Name       string          `json:"name,omitempty"`
}

// ToolCallEntry 工具调用条目（OpenAI 格式）
type ToolCallEntry struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction 工具调用函数信息
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ============ 工具定义 ============

// ToolDef 定义工具的 Schema（OpenAI function calling 格式）
type ToolDef struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction 工具函数定义
type ToolFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

// ToolCall 代表 LLM 返回的工具调用请求
type ToolCall struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ToolCallToEntry 将 ToolCall 转为 ToolCallEntry（用于消息历史）
func ToolCallToEntry(tc ToolCall) ToolCallEntry {
	argsJSON, _ := json.Marshal(tc.Arguments)
	return ToolCallEntry{
		ID:   tc.ID,
		Type: "function",
		Function: ToolCallFunction{
			Name:      tc.Name,
			Arguments: string(argsJSON),
		},
	}
}

// ============ 辅助类型 ============

// SendCallback 消息发送回调类型
type SendCallback func(msg OutboundMessage) error

// Spawner 子 Agent 生成器接口
type Spawner interface {
	Spawn(ctx context.Context, task, label, originChannel, originChatID string) (string, error)
}

// MarshalToolCallArgs 将工具调用参数序列化为 JSON 字符串
func MarshalToolCallArgs(args map[string]interface{}) string {
	data, _ := json.Marshal(args)
	return string(data)
}
