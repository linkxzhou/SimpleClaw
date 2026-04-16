package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/linkxzhou/SimpleClaw/config"
	"github.com/linkxzhou/SimpleClaw/providers"
	"github.com/linkxzhou/SimpleClaw/utils"
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

	// 费用统计
	if cfg.Cost.Enabled {
		fmt.Println()
		printCostStatus(cfg)
	}
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

// printCostStatus 输出费用追踪状态。
func printCostStatus(cfg *config.Config) {
	fmt.Println("Cost:")
	dataPath, err := utils.GetDataPath()
	if err != nil {
		fmt.Println("  (unable to read cost data)")
		return
	}
	costCfg := providers.CostConfig{
		Enabled:         cfg.Cost.Enabled,
		DailyLimitUSD:   cfg.Cost.DailyLimitUSD,
		MonthlyLimitUSD: cfg.Cost.MonthlyLimitUSD,
		WarnAtPercent:   cfg.Cost.WarnAtPercent,
	}
	ct, err := providers.NewCostTracker(costCfg, dataPath, nil)
	if err != nil {
		fmt.Println("  (unable to init cost tracker)")
		return
	}
	summary := ct.GetSummary()

	dailyPct := float64(0)
	if summary.DailyLimitUSD > 0 {
		dailyPct = summary.DailyCostUSD / summary.DailyLimitUSD * 100
	}
	monthlyPct := float64(0)
	if summary.MonthlyLimitUSD > 0 {
		monthlyPct = summary.MonthlyCostUSD / summary.MonthlyLimitUSD * 100
	}

	fmt.Printf("  Today:  $%.2f / $%.2f (%.1f%%)\n", summary.DailyCostUSD, summary.DailyLimitUSD, dailyPct)
	fmt.Printf("  Month:  $%.2f / $%.2f (%.1f%%)\n", summary.MonthlyCostUSD, summary.MonthlyLimitUSD, monthlyPct)

	if len(summary.ModelBreakdown) > 0 {
		fmt.Println("  Top models:")
		type modelCost struct {
			name string
			cost float64
		}
		var models []modelCost
		for name, cost := range summary.ModelBreakdown {
			models = append(models, modelCost{name, cost})
		}
		sort.Slice(models, func(i, j int) bool { return models[i].cost > models[j].cost })
		total := summary.MonthlyCostUSD
		if total == 0 {
			total = 1 // 避免除零
		}
		for i, m := range models {
			if i >= 5 {
				break
			}
			fmt.Printf("    %-35s $%.2f (%.1f%%)\n", m.name, m.cost, m.cost/total*100)
		}
	}
}
