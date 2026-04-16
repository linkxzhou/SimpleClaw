// SOP 执行引擎：按步骤执行 SOP，支持 checkpoint 暂停、审批确认和多种执行模式。

package sop

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ConfirmFunc 确认回调，返回 true 表示继续，false 表示取消。
type ConfirmFunc func(title, description string) bool

// ToolExecutor 工具执行接口（避免循环依赖 agent 包）。
type ToolExecutor interface {
	ExecuteTool(ctx context.Context, toolName string, args map[string]interface{}) (string, error)
}

// LLMExecutor LLM 执行接口。
type LLMExecutor interface {
	ChatSimple(ctx context.Context, systemPrompt, userPrompt, model string) (string, error)
}

// EngineConfig 引擎配置。
type EngineConfig struct {
	ConfirmFunc ConfirmFunc // 确认回调（nil = 自动通过）
	Tools       ToolExecutor
	LLM         LLMExecutor
	Model       string
	Logger      *slog.Logger
}

// Engine SOP 执行引擎。
type Engine struct {
	config EngineConfig
	runs   map[string]*SopRun
	mu     sync.Mutex
	logger *slog.Logger
}

// NewEngine 创建 SOP 执行引擎。
func NewEngine(config EngineConfig) *Engine {
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{
		config: config,
		runs:   make(map[string]*SopRun),
		logger: logger,
	}
}

// Execute 执行一个 SOP，返回运行结果。
func (e *Engine) Execute(ctx context.Context, sop Sop) *SopRun {
	run := &SopRun{
		SopName:   sop.Name,
		RunID:     fmt.Sprintf("run_%d", time.Now().UnixNano()),
		Status:    RunRunning,
		StartedAt: time.Now(),
	}

	e.mu.Lock()
	e.runs[run.RunID] = run
	e.mu.Unlock()

	e.logger.Info("sop: starting", "name", sop.Name, "run_id", run.RunID, "mode", sop.ExecutionMode)

	// Supervised 模式：开始前确认
	if sop.ExecutionMode == ModeSupervised || sop.ExecutionMode == ModeStepByStep {
		if !e.confirm(fmt.Sprintf("Execute SOP: %s", sop.Name), sop.Description) {
			run.Status = RunFailed
			run.Error = "user declined to start"
			e.complete(run)
			return run
		}
	}

	// 逐步执行
	for _, step := range sop.Steps {
		if ctx.Err() != nil {
			run.Status = RunFailed
			run.Error = "context cancelled"
			e.complete(run)
			return run
		}

		run.CurrentStep = step.Number

		// Checkpoint 或 StepByStep：暂停确认
		if step.IsCheckpoint || sop.ExecutionMode == ModeStepByStep {
			run.Status = RunPaused
			if !e.confirm(fmt.Sprintf("Step %d: %s", step.Number, step.Title), step.Body) {
				run.Status = RunFailed
				run.Error = fmt.Sprintf("user declined step %d", step.Number)
				e.complete(run)
				return run
			}
			run.Status = RunRunning
		}

		// 执行步骤
		startTime := time.Now()
		var result SopStepResult

		switch sop.ExecutionMode {
		case ModeDeterministic:
			result = e.executeDeterministic(ctx, step)
		default:
			result = e.executeWithLLM(ctx, step, sop)
		}

		result.StepNumber = step.Number
		result.Duration = time.Since(startTime)
		run.StepResults = append(run.StepResults, result)

		e.logger.Info("sop: step completed",
			"step", step.Number,
			"title", step.Title,
			"status", result.Status,
			"duration", result.Duration,
		)

		if result.Status == "failed" {
			run.Status = RunFailed
			run.Error = fmt.Sprintf("step %d failed: %s", step.Number, result.Output)
			e.complete(run)
			return run
		}
	}

	run.Status = RunCompleted
	e.complete(run)
	return run
}

// GetRun 获取运行实例。
func (e *Engine) GetRun(runID string) *SopRun {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.runs[runID]
}

// ListRuns 列出所有运行实例。
func (e *Engine) ListRuns() []*SopRun {
	e.mu.Lock()
	defer e.mu.Unlock()
	runs := make([]*SopRun, 0, len(e.runs))
	for _, r := range e.runs {
		runs = append(runs, r)
	}
	return runs
}

// ============ 内部方法 ============

func (e *Engine) confirm(title, description string) bool {
	if e.config.ConfirmFunc == nil {
		return true // 无确认回调 = 自动通过
	}
	return e.config.ConfirmFunc(title, description)
}

func (e *Engine) complete(run *SopRun) {
	now := time.Now()
	run.CompletedAt = &now
	e.logger.Info("sop: completed", "run_id", run.RunID, "status", run.Status,
		"steps", len(run.StepResults), "duration", now.Sub(run.StartedAt))
}

// executeDeterministic 确定性执行（无 LLM）：直接调用 SuggestedTools。
func (e *Engine) executeDeterministic(ctx context.Context, step SopStep) SopStepResult {
	if e.config.Tools == nil {
		return SopStepResult{Status: "failed", Output: "no tool executor configured"}
	}

	var outputs []string
	for _, toolName := range step.SuggestedTools {
		result, err := e.config.Tools.ExecuteTool(ctx, toolName, map[string]interface{}{
			"instruction": step.Body,
		})
		if err != nil {
			return SopStepResult{Status: "failed", Output: fmt.Sprintf("tool %s: %v", toolName, err)}
		}
		outputs = append(outputs, result)
	}

	return SopStepResult{
		Status: "success",
		Output: fmt.Sprintf("Executed %d tools: %s", len(step.SuggestedTools), joinTrunc(outputs, 500)),
	}
}

// executeWithLLM 通过 LLM 执行步骤。
func (e *Engine) executeWithLLM(ctx context.Context, step SopStep, sop Sop) SopStepResult {
	if e.config.LLM == nil {
		return SopStepResult{Status: "failed", Output: "no LLM executor configured"}
	}

	systemPrompt := fmt.Sprintf(`You are executing step %d of SOP "%s".
Step title: %s
Available tools: %s
Execute the step and report the result concisely.`,
		step.Number, sop.Name, step.Title,
		fmt.Sprintf("%v", step.SuggestedTools))

	resp, err := e.config.LLM.ChatSimple(ctx, systemPrompt, step.Body, e.config.Model)
	if err != nil {
		return SopStepResult{Status: "failed", Output: err.Error()}
	}

	return SopStepResult{
		Status: "success",
		Output: truncSop(resp, 1000),
	}
}

func truncSop(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func joinTrunc(items []string, maxLen int) string {
	result := ""
	for i, item := range items {
		if i > 0 {
			result += "; "
		}
		result += item
		if len(result) > maxLen {
			return result[:maxLen] + "..."
		}
	}
	return result
}
