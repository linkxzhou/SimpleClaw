// 成本追踪核心类型定义。
// 包含 TokenUsage、CostRecord、BudgetCheck 和 CostSummary。

package providers

// TokenUsage 记录单次 LLM 调用的 token 使用量和费用。
type TokenUsage struct {
	Model        string  `json:"model"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

// CostRecord 一条费用记录，持久化到 JSONL。
type CostRecord struct {
	Timestamp string     `json:"timestamp"`
	SessionID string     `json:"session_id"`
	Usage     TokenUsage `json:"usage"`
}

// BudgetCheck 预算检查结果。
type BudgetCheck int

const (
	BudgetAllowed  BudgetCheck = iota // 允许调用
	BudgetWarning                     // 接近阈值
	BudgetExceeded                    // 超出限额
)

// CostSummary 费用汇总信息。
type CostSummary struct {
	SessionCostUSD  float64            `json:"session_cost_usd"`
	SessionCalls    int                `json:"session_calls"`
	DailyCostUSD    float64            `json:"daily_cost_usd"`
	MonthlyCostUSD  float64            `json:"monthly_cost_usd"`
	DailyLimitUSD   float64            `json:"daily_limit_usd"`
	MonthlyLimitUSD float64            `json:"monthly_limit_usd"`
	ModelBreakdown  map[string]float64 `json:"model_breakdown"`
}
