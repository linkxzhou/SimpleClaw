// 费用记录的 JSONL 持久化存储。
// 按日分文件（costs/2026-04-15.jsonl），支持缓存聚合值避免全量扫描。

package providers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CostStorage 基于 JSONL 的费用存储。
type CostStorage struct {
	dir string // 存储目录（如 ~/.simpleclaw/costs/）
	mu  sync.Mutex

	// 缓存当日和当月聚合值
	dailyCache   float64
	monthlyCache float64
	cacheDate    string // "2026-04-15" — 缓存对应的日期
}

// NewCostStorage 创建费用存储，自动创建目录。
func NewCostStorage(dir string) (*CostStorage, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create cost dir: %w", err)
	}
	return &CostStorage{dir: dir}, nil
}

// AddRecord 追加一条费用记录到当日 JSONL 文件。
func (s *CostStorage) AddRecord(record CostRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	today := time.Now().Format("2006-01-02")

	// 日期变化时重置缓存
	if s.cacheDate != today {
		s.refreshCacheLocked(today)
	}

	filename := today + ".jsonl"
	f, err := os.OpenFile(filepath.Join(s.dir, filename), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open cost file: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal cost record: %w", err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write cost record: %w", err)
	}

	s.dailyCache += record.Usage.CostUSD
	s.monthlyCache += record.Usage.CostUSD
	return nil
}

// GetDailyCost 返回当日累计费用。
func (s *CostStorage) GetDailyCost() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	if s.cacheDate != today {
		s.refreshCacheLocked(today)
	}
	return s.dailyCache
}

// GetMonthlyCost 返回当月累计费用。
func (s *CostStorage) GetMonthlyCost() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	if s.cacheDate != today {
		s.refreshCacheLocked(today)
	}
	return s.monthlyCache
}

// GetModelBreakdown 返回当月各模型费用明细。
func (s *CostStorage) GetModelBreakdown() map[string]float64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	breakdown := make(map[string]float64)
	monthPrefix := time.Now().Format("2006-01")

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return breakdown
	}

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, monthPrefix) || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		records := s.readFile(filepath.Join(s.dir, name))
		for _, r := range records {
			breakdown[r.Usage.Model] += r.Usage.CostUSD
		}
	}
	return breakdown
}

// refreshCacheLocked 重新计算缓存（调用者需持有锁）。
func (s *CostStorage) refreshCacheLocked(today string) {
	s.cacheDate = today
	s.dailyCache = 0
	s.monthlyCache = 0

	// 扫描当日文件
	dailyFile := filepath.Join(s.dir, today+".jsonl")
	for _, r := range s.readFile(dailyFile) {
		s.dailyCache += r.Usage.CostUSD
	}

	// 扫描当月所有文件
	monthPrefix := today[:7] // "2026-04"
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, monthPrefix) || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		if name == today+".jsonl" {
			s.monthlyCache += s.dailyCache
			continue
		}
		for _, r := range s.readFile(filepath.Join(s.dir, name)) {
			s.monthlyCache += r.Usage.CostUSD
		}
	}
}

// readFile 读取 JSONL 文件中的所有记录。
func (s *CostStorage) readFile(path string) []CostRecord {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var records []CostRecord
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var r CostRecord
		if err := json.Unmarshal(scanner.Bytes(), &r); err == nil {
			records = append(records, r)
		}
	}
	return records
}
