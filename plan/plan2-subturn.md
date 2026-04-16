# Plan 2: SubTurn 子回合机制

## 来源

PicoClaw — `pkg/agent/subturn.go`

## 目标

增强 SimpleClaw 的 `SubagentManager`，引入同步/异步双模式子回合、并发控制、深度限制、共享 token 预算和独立超时，让子 Agent 从"简单后台任务"升级为"精细可控的嵌套执行单元"。

## 现状分析

- `agent/subagent.go` 的 `SubagentManager` 只支持异步模式（goroutine + 消息总线回传）
- 工具只能通过 `spawn` 创建子 Agent，无法同步等待结果
- 缺乏并发限制（可以无限 spawn）
- 子 Agent 使用父 Agent 的全部工具集（未做裁剪）
- 无深度限制（子 Agent 可以再 spawn 子 Agent，无限递归）
- 无 token 预算控制

## 技术方案

### 阶段一：SubTurnConfig + 同步模式（1 周）

#### 1.1 定义 SubTurnConfig

```go
// agent/subturn.go (新文件)
type SubTurnConfig struct {
    Model        string        // 可用不同模型（如轻量模型节省费用）
    Tools        []Tool        // 受限工具集（默认排除 message/spawn）
    SystemPrompt string        // 子回合系统提示词
    MaxTokens    int           // 最大输出 token
    Async        bool          // false=同步阻塞等结果，true=后台运行
    Critical     bool          // true=父结束后子继续运行
    Timeout      time.Duration // 独立超时（默认 5min）
    MaxContextRunes int        // 上下文软截断（0=自动计算）
}
```

#### 1.2 同步子回合实现

```go
func (sm *SubagentManager) SpawnSubTurn(ctx context.Context, cfg SubTurnConfig) (*SubTurnResult, error) {
    // 1. 深度检查
    depth := getDepthFromContext(ctx)
    if depth >= sm.maxDepth {
        return nil, ErrDepthLimitExceeded
    }

    // 2. 并发信号量获取
    select {
    case sm.semaphore <- struct{}{}:
        defer func() { <-sm.semaphore }()
    case <-time.After(sm.concurrencyTimeout):
        return nil, ErrConcurrencyTimeout
    }

    // 3. 创建临时会话（不污染父 session）
    ephemeralSession := NewEphemeralSession()

    // 4. 创建独立超时 context
    childCtx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
    defer cancel()
    childCtx = withDepth(childCtx, depth+1)

    // 5. 执行子回合 ReAct 循环
    result := sm.runTurn(childCtx, cfg, ephemeralSession)

    if cfg.Async {
        // 异步模式：结果送入 pendingResults channel
        sm.pendingResults <- result
    }
    return result, nil
}
```

#### 1.3 Ephemeral Session

```go
// agent/ephemeral.go (新文件)
type EphemeralSession struct {
    messages []Message
    maxSize  int // 默认 50 条，防止长时间子回合内存膨胀
}

// 实现 SessionStore 接口但不持久化
func (e *EphemeralSession) GetHistory(max int) []Message
func (e *EphemeralSession) AddMessage(msg Message)
func (e *EphemeralSession) Save() error { return nil } // no-op
```

### 阶段二：并发控制 + 深度限制（0.5 周）

#### 2.1 信号量并发控制

```go
type SubagentManager struct {
    // ... 现有字段
    semaphore          chan struct{} // 容量 = maxConcurrentSubTurns（默认 5）
    maxDepth           int          // 默认 3
    concurrencyTimeout time.Duration // 默认 30s
}

func NewSubagentManager(cfg SubagentConfig) *SubagentManager {
    return &SubagentManager{
        semaphore: make(chan struct{}, cfg.MaxConcurrent),
        maxDepth:  cfg.MaxDepth,
        // ...
    }
}
```

#### 2.2 Context 传递深度

```go
type depthKey struct{}

func withDepth(ctx context.Context, d int) context.Context {
    return context.WithValue(ctx, depthKey{}, d)
}

func getDepthFromContext(ctx context.Context) int {
    if d, ok := ctx.Value(depthKey{}).(int); ok {
        return d
    }
    return 0
}
```

### 阶段三：共享 Token 预算（1 周）

#### 3.1 原子计数器

```go
type TokenBudget struct {
    remaining atomic.Int64
}

func NewTokenBudget(total int64) *TokenBudget {
    b := &TokenBudget{}
    b.remaining.Store(total)
    return b
}

func (b *TokenBudget) Deduct(used int64) (remaining int64, ok bool) {
    for {
        current := b.remaining.Load()
        if current < used {
            return current, false // 预算不足
        }
        if b.remaining.CompareAndSwap(current, current-used) {
            return current - used, true
        }
    }
}
```

#### 3.2 在 Provider 调用后扣减

```go
// agent/agent.go 的 agentLoop 中，每次 LLM 调用后
resp, err := provider.Chat(ctx, req)
if err == nil && tokenBudget != nil {
    used := int64(resp.Usage.InputTokens + resp.Usage.OutputTokens)
    remaining, ok := tokenBudget.Deduct(used)
    if !ok {
        // 预算耗尽，优雅结束当前回合
        return &SubTurnResult{StopReason: "budget_exhausted"}
    }
}
```

### 阶段四：工具集成 + 改造 spawn（0.5 周）

#### 4.1 改造 spawn 工具

现有 `spawn` 工具改为内部调用 `SpawnSubTurn`：

```go
// agent/tools/spawn.go
func (t *SpawnTool) Execute(ctx context.Context, args SpawnArgs) (string, error) {
    result, err := sm.SpawnSubTurn(ctx, SubTurnConfig{
        Model:       args.Model,
        SystemPrompt: args.Task,
        Async:       true,  // spawn 默认异步
        Critical:    false,
        Timeout:     5 * time.Minute,
    })
    if err != nil {
        return "", err
    }
    return fmt.Sprintf("Subturn %s started", result.ID), nil
}
```

#### 4.2 新增 go_agent 的 SubTurn 支持

`go_agent` 工具可以用同步 SubTurn 等待 Go 脚本执行结果：

```go
// agent/tools/gocode.go
result, err := sm.SpawnSubTurn(ctx, SubTurnConfig{
    Async: false, // 同步等待结果
    Timeout: 2 * time.Minute,
    // ...
})
```

## 涉及文件

| 操作 | 文件 | 说明 |
|------|------|------|
| 新建 | `agent/subturn.go` | SubTurnConfig + SpawnSubTurn 核心逻辑 |
| 新建 | `agent/ephemeral.go` | 临时会话（不持久化） |
| 新建 | `agent/budget.go` | TokenBudget 原子计数器 |
| 新建 | `agent/subturn_test.go` | SubTurn 单元测试 |
| 重构 | `agent/subagent.go` | 整合到新 SubTurn 框架 |
| 修改 | `agent/tools/spawn.go` | 改用 SpawnSubTurn |
| 修改 | `config/schema.go` | 新增 SubTurn 配置项 |

## 配置扩展

```json
{
  "agents": {
    "defaults": {
      "subturn": {
        "maxDepth": 3,
        "maxConcurrent": 5,
        "concurrencyTimeoutSec": 30,
        "defaultTimeoutMinutes": 5,
        "defaultTokenBudget": 0
      }
    }
  }
}
```

## 验收标准

- [ ] 同步子回合能阻塞等待结果并返回
- [ ] 异步子回合结果通过 pendingResults channel 回传
- [ ] 超过 maxDepth 时返回 ErrDepthLimitExceeded
- [ ] 超过 maxConcurrent 时等待，超时返回 ErrConcurrencyTimeout
- [ ] 子回合使用 EphemeralSession，不影响父 session 历史
- [ ] TokenBudget 在多并发子回合间正确共享
- [ ] 现有 spawn 工具行为兼容（异步模式）

## 工期估算

**总计：3 周（1 人）**

| 阶段 | 工期 |
|------|------|
| SubTurnConfig + 同步模式 | 1 周 |
| 并发控制 + 深度限制 | 0.5 周 |
| 共享 Token 预算 | 1 周 |
| 工具集成 + 改造 spawn | 0.5 周 |

## 风险与缓解

| 风险 | 缓解措施 |
|------|---------|
| 死锁：子回合等信号量，父等子回合结果 | 信号量在 runTurn 完成后立即释放，不持有到 result 投递 |
| 子回合 context 泄露 | 独立 context + defer cancel；Critical 子回合有独立超时兜底 |
| 预算竞态 | 使用 CompareAndSwap 无锁原子操作 |
