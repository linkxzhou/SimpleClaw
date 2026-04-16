// 模型费率表：主流 LLM 每百万 token 价格（美元）。
// 未知模型不阻断调用（费用记为 0）。

package providers

import "strings"

// ModelPrice 每百万 token 的输入/输出费率（美元）。
type ModelPrice struct {
	Input  float64 // 每百万 input token
	Output float64 // 每百万 output token
}

// modelPricing 主流模型费率表。
// 键为规范化后的模型名（不含 vendor/ 前缀）。
var modelPricing = map[string]ModelPrice{
	// Anthropic
	"claude-sonnet-4-20250514": {3.0, 15.0},
	"claude-opus-4-20250514":   {15.0, 75.0},
	"claude-3-5-sonnet":        {3.0, 15.0},
	"claude-3-5-haiku":         {0.8, 4.0},
	"claude-3-haiku":           {0.25, 1.25},
	// OpenAI
	"gpt-4o":      {2.5, 10.0},
	"gpt-4o-mini": {0.15, 0.6},
	"gpt-4-turbo": {10.0, 30.0},
	"o1":          {15.0, 60.0},
	"o1-mini":     {3.0, 12.0},
	"o3-mini":     {1.1, 4.4},
	// DeepSeek
	"deepseek-chat":     {0.14, 0.28},
	"deepseek-reasoner": {0.55, 2.19},
	// Google
	"gemini-2.0-flash": {0.1, 0.4},
	"gemini-2.0-pro":   {1.25, 10.0},
	"gemini-1.5-pro":   {1.25, 5.0},
	"gemini-1.5-flash": {0.075, 0.3},
	// Groq (推理加速，按 Groq 定价)
	"llama-3.3-70b":  {0.59, 0.79},
	"llama-3.1-8b":   {0.05, 0.08},
	"mixtral-8x7b":   {0.24, 0.24},
	"gemma2-9b-it":   {0.2, 0.2},
	// Zhipu
	"glm-4-plus": {0.7, 0.7},
	"glm-4":      {1.4, 1.4},
}

// EstimateCost 根据模型名和 token 数估算费用（美元）。
// 未知模型返回 0（不阻断调用）。
func EstimateCost(model string, inputTokens, outputTokens int) float64 {
	price, ok := lookupPrice(model)
	if !ok {
		return 0
	}
	return float64(inputTokens)/1e6*price.Input + float64(outputTokens)/1e6*price.Output
}

// lookupPrice 查找模型费率，支持模糊匹配。
func lookupPrice(model string) (ModelPrice, bool) {
	norm := normalizeModelName(model)

	// 精确匹配
	if p, ok := modelPricing[norm]; ok {
		return p, true
	}

	// 前缀匹配（处理版本后缀，如 gpt-4o-2024-08-06）
	for key, p := range modelPricing {
		if strings.HasPrefix(norm, key) {
			return p, true
		}
	}

	return ModelPrice{}, false
}

// normalizeModelName 规范化模型名：移除 vendor/ 前缀，转小写。
func normalizeModelName(model string) string {
	// 移除 "vendor/" 前缀
	if idx := strings.Index(model, "/"); idx >= 0 {
		model = model[idx+1:]
	}
	return strings.ToLower(model)
}
