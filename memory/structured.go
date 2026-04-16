// JSONL 结构化记忆存储：与现有 Markdown 文件并存，
// Markdown 作为人可读视图，JSONL 作为结构化查询后端。

package memory

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// StructuredMemory JSONL 结构化记忆存储。
type StructuredMemory struct {
	filePath string
	mu       sync.RWMutex
	entries  []MemoryEntry // 内存缓存
	nextID   int
}

// NewStructuredMemory 创建结构化记忆存储。
// filePath 为 JSONL 文件路径（如 ~/.simpleclaw/workspace/memory/entries.jsonl）。
func NewStructuredMemory(filePath string) (*StructuredMemory, error) {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create dir: %w", err)
	}

	sm := &StructuredMemory{
		filePath: filePath,
		nextID:   1,
	}

	// 加载已有数据
	if err := sm.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load entries: %w", err)
	}

	return sm, nil
}

// Store 存储一条记忆。
func (sm *StructuredMemory) Store(key, content string, category MemoryCategory) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	score := CategoryInitialScore[category]
	entry := MemoryEntry{
		ID:        fmt.Sprintf("mem_%d", sm.nextID),
		Key:       key,
		Content:   content,
		Category:  category,
		Timestamp: time.Now().Format(time.RFC3339),
		Score:     &score,
	}
	sm.nextID++

	sm.entries = append(sm.entries, entry)
	return sm.appendToFile(entry)
}

// Upsert 按 key 更新已有记忆，不存在则新增。
func (sm *StructuredMemory) Upsert(key, content string, category MemoryCategory) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for i, e := range sm.entries {
		if e.Key == key {
			score := CategoryInitialScore[category]
			sm.entries[i].Content = content
			sm.entries[i].Timestamp = time.Now().Format(time.RFC3339)
			sm.entries[i].Score = &score
			return sm.rewrite()
		}
	}

	// 新增
	score := CategoryInitialScore[category]
	entry := MemoryEntry{
		ID:        fmt.Sprintf("mem_%d", sm.nextID),
		Key:       key,
		Content:   content,
		Category:  category,
		Timestamp: time.Now().Format(time.RFC3339),
		Score:     &score,
	}
	sm.nextID++
	sm.entries = append(sm.entries, entry)
	return sm.appendToFile(entry)
}

// Delete 按 ID 删除记忆。
func (sm *StructuredMemory) Delete(id string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for i, e := range sm.entries {
		if e.ID == id {
			sm.entries = append(sm.entries[:i], sm.entries[i+1:]...)
			return sm.rewrite()
		}
	}
	return nil
}

// Query 按类别查询记忆，返回最多 limit 条（按分数降序）。
// categories 为空则返回所有类别。
func (sm *StructuredMemory) Query(categories []MemoryCategory, limit int) []MemoryEntry {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	catSet := make(map[MemoryCategory]bool, len(categories))
	for _, c := range categories {
		catSet[c] = true
	}

	var result []MemoryEntry
	for _, e := range sm.entries {
		if len(catSet) > 0 && !catSet[e.Category] {
			continue
		}
		result = append(result, e)
	}

	// 按分数降序排序
	sort.Slice(result, func(i, j int) bool {
		si, sj := 0.0, 0.0
		if result[i].Score != nil {
			si = *result[i].Score
		}
		if result[j].Score != nil {
			sj = *result[j].Score
		}
		return si > sj
	})

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result
}

// All 返回所有记忆条目（副本）。
func (sm *StructuredMemory) All() []MemoryEntry {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	cp := make([]MemoryEntry, len(sm.entries))
	copy(cp, sm.entries)
	return cp
}

// Len 返回记忆条目数。
func (sm *StructuredMemory) Len() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.entries)
}

// Prune 移除分数低于 threshold 的非 Core 条目。
func (sm *StructuredMemory) Prune(threshold float64) int {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	var kept []MemoryEntry
	pruned := 0
	for _, e := range sm.entries {
		if e.Category == CategoryCore {
			kept = append(kept, e)
			continue
		}
		score := 0.0
		if e.Score != nil {
			score = *e.Score
		}
		if score >= threshold {
			kept = append(kept, e)
		} else {
			pruned++
		}
	}

	if pruned > 0 {
		sm.entries = kept
		sm.rewrite()
	}
	return pruned
}

// ============ 文件 IO ============

func (sm *StructuredMemory) load() error {
	f, err := os.Open(sm.filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024) // 1MB buffer

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry MemoryEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue // 跳过损坏行
		}
		sm.entries = append(sm.entries, entry)

		// 维护 nextID
		var id int
		if _, err := fmt.Sscanf(entry.ID, "mem_%d", &id); err == nil && id >= sm.nextID {
			sm.nextID = id + 1
		}
	}
	return scanner.Err()
}

func (sm *StructuredMemory) appendToFile(entry MemoryEntry) error {
	f, err := os.OpenFile(sm.filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = f.Write(data)
	return err
}

func (sm *StructuredMemory) rewrite() error {
	f, err := os.Create(sm.filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, e := range sm.entries {
		data, err := json.Marshal(e)
		if err != nil {
			continue
		}
		w.Write(data)
		w.WriteByte('\n')
	}
	return w.Flush()
}
