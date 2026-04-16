// 记忆时间衰减：对非 Core 类别的条目应用指数衰减。
// 公式: score * 2^(-age_days / half_life_days)
// Core 记忆永不衰减（半衰期 = 0 表示跳过）。

package memory

import (
	"math"
	"sort"
	"time"

	"github.com/linkxzhou/SimpleClaw/utils"
)

// ApplyTimeDecay 对条目列表就地应用时间衰减。
// Core 类别跳过；其他类别按各自半衰期衰减。
func ApplyTimeDecay(entries []MemoryEntry) {
	now := time.Now()

	for i := range entries {
		e := &entries[i]

		// Core 永不衰减
		halfLife := CategoryHalfLifeDays[e.Category]
		if halfLife <= 0 {
			continue
		}

		// 确保有初始分数
		if e.Score == nil {
			s := e.InitialScore()
			e.Score = &s
		}

		ts, err := time.Parse(time.RFC3339, e.Timestamp)
		if err != nil {
			continue
		}

		ageDays := now.Sub(ts).Hours() / 24.0
		if ageDays < 0 {
			ageDays = 0
		}

		decayFactor := math.Exp(-ageDays / halfLife * math.Ln2)
		newScore := e.InitialScore() * decayFactor
		e.Score = &newScore
	}
}

// SortByScore 按分数降序排序条目。
func SortByScore(entries []MemoryEntry) {
	sort.Slice(entries, func(i, j int) bool {
		si, sj := 0.0, 0.0
		if entries[i].Score != nil {
			si = *entries[i].Score
		}
		if entries[j].Score != nil {
			sj = *entries[j].Score
		}
		return si > sj
	})
}

// SelectByTokenBudget 按 token 预算选择记忆条目。
// 条目应已按分数降序排列。返回在 budget 内的条目。
func SelectByTokenBudget(entries []MemoryEntry, budget int) []MemoryEntry {
	if budget <= 0 {
		return entries
	}

	var result []MemoryEntry
	totalTokens := 0
	for _, e := range entries {
		tokens := utils.EstimateTokens(e.Content) + 10 // +10 为类别/时间戳前缀开销
		if totalTokens+tokens > budget {
			break
		}
		totalTokens += tokens
		result = append(result, e)
	}
	return result
}
