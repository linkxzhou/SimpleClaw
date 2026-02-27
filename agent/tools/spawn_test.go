package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// mockSpawner 模拟 Spawner 接口
type mockSpawner struct {
	lastTask    string
	lastLabel   string
	lastChannel string
	lastChatID  string
	result      string
	err         error
}

func (m *mockSpawner) Spawn(_ context.Context, task, label, originChannel, originChatID string) (string, error) {
	m.lastTask = task
	m.lastLabel = label
	m.lastChannel = originChannel
	m.lastChatID = originChatID
	return m.result, m.err
}

// ---------- SpawnTool 基本属性 ----------

func TestSpawnTool_Name(t *testing.T) {
	tool := NewSpawnTool(&mockSpawner{})
	if tool.Name() != "spawn" {
		t.Errorf("Name = %q, want %q", tool.Name(), "spawn")
	}
}

func TestSpawnTool_Description(t *testing.T) {
	tool := NewSpawnTool(&mockSpawner{})
	if tool.Description() == "" {
		t.Error("Description should not be empty")
	}
}

func TestSpawnTool_Parameters(t *testing.T) {
	tool := NewSpawnTool(&mockSpawner{})
	params := tool.Parameters()
	if params["type"] != "object" {
		t.Errorf("Parameters type = %v, want object", params["type"])
	}
}

// ---------- 执行 ----------

func TestSpawnTool_Success(t *testing.T) {
	spawner := &mockSpawner{result: "task started: abc123"}
	tool := NewSpawnTool(spawner)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"task":  "analyze logs",
		"label": "log-analyzer",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "task started: abc123" {
		t.Errorf("result = %q, want %q", result, "task started: abc123")
	}
	if spawner.lastTask != "analyze logs" {
		t.Errorf("lastTask = %q, want %q", spawner.lastTask, "analyze logs")
	}
	if spawner.lastLabel != "log-analyzer" {
		t.Errorf("lastLabel = %q, want %q", spawner.lastLabel, "log-analyzer")
	}
}

func TestSpawnTool_DefaultContext(t *testing.T) {
	spawner := &mockSpawner{result: "ok"}
	tool := NewSpawnTool(spawner)

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"task": "do something",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spawner.lastChannel != "cli" {
		t.Errorf("lastChannel = %q, want %q", spawner.lastChannel, "cli")
	}
	if spawner.lastChatID != "direct" {
		t.Errorf("lastChatID = %q, want %q", spawner.lastChatID, "direct")
	}
}

func TestSpawnTool_SetContext(t *testing.T) {
	spawner := &mockSpawner{result: "ok"}
	tool := NewSpawnTool(spawner)
	tool.SetContext("telegram", "99999")

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"task": "do something",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spawner.lastChannel != "telegram" {
		t.Errorf("lastChannel = %q, want %q", spawner.lastChannel, "telegram")
	}
	if spawner.lastChatID != "99999" {
		t.Errorf("lastChatID = %q, want %q", spawner.lastChatID, "99999")
	}
}

func TestSpawnTool_EmptyTask(t *testing.T) {
	tool := NewSpawnTool(&mockSpawner{})
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"task": "",
	})
	if err == nil {
		t.Fatal("expected error for empty task")
	}
}

func TestSpawnTool_SpawnerError(t *testing.T) {
	spawner := &mockSpawner{err: errors.New("spawn failed")}
	tool := NewSpawnTool(spawner)

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"task": "do something",
	})
	if err == nil {
		t.Fatal("expected error from spawner")
	}
	if !strings.Contains(err.Error(), "spawn failed") {
		t.Errorf("err = %v, expected spawn failed", err)
	}
}

func TestSpawnTool_EmptyLabel(t *testing.T) {
	spawner := &mockSpawner{result: "ok"}
	tool := NewSpawnTool(spawner)

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"task": "do something",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spawner.lastLabel != "" {
		t.Errorf("lastLabel = %q, expected empty", spawner.lastLabel)
	}
}
