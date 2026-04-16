// SOP 标准操作流程：数据模型定义。
// SOP 用 JSON 元数据 + Markdown 步骤定义可复用的多步骤工作流，
// 支持多种触发方式（cron/manual/webhook）和执行模式（auto/supervised/step_by_step/deterministic）。

package sop

import "time"

// SopPriority SOP 优先级。
type SopPriority string

const (
	PriorityLow      SopPriority = "low"
	PriorityNormal   SopPriority = "normal"
	PriorityHigh     SopPriority = "high"
	PriorityCritical SopPriority = "critical"
)

// SopExecutionMode 执行模式。
type SopExecutionMode string

const (
	ModeAuto          SopExecutionMode = "auto"           // 全自动
	ModeSupervised    SopExecutionMode = "supervised"     // 开始前确认
	ModeStepByStep    SopExecutionMode = "step_by_step"   // 每步确认
	ModeDeterministic SopExecutionMode = "deterministic"  // 无 LLM，纯工具链
)

// SopTrigger 触发方式。
type SopTrigger struct {
	Type       string `json:"type"`       // "cron" | "webhook" | "manual"
	Expression string `json:"expression"` // cron 表达式或 webhook 路径
}

// SopStep 一个 SOP 步骤。
type SopStep struct {
	Number         int      `json:"number"`
	Title          string   `json:"title"`
	Body           string   `json:"body"`            // 步骤描述/指令
	SuggestedTools []string `json:"suggested_tools"`
	IsCheckpoint   bool     `json:"is_checkpoint"`   // 暂停等待确认
}

// Sop 一个完整的 SOP 定义。
type Sop struct {
	Name          string           `json:"name"`
	Description   string           `json:"description"`
	Version       string           `json:"version"`
	Priority      SopPriority      `json:"priority"`
	ExecutionMode SopExecutionMode `json:"execution_mode"`
	Triggers      []SopTrigger     `json:"triggers"`
	Steps         []SopStep        `json:"steps"`
	CooldownSecs  int              `json:"cooldown_secs"`
	MaxConcurrent int              `json:"max_concurrent"`
}

// SopRunStatus 运行状态。
type SopRunStatus string

const (
	RunPending   SopRunStatus = "pending"
	RunRunning   SopRunStatus = "running"
	RunPaused    SopRunStatus = "paused"    // checkpoint 暂停
	RunCompleted SopRunStatus = "completed"
	RunFailed    SopRunStatus = "failed"
)

// SopStepResult 步骤执行结果。
type SopStepResult struct {
	StepNumber int           `json:"step_number"`
	Status     string        `json:"status"` // "success" | "failed" | "skipped"
	Output     string        `json:"output"`
	Duration   time.Duration `json:"duration"`
}

// SopRun 一次 SOP 运行实例。
type SopRun struct {
	SopName     string          `json:"sop_name"`
	RunID       string          `json:"run_id"`
	Status      SopRunStatus    `json:"status"`
	CurrentStep int             `json:"current_step"`
	StepResults []SopStepResult `json:"step_results"`
	StartedAt   time.Time       `json:"started_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
	Error       string          `json:"error,omitempty"`
}
