# memory — 持久化存储

`memory` 包提供 SimpleClaw 的三类持久化存储功能：会话管理、系统状态追踪和记忆存储。

## 文件结构

| 文件 | 说明 |
|------|------|
| `session.go` | Session 数据结构和基本操作 |
| `manager.go` | SessionManager — 会话生命周期管理与 JSONL 持久化 |
| `state.go` | StateManager — 系统状态的原子持久化 |
| `store.go` | MemoryStore — 长期记忆与每日笔记管理 |
| `session_test.go` | 会话相关单元测试 |

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
