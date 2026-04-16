// HistoryStore 对话历史追加存储：每轮对话结束后追加一条记录到 history.jsonl，
// 通过 cursor 机制让 Dream 服务增量消费新对话。

package memory

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// HistoryEntry 一条对话历史记录。
type HistoryEntry struct {
	Cursor     int64  `json:"cursor"`      // 自增游标
	Timestamp  string `json:"timestamp"`   // RFC3339
	SessionKey string `json:"session_key"`
	UserMsg    string `json:"user_msg"`    // 用户消息（截断）
	AssistMsg  string `json:"assist_msg"`  // 助手回复（截断）
	Summary    string `json:"summary"`     // 简要摘要
}

// HistoryStore 管理 history.jsonl 和 dream cursor。
type HistoryStore struct {
	filePath   string
	cursorFile string // .dream_cursor
	mu         sync.Mutex
	nextCursor int64
}

// NewHistoryStore 创建历史存储。
func NewHistoryStore(memoryDir string) (*HistoryStore, error) {
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		return nil, err
	}

	hs := &HistoryStore{
		filePath:   filepath.Join(memoryDir, "history.jsonl"),
		cursorFile: filepath.Join(memoryDir, ".dream_cursor"),
		nextCursor: 1,
	}

	// 计算下一个 cursor
	if entries, err := hs.readAll(); err == nil && len(entries) > 0 {
		hs.nextCursor = entries[len(entries)-1].Cursor + 1
	}

	return hs, nil
}

// Append 追加一条对话记录，自动分配 cursor。
func (h *HistoryStore) Append(entry HistoryEntry) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	entry.Cursor = h.nextCursor
	h.nextCursor++

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	f, err := os.OpenFile(h.filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

// ReadSince 读取 cursor 之后的所有条目。
func (h *HistoryStore) ReadSince(cursor int64) ([]HistoryEntry, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	all, err := h.readAll()
	if err != nil {
		return nil, err
	}

	var result []HistoryEntry
	for _, e := range all {
		if e.Cursor > cursor {
			result = append(result, e)
		}
	}
	return result, nil
}

// GetDreamCursor 获取 Dream 服务上次处理到的 cursor。
func (h *HistoryStore) GetDreamCursor() int64 {
	data, err := os.ReadFile(h.cursorFile)
	if err != nil {
		return 0
	}
	var cursor int64
	fmt.Sscanf(string(data), "%d", &cursor)
	return cursor
}

// SetDreamCursor 设置 Dream 服务的 cursor。
func (h *HistoryStore) SetDreamCursor(cursor int64) error {
	return os.WriteFile(h.cursorFile, []byte(fmt.Sprintf("%d", cursor)), 0o644)
}

// Len 返回总记录数。
func (h *HistoryStore) Len() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	entries, _ := h.readAll()
	return len(entries)
}

// Compact 压缩历史文件，仅保留最近 keepLast 条。
func (h *HistoryStore) Compact(keepLast int) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	all, err := h.readAll()
	if err != nil || len(all) <= keepLast {
		return err
	}

	kept := all[len(all)-keepLast:]
	return h.rewrite(kept)
}

// ============ 内部方法 ============

func (h *HistoryStore) readAll() ([]HistoryEntry, error) {
	f, err := os.Open(h.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var entries []HistoryEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 256*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e HistoryEntry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, scanner.Err()
}

func (h *HistoryStore) rewrite(entries []HistoryEntry) error {
	f, err := os.Create(h.filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, e := range entries {
		data, _ := json.Marshal(e)
		w.Write(data)
		w.WriteByte('\n')
	}
	return w.Flush()
}
