// ErrorClassifier 对 LLM API 错误进行分类，决定是否可重试及冷却策略。
// 支持 6 种错误类型：auth / rate_limit / billing / timeout / format / overloaded。

package providers

import (
	"fmt"
	"regexp"
	"strings"
)

// FailoverReason 描述 Provider 故障转移的原因。
type FailoverReason string

const (
	FailoverAuth       FailoverReason = "auth"       // API Key 无效、未授权
	FailoverRateLimit  FailoverReason = "rate_limit"  // 请求频率超限
	FailoverBilling    FailoverReason = "billing"     // 余额不足、账单问题
	FailoverTimeout    FailoverReason = "timeout"     // 请求超时
	FailoverFormat     FailoverReason = "format"      // 请求格式错误（不可重试）
	FailoverOverloaded FailoverReason = "overloaded"  // 服务过载
	FailoverUnknown    FailoverReason = "unknown"     // 未知错误
)

// IsRetriable 返回此类错误是否值得重试。
// 只有 format 错误不可重试（请求本身有问题，换 Provider 也没用）。
func (r FailoverReason) IsRetriable() bool {
	return r != FailoverFormat
}

// ProviderError 是包含 HTTP 状态码的 Provider 错误。
type ProviderError struct {
	StatusCode int
	Message    string
	ModelKey   string
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("provider error [%s] (status %d): %s", e.ModelKey, e.StatusCode, e.Message)
}

// FallbackExhaustedError 在所有候选 Provider 都失败时返回。
type FallbackExhaustedError struct {
	Attempts []Attempt
}

// Attempt 记录单次 Provider 尝试的结果。
type Attempt struct {
	ModelKey string
	Error    error
	Reason   FailoverReason
	Skipped  bool   // 因冷却跳过
}

func (e *FallbackExhaustedError) Error() string {
	var parts []string
	for _, a := range e.Attempts {
		if a.Skipped {
			parts = append(parts, fmt.Sprintf("%s: skipped (cooldown)", a.ModelKey))
		} else {
			parts = append(parts, fmt.Sprintf("%s: %v (%s)", a.ModelKey, a.Error, a.Reason))
		}
	}
	return fmt.Sprintf("all providers failed: [%s]", strings.Join(parts, "; "))
}

// ErrorClassifier 对 LLM API 错误进行分类。
type ErrorClassifier struct {
	patterns []classificationPattern
}

type classificationPattern struct {
	reason  FailoverReason
	pattern *regexp.Regexp
}

// NewErrorClassifier 创建一个带默认模式的错误分类器。
func NewErrorClassifier() *ErrorClassifier {
	return &ErrorClassifier{patterns: defaultPatterns()}
}

// Classify 对错误进行分类，返回 FailoverReason。
func (c *ErrorClassifier) Classify(err error) FailoverReason {
	if err == nil {
		return FailoverUnknown
	}

	// 优先检查 ProviderError 的 HTTP 状态码
	if pe, ok := err.(*ProviderError); ok {
		if reason := classifyStatusCode(pe.StatusCode); reason != FailoverUnknown {
			return reason
		}
	}

	// 消息模式匹配
	msg := strings.ToLower(err.Error())
	for _, p := range c.patterns {
		if p.pattern.MatchString(msg) {
			return p.reason
		}
	}

	return FailoverUnknown
}

// classifyStatusCode 根据 HTTP 状态码分类。
func classifyStatusCode(code int) FailoverReason {
	switch code {
	case 401, 403:
		return FailoverAuth
	case 429:
		return FailoverRateLimit
	case 402:
		return FailoverBilling
	case 408:
		return FailoverTimeout
	case 400, 422:
		return FailoverFormat
	case 500, 502, 503:
		return FailoverOverloaded
	}
	// 瞬态 HTTP 错误
	if code >= 521 && code <= 524 || code == 529 {
		return FailoverOverloaded
	}
	return FailoverUnknown
}

// defaultPatterns 返回默认的错误分类模式（按优先级排序）。
func defaultPatterns() []classificationPattern {
	return []classificationPattern{
		// rate_limit（最高优先级）
		{FailoverRateLimit, regexp.MustCompile(`(?i)rate.?limit|too many requests|quota exceeded|requests? per (minute|second|day)|throttl`)},
		{FailoverRateLimit, regexp.MustCompile(`(?i)retry.?after|slow down|capacity|concurrent`)},

		// overloaded
		{FailoverOverloaded, regexp.MustCompile(`(?i)overloaded|server.?busy|temporarily unavailable|service.?unavail`)},
		{FailoverOverloaded, regexp.MustCompile(`(?i)bad gateway|gateway.?timeout|upstream|backend`)},

		// billing
		{FailoverBilling, regexp.MustCompile(`(?i)insufficient.?(funds?|credits?|balance)|billing|payment|exceed.*(usage|budget)`)},
		{FailoverBilling, regexp.MustCompile(`(?i)plan.?limit|subscription|prepaid`)},

		// timeout
		{FailoverTimeout, regexp.MustCompile(`(?i)timed?\s*out|deadline|context.?(canceled|deadline)`)},

		// auth
		{FailoverAuth, regexp.MustCompile(`(?i)invalid.?(api.?key|token|auth)|unauthorized|forbidden|access.?denied`)},
		{FailoverAuth, regexp.MustCompile(`(?i)authentication|permission|credential`)},

		// format（不可重试）
		{FailoverFormat, regexp.MustCompile(`(?i)invalid.?(json|request|param)|malformed|bad.?request|missing.?field`)},
		{FailoverFormat, regexp.MustCompile(`(?i)unsupported.?(model|format|media)|content.?policy|moderation`)},
	}
}
