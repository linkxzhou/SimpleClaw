// CooldownTracker 对失败的 LLM Provider 实施智能冷却退避。
// 标准错误使用指数退避（1min → 5min → 25min → 60min cap），
// 计费错误使用长退避（5h → 10h → 20h → 24h cap）。

package providers

import (
	"math"
	"sync"
	"time"
)

// CooldownTracker 追踪 Provider 的冷却状态。
type CooldownTracker struct {
	mu      sync.RWMutex
	entries map[string]*cooldownEntry
}

type cooldownEntry struct {
	failCount   int
	lastFailure time.Time
	lastSuccess time.Time
	reason      FailoverReason
	cooldownEnd time.Time
}

// NewCooldownTracker 创建一个新的冷却追踪器。
func NewCooldownTracker() *CooldownTracker {
	return &CooldownTracker{
		entries: make(map[string]*cooldownEntry),
	}
}

// IsOnCooldown 检查指定 modelKey 是否在冷却中。
// 返回是否冷却中，以及剩余冷却时间。
func (ct *CooldownTracker) IsOnCooldown(modelKey string) (bool, time.Duration) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	entry, ok := ct.entries[modelKey]
	if !ok {
		return false, 0
	}

	// 24 小时无故障窗口自动重置
	if !entry.lastSuccess.IsZero() && time.Since(entry.lastSuccess) > 24*time.Hour {
		return false, 0
	}

	remaining := time.Until(entry.cooldownEnd)
	if remaining <= 0 {
		return false, 0
	}
	return true, remaining
}

// MarkFailure 标记 Provider 失败，根据 reason 计算冷却时间。
func (ct *CooldownTracker) MarkFailure(modelKey string, reason FailoverReason) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	entry, ok := ct.entries[modelKey]
	if !ok {
		entry = &cooldownEntry{}
		ct.entries[modelKey] = entry
	}

	entry.failCount++
	entry.lastFailure = time.Now()
	entry.reason = reason

	var cooldown time.Duration
	if reason == FailoverBilling {
		// 计费错误长退避: min(24h, 5h * 2^min(n-1, 10))
		n := entry.failCount - 1
		if n > 10 {
			n = 10
		}
		cooldown = time.Duration(5*math.Pow(2, float64(n))) * time.Hour
		if cooldown > 24*time.Hour {
			cooldown = 24 * time.Hour
		}
	} else {
		// 标准指数退避: min(1h, 1min * 5^min(n-1, 3))
		n := entry.failCount - 1
		if n > 3 {
			n = 3
		}
		cooldown = time.Duration(math.Pow(5, float64(n))) * time.Minute
		if cooldown > time.Hour {
			cooldown = time.Hour
		}
	}

	entry.cooldownEnd = time.Now().Add(cooldown)
}

// MarkSuccess 标记 Provider 成功，立即重置所有冷却状态。
func (ct *CooldownTracker) MarkSuccess(modelKey string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	entry, ok := ct.entries[modelKey]
	if !ok {
		ct.entries[modelKey] = &cooldownEntry{lastSuccess: time.Now()}
		return
	}

	entry.failCount = 0
	entry.lastSuccess = time.Now()
	entry.cooldownEnd = time.Time{} // 清除冷却
	entry.reason = ""
}

// Reset 清除所有冷却状态。
func (ct *CooldownTracker) Reset() {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.entries = make(map[string]*cooldownEntry)
}
