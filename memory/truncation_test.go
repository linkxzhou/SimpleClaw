package memory

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// FindSafeCutPoints 测试
// ---------------------------------------------------------------------------

func TestFindSafeCutPointsBasic(t *testing.T) {
	messages := []TruncMessage{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
		{Role: "user", Content: "How are you?"},
		{Role: "assistant", Content: "I'm fine."},
	}

	points := FindSafeCutPoints(messages)
	if len(points) == 0 {
		t.Fatal("应找到切割点")
	}

	// 第一个 assistant 回复后应是切割点
	found := false
	for _, p := range points {
		if p.Index == 3 { // assistant[2] 之后
			found = true
			break
		}
	}
	if !found {
		t.Error("应在 index=3（第一个 assistant 之后）找到切割点")
	}
}

func TestFindSafeCutPointsToolPair(t *testing.T) {
	// assistant 带 tool_use 后紧跟 tool result → 不应在中间切割
	messages := []TruncMessage{
		{Role: "system", Content: "System"},
		{Role: "user", Content: "Run command"},
		{Role: "assistant", Content: "Let me run that."},
		{Role: "tool", Content: "result: ok"},
		{Role: "assistant", Content: "Done!"},
	}

	points := FindSafeCutPoints(messages)
	for _, p := range points {
		if p.Index == 3 {
			t.Error("不应在 tool_use → tool_result 之间切割 (index=3)")
		}
	}
}

func TestFindSafeCutPointsNoSystemCut(t *testing.T) {
	messages := []TruncMessage{
		{Role: "system", Content: "Long system prompt..."},
	}
	points := FindSafeCutPoints(messages)
	if len(points) != 0 {
		t.Error("仅 system 消息不应产生切割点")
	}
}

// ---------------------------------------------------------------------------
// PickCutPoint 测试
// ---------------------------------------------------------------------------

func TestPickCutPointExactMatch(t *testing.T) {
	points := []CutPoint{
		{Index: 2, TokensSaved: 50},
		{Index: 4, TokensSaved: 150},
		{Index: 6, TokensSaved: 300},
	}
	got := PickCutPoint(points, 100)
	if got == nil || got.Index != 4 {
		t.Errorf("应选择 Index=4，实际 %v", got)
	}
}

func TestPickCutPointFallback(t *testing.T) {
	points := []CutPoint{
		{Index: 2, TokensSaved: 50},
	}
	got := PickCutPoint(points, 1000)
	if got == nil || got.Index != 2 {
		t.Error("无法满足目标时应选最后一个")
	}
}

func TestPickCutPointEmpty(t *testing.T) {
	got := PickCutPoint(nil, 100)
	if got != nil {
		t.Error("空列表应返回 nil")
	}
}

// ---------------------------------------------------------------------------
// TruncateMessages 测试
// ---------------------------------------------------------------------------

func TestTruncateMessagesNoBudgetExceeded(t *testing.T) {
	messages := []TruncMessage{
		{Role: "system", Content: "Hi"},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi"},
	}
	result, archived := TruncateMessages(messages, 100000)
	if archived != 0 {
		t.Errorf("预算充足不应截断，archived=%d", archived)
	}
	if len(result) != 3 {
		t.Errorf("消息数应不变，实际 %d", len(result))
	}
}

func TestTruncateMessagesPreservesSystem(t *testing.T) {
	messages := []TruncMessage{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: strings.Repeat("hello ", 1000)},
		{Role: "assistant", Content: strings.Repeat("world ", 1000)},
		{Role: "user", Content: "Latest question"},
		{Role: "assistant", Content: "Latest answer"},
	}

	result, archived := TruncateMessages(messages, 200)
	if archived <= 0 {
		t.Fatal("应执行截断")
	}

	// system 消息必须保留
	if result[0].Role != "system" || result[0].Content != "You are helpful." {
		t.Error("system 消息应保留")
	}

	// 最后的消息应保留
	last := result[len(result)-1]
	if last.Content != "Latest answer" {
		t.Errorf("最后一条消息应保留，实际 %q", last.Content)
	}
}

func TestTruncateMessagesArchivePlaceholder(t *testing.T) {
	messages := []TruncMessage{
		{Role: "system", Content: "System"},
		{Role: "user", Content: strings.Repeat("a", 5000)},
		{Role: "assistant", Content: strings.Repeat("b", 5000)},
		{Role: "user", Content: "recent"},
		{Role: "assistant", Content: "recent reply"},
	}

	result, _ := TruncateMessages(messages, 300)

	// 应有归档占位符
	hasPlaceholder := false
	for _, m := range result {
		if m.Role == "system" && strings.Contains(m.Content, "archived") {
			hasPlaceholder = true
			break
		}
	}
	if !hasPlaceholder {
		t.Error("截断后应包含归档占位符")
	}
}

func TestTruncateMessagesToolPairIntact(t *testing.T) {
	// 确保 tool_use → tool_result 不会被拆分
	messages := []TruncMessage{
		{Role: "system", Content: "System"},
		{Role: "user", Content: strings.Repeat("x", 3000)},
		{Role: "assistant", Content: strings.Repeat("y", 3000)},
		{Role: "user", Content: "run command"},
		{Role: "assistant", Content: "executing..."},
		{Role: "tool", Content: "result ok"},
		{Role: "assistant", Content: "Done"},
	}

	result, _ := TruncateMessages(messages, 500)

	// 检查不存在孤立 tool 消息（前面必须有 assistant）
	for i, m := range result {
		if m.Role == "tool" {
			if i == 0 || result[i-1].Role != "assistant" {
				t.Error("tool 消息前应有 assistant 消息")
			}
		}
	}
}

// ---------------------------------------------------------------------------
// 上下文窗口
// ---------------------------------------------------------------------------

func TestContextWindowProvider(t *testing.T) {
	// 这是间接测试 — 确保 providers 包的 GetContextWindow 可用
	// 直接测试放在 providers/context_window_test.go
}
