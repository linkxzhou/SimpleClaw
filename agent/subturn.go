// SubTurn 子回合机制：同步/异步双模式嵌套执行单元，
// 支持并发控制（信号量）、深度限制、独立超时和临时会话。

package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrDepthLimitExceeded = errors.New("subturn: max depth exceeded")
	ErrConcurrencyTimeout = errors.New("subturn: concurrency limit timeout")
)

// SubTurnConfig 子回合配置。
type SubTurnConfig struct {
	Model           string        // 可用不同模型
	SystemPrompt    string        // 子回合系统提示词
	UserMessage     string        // 要处理的用户消息
	Tools           []string      // 受限工具列表（空=使用默认）
	MaxTokens       int           // 最大输出 token
	Async           bool          // false=同步阻塞，true=后台运行
	Critical        bool          // true=父结束后子继续运行
	Timeout         time.Duration // 独立超时（默认 5min）
	MaxContextRunes int           // 上下文软截断（0=自动）
}

// SubTurnResult 子回合执行结果。
type SubTurnResult struct {
	ID         string        `json:"id"`
	Content    string        `json:"content"`
	StopReason string        `json:"stop_reason"` // "complete" | "timeout" | "budget_exhausted" | "error"
	Duration   time.Duration `json:"duration"`
	TokensUsed int64         `json:"tokens_used"`
	Error      error         `json:"-"`
}

// SubTurnManager 管理子回合的并发和深度控制。
type SubTurnManager struct {
	semaphore          chan struct{}
	maxDepth           int
	concurrencyTimeout time.Duration
	pendingResults     chan *SubTurnResult
	handler            SubTurnHandler // 实际执行逻辑
	nextID             atomic.Int64
	mu                 sync.Mutex
}

// SubTurnHandler 子回合处理函数。
type SubTurnHandler func(ctx context.Context, cfg SubTurnConfig) *SubTurnResult

// SubTurnManagerConfig 管理器配置。
type SubTurnManagerConfig struct {
	MaxConcurrent      int           // 最大并发子回合数（默认 5）
	MaxDepth           int           // 最大嵌套深度（默认 3）
	ConcurrencyTimeout time.Duration // 等待信号量超时（默认 30s）
}

// NewSubTurnManager 创建子回合管理器。
func NewSubTurnManager(config SubTurnManagerConfig, handler SubTurnHandler) *SubTurnManager {
	if config.MaxConcurrent <= 0 {
		config.MaxConcurrent = 5
	}
	if config.MaxDepth <= 0 {
		config.MaxDepth = 3
	}
	if config.ConcurrencyTimeout <= 0 {
		config.ConcurrencyTimeout = 30 * time.Second
	}
	return &SubTurnManager{
		semaphore:          make(chan struct{}, config.MaxConcurrent),
		maxDepth:           config.MaxDepth,
		concurrencyTimeout: config.ConcurrencyTimeout,
		pendingResults:     make(chan *SubTurnResult, 100),
		handler:            handler,
	}
}

// SpawnSubTurn 创建并执行子回合。
// 同步模式阻塞等待结果；异步模式立即返回，结果送入 PendingResults。
func (m *SubTurnManager) SpawnSubTurn(ctx context.Context, cfg SubTurnConfig) (*SubTurnResult, error) {
	// 1. 深度检查
	depth := GetDepthFromContext(ctx)
	if depth >= m.maxDepth {
		return nil, ErrDepthLimitExceeded
	}

	// 2. 并发信号量获取
	select {
	case m.semaphore <- struct{}{}:
		// 获得信号量
	case <-time.After(m.concurrencyTimeout):
		return nil, ErrConcurrencyTimeout
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	id := fmt.Sprintf("st_%d", m.nextID.Add(1))

	// 默认超时
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	run := func() *SubTurnResult {
		defer func() { <-m.semaphore }() // 释放信号量

		// 创建独立超时 context
		childCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		childCtx = WithDepth(childCtx, depth+1)

		start := time.Now()
		result := m.handler(childCtx, cfg)
		if result == nil {
			result = &SubTurnResult{StopReason: "complete"}
		}
		result.ID = id
		result.Duration = time.Since(start)
		return result
	}

	if cfg.Async {
		go func() {
			result := run()
			m.pendingResults <- result
		}()
		return &SubTurnResult{ID: id, StopReason: "async_started"}, nil
	}

	return run(), nil
}

// PendingResults 返回异步子回合的结果通道。
func (m *SubTurnManager) PendingResults() <-chan *SubTurnResult {
	return m.pendingResults
}

// ============ Context 深度传递 ============

type depthKey struct{}

// WithDepth 在 context 中设置嵌套深度。
func WithDepth(ctx context.Context, d int) context.Context {
	return context.WithValue(ctx, depthKey{}, d)
}

// GetDepthFromContext 从 context 获取当前嵌套深度。
func GetDepthFromContext(ctx context.Context) int {
	if d, ok := ctx.Value(depthKey{}).(int); ok {
		return d
	}
	return 0
}

// ============ TokenBudget 共享 token 预算 ============

// TokenBudget 原子计数器，多并发子回合间共享。
type TokenBudget struct {
	remaining atomic.Int64
}

// NewTokenBudget 创建 token 预算。
func NewTokenBudget(total int64) *TokenBudget {
	b := &TokenBudget{}
	b.remaining.Store(total)
	return b
}

// Deduct 扣减 token。返回剩余量和是否成功。
func (b *TokenBudget) Deduct(used int64) (remaining int64, ok bool) {
	for {
		current := b.remaining.Load()
		if current < used {
			return current, false // 预算不足
		}
		if b.remaining.CompareAndSwap(current, current-used) {
			return current - used, true
		}
	}
}

// Remaining 返回剩余预算。
func (b *TokenBudget) Remaining() int64 {
	return b.remaining.Load()
}

// ============ EphemeralSession 临时会话 ============

// EphemeralSession 临时会话，不持久化，用于子回合。
type EphemeralSession struct {
	messages []Message
	maxSize  int
	mu       sync.Mutex
}

// NewEphemeralSession 创建临时会话。
func NewEphemeralSession(maxSize int) *EphemeralSession {
	if maxSize <= 0 {
		maxSize = 50
	}
	return &EphemeralSession{maxSize: maxSize}
}

// GetOrCreateSession 实现 SessionStore 接口。
func (e *EphemeralSession) GetOrCreateSession(key string) []Message {
	e.mu.Lock()
	defer e.mu.Unlock()
	cp := make([]Message, len(e.messages))
	copy(cp, e.messages)
	return cp
}

// SaveSession 实现 SessionStore 接口（保存到内存，不持久化）。
func (e *EphemeralSession) SaveSession(key string, messages []Message) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(messages) > e.maxSize {
		messages = messages[len(messages)-e.maxSize:]
	}
	e.messages = make([]Message, len(messages))
	copy(e.messages, messages)
}
