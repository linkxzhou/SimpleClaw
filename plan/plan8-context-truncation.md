# Plan 8: 智能上下文截断

## 来源

PicoClaw — `pkg/agent/subturn.go` 的 `MaxContextRunes` + NanoBot — `nanobot/agent/memory.py` 的 `maybe_consolidate_by_tokens`

## 目标

将 SimpleClaw 的会话历史管理从"按消息条数截取"升级为"按 token 预算智能截断"，在安全切割点归档旧消息，保持语义完整性，避免上下文窗口溢出。

## 现状分析

- `memory/session.go` 的 `GetHistory(maxMessages)` 只按消息条数截取最近 N 条
- 不考虑消息长度差异：一条包含代码的消息可能是普通消息的 10 倍 token
- 无法感知模型上下文窗口限制
- 超长上下文直接导致 API 报错（HTTP 400），无优雅降级

## 技术方案

### 阶段一：Token 估算器（0.5 周）

#### 1.1 轻量估算

```go
// utils/tokens.go (新文件)
// 轻量估算：不依赖外部 tokenizer 库
// 英文按空格分词 * 1.3，中文按字符 * 0.6，代码按字符 * 0.3
func EstimateTokens(text string) int {
    if len(text) == 0 {
        return 0
    }

    var (
        alphaNum   int
        cjk        int
        other      int
    )
    for _, r := range text {
        if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hiragana, r) {
            cjk++
        } else if unicode.IsLetter(r) || unicode.IsDigit(r) {
            alphaNum++
        } else {
            other++
        }
    }

    // 经验系数：
    // - CJK 字符 ≈ 0.6 token/char（一个汉字 ≈ 1-2 tokens，取中间值）
    // - 英文 ≈ 0.25 token/char（约 4 chars/token）
    // - 其他（空格、标点）≈ 0.5 token/char
    tokens := int(float64(cjk)*0.6 + float64(alphaNum)*0.25 + float64(other)*0.5)
    if tokens == 0 {
        tokens = 1
    }
    return tokens
}

// 估算一组 messages 的总 token 数
func EstimateMessagesTokens(messages []Message) int {
    total := 0
    for _, m := range messages {
        total += EstimateTokens(m.Content) + 4 // 每条消息的 role/分隔符开销
    }
    return total
}
```

#### 1.2 模型上下文窗口映射

```go
// providers/context_window.go (新文件)
var ContextWindowSizes = map[string]int{
    "claude-sonnet-4-20250514": 200000,
    "claude-haiku-3.5":         200000,
    "gpt-4o":                   128000,
    "gpt-4o-mini":              128000,
    "deepseek-chat":            64000,
    // ... 可扩展
}

func GetContextWindow(model string) int {
    if size, ok := ContextWindowSizes[normalizeModel(model)]; ok {
        return size
    }
    return 8192 // 保守默认值
}
```

### 阶段二：安全切割点算法（1 周）

#### 2.1 核心概念

"安全切割点"是可以截断消息历史的位置，需满足：
- 不在 tool_use → tool_result 对中间切割
- 不在多轮连续对话的中间切割
- 尽量在 assistant 回复之后切割（语义完整）

```go
// memory/truncation.go (新文件)
type CutPoint struct {
    Index       int // 消息索引
    TokensSaved int // 截断后节省的 token 数
}

// FindSafeCutPoints 扫描消息历史，返回所有安全切割点
func FindSafeCutPoints(messages []Message) []CutPoint {
    var points []CutPoint
    tokensSoFar := 0

    for i, msg := range messages {
        tokensSoFar += EstimateTokens(msg.Content) + 4

        // 跳过 system 消息（始终保留）
        if msg.Role == "system" {
            continue
        }
        // 跳过 tool_result（必须和前面的 tool_use 一起）
        if msg.Role == "tool" {
            continue
        }
        // 在 assistant 回复之后是安全切割点
        if msg.Role == "assistant" {
            // 检查下一条不是 tool_result
            if i+1 < len(messages) && messages[i+1].Role == "tool" {
                continue // 跳过：assistant 的 tool_use 还没得到 result
            }
            points = append(points, CutPoint{Index: i + 1, TokensSaved: tokensSoFar})
        }
    }
    return points
}
```

#### 2.2 选择最佳切割点

```go
// 选择能节省 targetTokens 的最小切割点
func PickCutPoint(points []CutPoint, targetTokens int) *CutPoint {
    for _, p := range points {
        if p.TokensSaved >= targetTokens {
            return &p
        }
    }
    // 无法满足目标，选最后一个（最大切割）
    if len(points) > 0 {
        return &points[len(points)-1]
    }
    return nil
}
```

### 阶段三：自动截断 + 归档（1.5 周）

#### 3.1 在 Agent Loop 中集成

```go
// agent/agent.go — 在每次 LLM 调用前
func (a *Agent) prepareMessages(session *Session, newMsg string) []Message {
    history := session.GetHistory(0) // 取全部
    messages := a.contextBuilder.Build(history, newMsg)

    // 计算 token 预算
    contextWindow := GetContextWindow(a.model)
    maxCompletion := a.config.MaxTokens
    safetyBuffer := 1000
    budget := contextWindow - maxCompletion - safetyBuffer

    totalTokens := EstimateMessagesTokens(messages)
    if totalTokens <= budget {
        return messages // 无需截断
    }

    // 需要截断
    excess := totalTokens - budget/2 // 目标压缩到预算的一半，留余量
    return a.truncateAndArchive(session, messages, excess)
}

func (a *Agent) truncateAndArchive(session *Session, messages []Message, targetSave int) []Message {
    // 1. 找到安全切割点
    cutPoints := FindSafeCutPoints(messages)
    cut := PickCutPoint(cutPoints, targetSave)
    if cut == nil {
        // 无安全切割点，保留最近的消息
        return keepRecentMessages(messages, targetSave)
    }

    // 2. 归档被切掉的消息
    archived := messages[1:cut.Index] // 跳过 system 消息
    go a.archiveMessages(session, archived) // 异步归档

    // 3. 重组：system + 摘要占位符 + 保留的消息
    var result []Message
    result = append(result, messages[0]) // system
    result = append(result, Message{
        Role:    "system",
        Content: fmt.Sprintf("[Earlier conversation (%d messages) archived]", len(archived)),
    })
    result = append(result, messages[cut.Index:]...)
    return result
}
```

#### 3.2 归档实现

```go
func (a *Agent) archiveMessages(session *Session, messages []Message) {
    // 方式 1：简单截取（低成本）
    summary := formatMessagesAsSummary(messages)
    a.memoryStore.AppendHistory(summary)

    // 方式 2：LLM 摘要（高质量，可选）
    // if a.config.EnableLLMArchive {
    //     summary := a.llmSummarize(messages)
    //     a.memoryStore.AppendHistory(summary)
    // }

    // 更新 session 的 lastConsolidated 指针
    session.LastConsolidated = len(messages)
}
```

### 阶段四：软截断防线（0.5 周）

#### 4.1 MaxContextRunes 配置

```go
// config 中新增
type AgentConfig struct {
    // ... 现有字段
    MaxContextRunes int `json:"maxContextRunes"`
    // 0 = 自动（contextWindow * 0.75）
    // -1 = 不限制
    // >0 = 手动指定
}
```

#### 4.2 Rune 级软截断（最后防线）

```go
// 在 EstimateTokens 之外，对单条超长消息进行截断
func TruncateMessageContent(content string, maxRunes int) string {
    if maxRunes <= 0 {
        return content
    }
    runes := []rune(content)
    if len(runes) <= maxRunes {
        return content
    }
    return string(runes[:maxRunes]) + "\n... [truncated]"
}
```

## 涉及文件

| 操作 | 文件 | 说明 |
|------|------|------|
| 新建 | `utils/tokens.go` | Token 估算器 |
| 新建 | `providers/context_window.go` | 模型上下文窗口映射 |
| 新建 | `memory/truncation.go` | 安全切割点算法 |
| 新建 | `memory/truncation_test.go` | 单元测试 |
| 修改 | `agent/agent.go` | LLM 调用前自动截断 |
| 修改 | `config/schema.go` | MaxContextRunes 配置 |

## 验收标准

- [ ] Token 估算误差在 ±30% 以内
- [ ] 切割点不在 tool_use → tool_result 中间
- [ ] 超过 budget 时自动截断并归档
- [ ] 归档消息异步处理，不阻塞 Agent 响应
- [ ] 截断后插入摘要占位符，LLM 能理解上下文断裂
- [ ] MaxContextRunes 作为最后防线，防止单条超长消息

## 工期估算

**总计：3.5 周（1 人）**

| 阶段 | 工期 |
|------|------|
| Token 估算器 | 0.5 周 |
| 安全切割点算法 | 1 周 |
| 自动截断 + 归档 | 1.5 周 |
| 软截断防线 | 0.5 周 |
