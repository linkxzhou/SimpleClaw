# health — 健康检查与心跳服务

`health` 包提供 HTTP 健康检查端点和 Agent 周期性心跳唤醒机制。

## 文件结构

| 文件 | 说明 |
|------|------|
| `server.go` | HTTP 健康检查服务器（存活 / 就绪探针） |
| `heartbeat.go` | Agent 心跳服务（定时唤醒检查待办任务） |
| `heartbeat_test.go` | 心跳服务单元测试 |

## 核心组件

### Server — 健康检查服务器

提供两个标准 HTTP 探针端点，同时支持外部路由挂载（如 API Server）。

| 端点 | 说明 |
|------|------|
| `GET /health` | **存活探针** — 始终返回 200，附带 `uptime` |
| `GET /ready` | **就绪探针** — 就绪时 200，未就绪时 503 |

关键方法：

- `NewServer(host, port, logger)` — 创建服务器实例
- `SetReady(bool)` — 设置就绪状态（原子操作）
- `HandleFunc(pattern, handler)` — 注册外部路由（必须在 `Start` 前调用）
- `Start(ctx)` — 启动 HTTP 服务，context 取消时优雅关闭（5s 超时）

### HeartbeatService — 心跳服务

按配置的间隔定时检查工作区中的 `HEARTBEAT.md` 文件，发现待处理任务时唤醒 Agent。

工作流程：

1. 每隔 `interval` 读取 `{workspace}/HEARTBEAT.md`
2. 如果文件为空或仅含标题 / 注释 / 空待办项，跳过
3. 如果有内容，调用 `OnHeartbeat` 回调将提示词发送给 Agent
4. Agent 回复 `HEARTBEAT_OK` 表示无事可做，否则表示已处理任务

配置参数（`HeartbeatConfig`）：

| 字段 | 类型 | 说明 |
|------|------|------|
| `Workspace` | `string` | 工作区路径 |
| `OnHeartbeat` | `func(ctx, prompt) (string, error)` | 心跳回调 |
| `Interval` | `time.Duration` | 检查间隔（默认 30 分钟） |
| `Enabled` | `bool` | 是否启用 |
| `Logger` | `*slog.Logger` | 日志记录器 |

关键方法：

- `Start(ctx)` — 启动定时循环（阻塞）
- `Stop()` — 停止服务
- `TriggerNow(ctx)` — 手动触发一次心跳

## 使用示例

```go
// 创建 Health Server
svr := health.NewServer("0.0.0.0", 18790, logger)

// 挂载外部路由
svr.HandleFunc("/api/rpc", apiHandler)

// 启动（非阻塞）
go svr.Start(ctx)
svr.SetReady(true)

// 创建心跳服务
hb := health.NewHeartbeatService(health.HeartbeatConfig{
    Workspace:   "/path/to/workspace",
    OnHeartbeat: func(ctx context.Context, prompt string) (string, error) {
        return agent.ProcessDirect(ctx, prompt)
    },
    Interval: 30 * time.Minute,
    Enabled:  true,
})
go hb.Start(ctx)
```
