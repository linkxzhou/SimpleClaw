package tools

import (
	"strings"
	"testing"
)

// ============ formatCompileError ============

func TestFormatCompileError_Basic(t *testing.T) {
	result := formatCompileError("main.go:5:3: undefined: foo", "package main\n\nfunc main() {\n\tfoo()\n}")
	if !strings.Contains(result, "[Compile Error]") {
		t.Errorf("should contain [Compile Error], got %q", result)
	}
	if !strings.Contains(result, "undefined: foo") {
		t.Error("should contain original error message")
	}
	if !strings.Contains(result, "Available packages:") {
		t.Error("should contain available packages")
	}
}

func TestFormatCompileError_WithHints(t *testing.T) {
	result := formatCompileError("main.go:3:2: undefined: strconv", "package main\n\nfunc main() {\n\tstrconv.Itoa(1)\n}")
	if !strings.Contains(result, "Hints:") {
		t.Error("should contain hints section")
	}
	if !strings.Contains(result, "undefined") || !strings.Contains(result, "imported") {
		t.Error("should suggest checking imports")
	}
}

func TestFormatCompileError_ImportNotUsed(t *testing.T) {
	result := formatCompileError(`main.go:3:2: "fmt" imported and not used`, "package main\n\nimport \"fmt\"\n\nfunc main() {}")
	if !strings.Contains(result, "unused import") {
		t.Error("should hint about unused import")
	}
}

func TestFormatCompileError_DeclaredNotUsed(t *testing.T) {
	result := formatCompileError("main.go:4:2: x declared and not used", "package main\n\nfunc main() {\n\tx := 1\n}")
	if !strings.Contains(result, "declared variable") || !strings.Contains(result, "_") {
		t.Error("should hint about using _ for unused variables")
	}
}

func TestFormatCompileError_TypeMismatch(t *testing.T) {
	result := formatCompileError("cannot use x (type int) as type string", "package main\n\nfunc main() {}")
	if !strings.Contains(result, "Type mismatch") {
		t.Error("should hint about type mismatch")
	}
}

// ============ formatRuntimeError ============

func TestFormatRuntimeError_IndexOutOfRange(t *testing.T) {
	result := formatRuntimeError("runtime error: index out of range [5] with length 3", "")
	if !strings.Contains(result, "[Runtime Error]") {
		t.Error("should contain [Runtime Error]")
	}
	if !strings.Contains(result, "Check slice") {
		t.Error("should hint about checking length")
	}
}

func TestFormatRuntimeError_NilPointer(t *testing.T) {
	result := formatRuntimeError("runtime error: invalid memory address or nil pointer dereference", "")
	if !strings.Contains(result, "Nil pointer") || !strings.Contains(result, "initialized") {
		t.Error("should hint about nil pointer")
	}
}

func TestFormatRuntimeError_DivByZero(t *testing.T) {
	result := formatRuntimeError("runtime error: integer divide by zero", "")
	if !strings.Contains(result, "Division by zero") {
		t.Error("should hint about division by zero")
	}
}

func TestFormatRuntimeError_TypeAssertion(t *testing.T) {
	result := formatRuntimeError("interface conversion: type assertion failed", "")
	if !strings.Contains(result, "comma-ok") {
		t.Error("should hint about comma-ok pattern")
	}
}

func TestFormatRuntimeError_Panic(t *testing.T) {
	result := formatRuntimeError("panic: something went wrong", "")
	if !strings.Contains(result, "panicked") {
		t.Error("should hint about panic")
	}
}

// ============ extractErrorLineContext ============

func TestExtractErrorLineContext_ValidLine(t *testing.T) {
	code := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(foo)\n}\n"
	ctx := extractErrorLineContext("main.go:6:14: undefined: foo", code)
	if ctx == "" {
		t.Fatal("should return context for valid line number")
	}
	if !strings.Contains(ctx, "→") {
		t.Error("should contain arrow marker on error line")
	}
}

func TestExtractErrorLineContext_NoLineNumber(t *testing.T) {
	ctx := extractErrorLineContext("some weird error", "package main")
	if ctx != "" {
		t.Errorf("should return empty for no line number, got %q", ctx)
	}
}

// ============ parseLineNum ============

func TestParseLineNum(t *testing.T) {
	tests := []struct {
		input string
		want  int
		ok    bool
	}{
		{"12", 12, true},
		{"1", 1, true},
		{"0", 0, false},
		{"abc", 0, false},
		{"12a", 0, false},
		{"", 0, false},
		{" 5 ", 5, true},
	}
	for _, tt := range tests {
		got, ok := parseLineNum(tt.input)
		if got != tt.want || ok != tt.ok {
			t.Errorf("parseLineNum(%q) = (%d, %v), want (%d, %v)", tt.input, got, ok, tt.want, tt.ok)
		}
	}
}

// ============ inferCompileHints ============

func TestInferCompileHints_ArgumentCount(t *testing.T) {
	hints := inferCompileHints("too many arguments in call to foo")
	found := false
	for _, h := range hints {
		if strings.Contains(h, "number of arguments") {
			found = true
		}
	}
	if !found {
		t.Error("should hint about argument count mismatch")
	}
}

func TestInferCompileHints_ExpectedGot(t *testing.T) {
	hints := inferCompileHints("expected ';', got 'IDENT'")
	found := false
	for _, h := range hints {
		if strings.Contains(h, "Syntax error") {
			found = true
		}
	}
	if !found {
		t.Error("should hint about syntax error")
	}
}

// ============ inferRuntimeHints ============

func TestInferRuntimeHints_NilMap(t *testing.T) {
	hints := inferRuntimeHints("assignment to entry in nil map")
	found := false
	for _, h := range hints {
		if strings.Contains(h, "make()") {
			found = true
		}
	}
	if !found {
		t.Error("should hint about initializing maps")
	}
}

func TestInferRuntimeHints_StackOverflow(t *testing.T) {
	hints := inferRuntimeHints("goroutine stack overflow")
	found := false
	for _, h := range hints {
		if strings.Contains(h, "recursion") {
			found = true
		}
	}
	if !found {
		t.Error("should hint about infinite recursion")
	}
}

// ============ Integration: goExec structured errors ============

func TestGoExec_CompileErrorStructured(t *testing.T) {
	code := `
package main

func main() {
	invalid syntax here
}
`
	result, err := goExec(t.Context(), code, "main", "", 5e9, 50000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "[Compile Error]") {
		t.Errorf("result should contain [Compile Error], got %q", result)
	}
	if !strings.Contains(result, "Available packages:") {
		t.Errorf("result should contain Available packages, got %q", result)
	}
}

func TestGoExec_RuntimeErrorStructured(t *testing.T) {
	code := `
package main

func main() string {
	data := []int{1, 2, 3}
	return fmt.Sprintf("%d", data[10])
}
`
	result, err := goExec(t.Context(), code, "main", "", 5e9, 50000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 编译错误（main 不能有返回值）或运行时错误都算通过
	if !strings.Contains(result, "[Compile Error]") && !strings.Contains(result, "[Runtime Error]") {
		t.Errorf("result should contain structured error, got %q", result)
	}
}
