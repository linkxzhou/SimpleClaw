package tools

import (
	"context"
	"fmt"
	"sync"
)

// Spawner 子 Agent 生成器接口
type Spawner interface {
	Spawn(ctx context.Context, task, label, originChannel, originChatID string) (string, error)
}

// SpawnTool 子 Agent 生成工具
type SpawnTool struct {
	mu            sync.Mutex
	manager       Spawner
	originChannel string
	originChatID  string
}

// NewSpawnTool 创建子 Agent 生成工具
func NewSpawnTool(manager Spawner) *SpawnTool {
	return &SpawnTool{
		manager:       manager,
		originChannel: "cli",
		originChatID:  "direct",
	}
}

// SetContext 设置消息来源上下文
func (t *SpawnTool) SetContext(channel, chatID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.originChannel = channel
	t.originChatID = chatID
}

func (t *SpawnTool) Name() string        { return "spawn" }
func (t *SpawnTool) Description() string {
	return "Spawn a subagent to handle a task in the background. " +
		"Use this for complex or time-consuming tasks that can run independently. " +
		"The subagent will complete the task and report back when done."
}
func (t *SpawnTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task": map[string]interface{}{
				"type":        "string",
				"description": "The task for the subagent to complete",
			},
			"label": map[string]interface{}{
				"type":        "string",
				"description": "Optional short label for the task (for display)",
			},
		},
		"required": []string{"task"},
	}
}

func (t *SpawnTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	task, _ := params["task"].(string)
	if task == "" {
		return "", fmt.Errorf("task is required")
	}

	label, _ := params["label"].(string)

	t.mu.Lock()
	channel := t.originChannel
	chatID := t.originChatID
	t.mu.Unlock()

	return t.manager.Spawn(ctx, task, label, channel, chatID)
}
