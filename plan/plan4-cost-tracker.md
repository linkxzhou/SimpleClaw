# Plan 4: LLM 成本追踪 + 预算网关

## 来源

ZeroClaw — `src/cost/tracker.rs` + `src/cost/types.rs`

## 目标

为 SimpleClaw 增加 LLM API 调用费用追踪和预算控制，支持日/月限额、预警阈值、会话级和全局级费用统计，防止账单失控。

## 现状分析

- `providers/openai.go` 已解析 `usage` 字段（InputTokens/OutputTokens），但只做日志输出
- `FallbackChain` 没有费用拦截机制，超限仍会继续调用
- 无持久化的费用记录
- `simpleclaw status` 不显示费用信息

## 技术方案

### 阶段一：数据模型 + 存储（0.5 周）

#### 1.1 核心类型

```go
// providers/cost.go (新文件)
type TokenUsage struct {
    Model        string  `json:"model"`
    InputTokens  int     `json:"input_tokens"`
    OutputTokens int     `json:"output_tokens"`
    TotalTokens  int     `json:"total_tokens"`
    CostUSD      float64 `json:"cost_usd"`
}

type CostRecord struct {
    Timestamp  string     `json:"timestamp"`
    SessionID  string     `json:"session_id"`
    Usage      TokenUsage `json:"usage"`
}

type BudgetCheck int
const (
    BudgetAllowed  BudgetCheck = iota
    BudgetWarning              // 接近阈值
    BudgetExceeded             // 超出限额
)

type CostSummary struct {
    SessionCostUSD  float64            `json:"session_cost_usd"`
    DailyCostUSD    float64            `json:"daily_cost_usd"`
    MonthlyCostUSD  float64            `json:"monthly_cost_usd"`
    DailyLimitUSD   float64            `json:"daily_limit_usd"`
    MonthlyLimitUSD float64            `json:"monthly_limit_usd"`
    ModelBreakdown  map[string]float64 `json:"model_breakdown"`
}
```

#### 1.2 JSONL 存储

```go
// providers/cost_storage.go (新文件)
type CostStorage struct {
    dir      string // ~/.simpleclaw/costs/
    mu       sync.Mutex
    // 缓存当日和当月聚合值，避免每次全量扫描
    dailyCache   float64
    monthlyCache float64
    cacheDate    string // "2026-04-15"
}

// 按日分文件：costs/2026-04-15.jsonl
func (s *CostStorage) AddRecord(record CostRecord) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    filename := time.Now().Format("2006-01-02") + ".jsonl"
    f, err := os.OpenFile(filepath.Join(s.dir, filename), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    // ... 追加写入 JSON 行
    s.dailyCache += record.Usage.CostUSD
    s.monthlyCache += record.Usage.CostUSD
    return nil
}

func (s *CostStorage) GetAggregatedCosts() (daily, monthly float64, err error) {
    // 如果 cacheDate 是今天，直接返回缓存值
    // 否则重新扫描当日文件和当月所有文件
}
```

### 阶段二：CostTracker + 预算检查（1 周）

#### 2.1 CostTracker

```go
// providers/cost_tracker.go (新文件)
type CostConfig struct {
    Enabled       bool    `json:"enabled"`
    DailyLimitUSD float64 `json:"dailyLimitUsd"`   // 默认 10.0
    MonthlyLimitUSD float64 `json:"monthlyLimitUsd"` // 默认 100.0
    WarnAtPercent int     `json:"warnAtPercent"`    // 默认 80
}

type CostTracker struct {
    config       CostConfig
    storage      *CostStorage
    sessionID    string
    sessionCosts []CostRecord
    mu           sync.Mutex
}

func NewCostTracker(config CostConfig, dataDir string) (*CostTracker, error)

func (ct *CostTracker) CheckBudget(estimatedCostUSD float64) BudgetCheck {
    if !ct.config.Enabled {
        return BudgetAllowed
    }
    daily, monthly, _ := ct.storage.GetAggregatedCosts()

    // 检查日限额
    if daily + estimatedCostUSD > ct.config.DailyLimitUSD {
        return BudgetExceeded
    }
    // 检查月限额
    if monthly + estimatedCostUSD > ct.config.MonthlyLimitUSD {
        return BudgetExceeded
    }
    // 检查预警
    warnThreshold := float64(ct.config.WarnAtPercent) / 100.0
    if daily + estimatedCostUSD > ct.config.DailyLimitUSD * warnThreshold {
        return BudgetWarning
    }
    return BudgetAllowed
}

func (ct *CostTracker) RecordUsage(usage TokenUsage) error
func (ct *CostTracker) GetSummary() CostSummary
```

#### 2.2 模型费率表

```go
// providers/pricing.go (新文件)
// 主流模型每百万 token 费率（美元）
var ModelPricing = map[string]struct{ Input, Output float64 }{
    "claude-sonnet-4-20250514":   {3.0, 15.0},
    "claude-haiku-3.5":           {0.25, 1.25},
    "gpt-4o":                     {2.5, 10.0},
    "gpt-4o-mini":                {0.15, 0.6},
    "deepseek-chat":              {0.14, 0.28},
    // ... 按需扩展
}

func EstimateCost(model string, inputTokens, outputTokens int) float64 {
    pricing, ok := ModelPricing[normalizeModel(model)]
    if !ok {
        return 0 // 未知模型不计费
    }
    return float64(inputTokens)/1e6*pricing.Input + float64(outputTokens)/1e6*pricing.Output
}
```

### 阶段三：集成到 FallbackChain（0.5 周）

#### 3.1 调用前检查

```go
// providers/fallback.go — 修改 Chat() 方法
func (fc *FallbackChain) Chat(ctx context.Context, req ChatRequest) (*LLMResponse, error) {
    // 预估费用（按平均 token 量粗算）
    estimated := EstimateCost(req.Model, estimateInputTokens(req), req.MaxTokens/2)
    check := fc.costTracker.CheckBudget(estimated)

    switch check {
    case BudgetExceeded:
        summary := fc.costTracker.GetSummary()
        return nil, fmt.Errorf("budget exceeded: daily $%.2f/$%.2f, monthly $%.2f/$%.2f",
            summary.DailyCostUSD, summary.DailyLimitUSD,
            summary.MonthlyCostUSD, summary.MonthlyLimitUSD)
    case BudgetWarning:
        utils.LogWarn("Cost warning: approaching budget limit")
    }

    // 正常调用
    resp, err := fc.doChat(ctx, req)
    if err == nil {
        // 记录实际费用
        actualCost := EstimateCost(req.Model, resp.Usage.InputTokens, resp.Usage.OutputTokens)
        fc.costTracker.RecordUsage(TokenUsage{
            Model:        req.Model,
            InputTokens:  resp.Usage.InputTokens,
            OutputTokens: resp.Usage.OutputTokens,
            TotalTokens:  resp.Usage.InputTokens + resp.Usage.OutputTokens,
            CostUSD:      actualCost,
        })
    }
    return resp, err
}
```

### 阶段四：CLI 展示（0.5 周）

#### 4.1 扩展 simpleclaw status

```bash
$ simpleclaw status

🤖 SimpleClaw Status
...
Cost:
  Today:    $1.23 / $10.00 (12.3%)
  Month:    $15.67 / $100.00 (15.7%)
  Session:  $0.45 (38 calls)
  Top models:
    claude-sonnet-4-20250514  $12.34 (78.7%)
    gpt-4o-mini               $3.33 (21.3%)
```

## 涉及文件

| 操作 | 文件 | 说明 |
|------|------|------|
| 新建 | `providers/cost.go` | 核心类型定义 |
| 新建 | `providers/cost_storage.go` | JSONL 持久化存储 |
| 新建 | `providers/cost_tracker.go` | CostTracker + 预算检查 |
| 新建 | `providers/pricing.go` | 模型费率表 |
| 新建 | `providers/cost_test.go` | 单元测试 |
| 修改 | `providers/fallback.go` | 集成预算检查和费用记录 |
| 修改 | `config/schema.go` | 新增 CostConfig |
| 修改 | `cmd/status.go` | 展示费用统计 |

## 配置扩展

```json
{
  "cost": {
    "enabled": true,
    "dailyLimitUsd": 10.0,
    "monthlyLimitUsd": 100.0,
    "warnAtPercent": 80
  }
}
```

## 验收标准

- [ ] 每次 LLM 调用后费用记录写入 JSONL
- [ ] 超出日/月限额时返回明确错误而非继续调用
- [ ] 预警阈值触发 warning 日志
- [ ] `simpleclaw status` 展示当日/当月费用
- [ ] 按日分文件存储，历史可追溯
- [ ] 未知模型不阻断调用（费用记为 0）

## 工期估算

**总计：2.5 周（1 人）**

| 阶段 | 工期 |
|------|------|
| 数据模型 + 存储 | 0.5 周 |
| CostTracker + 预算检查 | 1 周 |
| 集成到 FallbackChain | 0.5 周 |
| CLI 展示 | 0.5 周 |

## 风险与缓解

| 风险 | 缓解措施 |
|------|---------|
| 费率表过时 | 费率表作为常量维护，定期更新；未知模型不阻断 |
| JSONL 文件增长 | 按日分文件；可添加月度自动归档（合并为月汇总） |
| 预估与实际偏差大 | 预估用于事前检查，事后按实际 token 记录 |
