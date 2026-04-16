package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// AutonomyLevel + NeedsApproval 测试
// ---------------------------------------------------------------------------

func TestFullModeNeverNeedsApproval(t *testing.T) {
	am := NewApprovalManager(ApprovalConfig{
		Level: AutonomyFull,
	}, nil, "", nil)

	for _, tool := range []string{"exec", "write_file", "read_file", "go_run"} {
		if am.NeedsApproval(tool) {
			t.Errorf("Full 模式下 %q 不应需要审批", tool)
		}
	}
}

func TestReadOnlyModeBlocksWriteTools(t *testing.T) {
	am := NewApprovalManager(ApprovalConfig{
		Level: AutonomyReadOnly,
	}, nil, "", nil)

	writeToolNames := []string{"exec", "write_file", "edit_file", "message", "spawn", "go_run", "go_agent"}
	for _, tool := range writeToolNames {
		if !am.NeedsApproval(tool) {
			t.Errorf("ReadOnly 模式下 %q 应需要审批（应被拒绝）", tool)
		}
	}

	// 只读工具不需要审批
	if am.NeedsApproval("read_file") {
		t.Error("ReadOnly 模式下 read_file 不应需要审批")
	}
}

func TestSupervisedModeAutoApprove(t *testing.T) {
	am := NewApprovalManager(ApprovalConfig{
		Level:       AutonomySupervised,
		AutoApprove: []string{"read_file", "list_dir"},
	}, nil, "", nil)

	if am.NeedsApproval("read_file") {
		t.Error("autoApprove 列表中的工具不应需要审批")
	}
	if am.NeedsApproval("list_dir") {
		t.Error("autoApprove 列表中的工具不应需要审批")
	}
	if !am.NeedsApproval("write_file") {
		t.Error("不在 autoApprove 列表中的写工具应需要审批")
	}
}

func TestSupervisedModeAlwaysAsk(t *testing.T) {
	am := NewApprovalManager(ApprovalConfig{
		Level:     AutonomySupervised,
		AlwaysAsk: []string{"exec"},
	}, nil, "", nil)

	// 即使加入会话白名单，alwaysAsk 工具仍需审批
	am.sessionAllowlist.Store("exec", true)
	if !am.NeedsApproval("exec") {
		t.Error("alwaysAsk 列表中的工具应始终需要审批")
	}
}

func TestSupervisedModeSessionAllowlist(t *testing.T) {
	am := NewApprovalManager(ApprovalConfig{
		Level: AutonomySupervised,
	}, nil, "", nil)

	if !am.NeedsApproval("write_file") {
		t.Error("初始状态下 write_file 应需要审批")
	}

	// 模拟用户选择 "always"
	am.sessionAllowlist.Store("write_file", true)
	if am.NeedsApproval("write_file") {
		t.Error("加入会话白名单后不应需要审批")
	}
}

// ---------------------------------------------------------------------------
// CheckApproval 测试
// ---------------------------------------------------------------------------

func TestCheckApprovalFullMode(t *testing.T) {
	am := NewApprovalManager(ApprovalConfig{Level: AutonomyFull}, nil, "", nil)
	resp := am.CheckApproval("exec", map[string]interface{}{"command": "rm -rf /"})
	if resp != ApprovalYes {
		t.Errorf("Full 模式应返回 Yes，实际 %s", resp)
	}
}

func TestCheckApprovalReadOnlyDenied(t *testing.T) {
	am := NewApprovalManager(ApprovalConfig{Level: AutonomyReadOnly}, nil, "", nil)
	resp := am.CheckApproval("write_file", map[string]interface{}{"path": "/tmp/test"})
	if resp != ApprovalNo {
		t.Errorf("ReadOnly 模式写工具应返回 No，实际 %s", resp)
	}
}

func TestCheckApprovalNonInteractiveDenied(t *testing.T) {
	am := NewNonInteractiveManager(ApprovalConfig{
		Level: AutonomySupervised,
	}, "", nil)
	resp := am.CheckApproval("write_file", map[string]interface{}{"path": "/tmp/test"})
	if resp != ApprovalNo {
		t.Errorf("非交互模式需确认的工具应返回 No，实际 %s", resp)
	}
}

func TestCheckApprovalNonInteractiveAutoApprove(t *testing.T) {
	am := NewNonInteractiveManager(ApprovalConfig{
		Level:       AutonomySupervised,
		AutoApprove: []string{"read_file"},
	}, "", nil)
	resp := am.CheckApproval("read_file", map[string]interface{}{"path": "/tmp/test"})
	if resp != ApprovalYes {
		t.Errorf("autoApprove 工具应返回 Yes，实际 %s", resp)
	}
}

func TestCheckApprovalInteractiveYes(t *testing.T) {
	am := NewApprovalManager(ApprovalConfig{
		Level: AutonomySupervised,
	}, func(req ApprovalRequest) ApprovalResponse {
		return ApprovalYes
	}, "", nil)

	resp := am.CheckApproval("write_file", map[string]interface{}{"path": "/tmp/test"})
	if resp != ApprovalYes {
		t.Errorf("交互确认返回 Yes 时应通过，实际 %s", resp)
	}
}

func TestCheckApprovalInteractiveNo(t *testing.T) {
	am := NewApprovalManager(ApprovalConfig{
		Level: AutonomySupervised,
	}, func(req ApprovalRequest) ApprovalResponse {
		return ApprovalNo
	}, "", nil)

	resp := am.CheckApproval("write_file", map[string]interface{}{"path": "/tmp/test"})
	if resp != ApprovalNo {
		t.Errorf("交互确认返回 No 时应拒绝，实际 %s", resp)
	}
}

func TestCheckApprovalInteractiveAlways(t *testing.T) {
	am := NewApprovalManager(ApprovalConfig{
		Level: AutonomySupervised,
	}, func(req ApprovalRequest) ApprovalResponse {
		return ApprovalAlways
	}, "", nil)

	// 第一次调用触发交互
	resp := am.CheckApproval("write_file", map[string]interface{}{"path": "/tmp/test"})
	if resp != ApprovalYes { // Always 被转为 Yes
		t.Errorf("Always 应转为 Yes，实际 %s", resp)
	}

	// 第二次不应再需要审批
	if am.NeedsApproval("write_file") {
		t.Error("选择 Always 后不应再需要审批")
	}
}

// ---------------------------------------------------------------------------
// GenerateSummary 测试
// ---------------------------------------------------------------------------

func TestGenerateSummary(t *testing.T) {
	tests := []struct {
		tool    string
		args    map[string]interface{}
		contain string
	}{
		{"exec", map[string]interface{}{"command": "ls -la"}, "Execute command: ls -la"},
		{"write_file", map[string]interface{}{"path": "/tmp/a.txt", "content": "hello"}, "Write to: /tmp/a.txt"},
		{"edit_file", map[string]interface{}{"path": "/tmp/b.txt", "old_text": "foo"}, "Edit: /tmp/b.txt"},
		{"message", map[string]interface{}{"channel": "telegram", "content": "hello"}, "Send message to telegram"},
		{"unknown_tool", map[string]interface{}{"a": 1}, "unknown_tool(1 args)"},
	}
	for _, tt := range tests {
		got := GenerateSummary(tt.tool, tt.args)
		if got == "" || !contains(got, tt.contain) {
			t.Errorf("GenerateSummary(%q) = %q，应包含 %q", tt.tool, got, tt.contain)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && findSubstring(s, substr))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// 审计日志测试
// ---------------------------------------------------------------------------

func TestAuditLogRecorded(t *testing.T) {
	am := NewApprovalManager(ApprovalConfig{Level: AutonomyFull}, nil, "", nil)
	am.CheckApproval("read_file", map[string]interface{}{"path": "/tmp/test"})
	am.CheckApproval("exec", map[string]interface{}{"command": "ls"})

	log := am.GetAuditLog()
	if len(log) != 2 {
		t.Errorf("期望 2 条审计日志，实际 %d", len(log))
	}
	if log[0].ToolName != "read_file" {
		t.Errorf("第一条日志工具名应为 read_file，实际 %s", log[0].ToolName)
	}
	if log[0].Decision != ApprovalYes {
		t.Errorf("Full 模式决策应为 yes，实际 %s", log[0].Decision)
	}
}

func TestAuditLogPersisted(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")

	am := NewApprovalManager(ApprovalConfig{Level: AutonomyFull}, nil, logPath, nil)
	am.CheckApproval("exec", map[string]interface{}{"command": "echo hello"})

	// 等待异步写入
	time.Sleep(100 * time.Millisecond)

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("审计日志文件应被创建: %v", err)
	}
	if len(data) == 0 {
		t.Error("审计日志文件不应为空")
	}
}

// ---------------------------------------------------------------------------
// isWriteTool 测试
// ---------------------------------------------------------------------------

func TestIsWriteTool(t *testing.T) {
	writePairs := map[string]bool{
		"write_file": true,
		"edit_file":  true,
		"exec":       true,
		"message":    true,
		"spawn":      true,
		"go_run":     true,
		"go_agent":   true,
		"read_file":  false,
		"list_dir":   false,
		"web_search": false,
		"web_fetch":  false,
	}
	for tool, expected := range writePairs {
		if got := isWriteTool(tool); got != expected {
			t.Errorf("isWriteTool(%q) = %v, 期望 %v", tool, got, expected)
		}
	}
}

// ---------------------------------------------------------------------------
// AsChecker 适配器测试
// ---------------------------------------------------------------------------

func TestAsCheckerInterface(t *testing.T) {
	am := NewApprovalManager(ApprovalConfig{Level: AutonomyFull}, nil, "", nil)
	checker := am.AsChecker()

	result := checker.CheckApproval("exec", map[string]interface{}{"command": "ls"})
	if result != "yes" {
		t.Errorf("Full 模式 checker 应返回 'yes'，实际 %q", result)
	}
}

func TestAsCheckerReadOnly(t *testing.T) {
	am := NewApprovalManager(ApprovalConfig{Level: AutonomyReadOnly}, nil, "", nil)
	checker := am.AsChecker()

	result := checker.CheckApproval("write_file", map[string]interface{}{"path": "/tmp/test"})
	if result != "no" {
		t.Errorf("ReadOnly 模式 checker 写工具应返回 'no'，实际 %q", result)
	}
}

// ---------------------------------------------------------------------------
// GetLevel 测试
// ---------------------------------------------------------------------------

func TestGetLevel(t *testing.T) {
	tests := []struct {
		level    AutonomyLevel
		expected AutonomyLevel
	}{
		{AutonomyFull, AutonomyFull},
		{AutonomySupervised, AutonomySupervised},
		{AutonomyReadOnly, AutonomyReadOnly},
	}
	for _, tt := range tests {
		am := NewApprovalManager(ApprovalConfig{Level: tt.level}, nil, "", nil)
		if got := am.GetLevel(); got != tt.expected {
			t.Errorf("GetLevel() = %q, 期望 %q", got, tt.expected)
		}
	}
}
