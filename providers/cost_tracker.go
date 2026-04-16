// CostTracker 负责费用追踪和预算检查。
// 支持日/月限额、预警阈值、会话级和全局级费用统计。

package providers

import (
	"log/slog"
	"sync"
	"time"
)

// CostConfig 成本追踪配置。
type CostConfig struct {
	Enabled         bool    `json:"enabled"`
	DailyLimitUSD   float64 `json:"dailyLimitUsd"`   // 日限额（美元），默认 10.0
	MonthlyLimitUSD float64 `json:"monthlyLimitUsd"` // 月限额（美元），默认 100.0
	WarnAtPercent   int     `json:"warnAtPercent"`    // 预警百分比，默认 80
}

// DefaultCostConfig 返回合理的默认配置。
func DefaultCostConfig() CostConfig {
	return CostConfig{
		Enabled:         false,
		DailyLimitUSD:   10.0,
		MonthlyLimitUSD: 100.0,
		WarnAtPercent:   80,
	}
}

// CostTracker 费用追踪器。
type CostTracker struct {
	config  CostConfig
	storage *CostStorage
	logger  *slog.Logger

	mu           sync.Mutex
	sessionCosts []CostRecord
}

// NewCostTracker 创建费用追踪器。
// dataDir 为数据根目录（如 ~/.simpleclaw），费用文件存储在 dataDir/costs/。
func NewCostTracker(config CostConfig, dataDir string, logger *slog.Logger) (*CostTracker, error) {
	if logger == nil {
		logger = slog.Default()
	}

	storage, err := NewCostStorage(dataDir + "/costs")
	if err != nil {
		return nil, err
	}

	return &CostTracker{
		config:  config,
		storage: storage,
		logger:  logger,
	}, nil
}

// CheckBudget 检查预算是否允许继续调用。
// estimatedCostUSD 为本次调用的预估费用。
func (ct *CostTracker) CheckBudget(estimatedCostUSD float64) BudgetCheck {
	if !ct.config.Enabled {
		return BudgetAllowed
	}

	daily := ct.storage.GetDailyCost()
	monthly := ct.storage.GetMonthlyCost()

	// 检查日限额
	if ct.config.DailyLimitUSD > 0 && daily+estimatedCostUSD > ct.config.DailyLimitUSD {
		return BudgetExceeded
	}
	// 检查月限额
	if ct.config.MonthlyLimitUSD > 0 && monthly+estimatedCostUSD > ct.config.MonthlyLimitUSD {
		return BudgetExceeded
	}

	// 检查预警
	warnThreshold := float64(ct.config.WarnAtPercent) / 100.0
	if ct.config.DailyLimitUSD > 0 && daily+estimatedCostUSD > ct.config.DailyLimitUSD*warnThreshold {
		return BudgetWarning
	}
	if ct.config.MonthlyLimitUSD > 0 && monthly+estimatedCostUSD > ct.config.MonthlyLimitUSD*warnThreshold {
		return BudgetWarning
	}

	return BudgetAllowed
}

// RecordUsage 记录一次 LLM 调用的费用。
func (ct *CostTracker) RecordUsage(usage TokenUsage, sessionID string) error {
	record := CostRecord{
		Timestamp: time.Now().Format(time.RFC3339),
		SessionID: sessionID,
		Usage:     usage,
	}

	ct.mu.Lock()
	ct.sessionCosts = append(ct.sessionCosts, record)
	ct.mu.Unlock()

	if err := ct.storage.AddRecord(record); err != nil {
		ct.logger.Error("failed to persist cost record", "error", err)
		return err
	}
	return nil
}

// GetSummary 返回当前费用汇总。
func (ct *CostTracker) GetSummary() CostSummary {
	ct.mu.Lock()
	var sessionCost float64
	sessionCalls := len(ct.sessionCosts)
	for _, r := range ct.sessionCosts {
		sessionCost += r.Usage.CostUSD
	}
	ct.mu.Unlock()

	return CostSummary{
		SessionCostUSD:  sessionCost,
		SessionCalls:    sessionCalls,
		DailyCostUSD:    ct.storage.GetDailyCost(),
		MonthlyCostUSD:  ct.storage.GetMonthlyCost(),
		DailyLimitUSD:   ct.config.DailyLimitUSD,
		MonthlyLimitUSD: ct.config.MonthlyLimitUSD,
		ModelBreakdown:  ct.storage.GetModelBreakdown(),
	}
}

// IsEnabled 返回费用追踪是否启用。
func (ct *CostTracker) IsEnabled() bool {
	return ct.config.Enabled
}
