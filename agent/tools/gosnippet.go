// GoSnippetTool 管理和执行保存的 Go 代码片段。
// 支持 save/list/run/delete 四种操作模式。
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Snippet 是代码片段的外部表示（与 agent.Snippet 同构）。
type Snippet struct {
	ID          string
	Name        string
	Description string
	Code        string
	FuncName    string
	Tags        []string
	UseCount    int
}

// SnippetProvider 定义 GoSnippetTool 所需的片段存储操作接口，
// 避免 tools → agent 的循环依赖。由 agent.SnippetStore 实现。
type SnippetProvider interface {
	SaveSnippet(id, name, description, code, funcName string, tags []string) error
	GetSnippet(id string) (*Snippet, bool)
	GetSnippetByName(name string) (*Snippet, bool)
	DeleteSnippet(id string) error
	SearchSnippets(query string) []*Snippet
	RecordSnippetUse(id string)
}

// GoSnippetTool 管理和执行保存的代码片段。
type GoSnippetTool struct {
	Store     SnippetProvider
	Timeout   time.Duration
	MaxOutput int
}

// NewGoSnippetTool 创建 GoSnippetTool。store 不可为 nil。
func NewGoSnippetTool(store SnippetProvider) *GoSnippetTool {
	return &GoSnippetTool{
		Store:     store,
		Timeout:   30 * time.Second,
		MaxOutput: 50000,
	}
}

func (t *GoSnippetTool) Name() string { return "go_snippet" }
func (t *GoSnippetTool) Description() string {
	return `Manage and execute saved Go code snippets for reuse.
Actions:
- "save"   — Save code as a reusable snippet (requires: name, code; optional: description, tags, func_name)
- "list"   — List saved snippets (optional: query to filter)
- "run"    — Execute a saved snippet by name or id (optional: input as JSON object)
- "delete" — Delete a snippet by name or id`
}

func (t *GoSnippetTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Operation: save, list, run, delete",
				"enum":        []string{"save", "list", "run", "delete"},
			},
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Snippet name (for save/run/delete)",
			},
			"code": map[string]interface{}{
				"type":        "string",
				"description": "Go source code (for save)",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "What the snippet does (for save)",
			},
			"func_name": map[string]interface{}{
				"type":        "string",
				"description": "Entry function name (default: Run, for save)",
			},
			"tags": map[string]interface{}{
				"type":        "string",
				"description": "Comma-separated tags (for save)",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query (for list)",
			},
			"id": map[string]interface{}{
				"type":        "string",
				"description": "Snippet ID (alternative to name for run/delete)",
			},
			"input": map[string]interface{}{
				"type":        "object",
				"description": "Input data passed to the snippet's entry function (for run)",
			},
		},
		"required": []string{"action"},
	}
}

func (t *GoSnippetTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	action, _ := params["action"].(string)
	switch action {
	case "save":
		return t.doSave(params)
	case "list":
		return t.doList(params)
	case "run":
		return t.doRun(ctx, params)
	case "delete":
		return t.doDelete(params)
	default:
		return "", fmt.Errorf("unknown action %q, expected: save, list, run, delete", action)
	}
}

func (t *GoSnippetTool) doSave(params map[string]interface{}) (string, error) {
	name, _ := params["name"].(string)
	code, _ := params["code"].(string)
	if name == "" || code == "" {
		return "", fmt.Errorf("'name' and 'code' are required for save")
	}

	desc, _ := params["description"].(string)
	funcName, _ := params["func_name"].(string)
	if funcName == "" {
		funcName = "Run"
	}

	var tags []string
	if tagsStr, ok := params["tags"].(string); ok && tagsStr != "" {
		for _, tag := range strings.Split(tagsStr, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				tags = append(tags, tag)
			}
		}
	}

	id := slugify(name)
	if err := t.Store.SaveSnippet(id, name, desc, code, funcName, tags); err != nil {
		return "", fmt.Errorf("save snippet: %w", err)
	}
	return fmt.Sprintf("Snippet saved: %s (id: %s)", name, id), nil
}

func (t *GoSnippetTool) doList(params map[string]interface{}) (string, error) {
	query, _ := params["query"].(string)
	snippets := t.Store.SearchSnippets(query)
	if len(snippets) == 0 {
		if query != "" {
			return fmt.Sprintf("No snippets matching %q", query), nil
		}
		return "No saved snippets.", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d snippet(s):\n\n", len(snippets)))
	for _, sn := range snippets {
		sb.WriteString(fmt.Sprintf("- **%s** (id: %s)\n", sn.Name, sn.ID))
		if sn.Description != "" {
			sb.WriteString(fmt.Sprintf("  %s\n", sn.Description))
		}
		if len(sn.Tags) > 0 {
			sb.WriteString(fmt.Sprintf("  Tags: %s\n", strings.Join(sn.Tags, ", ")))
		}
		sb.WriteString(fmt.Sprintf("  Used: %d times | Func: %s\n", sn.UseCount, sn.FuncName))
	}
	return sb.String(), nil
}

func (t *GoSnippetTool) doRun(ctx context.Context, params map[string]interface{}) (string, error) {
	sn := t.resolveSnippet(params)
	if sn == nil {
		return "", fmt.Errorf("snippet not found — provide 'name' or 'id'")
	}

	// 解析 input — 对于 Run(input map[string]interface{}) 签名，始终传递 map
	input := make(map[string]interface{})
	if inputRaw, ok := params["input"]; ok && inputRaw != nil {
		switch v := inputRaw.(type) {
		case map[string]interface{}:
			input = v
		case string:
			_ = json.Unmarshal([]byte(v), &input)
		}
	}

	t.Store.RecordSnippetUse(sn.ID)
	prefix := fmt.Sprintf("[Snippet: %s]", sn.Name)
	return goExec(ctx, sn.Code, sn.FuncName, prefix, t.Timeout, t.MaxOutput, input)
}

func (t *GoSnippetTool) doDelete(params map[string]interface{}) (string, error) {
	sn := t.resolveSnippet(params)
	if sn == nil {
		return "", fmt.Errorf("snippet not found — provide 'name' or 'id'")
	}
	if err := t.Store.DeleteSnippet(sn.ID); err != nil {
		return "", fmt.Errorf("delete snippet: %w", err)
	}
	return fmt.Sprintf("Snippet deleted: %s (id: %s)", sn.Name, sn.ID), nil
}

// resolveSnippet 通过 id 或 name 查找片段。
func (t *GoSnippetTool) resolveSnippet(params map[string]interface{}) *Snippet {
	if id, _ := params["id"].(string); id != "" {
		if sn, ok := t.Store.GetSnippet(id); ok {
			return sn
		}
	}
	if name, _ := params["name"].(string); name != "" {
		if sn, ok := t.Store.GetSnippetByName(name); ok {
			return sn
		}
		// 也尝试用 name 作为 id 查找
		if sn, ok := t.Store.GetSnippet(slugify(name)); ok {
			return sn
		}
	}
	return nil
}

// slugify 将名称转换为 URL-safe ID。
func slugify(name string) string {
	lower := strings.ToLower(name)
	var sb strings.Builder
	prevDash := false
	for _, c := range lower {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			sb.WriteRune(c)
			prevDash = false
		case c == ' ' || c == '_' || c == '-' || c == '.':
			if !prevDash && sb.Len() > 0 {
				sb.WriteRune('-')
				prevDash = true
			}
		}
	}
	result := sb.String()
	// 去掉尾部 dash
	return strings.TrimRight(result, "-")
}
