package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/linkxzhou/SimpleClaw/agent"
	"github.com/linkxzhou/SimpleClaw/bus"
	"github.com/linkxzhou/SimpleClaw/config"
	"github.com/linkxzhou/SimpleClaw/memory"
	"github.com/linkxzhou/SimpleClaw/utils"
)

// cmdAgent 与 Agent 交互：单消息模式或交互式 REPL。
func cmdAgent(args []string) {
	var message string
	var sessionID string

	// 解析参数
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-m", "--message":
			if i+1 < len(args) {
				i++
				message = args[i]
			}
		case "-s", "--session":
			if i+1 < len(args) {
				i++
				sessionID = args[i]
			}
		}
	}

	if sessionID == "" {
		sessionID = "cli:default"
	}

	// 加载配置
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Failed to load config: %v\n", err)
		fmt.Fprintln(os.Stderr, "Run 'simpleclaw onboard' first.")
		os.Exit(1)
	}

	apiKeys := cfg.GetAPIKeys()
	if len(apiKeys) == 0 && cfg.GetAPIKey() == "" {
		fmt.Fprintln(os.Stderr, "[ERROR] No API key configured.")
		os.Exit(1)
	}

	logger := slog.Default()

	// 创建 LLM Provider（带容错链）
	providerLLM, primaryModel, _ := buildProviderStack(cfg, logger)

	// 创建组件
	msgBus := bus.NewMessageBus(64, nil)

	// Session Manager（可选持久化）
	dataPath, _ := utils.GetDataPath()
	sessionsDir := filepath.Join(dataPath, "sessions")
	var sessionStore agent.SessionStore
	if sessionMgr, err := memory.NewSessionManager(sessionsDir, logger); err == nil {
		sessionStore = NewSessionStoreAdapter(sessionMgr, logger)
	}

	agentInstance := agent.NewAgent(agent.AgentConfig{
		Bus:           NewBusAdapter(msgBus),
		Provider:      providerLLM,
		Workspace:     cfg.WorkspacePath(),
		Model:         primaryModel,
		MaxIterations: cfg.Agents.Defaults.MaxToolIterations,
		BraveAPIKey:   cfg.Tools.Web.Search.APIKey,
		SessionStore:  sessionStore,
	})

	ctx := context.Background()

	if message != "" {
		// 单消息模式
		response, err := agentInstance.ProcessDirect(ctx, message)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\n%s %s\n", logo, response)
	} else {
		// 交互模式 (REPL)
		fmt.Printf("%s Interactive mode (type 'exit' or Ctrl+C to quit)\n\n", logo)

		scanner := bufio.NewScanner(os.Stdin)
		for {
			fmt.Print("You: ")
			if !scanner.Scan() {
				break
			}
			input := strings.TrimSpace(scanner.Text())
			if input == "" {
				continue
			}
			if input == "exit" || input == "quit" {
				fmt.Println("Goodbye!")
				break
			}

			response, err := agentInstance.ProcessDirect(ctx, input)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[ERROR] %v\n", err)
				continue
			}
			fmt.Printf("\n%s %s\n\n", logo, response)
		}
	}
}
