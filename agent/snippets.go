// SnippetStore 管理可复用的 Go 代码片段。
// 代码片段保存为 JSON 文件，按 ID 索引，支持按名称/标签搜索和使用频率排序。
// 片段可通过 go_snippet 工具保存、列出、执行和删除。
package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Snippet 可复用的代码片段。
type Snippet struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Code        string    `json:"code"`
	FuncName    string    `json:"func_name"`
	Tags        []string  `json:"tags"`
	UseCount    int       `json:"use_count"`
	LastUsed    time.Time `json:"last_used"`
	CreatedAt   time.Time `json:"created_at"`
}

// SnippetStore 管理代码片段的持久化存储。
type SnippetStore struct {
	dir      string
	snippets map[string]*Snippet
	mu       sync.RWMutex
}

// NewSnippetStore 创建或加载片段存储。dir 为持久化目录。
func NewSnippetStore(dir string) (*SnippetStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create snippet dir: %w", err)
	}
	s := &SnippetStore{
		dir:      dir,
		snippets: make(map[string]*Snippet),
	}
	if err := s.loadAll(); err != nil {
		return nil, err
	}
	return s, nil
}

// Save 保存片段到内存和磁盘。
func (s *SnippetStore) Save(snippet *Snippet) error {
	if snippet.ID == "" {
		return fmt.Errorf("snippet ID is required")
	}
	if snippet.Code == "" {
		return fmt.Errorf("snippet Code is required")
	}
	if snippet.CreatedAt.IsZero() {
		snippet.CreatedAt = time.Now()
	}
	if snippet.FuncName == "" {
		snippet.FuncName = "Run"
	}

	s.mu.Lock()
	s.snippets[snippet.ID] = snippet
	s.mu.Unlock()

	return s.persist(snippet)
}

// Get 按 ID 获取片段。
func (s *SnippetStore) Get(id string) (*Snippet, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sn, ok := s.snippets[id]
	return sn, ok
}

// GetByName 按名称精确匹配获取片段。
func (s *SnippetStore) GetByName(name string) (*Snippet, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	lower := strings.ToLower(name)
	for _, sn := range s.snippets {
		if strings.ToLower(sn.Name) == lower {
			return sn, true
		}
	}
	return nil, false
}

// Delete 按 ID 删除片段。
func (s *SnippetStore) Delete(id string) error {
	s.mu.Lock()
	delete(s.snippets, id)
	s.mu.Unlock()
	return os.Remove(filepath.Join(s.dir, id+".json"))
}

// Search 按 query 模糊匹配名称、描述和标签。
func (s *SnippetStore) Search(query string) []*Snippet {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if query == "" {
		return s.allSorted()
	}

	lower := strings.ToLower(query)
	var results []*Snippet
	for _, sn := range s.snippets {
		if matchSnippet(sn, lower) {
			results = append(results, sn)
		}
	}
	sortByUsage(results)
	return results
}

// ListByUsage 按使用频率降序返回前 limit 个片段。limit <= 0 返回全部。
func (s *SnippetStore) ListByUsage(limit int) []*Snippet {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := s.allSorted()
	if limit > 0 && limit < len(all) {
		return all[:limit]
	}
	return all
}

// RecordUse 记录片段被使用一次。
func (s *SnippetStore) RecordUse(id string) {
	s.mu.Lock()
	sn, ok := s.snippets[id]
	if ok {
		sn.UseCount++
		sn.LastUsed = time.Now()
	}
	s.mu.Unlock()
	if ok {
		_ = s.persist(sn)
	}
}

// Len 返回片段总数。
func (s *SnippetStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.snippets)
}

// ---------- SnippetProvider 接口适配（供 tools.GoSnippetTool 使用） ----------
//
// tools 包定义 SnippetProvider 接口和自己的 Snippet 类型以避免循环依赖。
// SnippetStoreAdapter 桥接 agent.SnippetStore → tools.SnippetProvider。
// 在 agent 包内完成适配，因为只有 agent 包可以同时访问自身类型和 tools 类型。

// BuildSummary 生成供 system prompt 注入的片段摘要。
func (s *SnippetStore) BuildSummary(limit int) string {
	snippets := s.ListByUsage(limit)
	if len(snippets) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Available Code Snippets\n\n")
	for _, sn := range snippets {
		sb.WriteString(fmt.Sprintf("- `%s` — %s", sn.Name, sn.Description))
		if sn.UseCount > 0 {
			sb.WriteString(fmt.Sprintf(" (used %d times)", sn.UseCount))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\nUse go_snippet(action:\"run\", name:\"<name>\", input:{...}) to execute saved snippets.\n")
	return sb.String()
}

// ---------- internal ----------

func (s *SnippetStore) loadAll() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue
		}
		var sn Snippet
		if json.Unmarshal(data, &sn) == nil && sn.ID != "" {
			s.snippets[sn.ID] = &sn
		}
	}
	return nil
}

func (s *SnippetStore) persist(sn *Snippet) error {
	data, err := json.MarshalIndent(sn, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, sn.ID+".json"), data, 0o644)
}

func (s *SnippetStore) allSorted() []*Snippet {
	all := make([]*Snippet, 0, len(s.snippets))
	for _, sn := range s.snippets {
		all = append(all, sn)
	}
	sortByUsage(all)
	return all
}

func matchSnippet(sn *Snippet, lowerQuery string) bool {
	if strings.Contains(strings.ToLower(sn.Name), lowerQuery) {
		return true
	}
	if strings.Contains(strings.ToLower(sn.Description), lowerQuery) {
		return true
	}
	for _, tag := range sn.Tags {
		if strings.Contains(strings.ToLower(tag), lowerQuery) {
			return true
		}
	}
	return false
}

func sortByUsage(snippets []*Snippet) {
	sort.Slice(snippets, func(i, j int) bool {
		if snippets[i].UseCount != snippets[j].UseCount {
			return snippets[i].UseCount > snippets[j].UseCount
		}
		return snippets[i].LastUsed.After(snippets[j].LastUsed)
	})
}
