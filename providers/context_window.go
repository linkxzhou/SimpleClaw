// 模型上下文窗口大小映射。
// 用于自动截断时确定 token 预算。

package providers

// contextWindowSizes 主流模型的上下文窗口大小（token 数）。
var contextWindowSizes = map[string]int{
	// Anthropic
	"claude-sonnet-4-20250514": 200000,
	"claude-opus-4-20250514":   200000,
	"claude-3-5-sonnet":        200000,
	"claude-3-5-haiku":         200000,
	"claude-3-haiku":           200000,
	// OpenAI
	"gpt-4o":      128000,
	"gpt-4o-mini": 128000,
	"gpt-4-turbo": 128000,
	"o1":          200000,
	"o1-mini":     128000,
	"o3-mini":     200000,
	// DeepSeek
	"deepseek-chat":     64000,
	"deepseek-reasoner": 64000,
	// Google
	"gemini-2.0-flash": 1048576,
	"gemini-2.0-pro":   1048576,
	"gemini-1.5-pro":   2097152,
	"gemini-1.5-flash": 1048576,
	// Groq
	"llama-3.3-70b": 131072,
	"llama-3.1-8b":  131072,
	"mixtral-8x7b":  32768,
	"gemma2-9b-it":  8192,
	// Zhipu
	"glm-4-plus": 128000,
	"glm-4":      128000,
}

const defaultContextWindow = 8192 // 保守默认值

// GetContextWindow 返回模型的上下文窗口大小（token 数）。
// 支持 "vendor/model" 格式和前缀匹配。未知模型返回 8192。
func GetContextWindow(model string) int {
	norm := normalizeModelName(model)

	// 精确匹配
	if size, ok := contextWindowSizes[norm]; ok {
		return size
	}

	// 前缀匹配
	for key, size := range contextWindowSizes {
		if len(norm) > len(key) && norm[:len(key)] == key {
			return size
		}
	}

	return defaultContextWindow
}
