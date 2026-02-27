package goscript

// goscript 核心功能测试。
// 包含以下测试场景：
//   - TestAll：运行 testdata 目录下的所有测试用例，对比解释器与原生 Go 的执行结果
//   - TestImportProgram：测试跨包导入功能
//   - TestSelectSendAndRecv：测试 select 语句中的 channel 发送和接收
//   - BenchmarkFibonacci：斐波那契递归性能基准测试
//   - TestGetGlobal：测试获取全局变量功能
//   - TestRunFunction：测试函数调用和 Exports map 导出
//   - TestRunWithContextMissingFunction：测试调用不存在函数的错误处理

import (
	"go/ast"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleClaw/goscript/testdata"

	"golang.org/x/tools/go/ssa"
)

const testTraceID = "gotest"

// runTestCases 执行测试用例，将 goscript 解释器的运行结果与原生 Go 的运行结果进行对比。
// 如果 funcName 为空，则测试所有导出函数；否则只测试指定函数。
func runTestCases(t *testing.T, funcName string) {
	if funcName != "" {
		debugMode = true
	}

	_, filename, _, _ := runtime.Caller(0)
	testdataDir := filepath.Join(filepath.Dir(filename), "testdata")
	files, err := ioutil.ReadDir(testdataDir)
	if err != nil {
		t.Error(err)
		return
	}

	// 通过反射获取 testdata.TestSet 的方法集作为预期结果
	testSet := reflect.ValueOf(testdata.TestSet)
	for _, file := range files {
		if file.Name() == "main.go" {
			continue
		}

		source, err := ioutil.ReadFile(filepath.Join(testdataDir, file.Name()))
		if err != nil {
			t.Error(err)
			return
		}

		// 将测试代码中的方法接收器声明替换为普通函数声明
		src := strings.Replace(string(source), `func (__testSet)`, `func `, -1)
		program, err := Compile(testTraceID, "testSet", src)
		if err != nil {
			t.Error(err)
			continue
		}

		// 遍历 SSA 包中的所有导出函数并逐一测试
		ssaPkg := program.SSAPackage()
		for name, member := range ssaPkg.Members {
			if _, ok := member.(*ssa.Function); !ok {
				continue
			}
			if !ast.IsExported(name) || (funcName != "" && name != funcName) {
				continue
			}

			t.Logf("Testing: %s", name)
			result, err := program.Run("", name)
			if err != nil {
				t.Logf("Error running %s: %v", name, err)
				continue
			}

			// 调用原生 Go 函数获取预期结果并对比
			expected := testSet.MethodByName(name).Call(nil)[0].Interface()
			if !reflect.DeepEqual(result, expected) {
				t.Logf("FAIL %s: expected %#v, got %#v", name, expected, result)
			} else {
				t.Logf("PASS %s", name)
			}
		}
	}
}

// TestAll 运行所有测试用例。
func TestAll(t *testing.T) {
	runTestCases(t, "")
}

// TestImportProgram 测试将编译后的脚本包导入到其他脚本中。
func TestImportProgram(t *testing.T) {
	mainSource := `
package test
	
import "pkg1"
import "pkg2"

var A = "1"
func test() string {
	return A + pkg1.F() + pkg2.S
}
`

	pkg1Source := `
package pkg1
func F() string {
	return "hello"
}
`

	pkg2Source := `
package pkg2
const S = "world"
func F() string {
	return "world"
}
`

	pkg1, err := Compile(testTraceID, "pkg1", pkg1Source)
	if err != nil {
		t.Fatal(err)
	}

	pkg2, err := Compile(testTraceID, "pkg2", pkg2Source)
	if err != nil {
		t.Fatal(err)
	}

	mainProg, err := Compile(testTraceID, "main", mainSource, pkg1.SSAPackage(), pkg2.SSAPackage())
	if err != nil {
		t.Fatal(err)
	}

	result, err := mainProg.Run("", "test")
	if err != nil {
		t.Fatal(err)
	}

	expected := "1helloworld"
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

// TestSelectSendAndRecv 测试 select 语句中的 channel 发送和接收操作。
func TestSelectSendAndRecv(t *testing.T) {
	source := `
package main

func testSend() int {
	ch := make(chan int, 1)
	select {
	case ch <- 7:
	default:
	}
	return <-ch
}

func testRecv() int {
	ch := make(chan int, 1)
	ch <- 9
	select {
	case v := <-ch:
		return v
	default:
		return 0
	}
}
`
	result, err := Run("", source, "testSend")
	if err != nil {
		t.Fatal(err)
	}
	if result != 7 {
		t.Errorf("Expected 7, got %v", result)
	}

	result, err = Run("", source, "testRecv")
	if err != nil {
		t.Fatal(err)
	}
	if result != 9 {
		t.Errorf("Expected 9, got %v", result)
	}
}

// BenchmarkFibonacci 斐波那契递归的性能基准测试。
// 用于衡量解释器执行递归函数的性能。
func BenchmarkFibonacci(b *testing.B) {
	b.StopTimer()
	b.ReportAllocs()

	code := `
package test

func fib(i int) int {
	if i < 2 {
		return i
	}
	return fib(i - 1) + fib(i - 2)
}

func test(i int) int {
	return fib(i)
}
`

	program, err := Compile(testTraceID, "test", code)
	if err != nil {
		b.Fatal(err)
	}

	f, err := os.Create("prof.out")
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()

	_ = pprof.StartCPUProfile(f)
	defer pprof.StopCPUProfile()

	b.StartTimer()
	var result interface{}
	for i := 0; i < b.N; i++ {
		result, err = program.Run("", "test", 25)
	}
	b.Logf("Result: %v, Error: %v", result, err)
}

// TestGetGlobal 测试获取脚本中的全局变量。
func TestGetGlobal(t *testing.T) {
	source := `
package main

var exports = map[string]interface{}{
	"test": 1,
}
`
	program, err := Compile(testTraceID, "test", source)
	if err != nil {
		t.Fatal(err)
	}

	exports, err := program.GetGlobal("exports")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("exports: %#v", exports)
}

// TestRunFunction 测试带参数的函数调用和全局变量初始化。
func TestRunFunction(t *testing.T) {
	source := `
package main

func testFunction(req map[string]interface{}) string {
	for i := 0; i < 10; i++ {
		println("testFunction:", i)
	}
	println("req:", req)
	return "hello world"
}

func testFunction1() string {
	for i := 0; i < 10; i++ {
		println("testFunction:", i)
	}
	return "hello world"
}

var Exports = map[string]interface{}{
	"testFunction": testFunction,
	"testFunction1": testFunction1,
}

var test = testFunction(nil)
var test1 = testFunction1()
`

	program, err := Compile(testTraceID, "test", source)
	if err != nil {
		t.Fatal(err)
	}

	req := map[string]interface{}{"test": 1}
	result, err := program.Run("", "testFunction", req)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Result: %#v", result)
}

// TestRunWithContextMissingFunction 测试调用不存在的函数时的错误处理。
func TestRunWithContextMissingFunction(t *testing.T) {
	source := `
package main

func test() int {
	return 1
}
`
	program, err := Compile(testTraceID, "test", source)
	if err != nil {
		t.Fatal(err)
	}

	result, ctx, err := program.RunWithContext("", "missing")
	if err == nil {
		t.Fatal("expected error for missing function")
	}
	if result != nil {
		t.Fatalf("expected nil result, got %v", result)
	}
	if ctx != nil {
		t.Fatalf("expected nil context, got %#v", ctx)
	}
}
