package main

// adapter.go 提供 bus 包和 agent 包之间的类型适配。
// agent 包定义了自己的 MessageBus / LLMProvider 接口（使用 agent 内部类型），
// 而 bus / providers / session 包有各自独立的类型。此文件桥接二者。

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/linkxzhou/SimpleClaw/agent"
	"github.com/linkxzhou/SimpleClaw/bus"
	"github.com/linkxzhou/SimpleClaw/config"
	"github.com/linkxzhou/SimpleClaw/memory"
	"github.com/linkxzhou/SimpleClaw/providers"
)

// ============ MessageBus 适配器 ============

// BusAdapter 将 *bus.MessageBus 适配为 agent.MessageBus 接口。
type BusAdapter struct {
	inner *bus.MessageBus
}

// NewBusAdapter 创建适配器。
func NewBusAdapter(b *bus.MessageBus) *BusAdapter {
	return &BusAdapter{inner: b}
}

func (a *BusAdapter) ConsumeInbound(ctx context.Context) (agent.InboundMessage, error) {
	return a.inner.ConsumeInbound(ctx)
}

func (a *BusAdapter) PublishOutbound(msg agent.OutboundMessage) error {
	return a.inner.PublishOutbound(msg)
}

func (a *BusAdapter) PublishInbound(msg agent.InboundMessage) error {
	return a.inner.PublishInbound(msg)
}

// ============ LLM Provider 适配器（单 Provider 直接调用） ============

// ProviderAdapter 将 *providers.OpenAIProvider 适配为 agent.LLMProvider 接口。
type ProviderAdapter struct {
	inner *providers.OpenAIProvider
}

// NewProviderAdapter 创建适配器。
func NewProviderAdapter(p *providers.OpenAIProvider) *ProviderAdapter {
	return &ProviderAdapter{inner: p}
}

func (a *ProviderAdapter) Chat(ctx context.Context, messages []agent.Message, toolDefs []agent.ToolDef, model string) (*agent.LLMResponse, error) {
	req := buildChatRequest(messages, toolDefs, model)
	resp, err := a.inner.Chat(ctx, req)
	if err != nil {
		return nil, err
	}
	return convertLLMResponse(resp), nil
}

func (a *ProviderAdapter) GetDefaultModel() string {
	return a.inner.GetDefaultModel()
}

// ============ FallbackChain Provider 适配器 ============

// FallbackProviderAdapter 将 *providers.FallbackChain 适配为 agent.LLMProvider 接口。
// 支持多 Provider 容错执行链。
type FallbackProviderAdapter struct {
	chain      *providers.FallbackChain
	candidates []providers.Candidate
	model      string
	logger     *slog.Logger
}

// NewFallbackProviderAdapter 创建 FallbackChain 适配器。
func NewFallbackProviderAdapter(
	chain *providers.FallbackChain,
	primaryModel string,
	fallbackModels []string,
	logger *slog.Logger,
) (*FallbackProviderAdapter, error) {
	candidates, err := chain.ResolveCandidates(primaryModel, fallbackModels)
	if err != nil {
		return nil, fmt.Errorf("resolve candidates: %w", err)
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &FallbackProviderAdapter{
		chain:      chain,
		candidates: candidates,
		model:      primaryModel,
		logger:     logger,
	}, nil
}

func (a *FallbackProviderAdapter) Chat(ctx context.Context, messages []agent.Message, toolDefs []agent.ToolDef, model string) (*agent.LLMResponse, error) {
	if model == "" {
		model = a.model
	}
	req := buildChatRequest(messages, toolDefs, model)
	resp, err := a.chain.Execute(ctx, a.candidates, req)
	if err != nil {
		return nil, err
	}
	return convertLLMResponse(resp), nil
}

func (a *FallbackProviderAdapter) GetDefaultModel() string {
	_, model := providers.SplitModelKey(a.model)
	return model
}

// ============ 共享转换辅助函数 ============

// buildChatRequest 将 agent 内部消息格式转换为 providers.ChatRequest。
func buildChatRequest(messages []agent.Message, toolDefs []agent.ToolDef, model string) providers.ChatRequest {
	// 转换 messages -> []map[string]interface{}
	msgsRaw := make([]map[string]interface{}, len(messages))
	for i, m := range messages {
		entry := map[string]interface{}{
			"role":    m.Role,
			"content": m.Content,
		}
		if m.ToolCallID != "" {
			entry["tool_call_id"] = m.ToolCallID
		}
		if m.Name != "" {
			entry["name"] = m.Name
		}
		if len(m.ToolCalls) > 0 {
			tcs := make([]map[string]interface{}, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				tcs[j] = map[string]interface{}{
					"id":   tc.ID,
					"type": tc.Type,
					"function": map[string]interface{}{
						"name":      tc.Function.Name,
						"arguments": tc.Function.Arguments,
					},
				}
			}
			entry["tool_calls"] = tcs
		}
		msgsRaw[i] = entry
	}

	// 转换 toolDefs -> []map[string]interface{}
	toolsRaw := make([]map[string]interface{}, len(toolDefs))
	for i, td := range toolDefs {
		toolsRaw[i] = map[string]interface{}{
			"type": td.Type,
			"function": map[string]interface{}{
				"name":        td.Function.Name,
				"description": td.Function.Description,
				"parameters":  td.Function.Parameters,
			},
		}
	}

	return providers.ChatRequest{
		Messages: msgsRaw,
		Tools:    toolsRaw,
		Model:    model,
	}
}

// convertLLMResponse 将 providers.LLMResponse 转换为 agent.LLMResponse。
func convertLLMResponse(resp *providers.LLMResponse) *agent.LLMResponse {
	var agentToolCalls []agent.ToolCall
	for _, tc := range resp.ToolCalls {
		agentToolCalls = append(agentToolCalls, agent.ToolCall{
			ID:        tc.ID,
			Name:      tc.Name,
			Arguments: tc.Arguments,
		})
	}
	return &agent.LLMResponse{
		Content:      resp.Content,
		ToolCalls:    agentToolCalls,
		HasToolCalls: len(agentToolCalls) > 0,
		FinishReason: resp.FinishReason,
	}
}

// ============ Session Store 适配器 ============

// SessionStoreAdapter 将 *memory.SessionManager 适配为 agent.SessionStore 接口。
type SessionStoreAdapter struct {
	mgr    *memory.SessionManager
	logger *slog.Logger
}

// NewSessionStoreAdapter 创建 Session Store 适配器。
func NewSessionStoreAdapter(mgr *memory.SessionManager, logger *slog.Logger) *SessionStoreAdapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &SessionStoreAdapter{mgr: mgr, logger: logger}
}

func (a *SessionStoreAdapter) GetOrCreateSession(key string) []agent.Message {
	s := a.mgr.GetOrCreate(key)
	history := s.GetHistory(50)
	result := make([]agent.Message, 0, len(history))
	for _, m := range history {
		role, _ := m["role"].(string)
		content, _ := m["content"].(string)
		if role != "" {
			result = append(result, agent.Message{Role: role, Content: content})
		}
	}
	return result
}

func (a *SessionStoreAdapter) SaveSession(key string, messages []agent.Message) {
	s := a.mgr.GetOrCreate(key)
	s.Clear()
	for _, m := range messages {
		s.AddMessage(m.Role, m.Content, nil)
	}
	if err := a.mgr.Save(s); err != nil {
		a.logger.Warn("failed to save session", "key", key, "error", err)
	}
}

// ============ Provider 初始化公共函数 ============

// resolveModelConfig 从配置中解析主模型和备选模型。
func resolveModelConfig(cfg *config.Config) (primaryModel string, fallbackModels []string) {
	primaryModel = cfg.Agents.Defaults.Model
	for _, ac := range cfg.Agents.List {
		if ac.Default || ac.ID == "" {
			if ac.Model != "" {
				primaryModel = ac.Model
			}
			fallbackModels = ac.Fallbacks
			break
		}
	}
	return
}

// buildProviderStack 创建 LLM Provider（带容错链），失败时退化到单 Provider。
func buildProviderStack(cfg *config.Config, logger *slog.Logger) (agent.LLMProvider, string, []string) {
	primaryModel, fallbackModels := resolveModelConfig(cfg)

	factory := providers.NewProviderFactory(providers.FactoryConfig{
		APIKeys:   cfg.GetAPIKeys(),
		Endpoints: cfg.GetEndpoints(),
		Timeout:   120 * time.Second,
	})
	classifier := providers.NewErrorClassifier()
	cooldownTracker := providers.NewCooldownTracker()
	fallbackChain := providers.NewFallbackChain(classifier, cooldownTracker, factory, logger)

	fbAdapter, err := NewFallbackProviderAdapter(fallbackChain, primaryModel, fallbackModels, logger)
	if err != nil {
		// 退化到单 Provider
		if logger != nil {
			logger.Warn("FallbackChain init failed, falling back to single provider", "error", err)
		}
		singleProvider := providers.NewOpenAIProvider(providers.OpenAIConfig{
			APIKey:       cfg.GetAPIKey(),
			APIBase:      cfg.GetAPIBase(),
			DefaultModel: cfg.Agents.Defaults.Model,
			Timeout:      120 * time.Second,
		})
		return NewProviderAdapter(singleProvider), primaryModel, fallbackModels
	}
	return fbAdapter, primaryModel, fallbackModels
}
