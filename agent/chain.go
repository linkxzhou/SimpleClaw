// Agent Chain：per-Agent 串行执行管理。
// 同一 Agent 的消息严格串行处理，不同 Agent 之间可并行。

package agent

import (
	"context"
	"log/slog"
	"sync"
)

// ChainMessage 待处理的消息。
type ChainMessage struct {
	AgentID string
	Content string
	From    string // 来源 Agent ID
}

// ChainHandler 消息处理回调。
type ChainHandler func(ctx context.Context, msg ChainMessage)

// AgentChainManager 管理 per-Agent 串行队列。
type AgentChainManager struct {
	chains  map[string]*agentChain
	handler ChainHandler
	mu      sync.Mutex
	logger  *slog.Logger
}

type agentChain struct {
	queue chan ChainMessage
	done  chan struct{}
}

// NewAgentChainManager 创建 Chain 管理器。
// bufSize 为每个 Agent 的队列缓冲大小（默认 100）。
func NewAgentChainManager(handler ChainHandler, logger *slog.Logger) *AgentChainManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &AgentChainManager{
		chains:  make(map[string]*agentChain),
		handler: handler,
		logger:  logger,
	}
}

// Enqueue 将消息放入指定 Agent 的串行队列。
// 如果该 Agent 的消费者 goroutine 还未启动，则启动。
func (m *AgentChainManager) Enqueue(msg ChainMessage) {
	m.mu.Lock()
	chain, ok := m.chains[msg.AgentID]
	if !ok {
		chain = &agentChain{
			queue: make(chan ChainMessage, 100),
			done:  make(chan struct{}),
		}
		m.chains[msg.AgentID] = chain
		go m.runChain(msg.AgentID, chain)
	}
	m.mu.Unlock()

	select {
	case chain.queue <- msg:
	default:
		m.logger.Warn("agent chain queue full, dropping message",
			"agent_id", msg.AgentID, "from", msg.From)
	}
}

// Stop 停止所有 Chain。
func (m *AgentChainManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, chain := range m.chains {
		close(chain.queue)
		<-chain.done // 等待消费者退出
	}
	m.chains = make(map[string]*agentChain)
}

// ActiveChains 返回当前活跃的 Agent Chain 数量。
func (m *AgentChainManager) ActiveChains() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.chains)
}

// QueueSize 返回指定 Agent 的队列长度。
func (m *AgentChainManager) QueueSize(agentID string) int {
	m.mu.Lock()
	chain, ok := m.chains[agentID]
	m.mu.Unlock()
	if !ok {
		return 0
	}
	return len(chain.queue)
}

func (m *AgentChainManager) runChain(agentID string, chain *agentChain) {
	defer close(chain.done)
	for msg := range chain.queue {
		func() {
			defer func() {
				if r := recover(); r != nil {
					m.logger.Error("agent chain handler panic",
						"agent_id", agentID, "panic", r)
				}
			}()
			m.handler(context.Background(), msg)
		}()
	}
}
