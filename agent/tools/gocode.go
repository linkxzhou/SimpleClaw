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
func goExec(ctx context.Context, code, funcName, prefix string, timeout time.Duration, maxOutput int, args ...interface{}) (string, error) {
	// 确保包含 package 声明
	if !strings.HasPrefix(strings.TrimSpace(code), "package ") {
		code = "package main\n\n" + code
	}

	// 编译
	traceID := fmt.Sprintf("gocode-%d", time.Now().UnixNano())
	program, err := goscript.Compile(traceID, "main", code)
	if err != nil {
		return fmt.Sprintf("Compilation error:\n%s", err.Error()), nil
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
			errMsg := fmt.Sprintf("Runtime error:\n%s", res.err.Error())
			if prefix != "" {
				errMsg = prefix + " " + errMsg
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
