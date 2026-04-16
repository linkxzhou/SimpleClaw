package providers

import "testing"

func TestGetContextWindowExact(t *testing.T) {
	tests := []struct {
		model    string
		expected int
	}{
		{"gpt-4o", 128000},
		{"openai/gpt-4o", 128000},
		{"claude-sonnet-4-20250514", 200000},
		{"anthropic/claude-sonnet-4-20250514", 200000},
		{"deepseek-chat", 64000},
	}
	for _, tt := range tests {
		got := GetContextWindow(tt.model)
		if got != tt.expected {
			t.Errorf("GetContextWindow(%q) = %d, 期望 %d", tt.model, got, tt.expected)
		}
	}
}

func TestGetContextWindowPrefix(t *testing.T) {
	// gpt-4o-2024-08-06 应匹配 gpt-4o
	got := GetContextWindow("gpt-4o-2024-08-06")
	if got != 128000 {
		t.Errorf("前缀匹配期望 128000，实际 %d", got)
	}
}

func TestGetContextWindowUnknown(t *testing.T) {
	got := GetContextWindow("unknown-model-xyz")
	if got != defaultContextWindow {
		t.Errorf("未知模型期望 %d，实际 %d", defaultContextWindow, got)
	}
}

func TestGetContextWindowGemini(t *testing.T) {
	got := GetContextWindow("gemini-2.0-flash")
	if got != 1048576 {
		t.Errorf("Gemini Flash 期望 1048576，实际 %d", got)
	}
}
