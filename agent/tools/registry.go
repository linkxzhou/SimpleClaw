package tools

import (
	"context"
	"fmt"
	"sync"
)

// Tool 工具接口（本地定义，避免循环导入）
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]interface{}
	Execute(ctx context.Context, params map[string]interface{}) (string, error)
}

// ToolDef 工具定义（OpenAI function calling 格式）
type ToolDef struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction 工具函数定义
type ToolFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

// ToolToSchema 将 Tool 转为 ToolDef
func ToolToSchema(t Tool) ToolDef {
	return ToolDef{
		Type: "function",
		Function: ToolFunction{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		},
	}
}

// ApprovalChecker 审批检查接口（由 agent.ApprovalManager 实现）。
// 定义在 tools 包内以避免 agent↔tools 循环依赖。
type ApprovalChecker interface {
	// CheckApproval 返回 "yes"/"no"/"always"。
	CheckApproval(toolName string, args map[string]interface{}) string
}

// Registry 工具注册中心
type Registry struct {
	mu              sync.RWMutex
	tools           map[string]Tool
	approvalChecker ApprovalChecker // 可选，nil 时不做审批检查
}

// NewRegistry 创建工具注册中心
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// SetApprovalChecker 设置审批检查器。
func (r *Registry) SetApprovalChecker(checker ApprovalChecker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.approvalChecker = checker
}

// Register 注册工具
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

// Unregister 注销工具
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tools, name)
}

// Get 获取工具
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// Has 检查工具是否已注册
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.tools[name]
	return ok
}

// GetDefinitions 获取所有工具定义（OpenAI 格式）
func (r *Registry) GetDefinitions() []ToolDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]ToolDef, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, ToolToSchema(t))
	}
	return defs
}

// Execute 执行工具（含审批检查）。
func (r *Registry) Execute(ctx context.Context, name string, params map[string]interface{}) string {
	r.mu.RLock()
	t, ok := r.tools[name]
	checker := r.approvalChecker
	r.mu.RUnlock()

	if !ok {
		return fmt.Sprintf("Error: Tool '%s' not found", name)
	}

	// 审批检查
	if checker != nil {
		decision := checker.CheckApproval(name, params)
		if decision == "no" {
			return fmt.Sprintf("Tool '%s' denied by approval policy", name)
		}
	}

	result, err := t.Execute(ctx, params)
	if err != nil {
		return fmt.Sprintf("Error executing %s: %s", name, err.Error())
	}
	return result
}

// Names 获取所有已注册工具名称
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// Len 已注册工具数量
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// RegisterCommonTools 注册主 Agent 和子 Agent 共用的工具集。
// 新增工具时只需改此处一处。
func RegisterCommonTools(r *Registry, workspace, braveAPIKey string) {
	// 文件工具
	r.Register(&ReadFileTool{})
	r.Register(&WriteFileTool{})
	r.Register(&EditFileTool{})
	r.Register(&ListDirTool{})

	// Shell 工具
	r.Register(NewExecTool(workspace))

	// Web 工具
	r.Register(NewWebSearchTool(braveAPIKey))
	r.Register(NewWebFetchTool())

	// Go 动态执行工具
	r.Register(NewGoRunTool())
	r.Register(NewGoAgentTool())
}
