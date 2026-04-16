package sop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// ParseSteps 测试
// ---------------------------------------------------------------------------

func TestParseStepsBasic(t *testing.T) {
	md := `## Step 1: Pull code
tools: exec
Run git pull origin main.

## Step 2: Run tests [checkpoint]
tools: exec
Run go test ./...

## Step 3: Build
tools: exec, write_file
Build the binary.`

	steps := ParseSteps(md)
	if len(steps) != 3 {
		t.Fatalf("期望 3 步，实际 %d", len(steps))
	}

	if steps[0].Number != 1 || steps[0].Title != "Pull code" {
		t.Errorf("Step 1 错误: %+v", steps[0])
	}
	if len(steps[0].SuggestedTools) != 1 || steps[0].SuggestedTools[0] != "exec" {
		t.Errorf("Step 1 tools 错误: %v", steps[0].SuggestedTools)
	}
	if steps[0].IsCheckpoint {
		t.Error("Step 1 不应是 checkpoint")
	}

	if !steps[1].IsCheckpoint {
		t.Error("Step 2 应是 checkpoint")
	}
	if steps[1].Title != "Run tests" {
		t.Errorf("Step 2 title 应去除 [checkpoint]，实际: %q", steps[1].Title)
	}

	if len(steps[2].SuggestedTools) != 2 {
		t.Errorf("Step 3 应有 2 个 tools，实际 %d", len(steps[2].SuggestedTools))
	}
}

func TestParseStepsEmpty(t *testing.T) {
	steps := ParseSteps("")
	if len(steps) != 0 {
		t.Errorf("空内容应返回 0 步，实际 %d", len(steps))
	}
}

func TestParseStepsMultilineBody(t *testing.T) {
	md := `## Step 1: Deploy
tools: exec
Line 1
Line 2
Line 3`

	steps := ParseSteps(md)
	if len(steps) != 1 {
		t.Fatalf("期望 1 步，实际 %d", len(steps))
	}
	if steps[0].Body == "" {
		t.Error("Body 不应为空")
	}
}

// ---------------------------------------------------------------------------
// LoadSOPs 测试
// ---------------------------------------------------------------------------

func TestLoadSOPs(t *testing.T) {
	dir := t.TempDir()
	sopsDir := filepath.Join(dir, "sops", "test-pipeline")
	os.MkdirAll(sopsDir, 0o755)

	sopJSON := map[string]interface{}{
		"name":           "test-pipeline",
		"description":    "Test pipeline",
		"priority":       "normal",
		"execution_mode": "auto",
		"triggers":       []map[string]string{{"type": "manual"}},
	}
	data, _ := json.Marshal(sopJSON)
	os.WriteFile(filepath.Join(sopsDir, "sop.json"), data, 0o644)
	os.WriteFile(filepath.Join(sopsDir, "steps.md"), []byte(`## Step 1: Hello
Do something.
`), 0o644)

	sops, err := LoadSOPs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sops) != 1 {
		t.Fatalf("期望 1 个 SOP，实际 %d", len(sops))
	}
	if sops[0].Name != "test-pipeline" {
		t.Errorf("名称应为 test-pipeline，实际 %q", sops[0].Name)
	}
	if len(sops[0].Steps) != 1 {
		t.Errorf("应有 1 步，实际 %d", len(sops[0].Steps))
	}
}

func TestLoadSOPsEmptyDir(t *testing.T) {
	dir := t.TempDir()
	sops, err := LoadSOPs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sops) != 0 {
		t.Errorf("空目录应返回 0 个 SOP，实际 %d", len(sops))
	}
}

// ---------------------------------------------------------------------------
// Engine 测试
// ---------------------------------------------------------------------------

type mockLLMExecutor struct {
	response string
}

func (m *mockLLMExecutor) ChatSimple(ctx context.Context, system, user, model string) (string, error) {
	return m.response, nil
}

type mockToolExecutor struct {
	result string
}

func (m *mockToolExecutor) ExecuteTool(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	return m.result, nil
}

func TestEngineAutoMode(t *testing.T) {
	engine := NewEngine(EngineConfig{
		LLM:   &mockLLMExecutor{response: "Done."},
		Model: "test-model",
	})

	s := Sop{
		Name:          "test",
		ExecutionMode: ModeAuto,
		Steps: []SopStep{
			{Number: 1, Title: "Step A", Body: "Do A"},
			{Number: 2, Title: "Step B", Body: "Do B"},
		},
	}

	run := engine.Execute(context.Background(), s)
	if run.Status != RunCompleted {
		t.Errorf("Auto 模式应完成，实际 %s (error: %s)", run.Status, run.Error)
	}
	if len(run.StepResults) != 2 {
		t.Errorf("应有 2 步结果，实际 %d", len(run.StepResults))
	}
}

func TestEngineSupervisedDecline(t *testing.T) {
	engine := NewEngine(EngineConfig{
		ConfirmFunc: func(title, desc string) bool { return false },
		LLM:        &mockLLMExecutor{response: "Done."},
	})

	s := Sop{
		Name:          "test",
		ExecutionMode: ModeSupervised,
		Steps:         []SopStep{{Number: 1, Title: "Step A", Body: "Do A"}},
	}

	run := engine.Execute(context.Background(), s)
	if run.Status != RunFailed {
		t.Errorf("拒绝确认应 Failed，实际 %s", run.Status)
	}
}

func TestEngineCheckpointDecline(t *testing.T) {
	engine := NewEngine(EngineConfig{
		ConfirmFunc: func(title, desc string) bool {
			return false // 拒绝 checkpoint
		},
		LLM:   &mockLLMExecutor{response: "Done."},
		Model: "test",
	})

	s := Sop{
		Name:          "test",
		ExecutionMode: ModeAuto,
		Steps: []SopStep{
			{Number: 1, Title: "Step A", Body: "Do A"},
			{Number: 2, Title: "Step B", Body: "Do B", IsCheckpoint: true},
		},
	}

	run := engine.Execute(context.Background(), s)
	// Step 1 完成，Step 2 checkpoint 被拒绝
	if run.Status != RunFailed {
		t.Errorf("Checkpoint 拒绝应 Failed，实际 %s", run.Status)
	}
	if len(run.StepResults) != 1 {
		t.Errorf("应有 1 步结果（Step 2 未执行），实际 %d", len(run.StepResults))
	}
}

func TestEngineDeterministicMode(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Tools: &mockToolExecutor{result: "tool output"},
	})

	s := Sop{
		Name:          "test",
		ExecutionMode: ModeDeterministic,
		Steps: []SopStep{
			{Number: 1, Title: "Run tool", Body: "Do it", SuggestedTools: []string{"exec"}},
		},
	}

	run := engine.Execute(context.Background(), s)
	if run.Status != RunCompleted {
		t.Errorf("Deterministic 应完成，实际 %s (error: %s)", run.Status, run.Error)
	}
}

func TestEngineStepByStepAllApproved(t *testing.T) {
	engine := NewEngine(EngineConfig{
		ConfirmFunc: func(title, desc string) bool { return true },
		LLM:        &mockLLMExecutor{response: "Done."},
		Model:       "test",
	})

	s := Sop{
		Name:          "test",
		ExecutionMode: ModeStepByStep,
		Steps: []SopStep{
			{Number: 1, Title: "A", Body: "Do A"},
			{Number: 2, Title: "B", Body: "Do B"},
			{Number: 3, Title: "C", Body: "Do C"},
		},
	}

	run := engine.Execute(context.Background(), s)
	if run.Status != RunCompleted {
		t.Errorf("全部确认应完成，实际 %s", run.Status)
	}
	if len(run.StepResults) != 3 {
		t.Errorf("应有 3 步结果，实际 %d", len(run.StepResults))
	}
}

func TestEngineContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	engine := NewEngine(EngineConfig{
		LLM: &mockLLMExecutor{response: "Done."},
	})

	s := Sop{
		Name:          "test",
		ExecutionMode: ModeAuto,
		Steps:         []SopStep{{Number: 1, Title: "A", Body: "Do A"}},
	}

	run := engine.Execute(ctx, s)
	if run.Status != RunFailed {
		t.Errorf("取消的 context 应 Failed，实际 %s", run.Status)
	}
}

func TestEngineGetRunAndListRuns(t *testing.T) {
	engine := NewEngine(EngineConfig{
		LLM: &mockLLMExecutor{response: "Done."},
	})

	s := Sop{
		Name:          "test",
		ExecutionMode: ModeAuto,
		Steps:         []SopStep{{Number: 1, Title: "A", Body: "Do A"}},
	}

	run := engine.Execute(context.Background(), s)
	if engine.GetRun(run.RunID) == nil {
		t.Error("GetRun 应能找到刚执行的 run")
	}

	runs := engine.ListRuns()
	if len(runs) != 1 {
		t.Errorf("ListRuns 应返回 1 个，实际 %d", len(runs))
	}
}
