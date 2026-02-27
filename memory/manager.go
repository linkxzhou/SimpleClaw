// 会话管理器：管理对话会话的生命周期，提供 JSONL 格式的持久化。
// JSONL 文件格式：第一行为元数据（_type: metadata），后续行为消息记录。
// 支持创建、加载、保存、删除和列出会话。

package memory

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleClaw/utils"
)

// SessionManager 管理对话会话，提供 JSONL 持久化。
type SessionManager struct {
	sessionsDir string              // 会话文件存储目录
	cache       map[string]*Session // 内存缓存（按会话键索引）
	mu          sync.RWMutex        // 保护 cache 的并发访问
	logger      *slog.Logger        // 日志记录器
}

// NewSessionManager 创建一个新的会话管理器。
// 自动创建会话存储目录。
func NewSessionManager(sessionsDir string, logger *slog.Logger) (*SessionManager, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create sessions dir: %w", err)
	}
	return &SessionManager{
		sessionsDir: sessionsDir,
		cache:       make(map[string]*Session),
		logger:      logger,
	}, nil
}

// GetOrCreate 获取已有会话或创建新会话。
// 优先从内存缓存获取，其次从磁盘加载，最后创建新会话。
func (m *SessionManager) GetOrCreate(key string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.cache[key]; ok {
		return s
	}

	s := m.load(key)
	if s == nil {
		s = NewSession(key)
	}
	m.cache[key] = s
	return s
}

// Save 将会话持久化到磁盘（JSONL 格式）。
// 第一行写入元数据，后续行写入消息记录。
func (m *SessionManager) Save(s *Session) error {
	path := m.sessionPath(s.Key)

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create session file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)

	// 先写入元数据行
	meta := map[string]interface{}{
		"_type":      "metadata",
		"created_at": s.CreatedAt.Format(time.RFC3339),
		"updated_at": s.UpdatedAt.Format(time.RFC3339),
		"metadata":   s.Metadata,
	}
	if err := enc.Encode(meta); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}

	// 写入消息记录
	for _, msg := range s.Messages {
		if err := enc.Encode(msg); err != nil {
			return fmt.Errorf("write message: %w", err)
		}
	}

	m.mu.Lock()
	m.cache[s.Key] = s
	m.mu.Unlock()

	return nil
}

// Delete 从缓存和磁盘删除会话。
func (m *SessionManager) Delete(key string) bool {
	m.mu.Lock()
	delete(m.cache, key)
	m.mu.Unlock()

	path := m.sessionPath(key)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return false
		}
		m.logger.Warn("failed to delete session file", "key", key, "error", err)
		return false
	}
	return true
}

// ListSessions 返回所有已存储会话的摘要信息，按更新时间倒序排列。
func (m *SessionManager) ListSessions() []SessionInfo {
	entries, err := os.ReadDir(m.sessionsDir)
	if err != nil {
		m.logger.Warn("failed to list sessions dir", "error", err)
		return nil
	}

	var sessions []SessionInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		info := m.readSessionInfo(filepath.Join(m.sessionsDir, entry.Name()))
		if info != nil {
			sessions = append(sessions, *info)
		}
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt > sessions[j].UpdatedAt
	})
	return sessions
}

// SessionInfo 包含会话的摘要信息。
type SessionInfo struct {
	Key       string `json:"key"`        // 会话键
	CreatedAt string `json:"created_at"` // 创建时间
	UpdatedAt string `json:"updated_at"` // 最后更新时间
	Path      string `json:"path"`       // 文件路径
}

// ---- 内部方法 ----

// sessionPath 将会话键转换为文件路径。
// 将冒号替换为下划线，并确保文件名安全。
func (m *SessionManager) sessionPath(key string) string {
	safe := utils.SafeFilename(strings.ReplaceAll(key, ":", "_"))
	return filepath.Join(m.sessionsDir, safe+".jsonl")
}

// load 从磁盘加载会话。
// 解析 JSONL 文件：第一行元数据，后续行消息。
func (m *SessionManager) load(key string) *Session {
	path := m.sessionPath(key)
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var messages []map[string]interface{}
	var metadata map[string]interface{}
	var createdAt time.Time

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB 缓冲区
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(line), &data); err != nil {
			continue
		}
		if data["_type"] == "metadata" {
			if md, ok := data["metadata"].(map[string]interface{}); ok {
				metadata = md
			}
			if ca, ok := data["created_at"].(string); ok {
				if t, err := time.Parse(time.RFC3339, ca); err == nil {
					createdAt = t
				}
			}
		} else {
			messages = append(messages, data)
		}
	}

	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	return &Session{
		Key:       key,
		Messages:  messages,
		CreatedAt: createdAt,
		UpdatedAt: time.Now(),
		Metadata:  metadata,
	}
}

// readSessionInfo 从 JSONL 文件的第一行读取会话摘要信息。
func (m *SessionManager) readSessionInfo(path string) *SessionInfo {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return nil
	}
	line := strings.TrimSpace(scanner.Text())
	if line == "" {
		return nil
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(line), &data); err != nil {
		return nil
	}
	if data["_type"] != "metadata" {
		return nil
	}

	name := filepath.Base(path)
	key := strings.TrimSuffix(name, ".jsonl")
	key = strings.ReplaceAll(key, "_", ":")

	ca, _ := data["created_at"].(string)
	ua, _ := data["updated_at"].(string)

	return &SessionInfo{
		Key:       key,
		CreatedAt: ca,
		UpdatedAt: ua,
		Path:      path,
	}
}

