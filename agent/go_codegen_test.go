package agent

import (
	"strings"
	"testing"
)

func TestGoCodegenAdvisor_BuildPrompt(t *testing.T) {
	advisor := &GoCodegenAdvisor{}
	prompt := advisor.BuildPrompt()

	if prompt == "" {
		t.Fatal("BuildPrompt should not return empty string")
	}

	// 应包含策略标题
	if !strings.Contains(prompt, "Go Code Generation Strategy") {
		t.Error("prompt should contain 'Go Code Generation Strategy'")
	}

	// 应包含可用包列表
	for _, pkg := range []string{"fmt", "math", "strings", "time"} {
		if !strings.Contains(prompt, pkg) {
			t.Errorf("prompt should contain package %q", pkg)
		}
	}

	// 应包含何时写代码的场景
	for _, scenario := range []string{"Math", "Date", "JSON", "Regular expression", "Encoding"} {
		if !strings.Contains(prompt, scenario) {
			t.Errorf("prompt should mention scenario %q", scenario)
		}
	}

	// 应包含何时不写代码的场景
	if !strings.Contains(prompt, "NOT to Write Code") {
		t.Error("prompt should contain 'When NOT to Write Code' section")
	}

	// 应包含代码模板
	if !strings.Contains(prompt, "Code Templates") {
		t.Error("prompt should contain 'Code Templates' section")
	}

	// 应包含错误处理指南
	if !strings.Contains(prompt, "Error Handling") {
		t.Error("prompt should contain 'Error Handling' section")
	}
}

func TestGetAvailablePackages(t *testing.T) {
	pkgs := getAvailablePackages()
	if pkgs == "" {
		t.Fatal("getAvailablePackages should not return empty")
	}
	// 至少包含基础包
	for _, pkg := range []string{"fmt", "math", "strings", "time"} {
		if !strings.Contains(pkgs, pkg) {
			t.Errorf("available packages should contain %q, got %q", pkg, pkgs)
		}
	}
}

func TestGoCodegenAdvisor_PromptContainsGoRunTemplate(t *testing.T) {
	advisor := &GoCodegenAdvisor{}
	prompt := advisor.BuildPrompt()

	if !strings.Contains(prompt, "go_run") {
		t.Error("prompt should reference go_run tool")
	}
	if !strings.Contains(prompt, "go_agent") {
		t.Error("prompt should reference go_agent tool")
	}
	if !strings.Contains(prompt, "func main()") {
		t.Error("prompt should contain go_run template with func main()")
	}
	if !strings.Contains(prompt, "func Run(input map[string]interface{})") {
		t.Error("prompt should contain go_agent template with func Run()")
	}
}
