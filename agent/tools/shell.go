// Shell 命令执行工具，带安全防护。
// 包含 ~30 条默认拒绝正则规则，路径遍历防护，可配置白名单。

package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// ExecTool Shell 命令执行工具（含安全防护）
type ExecTool struct {
	WorkingDir    string
	Timeout       time.Duration
	MaxOutput     int
	Workspace     string           // 工作区路径（用于路径检查）
	RestrictToWS  bool             // 是否限制在工作区内
	DenyPatterns  []*regexp.Regexp // 自定义拒绝模式
	AllowPatterns []*regexp.Regexp // 白名单模式（覆盖拒绝）
}

// NewExecTool 创建带安全防护的 Shell 执行工具
func NewExecTool(workingDir string) *ExecTool {
	return &ExecTool{
		WorkingDir:   workingDir,
		Timeout:      60 * time.Second,
		MaxOutput:    10000,
		DenyPatterns: defaultDenyPatterns(),
	}
}

func (t *ExecTool) Name() string { return "exec" }
func (t *ExecTool) Description() string {
	return "Execute a shell command and return its output. Dangerous commands are blocked for safety."
}
func (t *ExecTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The shell command to execute",
			},
			"working_dir": map[string]interface{}{
				"type":        "string",
				"description": "Optional working directory for the command",
			},
		},
		"required": []string{"command"},
	}
}

func (t *ExecTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	command, _ := params["command"].(string)
	if command == "" {
		return "", fmt.Errorf("command is required")
	}

	// 安全检查
	if err := t.checkSafety(command); err != nil {
		return fmt.Sprintf("Error: Command blocked - %s", err.Error()), nil
	}

	workingDir, _ := params["working_dir"].(string)
	if workingDir == "" {
		workingDir = t.WorkingDir
	}
	if workingDir == "" {
		workingDir, _ = os.Getwd()
	}

	timeout := t.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = workingDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	var parts []string

	if stdout.Len() > 0 {
		parts = append(parts, stdout.String())
	}

	if stderr.Len() > 0 {
		parts = append(parts, fmt.Sprintf("STDERR:\n%s", stderr.String()))
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Sprintf("Error: Command timed out after %s", timeout), nil
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			parts = append(parts, fmt.Sprintf("\nExit code: %d", exitErr.ExitCode()))
		}
	}

	result := "(no output)"
	if len(parts) > 0 {
		result = strings.Join(parts, "\n")
	}

	// 截断过长输出
	maxLen := t.MaxOutput
	if maxLen == 0 {
		maxLen = 10000
	}
	if len(result) > maxLen {
		result = result[:maxLen] + fmt.Sprintf("\n... (truncated, %d more chars)", len(result)-maxLen)
	}

	return result, nil
}

// checkSafety 执行安全检查：白名单 → 拒绝列表 → 路径遍历。
func (t *ExecTool) checkSafety(command string) error {
	// 1. 白名单检查（优先通过）
	for _, p := range t.AllowPatterns {
		if p.MatchString(command) {
			return nil
		}
	}

	// 2. 拒绝列表检查
	for _, p := range t.DenyPatterns {
		if p.MatchString(command) {
			return fmt.Errorf("matches deny pattern: %s", p.String())
		}
	}

	// 3. 路径遍历检查
	if t.RestrictToWS && t.Workspace != "" {
		if strings.Contains(command, "../") {
			return fmt.Errorf("path traversal not allowed")
		}
	}

	return nil
}

// defaultDenyPatterns 返回默认的命令拒绝规则。
func defaultDenyPatterns() []*regexp.Regexp {
	patterns := []string{
		// 文件删除
		`(?i)\brm\s+.*-[rRf]`,
		`(?i)\brm\s+-[rRf]`,
		`(?i)\brmdir\s+/`,
		// 磁盘操作
		`(?i)\bdd\s+if=`,
		`(?i)\bmkfs\b`,
		// 系统操作
		`(?i)\bshutdown\b`,
		`(?i)\breboot\b`,
		`(?i)\binit\s+[06]\b`,
		// 权限提升
		`(?i)\bsudo\b`,
		`(?i)\bsu\s+-`,
		`(?i)\bchmod\s+[0-7]{3,4}\b`,
		`(?i)\bchown\b`,
		// 命令注入
		`\$\(`,
		`(?i)\|\s*(sh|bash|zsh|fish)\b`,
		`(?i)\beval\b`,
		// 进程管理
		`(?i)\bkill\s+-9`,
		`(?i)\bkillall\b`,
		`(?i)\bpkill\b`,
		// 网络危险操作
		`(?i)curl.*\|\s*(sh|bash)`,
		`(?i)wget.*\|\s*(sh|bash)`,
		// 包管理全局安装
		`(?i)\bnpm\s+install\s+-g`,
		`(?i)\bpip\s+install\s+--user`,
		// 容器操作
		`(?i)\bdocker\s+(run|exec)\b`,
		// Git 危险操作
		`(?i)\bgit\s+push\b`,
		`(?i)\bgit\s+.*--force`,
		// Fork bomb
		`:\(\)\s*\{`,
	}

	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		compiled = append(compiled, regexp.MustCompile(p))
	}
	return compiled
}
