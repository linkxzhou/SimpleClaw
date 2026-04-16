package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleClaw/goscript"
	"github.com/linkxzhou/SimpleClaw/goscript/packages/tool/pkgs"
)

// availablePackages 从 pkgs.ImportPkgs 动态读取可用包列表并缓存。
var availablePackages string

func init() {
	names := make([]string, 0, len(pkgs.ImportPkgs))
	for pkg := range pkgs.ImportPkgs {
		names = append(names, pkg)
	}
	sort.Strings(names)
	availablePackages = strings.Join(names, ", ")
}

// ---------- 公共执行引擎 ----------

// goExecResult 保存 goscript 执行结果。
type goExecResult struct {
	value interface{}
	ctx   *goscript.ExecContext
	err   error
}

// goExec 编译并执行 Go 代码，返回格式化的结果字符串。
// prefix 为输出前缀（如 "[Go Agent: task]"），funcName 为调用函数名，
// args 为可选的函数参数。
// 编译/运行错误返回结构化报告（含错误类型、行号、修复建议），
// 帮助 Agent 在 ReAct 循环中自然修复代码。
func goExec(ctx context.Context, code, funcName, prefix string, timeout time.Duration, maxOutput int, args ...interface{}) (string, error) {
	// 确保包含 package 声明
	if !strings.HasPrefix(strings.TrimSpace(code), "package ") {
		code = "package main\n\n" + code
	}

	// 编译
	traceID := fmt.Sprintf("gocode-%d", time.Now().UnixNano())
	program, err := goscript.Compile(traceID, "main", code)
	if err != nil {
		return formatCompileError(err.Error(), code), nil
	}

	// 带超时执行
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	resultCh := make(chan goExecResult, 1)
	go func() {
		val, execCtx, err := program.RunWithContext(traceID, funcName, args...)
		resultCh <- goExecResult{value: val, ctx: execCtx, err: err}
	}()

	select {
	case res := <-resultCh:
		if res.err != nil {
			errMsg := formatRuntimeError(res.err.Error(), code)
			if prefix != "" {
				errMsg = prefix + "\n\n" + errMsg
			}
			return errMsg, nil
		}

		var parts []string
		if prefix != "" {
			parts = append(parts, prefix)
		}

		// 捕获打印输出
		if res.ctx != nil {
			output := res.ctx.Output()
			if output != "" {
				parts = append(parts, fmt.Sprintf("Output:\n%s", output))
			}
		}

		// 返回值
		if res.value != nil {
			parts = append(parts, fmt.Sprintf("Return: %v", res.value))
		} else if prefix != "" {
			// GoAgent 模式下显示 nil
			parts = append(parts, "Result: (nil)")
		}

		if len(parts) == 0 {
			return "(no output, no return value)", nil
		}

		result := strings.Join(parts, "\n\n")

		// 超长截断
		if maxOutput > 0 && len(result) > maxOutput {
			result = result[:maxOutput] + fmt.Sprintf("\n... (truncated, %d more chars)", len(result)-maxOutput)
		}

		return result, nil

	case <-time.After(timeout):
		msg := fmt.Sprintf("Error: execution timed out after %s", timeout)
		if prefix != "" {
			msg = prefix + " " + msg
		}
		return msg, nil
	case <-ctx.Done():
		msg := "Error: execution cancelled"
		if prefix != "" {
			msg = prefix + " " + msg
		}
		return msg, nil
	}
}

// ============ GoRunTool ============

// GoRunTool 使用 goscript SSA 解释器动态执行 Go 源代码。
// 这使 Agent 能够生成并立即运行 Go 代码，无需外部编译，
// 提供一个安全的沙盒环境，并可控地访问标准库包。
type GoRunTool struct {
	Timeout   time.Duration
	MaxOutput int
}

// NewGoRunTool 创建一个新的 GoRunTool。
func NewGoRunTool() *GoRunTool {
	return &GoRunTool{
		Timeout:   30 * time.Second,
		MaxOutput: 50000,
	}
}

func (t *GoRunTool) Name() string { return "go_run" }
func (t *GoRunTool) Description() string {
	return fmt.Sprintf(`Execute Go source code dynamically in a sandboxed interpreter.
The code is compiled to SSA and interpreted at runtime - no external compiler needed.
Available packages: %s.
The code must define the function specified by 'function' parameter (defaults to 'main').
The function should return a value (string recommended) as the result.`, availablePackages)
}

func (t *GoRunTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"code": map[string]interface{}{
				"type":        "string",
				"description": "要执行的 Go 源代码。必须是包含至少一个函数的完整 package。",
			},
			"function": map[string]interface{}{
				"type":        "string",
				"description": "要调用的函数名（默认: 'main'）。函数应无参数并返回一个值。",
			},
			"args": map[string]interface{}{
				"type":        "string",
				"description": "可选的 JSON 编码参数，传递给接受 map[string]interface{} 的函数。",
			},
		},
		"required": []string{"code"},
	}
}

func (t *GoRunTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	code, _ := params["code"].(string)
	if code == "" {
		return "", fmt.Errorf("code is required")
	}

	funcName, _ := params["function"].(string)
	if funcName == "" {
		funcName = "main"
	}

	return goExec(ctx, code, funcName, "", t.Timeout, t.MaxOutput)
}

// ============ GoAgentTool ============

// GoAgentTool 允许 Agent 生成 Go 代码作为动态子代理执行。
// 生成的代码可以定义自定义工具（包含 Name/Description/Execute），
// 并通过 goscript 即时编译和执行，实现 Go 语言的自举能力。
//
// 这是核心差异化能力：Agent 编写 Go 代码 → goscript 解释执行
// → 结果反馈到 Agent 循环。Agent 可以创建专用的计算逻辑、
// 数据处理管道或迷你代理——全部使用 Go 语言。
type GoAgentTool struct {
	Timeout   time.Duration
	MaxOutput int
}

// NewGoAgentTool 创建一个新的 GoAgentTool。
func NewGoAgentTool() *GoAgentTool {
	return &GoAgentTool{
		Timeout:   60 * time.Second,
		MaxOutput: 50000,
	}
}

func (t *GoAgentTool) Name() string { return "go_agent" }
func (t *GoAgentTool) Description() string {
	return fmt.Sprintf(`Create and execute a dynamic Go sub-agent for complex computation tasks.
Write Go code that defines a 'Run' function taking a map[string]interface{} input 
and returning a string result. The code runs in a sandboxed Go interpreter.

Use this when you need to:
- Perform complex data processing or transformation
- Run algorithms (sorting, searching, mathematical computation)
- Generate structured output from raw data
- Create reusable logic that can be parameterized via the input map

Available packages: %s.
The 'Run' function receives the 'input' parameter as its argument.

Example code:
  package main
  
  func Run(input map[string]interface{}) string {
      name, _ := input["name"].(string)
      return fmt.Sprintf("Hello, %%s!", name)
  }`, availablePackages)
}

func (t *GoAgentTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"code": map[string]interface{}{
				"type":        "string",
				"description": "定义 'Run(input map[string]interface{}) string' 函数的 Go 源代码。",
			},
			"input": map[string]interface{}{
				"type":        "object",
				"description": "传递给 Run 函数的输入数据，类型为 map[string]interface{}。",
			},
			"task": map[string]interface{}{
				"type":        "string",
				"description": "该 Go 代理执行任务的简要描述（用于日志记录）。",
			},
		},
		"required": []string{"code"},
	}
}

func (t *GoAgentTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	code, _ := params["code"].(string)
	if code == "" {
		return "", fmt.Errorf("code is required")
	}

	task, _ := params["task"].(string)
	if task == "" {
		task = "dynamic go agent"
	}

	// 解析输入参数
	input := make(map[string]interface{})
	if inputRaw, ok := params["input"]; ok && inputRaw != nil {
		switch v := inputRaw.(type) {
		case map[string]interface{}:
			input = v
		case string:
			_ = json.Unmarshal([]byte(v), &input)
		}
	}

	// 确保包含 package 声明（goExec 也会做，但这里提前处理以便 ParseFunctions）
	trimmed := strings.TrimSpace(code)
	if !strings.HasPrefix(trimmed, "package ") {
		code = "package main\n\n" + code
	}

	// 校验 Run 函数是否存在
	funcs, err := goscript.ParseFunctions(code, false)
	if err != nil {
		return fmt.Sprintf("Parse error:\n%s", err.Error()), nil
	}
	hasRun := false
	for _, fn := range funcs {
		if fn == "Run" {
			hasRun = true
			break
		}
	}
	if !hasRun {
		return "Error: code must define a 'Run' function. Example:\n\nfunc Run(input map[string]interface{}) string {\n    return \"result\"\n}", nil
	}

	prefix := fmt.Sprintf("[Go Agent: %s]", task)
	return goExec(ctx, code, "Run", prefix, t.Timeout, t.MaxOutput, input)
}

// ============ 结构化错误报告 ============

// formatCompileError 生成结构化编译错误报告。
// 解析错误消息中的行号，附加可用包列表和修复建议。
func formatCompileError(errMsg, code string) string {
	var sb strings.Builder
	sb.WriteString("[Compile Error]\n")
	sb.WriteString(errMsg)
	sb.WriteString("\n")

	// 提取行号相关上下文
	if lineCtx := extractErrorLineContext(errMsg, code); lineCtx != "" {
		sb.WriteString("\nSource context:\n")
		sb.WriteString(lineCtx)
		sb.WriteString("\n")
	}

	// 生成修复建议
	hints := inferCompileHints(errMsg)
	if len(hints) > 0 {
		sb.WriteString("\nHints:\n")
		for _, h := range hints {
			sb.WriteString("- ")
			sb.WriteString(h)
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\nAvailable packages: ")
	sb.WriteString(availablePackages)
	return sb.String()
}

// formatRuntimeError 生成结构化运行时错误报告。
func formatRuntimeError(errMsg, code string) string {
	var sb strings.Builder
	sb.WriteString("[Runtime Error]\n")
	sb.WriteString(errMsg)
	sb.WriteString("\n")

	hints := inferRuntimeHints(errMsg)
	if len(hints) > 0 {
		sb.WriteString("\nHints:\n")
		for _, h := range hints {
			sb.WriteString("- ")
			sb.WriteString(h)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// extractErrorLineContext 从编译错误中提取行号并返回源代码上下文。
// 错误格式通常为 "main.go:12:5: ..." 或 "12:5: ..."。
func extractErrorLineContext(errMsg, code string) string {
	// 匹配 "文件名:行号:" 或 "行号:" 格式
	idx := -1
	parts := strings.SplitN(errMsg, ":", 4)
	if len(parts) >= 3 {
		// 尝试第二段是否为行号（"main.go:12:..."）
		if n, ok := parseLineNum(parts[1]); ok {
			idx = n
		} else if n, ok := parseLineNum(parts[0]); ok {
			// 尝试第一段（"12:..."）
			idx = n
		}
	}
	if idx <= 0 {
		return ""
	}

	lines := strings.Split(code, "\n")
	start := idx - 2
	if start < 0 {
		start = 0
	}
	end := idx + 2
	if end > len(lines) {
		end = len(lines)
	}

	var sb strings.Builder
	for i := start; i < end; i++ {
		marker := "  "
		if i == idx-1 {
			marker = "→ "
		}
		sb.WriteString(fmt.Sprintf("%s%3d| %s\n", marker, i+1, lines[i]))
	}
	return sb.String()
}

// parseLineNum 解析字符串为行号。
func parseLineNum(s string) (int, bool) {
	s = strings.TrimSpace(s)
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	if n == 0 {
		return 0, false
	}
	return n, true
}

// inferCompileHints 根据编译错误模式生成修复建议。
func inferCompileHints(errMsg string) []string {
	lower := strings.ToLower(errMsg)
	var hints []string

	if strings.Contains(lower, "undefined") {
		if strings.Contains(lower, "cannot refer to unexported") {
			hints = append(hints, "You are trying to use an unexported (lowercase) identifier from another package.")
		} else {
			hints = append(hints, "An identifier is undefined. Check spelling, or it may be from a package that needs to be imported.")
			hints = append(hints, "Auto-import handles known packages. If the package is available, just use it directly.")
		}
	}
	if strings.Contains(lower, "imported and not used") {
		hints = append(hints, "Remove the unused import, or use the imported package in your code.")
	}
	if strings.Contains(lower, "cannot use") && strings.Contains(lower, "as type") {
		hints = append(hints, "Type mismatch. Use type assertion (val, ok := x.(Type)) or explicit conversion.")
	}
	if strings.Contains(lower, "too many arguments") || strings.Contains(lower, "not enough arguments") {
		hints = append(hints, "Check the function signature — the number of arguments does not match.")
	}
	if strings.Contains(lower, "declared and not used") {
		hints = append(hints, "Use the declared variable or replace it with _ to discard.")
	}
	if strings.Contains(lower, "expected") && strings.Contains(lower, "got") {
		hints = append(hints, "Syntax error. Check for missing braces, parentheses, or semicolons.")
	}
	if strings.Contains(lower, "main") && strings.Contains(lower, "return") {
		hints = append(hints, "func main() cannot have a return type in Go. Use a named function (e.g. func compute() string) instead.")
	}
	return hints
}

// inferRuntimeHints 根据运行时错误模式生成修复建议。
func inferRuntimeHints(errMsg string) []string {
	lower := strings.ToLower(errMsg)
	var hints []string

	if strings.Contains(lower, "index out of range") {
		hints = append(hints, "Check slice/array length before accessing by index.")
	}
	if strings.Contains(lower, "nil pointer") || strings.Contains(lower, "nil map") {
		hints = append(hints, "A nil value was dereferenced. Initialize maps with make() and check pointers before use.")
	}
	if strings.Contains(lower, "divide by zero") || strings.Contains(lower, "division by zero") {
		hints = append(hints, "Division by zero. Check the denominator before dividing.")
	}
	if strings.Contains(lower, "type assertion") {
		hints = append(hints, "Type assertion failed. Use the comma-ok pattern: val, ok := x.(Type)")
	}
	if strings.Contains(lower, "invalid memory address") {
		hints = append(hints, "Nil pointer dereference. Ensure the variable is properly initialized.")
	}
	if strings.Contains(lower, "stack overflow") || strings.Contains(lower, "goroutine") {
		hints = append(hints, "Possible infinite recursion or goroutine leak. Check your recursive calls and loop conditions.")
	}
	if strings.Contains(lower, "panic") {
		hints = append(hints, "The code panicked. Wrap risky operations with proper error checking.")
	}
	return hints
}
