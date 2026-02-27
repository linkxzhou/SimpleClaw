package tools

import (
	"context"
	"strings"
	"testing"
	"time"
)

// ============ availablePackages ============

func TestAvailablePackages(t *testing.T) {
	if availablePackages == "" {
		t.Fatal("availablePackages should not be empty")
	}
	// 至少包含 pkgs.ImportPkgs 中已启用的 4 个包
	for _, pkg := range []string{"fmt", "math", "strings", "time"} {
		if !strings.Contains(availablePackages, pkg) {
			t.Errorf("availablePackages = %q, missing %q", availablePackages, pkg)
		}
	}
}

func TestGoRunTool_DescriptionContainsPackages(t *testing.T) {
	tool := NewGoRunTool()
	desc := tool.Description()
	if !strings.Contains(desc, "fmt") || !strings.Contains(desc, "strings") {
		t.Errorf("Description should contain available packages, got %q", desc)
	}
}

func TestGoAgentTool_DescriptionContainsPackages(t *testing.T) {
	tool := NewGoAgentTool()
	desc := tool.Description()
	if !strings.Contains(desc, "fmt") || !strings.Contains(desc, "strings") {
		t.Errorf("Description should contain available packages, got %q", desc)
	}
}

// ============ GoRunTool 基本属性 ============

func TestGoRunTool_Name(t *testing.T) {
	tool := NewGoRunTool()
	if tool.Name() != "go_run" {
		t.Errorf("Name = %q, want %q", tool.Name(), "go_run")
	}
}

func TestGoRunTool_Description(t *testing.T) {
	tool := NewGoRunTool()
	if tool.Description() == "" {
		t.Error("Description should not be empty")
	}
}

func TestGoRunTool_Parameters(t *testing.T) {
	tool := NewGoRunTool()
	params := tool.Parameters()
	if params["type"] != "object" {
		t.Errorf("Parameters type = %v, want object", params["type"])
	}
}

func TestGoRunTool_Defaults(t *testing.T) {
	tool := NewGoRunTool()
	if tool.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", tool.Timeout)
	}
	if tool.MaxOutput != 50000 {
		t.Errorf("MaxOutput = %d, want 50000", tool.MaxOutput)
	}
}

// ============ GoRunTool 代码执行 ============

func TestGoRunTool_EmptyCode(t *testing.T) {
	tool := NewGoRunTool()
	_, err := tool.Execute(context.Background(), map[string]interface{}{"code": ""})
	if err == nil {
		t.Fatal("expected error for empty code")
	}
}

func TestGoRunTool_SimpleReturnWithCustomFunc(t *testing.T) {
	tool := NewGoRunTool()
	code := `
package main

func compute() string {
	return "hello from gorun"
}
`
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"code":     code,
		"function": "compute",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "hello from gorun") {
		t.Errorf("result = %q, expected to contain 'hello from gorun'", result)
	}
}

func TestGoRunTool_MainNoReturn(t *testing.T) {
	tool := NewGoRunTool()
	code := `
package main

func main() {
}
`
	result, err := tool.Execute(context.Background(), map[string]interface{}{"code": code})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "no output") {
		t.Errorf("result = %q, expected 'no output' message", result)
	}
}

func TestGoRunTool_AutoPackage(t *testing.T) {
	tool := NewGoRunTool()
	code := `
func compute() string {
	return "auto-package"
}
`
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"code":     code,
		"function": "compute",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "auto-package") {
		t.Errorf("result = %q, expected to contain 'auto-package'", result)
	}
}

func TestGoRunTool_CustomFunction(t *testing.T) {
	tool := NewGoRunTool()
	code := `
package main

func compute() string {
	return "computed"
}
`
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"code":     code,
		"function": "compute",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "computed") {
		t.Errorf("result = %q, expected to contain 'computed'", result)
	}
}

func TestGoRunTool_CompileError(t *testing.T) {
	tool := NewGoRunTool()
	code := `
package main

func main() {
	invalid syntax here
}
`
	result, err := tool.Execute(context.Background(), map[string]interface{}{"code": code})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(strings.ToLower(result), "error") {
		t.Errorf("result = %q, expected compilation error", result)
	}
}

func TestGoRunTool_MainWithReturnValueCompileError(t *testing.T) {
	tool := NewGoRunTool()
	code := `
package main

func main() string {
	return "should fail"
}
`
	result, err := tool.Execute(context.Background(), map[string]interface{}{"code": code})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(strings.ToLower(result), "error") {
		t.Errorf("result = %q, expected compile error for main with return", result)
	}
}

func TestGoRunTool_Timeout(t *testing.T) {
	tool := NewGoRunTool()
	tool.Timeout = 500 * time.Millisecond

	code := `
package main

import "time"

func main() {
	time.Sleep(10 * time.Second)
}
`
	result, err := tool.Execute(context.Background(), map[string]interface{}{"code": code})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "timed out") && !strings.Contains(result, "timeout") {
		t.Errorf("result = %q, expected timeout message", result)
	}
}

func TestGoRunTool_ContextCancelled(t *testing.T) {
	tool := NewGoRunTool()
	ctx, cancel := context.WithCancel(context.Background())

	code := `
package main

import "time"

func main() {
	time.Sleep(10 * time.Second)
}
`
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	result, err := tool.Execute(ctx, map[string]interface{}{"code": code})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "cancelled") && !strings.Contains(result, "timed out") {
		t.Errorf("result = %q, expected cancelled or timed out", result)
	}
}

func TestGoRunTool_StringImport(t *testing.T) {
	tool := NewGoRunTool()
	code := `
package main

import "strings"

func run() string {
	return strings.ToUpper("hello")
}
`
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"code":     code,
		"function": "run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "HELLO") {
		t.Errorf("result = %q, expected to contain 'HELLO'", result)
	}
}

func TestGoRunTool_MathImport(t *testing.T) {
	tool := NewGoRunTool()
	code := `
package main

import (
	"fmt"
	"math"
)

func calc() string {
	return fmt.Sprintf("%.2f", math.Sqrt(16))
}
`
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"code":     code,
		"function": "calc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "4.00") {
		t.Errorf("result = %q, expected to contain '4.00'", result)
	}
}

func TestGoRunTool_OutputTruncation(t *testing.T) {
	tool := NewGoRunTool()
	tool.MaxOutput = 100

	code := `
package main

import "strings"

func run() string {
	return strings.Repeat("x", 500)
}
`
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"code":     code,
		"function": "run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "truncated") {
		t.Errorf("result should be truncated, got len=%d", len(result))
	}
}

func TestGoRunTool_DefaultFunctionIsMain(t *testing.T) {
	tool := NewGoRunTool()
	code := `
package main

func main() {
}
`
	result, err := tool.Execute(context.Background(), map[string]interface{}{"code": code})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(strings.ToLower(result), "error") {
		t.Errorf("result = %q, should not be an error for valid main", result)
	}
}

func TestGoRunTool_MultipleFunctions(t *testing.T) {
	tool := NewGoRunTool()
	code := `
package main

func helper() string {
	return "helper"
}

func run() string {
	return "run:" + helper()
}
`
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"code":     code,
		"function": "run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "run:helper") {
		t.Errorf("result = %q, expected 'run:helper'", result)
	}
}

// ============ GoAgentTool 基本属性 ============

func TestGoAgentTool_Name(t *testing.T) {
	tool := NewGoAgentTool()
	if tool.Name() != "go_agent" {
		t.Errorf("Name = %q, want %q", tool.Name(), "go_agent")
	}
}

func TestGoAgentTool_Description(t *testing.T) {
	tool := NewGoAgentTool()
	if tool.Description() == "" {
		t.Error("Description should not be empty")
	}
}

func TestGoAgentTool_Parameters(t *testing.T) {
	tool := NewGoAgentTool()
	params := tool.Parameters()
	if params["type"] != "object" {
		t.Errorf("Parameters type = %v, want object", params["type"])
	}
}

func TestGoAgentTool_Defaults(t *testing.T) {
	tool := NewGoAgentTool()
	if tool.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", tool.Timeout)
	}
	if tool.MaxOutput != 50000 {
		t.Errorf("MaxOutput = %d, want 50000", tool.MaxOutput)
	}
}

// ============ GoAgentTool 代码执行 ============

func TestGoAgentTool_EmptyCode(t *testing.T) {
	tool := NewGoAgentTool()
	_, err := tool.Execute(context.Background(), map[string]interface{}{"code": ""})
	if err == nil {
		t.Fatal("expected error for empty code")
	}
}

func TestGoAgentTool_SimpleRun(t *testing.T) {
	tool := NewGoAgentTool()
	code := `
package main

func Run(input map[string]interface{}) string {
	return "hello from go agent"
}
`
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"code": code,
		"task": "test task",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "hello from go agent") {
		t.Errorf("result = %q, expected to contain 'hello from go agent'", result)
	}
	if !strings.Contains(result, "Go Agent: test task") {
		t.Errorf("result = %q, expected to contain task label", result)
	}
}

func TestGoAgentTool_WithInput(t *testing.T) {
	tool := NewGoAgentTool()
	code := `
package main

import "fmt"

func Run(input map[string]interface{}) string {
	name, _ := input["name"].(string)
	return fmt.Sprintf("hello %s", name)
}
`
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"code":  code,
		"input": map[string]interface{}{"name": "world"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "hello world") {
		t.Errorf("result = %q, expected to contain 'hello world'", result)
	}
}

func TestGoAgentTool_InputAsString(t *testing.T) {
	tool := NewGoAgentTool()
	code := `
package main

import "fmt"

func Run(input map[string]interface{}) string {
	val, _ := input["key"].(string)
	return fmt.Sprintf("got:%s", val)
}
`
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"code":  code,
		"input": `{"key": "value"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "got:value") {
		t.Errorf("result = %q, expected to contain 'got:value'", result)
	}
}

func TestGoAgentTool_NoRunFunction(t *testing.T) {
	tool := NewGoAgentTool()
	code := `
package main

func Compute() string {
	return "no run here"
}
`
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"code": code,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "must define a 'Run' function") {
		t.Errorf("result = %q, expected missing Run function error", result)
	}
}

func TestGoAgentTool_AutoPackage(t *testing.T) {
	tool := NewGoAgentTool()
	code := `
func Run(input map[string]interface{}) string {
	return "auto-pkg"
}
`
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"code": code,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "auto-pkg") {
		t.Errorf("result = %q, expected to contain 'auto-pkg'", result)
	}
}

func TestGoAgentTool_CompileError(t *testing.T) {
	tool := NewGoAgentTool()
	code := `
package main

func Run(input map[string]interface{}) string {
	invalid syntax
}
`
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"code": code,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(strings.ToLower(result), "error") {
		t.Errorf("result = %q, expected compilation error", result)
	}
}

func TestGoAgentTool_DefaultTask(t *testing.T) {
	tool := NewGoAgentTool()
	code := `
package main

func Run(input map[string]interface{}) string {
	return "ok"
}
`
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"code": code,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Go Agent: dynamic go agent") {
		t.Errorf("result = %q, expected default task label", result)
	}
}

func TestGoAgentTool_Timeout(t *testing.T) {
	tool := NewGoAgentTool()
	tool.Timeout = 500 * time.Millisecond

	code := `
package main

import "time"

func Run(input map[string]interface{}) string {
	time.Sleep(10 * time.Second)
	return "done"
}
`
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"code": code,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "timed out") {
		t.Errorf("result = %q, expected timeout message", result)
	}
}

func TestGoAgentTool_ContextCancelled(t *testing.T) {
	tool := NewGoAgentTool()
	ctx, cancel := context.WithCancel(context.Background())

	code := `
package main

import "time"

func Run(input map[string]interface{}) string {
	time.Sleep(10 * time.Second)
	return "done"
}
`
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"code": code,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "cancelled") && !strings.Contains(result, "timed out") {
		t.Errorf("result = %q, expected cancelled or timed out", result)
	}
}

func TestGoAgentTool_NilInput(t *testing.T) {
	tool := NewGoAgentTool()
	code := `
package main

import "fmt"

func Run(input map[string]interface{}) string {
	return fmt.Sprintf("len=%d", len(input))
}
`
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"code": code,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "len=0") {
		t.Errorf("result = %q, expected 'len=0'", result)
	}
}

func TestGoAgentTool_ParseError(t *testing.T) {
	tool := NewGoAgentTool()
	code := `
package main

func Run(input map[string]interface{}) string {
`
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"code": code,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(strings.ToLower(result), "error") {
		t.Errorf("result = %q, expected parse error", result)
	}
}

func TestGoAgentTool_OutputTruncation(t *testing.T) {
	tool := NewGoAgentTool()
	tool.MaxOutput = 100

	code := `
package main

import "strings"

func Run(input map[string]interface{}) string {
	return strings.Repeat("x", 500)
}
`
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"code": code,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "truncated") {
		t.Errorf("result should be truncated, got len=%d", len(result))
	}
}

func TestGoAgentTool_StringsImport(t *testing.T) {
	tool := NewGoAgentTool()
	code := `
package main

import "strings"

func Run(input map[string]interface{}) string {
	return strings.Join([]string{"a", "b", "c"}, "-")
}
`
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"code": code,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "a-b-c") {
		t.Errorf("result = %q, expected to contain 'a-b-c'", result)
	}
}

func TestGoAgentTool_InputMultipleKeys(t *testing.T) {
	tool := NewGoAgentTool()
	code := `
package main

import "fmt"

func Run(input map[string]interface{}) string {
	a, _ := input["a"].(string)
	b, _ := input["b"].(string)
	return fmt.Sprintf("%s+%s", a, b)
}
`
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"code":  code,
		"input": map[string]interface{}{"a": "x", "b": "y"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "x+y") {
		t.Errorf("result = %q, expected 'x+y'", result)
	}
}
