// FallbackChain 实现多 Provider 容错执行链。
// 按优先级尝试候选 Provider，自动跳过冷却中的 Provider，
// 对错误进行分类并决定是否重试。

package providers

import (
	"context"
	"fmt"
	"log/slog"
)

// Candidate 表示一个候选 Provider。
type Candidate struct {
	ModelKey string
	Provider LLMProvider
}

// FallbackChain 容错执行链。
type FallbackChain struct {
	classifier  *ErrorClassifier
	cooldown    *CooldownTracker
	factory     *ProviderFactory
	costTracker *CostTracker // 可选，为 nil 时不做费用追踪
	logger      *slog.Logger
}

// NewFallbackChain 创建容错执行链。
func NewFallbackChain(classifier *ErrorClassifier, cooldown *CooldownTracker, factory *ProviderFactory, logger *slog.Logger) *FallbackChain {
	if logger == nil {
		logger = slog.Default()
	}
	return &FallbackChain{
		classifier: classifier,
		cooldown:   cooldown,
		factory:    factory,
		logger:     logger,
	}
}

// SetCostTracker 设置费用追踪器（可选）。
func (fc *FallbackChain) SetCostTracker(ct *CostTracker) {
	fc.costTracker = ct
}

// ResolveCandidates 从 primary + fallbacks 解析去重候选列表。
func (fc *FallbackChain) ResolveCandidates(primary string, fallbacks []string) ([]Candidate, error) {
	seen := make(map[string]bool)
	var candidates []Candidate

	all := append([]string{primary}, fallbacks...)
	for _, modelKey := range all {
		if modelKey == "" || seen[modelKey] {
			continue
		}
		seen[modelKey] = true

		provider, err := fc.factory.CreateProvider(modelKey)
		if err != nil {
			fc.logger.Warn("skip candidate: cannot create provider", "model", modelKey, "error", err)
			continue
		}

		candidates = append(candidates, Candidate{
			ModelKey: modelKey,
			Provider: provider,
		})
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no valid candidates from primary=%s fallbacks=%v", primary, fallbacks)
	}

	return candidates, nil
}

// Execute 按优先级逐个尝试候选 Provider，返回第一个成功的响应。
func (fc *FallbackChain) Execute(ctx context.Context, candidates []Candidate, req ChatRequest) (*LLMResponse, error) {
	// 预算检查（调用前）
	if fc.costTracker != nil && fc.costTracker.IsEnabled() {
		estimated := EstimateCost(req.Model, estimateInputTokens(req), req.MaxTokens/2)
		switch fc.costTracker.CheckBudget(estimated) {
		case BudgetExceeded:
			s := fc.costTracker.GetSummary()
			return nil, fmt.Errorf("budget exceeded: daily $%.2f/$%.2f, monthly $%.2f/$%.2f",
				s.DailyCostUSD, s.DailyLimitUSD, s.MonthlyCostUSD, s.MonthlyLimitUSD)
		case BudgetWarning:
			fc.logger.Warn("cost warning: approaching budget limit")
		}
	}

	var attempts []Attempt

	for _, candidate := range candidates {
		// 1. 检查 context 取消（不回退，直接返回）
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// 2. 检查冷却
		if onCooldown, remaining := fc.cooldown.IsOnCooldown(candidate.ModelKey); onCooldown {
			fc.logger.Debug("skip candidate: on cooldown",
				"model", candidate.ModelKey,
				"remaining", remaining.String())
			attempts = append(attempts, Attempt{
				ModelKey: candidate.ModelKey,
				Skipped:  true,
			})
			continue
		}

		// 3. 执行请求
		// 确保使用候选模型（覆盖请求中的 model）
		reqCopy := req
		_, model := SplitModelKey(candidate.ModelKey)
		reqCopy.Model = model

		resp, err := candidate.Provider.Chat(ctx, reqCopy)
		if err == nil {
			// 检查是否为 error finish reason（非 HTTP 错误但 LLM 返回错误）
			if resp.FinishReason == "error" {
				err = &ProviderError{
					StatusCode: 0,
					Message:    resp.Content,
					ModelKey:   candidate.ModelKey,
				}
			} else {
				fc.cooldown.MarkSuccess(candidate.ModelKey)
				fc.recordCost(candidate.ModelKey, resp)
				return resp, nil
			}
		}

		// 4. 分类错误
		reason := fc.classifier.Classify(err)
		fc.logger.Warn("candidate failed",
			"model", candidate.ModelKey,
			"reason", string(reason),
			"error", err)

		// 5. 不可重试 → 立即返回
		if !reason.IsRetriable() {
			return nil, fmt.Errorf("[%s] %w (non-retriable: %s)", candidate.ModelKey, err, reason)
		}

		// 6. 标记冷却 → 尝试下一个
		fc.cooldown.MarkFailure(candidate.ModelKey, reason)
		attempts = append(attempts, Attempt{
			ModelKey: candidate.ModelKey,
			Error:    err,
			Reason:   reason,
		})
	}

	return nil, &FallbackExhaustedError{Attempts: attempts}
}

// ExecuteDirect 使用单个 Provider 直接执行请求（无容错，用于 CLI 模式）。
func (fc *FallbackChain) ExecuteDirect(ctx context.Context, modelKey string, req ChatRequest) (*LLMResponse, error) {
	provider, err := fc.factory.CreateProvider(modelKey)
	if err != nil {
		return nil, err
	}

	_, model := SplitModelKey(modelKey)
	req.Model = model
	resp, err := provider.Chat(ctx, req)
	if err == nil {
		fc.recordCost(modelKey, resp)
	}
	return resp, err
}

// recordCost 记录一次调用的实际费用。
func (fc *FallbackChain) recordCost(modelKey string, resp *LLMResponse) {
	if fc.costTracker == nil || resp == nil {
		return
	}
	inputTokens, _ := resp.Usage["prompt_tokens"]
	outputTokens, _ := resp.Usage["completion_tokens"]
	if inputTokens == 0 && outputTokens == 0 {
		return
	}

	cost := EstimateCost(modelKey, inputTokens, outputTokens)
	_ = fc.costTracker.RecordUsage(TokenUsage{
		Model:        modelKey,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  inputTokens + outputTokens,
		CostUSD:      cost,
	}, "")
}

// estimateInputTokens 粗估输入 token 数（按平均每条消息 100 token）。
func estimateInputTokens(req ChatRequest) int {
	return len(req.Messages) * 100
}

// GetCostTracker 返回关联的 CostTracker（可能为 nil）。
func (fc *FallbackChain) GetCostTracker() *CostTracker {
	return fc.costTracker
}
