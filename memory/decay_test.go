package memory

import (
	"math"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// ApplyTimeDecay 测试
// ---------------------------------------------------------------------------

func TestApplyTimeDecayCoreNeverDecays(t *testing.T) {
	score := 1.0
	entries := []MemoryEntry{
		{
			Category:  CategoryCore,
			Timestamp: time.Now().Add(-365 * 24 * time.Hour).Format(time.RFC3339), // 1 年前
			Score:     &score,
		},
	}
	ApplyTimeDecay(entries)
	if *entries[0].Score != 1.0 {
		t.Errorf("Core 记忆不应衰减，实际 %f", *entries[0].Score)
	}
}

func TestApplyTimeDecayDailyHalfLife(t *testing.T) {
	entries := []MemoryEntry{
		{
			Category:  CategoryDaily,
			Timestamp: time.Now().Add(-7 * 24 * time.Hour).Format(time.RFC3339), // 7 天前
			Score:     floatPtr(0.6),
		},
	}
	ApplyTimeDecay(entries)
	// 半衰期 7 天，7 天后应为初始分数的 ~50% = 0.6 * 0.5 = 0.3
	if *entries[0].Score < 0.25 || *entries[0].Score > 0.35 {
		t.Errorf("Daily 7 天后应为 ~0.30，实际 %f", *entries[0].Score)
	}
}

func TestApplyTimeDecayConversationFastDecay(t *testing.T) {
	entries := []MemoryEntry{
		{
			Category:  CategoryConversation,
			Timestamp: time.Now().Add(-6 * 24 * time.Hour).Format(time.RFC3339), // 6 天前
			Score:     floatPtr(0.4),
		},
	}
	ApplyTimeDecay(entries)
	// 半衰期 3 天，6 天后应为初始分数的 ~25% = 0.4 * 0.25 = 0.1
	if *entries[0].Score < 0.05 || *entries[0].Score > 0.15 {
		t.Errorf("Conversation 6 天后应为 ~0.10，实际 %f", *entries[0].Score)
	}
}

func TestApplyTimeDecayProceduralSlowDecay(t *testing.T) {
	entries := []MemoryEntry{
		{
			Category:  CategoryProcedural,
			Timestamp: time.Now().Add(-30 * 24 * time.Hour).Format(time.RFC3339), // 30 天前
			Score:     floatPtr(0.8),
		},
	}
	ApplyTimeDecay(entries)
	// 半衰期 30 天，30 天后应为初始分数的 ~50% = 0.8 * 0.5 = 0.4
	if *entries[0].Score < 0.35 || *entries[0].Score > 0.45 {
		t.Errorf("Procedural 30 天后应为 ~0.40，实际 %f", *entries[0].Score)
	}
}

func TestApplyTimeDecayRecentEntryMinimalDecay(t *testing.T) {
	entries := []MemoryEntry{
		{
			Category:  CategoryDaily,
			Timestamp: time.Now().Add(-1 * time.Hour).Format(time.RFC3339), // 1 小时前
			Score:     floatPtr(0.6),
		},
	}
	ApplyTimeDecay(entries)
	// 1 小时几乎不衰减
	if *entries[0].Score < 0.55 {
		t.Errorf("1 小时前的条目几乎不应衰减，实际 %f", *entries[0].Score)
	}
}

func TestApplyTimeDecayNilScore(t *testing.T) {
	entries := []MemoryEntry{
		{
			Category:  CategoryDaily,
			Timestamp: time.Now().Add(-7 * 24 * time.Hour).Format(time.RFC3339),
			Score:     nil,
		},
	}
	ApplyTimeDecay(entries)
	if entries[0].Score == nil {
		t.Error("nil Score 应被初始化")
	}
}

// ---------------------------------------------------------------------------
// SortByScore 测试
// ---------------------------------------------------------------------------

func TestSortByScore(t *testing.T) {
	entries := []MemoryEntry{
		{ID: "a", Score: floatPtr(0.3)},
		{ID: "b", Score: floatPtr(0.9)},
		{ID: "c", Score: floatPtr(0.1)},
		{ID: "d", Score: floatPtr(0.6)},
	}
	SortByScore(entries)
	if entries[0].ID != "b" || entries[1].ID != "d" || entries[2].ID != "a" || entries[3].ID != "c" {
		t.Errorf("排序错误: %s %s %s %s", entries[0].ID, entries[1].ID, entries[2].ID, entries[3].ID)
	}
}

// ---------------------------------------------------------------------------
// SelectByTokenBudget 测试
// ---------------------------------------------------------------------------

func TestSelectByTokenBudgetAll(t *testing.T) {
	entries := []MemoryEntry{
		{Content: "short"},
		{Content: "also short"},
	}
	result := SelectByTokenBudget(entries, 10000)
	if len(result) != 2 {
		t.Errorf("预算充足应返回全部，实际 %d", len(result))
	}
}

func TestSelectByTokenBudgetLimited(t *testing.T) {
	entries := []MemoryEntry{
		{Content: "hello world"},
		{Content: "this is a very long content that takes many tokens " + string(make([]byte, 5000))},
	}
	result := SelectByTokenBudget(entries, 50)
	if len(result) != 1 {
		t.Errorf("预算受限应只返回 1 条，实际 %d", len(result))
	}
}

func TestSelectByTokenBudgetZero(t *testing.T) {
	entries := []MemoryEntry{{Content: "test"}}
	result := SelectByTokenBudget(entries, 0)
	if len(result) != 1 {
		t.Error("budget=0 应不限制")
	}
}

// ---------------------------------------------------------------------------
// StructuredMemory 测试
// ---------------------------------------------------------------------------

func TestStructuredMemoryStoreAndQuery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "entries.jsonl")
	sm, err := NewStructuredMemory(path)
	if err != nil {
		t.Fatal(err)
	}

	sm.Store("user_name", "Alice", CategoryCore)
	sm.Store("today_chat", "Discussed Go testing", CategoryDaily)
	sm.Store("pref_lang", "Prefers Go over Python", CategoryProcedural)

	if sm.Len() != 3 {
		t.Errorf("期望 3 条，实际 %d", sm.Len())
	}

	// 查询 Core
	coreEntries := sm.Query([]MemoryCategory{CategoryCore}, 10)
	if len(coreEntries) != 1 || coreEntries[0].Content != "Alice" {
		t.Errorf("Core 查询异常: %v", coreEntries)
	}

	// 查询全部
	all := sm.Query(nil, 0)
	if len(all) != 3 {
		t.Errorf("全部查询应返回 3 条，实际 %d", len(all))
	}
}

func TestStructuredMemoryUpsert(t *testing.T) {
	dir := t.TempDir()
	sm, _ := NewStructuredMemory(filepath.Join(dir, "e.jsonl"))

	sm.Store("user_name", "Alice", CategoryCore)
	sm.Upsert("user_name", "Bob", CategoryCore)

	if sm.Len() != 1 {
		t.Errorf("Upsert 同 key 应覆盖，条目数应为 1，实际 %d", sm.Len())
	}
	all := sm.All()
	if all[0].Content != "Bob" {
		t.Errorf("Upsert 后内容应为 Bob，实际 %q", all[0].Content)
	}
}

func TestStructuredMemoryDelete(t *testing.T) {
	dir := t.TempDir()
	sm, _ := NewStructuredMemory(filepath.Join(dir, "e.jsonl"))

	sm.Store("k1", "v1", CategoryDaily)
	sm.Store("k2", "v2", CategoryDaily)

	all := sm.All()
	sm.Delete(all[0].ID)

	if sm.Len() != 1 {
		t.Errorf("删除后应剩 1 条，实际 %d", sm.Len())
	}
}

func TestStructuredMemoryPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "entries.jsonl")

	// 写入
	sm1, _ := NewStructuredMemory(path)
	sm1.Store("key1", "value1", CategoryCore)
	sm1.Store("key2", "value2", CategoryDaily)

	// 重新加载
	sm2, err := NewStructuredMemory(path)
	if err != nil {
		t.Fatal(err)
	}
	if sm2.Len() != 2 {
		t.Errorf("重新加载后应有 2 条，实际 %d", sm2.Len())
	}
}

func TestStructuredMemoryPrune(t *testing.T) {
	dir := t.TempDir()
	sm, _ := NewStructuredMemory(filepath.Join(dir, "e.jsonl"))

	sm.Store("core_fact", "important", CategoryCore)
	sm.Store("old_chat", "yesterday", CategoryConversation)

	// 手动设低分
	entries := sm.All()
	for i := range entries {
		if entries[i].Category == CategoryConversation {
			low := 0.01
			entries[i].Score = &low
		}
	}
	// 因为 Prune 基于内存中的 Score，需要通过实际衰减触发
	// 这里直接验证 Core 不被清理
	pruned := sm.Prune(0.5)
	// Conversation 初始分数 0.4 < 0.5 阈值，应被清理
	if pruned != 1 {
		t.Errorf("应清理 1 条，实际 %d", pruned)
	}
	if sm.Len() != 1 {
		t.Errorf("清理后应剩 1 条 Core，实际 %d", sm.Len())
	}
}

// ---------------------------------------------------------------------------
// 衰减精度验证
// ---------------------------------------------------------------------------

func TestDecayMathPrecision(t *testing.T) {
	// 验证指数衰减公式精度
	halfLife := 7.0
	ageDays := 14.0
	// 2 个半衰期后应为 25%
	factor := math.Exp(-ageDays / halfLife * math.Ln2)
	expected := 0.25
	if math.Abs(factor-expected) > 0.001 {
		t.Errorf("2 个半衰期后应为 25%%，实际 %f", factor)
	}
}

// ---------------------------------------------------------------------------
// 辅助
// ---------------------------------------------------------------------------

func floatPtr(f float64) *float64 { return &f }
