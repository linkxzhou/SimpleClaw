# Plan 5: 记忆衰减 + 分类存储

## 来源

ZeroClaw — `src/memory/decay.rs` + `src/memory/consolidation.rs` + `src/memory/traits.rs`

## 目标

为 SimpleClaw 的记忆系统引入分类标签和时间衰减评分，在上下文窗口受限时智能选择最相关的记忆条目，核心事实永不丢失。

## 现状分析

- `memory/store.go` 的 `MemoryStore` 以平面 Markdown 文件存储（`MEMORY.md` + 每日 `YYYY-MM-DD.md`）
- `ContextBuilder` 将全部记忆内容拼入 system prompt，无优先级
- 记忆只增不减，随时间累积会挤占上下文窗口
- 无法区分"用户姓名"（永久记忆）和"今天讨论了天气"（临时记忆）

## 技术方案

### 阶段一：结构化记忆条目（1 周）

#### 1.1 记忆条目类型

```go
// memory/entry.go (新文件)
type MemoryCategory string
const (
    CategoryCore         MemoryCategory = "core"         // 核心事实（永不衰减）
    CategoryDaily        MemoryCategory = "daily"        // 每日对话摘要
    CategoryConversation MemoryCategory = "conversation" // 对话上下文片段
    CategoryProcedural   MemoryCategory = "procedural"   // 操作步骤/偏好
)

type MemoryEntry struct {
    ID        string         `json:"id"`
    Key       string         `json:"key"`       // 短标识（如 "user_name"）
    Content   string         `json:"content"`
    Category  MemoryCategory `json:"category"`
    Timestamp string         `json:"timestamp"` // RFC3339
    Score     *float64       `json:"score,omitempty"` // 衰减后分数
}
```

#### 1.2 JSONL 存储后端

在现有 Markdown 文件之外，新增结构化存储（两者并存，Markdown 作为人可读视图）：

```go
// memory/structured.go (新文件)
type StructuredMemory struct {
    filePath string // ~/.simpleclaw/workspace/memory/entries.jsonl
    mu       sync.RWMutex
    entries  []MemoryEntry // 内存缓存
}

func (sm *StructuredMemory) Store(key, content string, category MemoryCategory) error
func (sm *StructuredMemory) Query(categories []MemoryCategory, limit int) []MemoryEntry
func (sm *StructuredMemory) Delete(id string) error
func (sm *StructuredMemory) Update(id string, content string) error
func (sm *StructuredMemory) All() []MemoryEntry
```

### 阶段二：时间衰减算法（0.5 周）

#### 2.1 指数衰减函数

```go
// memory/decay.go (新文件)
const DefaultHalfLifeDays = 7.0 // 7 天后分数降至 50%

// ApplyTimeDecay 对非 Core 类别的条目应用指数时间衰减
// 公式: score * 2^(-age_days / half_life_days)
func ApplyTimeDecay(entries []MemoryEntry, halfLifeDays float64) {
    if halfLifeDays <= 0 {
        halfLifeDays = DefaultHalfLifeDays
    }
    now := time.Now()

    for i := range entries {
        // Core 记忆永不衰减
        if entries[i].Category == CategoryCore {
            continue
        }
        if entries[i].Score == nil {
            continue
        }

        ts, err := time.Parse(time.RFC3339, entries[i].Timestamp)
        if err != nil {
            continue
        }

        ageDays := now.Sub(ts).Hours() / 24.0
        decayFactor := math.Exp(-ageDays / halfLifeDays * math.Ln2)
        newScore := *entries[i].Score * decayFactor
        entries[i].Score = &newScore
    }
}
```

#### 2.2 评分规则

| 类别 | 初始分数 | 衰减 | 说明 |
|------|---------|------|------|
| Core | 1.0 | 永不衰减 | 用户姓名、关键偏好、重要决定 |
| Procedural | 0.8 | 半衰期 30 天 | 操作步骤、工作流偏好 |
| Daily | 0.6 | 半衰期 7 天 | 每日对话摘要 |
| Conversation | 0.4 | 半衰期 3 天 | 临时对话片段 |

### 阶段三：两阶段整合（对话后）（1.5 周）

#### 3.1 轻量整合器

每轮对话结束后，自动提取有价值的信息存入结构化记忆：

```go
// memory/consolidation.go (新文件)
type ConsolidationResult struct {
    HistoryEntry string   // 当日摘要 → Daily 类别
    MemoryUpdate *string  // 新事实 → Core 类别（nil = 无新信息）
    Facts        []string // 原子事实列表
}

func ConsolidateTurn(
    provider LLMProvider,
    model string,
    userMessage string,
    assistantResponse string,
) (*ConsolidationResult, error) {
    // 截断过长输入（4000 字符上限，注意 UTF-8 安全切割）
    turnText := truncateUTF8Safe(
        fmt.Sprintf("User: %s\nAssistant: %s", userMessage, assistantResponse),
        4000,
    )

    // 调用 LLM 提取
    resp, err := provider.Chat(ctx, ChatRequest{
        Model: model, // 使用廉价模型如 gpt-4o-mini
        Messages: []Message{
            {Role: "system", Content: consolidationSystemPrompt},
            {Role: "user", Content: turnText},
        },
    })

    // 解析 JSON 响应
    var result ConsolidationResult
    json.Unmarshal([]byte(resp.Content), &result)
    return &result, nil
}
```

#### 3.2 System Prompt

```
你是一个记忆整合引擎。给定一轮对话，提取：
1. "history_entry": 本轮对话的简要摘要（1-2 句话）
2. "memory_update": 任何新的事实、偏好、决定或承诺。如果没有新信息，返回 null
仅返回 JSON: {"history_entry": "...", "memory_update": "..." 或 null}
```

#### 3.3 冲突检测

```go
// memory/conflict.go (新文件)
// 新事实与已有记忆冲突时的处理
func ResolveConflict(existing MemoryEntry, newContent string) MemoryEntry {
    // 策略：新信息覆盖旧信息，更新时间戳
    existing.Content = newContent
    existing.Timestamp = time.Now().Format(time.RFC3339)
    score := 1.0
    existing.Score = &score // 重置分数
    return existing
}
```

### 阶段四：ContextBuilder 集成（1 周）

#### 4.1 按分数排序 + 预算截断

```go
// agent/context.go — 修改 buildMemoryContext()
func (cb *ContextBuilder) buildMemoryContext(maxTokens int) string {
    entries := cb.structuredMemory.All()

    // 1. 应用时间衰减
    ApplyTimeDecay(entries, DefaultHalfLifeDays)

    // 2. 按分数降序排序
    sort.Slice(entries, func(i, j int) bool {
        si, sj := 0.0, 0.0
        if entries[i].Score != nil { si = *entries[i].Score }
        if entries[j].Score != nil { sj = *entries[j].Score }
        return si > sj
    })

    // 3. 按 token 预算截取
    var result strings.Builder
    tokenCount := 0
    for _, e := range entries {
        entryTokens := estimateTokens(e.Content)
        if tokenCount + entryTokens > maxTokens {
            break
        }
        result.WriteString(fmt.Sprintf("[%s|%s] %s\n", e.Category, e.Timestamp[:10], e.Content))
        tokenCount += entryTokens
    }
    return result.String()
}
```

## 涉及文件

| 操作 | 文件 | 说明 |
|------|------|------|
| 新建 | `memory/entry.go` | MemoryEntry + MemoryCategory 类型 |
| 新建 | `memory/structured.go` | JSONL 结构化存储 |
| 新建 | `memory/decay.go` | 时间衰减算法 |
| 新建 | `memory/consolidation.go` | 对话后自动整合 |
| 新建 | `memory/conflict.go` | 冲突检测与解决 |
| 新建 | `memory/decay_test.go` | 衰减算法单元测试 |
| 修改 | `memory/store.go` | 集成 StructuredMemory |
| 修改 | `agent/context.go` | 按分数排序 + 预算截断 |
| 修改 | `agent/agent.go` | 对话结束后触发 ConsolidateTurn |

## 验收标准

- [ ] Core 记忆永不衰减，7 天后分数仍为 1.0
- [ ] Daily 记忆 7 天后分数降至 ~50%，14 天后 ~25%
- [ ] ContextBuilder 优先选择高分记忆填充上下文
- [ ] 对话结束后自动提取新事实存入 Core 类别
- [ ] 冲突时新信息覆盖旧信息并重置分数

## 工期估算

**总计：4 周（1 人）**

| 阶段 | 工期 |
|------|------|
| 结构化记忆条目 | 1 周 |
| 时间衰减算法 | 0.5 周 |
| 两阶段整合 | 1.5 周 |
| ContextBuilder 集成 | 1 周 |

## 风险与缓解

| 风险 | 缓解措施 |
|------|---------|
| 整合 LLM 调用增加 API 费用 | 使用廉价模型（gpt-4o-mini）；可配置关闭 |
| 衰减参数不合理导致重要记忆丢失 | Core 类别永不衰减兜底；半衰期可配置 |
| 与现有 Markdown 记忆并存的一致性 | 结构化存储为主，Markdown 作为只读人类视图，定期同步 |
