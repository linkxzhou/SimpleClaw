// OpenAI 兼容的 LLM 提供商实现。
// 通过 HTTP 调用 /chat/completions 端点，支持 OpenAI、OpenRouter、vLLM、智谱等
// 所有兼容 OpenAI API 格式的服务。

package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OpenAIProvider 使用 OpenAI 兼容的 Chat Completions API 实现 LLMProvider。
// 兼容 OpenAI、OpenRouter、vLLM、智谱等所有 OpenAI 兼容端点。
type OpenAIProvider struct {
	apiKey       string       // API 密钥
	apiBase      string       // API 基础 URL
	defaultModel string       // 默认模型名称
	client       *http.Client // HTTP 客户端
}

// OpenAIConfig 包含 OpenAI 提供商的配置。
type OpenAIConfig struct {
	APIKey       string        // API 密钥
	APIBase      string        // API 基础 URL（默认 https://api.openai.com/v1）
	DefaultModel string        // 默认模型（默认 gpt-4o）
	Timeout      time.Duration // HTTP 请求超时时间
}

// NewOpenAIProvider 创建一个新的 OpenAI 兼容 LLM 提供商。
func NewOpenAIProvider(cfg OpenAIConfig) *OpenAIProvider {
	if cfg.APIBase == "" {
		cfg.APIBase = "https://api.openai.com/v1"
	}
	if cfg.DefaultModel == "" {
		cfg.DefaultModel = "gpt-4o"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 120 * time.Second
	}
	return &OpenAIProvider{
		apiKey:       cfg.APIKey,
		apiBase:      cfg.APIBase,
		defaultModel: cfg.DefaultModel,
		client:       &http.Client{Timeout: cfg.Timeout},
	}
}

// Chat 发送聊天补全请求。
// 构建请求体，发送 HTTP POST 到 /chat/completions，并解析响应。
func (p *OpenAIProvider) Chat(ctx context.Context, req ChatRequest) (*LLMResponse, error) {
	model := req.Model
	if model == "" {
		model = p.defaultModel
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	temp := req.Temperature
	if temp <= 0 {
		temp = 0.7
	}

	// 构建请求体
	body := map[string]interface{}{
		"model":       model,
		"messages":    req.Messages,
		"max_tokens":  maxTokens,
		"temperature": temp,
	}
	if len(req.Tools) > 0 {
		body["tools"] = req.Tools
		body["tool_choice"] = "auto"
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := p.apiBase + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// 非 200 状态码返回结构化错误（便于 ErrorClassifier 分类）
	if resp.StatusCode != http.StatusOK {
		return nil, &ProviderError{
			StatusCode: resp.StatusCode,
			Message:    string(respBody),
			ModelKey:   model,
		}
	}

	return p.parseResponse(respBody)
}

// GetDefaultModel 返回默认模型名称。
func (p *OpenAIProvider) GetDefaultModel() string {
	return p.defaultModel
}

// parseResponse 解析 OpenAI API 的 JSON 响应。
// 提取文本内容、工具调用和 token 使用统计。
func (p *OpenAIProvider) parseResponse(data []byte) (*LLMResponse, error) {
	var raw struct {
		Choices []struct {
			Message struct {
				Content   *string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if len(raw.Choices) == 0 {
		return &LLMResponse{
			Content:      "No response from LLM",
			FinishReason: "error",
		}, nil
	}

	choice := raw.Choices[0]
	result := &LLMResponse{
		FinishReason: choice.FinishReason,
	}
	if result.FinishReason == "" {
		result.FinishReason = "stop"
	}

	if choice.Message.Content != nil {
		result.Content = *choice.Message.Content
	}

	// 解析工具调用：将 arguments 字符串反序列化为 map
	for _, tc := range choice.Message.ToolCalls {
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			args = map[string]interface{}{"raw": tc.Function.Arguments}
		}
		result.ToolCalls = append(result.ToolCalls, ToolCallRequest{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}

	// 提取 token 使用统计
	if raw.Usage != nil {
		result.Usage = map[string]int{
			"prompt_tokens":     raw.Usage.PromptTokens,
			"completion_tokens": raw.Usage.CompletionTokens,
			"total_tokens":      raw.Usage.TotalTokens,
		}
	}

	return result, nil
}
