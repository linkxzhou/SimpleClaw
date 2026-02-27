// 状态管理：原子持久化的系统状态。
// 追踪最近活跃的 Channel/ChatID，供心跳服务和设备事件使用。
// 使用 temp file + rename 模式保证写入的原子性（POSIX 安全）。

package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// State 持久化的系统状态。
type State struct {
	LastChannel string    `json:"last_channel"` // 最近活跃渠道
	LastChatID  string    `json:"last_chat_id"` // 最近活跃聊天 ID
	Timestamp   time.Time `json:"timestamp"`    // 最后更新时间
}

// StateManager 管理系统状态的原子持久化。
type StateManager struct {
	mu       sync.RWMutex
	filePath string
	state    State
}

// NewStateManager 创建状态管理器并加载已有状态。
func NewStateManager(dataDir string) *StateManager {
	fp := filepath.Join(dataDir, "state", "state.json")

	m := &StateManager{filePath: fp}
	m.load()
	return m
}

// Update 更新最近活跃的 Channel 和 ChatID。
func (m *StateManager) Update(channel, chatID string) {
	m.mu.Lock()
	m.state.LastChannel = channel
	m.state.LastChatID = chatID
	m.state.Timestamp = time.Now()
	m.mu.Unlock()

	go m.save() // 异步保存
}

// Get 返回当前状态的快照。
func (m *StateManager) Get() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// GetLastActive 返回最近活跃的 channel 和 chatID。
func (m *StateManager) GetLastActive() (string, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.LastChannel, m.state.LastChatID
}

// load 从磁盘加载状态。
func (m *StateManager) load() {
	data, err := os.ReadFile(m.filePath)
	if err != nil {
		return // 文件不存在或读取失败，使用零值
	}
	_ = json.Unmarshal(data, &m.state)
}

// save 原子保存状态到磁盘（temp file + rename）。
func (m *StateManager) save() {
	m.mu.RLock()
	data, err := json.MarshalIndent(m.state, "", "  ")
	m.mu.RUnlock()

	if err != nil {
		return
	}

	// 确保目录存在
	dir := filepath.Dir(m.filePath)
	_ = os.MkdirAll(dir, 0755)

	// 原子写入：先写 temp 文件，再 rename
	tmpPath := m.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return
	}
	_ = os.Rename(tmpPath, m.filePath)
}
