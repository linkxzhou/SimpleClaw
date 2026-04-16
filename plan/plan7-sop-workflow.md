# Plan 7: SOP 标准操作流程

## 来源

ZeroClaw — `src/sop/types.rs` + `src/sop/mod.rs` + `src/sop/engine.rs`

## 目标

为 SimpleClaw 引入 SOP（标准操作流程）系统，用 JSON + Markdown 定义可复用的多步骤工作流，支持多种触发方式和执行模式，让 `cron/` 从"执行单条消息"升级为"编排多步骤流程"。

## 现状分析

- `cron/` 的 Job 只能执行一条消息（`Payload.Message`），无法编排多步骤
- 复杂工作流只能靠 Agent 在一次对话中自行规划，不可复用
- 缺乏 checkpoint（关键步骤暂停确认）机制
- 无确定性执行模式（不走 LLM 的纯工具链）

## 技术方案

### 阶段一：SOP 定义与加载（1 周）

#### 1.1 SOP 数据模型

```go
// sop/types.go (新文件)
type SopPriority string
const (
    PriorityLow      SopPriority = "low"
    PriorityNormal   SopPriority = "normal"
    PriorityHigh     SopPriority = "high"
    PriorityCritical SopPriority = "critical"
)

type SopExecutionMode string
const (
    ModeAuto          SopExecutionMode = "auto"           // 全自动
    ModeSupervised    SopExecutionMode = "supervised"     // 开始前确认
    ModeStepByStep    SopExecutionMode = "step_by_step"   // 每步确认
    ModeDeterministic SopExecutionMode = "deterministic"  // 无 LLM，纯工具链
)

type SopTrigger struct {
    Type       string `json:"type"`       // "cron" | "webhook" | "manual"
    Expression string `json:"expression"` // cron 表达式 或 webhook 路径
}

type SopStep struct {
    Number        int      `json:"number"`
    Title         string   `json:"title"`
    Body          string   `json:"body"`          // 步骤描述/指令
    SuggestedTools []string `json:"suggested_tools"` // 建议使用的工具
    IsCheckpoint  bool     `json:"is_checkpoint"` // 暂停等待确认
}

type Sop struct {
    Name           string           `json:"name"`
    Description    string           `json:"description"`
    Version        string           `json:"version"`
    Priority       SopPriority      `json:"priority"`
    ExecutionMode  SopExecutionMode `json:"execution_mode"`
    Triggers       []SopTrigger     `json:"triggers"`
    Steps          []SopStep        `json:"steps"`
    CooldownSecs   int              `json:"cooldown_secs"`   // 冷却时间
    MaxConcurrent  int              `json:"max_concurrent"`  // 最大并发执行数
}
```

#### 1.2 SOP 文件格式

每个 SOP 是一个目录，包含 `sop.json` + `steps.md`：

```
~/.simpleclaw/workspace/sops/
├── deploy-pipeline/
│   ├── sop.json      # 元数据 + 触发器
│   └── steps.md      # 步骤定义
├── daily-report/
│   ├── sop.json
│   └── steps.md
```

**sop.json 示例**：
```json
{
  "name": "deploy-pipeline",
  "description": "标准部署流水线",
  "priority": "high",
  "execution_mode": "supervised",
  "cooldown_secs": 300,
  "max_concurrent": 1,
  "triggers": [
    { "type": "manual" },
    { "type": "cron", "expression": "0 2 * * *" }
  ]
}
```

**steps.md 示例**：
```markdown
## Step 1: 拉取最新代码
tools: exec
执行 `git pull origin main`，确保在最新版本上操作。

## Step 2: 运行测试 [checkpoint]
tools: exec
执行 `go test ./...`。如果有测试失败，报告失败用例并停止。

## Step 3: 构建二进制
tools: exec
执行 `go build -o simpleclaw ./cmd/`

## Step 4: 部署到生产
tools: exec, write_file
将构建产物复制到部署目录，更新版本号。
```

#### 1.3 SOP 加载器

```go
// sop/loader.go (新文件)
func LoadSOPs(workspaceDir string) ([]Sop, error) {
    sopsDir := filepath.Join(workspaceDir, "sops")
    // 扫描子目录，读取 sop.json + 解析 steps.md
}

func ParseSteps(markdown string) []SopStep {
    // 按 ## Step N: Title 分割
    // 解析 [checkpoint] 标记
    // 提取 tools: 行
}
```

### 阶段二：SOP 执行引擎（2 周）

#### 2.1 SOP 运行时

```go
// sop/engine.go (新文件)
type SopRun struct {
    SopName    string
    RunID      string
    Status     SopRunStatus // pending | running | paused | completed | failed
    CurrentStep int
    StepResults []SopStepResult
    StartedAt  time.Time
    CompletedAt *time.Time
}

type SopRunStatus string
const (
    RunPending   SopRunStatus = "pending"
    RunRunning   SopRunStatus = "running"
    RunPaused    SopRunStatus = "paused"   // checkpoint 暂停
    RunCompleted SopRunStatus = "completed"
    RunFailed    SopRunStatus = "failed"
)

type SopStepResult struct {
    StepNumber int
    Status     string // success | failed | skipped
    Output     string
    Duration   time.Duration
}
```

#### 2.2 执行引擎

```go
type SopEngine struct {
    provider LLMProvider
    tools    *ToolRegistry
    approval *ApprovalManager // 复用 Plan 6 的审批网关
    runs     map[string]*SopRun
    mu       sync.Mutex
}

func (e *SopEngine) Execute(ctx context.Context, sop Sop) (*SopRun, error) {
    run := &SopRun{
        SopName: sop.Name,
        RunID:   generateID(),
        Status:  RunRunning,
    }

    // Supervised 模式：开始前确认
    if sop.ExecutionMode == ModeSupervised {
        if !e.approval.ConfirmStart(sop) {
            run.Status = RunFailed
            return run, nil
        }
    }

    for i, step := range sop.Steps {
        run.CurrentStep = i + 1

        // Checkpoint 步骤 或 StepByStep 模式：暂停确认
        if step.IsCheckpoint || sop.ExecutionMode == ModeStepByStep {
            run.Status = RunPaused
            if !e.approval.ConfirmStep(step) {
                run.Status = RunFailed
                return run, nil
            }
            run.Status = RunRunning
        }

        // 执行步骤
        var result SopStepResult
        switch sop.ExecutionMode {
        case ModeDeterministic:
            result = e.executeDeterministic(ctx, step)
        default:
            result = e.executeWithLLM(ctx, step, sop)
        }

        run.StepResults = append(run.StepResults, result)
        if result.Status == "failed" {
            run.Status = RunFailed
            return run, nil
        }
    }

    run.Status = RunCompleted
    return run, nil
}
```

#### 2.3 确定性执行（无 LLM）

```go
func (e *SopEngine) executeDeterministic(ctx context.Context, step SopStep) SopStepResult {
    // 直接从 step.Body 中提取命令/工具调用
    // 不走 LLM，省 API 费用
    // 步骤输出作为下一步的输入（管道模式）
    for _, toolName := range step.SuggestedTools {
        result, err := e.tools.Execute(ctx, toolName, parseArgsFromBody(step.Body))
        if err != nil {
            return SopStepResult{Status: "failed", Output: err.Error()}
        }
        // 管道：输出传递给下一步
    }
    return SopStepResult{Status: "success"}
}
```

### 阶段三：Cron + CLI 集成（1 周）

#### 3.1 Cron 触发 SOP

```go
// 在 gateway 启动时，将 SOP 中的 cron triggers 注册为定时任务
for _, sop := range sops {
    for _, trigger := range sop.Triggers {
        if trigger.Type == "cron" {
            cronService.AddJob(cron.Job{
                Name:     "sop:" + sop.Name,
                Schedule: cron.Schedule{Type: "cron", Cron: trigger.Expression},
                Payload:  cron.Payload{Message: "__sop__:" + sop.Name},
            })
        }
    }
}
```

#### 3.2 CLI 命令

```bash
simpleclaw sop list                    # 列出所有 SOP
simpleclaw sop run <name>              # 手动执行
simpleclaw sop run <name> --step-by-step # 逐步确认模式
simpleclaw sop status <run_id>         # 查看执行状态
simpleclaw sop history                 # 查看执行历史
```

## 涉及文件

| 操作 | 文件 | 说明 |
|------|------|------|
| 新建 | `sop/types.go` | SOP 数据模型 |
| 新建 | `sop/loader.go` | SOP 加载 + Markdown 解析 |
| 新建 | `sop/engine.go` | SOP 执行引擎 |
| 新建 | `sop/engine_test.go` | 单元测试 |
| 新建 | `cmd/sop.go` | CLI 命令 |
| 修改 | `cmd/gateway.go` | SOP cron trigger 注册 |
| 修改 | `cmd/main.go` | 新增 sop 子命令 |

## 验收标准

- [ ] SOP 目录加载正确，steps.md 解析为结构化步骤
- [ ] Auto 模式全自动执行
- [ ] Supervised 模式开始前弹确认
- [ ] StepByStep 模式每步弹确认
- [ ] Checkpoint 步骤暂停等待确认
- [ ] Deterministic 模式不调用 LLM
- [ ] Cron trigger 按时触发 SOP
- [ ] CLI 支持列出、执行、查看状态

## 工期估算

**总计：4 周（1 人）**

| 阶段 | 工期 |
|------|------|
| SOP 定义与加载 | 1 周 |
| SOP 执行引擎 | 2 周 |
| Cron + CLI 集成 | 1 周 |

## 依赖

- Plan 6（审批网关）：SOP 的 Supervised/StepByStep/Checkpoint 复用审批机制
