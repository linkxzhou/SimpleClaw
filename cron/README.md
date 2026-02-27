# cron

定时任务管理包，支持一次性、周期性和 cron 表达式三种调度方式，任务通过 JSON 文件持久化。

## 架构

```
                    ┌─────────────────────┐
                    │       Service       │
                    │                     │
                    │  loadStore()        │ ◄── jobs.json
                    │  armTimer()         │
                    │  onTimer()          │
                    │  executeJob()       │ ──► OnJobFunc 回调
                    └─────────┬───────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
        ScheduleAt      ScheduleEvery    ScheduleCron
        (一次性)         (周期性)         (Cron 表达式)
```

## 目录结构

```
cron/
├── types.go         # 类型定义（Schedule、Payload、Job、Store 等）
├── service.go       # Service 实现（调度、执行、持久化）
└── service_test.go  # 测试
```

## 调度方式

| 类型 | Kind | 说明 | 示例 |
|------|------|------|------|
| 一次性 | `at` | 在指定时间戳执行一次 | 2026-03-01T09:00:00 |
| 周期性 | `every` | 按固定间隔重复执行 | 每 3600000ms (1小时) |
| Cron 表达式 | `cron` | 标准 cron 语法 | `0 9 * * *` |

## 核心类型

```go
// Job 表示一个定时任务
type Job struct {
    ID             string   // 唯一标识
    Name           string   // 任务名称
    Enabled        bool     // 是否启用
    Schedule       Schedule // 调度计划
    Payload        Payload  // 执行内容（消息 + 投递目标）
    State          JobState // 运行状态（下次/上次执行时间、状态）
    DeleteAfterRun bool     // 一次性任务执行后是否自动删除
}

// Payload 定义任务执行时的动作
type Payload struct {
    Kind    PayloadKind // "agent_turn" 或 "system_event"
    Message string      // 发送给 Agent 的消息
    Deliver bool        // 是否将响应投递到频道
    Channel string      // 目标频道
    To      string      // 目标接收者
}
```

## Service API

| 方法 | 说明 |
|------|------|
| `NewService(storePath, onJob, logger)` | 创建服务 |
| `Start(ctx)` | 启动服务（阻塞，直到 ctx 取消） |
| `Stop()` | 停止服务 |
| `ListJobs(includeDisabled)` | 列出任务（按下次执行时间排序） |
| `AddJob(name, sched, message, ...)` | 添加任务 |
| `RemoveJob(jobID)` | 删除任务 |
| `EnableJob(jobID, enabled)` | 启用/禁用任务 |
| `RunJob(ctx, jobID, force)` | 手动执行任务 |
| `Status()` | 返回服务状态摘要 |

## 调度机制

采用**单定时器模式**：只维护一个指向最近到期任务的 `time.AfterFunc` 定时器。

```
Start → loadStore → recomputeNextRuns → armTimer(最近到期时间)
                                              │
                                              ▼ 定时器触发
                                         onTimer()
                                              │
                                    ┌─────────▼─────────┐
                                    │ 收集所有到期任务    │
                                    │ 逐个 executeJob()  │
                                    │ saveStore()        │
                                    │ armTimer(下一个)    │
                                    └───────────────────┘
```

## 持久化

任务存储在 `~/.simpleclaw/cron/jobs.json`：

```json
{
  "version": 1,
  "jobs": [
    {
      "id": "17090834",
      "name": "daily report",
      "enabled": true,
      "schedule": { "kind": "every", "everyMs": 3600000 },
      "payload": { "kind": "agent_turn", "message": "Generate daily report" },
      "state": { "nextRunAtMs": 1709087040000, "lastStatus": "ok" }
    }
  ]
}
```
