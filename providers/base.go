// Package providers 提供 SimpleClaw 的 LLM 提供商抽象层。
// 定义了统一的 LLMProvider 接口和请求/响应类型，
// 使 Agent 核心可以无感知地切换不同的 LLM 后端。
package providers

import (
	"context"
)

// ToolCallRequest 表示 LLM 返回的工具调用请求。
type ToolCallRequest struct {
	ID        string                 `json:"id"`        // 工具调用唯一 ID
	Name      string                 `json:"name"`      // 工具名称
	Arguments map[string]interface{} `json:"arguments"` // 工具参数
}

// LLMResponse 表示 LLM 提供商的响应。
type LLMResponse struct {
	Content      string            `json:"content,omitempty"`    // 文本响应内容
	ToolCalls    []ToolCallRequest `json:"tool_calls,omitempty"` // 工具调用列表
	FinishReason string            `json:"finish_reason"`        // 结束原因（stop/tool_calls/error）
	Usage        map[string]int    `json:"usage,omitempty"`      // Token 使用统计
}

// HasToolCalls 返回响应中是否包含工具调用。
func (r *LLMResponse) HasToolCalls() bool {
	return len(r.ToolCalls) > 0
}

// LLMProvider 是所有 LLM 提供商实现必须满足的接口。
type LLMProvider interface {
	// Chat 发送聊天补全请求并返回响应。
	Chat(ctx context.Context, req ChatRequest) (*LLMResponse, error)

	// GetDefaultModel 返回此提供商的默认模型。
	GetDefaultModel() string
}

// ============ 强类型消息与工具定义 ============

// ChatMessage 聊天消息（强类型，取代 map[string]interface{}）
type ChatMessage struct {
	Role       string              `json:"role"`
	Content    string              `json:"content"`
	ToolCalls  []ChatToolCallEntry `json:"tool_calls,omitempty"`
	ToolCallID string              `json:"tool_call_id,omitempty"`
	Name       string              `json:"name,omitempty"`
}

// ChatToolCallEntry 工具调用条目（OpenAI 格式）
type ChatToolCallEntry struct {
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function ChatToolCallFunction `json:"function"`
}

// ChatToolCallFunction 工具调用函数信息
type ChatToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatToolDef 工具定义（OpenAI function calling 格式）
type ChatToolDef struct {
	Type     string           `json:"type"`
	Function ChatToolFunction `json:"function"`
}

// ChatToolFunction 工具函数定义
type ChatToolFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

// ChatRequest 包含聊天补全请求的参数（强类型版本）。
type ChatRequest struct {
	Messages    []ChatMessage `json:"messages"`              // 消息列表（强类型）
	Tools       []ChatToolDef `json:"tools,omitempty"`       // 工具定义列表（强类型）
	Model       string        `json:"model,omitempty"`       // 模型名称
	MaxTokens   int           `json:"max_tokens,omitempty"`  // 最大生成 token 数
	Temperature float64       `json:"temperature,omitempty"` // 采样温度
}
