package main

import (
	"fmt"
	"os"

	"github.com/linkxzhou/SimpleClaw/config"
)

// cmdStatus 显示 SimpleClaw 系统状态。
func cmdStatus(args []string) {
	configPath := config.GetConfigPath()
	fmt.Printf("%s SimpleClaw Status\n\n", logo)

	// 配置文件
	configExists := false
	if _, err := os.Stat(configPath); err == nil {
		configExists = true
	}
	fmt.Printf("Config:    %s %s\n", configPath, checkMark(configExists))

	if !configExists {
		fmt.Println("\nRun 'simpleclaw onboard' to initialize.")
		return
	}

	// 加载配置
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Failed to load config: %v\n", err)
		return
	}

	// 工作区
	workspace := cfg.WorkspacePath()
	wsExists := false
	if _, err := os.Stat(workspace); err == nil {
		wsExists = true
	}
	fmt.Printf("Workspace: %s %s\n", workspace, checkMark(wsExists))

	// 模型
	fmt.Printf("Model:     %s\n", cfg.Agents.Defaults.Model)

	fmt.Println()

	// API Keys 状态
	fmt.Println("Providers:")
	printKeyStatus("  OpenRouter", cfg.Providers.OpenRouter.APIKey)
	printKeyStatus("  Anthropic ", cfg.Providers.Anthropic.APIKey)
	printKeyStatus("  OpenAI    ", cfg.Providers.OpenAI.APIKey)
	printKeyStatus("  Gemini    ", cfg.Providers.Gemini.APIKey)
	printKeyStatus("  Zhipu     ", cfg.Providers.Zhipu.APIKey)
	printKeyStatus("  Groq      ", cfg.Providers.Groq.APIKey)

	if cfg.Providers.VLLM.APIBase != "" {
		fmt.Printf("  vLLM/Local  ✓ %s\n", cfg.Providers.VLLM.APIBase)
	} else {
		fmt.Println("  vLLM/Local  not set")
	}

	fmt.Println()

	// Channels 状态
	fmt.Println("Channels:")
	printChannelStatus("  Telegram  ", cfg.Channels.Telegram.Enabled)
	printChannelStatus("  Discord   ", cfg.Channels.Discord.Enabled)
	printChannelStatus("  WhatsApp  ", cfg.Channels.WhatsApp.Enabled)
	printChannelStatus("  Feishu    ", cfg.Channels.Feishu.Enabled)
}

// cmdChannels 管理 channels（简化为状态显示）。
func cmdChannels(args []string) {
	if len(args) == 0 || args[0] == "status" {
		cmdStatus(nil) // 复用 status 的 channels 部分
		return
	}

	fmt.Fprintf(os.Stderr, "Unknown channels subcommand: %s\n", args[0])
	fmt.Println("Usage: simpleclaw channels [status]")
}

// checkMark 返回状态标记符号：✓ 或 ✗。
func checkMark(ok bool) string {
	if ok {
		return "✓"
	}
	return "✗"
}

// printKeyStatus 输出 API Key 的配置状态。
func printKeyStatus(label, key string) {
	if key != "" {
		fmt.Printf("%s  ✓ (configured)\n", label)
	} else {
		fmt.Printf("%s  not set\n", label)
	}
}

// printChannelStatus 输出渠道启用状态。
func printChannelStatus(label string, enabled bool) {
	if enabled {
		fmt.Printf("%s  enabled\n", label)
	} else {
		fmt.Printf("%s  disabled\n", label)
	}
}
