# routing — 多 Agent 消息路由

`routing` 包提供多 Agent 场景下的消息路由解析，根据入站消息的渠道和身份信息将其分配到对应的 Agent。

## 文件结构

| 文件 | 说明 |
|------|------|
| `route.go` | 路由解析器（`RouteResolver`）和绑定配置（`BindingConfig`） |

## 核心概念

### BindingConfig — 路由绑定规则

每条绑定规则描述一个消息匹配条件到目标 Agent 的映射：

```go
type BindingConfig struct {
    Agent   string // 目标 Agent ID
    Channel string // 渠道名（"*" 表示通配）
    Peer    string // 对话方 ID（可选）
    Guild   string // 群组/服务器 ID（可选）
    Team    string // 团队 ID（可选）
    Account string // 账号 ID（可选）
}
```

### ResolvedRoute — 路由结果

```go
type ResolvedRoute struct {
    AgentID    string // 匹配到的 Agent ID
    SessionKey string // 会话键（格式：channel:chatID）
    MatchedBy  string // 匹配方式描述
}
```

## 7 级优先级路由

`RouteResolver.Resolve(channel, senderID, chatID)` 按以下优先级从高到低匹配：

| 优先级 | 匹配方式 | MatchedBy | 说明 |
|--------|---------|-----------|------|
| 1 | Peer 精确匹配 | `binding.peer` | senderID 与 Peer 完全匹配 |
| 2 | Guild 匹配 | `binding.guild` | chatID 与 Guild 完全匹配 |
| 3 | Team 匹配 | `binding.team` | chatID 与 Team 完全匹配 |
| 4 | Account 匹配 | `binding.account` | senderID 与 Account 完全匹配 |
| 5 | Channel 通配符 | `binding.channel` | Channel 为 `*` 且无其他条件 |
| 6 | Channel 精确匹配 | `binding.channel_exact` | Channel 名完全匹配且无其他条件 |
| 7 | 默认 | `default` | 使用默认 Agent |

每级匹配时都会检查 Channel 条件（`*` 通配或精确匹配）。

## 使用示例

```go
bindings := []routing.BindingConfig{
    {Agent: "code-agent", Channel: "telegram", Peer: "user123"},
    {Agent: "chat-agent", Channel: "*"},
}

resolver := routing.NewRouteResolver(bindings, "default-agent")

// user123 在 telegram 上 → code-agent（Peer 匹配）
route := resolver.Resolve("telegram", "user123", "chat456")
// route.AgentID = "code-agent", route.MatchedBy = "binding.peer"

// 其他用户 → chat-agent（Channel 通配符）
route = resolver.Resolve("whatsapp", "user999", "chat789")
// route.AgentID = "chat-agent", route.MatchedBy = "binding.channel"
```

## 配置示例

在 `config.json` 中配置路由绑定：

```json
{
  "bindings": [
    {"agent": "code-agent", "channel": "telegram", "peer": "user123"},
    {"agent": "ops-agent", "channel": "telegram", "guild": "devops-group"},
    {"agent": "default-agent", "channel": "*"}
  ]
}
```

## 状态

路由解析器已完整实现 7 级优先级匹配，但尚未接入 `gateway.go` 主流程（参见 `cmd/gateway.go` 中的 `TODO(plan3)`）。当前 gateway 仍使用单 Agent 模式，待接入后可实现多 Agent 自动路由。
