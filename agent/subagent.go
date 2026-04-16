package agent

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"sync"

	"github.com/linkxzhou/SimpleClaw/agent/tools"
)

// SubagentManager 管理后台子 Agent 的执行
type SubagentManager struct {
	provider     LLMProvider
	workspace    string
	bus          MessageBus
	model        string
	braveAPIKey  string
	mu           sync.Mutex
	runningTasks map[string]context.CancelFunc
}

// NewSubagentManager 创建子 Agent 管理器
func NewSubagentManager(
	provider LLMProvider,
	workspace string,
	bus MessageBus,
	model string,
	braveAPIKey string,
) *SubagentManager {
	return &SubagentManager{
		provider:     provider,
		workspace:    workspace,
		bus:          bus,
		model:        model,
		braveAPIKey:  braveAPIKey,
		runningTasks: make(map[string]context.CancelFunc),
	}
}

// Spawn 启动子 Agent 执行后台任务（实现 tools.Spawner 接口）
func (m *SubagentManager) Spawn(ctx context.Context, task, label, originChannel, originChatID string) (string, error) {
	taskID := fmt.Sprintf("%08x", rand.Int31())
	displayLabel := label
	if displayLabel == "" {
		if len(task) > 30 {
			displayLabel = task[:30] + "..."
		} else {
			displayLabel = task
		}
	}

	taskCtx, cancel := context.WithCancel(context.Background())

	m.mu.Lock()
	m.runningTasks[taskID] = cancel
	m.mu.Unlock()

	go m.runSubagent(taskCtx, taskID, task, displayLabel, originChannel, originChatID)

	slog.Info("Spawned subagent", "task_id", taskID, "label", displayLabel)
	return fmt.Sprintf("Subagent [%s] started (id: %s). I'll notify you when it completes.", displayLabel, taskID), nil
}

// GetRunningCount 获取当前运行的子 Agent 数量
func (m *SubagentManager) GetRunningCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.runningTasks)
}

// runSubagent 执行子 Agent 任务并回报结果
func (m *SubagentManager) runSubagent(
	ctx context.Context,
	taskID, task, label, originChannel, originChatID string,
) {
	defer func() {
		m.mu.Lock()
		delete(m.runningTasks, taskID)
		m.mu.Unlock()
	}()

	slog.Info("Subagent starting task", "task_id", taskID, "label", label)

	// 构建子 Agent 的工具注册（无 message 和 spawn 工具，防止递归）
	registry := tools.NewRegistry()
	tools.RegisterCommonTools(registry, m.workspace, m.braveAPIKey)

	// 构建子 Agent 专用 system prompt
	systemPrompt := m.buildSubagentPrompt(task)
	messages := []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: task},
	}

	// 使用公共 RunLoop（子 Agent 用 Debug 级日志）
	subLogger := slog.Default().With("subagent", taskID)
	finalResult, err := RunLoop(ctx, LoopConfig{
		Provider:      m.provider,
		Model:         m.model,
		Registry:      registry,
		MaxIterations: 15,
		Logger:        subLogger,
	}, messages)

	if err != nil {
		slog.Error("Subagent LLM error", "task_id", taskID, "error", err)
		m.announceResult(taskID, label, task, fmt.Sprintf("Error: %s", err.Error()), originChannel, originChatID, "error")
		return
	}

	if finalResult == "" {
		finalResult = "Task completed but no final response was generated."
	}

	slog.Info("Subagent completed successfully", "task_id", taskID)
	m.announceResult(taskID, label, task, finalResult, originChannel, originChatID, "ok")
}

// announceResult 通过消息总线回报子 Agent 结果
func (m *SubagentManager) announceResult(
	taskID, label, task, result, originChannel, originChatID, status string,
) {
	statusText := "completed successfully"
	if status != "ok" {
		statusText = "failed"
	}

	announceContent := fmt.Sprintf(`[Subagent '%s' %s]

Task: %s

Result:
%s

Summarize this naturally for the user. Keep it brief (1-2 sentences). Do not mention technical details like "subagent" or task IDs.`,
		label, statusText, task, result)

	msg := InboundMessage{
		Channel:  "system",
		SenderID: "subagent",
		ChatID:   fmt.Sprintf("%s:%s", originChannel, originChatID),
		Content:  announceContent,
	}

	if err := m.bus.PublishInbound(msg); err != nil {
		slog.Error("Failed to announce subagent result", "task_id", taskID, "error", err)
	}
}

// buildSubagentPrompt 构建子 Agent 专用的 system prompt
func (m *SubagentManager) buildSubagentPrompt(task string) string {
	return fmt.Sprintf(`# Subagent

You are a subagent spawned by the main agent to complete a specific task.

## Your Task
%s

## Rules
1. Stay focused - complete only the assigned task, nothing else
2. Your final response will be reported back to the main agent
3. Do not initiate conversations or take on side tasks
4. Be concise but informative in your findings

## What You Can Do
- Read and write files in the workspace
- Execute shell commands
- Search the web and fetch web pages
- Complete the task thoroughly

## What You Cannot Do
- Send messages directly to users (no message tool available)
- Spawn other subagents
- Access the main agent's conversation history

## Workspace
Your workspace is at: %s

When you have completed the task, provide a clear summary of your findings or actions.`, task, m.workspace)
}
