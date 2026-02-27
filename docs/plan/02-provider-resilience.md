# SimpleClaw 重构方案 — Provider 容错体系

## 1. 概述

当前 SimpleClaw 的 `providers/openai.go` 实现了单一 OpenAI 兼容 API 调用，没有任何容错机制。参考 PicoClaw 的三层容错设计（FallbackChain + CooldownTracker + ErrorClassifier），结合 Nanobot 的 LiteLLM 统一接口思想，设计适合 SimpleClaw 的 Provider 容错体系。

---

## 2. ErrorClassifier — 错误分类器

### 2.1 设计目标

将 LLM API 返回的错误分类为 6 种类型，决定是否可重试、是否应冷却、冷却策略。

### 2.2 错误分类

```go
// providers/errors.go

type FailoverReason string

const (
    FailoverAuth       FailoverReason = "auth"        // API Key 无效、未授权
    FailoverRateLimit  FailoverReason = "rate_limit"   // 请求频率超限
    FailoverBilling    FailoverReason = "billing"      // 余额不足、账单问题
    FailoverTimeout    FailoverReason = "timeout"      // 请求超时
    FailoverFormat     FailoverReason = "format"       // 请求格式错误（不可重试）
    FailoverOverloaded FailoverReason = "overloaded"   // 服务过载
)
```

### 2.3 分类策略

**HTTP 状态码优先**：

| 状态码 | 分类 |
|--------|------|
| 401 | auth |
| 403 | auth |
| 429 | rate_limit |
| 402 | billing |
| 408 | timeout |
| 400 | format |
| 422 | format |
| 500, 502, 503 | overloaded |
| 521-524, 529 | overloaded |

**消息模式匹配（~40条正则）**：

```go
var classificationPatterns = []struct {
    reason  FailoverReason
    pattern *regexp.Regexp
}{
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
```

### 2.4 重试判定

```go
func (r FailoverReason) IsRetriable() bool {
    return r != FailoverFormat // 只有 format 错误不可重试
}
```

---

## 3. CooldownTracker — 冷却追踪器

### 3.1 设计目标

对失败的 Provider 实施智能冷却，避免反复请求已知故障的服务。

### 3.2 冷却策略

**标准指数退避**（rate_limit / timeout / overloaded / auth）：
```
冷却时间 = min(1h, 1min × 5^min(n-1, 3))
第1次失败: 1min
第2次失败: 5min
第3次失败: 25min
第4次及以后: 60min (cap)
```

**计费错误退避**（billing）：
```
冷却时间 = min(24h, 5h × 2^min(n-1, 10))
第1次失败: 5h
第2次失败: 10h
第3次失败: 20h
第4次及以后: 24h (cap)
```

### 3.3 数据结构

```go
// providers/cooldown.go

type CooldownTracker struct {
    mu      sync.RWMutex
    entries map[string]*cooldownEntry // key = "vendor/model"
}

type cooldownEntry struct {
    failCount    int
    lastFailure  time.Time
    lastSuccess  time.Time
    reason       FailoverReason
    cooldownEnd  time.Time
}
```

### 3.4 核心方法

```go
// 检查是否在冷却中
func (ct *CooldownTracker) IsOnCooldown(modelKey string) (bool, time.Duration)

// 标记失败（根据 reason 计算冷却时间）
func (ct *CooldownTracker) MarkFailure(modelKey string, reason FailoverReason)

// 标记成功（立即重置所有冷却状态）
func (ct *CooldownTracker) MarkSuccess(modelKey string)

// 24小时无故障窗口自动重置
func (ct *CooldownTracker) autoReset(entry *cooldownEntry)
```

---

## 4. FallbackChain — 容错执行链

### 4.1 设计目标

支持配置多个备选 Provider/Model，按优先级顺序尝试，自动跳过冷却中的 Provider。

### 4.2 配置模型

```json
{
  "agents": {
    "defaults": {
      "model": "anthropic/claude-sonnet-4-20250514"
    },
    "list": [
      {
        "id": "main",
        "model": "anthropic/claude-sonnet-4-20250514",
        "fallbacks": [
          "openai/gpt-4o",
          "deepseek/deepseek-chat",
          "zhipu/glm-4"
        ]
      }
    ]
  }
}
```

### 4.3 执行流程

```go
// providers/fallback.go

type FallbackChain struct {
    classifier *ErrorClassifier
    cooldown   *CooldownTracker
    factory    *ProviderFactory
}

type Candidate struct {
    ModelKey string        // "vendor/model"
    Provider LLMProvider
}

func (fc *FallbackChain) Execute(ctx context.Context, candidates []Candidate, request *ChatRequest) (*LLMResponse, error) {
    var attempts []Attempt

    for _, candidate := range candidates {
        // 1. 检查 context 取消
        if ctx.Err() != nil {
            return nil, ctx.Err()  // 不回退
        }

        // 2. 检查冷却
        if onCooldown, remaining := fc.cooldown.IsOnCooldown(candidate.ModelKey); onCooldown {
            attempts = append(attempts, Attempt{ModelKey: candidate.ModelKey, Skipped: true, Reason: "cooldown", Remaining: remaining})
            continue
        }

        // 3. 执行请求
        resp, err := candidate.Provider.Chat(ctx, request)
        if err == nil {
            fc.cooldown.MarkSuccess(candidate.ModelKey)
            return resp, nil
        }

        // 4. 分类错误
        reason := fc.classifier.Classify(err)

        // 5. 不可重试 → 立即返回
        if !reason.IsRetriable() {
            return nil, fmt.Errorf("[%s] %w (non-retriable: %s)", candidate.ModelKey, err, reason)
        }

        // 6. 标记冷却 → 尝试下一个
        fc.cooldown.MarkFailure(candidate.ModelKey, reason)
        attempts = append(attempts, Attempt{ModelKey: candidate.ModelKey, Error: err, Reason: reason})
    }

    return nil, &FallbackExhaustedError{Attempts: attempts}
}
```

---

## 5. ProviderFactory — Provider 工厂

### 5.1 设计目标

根据 `vendor/model` 格式的 modelKey 自动创建对应的 Provider 实例。

### 5.2 路由规则

```go
// providers/factory.go

func (f *ProviderFactory) CreateProvider(modelKey string) (LLMProvider, error) {
    vendor, model := splitModelKey(modelKey)

    switch vendor {
    case "anthropic":
        return NewOpenAIProvider(anthropicEndpoint, apiKey, model), nil
    case "openai":
        return NewOpenAIProvider(openaiEndpoint, apiKey, model), nil
    case "deepseek":
        return NewOpenAIProvider(deepseekEndpoint, apiKey, model), nil
    case "zhipu":
        return NewOpenAIProvider(zhipuEndpoint, apiKey, model), nil
    case "groq":
        return NewOpenAIProvider(groqEndpoint, apiKey, model), nil
    case "gemini":
        return NewOpenAIProvider(geminiEndpoint, apiKey, model), nil
    case "openrouter":
        return NewOpenAIProvider(openrouterEndpoint, apiKey, model), nil
    case "ollama":
        return NewOpenAIProvider(ollamaEndpoint, "", model), nil
    case "vllm":
        return NewOpenAIProvider(vllmEndpoint, apiKey, model), nil
    // ... 更多 Provider
    default:
        return nil, fmt.Errorf("unknown vendor: %s", vendor)
    }
}
```

### 5.3 关键设计：复用 OpenAIProvider

当前 SimpleClaw 的 `OpenAIProvider` 已经实现了通用 OpenAI 兼容 API，大多数 LLM 厂商都兼容此协议。因此：
- **不需要** 为每个厂商写独立 Provider
- 只需不同的 `baseURL` + `apiKey` + `model`
- 保持代码量最小化

### 5.4 Endpoint 配置表

```go
var defaultEndpoints = map[string]string{
    "openai":      "https://api.openai.com/v1",
    "anthropic":   "https://api.anthropic.com/v1",
    "deepseek":    "https://api.deepseek.com/v1",
    "zhipu":       "https://open.bigmodel.cn/api/paas/v4",
    "groq":        "https://api.groq.com/openai/v1",
    "gemini":      "https://generativelanguage.googleapis.com/v1beta/openai",
    "openrouter":  "https://openrouter.ai/api/v1",
    "moonshot":    "https://api.moonshot.cn/v1",
    "qwen":        "https://dashscope.aliyuncs.com/compatible-mode/v1",
    "ollama":      "http://localhost:11434/v1",
    "vllm":        "http://localhost:8000/v1",
}
```

---

## 6. 集成到现有代码

### 6.1 修改 `providers/openai.go`

```go
// 增强 OpenAIProvider 构造函数
func NewOpenAIProvider(baseURL, apiKey, model string) *OpenAIProvider {
    return &OpenAIProvider{
        baseURL: baseURL,
        apiKey:  apiKey,
        model:   model,
        client:  &http.Client{Timeout: 120 * time.Second},
    }
}

// 增强 Chat 方法：返回结构化错误（含 HTTP 状态码）
func (p *OpenAIProvider) Chat(ctx context.Context, req *ChatRequest) (*LLMResponse, error) {
    // ... 现有逻辑 ...
    // 错误时包装 HTTP 状态码信息
    if resp.StatusCode != http.StatusOK {
        return nil, &ProviderError{
            StatusCode: resp.StatusCode,
            Message:    string(body),
            ModelKey:   p.model,
        }
    }
}
```

### 6.2 修改 `cmd/gateway.go`

```go
// 旧代码
provider := providers.NewOpenAIProvider(cfg)

// 新代码
factory := providers.NewProviderFactory(cfg)
cooldown := providers.NewCooldownTracker()
classifier := providers.NewErrorClassifier()
fallback := providers.NewFallbackChain(classifier, cooldown, factory)

// AgentLoop 使用 FallbackChain 替代直接 Provider
agent := agent.New(bus, fallback, cfg)
```

### 6.3 修改 `agent/agent.go`

```go
// 旧代码
func (a *Agent) agentLoop(messages []Message) (string, error) {
    resp, err := a.provider.Chat(ctx, req)
    // ...
}

// 新代码
func (a *Agent) agentLoop(messages []Message) (string, error) {
    candidates := a.fallbackChain.ResolveCandidates(a.agentConfig)
    resp, err := a.fallbackChain.Execute(ctx, candidates, req)
    // ...
}
```

---

## 7. 实施计划

| 阶段 | 文件 | 工作量 |
|------|------|--------|
| Phase 1 | `providers/errors.go` — ErrorClassifier | 1天 |
| Phase 2 | `providers/cooldown.go` — CooldownTracker | 1天 |
| Phase 3 | `providers/fallback.go` — FallbackChain | 1天 |
| Phase 4 | `providers/factory.go` — ProviderFactory | 0.5天 |
| Phase 5 | 集成到 agent + gateway | 1天 |
| Phase 6 | 单元测试 | 1天 |

**总计**：~5.5 天
