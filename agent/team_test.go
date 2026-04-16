package agent

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// AgentRegistry 测试
// ---------------------------------------------------------------------------

func TestRegistryBasic(t *testing.T) {
	r := NewAgentRegistry()
	r.RegisterAgent(AgentProfile{ID: "coder", Model: "gpt-4o"})
	r.RegisterAgent(AgentProfile{ID: "reviewer", Model: "gpt-4o-mini"})

	if r.AgentCount() != 2 {
		t.Errorf("应有 2 个 Agent，实际 %d", r.AgentCount())
	}
	if r.GetAgent("coder") == nil {
		t.Error("coder 应存在")
	}
	if !r.HasAgent("reviewer") {
		t.Error("reviewer 应存在")
	}
	if r.HasAgent("unknown") {
		t.Error("unknown 不应存在")
	}
}

func TestRegistryDefault(t *testing.T) {
	r := NewAgentRegistry()
	r.RegisterAgent(AgentProfile{ID: "a"})
	r.RegisterAgent(AgentProfile{ID: "b"})

	// 第一个注册的应为默认
	if r.GetDefault().ID != "a" {
		t.Errorf("默认应为 a，实际 %s", r.GetDefault().ID)
	}

	r.SetDefault("b")
	if r.GetDefault().ID != "b" {
		t.Errorf("设置默认后应为 b，实际 %s", r.GetDefault().ID)
	}
}

func TestRegistryTeam(t *testing.T) {
	r := NewAgentRegistry()
	r.RegisterAgent(AgentProfile{ID: "coder"})
	r.RegisterAgent(AgentProfile{ID: "reviewer"})
	r.RegisterAgent(AgentProfile{ID: "tester"})

	r.RegisterTeam(TeamConfig{
		ID:     "engineering",
		Agents: []string{"coder", "reviewer", "tester"},
		Leader: "coder",
	})

	if r.TeamCount() != 1 {
		t.Errorf("应有 1 个团队，实际 %d", r.TeamCount())
	}

	team := r.GetTeam("engineering")
	if team == nil || len(team.Agents) != 3 {
		t.Error("engineering 团队应有 3 个成员")
	}

	mates := r.GetTeammates("coder", "engineering")
	if len(mates) != 2 {
		t.Errorf("coder 的队友应有 2 个，实际 %d", len(mates))
	}
}

func TestRegistryListAgents(t *testing.T) {
	r := NewAgentRegistry()
	r.RegisterAgent(AgentProfile{ID: "x"})
	r.RegisterAgent(AgentProfile{ID: "y"})

	ids := r.ListAgents()
	if len(ids) != 2 {
		t.Errorf("应列出 2 个，实际 %d", len(ids))
	}
}

// ---------------------------------------------------------------------------
// ParseBracketTags 测试
// ---------------------------------------------------------------------------

func TestParseMentionBasic(t *testing.T) {
	r := NewAgentRegistry()
	r.RegisterAgent(AgentProfile{ID: "coder"})

	result := ParseBracketTags("Hello [@coder: fix the bug]", "main", r)
	if len(result.Mentions) != 1 {
		t.Fatalf("应有 1 个 mention，实际 %d", len(result.Mentions))
	}
	if result.Mentions[0].AgentIDs[0] != "coder" {
		t.Errorf("目标应为 coder，实际 %v", result.Mentions[0].AgentIDs)
	}
	if result.Mentions[0].Message != "fix the bug" {
		t.Errorf("消息应为 'fix the bug'，实际 %q", result.Mentions[0].Message)
	}
	if result.CleanText != "Hello" {
		t.Errorf("CleanText 应为 'Hello'，实际 %q", result.CleanText)
	}
}

func TestParseMentionMultiTarget(t *testing.T) {
	r := NewAgentRegistry()
	r.RegisterAgent(AgentProfile{ID: "a"})
	r.RegisterAgent(AgentProfile{ID: "b"})

	result := ParseBracketTags("[@a,b: do something]", "main", r)
	if len(result.Mentions) != 1 {
		t.Fatalf("应有 1 个 mention，实际 %d", len(result.Mentions))
	}
	if len(result.Mentions[0].AgentIDs) != 2 {
		t.Errorf("应有 2 个目标，实际 %d", len(result.Mentions[0].AgentIDs))
	}
}

func TestParseSelfReferenceFiltered(t *testing.T) {
	r := NewAgentRegistry()
	r.RegisterAgent(AgentProfile{ID: "main"})

	result := ParseBracketTags("[@main: self reference]", "main", r)
	if len(result.Mentions) != 0 {
		t.Error("自引用应被过滤")
	}
}

func TestParseUnknownAgentFiltered(t *testing.T) {
	r := NewAgentRegistry()
	// 不注册任何 Agent

	result := ParseBracketTags("[@unknown: test]", "main", r)
	if len(result.Mentions) != 0 {
		t.Error("未注册的 Agent 应被过滤")
	}
}

func TestParseChatroomTag(t *testing.T) {
	r := NewAgentRegistry()
	r.RegisterTeam(TeamConfig{ID: "eng", Agents: []string{"a", "b"}})

	result := ParseBracketTags("[#eng: team discussion]", "a", r)
	if len(result.ChatRooms) != 1 {
		t.Fatalf("应有 1 个 chatroom，实际 %d", len(result.ChatRooms))
	}
	if result.ChatRooms[0].TeamID != "eng" {
		t.Errorf("TeamID 应为 eng，实际 %q", result.ChatRooms[0].TeamID)
	}
}

func TestParseNestedBrackets(t *testing.T) {
	r := NewAgentRegistry()
	r.RegisterAgent(AgentProfile{ID: "coder"})

	result := ParseBracketTags("[@coder: fix arr[0] bug]", "main", r)
	if len(result.Mentions) != 1 {
		t.Fatalf("应有 1 个 mention，实际 %d", len(result.Mentions))
	}
	if result.Mentions[0].Message != "fix arr[0] bug" {
		t.Errorf("消息应保留嵌套括号，实际 %q", result.Mentions[0].Message)
	}
}

func TestParseNoTags(t *testing.T) {
	result := ParseBracketTags("Hello world, no tags here", "main", nil)
	if len(result.Mentions) != 0 || len(result.ChatRooms) != 0 {
		t.Error("无标签时不应有解析结果")
	}
	if result.CleanText != "Hello world, no tags here" {
		t.Errorf("CleanText 异常: %q", result.CleanText)
	}
}

func TestParseMultipleTags(t *testing.T) {
	r := NewAgentRegistry()
	r.RegisterAgent(AgentProfile{ID: "a"})
	r.RegisterAgent(AgentProfile{ID: "b"})

	result := ParseBracketTags("Start [@a: task1] middle [@b: task2] end", "main", r)
	if len(result.Mentions) != 2 {
		t.Errorf("应有 2 个 mention，实际 %d", len(result.Mentions))
	}
	if result.CleanText != "Start  middle  end" {
		t.Errorf("CleanText 异常: %q", result.CleanText)
	}
}

// ---------------------------------------------------------------------------
// isValidAgentID 测试
// ---------------------------------------------------------------------------

func TestIsValidAgentID(t *testing.T) {
	tests := []struct {
		id    string
		valid bool
	}{
		{"coder", true},
		{"code-reviewer", true},
		{"agent_1", true},
		{"", false},
		{"a b", false},
		{"abc!def", false},
	}
	for _, tt := range tests {
		if got := isValidAgentID(tt.id); got != tt.valid {
			t.Errorf("isValidAgentID(%q) = %v, want %v", tt.id, got, tt.valid)
		}
	}
}

// ---------------------------------------------------------------------------
// AgentChainManager 测试
// ---------------------------------------------------------------------------

func TestChainSerialExecution(t *testing.T) {
	var order []string
	var mu sync.Mutex

	handler := func(ctx context.Context, msg ChainMessage) {
		time.Sleep(10 * time.Millisecond)
		mu.Lock()
		order = append(order, msg.Content)
		mu.Unlock()
	}

	cm := NewAgentChainManager(handler, nil)
	cm.Enqueue(ChainMessage{AgentID: "a", Content: "1"})
	cm.Enqueue(ChainMessage{AgentID: "a", Content: "2"})
	cm.Enqueue(ChainMessage{AgentID: "a", Content: "3"})

	time.Sleep(100 * time.Millisecond)
	cm.Stop()

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 3 || order[0] != "1" || order[1] != "2" || order[2] != "3" {
		t.Errorf("串行顺序应为 1,2,3，实际 %v", order)
	}
}

func TestChainParallelAgents(t *testing.T) {
	var count int32

	handler := func(ctx context.Context, msg ChainMessage) {
		atomic.AddInt32(&count, 1)
		time.Sleep(20 * time.Millisecond)
	}

	cm := NewAgentChainManager(handler, nil)
	cm.Enqueue(ChainMessage{AgentID: "a", Content: "1"})
	cm.Enqueue(ChainMessage{AgentID: "b", Content: "2"})
	cm.Enqueue(ChainMessage{AgentID: "c", Content: "3"})

	time.Sleep(50 * time.Millisecond)
	cm.Stop()

	if atomic.LoadInt32(&count) != 3 {
		t.Errorf("3 个不同 Agent 应全部完成，实际 %d", atomic.LoadInt32(&count))
	}
	if cm.ActiveChains() != 0 {
		t.Errorf("Stop 后应无活跃 chain，实际 %d", cm.ActiveChains())
	}
}

func TestChainPanicRecovery(t *testing.T) {
	var completed int32

	handler := func(ctx context.Context, msg ChainMessage) {
		if msg.Content == "panic" {
			panic("test panic")
		}
		atomic.AddInt32(&completed, 1)
	}

	cm := NewAgentChainManager(handler, nil)
	cm.Enqueue(ChainMessage{AgentID: "a", Content: "panic"})
	cm.Enqueue(ChainMessage{AgentID: "a", Content: "ok"})

	time.Sleep(50 * time.Millisecond)
	cm.Stop()

	if atomic.LoadInt32(&completed) != 1 {
		t.Errorf("panic 后续消息应继续处理，实际完成 %d", atomic.LoadInt32(&completed))
	}
}

func TestChainActiveChains(t *testing.T) {
	cm := NewAgentChainManager(func(ctx context.Context, msg ChainMessage) {}, nil)
	cm.Enqueue(ChainMessage{AgentID: "a"})
	cm.Enqueue(ChainMessage{AgentID: "b"})

	time.Sleep(10 * time.Millisecond)
	if cm.ActiveChains() != 2 {
		t.Errorf("应有 2 个活跃 chain，实际 %d", cm.ActiveChains())
	}
	cm.Stop()
}
