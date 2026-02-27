// Package config 提供 SimpleClaw 的配置模式定义和加载功能。
// 包含根配置结构体、各子模块配置（Agent、Channels、Providers、Gateway、Tools），
// 以及默认配置生成和辅助方法。
package config

import (
	"strings"

	"github.com/linkxzhou/SimpleClaw/utils"
)

// Config 是 SimpleClaw 的根配置结构体。
type Config struct {
	Agents    AgentsConfig    `json:"agents"`              // Agent 相关配置
	Bindings  []BindingConfig `json:"bindings,omitempty"`  // 多 Agent 路由绑定
	Channels  ChannelsConfig  `json:"channels"`            // 聊天渠道配置
	Providers ProvidersConfig `json:"providers"`           // LLM 提供商配置
	Gateway   GatewayConfig   `json:"gateway"`             // 网关/服务器配置
	Tools     ToolsConfig     `json:"tools"`               // 工具配置
	Heartbeat HeartbeatConfig `json:"heartbeat"`           // 心跳配置
}

// DefaultConfig 返回带有合理默认值的配置。
func DefaultConfig() *Config {
	return &Config{
		Agents:    defaultAgentsConfig(),
		Channels:  defaultChannelsConfig(),
		Providers: defaultProvidersConfig(),
		Gateway:   GatewayConfig{Host: "0.0.0.0", Port: 18790},
		Tools:     defaultToolsConfig(),
		Heartbeat: HeartbeatConfig{Enabled: true, IntervalMin: 30},
	}
}

// WorkspacePath 返回展开后的工作区路径（将 ~ 替换为用户主目录）。
func (c *Config) WorkspacePath() string {
	return utils.ExpandHome(c.Agents.Defaults.Workspace)
}

// GetAPIKey 按优先级返回第一个非空的 API Key。
func (c *Config) GetAPIKey() string {
	for _, key := range []string{
		c.Agents.Defaults.APIKey,
		c.Providers.OpenRouter.APIKey,
		c.Providers.Anthropic.APIKey,
		c.Providers.OpenAI.APIKey,
		c.Providers.Gemini.APIKey,
		c.Providers.Zhipu.APIKey,
		c.Providers.Groq.APIKey,
		c.Providers.DeepSeek.APIKey,
		c.Providers.VLLM.APIKey,
	} {
		if key != "" {
			return key
		}
	}
	return ""
}

// GetAPIBase 根据当前活跃的提供商返回 API 基础 URL。
func (c *Config) GetAPIBase() string {
	if c.Agents.Defaults.APIBase != "" {
		return c.Agents.Defaults.APIBase
	}
	if c.Providers.OpenRouter.APIKey != "" {
		if c.Providers.OpenRouter.APIBase != "" {
			return c.Providers.OpenRouter.APIBase
		}
		return "https://openrouter.ai/api/v1"
	}
	if c.Providers.Zhipu.APIKey != "" {
		return c.Providers.Zhipu.APIBase
	}
	if c.Providers.VLLM.APIBase != "" {
		return c.Providers.VLLM.APIBase
	}
	return ""
}

// GetAPIKeys 返回所有已配置的 vendor -> apiKey 映射（用于 ProviderFactory）。
func (c *Config) GetAPIKeys() map[string]string {
	keys := make(map[string]string)
	if c.Providers.OpenRouter.APIKey != "" {
		keys["openrouter"] = c.Providers.OpenRouter.APIKey
	}
	if c.Providers.Anthropic.APIKey != "" {
		keys["anthropic"] = c.Providers.Anthropic.APIKey
	}
	if c.Providers.OpenAI.APIKey != "" {
		keys["openai"] = c.Providers.OpenAI.APIKey
	}
	if c.Providers.Gemini.APIKey != "" {
		keys["gemini"] = c.Providers.Gemini.APIKey
	}
	if c.Providers.Zhipu.APIKey != "" {
		keys["zhipu"] = c.Providers.Zhipu.APIKey
	}
	if c.Providers.Groq.APIKey != "" {
		keys["groq"] = c.Providers.Groq.APIKey
	}
	if c.Providers.DeepSeek.APIKey != "" {
		keys["deepseek"] = c.Providers.DeepSeek.APIKey
	}
	if c.Providers.VLLM.APIKey != "" {
		keys["vllm"] = c.Providers.VLLM.APIKey
	}
	// 如果有 agents.defaults 的 apiKey，用于默认 vendor
	if c.Agents.Defaults.APIKey != "" {
		vendor := vendorFromModel(c.Agents.Defaults.Model)
		if keys[vendor] == "" {
			keys[vendor] = c.Agents.Defaults.APIKey
		}
	}
	return keys
}

// GetEndpoints 返回所有自定义 endpoint 映射。
// 优先使用各 provider 的 apiBase，其次使用 agents.defaults.apiBase 作为全局回退。
func (c *Config) GetEndpoints() map[string]string {
	eps := make(map[string]string)

	// 收集各 vendor 的自定义 endpoint
	vendorBases := []struct {
		vendor string
		base   string
	}{
		{"openai", c.Providers.OpenAI.APIBase},
		{"anthropic", c.Providers.Anthropic.APIBase},
		{"openrouter", c.Providers.OpenRouter.APIBase},
		{"groq", c.Providers.Groq.APIBase},
		{"zhipu", c.Providers.Zhipu.APIBase},
		{"gemini", c.Providers.Gemini.APIBase},
		{"deepseek", c.Providers.DeepSeek.APIBase},
		{"vllm", c.Providers.VLLM.APIBase},
	}
	for _, v := range vendorBases {
		if v.base != "" {
			eps[v.vendor] = v.base
		}
	}

	// agents.defaults.apiBase 作为全局回退
	if c.Agents.Defaults.APIBase != "" {
		vendor := vendorFromModel(c.Agents.Defaults.Model)
		if eps[vendor] == "" {
			eps[vendor] = c.Agents.Defaults.APIBase
		}
	}

	return eps
}

// vendorFromModel 从 "vendor/model" 格式中提取 vendor 名称，默认 "openai"。
func vendorFromModel(model string) string {
	if model == "" {
		return "openai"
	}
	parts := strings.SplitN(model, "/", 2)
	if len(parts) == 2 {
		return parts[0]
	}
	return "openai"
}

// ============ Agent 配置 ============

// AgentsConfig 包含 Agent 相关的配置。
type AgentsConfig struct {
	Defaults AgentDefaults  `json:"defaults"`         // Agent 默认参数
	List     []AgentConfig  `json:"list,omitempty"`   // 多 Agent 列表
}

// AgentDefaults 包含 Agent 的默认参数。
type AgentDefaults struct {
	Workspace          string  `json:"workspace"`                        // 工作区路径（支持 ~ 前缀）
	Model              string  `json:"model"`                            // 默认 LLM 模型名称
	MaxTokens          int     `json:"maxTokens"`                        // 最大生成 token 数
	Temperature        float64 `json:"temperature"`                      // 采样温度（0-1）
	MaxToolIterations  int     `json:"maxToolIterations"`                // 工具调用最大迭代次数
	RestrictToWorkspace bool   `json:"restrictToWorkspace,omitempty"`    // 限制文件操作在工作区内
	APIBase            string  `json:"apiBase,omitempty"`                // API 基础 URL
	APIKey             string  `json:"apiKey,omitempty"`                 // API 密钥
}

// AgentConfig 单个 Agent 的配置。
type AgentConfig struct {
	ID        string   `json:"id"`                   // Agent 唯一标识
	Model     string   `json:"model,omitempty"`      // 模型（空则使用 defaults）
	Workspace string   `json:"workspace,omitempty"`  // 工作区（空则使用 defaults）
	Fallbacks []string `json:"fallbacks,omitempty"`  // 备选模型列表
	Default   bool     `json:"default,omitempty"`    // 是否为默认 Agent
	RestrictToWorkspace *bool `json:"restrictToWorkspace,omitempty"`
}

func defaultAgentsConfig() AgentsConfig {
	return AgentsConfig{Defaults: AgentDefaults{
		Workspace:         "~/.simpleclaw/workspace",
		Model:             "anthropic/claude-sonnet-4-20250514",
		MaxTokens:         8192,
		Temperature:       0.7,
		MaxToolIterations: 20,
		RestrictToWorkspace: true,
	}}
}

// ============ 路由绑定 ============

// BindingConfig 描述 Agent 路由绑定规则。
type BindingConfig struct {
	Agent   string `json:"agent"`             // 目标 Agent ID
	Channel string `json:"channel"`           // 渠道名（"*" 表示通配）
	Peer    string `json:"peer,omitempty"`    // 对话方 ID
	Guild   string `json:"guild,omitempty"`   // 群组/服务器 ID
	Team    string `json:"team,omitempty"`    // 团队 ID
	Account string `json:"account,omitempty"` // 账号 ID
}

// ============ 聊天渠道 ============

// ChannelsConfig 包含聊天渠道的配置。
type ChannelsConfig struct {
	Telegram TelegramConfig `json:"telegram"`
	WhatsApp WhatsAppConfig `json:"whatsapp"`
	Discord  DiscordConfig  `json:"discord,omitempty"`
	Feishu   FeishuConfig   `json:"feishu,omitempty"`
}

func defaultChannelsConfig() ChannelsConfig {
	return ChannelsConfig{
		Telegram: TelegramConfig{Enabled: false},
		WhatsApp: WhatsAppConfig{Enabled: false, BridgeURL: "ws://localhost:3001"},
		Discord:  DiscordConfig{Enabled: false},
		Feishu:   FeishuConfig{Enabled: false},
	}
}

// TelegramConfig Telegram 渠道配置。
type TelegramConfig struct {
	Enabled   bool     `json:"enabled"`
	Token     string   `json:"token"`
	AllowFrom []string `json:"allowFrom,omitempty"`
}

// WhatsAppConfig WhatsApp 渠道配置。
type WhatsAppConfig struct {
	Enabled   bool     `json:"enabled"`
	BridgeURL string   `json:"bridgeUrl"`
	AllowFrom []string `json:"allowFrom,omitempty"`
}

// DiscordConfig Discord 渠道配置。
type DiscordConfig struct {
	Enabled   bool   `json:"enabled"`
	Token     string `json:"token,omitempty"`
	GuildID   string `json:"guildId,omitempty"`
	ChannelID string `json:"channelId,omitempty"`
}

// FeishuConfig 飞书 / Lark 渠道配置。
type FeishuConfig struct {
	Enabled   bool   `json:"enabled"`
	AppID     string `json:"appId,omitempty"`
	AppSecret string `json:"appSecret,omitempty"`
}

// ============ LLM 提供商 ============

// ProvidersConfig 包含所有 LLM 提供商的配置。
type ProvidersConfig struct {
	Anthropic  ProviderConfig `json:"anthropic"`
	OpenAI     ProviderConfig `json:"openai"`
	OpenRouter ProviderConfig `json:"openrouter"`
	Groq       ProviderConfig `json:"groq"`
	Zhipu      ProviderConfig `json:"zhipu"`
	VLLM       ProviderConfig `json:"vllm"`
	Gemini     ProviderConfig `json:"gemini"`
	DeepSeek   ProviderConfig `json:"deepseek"`
}

func defaultProvidersConfig() ProvidersConfig {
	return ProvidersConfig{}
}

// ProviderConfig 单个 LLM 提供商的配置。
type ProviderConfig struct {
	APIKey  string `json:"apiKey"`
	APIBase string `json:"apiBase,omitempty"`
}

// ============ 网关 ============

// GatewayConfig 网关/服务器配置。
type GatewayConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// ============ 心跳 ============

// HeartbeatConfig 心跳配置。
type HeartbeatConfig struct {
	Enabled     bool `json:"enabled"`
	IntervalMin int  `json:"intervalMin"` // 间隔（分钟）
}

// ============ 工具 ============

// ToolsConfig 工具配置。
type ToolsConfig struct {
	Web  WebToolsConfig `json:"web"`
	Exec ExecConfig     `json:"exec,omitempty"`
}

func defaultToolsConfig() ToolsConfig {
	return ToolsConfig{
		Web: WebToolsConfig{
			Search: WebSearchConfig{MaxResults: 5, Engine: "auto"},
		},
		Exec: ExecConfig{
			Timeout:    60,
			MaxOutput:  10000,
		},
	}
}

// WebToolsConfig Web 工具配置。
type WebToolsConfig struct {
	Search WebSearchConfig `json:"search"`
}

// WebSearchConfig Web 搜索配置。
type WebSearchConfig struct {
	APIKey     string `json:"apiKey"`
	MaxResults int    `json:"maxResults"`
	Engine     string `json:"engine,omitempty"` // "brave", "duckduckgo", "auto"
}

// ExecConfig Shell 执行工具的安全配置。
type ExecConfig struct {
	Timeout   int  `json:"timeout,omitempty"`   // 超时秒数
	MaxOutput int  `json:"maxOutput,omitempty"` // 最大输出字符数
}


