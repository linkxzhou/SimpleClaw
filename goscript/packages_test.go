package goscript

// 外部包集成测试。
// 测试 goscript 解释器能否正确调用已注册的标准库包，
// 包括 sync/atomic、encoding/base64、bytes、fmt、net/http、
// encoding/json、math、strings、time、regexp 等。

import (
	"testing"
)

// TestAtomicPackage 测试 sync/atomic 包的原子操作。
func TestAtomicPackage(t *testing.T) {
	source := `
package main

import "sync/atomic"

func test() int32 {
	var val int32 = 0
	atomic.AddInt32(&val, 1)
	return val
}
`
	result, err := Run("", source, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result != int32(1) {
		t.Errorf("Expected 1, got %v", result)
	}
}

// TestBase64Package 测试 encoding/base64 包的编码功能。
func TestBase64Package(t *testing.T) {
	source := `
package main

import "encoding/base64"

func test() string {
	return base64.StdEncoding.EncodeToString([]byte("hello"))
}
`
	result, err := Run("", source, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result != "aGVsbG8=" {
		t.Errorf("Expected 'aGVsbG8=', got %v", result)
	}
}

// TestBytesPackage 测试 bytes 包的字节比较功能。
func TestBytesPackage(t *testing.T) {
	source := `
package main

import "bytes"

func test() bool {
	return bytes.Equal([]byte("hello"), []byte("hello"))
}
`
	result, err := Run("", source, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result != true {
		t.Errorf("Expected true, got %v", result)
	}
}

// TestFmtPackage 测试 fmt 包的格式化输出功能。
func TestFmtPackage(t *testing.T) {
	source := `
package main

import "fmt"

func test() string {
	return fmt.Sprintf("Hello %s", "World")
}
`
	result, err := Run("", source, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result != "Hello World" {
		t.Errorf("Expected 'Hello World', got %v", result)
	}
}

// TestHTTPPackage 测试 net/http 包的常量访问。
func TestHTTPPackage(t *testing.T) {
	source := `
package main

import "net/http"

func test() string {
	return http.MethodGet
}
`
	result, err := Run("", source, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result != "GET" {
		t.Errorf("Expected 'GET', got %v", result)
	}
}

// TestHTTPRequest 测试通过 net/http 发送 HTTP 请求。
func TestHTTPRequest(t *testing.T) {
	source := `
package main

import (
	"net/http"
	"io/ioutil"
)

func test() (int, string) {
	resp, err := http.Get("http://www.qq.com")
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	
	body, _ := ioutil.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}
`
	_, err := Run("", source, "test")
	if err != nil {
		t.Error(err)
	}
}

// TestHTTPSRequest 测试通过 net/http 发送 HTTPS 请求。
func TestHTTPSRequest(t *testing.T) {
	source := `
package main

import (
	"net/http"
	"io/ioutil"
)

func test() (int, string) {
	resp, err := http.Get("https://www.qq.com")
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	
	body, _ := ioutil.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}
`
	_, err := Run("", source, "test")
	if err != nil {
		t.Error(err)
	}
}

// TestJSONPackage 测试 encoding/json 包的序列化功能。
func TestJSONPackage(t *testing.T) {
	source := `
package main

import "encoding/json"

func test() string {
	data := map[string]string{"name": "John"}
	b, _ := json.Marshal(data)
	return string(b)
}
`
	result, err := Run("", source, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result != `{"name":"John"}` {
		t.Errorf("Expected '{\"name\":\"John\"}', got %v", result)
	}
}

// TestMathPackage 测试 math 包的数学函数。
func TestMathPackage(t *testing.T) {
	source := `
package main

import "math"

func test() float64 {
	return math.Sqrt(16)
}
`
	result, err := Run("", source, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result != 4.0 {
		t.Errorf("Expected 4.0, got %v", result)
	}
}

// TestStringsPackage 测试 strings 包的字符串操作功能。
func TestStringsPackage(t *testing.T) {
	source := `
package main

import "strings"

func test() string {
	return strings.ToUpper("hello")
}
`
	result, err := Run("", source, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result != "HELLO" {
		t.Errorf("Expected 'HELLO', got %v", result)
	}
}

// TestTimePackage 测试 time 包的时间常量和运算。
func TestTimePackage(t *testing.T) {
	source := `
package main

import "time"

func test() bool {
	return time.Second == 1000*time.Millisecond
}
`
	result, err := Run("", source, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result != true {
		t.Errorf("Expected true, got %v", result)
	}
}

// TestRegexpPackage 测试 regexp 包的正则表达式匹配功能。
func TestRegexpPackage(t *testing.T) {
	source := `
package main

import "regexp"

func test() bool {
	matched, _ := regexp.MatchString("^[a-z]+$", "hello")
	return matched
}
`
	result, err := Run("", source, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result != true {
		t.Errorf("Expected true, got %v", result)
	}
}
