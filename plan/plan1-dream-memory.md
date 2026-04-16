# Plan 1: Dream 两阶段记忆整合

## 来源

NanoBot — `nanobot/agent/memory.py` → `Dream` 类 + `Consolidator` 类

## 目标

为 SimpleClaw 增加自动记忆整理能力，通过定时任务将碎片化的对话历史整合为结构化长期知识，解决当前 `MemoryStore` 只做文件读写、缺乏自动整理的问题。

## 现状分析

- `memory/store.go` 的 `MemoryStore` 提供 `MEMORY.md`（长期记忆）和每日笔记（`YYYY-MM-DD.md`）的读写
- 每日笔记持续增长，但从未自动整理或清理
- `ContextBuilder` 每次把全部记忆文件塞进 system prompt，上下文越来越长
- 缺乏从对话历史中自动提取知识的能力

## 技术方案

### 阶段一：基础设施（1 周）

#### 1.1 新增 history.jsonl 追加存储

在 `memory/` 目录下新增结构化历史文件，每轮对话结束后追加一条记录：

```go
// memory/history.go (新文件)
type HistoryEntry struct {
    Cursor    int64  `json:"cursor"`    // 自增游标
    Timestamp string `json:"timestamp"` // RFC3339
    SessionKey string `json:"session_key"`
    Summary   string `json:"summary"`   // 对话摘要或原始内容
}

type HistoryStore struct {
    filePath    string
    cursorFile  string // .dream_cursor — Dream 已处理到的位置
    mu          sync.Mutex
}

func (h *HistoryStore) Append(entry HistoryEntry) error
func (h *HistoryStore) ReadSince(cursor int64) ([]HistoryEntry, error)
func (h *HistoryStore) GetDreamCursor() int64
func (h *HistoryStore) SetDreamCursor(cursor int64) error
func (h *HistoryStore) Compact(keepLast int) error // 压缩旧条目
```

#### 1.2 对话结束时归档

在 `agent/agent.go` 的 `processMessage()` 完成后，将本轮对话摘要追加到 `history.jsonl`：

```go
// 简单模式：直接记录用户消息+助手回复的前 200 字
// 高级模式：调用 LLM 生成一句话摘要（后续阶段实现）
historyStore.Append(HistoryEntry{
    Timestamp:  time.Now().Format(time.RFC3339),
    SessionKey: session.Key,
    Summary:    truncate(userMsg + " → " + assistantReply, 500),
})
```

### 阶段二：Dream Phase 1 — 分析（1 周）

#### 2.1 新增 Dream 服务

```go
// memory/dream.go (新文件)
type DreamService struct {
    store      *MemoryStore
    history    *HistoryStore
    provider   LLMProvider // 复用现有 provider 接口
    model      string
    batchSize  int // 每批处理条数，默认 20
}

func (d *DreamService) Run(ctx context.Context) (bool, error)
```

#### 2.2 Phase 1 实现

```go
func (d *DreamService) phase1Analyze(entries []HistoryEntry) (string, error) {
    // 1. 读取当前 MEMORY.md / SOUL.md / USER.md 内容
    currentMemory := d.store.ReadMemory()
    currentSoul := d.store.ReadSoul()
    currentUser := d.store.ReadUser()

    // 2. 拼装提示词
    prompt := fmt.Sprintf(`## 对话历史\n%s\n\n## 当前 MEMORY.md\n%s\n\n## 当前 USER.md\n%s`,
        formatEntries(entries), currentMemory, currentUser)

    // 3. 调用 LLM 分析
    resp, err := d.provider.Chat(ctx, ChatRequest{
        Model: d.model,
        Messages: []Message{
            {Role: "system", Content: dreamPhase1SystemPrompt},
            {Role: "user", Content: prompt},
        },
    })
    return resp.Content, err
}
```

Phase 1 system prompt 要求 LLM 输出：
- 哪些是新事实需要记住
- 哪些旧记忆已过时需要更新
- 用户偏好是否有变化
- 是否有重要的行为模式

### 阶段三：Dream Phase 2 — 编辑（1.5 周）

#### 3.1 利用现有工具执行编辑

Phase 2 复用 SimpleClaw 现有的 Agent ReAct 循环，给它注册受限的工具集：

```go
func (d *DreamService) phase2Edit(ctx context.Context, analysis string) error {
    // 创建受限子 Agent，只能 read_file + edit_file，目录限定 workspace
    dreamAgent := NewSubAgent(SubAgentConfig{
        Tools:       []Tool{readFileTool, editFileTool}, // 仅两个工具
        SystemPrompt: dreamPhase2SystemPrompt,
        MaxIterations: 10,
    })

    // Phase 1 分析结果作为 user message
    result := dreamAgent.ProcessDirect(ctx, analysis + "\n\n" + fileContext)
    return result.Error
}
```

#### 3.2 Phase 2 system prompt 关键约束

- 只做增量编辑，不要替换整个文件
- MEMORY.md 中每条记忆保持 `[日期] 内容` 格式
- 核心事实放在文件顶部，日常细节放在底部
- 过时信息直接删除，不要注释

### 阶段四：Cron 集成 + 游标管理（0.5 周）

#### 4.1 注册 Dream 定时任务

```go
// 在 gateway 启动时注册
cronService.AddJob(cron.Job{
    Name:     "dream",
    Schedule: cron.Schedule{Type: "every", Interval: 3600}, // 每小时
    Payload:  cron.Payload{Message: "__dream__"}, // 内部标记，不走 Agent
})
```

#### 4.2 游标推进策略

- **正常完成**：推进游标到 batch 最后一条
- **Phase 1 失败**：不推进游标，下次重试
- **Phase 2 失败**：仍然推进游标（避免 Phase 1 重复分析），但记录 warning
- **首次启动**：游标设为当前最大值，不回溯处理历史

## 涉及文件

| 操作 | 文件 | 说明 |
|------|------|------|
| 新建 | `memory/history.go` | HistoryStore — JSONL 追加存储 + 游标管理 |
| 新建 | `memory/dream.go` | DreamService — 两阶段记忆整合 |
| 新建 | `memory/dream_test.go` | Dream 单元测试 |
| 修改 | `memory/store.go` | 添加 ReadSoul/ReadUser 方法（如尚无） |
| 修改 | `agent/agent.go` | 对话结束后追加 history 记录 |
| 修改 | `cmd/gateway.go` | 注册 Dream 定时任务 |

## 验收标准

- [ ] `history.jsonl` 在每轮对话后自动追加记录
- [ ] Dream 定时任务每小时运行，增量处理未分析的历史
- [ ] Phase 1 能正确识别新事实和过时信息
- [ ] Phase 2 能对 MEMORY.md 做增量编辑而非全量替换
- [ ] 游标机制保证不重复处理
- [ ] Dream 失败不影响正常 Agent 服务

## 工期估算

**总计：4 周（1 人）**

| 阶段 | 工期 | 产出 |
|------|------|------|
| 基础设施 | 1 周 | HistoryStore + 对话归档 |
| Phase 1 分析 | 1 周 | LLM 分析流程 + prompt 调优 |
| Phase 2 编辑 | 1.5 周 | 子 Agent 编辑 + prompt 调优 |
| Cron 集成 | 0.5 周 | 定时任务 + 游标管理 + 测试 |

## 风险与缓解

| 风险 | 缓解措施 |
|------|---------|
| LLM 分析质量不稳定 | Phase 1 prompt 迭代调优；Phase 2 失败仍推进游标避免阻塞 |
| API 费用增加 | Dream 使用廉价模型（如 gpt-4o-mini）；可配置开关关闭 |
| 并发安全 | HistoryStore 内置 mutex；Dream 任务用 singleflight 防重入 |
