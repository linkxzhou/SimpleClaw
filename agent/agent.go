package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleClaw/agent/tools"
	"github.com/linkxzhou/SimpleClaw/memory"
	"github.com/linkxzhou/SimpleClaw/providers"
	"github.com/linkxzhou/SimpleClaw/utils"
)

// SessionStore 定义外部 session 存储接口（由 cmd 层注入实现）。
type SessionStore interface {
	// GetOrCreateSession 获取或创建指定 key 的 session 消息历史。
	GetOrCreateSession(key string) []Message
	// SaveSession 保存 session 消息历史。
	SaveSession(key string, messages []Message)
}

// StateUpdater 通知状态层最近活跃信息的回调。
type StateUpdater func(channel, chatID string)

// Agent 核心处理引擎
type Agent struct {
	bus             MessageBus
	provider        LLMProvider
	workspace       string
	model           string
	maxIterations   int
	maxContextRunes int
	maxTokens       int
	braveAPIKey     string

	contextBuilder *ContextBuilder
	sessionStore   SessionStore
	stateUpdater   StateUpdater
	historyStore   *memory.HistoryStore // 对话历史（Dream 消费）
	tools          *tools.Registry
	subagents      *SubagentManager

	running bool
	mu      sync.Mutex
}

// AgentConfig Agent 配置
type AgentConfig struct {
	Bus              MessageBus
	Provider         LLMProvider
	Workspace        string
	Model            string
	MaxIterations    int
	MaxContextRunes  int              // 单条消息最大 rune 数（0=auto, -1=不限）
	MaxTokens        int              // 最大生成 token 数（用于计算上下文预算）
	BraveAPIKey      string
	SessionStore     SessionStore         // 可选，nil 时使用内存回退
	StateUpdater     StateUpdater         // 可选，nil 则不通知
	ApprovalManager  *ApprovalManager     // 可选，nil 时不做审批
	HistoryStore     *memory.HistoryStore // 可选，nil 时不记录对话历史
}

// NewAgent 创建 Agent 实例
func NewAgent(cfg AgentConfig) *Agent {
	if cfg.MaxIterations == 0 {
		cfg.MaxIterations = 20
	}
	if cfg.Model == "" {
		cfg.Model = cfg.Provider.GetDefaultModel()
	}

	store := cfg.SessionStore
	if store == nil {
		store = newMemorySessionStore()
	}

	a := &Agent{
		bus:             cfg.Bus,
		provider:        cfg.Provider,
		workspace:       cfg.Workspace,
		model:           cfg.Model,
		maxIterations:   cfg.MaxIterations,
		maxContextRunes: cfg.MaxContextRunes,
		maxTokens:       cfg.MaxTokens,
		braveAPIKey:     cfg.BraveAPIKey,
		contextBuilder:  NewContextBuilder(cfg.Workspace),
		sessionStore:    store,
		stateUpdater:    cfg.StateUpdater,
		historyStore:    cfg.HistoryStore,
		tools:           tools.NewRegistry(),
	}

	a.subagents = NewSubagentManager(cfg.Provider, cfg.Workspace, cfg.Bus, cfg.Model, cfg.BraveAPIKey)
	a.registerDefaultTools()

	// 设置审批检查器
	if cfg.ApprovalManager != nil {
		a.tools.SetApprovalChecker(cfg.ApprovalManager.AsChecker())
	}

	return a
}

// registerDefaultTools 注册默认工具集
func (a *Agent) registerDefaultTools() {
	// 注册主 Agent 与子 Agent 共用的通用工具
	tools.RegisterCommonTools(a.tools, a.workspace, a.braveAPIKey)

	// 仅主 Agent 可用的工具（消息发送 + 子 Agent 生成）
	messageTool := tools.NewMessageTool(func(msg tools.OutboundMessage) error {
		return a.bus.PublishOutbound(OutboundMessage{
			Channel: msg.Channel,
			ChatID:  msg.ChatID,
			Content: msg.Content,
		})
	})
	a.tools.Register(messageTool)

	spawnTool := tools.NewSpawnTool(a.subagents)
	a.tools.Register(spawnTool)
}

// RegisterTool 注册自定义工具
func (a *Agent) RegisterTool(t tools.Tool) {
	a.tools.Register(t)
}

// Run 启动 Agent 主循环
func (a *Agent) Run(ctx context.Context) error {
	a.mu.Lock()
	a.running = true
	a.mu.Unlock()

	slog.Info("Agent loop started")

	for {
		select {
		case <-ctx.Done():
			a.mu.Lock()
			a.running = false
			a.mu.Unlock()
			slog.Info("Agent loop stopping")
			return nil
		default:
			msg, err := a.bus.ConsumeInbound(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				continue
			}

			go func(m InboundMessage) {
				response, err := a.processMessage(ctx, m)
				if err != nil {
					slog.Error("Error processing message", "sender", m.SenderID, "error", err)
					if pubErr := a.bus.PublishOutbound(OutboundMessage{
						Channel: m.Channel,
						ChatID:  m.ChatID,
						Content: fmt.Sprintf("Sorry, I encountered an error: %s", err.Error()),
					}); pubErr != nil {
						slog.Error("Failed to publish error response", "channel", m.Channel, "error", pubErr)
					}
					return
				}
				if response != nil {
					if pubErr := a.bus.PublishOutbound(*response); pubErr != nil {
						slog.Error("Failed to publish response", "channel", m.Channel, "error", pubErr)
					}
				}
			}(msg)
		}
	}
}

// Stop 停止 Agent
func (a *Agent) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.running = false
	slog.Info("Agent loop stopping")
}

// ProcessDirect 直接处理消息（用于 CLI 模式）
func (a *Agent) ProcessDirect(ctx context.Context, content string) (string, error) {
	msg := InboundMessage{
		Channel:  "cli",
		SenderID: "user",
		ChatID:   "direct",
		Content:  content,
	}

	response, err := a.processMessage(ctx, msg)
	if err != nil {
		return "", err
	}
	if response == nil {
		return "", nil
	}
	return response.Content, nil
}

// processMessage 处理单条入站消息
func (a *Agent) processMessage(ctx context.Context, msg InboundMessage) (*OutboundMessage, error) {
	// 处理系统消息（子 Agent 结果回报）
	if msg.Channel == "system" {
		return a.processSystemMessage(ctx, msg)
	}

	slog.Info("Processing message", "channel", msg.Channel, "sender", msg.SenderID)

	// 通知 state 层最近活跃信息
	if a.stateUpdater != nil && msg.Channel != "cli" {
		a.stateUpdater(msg.Channel, msg.ChatID)
	}

	// 获取或创建 Session
	sessionKey := msg.SessionKey()
	history := a.sessionStore.GetOrCreateSession(sessionKey)

	// 更新工具上下文
	a.updateToolContexts(msg.Channel, msg.ChatID)

	// 构建初始消息
	messages := a.contextBuilder.BuildMessages(
		history,
		msg.Content,
		msg.Media,
	)

	// 智能上下文截断：按 token 预算截断，防止上下文窗口溢出
	messages = a.truncateIfNeeded(messages)

	// Agent 循环
	finalContent, err := a.agentLoop(ctx, messages)
	if err != nil {
		return nil, err
	}

	if finalContent == "" {
		finalContent = "I've completed processing but have no response to give."
	}

	// 保存到 Session
	history = append(history, Message{Role: "user", Content: msg.Content})
	history = append(history, Message{Role: "assistant", Content: finalContent})
	// 保留最近 50 条
	if len(history) > 50 {
		history = history[len(history)-50:]
	}
	a.sessionStore.SaveSession(sessionKey, history)

	// 异步记录对话历史（供 Dream 消费）
	if a.historyStore != nil {
		go func() {
			a.historyStore.Append(memory.HistoryEntry{
				Timestamp:  time.Now().Format(time.RFC3339),
				SessionKey: sessionKey,
				UserMsg:    truncStr(msg.Content, 500),
				AssistMsg:  truncStr(finalContent, 500),
				Summary:    truncStr(msg.Content+" → "+finalContent, 300),
			})
		}()
	}

	return &OutboundMessage{
		Channel: msg.Channel,
		ChatID:  msg.ChatID,
		Content: finalContent,
	}, nil
}

// processSystemMessage 处理系统消息（子 Agent 结果回报）
func (a *Agent) processSystemMessage(ctx context.Context, msg InboundMessage) (*OutboundMessage, error) {
	slog.Info("Processing system message", "sender", msg.SenderID)

	originChannel := "cli"
	originChatID := msg.ChatID
	if idx := strings.Index(msg.ChatID, ":"); idx >= 0 {
		originChannel = msg.ChatID[:idx]
		originChatID = msg.ChatID[idx+1:]
	}

	sessionKey := fmt.Sprintf("%s:%s", originChannel, originChatID)
	history := a.sessionStore.GetOrCreateSession(sessionKey)

	a.updateToolContexts(originChannel, originChatID)

	messages := a.contextBuilder.BuildMessages(
		history,
		msg.Content,
		nil,
	)

	// 智能上下文截断
	messages = a.truncateIfNeeded(messages)

	finalContent, err := a.agentLoop(ctx, messages)
	if err != nil {
		return nil, err
	}

	if finalContent == "" {
		finalContent = "Background task completed."
	}

	history = append(history, Message{Role: "user", Content: fmt.Sprintf("[System: %s] %s", msg.SenderID, msg.Content)})
	history = append(history, Message{Role: "assistant", Content: finalContent})
	if len(history) > 50 {
		history = history[len(history)-50:]
	}
	a.sessionStore.SaveSession(sessionKey, history)

	return &OutboundMessage{
		Channel: originChannel,
		ChatID:  originChatID,
		Content: finalContent,
	}, nil
}

// agentLoop 核心 ReAct 循环
func (a *Agent) agentLoop(ctx context.Context, messages []Message) (string, error) {
	return RunLoop(ctx, LoopConfig{
		Provider:      a.provider,
		Model:         a.model,
		Registry:      a.tools,
		MaxIterations: a.maxIterations,
		Logger:        slog.Default(),
	}, messages)
}

// truncateIfNeeded 按模型上下文窗口智能截断消息列表。
// 同时对单条超长消息执行 rune 级截断（最后防线）。
func (a *Agent) truncateIfNeeded(messages []Message) []Message {
	// 1. 单条消息 rune 级截断（最后防线）
	maxRunes := a.maxContextRunes
	if maxRunes == 0 {
		// auto: 按上下文窗口大小推算（约 4 char/token）
		ctxWindow := providers.GetContextWindow(a.model)
		maxRunes = ctxWindow * 4
	}
	if maxRunes > 0 { // -1 表示不限
		for i := range messages {
			if messages[i].Role == "system" {
				continue // 不截断 system prompt
			}
			messages[i].Content = utils.TruncateContent(messages[i].Content, maxRunes)
		}
	}

	// 2. 计算 token 预算
	contextWindow := providers.GetContextWindow(a.model)
	maxCompletion := a.maxTokens
	if maxCompletion <= 0 {
		maxCompletion = 8192
	}
	safetyBuffer := 1000
	budget := contextWindow - maxCompletion - safetyBuffer
	if budget <= 0 {
		return messages // 无法截断
	}

	// 3. 转换为截断算法的消息格式
	truncMsgs := make([]memory.TruncMessage, len(messages))
	for i, m := range messages {
		truncMsgs[i] = memory.TruncMessage{Role: m.Role, Content: m.Content}
	}

	// 4. 执行截断
	truncated, archivedCount := memory.TruncateMessages(truncMsgs, budget)
	if archivedCount > 0 {
		slog.Info("Context truncated",
			"archived", archivedCount,
			"before", len(messages),
			"after", len(truncated),
			"model", a.model,
			"budget", budget,
		)
	}

	// 5. 转换回 Message 格式
	result := make([]Message, len(truncated))
	for i, tm := range truncated {
		result[i] = Message{Role: tm.Role, Content: tm.Content}
	}
	return result
}

// updateToolContexts 更新工具的上下文信息
func (a *Agent) updateToolContexts(channel, chatID string) {
	if t, ok := a.tools.Get("message"); ok {
		if mt, ok := t.(*tools.MessageTool); ok {
			mt.SetContext(channel, chatID)
		}
	}
	if t, ok := a.tools.Get("spawn"); ok {
		if st, ok := t.(*tools.SpawnTool); ok {
			st.SetContext(channel, chatID)
		}
	}
}

// convertToolDefs 将 tools.ToolDef 转换为 agent.ToolDef
func convertToolDefs(defs []tools.ToolDef) []ToolDef {
	result := make([]ToolDef, len(defs))
	for i, d := range defs {
		result[i] = ToolDef{
			Type: d.Type,
			Function: ToolFunction{
				Name:        d.Function.Name,
				Description: d.Function.Description,
				Parameters:  d.Function.Parameters,
			},
		}
	}
	return result
}

// ============ 内存回退 SessionStore ============

type memorySessionStore struct {
	mu       sync.RWMutex
	sessions map[string][]Message
}

func newMemorySessionStore() *memorySessionStore {
	return &memorySessionStore{sessions: make(map[string][]Message)}
}

func (s *memorySessionStore) GetOrCreateSession(key string) []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if msgs, ok := s.sessions[key]; ok {
		cp := make([]Message, len(msgs))
		copy(cp, msgs)
		return cp
	}
	return nil
}

func (s *memorySessionStore) SaveSession(key string, messages []Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]Message, len(messages))
	copy(cp, messages)
	s.sessions[key] = cp
}

// truncStr 截断字符串到指定长度。
func truncStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
