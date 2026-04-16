package memory

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// HistoryStore 测试
// ---------------------------------------------------------------------------

func TestHistoryStoreAppendAndRead(t *testing.T) {
	dir := t.TempDir()
	hs, err := NewHistoryStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	hs.Append(HistoryEntry{
		Timestamp:  time.Now().Format(time.RFC3339),
		SessionKey: "cli:default",
		UserMsg:    "Hello",
		AssistMsg:  "Hi there!",
		Summary:    "Greeting",
	})
	hs.Append(HistoryEntry{
		Timestamp:  time.Now().Format(time.RFC3339),
		SessionKey: "cli:default",
		UserMsg:    "What time is it?",
		AssistMsg:  "It's 3pm.",
		Summary:    "Time query",
	})

	if hs.Len() != 2 {
		t.Errorf("期望 2 条，实际 %d", hs.Len())
	}
}

func TestHistoryStoreReadSince(t *testing.T) {
	dir := t.TempDir()
	hs, _ := NewHistoryStore(dir)

	for i := 0; i < 5; i++ {
		hs.Append(HistoryEntry{
			Timestamp: time.Now().Format(time.RFC3339),
			Summary:   "entry",
		})
	}

	// 读取 cursor=2 之后的条目
	entries, err := hs.ReadSince(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Errorf("cursor=2 后应有 3 条，实际 %d", len(entries))
	}
	if entries[0].Cursor != 3 {
		t.Errorf("第一条 cursor 应为 3，实际 %d", entries[0].Cursor)
	}
}

func TestHistoryStoreCursor(t *testing.T) {
	dir := t.TempDir()
	hs, _ := NewHistoryStore(dir)

	if c := hs.GetDreamCursor(); c != 0 {
		t.Errorf("初始 cursor 应为 0，实际 %d", c)
	}

	hs.SetDreamCursor(42)
	if c := hs.GetDreamCursor(); c != 42 {
		t.Errorf("设置后 cursor 应为 42，实际 %d", c)
	}
}

func TestHistoryStoreCompact(t *testing.T) {
	dir := t.TempDir()
	hs, _ := NewHistoryStore(dir)

	for i := 0; i < 10; i++ {
		hs.Append(HistoryEntry{Summary: "entry"})
	}

	hs.Compact(3)
	if hs.Len() != 3 {
		t.Errorf("压缩后应剩 3 条，实际 %d", hs.Len())
	}
}

func TestHistoryStorePersistence(t *testing.T) {
	dir := t.TempDir()

	hs1, _ := NewHistoryStore(dir)
	hs1.Append(HistoryEntry{Summary: "a"})
	hs1.Append(HistoryEntry{Summary: "b"})

	hs2, _ := NewHistoryStore(dir)
	if hs2.Len() != 2 {
		t.Errorf("重新加载后应有 2 条，实际 %d", hs2.Len())
	}
}

func TestHistoryStoreCursorAutoIncrement(t *testing.T) {
	dir := t.TempDir()
	hs, _ := NewHistoryStore(dir)

	hs.Append(HistoryEntry{Summary: "first"})
	hs.Append(HistoryEntry{Summary: "second"})

	entries, _ := hs.ReadSince(0)
	if len(entries) < 2 {
		t.Fatal("应有 2 条")
	}
	if entries[0].Cursor != 1 || entries[1].Cursor != 2 {
		t.Errorf("cursor 应为 1,2，实际 %d,%d", entries[0].Cursor, entries[1].Cursor)
	}
}

// ---------------------------------------------------------------------------
// DreamService 测试（使用 mock LLM）
// ---------------------------------------------------------------------------

type mockDreamProvider struct {
	response string
	err      error
}

func (m *mockDreamProvider) ChatSimple(ctx context.Context, systemPrompt, userPrompt, model string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func TestDreamServiceNoNewEntries(t *testing.T) {
	dir := t.TempDir()
	memDir := filepath.Join(dir, "memory")
	store := NewMemoryStore(dir)
	hs, _ := NewHistoryStore(memDir)
	sm, _ := NewStructuredMemory(filepath.Join(memDir, "entries.jsonl"))

	ds := NewDreamService(store, hs, sm, &mockDreamProvider{}, DreamConfig{Enabled: true}, nil)

	result, err := ds.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 0 {
		t.Errorf("无新条目时应处理 0 条，实际 %d", result.Processed)
	}
}

func TestDreamServicePhase1And2(t *testing.T) {
	dir := t.TempDir()
	memDir := filepath.Join(dir, "memory")
	store := NewMemoryStore(dir)
	hs, _ := NewHistoryStore(memDir)
	sm, _ := NewStructuredMemory(filepath.Join(memDir, "entries.jsonl"))

	// 添加历史
	hs.Append(HistoryEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		UserMsg:   "My name is Alice",
		AssistMsg: "Nice to meet you, Alice!",
		Summary:   "Introduction",
	})

	// Mock LLM 返回分析结果
	analysis := phase1AnalysisResult{
		NewFacts: []struct {
			Key      string `json:"key"`
			Content  string `json:"content"`
			Category string `json:"category"`
		}{
			{Key: "user_name", Content: "User's name is Alice", Category: "core"},
		},
	}
	analysisJSON, _ := json.Marshal(analysis)

	ds := NewDreamService(store, hs, sm, &mockDreamProvider{
		response: string(analysisJSON),
	}, DreamConfig{Enabled: true}, nil)

	result, err := ds.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 {
		t.Errorf("应处理 1 条，实际 %d", result.Processed)
	}
	if result.NewFacts != 1 {
		t.Errorf("应有 1 条新事实，实际 %d", result.NewFacts)
	}

	// 验证结构化记忆
	entries := sm.Query([]MemoryCategory{CategoryCore}, 10)
	if len(entries) != 1 || entries[0].Key != "user_name" {
		t.Errorf("应有 user_name Core 记忆，实际 %v", entries)
	}

	// 验证 cursor 推进
	if result.Cursor != 1 {
		t.Errorf("cursor 应推进到 1，实际 %d", result.Cursor)
	}
}

func TestDreamServiceCursorAdvance(t *testing.T) {
	dir := t.TempDir()
	memDir := filepath.Join(dir, "memory")
	store := NewMemoryStore(dir)
	hs, _ := NewHistoryStore(memDir)
	sm, _ := NewStructuredMemory(filepath.Join(memDir, "entries.jsonl"))

	// 添加 3 条历史
	for i := 0; i < 3; i++ {
		hs.Append(HistoryEntry{
			Timestamp: time.Now().Format(time.RFC3339),
			Summary:   "chat",
		})
	}

	ds := NewDreamService(store, hs, sm, &mockDreamProvider{
		response: `{"new_facts": [], "obsolete": []}`,
	}, DreamConfig{Enabled: true}, nil)

	// 第一次运行
	result1, _ := ds.Run(context.Background())
	if result1.Processed != 3 {
		t.Errorf("第一次应处理 3 条，实际 %d", result1.Processed)
	}

	// 第二次运行（无新条目）
	result2, _ := ds.Run(context.Background())
	if result2.Processed != 0 {
		t.Errorf("第二次应处理 0 条，实际 %d", result2.Processed)
	}
}

func TestDreamServiceBatchLimit(t *testing.T) {
	dir := t.TempDir()
	memDir := filepath.Join(dir, "memory")
	store := NewMemoryStore(dir)
	hs, _ := NewHistoryStore(memDir)
	sm, _ := NewStructuredMemory(filepath.Join(memDir, "entries.jsonl"))

	for i := 0; i < 50; i++ {
		hs.Append(HistoryEntry{
			Timestamp: time.Now().Format(time.RFC3339),
			Summary:   "chat",
		})
	}

	ds := NewDreamService(store, hs, sm, &mockDreamProvider{
		response: `{"new_facts": [], "obsolete": []}`,
	}, DreamConfig{Enabled: true, BatchSize: 10}, nil)

	result, _ := ds.Run(context.Background())
	if result.Processed != 10 {
		t.Errorf("BatchSize=10 时应只处理 10 条，实际 %d", result.Processed)
	}
}

func TestDreamServiceObsoleteRemoval(t *testing.T) {
	dir := t.TempDir()
	memDir := filepath.Join(dir, "memory")
	store := NewMemoryStore(dir)
	hs, _ := NewHistoryStore(memDir)
	sm, _ := NewStructuredMemory(filepath.Join(memDir, "entries.jsonl"))

	// 预存一条记忆
	sm.Store("old_fact", "outdated info", CategoryCore)

	hs.Append(HistoryEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Summary:   "correction",
	})

	ds := NewDreamService(store, hs, sm, &mockDreamProvider{
		response: `{"new_facts": [], "obsolete": ["old_fact"]}`,
	}, DreamConfig{Enabled: true}, nil)

	ds.Run(context.Background())

	// old_fact 应被删除
	entries := sm.All()
	for _, e := range entries {
		if e.Key == "old_fact" {
			t.Error("old_fact 应被删除")
		}
	}
}

func TestDreamServiceMarkdownSync(t *testing.T) {
	dir := t.TempDir()
	memDir := filepath.Join(dir, "memory")
	store := NewMemoryStore(dir)
	hs, _ := NewHistoryStore(memDir)
	sm, _ := NewStructuredMemory(filepath.Join(memDir, "entries.jsonl"))

	hs.Append(HistoryEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Summary:   "test",
	})

	ds := NewDreamService(store, hs, sm, &mockDreamProvider{
		response: `{"new_facts": [{"key": "user_lang", "content": "Prefers Go", "category": "procedural"}], "obsolete": []}`,
	}, DreamConfig{Enabled: true}, nil)

	ds.Run(context.Background())

	// MEMORY.md 应被同步
	md := store.ReadLongTerm()
	if md == "" {
		t.Error("MEMORY.md 应被写入")
	}
	if !containsStr(md, "Prefers Go") {
		t.Error("MEMORY.md 应包含 'Prefers Go'")
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
