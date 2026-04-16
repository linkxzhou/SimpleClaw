// GoCodegen Advisor：代码生成决策策略。
// 在 system prompt 中注入策略，引导 Agent 在合适场景主动使用 go_run/go_agent，
// 而非用自然语言硬算。同时提供可用包清单和代码模板。
package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/linkxzhou/SimpleClaw/goscript/packages/tool/pkgs"
)

// GoCodegenAdvisor 生成代码决策策略 prompt。
type GoCodegenAdvisor struct{}

// getAvailablePackages 从 goscript 注册表动态读取可用包名。
func getAvailablePackages() string {
	names := make([]string, 0, len(pkgs.ImportPkgs))
	for pkg := range pkgs.ImportPkgs {
		names = append(names, pkg)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// BuildPrompt 返回注入 system prompt 的代码生成策略段落。
func (a *GoCodegenAdvisor) BuildPrompt() string {
	packages := getAvailablePackages()

	return fmt.Sprintf(`## Go Code Generation Strategy

You have a built-in Go interpreter (goscript) that can compile and execute Go code instantly.
Use **go_run** for one-shot computations and **go_agent** when you need structured input via a Run(input) function.

### When to Write Code (PREFER code over natural language)

- **Math / arithmetic** involving more than 2 steps or floating-point precision
- **Date / time calculations** (business days, durations, timezone conversions)
- **Data sorting, filtering, grouping, or aggregation**
- **Regular expressions** or complex string transformations
- **JSON / XML / CSV parsing and restructuring**
- **Encoding / decoding** (base64, hex, URL encoding, hashing)
- **Algorithms** (search, sort, combinatorics, graph traversal)
- **Statistical calculations** (mean, median, standard deviation, percentiles)

### When NOT to Write Code

- Simple Q&A, chat, or explanations — just answer in natural language
- File read/write/edit — use read_file, write_file, edit_file tools
- Shell commands — use exec tool
- Web lookups — use web_search, web_fetch tools

### Available Packages

%s

Packages are auto-imported: just use them in code without explicit import statements.

### Code Templates

**go_run** — quick one-shot computation:
` + "```" + `go
package main

func main() string {
    // your computation here
    return result
}
` + "```" + `

**go_agent** — parameterized computation:
` + "```" + `go
package main

func Run(input map[string]interface{}) string {
    val, _ := input["key"].(string)
    // process val
    return result
}
` + "```" + `

### Error Handling

If your code fails to compile or panics at runtime, you will receive a structured error report.
Read the error carefully, fix the code, and retry. Common issues:
- Missing return type on main() — use a named function instead if you need a return value
- Type assertion failures — always use the comma-ok pattern: val, ok := x.(Type)
- Index out of range — check length before accessing slices/arrays`, packages)
}
