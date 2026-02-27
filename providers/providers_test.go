// providers 包的单元测试。
// 使用 httptest.Server 模拟 OpenAI API 端点，无需真实 API Key。
// 覆盖：
// - LLMResponse: HasToolCalls
// - OpenAIProvider: NewOpenAIProvider 默认值、GetDefaultModel、Chat（文本/工具调用/错误/超时）、parseResponse
// - GroqTranscriptionProvider: NewGroqTranscriptionProvider、Transcribe（成功/无 Key/文件不存在/API 错误）

package providers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ============ LLMResponse 测试 ============

// TestHasToolCallsTrue 测试有工具调用时返回 true。
func TestHasToolCallsTrue(t *testing.T) {
	r := &LLMResponse{
		ToolCalls: []ToolCallRequest{{ID: "1", Name: "search"}},
	}
	if !r.HasToolCalls() {
		t.Error("HasToolCalls() = false, want true")
	}
}

// TestHasToolCallsFalse 测试无工具调用时返回 false。
func TestHasToolCallsFalse(t *testing.T) {
	r := &LLMResponse{Content: "hello"}
	if r.HasToolCalls() {
		t.Error("HasToolCalls() = true, want false")
	}
}

// TestHasToolCallsNil 测试 ToolCalls 为 nil 时返回 false。
func TestHasToolCallsNil(t *testing.T) {
	r := &LLMResponse{}
	if r.HasToolCalls() {
		t.Error("HasToolCalls() = true for nil ToolCalls, want false")
	}
}

// ============ NewOpenAIProvider 测试 ============

// TestNewOpenAIProviderDefaults 测试默认配置值。
func TestNewOpenAIProviderDefaults(t *testing.T) {
	p := NewOpenAIProvider(OpenAIConfig{APIKey: "test-key"})

	if p.apiKey != "test-key" {
		t.Errorf("apiKey = %q, want %q", p.apiKey, "test-key")
	}
	if p.apiBase != "https://api.openai.com/v1" {
		t.Errorf("apiBase = %q, want default", p.apiBase)
	}
	if p.defaultModel != "gpt-4o" {
		t.Errorf("defaultModel = %q, want gpt-4o", p.defaultModel)
	}
	if p.client.Timeout != 120*time.Second {
		t.Errorf("timeout = %v, want 120s", p.client.Timeout)
	}
}

// TestNewOpenAIProviderCustom 测试自定义配置值。
func TestNewOpenAIProviderCustom(t *testing.T) {
	p := NewOpenAIProvider(OpenAIConfig{
		APIKey:       "my-key",
		APIBase:      "https://custom.api/v1",
		DefaultModel: "claude-3",
		Timeout:      30 * time.Second,
	})

	if p.apiBase != "https://custom.api/v1" {
		t.Errorf("apiBase = %q", p.apiBase)
	}
	if p.defaultModel != "claude-3" {
		t.Errorf("defaultModel = %q", p.defaultModel)
	}
	if p.client.Timeout != 30*time.Second {
		t.Errorf("timeout = %v", p.client.Timeout)
	}
}

// TestGetDefaultModel 测试获取默认模型名。
func TestGetDefaultModel(t *testing.T) {
	p := NewOpenAIProvider(OpenAIConfig{DefaultModel: "my-model"})
	if p.GetDefaultModel() != "my-model" {
		t.Errorf("GetDefaultModel() = %q, want my-model", p.GetDefaultModel())
	}
}

// TestOpenAIProviderImplementsInterface 测试 OpenAIProvider 实现 LLMProvider 接口。
func TestOpenAIProviderImplementsInterface(t *testing.T) {
	var _ LLMProvider = (*OpenAIProvider)(nil)
}

// ============ Chat 测试（使用 httptest 模拟服务器）============

// newTestProvider 创建一个连接到 httptest 服务器的 OpenAIProvider。
func newTestProvider(t *testing.T, handler http.HandlerFunc) (*OpenAIProvider, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	p := NewOpenAIProvider(OpenAIConfig{
		APIKey:  "test-key",
		APIBase: srv.URL,
		Timeout: 5 * time.Second,
	})
	return p, srv
}

// TestChatTextResponse 测试普通文本响应。
func TestChatTextResponse(t *testing.T) {
	p, srv := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		// 验证请求
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %s, want /chat/completions", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type = %q", r.Header.Get("Content-Type"))
		}

		// 验证请求体
		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]interface{}
		json.Unmarshal(body, &reqBody)

		if reqBody["model"] != "gpt-4o" {
			t.Errorf("model = %v", reqBody["model"])
		}

		// 返回响应
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"content": "Hello! How can I help you?",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     10,
				"completion_tokens": 8,
				"total_tokens":      18,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	req := ChatRequest{
		Messages: []map[string]interface{}{
			{"role": "user", "content": "Hello!"},
		},
	}

	result, err := p.Chat(context.Background(), req)
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}
	if result.Content != "Hello! How can I help you?" {
		t.Errorf("Content = %q", result.Content)
	}
	if result.FinishReason != "stop" {
		t.Errorf("FinishReason = %q", result.FinishReason)
	}
	if result.HasToolCalls() {
		t.Error("should not have tool calls")
	}
	if result.Usage["total_tokens"] != 18 {
		t.Errorf("total_tokens = %d, want 18", result.Usage["total_tokens"])
	}
}

// TestChatToolCallResponse 测试工具调用响应。
func TestChatToolCallResponse(t *testing.T) {
	p, srv := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"content": nil,
						"tool_calls": []map[string]interface{}{
							{
								"id": "call_abc123",
								"function": map[string]interface{}{
									"name":      "web_search",
									"arguments": `{"query":"golang testing"}`,
								},
							},
							{
								"id": "call_def456",
								"function": map[string]interface{}{
									"name":      "read_file",
									"arguments": `{"path":"/tmp/test.go"}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	result, err := p.Chat(context.Background(), ChatRequest{
		Messages: []map[string]interface{}{
			{"role": "user", "content": "Search for Go testing"},
		},
		Tools: []map[string]interface{}{
			{"type": "function", "function": map[string]interface{}{"name": "web_search"}},
		},
	})
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}

	if !result.HasToolCalls() {
		t.Fatal("should have tool calls")
	}
	if len(result.ToolCalls) != 2 {
		t.Fatalf("tool calls count = %d, want 2", len(result.ToolCalls))
	}

	tc0 := result.ToolCalls[0]
	if tc0.ID != "call_abc123" {
		t.Errorf("tc[0].ID = %q", tc0.ID)
	}
	if tc0.Name != "web_search" {
		t.Errorf("tc[0].Name = %q", tc0.Name)
	}
	if tc0.Arguments["query"] != "golang testing" {
		t.Errorf("tc[0].Arguments = %v", tc0.Arguments)
	}

	tc1 := result.ToolCalls[1]
	if tc1.Name != "read_file" {
		t.Errorf("tc[1].Name = %q", tc1.Name)
	}
	if tc1.Arguments["path"] != "/tmp/test.go" {
		t.Errorf("tc[1].Arguments = %v", tc1.Arguments)
	}

	if result.FinishReason != "tool_calls" {
		t.Errorf("FinishReason = %q, want tool_calls", result.FinishReason)
	}
}

// TestChatAPIError 测试 API 返回非 200 状态码。
func TestChatAPIError(t *testing.T) {
	p, srv := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"rate limit exceeded"}}`))
	})
	defer srv.Close()

	result, err := p.Chat(context.Background(), ChatRequest{
		Messages: []map[string]interface{}{
			{"role": "user", "content": "hello"},
		},
	})

	// 非 200 返回 *ProviderError
	if err == nil {
		t.Fatal("Chat should return error for 429 response")
	}
	pe, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected *ProviderError, got %T: %v", err, err)
	}
	if pe.StatusCode != 429 {
		t.Errorf("StatusCode = %d, want 429", pe.StatusCode)
	}
	_ = result
}

// TestChatServerError500 测试 500 服务器错误。
func TestChatServerError500(t *testing.T) {
	p, srv := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	})
	defer srv.Close()

	result, err := p.Chat(context.Background(), ChatRequest{
		Messages: []map[string]interface{}{
			{"role": "user", "content": "hello"},
		},
	})
	// 非 200 返回 *ProviderError
	if err == nil {
		t.Fatal("Chat should return error for 500 response")
	}
	pe, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected *ProviderError, got %T: %v", err, err)
	}
	if pe.StatusCode != 500 {
		t.Errorf("StatusCode = %d, want 500", pe.StatusCode)
	}
	_ = result
}

// TestChatContextCancelled 测试上下文取消。
func TestChatContextCancelled(t *testing.T) {
	p, srv := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, err := p.Chat(ctx, ChatRequest{
		Messages: []map[string]interface{}{
			{"role": "user", "content": "hello"},
		},
	})
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

// TestChatCustomModel 测试使用自定义模型。
func TestChatCustomModel(t *testing.T) {
	var receivedModel string
	p, srv := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]interface{}
		json.Unmarshal(body, &reqBody)
		receivedModel, _ = reqBody["model"].(string)

		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "ok"}, "finish_reason": "stop"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	p.Chat(context.Background(), ChatRequest{
		Messages: []map[string]interface{}{{"role": "user", "content": "hi"}},
		Model:    "claude-3-opus",
	})

	if receivedModel != "claude-3-opus" {
		t.Errorf("model sent = %q, want claude-3-opus", receivedModel)
	}
}

// TestChatNoAuthHeader 测试无 API Key 时不发送 Authorization 头。
func TestChatNoAuthHeader(t *testing.T) {
	var authHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "ok"}, "finish_reason": "stop"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewOpenAIProvider(OpenAIConfig{APIKey: "", APIBase: srv.URL})
	p.Chat(context.Background(), ChatRequest{
		Messages: []map[string]interface{}{{"role": "user", "content": "hi"}},
	})

	if authHeader != "" {
		t.Errorf("should not send Authorization when no API key, got %q", authHeader)
	}
}

// TestChatWithTools 测试请求体中包含 tools 和 tool_choice。
func TestChatWithTools(t *testing.T) {
	var hasTools, hasToolChoice bool
	p, srv := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]interface{}
		json.Unmarshal(body, &reqBody)

		_, hasTools = reqBody["tools"]
		_, hasToolChoice = reqBody["tool_choice"]

		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "ok"}, "finish_reason": "stop"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	p.Chat(context.Background(), ChatRequest{
		Messages: []map[string]interface{}{{"role": "user", "content": "hi"}},
		Tools: []map[string]interface{}{
			{"type": "function", "function": map[string]interface{}{"name": "test"}},
		},
	})

	if !hasTools {
		t.Error("request should contain 'tools'")
	}
	if !hasToolChoice {
		t.Error("request should contain 'tool_choice'")
	}
}

// TestChatWithoutTools 测试无 tools 时请求体不包含 tools 字段。
func TestChatWithoutTools(t *testing.T) {
	var hasTools bool
	p, srv := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]interface{}
		json.Unmarshal(body, &reqBody)
		_, hasTools = reqBody["tools"]

		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "ok"}, "finish_reason": "stop"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	p.Chat(context.Background(), ChatRequest{
		Messages: []map[string]interface{}{{"role": "user", "content": "hi"}},
	})

	if hasTools {
		t.Error("request should NOT contain 'tools' when none provided")
	}
}

// ============ parseResponse 边界测试 ============

// TestParseResponseEmptyChoices 测试 choices 为空。
func TestParseResponseEmptyChoices(t *testing.T) {
	p := NewOpenAIProvider(OpenAIConfig{})
	data := []byte(`{"choices":[]}`)
	result, err := p.parseResponse(data)
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}
	if result.Content != "No response from LLM" {
		t.Errorf("Content = %q", result.Content)
	}
	if result.FinishReason != "error" {
		t.Errorf("FinishReason = %q", result.FinishReason)
	}
}

// TestParseResponseNoFinishReason 测试缺少 finish_reason 时默认为 "stop"。
func TestParseResponseNoFinishReason(t *testing.T) {
	p := NewOpenAIProvider(OpenAIConfig{})
	data := []byte(`{"choices":[{"message":{"content":"hello"}}]}`)
	result, err := p.parseResponse(data)
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}
	if result.FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want stop", result.FinishReason)
	}
}

// TestParseResponseNullContent 测试 content 为 null（工具调用场景）。
func TestParseResponseNullContent(t *testing.T) {
	p := NewOpenAIProvider(OpenAIConfig{})
	data := []byte(`{"choices":[{"message":{"content":null,"tool_calls":[{"id":"1","function":{"name":"test","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`)
	result, err := p.parseResponse(data)
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}
	if result.Content != "" {
		t.Errorf("Content should be empty for null, got %q", result.Content)
	}
	if !result.HasToolCalls() {
		t.Error("should have tool calls")
	}
}

// TestParseResponseInvalidJSON 测试无效 JSON。
func TestParseResponseInvalidJSON(t *testing.T) {
	p := NewOpenAIProvider(OpenAIConfig{})
	_, err := p.parseResponse([]byte(`not json`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// TestParseResponseInvalidToolArguments 测试工具调用参数非法 JSON 时回退到 raw。
func TestParseResponseInvalidToolArguments(t *testing.T) {
	p := NewOpenAIProvider(OpenAIConfig{})
	data := []byte(`{"choices":[{"message":{"tool_calls":[{"id":"1","function":{"name":"test","arguments":"not valid json"}}]},"finish_reason":"tool_calls"}]}`)
	result, err := p.parseResponse(data)
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(result.ToolCalls))
	}
	if result.ToolCalls[0].Arguments["raw"] != "not valid json" {
		t.Errorf("should fallback to raw, got %v", result.ToolCalls[0].Arguments)
	}
}

// TestParseResponseNoUsage 测试无 usage 字段。
func TestParseResponseNoUsage(t *testing.T) {
	p := NewOpenAIProvider(OpenAIConfig{})
	data := []byte(`{"choices":[{"message":{"content":"hi"},"finish_reason":"stop"}]}`)
	result, err := p.parseResponse(data)
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}
	if result.Usage != nil {
		t.Errorf("Usage should be nil, got %v", result.Usage)
	}
}

// TestParseResponseWithUsage 测试有 usage 字段。
func TestParseResponseWithUsage(t *testing.T) {
	p := NewOpenAIProvider(OpenAIConfig{})
	data := []byte(`{"choices":[{"message":{"content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`)
	result, err := p.parseResponse(data)
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}
	if result.Usage == nil {
		t.Fatal("Usage should not be nil")
	}
	if result.Usage["prompt_tokens"] != 5 {
		t.Errorf("prompt_tokens = %d, want 5", result.Usage["prompt_tokens"])
	}
	if result.Usage["completion_tokens"] != 3 {
		t.Errorf("completion_tokens = %d, want 3", result.Usage["completion_tokens"])
	}
	if result.Usage["total_tokens"] != 8 {
		t.Errorf("total_tokens = %d, want 8", result.Usage["total_tokens"])
	}
}

// ============ GroqTranscriptionProvider 测试 ============

// TestNewGroqTranscriptionProviderWithKey 测试使用指定 Key 创建。
func TestNewGroqTranscriptionProviderWithKey(t *testing.T) {
	g := NewGroqTranscriptionProvider("my-groq-key")
	if g.apiKey != "my-groq-key" {
		t.Errorf("apiKey = %q", g.apiKey)
	}
	if g.apiURL == "" {
		t.Error("apiURL should not be empty")
	}
}

// TestNewGroqTranscriptionProviderFromEnv 测试从环境变量获取 Key。
func TestNewGroqTranscriptionProviderFromEnv(t *testing.T) {
	os.Setenv("GROQ_API_KEY", "env-key-123")
	defer os.Unsetenv("GROQ_API_KEY")

	g := NewGroqTranscriptionProvider("")
	if g.apiKey != "env-key-123" {
		t.Errorf("apiKey = %q, want env-key-123", g.apiKey)
	}
}

// TestGroqTranscriptionProviderImplementsInterface 测试实现 TranscriptionProvider 接口。
func TestGroqTranscriptionProviderImplementsInterface(t *testing.T) {
	var _ TranscriptionProvider = (*GroqTranscriptionProvider)(nil)
}

// TestTranscribeNoAPIKey 测试无 API Key 时返回错误。
func TestTranscribeNoAPIKey(t *testing.T) {
	g := &GroqTranscriptionProvider{apiKey: ""}
	_, err := g.Transcribe(context.Background(), "/tmp/test.wav")
	if err == nil {
		t.Error("expected error for empty API key")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("error = %v, want 'not configured'", err)
	}
}

// TestTranscribeFileNotFound 测试音频文件不存在。
func TestTranscribeFileNotFound(t *testing.T) {
	g := &GroqTranscriptionProvider{apiKey: "test-key", apiURL: "http://localhost"}
	_, err := g.Transcribe(context.Background(), "/nonexistent/audio.wav")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
	if !strings.Contains(err.Error(), "open audio file") {
		t.Errorf("error = %v", err)
	}
}

// TestTranscribeSuccess 测试成功转写。
func TestTranscribeSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Errorf("content-type = %q, want multipart/form-data", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Authorization") != "Bearer test-groq-key" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}

		// 验证 multipart 表单
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Errorf("parse multipart: %v", err)
		}
		if r.FormValue("model") != "whisper-large-v3" {
			t.Errorf("model = %q, want whisper-large-v3", r.FormValue("model"))
		}
		_, header, err := r.FormFile("file")
		if err != nil {
			t.Errorf("form file error: %v", err)
		}
		if header.Filename != "test.wav" {
			t.Errorf("filename = %q, want test.wav", header.Filename)
		}

		// 返回转写结果
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"text": "Hello, this is a transcription test.",
		})
	}))
	defer srv.Close()

	// 创建临时音频文件
	tmpFile := filepath.Join(t.TempDir(), "test.wav")
	os.WriteFile(tmpFile, []byte("fake audio data"), 0644)

	g := &GroqTranscriptionProvider{
		apiKey: "test-groq-key",
		apiURL: srv.URL,
		client: &http.Client{Timeout: 5 * time.Second},
	}

	result, err := g.Transcribe(context.Background(), tmpFile)
	if err != nil {
		t.Fatalf("Transcribe error: %v", err)
	}
	if result != "Hello, this is a transcription test." {
		t.Errorf("result = %q", result)
	}
}

// TestTranscribeAPIError 测试 API 返回错误。
func TestTranscribeAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"invalid audio format"}}`))
	}))
	defer srv.Close()

	tmpFile := filepath.Join(t.TempDir(), "bad.wav")
	os.WriteFile(tmpFile, []byte("bad data"), 0644)

	g := &GroqTranscriptionProvider{
		apiKey: "test-key",
		apiURL: srv.URL,
		client: &http.Client{Timeout: 5 * time.Second},
	}

	_, err := g.Transcribe(context.Background(), tmpFile)
	if err == nil {
		t.Error("expected error for API error")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error should contain status code, got: %v", err)
	}
}

// TestTranscribeInvalidResponseJSON 测试 API 返回无效 JSON。
func TestTranscribeInvalidResponseJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not valid json"))
	}))
	defer srv.Close()

	tmpFile := filepath.Join(t.TempDir(), "test.wav")
	os.WriteFile(tmpFile, []byte("audio"), 0644)

	g := &GroqTranscriptionProvider{
		apiKey: "test-key",
		apiURL: srv.URL,
		client: &http.Client{Timeout: 5 * time.Second},
	}

	_, err := g.Transcribe(context.Background(), tmpFile)
	if err == nil {
		t.Error("expected error for invalid JSON response")
	}
}

// ============ ChatRequest 默认值填充测试 ============

// TestChatDefaultMaxTokensAndTemperature 测试 Chat 对 MaxTokens 和 Temperature 的默认值填充。
func TestChatDefaultMaxTokensAndTemperature(t *testing.T) {
	var receivedMaxTokens float64
	var receivedTemp float64

	p, srv := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]interface{}
		json.Unmarshal(body, &reqBody)
		receivedMaxTokens, _ = reqBody["max_tokens"].(float64)
		receivedTemp, _ = reqBody["temperature"].(float64)

		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "ok"}, "finish_reason": "stop"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	// MaxTokens=0, Temperature=0 → 应使用默认值
	p.Chat(context.Background(), ChatRequest{
		Messages: []map[string]interface{}{{"role": "user", "content": "hi"}},
	})

	if receivedMaxTokens != 4096 {
		t.Errorf("max_tokens = %v, want 4096", receivedMaxTokens)
	}
	if receivedTemp != 0.7 {
		t.Errorf("temperature = %v, want 0.7", receivedTemp)
	}
}
