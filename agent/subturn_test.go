package agent

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// SubTurnManager 测试
// ---------------------------------------------------------------------------

func TestSubTurnSyncBasic(t *testing.T) {
	handler := func(ctx context.Context, cfg SubTurnConfig) *SubTurnResult {
		return &SubTurnResult{Content: "done: " + cfg.UserMessage, StopReason: "complete"}
	}

	m := NewSubTurnManager(SubTurnManagerConfig{}, handler)
	result, err := m.SpawnSubTurn(context.Background(), SubTurnConfig{
		UserMessage: "test task",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "done: test task" {
		t.Errorf("期望 'done: test task'，实际 %q", result.Content)
	}
	if result.Duration <= 0 {
		t.Error("Duration 应 > 0")
	}
}

func TestSubTurnAsync(t *testing.T) {
	handler := func(ctx context.Context, cfg SubTurnConfig) *SubTurnResult {
		time.Sleep(20 * time.Millisecond)
		return &SubTurnResult{Content: "async done", StopReason: "complete"}
	}

	m := NewSubTurnManager(SubTurnManagerConfig{}, handler)
	result, err := m.SpawnSubTurn(context.Background(), SubTurnConfig{
		Async:   true,
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != "async_started" {
		t.Errorf("异步应立即返回 async_started，实际 %q", result.StopReason)
	}

	// 等待异步结果
	select {
	case r := <-m.PendingResults():
		if r.Content != "async done" {
			t.Errorf("异步结果应为 'async done'，实际 %q", r.Content)
		}
	case <-time.After(time.Second):
		t.Fatal("等待异步结果超时")
	}
}

func TestSubTurnDepthLimit(t *testing.T) {
	handler := func(ctx context.Context, cfg SubTurnConfig) *SubTurnResult {
		return &SubTurnResult{StopReason: "complete"}
	}

	m := NewSubTurnManager(SubTurnManagerConfig{MaxDepth: 2}, handler)

	// 深度 0 → OK
	ctx := WithDepth(context.Background(), 0)
	_, err := m.SpawnSubTurn(ctx, SubTurnConfig{Timeout: time.Second})
	if err != nil {
		t.Errorf("深度 0 应允许，实际 %v", err)
	}

	// 深度 1 → OK
	ctx = WithDepth(context.Background(), 1)
	_, err = m.SpawnSubTurn(ctx, SubTurnConfig{Timeout: time.Second})
	if err != nil {
		t.Errorf("深度 1 应允许，实际 %v", err)
	}

	// 深度 2 → 超限
	ctx = WithDepth(context.Background(), 2)
	_, err = m.SpawnSubTurn(ctx, SubTurnConfig{Timeout: time.Second})
	if err != ErrDepthLimitExceeded {
		t.Errorf("深度 2 应被拒绝，实际 %v", err)
	}
}

func TestSubTurnConcurrencyLimit(t *testing.T) {
	var running int32

	handler := func(ctx context.Context, cfg SubTurnConfig) *SubTurnResult {
		cur := atomic.AddInt32(&running, 1)
		if cur > 2 {
			t.Errorf("并发数超过上限: %d", cur)
		}
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt32(&running, -1)
		return &SubTurnResult{StopReason: "complete"}
	}

	m := NewSubTurnManager(SubTurnManagerConfig{
		MaxConcurrent:      2,
		ConcurrencyTimeout: time.Second,
	}, handler)

	// 启动 3 个异步子回合，最多并发 2
	for i := 0; i < 3; i++ {
		m.SpawnSubTurn(context.Background(), SubTurnConfig{Async: true, Timeout: 5 * time.Second})
	}

	time.Sleep(200 * time.Millisecond) // 等待全部完成
}

func TestSubTurnTimeout(t *testing.T) {
	handler := func(ctx context.Context, cfg SubTurnConfig) *SubTurnResult {
		select {
		case <-ctx.Done():
			return &SubTurnResult{StopReason: "timeout"}
		case <-time.After(5 * time.Second):
			return &SubTurnResult{StopReason: "complete"}
		}
	}

	m := NewSubTurnManager(SubTurnManagerConfig{}, handler)
	result, err := m.SpawnSubTurn(context.Background(), SubTurnConfig{
		Timeout: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != "timeout" {
		t.Errorf("应超时，实际 %q", result.StopReason)
	}
}

// ---------------------------------------------------------------------------
// TokenBudget 测试
// ---------------------------------------------------------------------------

func TestTokenBudgetDeduct(t *testing.T) {
	b := NewTokenBudget(1000)

	remaining, ok := b.Deduct(300)
	if !ok || remaining != 700 {
		t.Errorf("扣减 300 后应剩 700，实际 %d, ok=%v", remaining, ok)
	}

	remaining, ok = b.Deduct(800)
	if ok {
		t.Error("预算不足时应返回 false")
	}
	if remaining != 700 {
		t.Errorf("失败时应返回当前剩余 700，实际 %d", remaining)
	}
}

func TestTokenBudgetConcurrent(t *testing.T) {
	b := NewTokenBudget(10000)
	var wg sync.WaitGroup
	var successCount int32

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, ok := b.Deduct(100)
			if ok {
				atomic.AddInt32(&successCount, 1)
			}
		}()
	}

	wg.Wait()
	if atomic.LoadInt32(&successCount) != 100 {
		t.Errorf("10000 预算扣 100*100 应全部成功，实际 %d", atomic.LoadInt32(&successCount))
	}
	if b.Remaining() != 0 {
		t.Errorf("全部扣完后应剩 0，实际 %d", b.Remaining())
	}
}

// ---------------------------------------------------------------------------
// EphemeralSession 测试
// ---------------------------------------------------------------------------

func TestEphemeralSession(t *testing.T) {
	s := NewEphemeralSession(5)

	msgs := s.GetOrCreateSession("key")
	if len(msgs) != 0 {
		t.Error("初始应为空")
	}

	s.SaveSession("key", []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	})

	msgs = s.GetOrCreateSession("key")
	if len(msgs) != 2 {
		t.Errorf("应有 2 条，实际 %d", len(msgs))
	}
}

func TestEphemeralSessionMaxSize(t *testing.T) {
	s := NewEphemeralSession(3)

	msgs := make([]Message, 10)
	for i := range msgs {
		msgs[i] = Message{Role: "user", Content: "msg"}
	}
	s.SaveSession("key", msgs)

	result := s.GetOrCreateSession("key")
	if len(result) != 3 {
		t.Errorf("超过 maxSize 应截断为 3，实际 %d", len(result))
	}
}

// ---------------------------------------------------------------------------
// Context Depth 测试
// ---------------------------------------------------------------------------

func TestContextDepth(t *testing.T) {
	ctx := context.Background()
	if GetDepthFromContext(ctx) != 0 {
		t.Error("默认深度应为 0")
	}

	ctx = WithDepth(ctx, 5)
	if GetDepthFromContext(ctx) != 5 {
		t.Errorf("设置后应为 5，实际 %d", GetDepthFromContext(ctx))
	}
}
