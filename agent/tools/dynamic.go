// DynamicTool 和 DynamicToolManager：将代码片段升级为一等工具。
// DynamicTool 预编译 goscript 代码，实现 Tool 接口。
// DynamicToolManager 管理升级/降级和持久化。
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleClaw/goscript"
)

// DynamicTool 由 goscript 代码驱动的动态工具。
// 预编译代码以避免重复编译开销。
type DynamicTool struct {
	name        string
	description string
	paramSchema map[string]interface{}
	code        string
	funcName    string
	program     *goscript.Program
	timeout     time.Duration
	maxOutput   int
}

func (t *DynamicTool) Name() string                       { return t.name }
func (t *DynamicTool) Description() string                { return t.description }
func (t *DynamicTool) Parameters() map[string]interface{} { return t.paramSchema }

func (t *DynamicTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	// 将 params 作为 input 传递给预编译程序
	input := make(map[string]interface{})
	if inputRaw, ok := params["input"]; ok {
		switch v := inputRaw.(type) {
		case map[string]interface{}:
			input = v
		case string:
			_ = json.Unmarshal([]byte(v), &input)
		}
	} else {
		// 直接将所有 params（除去元字段）作为 input
		for k, v := range params {
			input[k] = v
		}
	}

	prefix := fmt.Sprintf("[DynamicTool: %s]", t.name)

	// 使用预编译程序执行
	if t.program != nil {
		return t.executePrecompiled(ctx, input, prefix)
	}
	// 回退到常规 goExec
	return goExec(ctx, t.code, t.funcName, prefix, t.timeout, t.maxOutput, input)
}

func (t *DynamicTool) executePrecompiled(ctx context.Context, input map[string]interface{}, prefix string) (string, error) {
	resultCh := make(chan goExecResult, 1)
	traceID := fmt.Sprintf("dynamic-%s-%d", t.name, time.Now().UnixNano())
	go func() {
		val, execCtx, err := t.program.RunWithContext(traceID, t.funcName, input)
		resultCh <- goExecResult{value: val, ctx: execCtx, err: err}
	}()

	timeout := t.timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	select {
	case res := <-resultCh:
		if res.err != nil {
			return fmt.Sprintf("%s\n\n[Runtime Error]\n%s", prefix, res.err.Error()), nil
		}
		var parts []string
		parts = append(parts, prefix)
		if res.ctx != nil {
			if output := res.ctx.Output(); output != "" {
				parts = append(parts, fmt.Sprintf("Output:\n%s", output))
			}
		}
		if res.value != nil {
			parts = append(parts, fmt.Sprintf("Return: %v", res.value))
		}
		result := strings.Join(parts, "\n\n")
		if t.maxOutput > 0 && len(result) > t.maxOutput {
			result = result[:t.maxOutput] + fmt.Sprintf("\n... (truncated, %d more chars)", len(result)-t.maxOutput)
		}
		return result, nil
	case <-time.After(timeout):
		return fmt.Sprintf("%s Error: timed out after %s", prefix, timeout), nil
	case <-ctx.Done():
		return prefix + " Error: execution cancelled", nil
	}
}

// dynamicToolConfig 持久化配置。
type dynamicToolConfig struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Code        string                 `json:"code"`
	FuncName    string                 `json:"func_name"`
	ParamSchema map[string]interface{} `json:"param_schema,omitempty"`
	SnippetID   string                 `json:"snippet_id,omitempty"`
}

// DynamicToolManager 管理动态注册的工具。
type DynamicToolManager struct {
	registry *Registry
	dir      string // 持久化目录
	configs  map[string]*dynamicToolConfig
	mu       sync.RWMutex
}

// NewDynamicToolManager 创建动态工具管理器。
func NewDynamicToolManager(registry *Registry, dir string) (*DynamicToolManager, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create dynamic tools dir: %w", err)
	}
	m := &DynamicToolManager{
		registry: registry,
		dir:      dir,
		configs:  make(map[string]*dynamicToolConfig),
	}
	return m, nil
}

// Promote 将代码升级为动态工具并注册到 Registry。
func (m *DynamicToolManager) Promote(name, description, code, funcName string, paramSchema map[string]interface{}, snippetID string) error {
	if name == "" || code == "" {
		return fmt.Errorf("name and code are required")
	}
	if funcName == "" {
		funcName = "Run"
	}

	// 确保 package 声明
	trimmed := strings.TrimSpace(code)
	if !strings.HasPrefix(trimmed, "package ") {
		code = "package main\n\n" + code
	}

	// 预编译验证
	traceID := fmt.Sprintf("dynamic-promote-%s", name)
	program, err := goscript.Compile(traceID, "main", code)
	if err != nil {
		return fmt.Errorf("compile error: %w", err)
	}

	// 默认参数 schema
	if paramSchema == nil {
		paramSchema = map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"input": map[string]interface{}{
					"type":        "object",
					"description": "Input data for the tool",
				},
			},
		}
	}

	tool := &DynamicTool{
		name:        name,
		description: description,
		paramSchema: paramSchema,
		code:        code,
		funcName:    funcName,
		program:     program,
		timeout:     30 * time.Second,
		maxOutput:   50000,
	}

	// 注册
	m.registry.Register(tool)

	// 持久化
	cfg := &dynamicToolConfig{
		Name:        name,
		Description: description,
		Code:        code,
		FuncName:    funcName,
		ParamSchema: paramSchema,
		SnippetID:   snippetID,
	}
	m.mu.Lock()
	m.configs[name] = cfg
	m.mu.Unlock()

	return m.persistConfig(cfg)
}

// Demote 将动态工具注销并删除持久化配置。
func (m *DynamicToolManager) Demote(name string) error {
	m.mu.Lock()
	_, ok := m.configs[name]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("dynamic tool %q not found", name)
	}
	delete(m.configs, name)
	m.mu.Unlock()

	m.registry.Unregister(name)
	return os.Remove(filepath.Join(m.dir, name+".json"))
}

// List 返回所有动态工具名称。
func (m *DynamicToolManager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.configs))
	for name := range m.configs {
		names = append(names, name)
	}
	return names
}

// GetConfig 获取动态工具配置。
func (m *DynamicToolManager) GetConfig(name string) (*dynamicToolConfig, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg, ok := m.configs[name]
	return cfg, ok
}

// LoadPersistedTools 启动时加载之前注册的动态工具。
func (m *DynamicToolManager) LoadPersistedTools() (int, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	loaded := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(m.dir, e.Name()))
		if err != nil {
			continue
		}
		var cfg dynamicToolConfig
		if json.Unmarshal(data, &cfg) != nil || cfg.Name == "" {
			continue
		}
		if err := m.Promote(cfg.Name, cfg.Description, cfg.Code, cfg.FuncName, cfg.ParamSchema, cfg.SnippetID); err != nil {
			continue
		}
		loaded++
	}
	return loaded, nil
}

func (m *DynamicToolManager) persistConfig(cfg *dynamicToolConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(m.dir, cfg.Name+".json"), data, 0o644)
}
