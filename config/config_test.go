package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/linkxzhou/SimpleClaw/utils"
)

// ---------------------------------------------------------------------------
// DefaultConfig 测试
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig 不应返回 nil")
	}
	if cfg.Agents.Defaults.Model != "anthropic/claude-sonnet-4-20250514" {
		t.Errorf("期望默认模型 anthropic/claude-sonnet-4-20250514，实际 %q", cfg.Agents.Defaults.Model)
	}
	if cfg.Agents.Defaults.MaxTokens != 8192 {
		t.Errorf("期望 maxTokens=8192，实际 %d", cfg.Agents.Defaults.MaxTokens)
	}
	if cfg.Agents.Defaults.Temperature != 0.7 {
		t.Errorf("期望 temperature=0.7，实际 %f", cfg.Agents.Defaults.Temperature)
	}
	if cfg.Agents.Defaults.MaxToolIterations != 20 {
		t.Errorf("期望 maxToolIterations=20，实际 %d", cfg.Agents.Defaults.MaxToolIterations)
	}
	if cfg.Gateway.Host != "0.0.0.0" {
		t.Errorf("期望 host=0.0.0.0，实际 %q", cfg.Gateway.Host)
	}
	if cfg.Gateway.Port != 18790 {
		t.Errorf("期望 port=18790，实际 %d", cfg.Gateway.Port)
	}
	if cfg.Tools.Web.Search.MaxResults != 5 {
		t.Errorf("期望 maxResults=5，实际 %d", cfg.Tools.Web.Search.MaxResults)
	}
}

func TestDefaultConfigChannels(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Channels.Telegram.Enabled {
		t.Error("Telegram 默认应禁用")
	}
	if cfg.Channels.WhatsApp.Enabled {
		t.Error("WhatsApp 默认应禁用")
	}
	if cfg.Channels.WhatsApp.BridgeURL != "ws://localhost:3001" {
		t.Errorf("期望 BridgeURL=ws://localhost:3001，实际 %q", cfg.Channels.WhatsApp.BridgeURL)
	}
}

// ---------------------------------------------------------------------------
// GetAPIKey 测试
// ---------------------------------------------------------------------------

func TestGetAPIKeyEmpty(t *testing.T) {
	cfg := DefaultConfig()
	if key := cfg.GetAPIKey(); key != "" {
		t.Errorf("默认配置无 key 应返回空，实际 %q", key)
	}
}

func TestGetAPIKeyPriority(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Providers.OpenAI.APIKey = "openai-key"
	cfg.Providers.Anthropic.APIKey = "anthropic-key"
	cfg.Providers.OpenRouter.APIKey = "openrouter-key"

	// OpenRouter 优先级最高（providers 中）
	if key := cfg.GetAPIKey(); key != "openrouter-key" {
		t.Errorf("期望 OpenRouter key，实际 %q", key)
	}

	// agents.defaults.apiKey 优先级高于所有 providers
	cfg.Agents.Defaults.APIKey = "agent-key"
	if key := cfg.GetAPIKey(); key != "agent-key" {
		t.Errorf("期望 agent-key（agents.defaults 优先），实际 %q", key)
	}

	cfg.Agents.Defaults.APIKey = ""
	cfg.Providers.OpenRouter.APIKey = ""
	if key := cfg.GetAPIKey(); key != "anthropic-key" {
		t.Errorf("期望 Anthropic key，实际 %q", key)
	}

	cfg.Providers.Anthropic.APIKey = ""
	if key := cfg.GetAPIKey(); key != "openai-key" {
		t.Errorf("期望 OpenAI key，实际 %q", key)
	}
}

func TestGetAPIKeyAllProviders(t *testing.T) {
	providers := []struct {
		name   string
		setKey func(*Config)
	}{
		{"Gemini", func(c *Config) { c.Providers.Gemini.APIKey = "gemini-key" }},
		{"Zhipu", func(c *Config) { c.Providers.Zhipu.APIKey = "zhipu-key" }},
		{"Groq", func(c *Config) { c.Providers.Groq.APIKey = "groq-key" }},
		{"VLLM", func(c *Config) { c.Providers.VLLM.APIKey = "vllm-key" }},
	}
	for _, p := range providers {
		t.Run(p.name, func(t *testing.T) {
			cfg := DefaultConfig()
			p.setKey(cfg)
			if key := cfg.GetAPIKey(); key == "" {
				t.Errorf("%s key 未被检测到", p.name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetAPIBase 测试
// ---------------------------------------------------------------------------

func TestGetAPIBaseEmpty(t *testing.T) {
	cfg := DefaultConfig()
	if base := cfg.GetAPIBase(); base != "" {
		t.Errorf("默认配置应返回空，实际 %q", base)
	}
}

func TestGetAPIBaseAgentDefaults(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agents.Defaults.APIBase = "https://agent-custom.api/v1"
	// agents.defaults.apiBase 优先级最高
	if base := cfg.GetAPIBase(); base != "https://agent-custom.api/v1" {
		t.Errorf("期望 agent-custom base，实际 %q", base)
	}

	// 即使 providers 也有配置，agents.defaults 仍然优先
	cfg.Providers.OpenRouter.APIKey = "key"
	cfg.Providers.OpenRouter.APIBase = "https://openrouter-custom/v1"
	if base := cfg.GetAPIBase(); base != "https://agent-custom.api/v1" {
		t.Errorf("agents.defaults.apiBase 应优先于 providers，实际 %q", base)
	}
}

func TestGetAPIBaseOpenRouter(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Providers.OpenRouter.APIKey = "key"
	if base := cfg.GetAPIBase(); base != "https://openrouter.ai/api/v1" {
		t.Errorf("期望默认 OpenRouter base，实际 %q", base)
	}

	cfg.Providers.OpenRouter.APIBase = "https://custom.api/v1"
	if base := cfg.GetAPIBase(); base != "https://custom.api/v1" {
		t.Errorf("期望自定义 base，实际 %q", base)
	}
}

func TestGetAPIBaseZhipu(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Providers.Zhipu.APIKey = "key"
	cfg.Providers.Zhipu.APIBase = "https://zhipu.api/v1"
	if base := cfg.GetAPIBase(); base != "https://zhipu.api/v1" {
		t.Errorf("期望智谱 base，实际 %q", base)
	}
}

func TestGetAPIBaseVLLM(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Providers.VLLM.APIBase = "http://localhost:8000/v1"
	if base := cfg.GetAPIBase(); base != "http://localhost:8000/v1" {
		t.Errorf("期望 VLLM base，实际 %q", base)
	}
}

// ---------------------------------------------------------------------------
// WorkspacePath 测试
// ---------------------------------------------------------------------------

func TestWorkspacePathExpand(t *testing.T) {
	cfg := DefaultConfig()
	path := cfg.WorkspacePath()
	// 默认路径含 ~ ，展开后不应以 ~ 开头
	if len(path) > 0 && path[0] == '~' {
		t.Errorf("~ 应被展开，实际 %q", path)
	}
}

func TestWorkspacePathAbsolute(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agents.Defaults.Workspace = "/absolute/path"
	if path := cfg.WorkspacePath(); path != "/absolute/path" {
		t.Errorf("绝对路径不应被修改，实际 %q", path)
	}
}

// ---------------------------------------------------------------------------
// ExpandHome 测试（已迁移到 utils 包）
// ---------------------------------------------------------------------------

func TestExpandHomeTilde(t *testing.T) {
	result := utils.ExpandHome("~/documents")
	if result[0] == '~' {
		t.Errorf("~ 应被展开，实际 %q", result)
	}
	if !filepath.IsAbs(result) {
		t.Errorf("结果应为绝对路径，实际 %q", result)
	}
}

func TestExpandHomeNoTilde(t *testing.T) {
	result := utils.ExpandHome("/absolute/path")
	if result != "/absolute/path" {
		t.Errorf("无 ~ 不应修改，实际 %q", result)
	}
}

func TestExpandHomeEmpty(t *testing.T) {
	result := utils.ExpandHome("")
	if result != "" {
		t.Errorf("空字符串不应修改，实际 %q", result)
	}
}

func TestExpandHomeSingleChar(t *testing.T) {
	result := utils.ExpandHome("a")
	if result != "a" {
		t.Errorf("单字符不应修改，实际 %q", result)
	}
}

// ---------------------------------------------------------------------------
// Load 测试
// ---------------------------------------------------------------------------

func TestLoadFileNotExist(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatalf("文件不存在应返回默认配置，不应出错: %v", err)
	}
	if cfg == nil {
		t.Fatal("配置不应为 nil")
	}
	if cfg.Gateway.Port != 18790 {
		t.Errorf("期望默认端口 18790，实际 %d", cfg.Gateway.Port)
	}
}

func TestLoadValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := DefaultConfig()
	cfg.Gateway.Port = 9999
	cfg.Providers.OpenAI.APIKey = "test-key"
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(path, data, 0644)

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if loaded.Gateway.Port != 9999 {
		t.Errorf("期望端口 9999，实际 %d", loaded.Gateway.Port)
	}
	if loaded.Providers.OpenAI.APIKey != "test-key" {
		t.Errorf("期望 APIKey=test-key，实际 %q", loaded.Providers.OpenAI.APIKey)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte("{not json}"), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("无效 JSON 应返回默认配置: %v", err)
	}
	if cfg.Gateway.Port != 18790 {
		t.Errorf("无效 JSON 应使用默认值")
	}
}

func TestLoadPartialJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"gateway":{"port":12345}}`), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if cfg.Gateway.Port != 12345 {
		t.Errorf("期望端口 12345，实际 %d", cfg.Gateway.Port)
	}
}

// ---------------------------------------------------------------------------
// Save 测试
// ---------------------------------------------------------------------------

func TestSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "config.json")

	cfg := DefaultConfig()
	cfg.Gateway.Port = 8080

	if err := Save(cfg, path); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	// 验证文件内容
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取文件失败: %v", err)
	}
	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("解析 JSON 失败: %v", err)
	}
	if loaded.Gateway.Port != 8080 {
		t.Errorf("期望端口 8080，实际 %d", loaded.Gateway.Port)
	}
}

func TestSaveCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "config.json")

	if err := Save(DefaultConfig(), path); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("文件应被创建")
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	original := DefaultConfig()
	original.Providers.Anthropic.APIKey = "anthropic-key-123"
	original.Agents.Defaults.Temperature = 0.5
	original.Agents.Defaults.APIBase = "https://custom.api/v1"
	original.Agents.Defaults.APIKey = "agent-direct-key"
	original.Channels.Telegram.Enabled = true
	original.Channels.Telegram.Token = "tg-token"

	if err := Save(original, path); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}

	if loaded.Providers.Anthropic.APIKey != "anthropic-key-123" {
		t.Error("Anthropic key 不匹配")
	}
	if loaded.Agents.Defaults.Temperature != 0.5 {
		t.Error("Temperature 不匹配")
	}
	if loaded.Agents.Defaults.APIBase != "https://custom.api/v1" {
		t.Error("APIBase 不匹配")
	}
	if loaded.Agents.Defaults.APIKey != "agent-direct-key" {
		t.Error("APIKey 不匹配")
	}
	if !loaded.Channels.Telegram.Enabled {
		t.Error("Telegram 应启用")
	}
	if loaded.Channels.Telegram.Token != "tg-token" {
		t.Error("Telegram token 不匹配")
	}
}

// ---------------------------------------------------------------------------
// JSON 序列化测试
// ---------------------------------------------------------------------------

func TestConfigJSONRoundTrip(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Providers.OpenAI.APIKey = "sk-test"
	cfg.Channels.WhatsApp.AllowFrom = []string{"user1", "user2"}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if loaded.Providers.OpenAI.APIKey != "sk-test" {
		t.Error("APIKey 不匹配")
	}
	if len(loaded.Channels.WhatsApp.AllowFrom) != 2 {
		t.Error("AllowFrom 长度不匹配")
	}
}
