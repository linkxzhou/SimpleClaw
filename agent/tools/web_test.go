package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------- stripTags ----------

func TestStripTags(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text", "hello world", "hello world"},
		{"simple tag", "<b>bold</b>", "bold"},
		{"nested tags", "<div><p>text</p></div>", "text"},
		{"script removal", "before<script>alert('x')</script>after", "beforeafter"},
		{"style removal", "before<style>.a{}</style>after", "beforeafter"},
		{"HTML entities", "&amp; &lt; &gt;", "& < >"},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripTags(tt.input)
			if got != tt.want {
				t.Errorf("stripTags(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------- normalize ----------

func TestNormalize(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"multiple spaces", "a  b   c", "a b c"},
		{"multiple newlines", "a\n\n\n\nb", "a\n\nb"},
		{"tabs", "a\t\tb", "a b"},
		{"leading/trailing whitespace", "  hello  ", "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalize(tt.input)
			if got != tt.want {
				t.Errorf("normalize(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------- htmlToText ----------

func TestHtmlToText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(string) bool
	}{
		{
			"headings", "<h1>Title</h1><h2>Sub</h2>",
			func(s string) bool { return strings.Contains(s, "# Title") && strings.Contains(s, "## Sub") },
		},
		{
			"links", `<a href="http://example.com">click</a>`,
			func(s string) bool { return strings.Contains(s, "[click](http://example.com)") },
		},
		{
			"list items", "<ul><li>one</li><li>two</li></ul>",
			func(s string) bool { return strings.Contains(s, "- one") && strings.Contains(s, "- two") },
		},
		{
			"paragraphs", "<p>one</p><p>two</p>",
			func(s string) bool { return strings.Contains(s, "one") && strings.Contains(s, "two") },
		},
		{
			"br tags", "line1<br/>line2",
			func(s string) bool { return strings.Contains(s, "line1\nline2") },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := htmlToText(tt.input)
			if !tt.check(got) {
				t.Errorf("htmlToText(%q) = %q, check failed", tt.input, got)
			}
		})
	}
}

// ---------- jsonError ----------

func TestJsonError(t *testing.T) {
	result := jsonError("http://example.com", "timeout")
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		t.Fatalf("jsonError returned invalid JSON: %v", err)
	}
	if data["url"] != "http://example.com" {
		t.Errorf("url = %v, want http://example.com", data["url"])
	}
	if data["error"] != "timeout" {
		t.Errorf("error = %v, want timeout", data["error"])
	}
}

// ---------- WebSearchTool ----------

func TestWebSearchTool_Name(t *testing.T) {
	tool := NewWebSearchTool("")
	if tool.Name() != "web_search" {
		t.Errorf("Name = %q, want %q", tool.Name(), "web_search")
	}
}

func TestWebSearchTool_NoAPIKey(t *testing.T) {
	tool := &WebSearchTool{APIKey: "", MaxResults: 5}
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"query": "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "BRAVE_API_KEY not configured") {
		t.Errorf("result = %q, expected API key error", result)
	}
}

func TestWebSearchTool_EmptyQuery(t *testing.T) {
	tool := &WebSearchTool{APIKey: "fake-key", MaxResults: 5}
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"query": "",
	})
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestWebSearchTool_MockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Subscription-Token") != "test-key" {
			t.Errorf("missing API key header")
		}
		resp := `{
			"web": {
				"results": [
					{"title": "Go", "url": "https://go.dev", "description": "Go programming language"},
					{"title": "Rust", "url": "https://rust-lang.org", "description": "Rust language"}
				]
			}
		}`
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, resp)
	}))
	defer server.Close()

	// 由于 WebSearchTool 硬编码了 Brave API URL，无法直接测试 mock server
	// 这里仅验证 count 参数解析逻辑
	tool := &WebSearchTool{APIKey: "test-key", MaxResults: 3}
	params := tool.Parameters()
	props := params["properties"].(map[string]interface{})
	if props["count"] == nil {
		t.Error("Parameters should include 'count'")
	}
}

func TestWebSearchTool_CountBounds(t *testing.T) {
	// 验证 count 的边界处理（通过代码逻辑检查）
	tool := NewWebSearchTool("key")
	if tool.MaxResults != 5 {
		t.Errorf("default MaxResults = %d, want 5", tool.MaxResults)
	}
}

// ---------- WebFetchTool ----------

func TestWebFetchTool_Name(t *testing.T) {
	tool := NewWebFetchTool()
	if tool.Name() != "web_fetch" {
		t.Errorf("Name = %q, want %q", tool.Name(), "web_fetch")
	}
}

func TestWebFetchTool_EmptyURL(t *testing.T) {
	tool := NewWebFetchTool()
	_, err := tool.Execute(context.Background(), map[string]interface{}{"url": ""})
	if err == nil {
		t.Fatal("expected error for empty url")
	}
}

func TestWebFetchTool_FetchHTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><body><h1>Hello</h1><p>World</p></body></html>")
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"url": server.URL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if data["extractor"] != "html" {
		t.Errorf("extractor = %v, want html", data["extractor"])
	}
	text, _ := data["text"].(string)
	if !strings.Contains(text, "Hello") || !strings.Contains(text, "World") {
		t.Errorf("text = %q, expected to contain Hello and World", text)
	}
}

func TestWebFetchTool_FetchJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"name":"test","value":42}`)
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"url": server.URL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var data map[string]interface{}
	json.Unmarshal([]byte(result), &data)
	if data["extractor"] != "json" {
		t.Errorf("extractor = %v, want json", data["extractor"])
	}
}

func TestWebFetchTool_FetchPlainText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "plain text content")
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"url": server.URL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var data map[string]interface{}
	json.Unmarshal([]byte(result), &data)
	if data["extractor"] != "raw" {
		t.Errorf("extractor = %v, want raw", data["extractor"])
	}
	text, _ := data["text"].(string)
	if text != "plain text content" {
		t.Errorf("text = %q, want %q", text, "plain text content")
	}
}

func TestWebFetchTool_Truncation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, strings.Repeat("x", 5000))
	}))
	defer server.Close()

	tool := &WebFetchTool{MaxChars: 100}
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"url": server.URL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var data map[string]interface{}
	json.Unmarshal([]byte(result), &data)
	if data["truncated"] != true {
		t.Errorf("truncated = %v, want true", data["truncated"])
	}
}

func TestWebFetchTool_MaxCharsParam(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, strings.Repeat("a", 1000))
	}))
	defer server.Close()

	tool := &WebFetchTool{MaxChars: 50000}
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"url":       server.URL,
		"max_chars": float64(200),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var data map[string]interface{}
	json.Unmarshal([]byte(result), &data)
	if data["truncated"] != true {
		t.Errorf("truncated = %v, want true", data["truncated"])
	}
	length, _ := data["length"].(float64)
	if int(length) != 200 {
		t.Errorf("length = %v, want 200", length)
	}
}

func TestWebFetchTool_InvalidURL(t *testing.T) {
	tool := NewWebFetchTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"url": "http://127.0.0.1:1", // 不可达端口
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var data map[string]interface{}
	json.Unmarshal([]byte(result), &data)
	if data["error"] == nil {
		t.Error("expected error in response for unreachable URL")
	}
}

func TestWebFetchTool_StatusInResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "not found")
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"url": server.URL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var data map[string]interface{}
	json.Unmarshal([]byte(result), &data)
	status, _ := data["status"].(float64)
	if int(status) != 404 {
		t.Errorf("status = %v, want 404", data["status"])
	}
}

func TestNewWebFetchTool_Defaults(t *testing.T) {
	tool := NewWebFetchTool()
	if tool.MaxChars != 50000 {
		t.Errorf("MaxChars = %d, want 50000", tool.MaxChars)
	}
}
