package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// testSnippetStore 是 SnippetProvider 接口的纯内存测试实现，
// 不依赖 agent 包，避免循环依赖。
type testSnippetStore struct {
	mu       sync.RWMutex
	snippets map[string]*Snippet
}

func newTestStore(t *testing.T) *testSnippetStore {
	t.Helper()
	return &testSnippetStore{snippets: make(map[string]*Snippet)}
}

func (s *testSnippetStore) SaveSnippet(id, name, description, code, funcName string, tags []string) error {
	if id == "" {
		return fmt.Errorf("id required")
	}
	if code == "" {
		return fmt.Errorf("code required")
	}
	if funcName == "" {
		funcName = "Run"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snippets[id] = &Snippet{ID: id, Name: name, Description: description, Code: code, FuncName: funcName, Tags: tags}
	return nil
}

func (s *testSnippetStore) GetSnippet(id string) (*Snippet, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sn, ok := s.snippets[id]
	return sn, ok
}

func (s *testSnippetStore) GetSnippetByName(name string) (*Snippet, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	lower := strings.ToLower(name)
	for _, sn := range s.snippets {
		if strings.ToLower(sn.Name) == lower {
			return sn, true
		}
	}
	return nil, false
}

func (s *testSnippetStore) DeleteSnippet(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.snippets, id)
	return nil
}

func (s *testSnippetStore) SearchSnippets(query string) []*Snippet {
	s.mu.RLock()
	defer s.mu.RUnlock()
	lower := strings.ToLower(query)
	var results []*Snippet
	for _, sn := range s.snippets {
		if query == "" || strings.Contains(strings.ToLower(sn.Name), lower) ||
			strings.Contains(strings.ToLower(sn.Description), lower) {
			results = append(results, sn)
		}
	}
	return results
}

func (s *testSnippetStore) RecordSnippetUse(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sn, ok := s.snippets[id]; ok {
		sn.UseCount++
	}
}

func (s *testSnippetStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.snippets)
}

// ============ GoSnippetTool 基本属性 ============

func TestGoSnippetTool_Name(t *testing.T) {
	tool := NewGoSnippetTool(newTestStore(t))
	if tool.Name() != "go_snippet" {
		t.Errorf("Name = %q, want %q", tool.Name(), "go_snippet")
	}
}

func TestGoSnippetTool_Description(t *testing.T) {
	tool := NewGoSnippetTool(newTestStore(t))
	desc := tool.Description()
	for _, kw := range []string{"save", "list", "run", "delete"} {
		if !strings.Contains(desc, kw) {
			t.Errorf("Description should mention %q", kw)
		}
	}
}

func TestGoSnippetTool_Parameters(t *testing.T) {
	tool := NewGoSnippetTool(newTestStore(t))
	params := tool.Parameters()
	if params["type"] != "object" {
		t.Errorf("Parameters type = %v, want object", params["type"])
	}
}

// ============ Save ============

func TestGoSnippetTool_Save(t *testing.T) {
	store := newTestStore(t)
	tool := NewGoSnippetTool(store)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"action":      "save",
		"name":        "My Parser",
		"code":        "package main\nfunc Run(input map[string]interface{}) string { return \"ok\" }",
		"description": "a parser",
		"tags":        "csv, data",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "Snippet saved") {
		t.Errorf("result = %q, expected saved confirmation", result)
	}

	// 验证已存储
	if store.Len() != 1 {
		t.Errorf("store.Len() = %d, want 1", store.Len())
	}
}

func TestGoSnippetTool_Save_MissingFields(t *testing.T) {
	tool := NewGoSnippetTool(newTestStore(t))

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"action": "save",
		"name":   "test",
		// missing code
	})
	if err == nil {
		t.Error("expected error for missing code")
	}

	_, err = tool.Execute(context.Background(), map[string]interface{}{
		"action": "save",
		"code":   "x",
		// missing name
	})
	if err == nil {
		t.Error("expected error for missing name")
	}
}

// ============ List ============

func TestGoSnippetTool_List(t *testing.T) {
	store := newTestStore(t)
	store.SaveSnippet("a", "Alpha", "first", "x", "Run", nil)
	store.SaveSnippet("b", "Beta", "second", "x", "Run", nil)

	tool := NewGoSnippetTool(store)
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"action": "list",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "Alpha") || !strings.Contains(result, "Beta") {
		t.Errorf("result should list both snippets, got %q", result)
	}
}

func TestGoSnippetTool_List_WithQuery(t *testing.T) {
	store := newTestStore(t)
	store.SaveSnippet("a", "CSV Parser", "", "x", "Run", nil)
	store.SaveSnippet("b", "JSON Diff", "", "x", "Run", nil)

	tool := NewGoSnippetTool(store)
	result, _ := tool.Execute(context.Background(), map[string]interface{}{
		"action": "list",
		"query":  "csv",
	})
	if !strings.Contains(result, "CSV Parser") {
		t.Error("should find CSV Parser")
	}
	if strings.Contains(result, "JSON Diff") {
		t.Error("should not find JSON Diff")
	}
}

func TestGoSnippetTool_List_Empty(t *testing.T) {
	tool := NewGoSnippetTool(newTestStore(t))
	result, _ := tool.Execute(context.Background(), map[string]interface{}{
		"action": "list",
	})
	if !strings.Contains(result, "No saved snippets") {
		t.Errorf("result = %q, want 'No saved snippets'", result)
	}
}

// ============ Run ============

func TestGoSnippetTool_Run_ByName(t *testing.T) {
	store := newTestStore(t)
	store.SaveSnippet("calc", "Calculator", "",
		"package main\n\nimport \"fmt\"\n\nfunc Run(input map[string]interface{}) string {\n\treturn fmt.Sprintf(\"result: %v\", 42)\n}",
		"Run", nil)

	tool := NewGoSnippetTool(store)
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"action": "run",
		"name":   "Calculator",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "result: 42") {
		t.Errorf("result = %q, expected 'result: 42'", result)
	}
	if !strings.Contains(result, "[Snippet: Calculator]") {
		t.Errorf("result should contain snippet label, got %q", result)
	}

	// 验证使用计数增加
	sn, _ := store.GetSnippet("calc")
	if sn.UseCount != 1 {
		t.Errorf("UseCount = %d, want 1", sn.UseCount)
	}
}

func TestGoSnippetTool_Run_ByID(t *testing.T) {
	store := newTestStore(t)
	store.SaveSnippet("my-id", "By ID", "",
		"package main\nfunc Run(input map[string]interface{}) string { return \"by-id\" }",
		"Run", nil)

	tool := NewGoSnippetTool(store)
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"action": "run",
		"id":     "my-id",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "by-id") {
		t.Errorf("result = %q, expected 'by-id'", result)
	}
}

func TestGoSnippetTool_Run_WithInput(t *testing.T) {
	store := newTestStore(t)
	store.SaveSnippet("greeter", "Greeter", "",
		"package main\n\nimport \"fmt\"\n\nfunc Run(input map[string]interface{}) string {\n\tname, _ := input[\"name\"].(string)\n\treturn fmt.Sprintf(\"Hello, %s!\", name)\n}",
		"Run", nil)

	tool := NewGoSnippetTool(store)
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"action": "run",
		"name":   "Greeter",
		"input":  map[string]interface{}{"name": "World"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "Hello, World!") {
		t.Errorf("result = %q, expected 'Hello, World!'", result)
	}
}

func TestGoSnippetTool_Run_NotFound(t *testing.T) {
	tool := NewGoSnippetTool(newTestStore(t))
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"action": "run",
		"name":   "nonexistent",
	})
	if err == nil {
		t.Error("expected error for nonexistent snippet")
	}
}

// ============ Delete ============

func TestGoSnippetTool_Delete(t *testing.T) {
	store := newTestStore(t)
	store.SaveSnippet("d1", "To Delete", "", "x", "Run", nil)

	tool := NewGoSnippetTool(store)
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"action": "delete",
		"name":   "To Delete",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "deleted") {
		t.Errorf("result = %q, expected deletion confirmation", result)
	}
	if store.Len() != 0 {
		t.Error("store should be empty after delete")
	}
}

func TestGoSnippetTool_Delete_NotFound(t *testing.T) {
	tool := NewGoSnippetTool(newTestStore(t))
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"action": "delete",
		"name":   "nonexistent",
	})
	if err == nil {
		t.Error("expected error for nonexistent snippet")
	}
}

// ============ Unknown action ============

func TestGoSnippetTool_UnknownAction(t *testing.T) {
	tool := NewGoSnippetTool(newTestStore(t))
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"action": "unknown",
	})
	if err == nil {
		t.Error("expected error for unknown action")
	}
}

// ============ slugify ============

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"CSV Parser", "csv-parser"},
		{"json_diff", "json-diff"},
		{"Date Calc 2.0", "date-calc-2-0"},
		{"  spaces  ", "spaces"},
		{"UPPER", "upper"},
		{"a--b", "a-b"},
	}
	for _, tt := range tests {
		got := slugify(tt.input)
		if got != tt.want {
			t.Errorf("slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
