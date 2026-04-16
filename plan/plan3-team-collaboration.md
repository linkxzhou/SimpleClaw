# Plan 3: 多 Agent 团队协作 + 扇出执行

## 来源

TinyAGI — `packages/teams/src/routing.ts` + `packages/main/src/index.ts` + `packages/core/src/queues.ts`

## 目标

为 SimpleClaw 增加多 Agent 团队编排能力，支持 Agent 在 LLM 响应中通过标签语法自然地触发跨 Agent 协作（扇出），以及可靠的消息队列。

## 现状分析

- `SubagentManager` 只支持父 → 子单向 spawn，子 Agent 之间不能互相通信
- Agent 间协作只能通过 `spawn` 工具，需要显式编写，不够自然
- 消息总线 `bus/` 是纯内存 channel 队列，无持久化和重试机制
- 无多 Agent 团队概念，无法定义 Agent 角色分工

## 技术方案

### 阶段一：团队配置 + Agent 注册表（1 周）

#### 1.1 配置扩展

```json
{
  "teams": {
    "engineering": {
      "agents": ["coder", "reviewer", "tester"],
      "leader": "coder"
    }
  },
  "agents": {
    "profiles": {
      "coder": {
        "model": "anthropic/claude-sonnet-4-20250514",
        "systemPrompt": "你是一个代码编写专家..."
      },
      "reviewer": {
        "model": "anthropic/claude-sonnet-4-20250514",
        "systemPrompt": "你是一个代码审查专家..."
      },
      "tester": {
        "model": "openai/gpt-4o-mini",
        "systemPrompt": "你是一个测试专家..."
      }
    }
  }
}
```

#### 1.2 Agent 注册表

```go
// agent/registry.go (新文件)
type AgentProfile struct {
    ID           string
    Model        string
    SystemPrompt string
    Tools        []string // 允许使用的工具列表
    Teams        []string // 所属团队
}

type AgentRegistry struct {
    mu       sync.RWMutex
    agents   map[string]*AgentProfile
    teams    map[string]*TeamConfig
    default_ string // 默认 agent ID
}

func (r *AgentRegistry) GetAgent(id string) *AgentProfile
func (r *AgentRegistry) GetTeam(id string) *TeamConfig
func (r *AgentRegistry) GetTeammates(agentID, teamID string) []string
```

### 阶段二：Bracket Tag 解析 + 消息路由（1.5 周）

#### 2.1 标签解析器

```go
// agent/tags.go (新文件)
type Mention struct {
    AgentID string
    Message string
}

type ChatRoomMessage struct {
    TeamID  string
    Message string
}

// 解析 [@agent: message] 和 [#team: message]
// 支持嵌套括号（如 [@coder: fix arr[0] bug]）
func ParseBracketTags(response string) (mentions []Mention, chatroom []ChatRoomMessage, cleanText string) {
    // 基于括号深度计数的状态机解析器
    // 1. 扫描 '['，检查是否跟 '@' 或 '#'
    // 2. 记录括号深度，只在回到深度 0 时关闭标签
    // 3. 验证 agent/team ID 是否合法（存在于注册表中）
    // 4. 排除自引用（agent 不能 mention 自己）
    // 5. 支持逗号分隔多目标：[@coder,reviewer: message]
}
```

#### 2.2 响应后处理

在 Agent Loop 完成 LLM 响应后，解析标签并路由：

```go
// agent/agent.go — processLLMResponse()
func (a *Agent) processLLMResponse(ctx context.Context, response string, sourceAgent string) {
    mentions, chatroom, cleanText := ParseBracketTags(response)

    // 1. 处理 @mentions → 扇出到目标 Agent
    for _, m := range mentions {
        a.bus.PublishInbound(bus.InboundMessage{
            Channel:  "internal",
            SenderID: sourceAgent,
            Content:  fmt.Sprintf("[From @%s]: %s", sourceAgent, m.Message),
            Metadata: map[string]string{
                "target_agent": m.AgentID,
                "from_agent":   sourceAgent,
            },
        })
    }

    // 2. 处理 #chatroom → 广播到团队所有成员（排除发送者）
    for _, cr := range chatroom {
        teammates := a.registry.GetTeammates(sourceAgent, cr.TeamID)
        for _, t := range teammates {
            a.bus.PublishInbound(bus.InboundMessage{
                Channel:  "chatroom",
                SenderID: sourceAgent,
                Content:  fmt.Sprintf("[Chat #%s from @%s]: %s", cr.TeamID, sourceAgent, cr.Message),
                Metadata: map[string]string{"target_agent": t},
            })
        }
    }

    // 3. 原始响应去掉标签后发给用户
    a.bus.PublishOutbound(bus.OutboundMessage{Content: cleanText})
}
```

### 阶段三：Agent Chain 串行保证（1 周）

#### 3.1 Per-Agent 串行执行

```go
// agent/chain.go (新文件)
type AgentChainManager struct {
    mu     sync.Mutex
    chains map[string]*agentChain // key = agentID
}

type agentChain struct {
    queue chan *bus.InboundMessage // 缓冲队列
    done  chan struct{}
}

// 每个 Agent 一个 goroutine，串行处理消息
func (m *AgentChainManager) Enqueue(agentID string, msg *bus.InboundMessage) {
    m.mu.Lock()
    chain, ok := m.chains[agentID]
    if !ok {
        chain = &agentChain{queue: make(chan *bus.InboundMessage, 100)}
        m.chains[agentID] = chain
        go m.runChain(agentID, chain) // 启动消费 goroutine
    }
    m.mu.Unlock()
    chain.queue <- msg
}

func (m *AgentChainManager) runChain(agentID string, chain *agentChain) {
    for msg := range chain.queue {
        // 串行处理：同一 Agent 一次只处理一条消息
        m.processMessage(agentID, msg)
    }
}
```

### 阶段四：消息可靠性增强（1 周）

#### 4.1 消息状态机

```go
// bus/reliable.go (新文件)
type MessageStatus string
const (
    StatusPending    MessageStatus = "pending"
    StatusProcessing MessageStatus = "processing"
    StatusCompleted  MessageStatus = "completed"
    StatusFailed     MessageStatus = "failed"
    StatusDead       MessageStatus = "dead" // 重试 maxRetries 次后
)

type ReliableMessage struct {
    ID         string
    Status     MessageStatus
    RetryCount int
    MaxRetries int // 默认 5
    CreatedAt  time.Time
    UpdatedAt  time.Time
    Message    bus.InboundMessage
}
```

#### 4.2 重试 + 死信

```go
func (q *ReliableQueue) ProcessMessage(msg *ReliableMessage) {
    msg.Status = StatusProcessing
    err := q.handler(msg.Message)
    if err != nil {
        msg.RetryCount++
        if msg.RetryCount >= msg.MaxRetries {
            msg.Status = StatusDead
            q.deadLetterQueue <- msg
        } else {
            msg.Status = StatusPending // 回到队列重试
        }
    } else {
        msg.Status = StatusCompleted
    }
}
```

#### 4.3 连续聊天室消息合并

```go
// 连续的 chatroom 消息在 100ms 窗口内合并为一条
func (m *AgentChainManager) batchChatroomMessages(agentID string, msgs []*bus.InboundMessage) []*bus.InboundMessage {
    var batched []*bus.InboundMessage
    var chatroomBuf []string
    for _, msg := range msgs {
        if msg.Channel == "chatroom" {
            chatroomBuf = append(chatroomBuf, msg.Content)
        } else {
            if len(chatroomBuf) > 0 {
                batched = append(batched, &bus.InboundMessage{
                    Channel: "chatroom",
                    Content: strings.Join(chatroomBuf, "\n---\n"),
                })
                chatroomBuf = nil
            }
            batched = append(batched, msg)
        }
    }
    return batched
}
```

## 涉及文件

| 操作 | 文件 | 说明 |
|------|------|------|
| 新建 | `agent/registry.go` | Agent 注册表 + 团队配置 |
| 新建 | `agent/tags.go` | Bracket Tag 解析器 |
| 新建 | `agent/chain.go` | Per-Agent 串行执行管理 |
| 新建 | `bus/reliable.go` | 消息状态机 + 重试 + 死信 |
| 修改 | `agent/agent.go` | 响应后处理 → 标签路由 |
| 修改 | `config/schema.go` | 新增 teams + agent profiles 配置 |
| 修改 | `agent/context.go` | ContextBuilder 注入 teammates 信息 |

## 验收标准

- [ ] Agent LLM 响应中的 `[@agent: msg]` 标签被正确解析并路由
- [ ] 扇出：`[@a,b: msg]` 同时路由到多个 Agent
- [ ] 同一 Agent 的消息严格串行处理
- [ ] 不同 Agent 之间并行处理
- [ ] 消息失败后自动重试，超过上限进入死信
- [ ] 连续聊天室消息自动合并

## 工期估算

**总计：4.5 周（1 人）**

| 阶段 | 工期 |
|------|------|
| 团队配置 + Agent 注册表 | 1 周 |
| Bracket Tag 解析 + 路由 | 1.5 周 |
| Agent Chain 串行保证 | 1 周 |
| 消息可靠性增强 | 1 周 |

## 风险与缓解

| 风险 | 缓解措施 |
|------|---------|
| 标签解析误匹配正常文本 | 严格要求 `[@` 和 `[#` 前缀；Agent ID 必须在注册表中存在 |
| 循环 mention 无限递归 | 标记消息来源链，检测循环引用立即终止 |
| 消息积压导致内存增长 | 缓冲队列设上限（100）；死信队列定期清理 |
