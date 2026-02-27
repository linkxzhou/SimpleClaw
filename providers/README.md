# providers — LLM 提供商抽象层

`providers` 包提供 SimpleClaw 的 LLM 提供商抽象，包括统一接口、工厂模式创建、多 Provider 容错链和错误分类。

## 文件结构

| 文件 | 说明 |
|------|------|
| `base.go` | 核心接口定义（`LLMProvider`、`ChatRequest`、`LLMResponse`） |
| `openai.go` | OpenAI 兼容 API 实现（支持所有兼容厂商） |
| `factory.go` | Provider 工厂 — 按 `vendor/model` 格式创建 Provider |
| `fallback.go` | FallbackChain — 多 Provider 容错执行链 |
| `errors.go` | ErrorClassifier — 错误分类与重试决策 |
| `cooldown.go` | CooldownTracker — 智能冷却退避追踪 |
| `transcription.go` | 语音转写接口与 Groq Whisper 实现 |
| `providers_test.go` | 单元测试 |

## 核心接口

### LLMProvider

所有 LLM 后端必须实现的统一接口：

```go
type LLMProvider interface {
    Chat(ctx context.Context, req ChatRequest) (*LLMResponse, error)
    GetDefaultModel() string
}
```

### ChatRequest / LLMResponse

```go
type ChatRequest struct {
    Messages    []map[string]interface{} // 消息列表
    Tools       []map[string]interface{} // 工具定义（可选）
    Model       string                   // 模型名称
    MaxTokens   int                      // 最大 token 数
    Temperature float64                  // 采样温度
}

type LLMResponse struct {
    Content      string            // 文本响应
    ToolCalls    []ToolCallRequest // 工具调用列表
    FinishReason string            // 结束原因（stop / tool_calls / error）
    Usage        map[string]int    // Token 使用统计
}
```

## 核心组件

### OpenAIProvider

兼容所有 OpenAI API 格式的 LLM 服务，通过 HTTP POST 调用 `/chat/completions` 端点。

支持的厂商：OpenAI、Anthropic、DeepSeek、智谱、Groq、Gemini、OpenRouter、Moonshot、通义千问、Nvidia、Cerebras、Ollama、vLLM。

### ProviderFactory

根据 `vendor/model` 格式（如 `anthropic/claude-sonnet-4-20250514`）自动创建对应的 Provider：

```go
factory := providers.NewProviderFactory(providers.FactoryConfig{
    APIKeys:   map[string]string{"anthropic": "sk-xxx"},
    Endpoints: map[string]string{"anthropic": "https://custom.api/v1"},
    Timeout:   120 * time.Second,
})
provider, _ := factory.CreateProvider("anthropic/claude-sonnet-4-20250514")
```

`SplitModelKey("anthropic/claude-sonnet-4-20250514")` → `vendor="anthropic"`, `model="claude-sonnet-4-20250514"`

### FallbackChain — 容错执行链

按优先级逐个尝试候选 Provider，自动跳过冷却中的 Provider，对错误分类后决定是否重试：

```
Primary → Fallback1 → Fallback2 → ... → FallbackExhaustedError
```

关键方法：

- `ResolveCandidates(primary, fallbacks)` — 解析去重候选列表
- `Execute(ctx, candidates, req)` — 执行容错链
- `ExecuteDirect(ctx, modelKey, req)` — 单 Provider 直接执行

### ErrorClassifier — 错误分类

根据 HTTP 状态码和错误消息模式匹配，将错误分为 7 类：

| 错误类型 | HTTP 状态码 | 可重试 | 说明 |
|---------|------------|--------|------|
| `auth` | 401, 403 | 是 | API Key 无效 |
| `rate_limit` | 429 | 是 | 频率超限 |
| `billing` | 402 | 是 | 余额不足 |
| `timeout` | 408 | 是 | 请求超时 |
| `overloaded` | 500, 502, 503 | 是 | 服务过载 |
| `format` | 400, 422 | **否** | 请求格式错误 |
| `unknown` | — | 是 | 未知错误 |

### CooldownTracker — 冷却退避

对失败的 Provider 实施智能退避：

- **标准错误**：指数退避 `1min → 5min → 25min → 60min（上限）`
- **计费错误**：长退避 `5h → 10h → 20h → 24h（上限）`
- 成功后立即重置冷却状态
- 24 小时无故障窗口自动重置

### TranscriptionProvider — 语音转写

`TranscriptionProvider` 接口：

```go
type TranscriptionProvider interface {
    Transcribe(ctx context.Context, filePath string) (string, error)
}
```

内置 `GroqTranscriptionProvider` 实现，使用 Groq 的 Whisper Large V3 模型。

## 内置端点

| Vendor | 默认端点 |
|--------|---------|
| openai | `https://api.openai.com/v1` |
| anthropic | `https://api.anthropic.com/v1` |
| deepseek | `https://api.deepseek.com/v1` |
| zhipu | `https://open.bigmodel.cn/api/paas/v4` |
| groq | `https://api.groq.com/openai/v1` |
| gemini | `https://generativelanguage.googleapis.com/v1beta/openai` |
| openrouter | `https://openrouter.ai/api/v1` |
| moonshot | `https://api.moonshot.cn/v1` |
| qwen | `https://dashscope.aliyuncs.com/compatible-mode/v1` |
| ollama | `http://localhost:11434/v1` |
| vllm | `http://localhost:8000/v1` |
