package tools

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// dummyTool 测试用的简单工具
type dummyTool struct {
	name   string
	result string
	err    error
}

func (d *dummyTool) Name() string                    { return d.name }
func (d *dummyTool) Description() string             { return "dummy tool for testing" }
func (d *dummyTool) Parameters() map[string]interface{} { return map[string]interface{}{"type": "object"} }
func (d *dummyTool) Execute(_ context.Context, _ map[string]interface{}) (string, error) {
	return d.result, d.err
}

// ---------- ToolToSchema ----------

func TestToolToSchema(t *testing.T) {
	tool := &dummyTool{name: "test_tool", result: "ok"}
	schema := ToolToSchema(tool)

	if schema.Type != "function" {
		t.Errorf("Type = %q, want %q", schema.Type, "function")
	}
	if schema.Function.Name != "test_tool" {
		t.Errorf("Function.Name = %q, want %q", schema.Function.Name, "test_tool")
	}
	if schema.Function.Description != "dummy tool for testing" {
		t.Errorf("Function.Description = %q, want %q", schema.Function.Description, "dummy tool for testing")
	}
	if schema.Function.Parameters == nil {
		t.Error("Function.Parameters should not be nil")
	}
}

// ---------- Registry ----------

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
	if r.Len() != 0 {
		t.Errorf("Len = %d, want 0", r.Len())
	}
}

func TestRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	tool := &dummyTool{name: "alpha"}
	r.Register(tool)

	got, ok := r.Get("alpha")
	if !ok {
		t.Fatal("Get returned false for registered tool")
	}
	if got.Name() != "alpha" {
		t.Errorf("Name = %q, want %q", got.Name(), "alpha")
	}
}

func TestGetNotFound(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("Get returned true for unregistered tool")
	}
}

func TestHas(t *testing.T) {
	r := NewRegistry()
	r.Register(&dummyTool{name: "alpha"})

	if !r.Has("alpha") {
		t.Error("Has returned false for registered tool")
	}
	if r.Has("beta") {
		t.Error("Has returned true for unregistered tool")
	}
}

func TestUnregister(t *testing.T) {
	r := NewRegistry()
	r.Register(&dummyTool{name: "alpha"})
	r.Unregister("alpha")

	if r.Has("alpha") {
		t.Error("tool should be unregistered")
	}
	if r.Len() != 0 {
		t.Errorf("Len = %d, want 0", r.Len())
	}
}

func TestUnregisterNonexistent(t *testing.T) {
	r := NewRegistry()
	r.Unregister("nonexistent") // 不应 panic
}

func TestLen(t *testing.T) {
	r := NewRegistry()
	r.Register(&dummyTool{name: "a"})
	r.Register(&dummyTool{name: "b"})
	r.Register(&dummyTool{name: "c"})

	if r.Len() != 3 {
		t.Errorf("Len = %d, want 3", r.Len())
	}
}

func TestNames(t *testing.T) {
	r := NewRegistry()
	r.Register(&dummyTool{name: "b"})
	r.Register(&dummyTool{name: "a"})

	names := r.Names()
	if len(names) != 2 {
		t.Fatalf("len(Names) = %d, want 2", len(names))
	}
	nameSet := map[string]bool{}
	for _, n := range names {
		nameSet[n] = true
	}
	if !nameSet["a"] || !nameSet["b"] {
		t.Errorf("Names = %v, want [a, b]", names)
	}
}

func TestGetDefinitions(t *testing.T) {
	r := NewRegistry()
	r.Register(&dummyTool{name: "x"})
	r.Register(&dummyTool{name: "y"})

	defs := r.GetDefinitions()
	if len(defs) != 2 {
		t.Fatalf("len(GetDefinitions) = %d, want 2", len(defs))
	}
	for _, d := range defs {
		if d.Type != "function" {
			t.Errorf("Type = %q, want %q", d.Type, "function")
		}
	}
}

func TestRegisterOverwrite(t *testing.T) {
	r := NewRegistry()
	r.Register(&dummyTool{name: "alpha", result: "v1"})
	r.Register(&dummyTool{name: "alpha", result: "v2"})

	if r.Len() != 1 {
		t.Errorf("Len = %d, want 1", r.Len())
	}
	got, _ := r.Get("alpha")
	result, _ := got.Execute(context.Background(), nil)
	if result != "v2" {
		t.Errorf("result = %q, want %q", result, "v2")
	}
}

func TestExecuteSuccess(t *testing.T) {
	r := NewRegistry()
	r.Register(&dummyTool{name: "alpha", result: "hello"})

	result := r.Execute(context.Background(), "alpha", nil)
	if result != "hello" {
		t.Errorf("result = %q, want %q", result, "hello")
	}
}

func TestExecuteNotFound(t *testing.T) {
	r := NewRegistry()
	result := r.Execute(context.Background(), "missing", nil)
	if result != "Error: Tool 'missing' not found" {
		t.Errorf("result = %q, unexpected", result)
	}
}

func TestExecuteError(t *testing.T) {
	r := NewRegistry()
	r.Register(&dummyTool{name: "fail", err: fmt.Errorf("boom")})

	result := r.Execute(context.Background(), "fail", nil)
	expected := "Error executing fail: boom"
	if result != expected {
		t.Errorf("result = %q, want %q", result, expected)
	}
}

func TestRegistryConcurrency(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("tool_%d", i)
			r.Register(&dummyTool{name: name, result: name})
			r.Has(name)
			r.Get(name)
			r.Names()
			r.GetDefinitions()
			r.Execute(context.Background(), name, nil)
		}(i)
	}
	wg.Wait()

	if r.Len() != 50 {
		t.Errorf("Len = %d, want 50", r.Len())
	}
}
