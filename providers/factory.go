// ProviderFactory 根据 vendor/model 格式的 modelKey 创建对应的 LLM Provider。
// 大多数厂商兼容 OpenAI API，因此复用 OpenAIProvider 配以不同的 baseURL。

package providers

import (
	"fmt"
	"strings"
	"time"
)

// 默认 API 端点。
var defaultEndpoints = map[string]string{
	"openai":      "https://api.openai.com/v1",
	"anthropic":   "https://api.anthropic.com/v1",
	"deepseek":    "https://api.deepseek.com/v1",
	"zhipu":       "https://open.bigmodel.cn/api/paas/v4",
	"groq":        "https://api.groq.com/openai/v1",
	"gemini":      "https://generativelanguage.googleapis.com/v1beta/openai",
	"openrouter":  "https://openrouter.ai/api/v1",
	"moonshot":    "https://api.moonshot.cn/v1",
	"qwen":        "https://dashscope.aliyuncs.com/compatible-mode/v1",
	"nvidia":      "https://integrate.api.nvidia.com/v1",
	"cerebras":    "https://api.cerebras.ai/v1",
	"ollama":      "http://localhost:11434/v1",
	"vllm":        "http://localhost:8000/v1",
}

// ProviderFactory 根据 vendor/model 格式创建 Provider。
type ProviderFactory struct {
	apiKeys   map[string]string // vendor -> apiKey
	endpoints map[string]string // vendor -> custom endpoint
	timeout   time.Duration
}

// FactoryConfig 包含 Factory 的配置。
type FactoryConfig struct {
	APIKeys   map[string]string // vendor -> apiKey
	Endpoints map[string]string // vendor -> custom base URL
	Timeout   time.Duration
}

// NewProviderFactory 创建 Provider 工厂。
func NewProviderFactory(cfg FactoryConfig) *ProviderFactory {
	if cfg.Timeout == 0 {
		cfg.Timeout = 120 * time.Second
	}
	if cfg.APIKeys == nil {
		cfg.APIKeys = make(map[string]string)
	}
	if cfg.Endpoints == nil {
		cfg.Endpoints = make(map[string]string)
	}
	return &ProviderFactory{
		apiKeys:   cfg.APIKeys,
		endpoints: cfg.Endpoints,
		timeout:   cfg.Timeout,
	}
}

// CreateProvider 根据 modelKey ("vendor/model") 创建 Provider。
func (f *ProviderFactory) CreateProvider(modelKey string) (LLMProvider, error) {
	vendor, model := SplitModelKey(modelKey)

	apiKey := f.apiKeys[vendor]
	baseURL := f.getEndpoint(vendor)

	if baseURL == "" {
		return nil, fmt.Errorf("unknown vendor: %s (model: %s)", vendor, model)
	}

	return NewOpenAIProvider(OpenAIConfig{
		APIKey:       apiKey,
		APIBase:      baseURL,
		DefaultModel: model,
		Timeout:      f.timeout,
	}), nil
}

// getEndpoint 返回 vendor 的 API 端点（优先使用自定义端点）。
func (f *ProviderFactory) getEndpoint(vendor string) string {
	if ep, ok := f.endpoints[vendor]; ok && ep != "" {
		return ep
	}
	return defaultEndpoints[vendor]
}

// SplitModelKey 拆分 "vendor/model" 为 vendor 和 model。
// 如果没有 "/" 前缀，默认 vendor 为 "openai"。
func SplitModelKey(modelKey string) (vendor, model string) {
	parts := strings.SplitN(modelKey, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "openai", modelKey
}
