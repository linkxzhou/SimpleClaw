package providers

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// EstimateCost / pricing 测试
// ---------------------------------------------------------------------------

func TestEstimateCostKnownModel(t *testing.T) {
	// claude-sonnet-4: input $3/M, output $15/M
	cost := EstimateCost("anthropic/claude-sonnet-4-20250514", 1000, 500)
	// 1000/1e6*3 + 500/1e6*15 = 0.003 + 0.0075 = 0.0105
	if cost < 0.0104 || cost > 0.0106 {
		t.Errorf("期望 ~0.0105，实际 %f", cost)
	}
}

func TestEstimateCostGPT4o(t *testing.T) {
	// gpt-4o: input $2.5/M, output $10/M
	cost := EstimateCost("openai/gpt-4o", 10000, 2000)
	expected := 10000.0/1e6*2.5 + 2000.0/1e6*10.0 // 0.025 + 0.02 = 0.045
	if cost < expected-0.001 || cost > expected+0.001 {
		t.Errorf("期望 ~%.4f，实际 %f", expected, cost)
	}
}

func TestEstimateCostUnknownModel(t *testing.T) {
	cost := EstimateCost("unknown-vendor/unknown-model", 10000, 5000)
	if cost != 0 {
		t.Errorf("未知模型应返回 0，实际 %f", cost)
	}
}

func TestEstimateCostPrefixMatch(t *testing.T) {
	// "gpt-4o-2024-08-06" 应匹配 "gpt-4o" 前缀
	cost := EstimateCost("gpt-4o-2024-08-06", 1000, 1000)
	if cost == 0 {
		t.Error("前缀匹配应生效，但返回了 0")
	}
}

func TestEstimateCostDeepSeek(t *testing.T) {
	cost := EstimateCost("deepseek-chat", 100000, 50000)
	// input $0.14/M, output $0.28/M
	expected := 100000.0/1e6*0.14 + 50000.0/1e6*0.28
	if cost < expected-0.001 || cost > expected+0.001 {
		t.Errorf("期望 ~%.4f，实际 %f", expected, cost)
	}
}

func TestNormalizeModelName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"anthropic/claude-sonnet-4-20250514", "claude-sonnet-4-20250514"},
		{"GPT-4o", "gpt-4o"},
		{"openai/GPT-4O-Mini", "gpt-4o-mini"},
		{"deepseek-chat", "deepseek-chat"},
	}
	for _, tt := range tests {
		got := normalizeModelName(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeModelName(%q) = %q, 期望 %q", tt.input, got, tt.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// CostStorage 测试
// ---------------------------------------------------------------------------

func TestCostStorageAddAndRead(t *testing.T) {
	dir := t.TempDir()
	storage, err := NewCostStorage(dir)
	if err != nil {
		t.Fatalf("创建 storage 失败: %v", err)
	}

	record := CostRecord{
		Timestamp: time.Now().Format(time.RFC3339),
		SessionID: "sess-1",
		Usage: TokenUsage{
			Model:        "gpt-4o",
			InputTokens:  1000,
			OutputTokens: 500,
			TotalTokens:  1500,
			CostUSD:      0.03,
		},
	}

	if err := storage.AddRecord(record); err != nil {
		t.Fatalf("添加记录失败: %v", err)
	}

	daily := storage.GetDailyCost()
	if daily < 0.029 || daily > 0.031 {
		t.Errorf("日费用应为 ~0.03，实际 %f", daily)
	}

	monthly := storage.GetMonthlyCost()
	if monthly < 0.029 || monthly > 0.031 {
		t.Errorf("月费用应为 ~0.03，实际 %f", monthly)
	}
}

func TestCostStorageMultipleRecords(t *testing.T) {
	dir := t.TempDir()
	storage, err := NewCostStorage(dir)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 5; i++ {
		storage.AddRecord(CostRecord{
			Timestamp: time.Now().Format(time.RFC3339),
			SessionID: "sess",
			Usage:     TokenUsage{CostUSD: 0.01},
		})
	}

	daily := storage.GetDailyCost()
	if daily < 0.049 || daily > 0.051 {
		t.Errorf("期望 ~0.05，实际 %f", daily)
	}
}

func TestCostStorageModelBreakdown(t *testing.T) {
	dir := t.TempDir()
	storage, err := NewCostStorage(dir)
	if err != nil {
		t.Fatal(err)
	}

	storage.AddRecord(CostRecord{
		Timestamp: time.Now().Format(time.RFC3339),
		Usage:     TokenUsage{Model: "gpt-4o", CostUSD: 0.10},
	})
	storage.AddRecord(CostRecord{
		Timestamp: time.Now().Format(time.RFC3339),
		Usage:     TokenUsage{Model: "claude-sonnet", CostUSD: 0.20},
	})
	storage.AddRecord(CostRecord{
		Timestamp: time.Now().Format(time.RFC3339),
		Usage:     TokenUsage{Model: "gpt-4o", CostUSD: 0.05},
	})

	breakdown := storage.GetModelBreakdown()
	if len(breakdown) != 2 {
		t.Fatalf("期望 2 个模型，实际 %d", len(breakdown))
	}
	if breakdown["gpt-4o"] < 0.14 || breakdown["gpt-4o"] > 0.16 {
		t.Errorf("gpt-4o 应为 ~0.15，实际 %f", breakdown["gpt-4o"])
	}
	if breakdown["claude-sonnet"] < 0.19 || breakdown["claude-sonnet"] > 0.21 {
		t.Errorf("claude-sonnet 应为 ~0.20，实际 %f", breakdown["claude-sonnet"])
	}
}

func TestCostStorageEmptyDir(t *testing.T) {
	dir := t.TempDir()
	storage, err := NewCostStorage(dir)
	if err != nil {
		t.Fatal(err)
	}

	if daily := storage.GetDailyCost(); daily != 0 {
		t.Errorf("空目录日费用应为 0，实际 %f", daily)
	}
	if monthly := storage.GetMonthlyCost(); monthly != 0 {
		t.Errorf("空目录月费用应为 0，实际 %f", monthly)
	}
}

func TestCostStorageFileCreated(t *testing.T) {
	dir := t.TempDir()
	storage, err := NewCostStorage(dir)
	if err != nil {
		t.Fatal(err)
	}

	storage.AddRecord(CostRecord{
		Timestamp: time.Now().Format(time.RFC3339),
		Usage:     TokenUsage{CostUSD: 0.01},
	})

	today := time.Now().Format("2006-01-02")
	path := filepath.Join(dir, today+".jsonl")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("JSONL 文件应被创建: %s", path)
	}
}

// ---------------------------------------------------------------------------
// CostTracker 测试
// ---------------------------------------------------------------------------

func TestCostTrackerDisabled(t *testing.T) {
	dir := t.TempDir()
	ct, err := NewCostTracker(CostConfig{Enabled: false}, dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	if check := ct.CheckBudget(100.0); check != BudgetAllowed {
		t.Errorf("禁用时应始终返回 BudgetAllowed，实际 %d", check)
	}
}

func TestCostTrackerBudgetAllowed(t *testing.T) {
	dir := t.TempDir()
	ct, err := NewCostTracker(CostConfig{
		Enabled:         true,
		DailyLimitUSD:   10.0,
		MonthlyLimitUSD: 100.0,
		WarnAtPercent:   80,
	}, dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	if check := ct.CheckBudget(0.01); check != BudgetAllowed {
		t.Errorf("预算充足时应返回 BudgetAllowed，实际 %d", check)
	}
}

func TestCostTrackerBudgetExceeded(t *testing.T) {
	dir := t.TempDir()
	ct, err := NewCostTracker(CostConfig{
		Enabled:         true,
		DailyLimitUSD:   1.0,
		MonthlyLimitUSD: 100.0,
		WarnAtPercent:   80,
	}, dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	// 先记录接近限额的费用
	ct.RecordUsage(TokenUsage{CostUSD: 0.95}, "sess")

	// 再检查一笔较大的费用
	if check := ct.CheckBudget(0.10); check != BudgetExceeded {
		t.Errorf("超出日限额应返回 BudgetExceeded，实际 %d", check)
	}
}

func TestCostTrackerBudgetWarning(t *testing.T) {
	dir := t.TempDir()
	ct, err := NewCostTracker(CostConfig{
		Enabled:         true,
		DailyLimitUSD:   10.0,
		MonthlyLimitUSD: 100.0,
		WarnAtPercent:   80,
	}, dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	// 记录到 80% 以上
	ct.RecordUsage(TokenUsage{CostUSD: 8.0}, "sess")

	if check := ct.CheckBudget(0.5); check != BudgetWarning {
		t.Errorf("接近阈值应返回 BudgetWarning，实际 %d", check)
	}
}

func TestCostTrackerGetSummary(t *testing.T) {
	dir := t.TempDir()
	ct, err := NewCostTracker(CostConfig{
		Enabled:         true,
		DailyLimitUSD:   10.0,
		MonthlyLimitUSD: 100.0,
		WarnAtPercent:   80,
	}, dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	ct.RecordUsage(TokenUsage{Model: "gpt-4o", CostUSD: 0.05}, "sess-1")
	ct.RecordUsage(TokenUsage{Model: "claude-sonnet", CostUSD: 0.10}, "sess-1")

	summary := ct.GetSummary()
	if summary.SessionCalls != 2 {
		t.Errorf("期望 2 次调用，实际 %d", summary.SessionCalls)
	}
	if summary.SessionCostUSD < 0.14 || summary.SessionCostUSD > 0.16 {
		t.Errorf("会话费用应为 ~0.15，实际 %f", summary.SessionCostUSD)
	}
	if summary.DailyLimitUSD != 10.0 {
		t.Errorf("日限额应为 10.0，实际 %f", summary.DailyLimitUSD)
	}
}

func TestCostTrackerMonthlyExceeded(t *testing.T) {
	dir := t.TempDir()
	ct, err := NewCostTracker(CostConfig{
		Enabled:         true,
		DailyLimitUSD:   1000.0, // 日限额很高不触发
		MonthlyLimitUSD: 5.0,
		WarnAtPercent:   80,
	}, dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	ct.RecordUsage(TokenUsage{CostUSD: 4.5}, "sess")

	if check := ct.CheckBudget(1.0); check != BudgetExceeded {
		t.Errorf("超出月限额应返回 BudgetExceeded，实际 %d", check)
	}
}

// ---------------------------------------------------------------------------
// BudgetCheck 常量值测试
// ---------------------------------------------------------------------------

func TestBudgetCheckValues(t *testing.T) {
	if BudgetAllowed != 0 {
		t.Error("BudgetAllowed 应为 0")
	}
	if BudgetWarning != 1 {
		t.Error("BudgetWarning 应为 1")
	}
	if BudgetExceeded != 2 {
		t.Error("BudgetExceeded 应为 2")
	}
}
