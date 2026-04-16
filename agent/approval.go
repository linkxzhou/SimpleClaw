// 审批网关：工具调用前的审批拦截层。
// 按自治级别（Full/Supervised/ReadOnly）控制 Agent 操作权限，
// CLI 模式支持交互式确认（y/n/always），渠道模式自动拒绝需确认的操作。

package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ============ 核心类型 ============

// AutonomyLevel 自治级别。
type AutonomyLevel string

const (
	AutonomyFull       AutonomyLevel = "full"       // 全自动，不弹确认
	AutonomySupervised AutonomyLevel = "supervised" // 默认需确认，白名单豁免
	AutonomyReadOnly   AutonomyLevel = "read_only"  // 只读，写操作全部拒绝
)

// ApprovalResponse 审批决策。
type ApprovalResponse string

const (
	ApprovalYes    ApprovalResponse = "yes"    // 本次允许
	ApprovalNo     ApprovalResponse = "no"     // 本次拒绝
	ApprovalAlways ApprovalResponse = "always" // 允许并加入会话白名单
)

// ApprovalRequest 审批请求。
type ApprovalRequest struct {
	ToolName  string
	Arguments map[string]interface{}
	Summary   string // 人可读的操作摘要
}

// ApprovalLogEntry 审计日志条目。
type ApprovalLogEntry struct {
	Timestamp string           `json:"timestamp"`
	ToolName  string           `json:"tool_name"`
	Summary   string           `json:"summary"`
	Decision  ApprovalResponse `json:"decision"`
	Source    string           `json:"source"` // "auto_approved", "interactive", "non_interactive_denied", "read_only_denied"
}

// PromptFunc 交互式确认回调类型。
type PromptFunc func(req ApprovalRequest) ApprovalResponse

// ============ ApprovalManager ============

// ApprovalManager 管理工具调用审批。
type ApprovalManager struct {
	level            AutonomyLevel
	autoApprove      map[string]bool // 免确认工具
	alwaysAsk        map[string]bool // 强制确认工具
	interactive      bool            // CLI 模式 = true, 渠道模式 = false
	sessionAllowlist sync.Map        // 会话级 "Always" 白名单
	promptFunc       PromptFunc      // CLI 交互回调

	auditLog []ApprovalLogEntry
	auditMu  sync.Mutex
	logPath  string // 审计日志文件路径（空则不持久化）
	logger   *slog.Logger
}

// ApprovalConfig 审批配置。
type ApprovalConfig struct {
	Level       AutonomyLevel `json:"level"`
	AutoApprove []string      `json:"autoApprove,omitempty"` // 免确认工具列表
	AlwaysAsk   []string      `json:"alwaysAsk,omitempty"`   // 强制确认工具列表
}

// DefaultApprovalConfig 返回默认审批配置。
func DefaultApprovalConfig() ApprovalConfig {
	return ApprovalConfig{
		Level: AutonomyFull,
		AutoApprove: []string{
			"read_file", "list_dir", "web_search", "web_fetch",
		},
		AlwaysAsk: []string{"exec"},
	}
}

// NewApprovalManager 创建交互式审批管理器（CLI 模式）。
func NewApprovalManager(cfg ApprovalConfig, promptFn PromptFunc, logPath string, logger *slog.Logger) *ApprovalManager {
	if logger == nil {
		logger = slog.Default()
	}
	if promptFn == nil {
		promptFn = func(req ApprovalRequest) ApprovalResponse { return ApprovalNo }
	}
	return &ApprovalManager{
		level:       cfg.Level,
		autoApprove: toSet(cfg.AutoApprove),
		alwaysAsk:   toSet(cfg.AlwaysAsk),
		interactive: true,
		promptFunc:  promptFn,
		logPath:     logPath,
		logger:      logger,
	}
}

// NewNonInteractiveManager 创建非交互式审批管理器（渠道模式）。
func NewNonInteractiveManager(cfg ApprovalConfig, logPath string, logger *slog.Logger) *ApprovalManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &ApprovalManager{
		level:       cfg.Level,
		autoApprove: toSet(cfg.AutoApprove),
		alwaysAsk:   toSet(cfg.AlwaysAsk),
		interactive: false,
		logPath:     logPath,
		logger:      logger,
	}
}

// ============ 核心判断逻辑 ============

// NeedsApproval 判断工具是否需要审批。
func (am *ApprovalManager) NeedsApproval(toolName string) bool {
	switch am.level {
	case AutonomyFull:
		return false
	case AutonomyReadOnly:
		return isWriteTool(toolName)
	case AutonomySupervised:
		// autoApprove 列表中的工具免确认
		if am.autoApprove[toolName] {
			return false
		}
		// 会话白名单中的工具免确认（除非在 alwaysAsk 列表中）
		if _, ok := am.sessionAllowlist.Load(toolName); ok {
			if am.alwaysAsk[toolName] {
				return true
			}
			return false
		}
		return true
	default:
		return true
	}
}

// CheckApproval 执行审批检查，返回审批决策。
func (am *ApprovalManager) CheckApproval(toolName string, args map[string]interface{}) ApprovalResponse {
	// ReadOnly 模式：写工具直接拒绝
	if am.level == AutonomyReadOnly && isWriteTool(toolName) {
		am.logDecision(toolName, args, ApprovalNo, "read_only_denied")
		return ApprovalNo
	}

	// 不需要审批 → 自动通过
	if !am.NeedsApproval(toolName) {
		am.logDecision(toolName, args, ApprovalYes, "auto_approved")
		return ApprovalYes
	}

	// 非交互模式：需确认的工具自动拒绝
	if !am.interactive {
		am.logDecision(toolName, args, ApprovalNo, "non_interactive_denied")
		return ApprovalNo
	}

	// CLI 交互确认
	req := ApprovalRequest{
		ToolName:  toolName,
		Arguments: args,
		Summary:   GenerateSummary(toolName, args),
	}
	response := am.promptFunc(req)

	if response == ApprovalAlways {
		am.sessionAllowlist.Store(toolName, true)
		response = ApprovalYes // 返回 Yes，but 已记住
	}

	am.logDecision(toolName, args, response, "interactive")
	return response
}

// AsChecker 返回一个实现 tools.ApprovalChecker 接口的适配器。
func (am *ApprovalManager) AsChecker() approvalCheckerAdapter {
	return approvalCheckerAdapter{am: am}
}

// approvalCheckerAdapter 适配 ApprovalManager 到 tools.ApprovalChecker 接口。
type approvalCheckerAdapter struct {
	am *ApprovalManager
}

func (a approvalCheckerAdapter) CheckApproval(toolName string, args map[string]interface{}) string {
	return string(a.am.CheckApproval(toolName, args))
}

// GetLevel 返回当前自治级别。
func (am *ApprovalManager) GetLevel() AutonomyLevel {
	return am.level
}

// GetAuditLog 返回审计日志副本。
func (am *ApprovalManager) GetAuditLog() []ApprovalLogEntry {
	am.auditMu.Lock()
	defer am.auditMu.Unlock()
	cp := make([]ApprovalLogEntry, len(am.auditLog))
	copy(cp, am.auditLog)
	return cp
}

// ============ 操作摘要 ============

// GenerateSummary 生成人可读的操作摘要。
func GenerateSummary(toolName string, args map[string]interface{}) string {
	switch toolName {
	case "exec":
		if cmd, ok := args["command"].(string); ok {
			return fmt.Sprintf("Execute command: %s", truncSummary(cmd, 120))
		}
	case "write_file":
		path, _ := args["path"].(string)
		content, _ := args["content"].(string)
		return fmt.Sprintf("Write to: %s (%d bytes)", path, len(content))
	case "edit_file":
		path, _ := args["path"].(string)
		oldText, _ := args["old_text"].(string)
		return fmt.Sprintf("Edit: %s (replace %d chars)", path, len(oldText))
	case "message":
		channel, _ := args["channel"].(string)
		content, _ := args["content"].(string)
		return fmt.Sprintf("Send message to %s: %s", channel, truncSummary(content, 80))
	case "spawn":
		task, _ := args["task"].(string)
		return fmt.Sprintf("Spawn subagent: %s", truncSummary(task, 80))
	case "go_run":
		code, _ := args["code"].(string)
		return fmt.Sprintf("Run Go code (%d chars)", len(code))
	case "go_agent":
		task, _ := args["task"].(string)
		return fmt.Sprintf("Run Go agent: %s", truncSummary(task, 80))
	}
	return fmt.Sprintf("%s(%d args)", toolName, len(args))
}

// ============ 工具分类 ============

// 写操作工具集合
var writeTools = map[string]bool{
	"write_file": true,
	"edit_file":  true,
	"exec":       true,
	"message":    true,
	"spawn":      true,
	"go_run":     true,
	"go_agent":   true,
}

// isWriteTool 判断工具是否为写操作。
func isWriteTool(name string) bool {
	return writeTools[name]
}

// ============ 审计日志 ============

func (am *ApprovalManager) logDecision(toolName string, args map[string]interface{}, decision ApprovalResponse, source string) {
	entry := ApprovalLogEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		ToolName:  toolName,
		Summary:   GenerateSummary(toolName, args),
		Decision:  decision,
		Source:    source,
	}

	am.auditMu.Lock()
	am.auditLog = append(am.auditLog, entry)
	am.auditMu.Unlock()

	am.logger.Debug("approval decision",
		"tool", toolName,
		"decision", decision,
		"source", source,
	)

	// 异步持久化
	if am.logPath != "" {
		go am.persistEntry(entry)
	}
}

func (am *ApprovalManager) persistEntry(entry ApprovalLogEntry) {
	dir := filepath.Dir(am.logPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		am.logger.Warn("failed to create audit dir", "error", err)
		return
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	data = append(data, '\n')

	f, err := os.OpenFile(am.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		am.logger.Warn("failed to open audit log", "error", err)
		return
	}
	defer f.Close()
	f.Write(data)
}

// ============ 辅助函数 ============

func toSet(items []string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, item := range items {
		m[item] = true
	}
	return m
}

// CLIPrompt 终端交互式审批确认（y/n/a）。
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

func truncSummary(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
