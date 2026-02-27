package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/linkxzhou/SimpleClaw/agent"
	"github.com/linkxzhou/SimpleClaw/bus"
	"github.com/linkxzhou/SimpleClaw/channels"
	"github.com/linkxzhou/SimpleClaw/config"
	"github.com/linkxzhou/SimpleClaw/cron"
	"github.com/linkxzhou/SimpleClaw/health"
	"github.com/linkxzhou/SimpleClaw/memory"
	"github.com/linkxzhou/SimpleClaw/utils"
)

// cmdGateway 启动完整网关：Agent + Channels + Cron + Heartbeat + Health。
func cmdGateway(args []string) {
	verbose := false
	for _, a := range args {
		if a == "-v" || a == "--verbose" {
			verbose = true
		}
	}

	if verbose {
		utils.InitLog(&utils.LogConfig{
			ConsoleEnabled: true,
			ConsoleLevel:   utils.LogLevelDebug,
			FileEnabled:    true,
			FileLevel:      utils.LogLevelDebug,
			Filename:       "logs/main.log",
			MaxSize:        100,
			MaxBackups:     10,
			MaxAge:         7,
			Compress:       true,
		})
	}

	fmt.Printf("%s Starting SimpleClaw gateway...\n", logo)

	// 加载配置
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Failed to load config: %v\n", err)
		fmt.Fprintln(os.Stderr, "Run 'simpleclaw onboard' first.")
		os.Exit(1)
	}

	// 检查 API Key
	apiKeys := cfg.GetAPIKeys()
	if len(apiKeys) == 0 && cfg.GetAPIKey() == "" {
		fmt.Fprintln(os.Stderr, "[ERROR] No API key configured.")
		fmt.Fprintln(os.Stderr, "Set one in ~/.simpleclaw/config.json under providers.<vendor>.apiKey")
		os.Exit(1)
	}

	logger := slog.Default()
	workspace := cfg.WorkspacePath()
	dataPath, _ := utils.GetDataPath()

	// 创建消息总线
	msgBus := bus.NewMessageBus(256, logger)

	// 创建 LLM Provider（带容错链）
	providerLLM, primaryModel, fallbackModels := buildProviderStack(cfg, logger)

	// 创建 State Manager
	stateMgr := memory.NewStateManager(dataPath)

	// 创建 Session Manager
	sessionsDir := filepath.Join(dataPath, "sessions")
	sessionMgr, err := memory.NewSessionManager(sessionsDir, logger)
	if err != nil {
		logger.Warn("failed to create session manager, using in-memory", "error", err)
	}

	// 创建 Agent
	agentCfg := agent.AgentConfig{
		Bus:           NewBusAdapter(msgBus),
		Provider:      providerLLM,
		Workspace:     workspace,
		Model:         primaryModel,
		MaxIterations: cfg.Agents.Defaults.MaxToolIterations,
		BraveAPIKey:   cfg.Tools.Web.Search.APIKey,
		StateUpdater: func(channel, chatID string) {
			stateMgr.Update(channel, chatID)
		},
	}
	if sessionMgr != nil {
		agentCfg.SessionStore = NewSessionStoreAdapter(sessionMgr, logger)
	}
	agentInstance := agent.NewAgent(agentCfg)

	// 创建 Cron 服务
	cronStorePath := filepath.Join(dataPath, "cron", "jobs.json")
	cronOnJob := func(ctx context.Context, job *cron.Job) (string, error) {
		response, err := agentInstance.ProcessDirect(ctx, job.Payload.Message)
		if err != nil {
			return "", err
		}
		// 投递到频道
		if job.Payload.Deliver && job.Payload.To != "" {
			ch := job.Payload.Channel
			if ch == "" {
				ch = "whatsapp"
			}
			_ = msgBus.PublishOutbound(bus.OutboundMessage{
				Channel: ch,
				ChatID:  job.Payload.To,
				Content: response,
			})
		}
		return response, nil
	}
	cronSvc := cron.NewService(cronStorePath, cronOnJob, logger)

	// 创建 Heartbeat 服务
	heartbeatOnBeat := func(ctx context.Context, prompt string) (string, error) {
		return agentInstance.ProcessDirect(ctx, prompt)
	}
	heartbeatSvc := health.NewHeartbeatService(health.HeartbeatConfig{
		Workspace:   workspace,
		OnHeartbeat: heartbeatOnBeat,
		Interval:    time.Duration(cfg.Heartbeat.IntervalMin) * time.Minute,
		Enabled:     cfg.Heartbeat.Enabled,
		Logger:      logger,
	})

	// 创建 Channel Manager
	channelMgr := channels.NewManager(cfg, msgBus, logger)

	// 创建 Health Server
	healthSvr := health.NewServer(cfg.Gateway.Host, cfg.Gateway.Port, logger)

	// 创建 API Server 并注册路由
	apiSvr := NewAPIServer(agentInstance, sessionMgr, cronSvc, channelMgr, msgBus, cfg, logger)
	apiSvr.RegisterRoutes(healthSvr)

	// 打印启动信息
	enabledChannels := channelMgr.EnabledChannels()
	if len(enabledChannels) > 0 {
		fmt.Printf("  ✓ Channels enabled: %v\n", enabledChannels)
	} else {
		fmt.Println("  [WARN] No channels enabled")
	}

	cronStatus := cronSvc.Status()
	if jobs, ok := cronStatus["jobs"].(int); ok && jobs > 0 {
		fmt.Printf("  ✓ Cron: %d scheduled jobs\n", jobs)
	}
	if cfg.Heartbeat.Enabled {
		fmt.Printf("  ✓ Heartbeat: every %dm\n", cfg.Heartbeat.IntervalMin)
	}
	fmt.Printf("  ✓ Health: http://%s:%d/health\n", cfg.Gateway.Host, cfg.Gateway.Port)
	fmt.Printf("  ✓ API:    http://%s:%d/api/rpc\n", cfg.Gateway.Host, cfg.Gateway.Port)
	fmt.Printf("  ✓ Web UI: http://%s:%d/\n", cfg.Gateway.Host, cfg.Gateway.Port)
	fmt.Printf("  ✓ Model: %s\n", primaryModel)
	if len(fallbackModels) > 0 {
		fmt.Printf("  ✓ Fallbacks: %v\n", fallbackModels)
	}

	// 启动所有服务
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 优雅关闭
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		heartbeatSvc.Stop()
		cronSvc.Stop()
		agentInstance.Stop()
		channelMgr.StopAll()
		msgBus.Stop()
		cancel()
	}()

	// 启动 Health Server（非阻塞）
	go func() {
		if err := healthSvr.Start(ctx); err != nil {
			logger.Error("health server error", "error", err)
		}
	}()

	// 启动 Cron 和 Heartbeat（非阻塞）
	go cronSvc.Start(ctx)
	go heartbeatSvc.Start(ctx)

	// 启动 Channel Manager（非阻塞）
	go func() {
		if err := channelMgr.StartAll(ctx); err != nil {
			slog.Error("Channel manager error", "error", err)
		}
	}()

	// 标记就绪
	healthSvr.SetReady(true)

	fmt.Printf("\n%s SimpleClaw gateway is running. Press Ctrl+C to stop.\n", logo)

	// 启动 Agent 主循环（阻塞）
	if err := agentInstance.Run(ctx); err != nil {
		slog.Error("Agent error", "error", err)
	}
}
