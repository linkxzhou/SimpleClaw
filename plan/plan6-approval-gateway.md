# Plan 6: 审批网关 + 自治级别

## 来源

ZeroClaw — `src/approval/mod.rs`

## 目标

为 SimpleClaw 增加工具调用前的审批拦截层，按自治级别控制 Agent 的操作权限，CLI 模式支持交互式确认，渠道模式按白名单自动决策，所有审批记录可审计。

## 现状分析

- `agent/tools/shell.go` 有基础安全防护（Shell 命令黑名单：`rm -rf /`、`mkfs` 等）
- 但防护是硬编码的，无法按场景调整
- Telegram/WhatsApp 渠道下 Agent 拥有与 CLI 相同的权限，无差异化
- `write_file`、`edit_file` 等写操作无任何确认机制
- 无操作审计日志

## 技术方案

### 阶段一：自治级别 + 审批接口（1 周）

#### 1.1 核心类型

```go
// agent/approval.go (新文件)
type AutonomyLevel string
const (
    AutonomyFull       AutonomyLevel = "full"       // 全自动，不弹确认
    AutonomySupervised AutonomyLevel = "supervised" // 默认需确认，白名单豁免
    AutonomyReadOnly   AutonomyLevel = "read_only"  // 只读，写操作全部拒绝
)

type ApprovalResponse string
const (
    ApprovalYes    ApprovalResponse = "yes"    // 本次允许
    ApprovalNo     ApprovalResponse = "no"     // 本次拒绝
    ApprovalAlways ApprovalResponse = "always" // 允许并加入会话白名单
)

type ApprovalRequest struct {
    ToolName  string
    Arguments map[string]interface{}
    Summary   string // 人可读的操作摘要
}

type ApprovalLogEntry struct {
    Timestamp string           `json:"timestamp"`
    ToolName  string           `json:"tool_name"`
    Summary   string           `json:"summary"`
    Decision  ApprovalResponse `json:"decision"`
    Channel   string           `json:"channel"`
}
```

#### 1.2 ApprovalManager

```go
type ApprovalManager struct {
    autonomyLevel    AutonomyLevel
    autoApprove      map[string]bool // 免确认工具（如 read_file, list_dir）
    alwaysAsk        map[string]bool // 强制确认工具（如 exec 危险命令）
    nonInteractive   bool            // 渠道模式 = true
    sessionAllowlist sync.Map        // 会话级"Always"白名单
    auditLog         []ApprovalLogEntry
    mu               sync.Mutex
    promptFunc       func(req ApprovalRequest) ApprovalResponse // CLI 交互回调
}

func NewApprovalManager(config ApprovalConfig, interactive bool) *ApprovalManager
func NewNonInteractiveManager(config ApprovalConfig) *ApprovalManager
```

#### 1.3 判断逻辑

```go
func (am *ApprovalManager) NeedsApproval(toolName string) bool {
    // Full 模式：永不确认
    if am.autonomyLevel == AutonomyFull {
        return false
    }
    // ReadOnly 模式：写工具直接拒绝（不弹确认）
    if am.autonomyLevel == AutonomyReadOnly {
        return isWriteTool(toolName)
    }
    // Supervised 模式：
    // 1. autoApprove 列表中的工具 → 不需确认
    if am.autoApprove[toolName] {
        return false
    }
    // 2. 会话白名单中的工具（用户选过 "Always"）→ 不需确认
    if _, ok := am.sessionAllowlist.Load(toolName); ok {
        // 但 alwaysAsk 列表中的工具除外（始终确认）
        if am.alwaysAsk[toolName] {
            return true
        }
        return false
    }
    // 3. 其他 → 需要确认
    return true
}
```

### 阶段二：CLI 交互确认（0.5 周）

#### 2.1 终端提示

```go
func CLIPrompt(req ApprovalRequest) ApprovalResponse {
    fmt.Printf("\n⚠️  Tool: %s\n", req.ToolName)
    fmt.Printf("   %s\n", req.Summary)
    fmt.Printf("   [y]es / [n]o / [a]lways: ")

    reader := bufio.NewReader(os.Stdin)
    input, _ := reader.ReadString('\n')
    input = strings.TrimSpace(strings.ToLower(input))

    switch input {
    case "y", "yes":
        return ApprovalYes
    case "a", "always":
        return ApprovalAlways
    default:
        return ApprovalNo
    }
}
```

#### 2.2 操作摘要生成

```go
func GenerateSummary(toolName string, args map[string]interface{}) string {
    switch toolName {
    case "exec":
        return fmt.Sprintf("Execute: %s", args["command"])
    case "write_file":
        return fmt.Sprintf("Write to: %s (%d bytes)", args["path"], len(args["content"].(string)))
    case "edit_file":
        return fmt.Sprintf("Edit: %s (replace %d chars)", args["path"], len(args["old_text"].(string)))
    default:
        return fmt.Sprintf("%s(%v)", toolName, args)
    }
}
```

### 阶段三：渠道模式 + 审计日志（1 周）

#### 3.1 非交互模式

```go
func (am *ApprovalManager) CheckApproval(toolName string, args map[string]interface{}) (ApprovalResponse, error) {
    if !am.NeedsApproval(toolName) {
        am.logDecision(toolName, args, ApprovalYes, "auto_approved")
        return ApprovalYes, nil
    }

    if am.nonInteractive {
        // 渠道模式：需要确认的工具自动拒绝
        am.logDecision(toolName, args, ApprovalNo, "non_interactive_denied")
        return ApprovalNo, nil
    }

    // CLI 模式：弹交互确认
    req := ApprovalRequest{
        ToolName:  toolName,
        Arguments: args,
        Summary:   GenerateSummary(toolName, args),
    }
    response := am.promptFunc(req)

    if response == ApprovalAlways {
        am.sessionAllowlist.Store(toolName, true)
    }

    am.logDecision(toolName, args, response, "interactive")
    return response, nil
}
```

#### 3.2 审计日志持久化

```go
func (am *ApprovalManager) logDecision(toolName string, args map[string]interface{}, decision ApprovalResponse, channel string) {
    entry := ApprovalLogEntry{
        Timestamp: time.Now().Format(time.RFC3339),
        ToolName:  toolName,
        Summary:   GenerateSummary(toolName, args),
        Decision:  decision,
        Channel:   channel,
    }
    am.mu.Lock()
    am.auditLog = append(am.auditLog, entry)
    am.mu.Unlock()

    // 异步写入文件 ~/.simpleclaw/audit.jsonl
    go am.persistEntry(entry)
}
```

### 阶段四：工具注册表集成（0.5 周）

#### 4.1 在 Registry 中嵌入审批

```go
// agent/tools/registry.go — 修改 Execute()
func (r *Registry) Execute(ctx context.Context, name string, args map[string]interface{}) (string, error) {
    // 审批检查
    if r.approvalManager != nil {
        decision, err := r.approvalManager.CheckApproval(name, args)
        if err != nil {
            return "", err
        }
        if decision == ApprovalNo {
            return "", fmt.Errorf("tool %q denied by approval policy", name)
        }
    }

    // 正常执行
    tool, ok := r.tools[name]
    if !ok {
        return "", fmt.Errorf("unknown tool: %s", name)
    }
    return tool.Execute(ctx, args)
}
```

## 涉及文件

| 操作 | 文件 | 说明 |
|------|------|------|
| 新建 | `agent/approval.go` | ApprovalManager + 核心类型 |
| 新建 | `agent/approval_test.go` | 单元测试 |
| 修改 | `agent/tools/registry.go` | Execute() 中嵌入审批检查 |
| 修改 | `config/schema.go` | 新增 ApprovalConfig |
| 修改 | `cmd/gateway.go` | 渠道模式创建 NonInteractive Manager |
| 修改 | `cmd/agent.go` | CLI 模式创建 Interactive Manager |

## 配置扩展

```json
{
  "approval": {
    "level": "supervised",
    "autoApprove": ["read_file", "list_dir", "web_search", "web_fetch"],
    "alwaysAsk": ["exec"]
  }
}
```

## 工具分类

| 分类 | 工具 | 默认策略 |
|------|------|---------|
| 只读（安全） | `read_file`, `list_dir`, `web_search`, `web_fetch` | autoApprove |
| 写入（需确认） | `write_file`, `edit_file` | supervised 下需确认 |
| 危险（强制确认） | `exec` | alwaysAsk |
| 通信 | `message`, `spawn` | supervised 下需确认 |
| 代码执行 | `go_run`, `go_agent` | supervised 下需确认 |

## 验收标准

- [ ] `full` 模式下所有工具直接执行
- [ ] `supervised` 模式下 CLI 弹出确认提示
- [ ] 选择 "always" 后同类工具不再弹确认
- [ ] `read_only` 模式下写工具被拒绝
- [ ] 渠道模式下需确认的工具自动拒绝
- [ ] 审计日志记录所有决策

## 工期估算

**总计：3 周（1 人）**

| 阶段 | 工期 |
|------|------|
| 自治级别 + 审批接口 | 1 周 |
| CLI 交互确认 | 0.5 周 |
| 渠道模式 + 审计日志 | 1 周 |
| 工具注册表集成 | 0.5 周 |
