package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/linkxzhou/SimpleClaw/agent/tools"
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
	bus           MessageBus
	provider      LLMProvider
	workspace     string
	model         string
	maxIterations int
	braveAPIKey   string

	contextBuilder *ContextBuilder
	sessionStore   SessionStore
	stateUpdater   StateUpdater
	tools          *tools.Registry
	subagents      *SubagentManager

	running bool
	mu      sync.Mutex
}

// AgentConfig Agent 配置
type AgentConfig struct {
	Bus           MessageBus
	Provider      LLMProvider
	Workspace     string
	Model         string
	MaxIterations int
	BraveAPIKey   string
	SessionStore  SessionStore  // 可选，nil 时使用内存回退
	StateUpdater  StateUpdater  // 可选，nil 则不通知
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
		bus:            cfg.Bus,
		provider:       cfg.Provider,
		workspace:      cfg.Workspace,
		model:          cfg.Model,
		maxIterations:  cfg.MaxIterations,
		braveAPIKey:    cfg.BraveAPIKey,
		contextBuilder: NewContextBuilder(cfg.Workspace),
		sessionStore:   store,
		stateUpdater:   cfg.StateUpdater,
		tools:          tools.NewRegistry(),
	}

	a.subagents = NewSubagentManager(cfg.Provider, cfg.Workspace, cfg.Bus, cfg.Model, cfg.BraveAPIKey)
	a.registerDefaultTools()

	return a
}

// registerDefaultTools 注册默认工具集
func (a *Agent) registerDefaultTools() {
	// 文件工具
	a.tools.Register(&tools.ReadFileTool{})
	a.tools.Register(&tools.WriteFileTool{})
	a.tools.Register(&tools.EditFileTool{})
	a.tools.Register(&tools.ListDirTool{})

	// Shell 工具
	a.tools.Register(tools.NewExecTool(a.workspace))

	// Web 工具
	a.tools.Register(tools.NewWebSearchTool(a.braveAPIKey))
	a.tools.Register(tools.NewWebFetchTool())

	// Go 动态执行工具
	a.tools.Register(tools.NewGoRunTool())
	a.tools.Register(tools.NewGoAgentTool())

	// 消息工具
	messageTool := tools.NewMessageTool(func(msg tools.OutboundMessage) error {
		return a.bus.PublishOutbound(OutboundMessage{
			Channel: msg.Channel,
			ChatID:  msg.ChatID,
			Content: msg.Content,
		})
	})
	a.tools.Register(messageTool)

	// Spawn 工具
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
					_ = a.bus.PublishOutbound(OutboundMessage{
						Channel: m.Channel,
						ChatID:  m.ChatID,
						Content: fmt.Sprintf("Sorry, I encountered an error: %s", err.Error()),
					})
					return
				}
				if response != nil {
					_ = a.bus.PublishOutbound(*response)
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
	toolDefs := convertToolDefs(a.tools.GetDefinitions())

	for i := 0; i < a.maxIterations; i++ {
		slog.Info("agentLoop iteration start", "iteration", i+1, "maxIterations", a.maxIterations)

		response, err := a.provider.Chat(ctx, messages, toolDefs, a.model)
		if err != nil {
			return "", fmt.Errorf("LLM chat error: %w", err)
		}

		if !response.HasToolCalls {
			contentPreview := response.Content
			if len(contentPreview) > 200 {
				contentPreview = contentPreview[:200] + "..."
			}
			slog.Info("agentLoop final response", "iteration", i+1, "contentLength", len(response.Content), "preview", contentPreview)
			return response.Content, nil
		}

		// 收集工具调用名称用于日志
		toolNames := make([]string, 0, len(response.ToolCalls))
		for _, tc := range response.ToolCalls {
			toolNames = append(toolNames, tc.Name)
		}
		slog.Info("agentLoop tool calls", "iteration", i+1, "count", len(response.ToolCalls), "tools", strings.Join(toolNames, ", "))

		// 添加 assistant 消息（包含 tool calls）
		var toolCallEntries []ToolCallEntry
		for _, tc := range response.ToolCalls {
			toolCallEntries = append(toolCallEntries, ToolCallToEntry(tc))
		}
		messages = a.contextBuilder.AddAssistantMessage(messages, response.Content, toolCallEntries)

		// 执行工具
		for _, tc := range response.ToolCalls {
			slog.Info("agentLoop executing tool", "iteration", i+1, "tool", tc.Name, "args", MarshalToolCallArgs(tc.Arguments))
			result := a.tools.Execute(ctx, tc.Name, tc.Arguments)
			resultPreview := result
			if len(resultPreview) > 300 {
				resultPreview = resultPreview[:300] + "..."
			}
			slog.Info("agentLoop tool result", "iteration", i+1, "tool", tc.Name, "resultLength", len(result), "preview", resultPreview)
			messages = a.contextBuilder.AddToolResult(messages, tc.ID, tc.Name, result)
		}
	}

	slog.Warn("agentLoop reached max iterations", "maxIterations", a.maxIterations)
	return "I've reached the maximum number of iterations.", nil
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
