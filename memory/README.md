# memory — 持久化存储

`memory` 包提供 SimpleClaw 的三类持久化存储功能：会话管理、系统状态追踪和记忆存储。

## 文件结构

| 文件 | 说明 |
|------|------|
| `session.go` | Session 数据结构和基本操作 |
| `manager.go` | SessionManager — 会话生命周期管理与 JSONL 持久化 |
| `state.go` | StateManager — 系统状态的原子持久化 |
| `store.go` | MemoryStore — 长期记忆与每日笔记管理 |
| `entry.go` | 结构化记忆条目（四级分类 + 时间衰减评分） |
| `structured.go` | JSONL 结构化记忆存储（与 Markdown 并存） |
| `decay.go` | 记忆时间衰减（指数衰减模型） |
| `dream.go` | Dream 两阶段记忆整合服务（LLM 分析 → 写入） |
| `history.go` | HistoryStore 对话历史追加存储（增量消费） |
| `truncation.go` | 智能上下文截断（安全切割点归档） |
| `session_test.go` | 会话相关单元测试 |
| `decay_test.go` | 衰减测试 |
| `dream_test.go` | Dream 测试 |
| `truncation_test.go` | 截断测试 |

## 核心组件

### Session — 对话会话

表示一次对话的完整上下文，包含消息历史和元数据。

```go
type Session struct {
    Key       string                   // 会话键（格式：channel:chat_id）
    Messages  []map[string]interface{} // 消息历史
    CreatedAt time.Time                // 创建时间
    UpdatedAt time.Time                // 最后更新时间
    Metadata  map[string]interface{}   // 额外元数据
}
```

关键方法：

- `NewSession(key)` — 创建空会话
- `AddMessage(role, content, extra)` — 添加消息（自动记录时间戳）
- `GetHistory(maxMessages)` — 获取最近 N 条消息（仅 role + content）
- `Clear()` — 清空所有消息

### SessionManager — 会话管理器

管理会话的完整生命周期，提供内存缓存 + JSONL 文件持久化的两级存储。

JSONL 文件格式：
- 第一行：元数据（`_type: "metadata"`，含 `created_at`、`updated_at`）
- 后续行：每行一条 JSON 消息记录

关键方法：

- `NewSessionManager(sessionsDir, logger)` — 创建管理器（自动建目录）
- `GetOrCreate(key)` — 获取会话（缓存 → 磁盘 → 新建）
- `Save(session)` — 持久化到 JSONL 文件
- `Delete(key)` — 从缓存和磁盘删除
- `ListSessions()` — 列出所有会话摘要（按更新时间倒序）

### StateManager — 系统状态管理

追踪最近活跃的 Channel / ChatID，供心跳服务和设备事件使用。使用 temp file + rename 模式保证写入原子性。

```go
type State struct {
    LastChannel string    // 最近活跃渠道
    LastChatID  string    // 最近活跃聊天 ID
    Timestamp   time.Time // 最后更新时间
}
```

关键方法：

- `NewStateManager(dataDir)` — 创建管理器并加载已有状态
- `Update(channel, chatID)` — 更新状态（异步保存）
- `Get()` — 获取状态快照
- `GetLastActive()` — 获取最近活跃的 channel 和 chatID

状态文件路径：`{dataDir}/state/state.json`

### MemoryStore — 记忆存储

管理 Agent 的长期记忆（`MEMORY.md`）和每日笔记（`YYYY-MM-DD.md`）。

关键方法：

| 方法 | 说明 |
|------|------|
| `ReadLongTerm()` | 读取长期记忆 |
| `WriteLongTerm(content)` | 写入长期记忆 |
| `ReadToday()` | 读取今日笔记 |
| `AppendToday(content)` | 追加今日笔记（自动添加日期标题） |
| `GetRecentMemories(days)` | 获取最近 N 天的记忆 |
| `ListMemoryFiles()` | 列出所有记忆文件（最新在前） |
| `GetMemoryContext()` | 生成 system prompt 用的记忆上下文 |

存储目录：`{workspace}/memory/`

## 使用示例

```go
// 会话管理
mgr, _ := memory.NewSessionManager("~/.simpleclaw/sessions", logger)
session := mgr.GetOrCreate("telegram:12345")
session.AddMessage("user", "Hello", nil)
mgr.Save(session)

// 状态管理
state := memory.NewStateManager("~/.simpleclaw")
state.Update("telegram", "12345")

// 记忆存储
store := memory.NewMemoryStore("/path/to/workspace")
store.AppendToday("用户讨论了项目架构")
ctx := store.GetMemoryContext()
```

## 结构化记忆 (`entry.go` + `structured.go`)

在原有 Markdown 文件基础上，新增 JSONL 结构化查询后端。Markdown 作为人可读视图，JSONL 作为机器查询层。

### 四级分类

| 类别 | 说明 | 衰减 |
|------|------|------|
| `core` | 核心事实（名称、角色、偏好） | **永不衰减** |
| `daily` | 日常记录 | 指数衰减 |
| `conversation` | 对话摘要 | 指数衰减 |
| `procedural` | 操作流程（如何做某事） | 指数衰减 |

### 记忆条目

```go
type MemoryEntry struct {
    ID        string         // 唯一标识
    Category  MemoryCategory // 分类
    Content   string         // 内容
    Score     float64        // 重要性评分 (0-1)
    CreatedAt time.Time      // 创建时间
    Tags      []string       // 标签
}
```

## 记忆衰减 (`decay.go`)

对非 Core 类别的记忆条目应用指数衰减：

```
decayed_score = score × 2^(-age_days / half_life_days)
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `HalfLifeDays` | 7 | 半衰期（天） |
| `MinScore` | 0.05 | 低于此分值的条目标记为可回收 |

- `Core` 类别永不衰减
- `ApplyDecay(entries)` — 批量应用衰减，返回更新后的条目列表
- `PruneExpired(entries)` — 移除分值低于阈值的条目

## Dream 记忆整合 (`dream.go`)

两阶段 LLM 驱动的自动记忆整合：

```
Phase 1 — 分析：读取 HistoryStore 新增对话 → LLM 提取事实/偏好/摘要
Phase 2 — 编辑：将提取结果写入结构化记忆 + 同步更新 MEMORY.md
```

- 由 Cron 定时触发（默认每小时）
- 通过 HistoryStore 的 cursor 机制增量消费，不重复处理
- Phase 1 和 Phase 2 可独立运行（支持只分析不写入）

## 对话历史 (`history.go`)

`HistoryStore` 追加式对话记录存储：

- 每轮对话追加到 `history.jsonl`
- 通过 cursor（字节偏移量）实现增量读取
- Dream 服务通过 `ReadSince(cursor)` 消费新增记录
- 不修改已有记录，只追加

## 智能上下文截断 (`truncation.go`)

当对话历史超过模型上下文窗口时，自动在安全切割点归档旧消息：

- **System 消息始终保留** — 不参与截断
- **Tool 对完整性** — `tool_use` → `tool_result` 消息对不拆分
- **安全切割点** — 在 user 消息边界处截断（不在 assistant 回复中间切）
- Token 估算使用 `utils.EstimateTokens`（轻量级，无外部依赖）
