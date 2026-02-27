// 记忆存储：管理长期记忆 (MEMORY.md) 和每日笔记 (YYYY-MM-DD.md)。

package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleClaw/utils"
)

// MemoryStore 记忆管理系统
// 支持长期记忆 (MEMORY.md) 和每日笔记 (YYYY-MM-DD.md)
type MemoryStore struct {
	workspace  string
	memoryDir  string
	memoryFile string
}

// NewMemoryStore 创建记忆存储
func NewMemoryStore(workspace string) *MemoryStore {
	memoryDir := filepath.Join(workspace, "memory")
	_ = os.MkdirAll(memoryDir, 0755)
	return &MemoryStore{
		workspace:  workspace,
		memoryDir:  memoryDir,
		memoryFile: filepath.Join(memoryDir, "MEMORY.md"),
	}
}

// ============ 长期记忆 ============

// ReadLongTerm 读取长期记忆
func (m *MemoryStore) ReadLongTerm() string {
	content, err := os.ReadFile(m.memoryFile)
	if err != nil {
		return ""
	}
	return string(content)
}

// WriteLongTerm 写入长期记忆
func (m *MemoryStore) WriteLongTerm(content string) error {
	_ = os.MkdirAll(m.memoryDir, 0755)
	return os.WriteFile(m.memoryFile, []byte(content), 0644)
}

// ============ 每日笔记 ============

// GetTodayFile 获取今天的记忆文件路径
func (m *MemoryStore) GetTodayFile() string {
	return filepath.Join(m.memoryDir, utils.TodayDate()+".md")
}

// ReadToday 读取今天的笔记
func (m *MemoryStore) ReadToday() string {
	content, err := os.ReadFile(m.GetTodayFile())
	if err != nil {
		return ""
	}
	return string(content)
}

// AppendToday 追加内容到今天的笔记
func (m *MemoryStore) AppendToday(content string) error {
	todayFile := m.GetTodayFile()

	existing, err := os.ReadFile(todayFile)
	if err == nil {
		content = string(existing) + "\n" + content
	} else {
		// 新建文件，添加日期头
		content = fmt.Sprintf("# %s\n\n%s", utils.TodayDate(), content)
	}

	return os.WriteFile(todayFile, []byte(content), 0644)
}

// ============ 近期记忆 ============

// GetRecentMemories 获取最近 N 天的记忆
func (m *MemoryStore) GetRecentMemories(days int) string {
	var memories []string
	today := time.Now()

	for i := 0; i < days; i++ {
		date := today.AddDate(0, 0, -i)
		dateStr := date.Format("2006-01-02")
		filePath := filepath.Join(m.memoryDir, dateStr+".md")

		content, err := os.ReadFile(filePath)
		if err == nil {
			memories = append(memories, string(content))
		}
	}

	return strings.Join(memories, "\n\n---\n\n")
}

// ListMemoryFiles 列出所有记忆文件（最新在前）
func (m *MemoryStore) ListMemoryFiles() []string {
	if _, err := os.Stat(m.memoryDir); os.IsNotExist(err) {
		return nil
	}

	matches, _ := filepath.Glob(filepath.Join(m.memoryDir, "????-??-??.md"))
	sort.Sort(sort.Reverse(sort.StringSlice(matches)))
	return matches
}

// ============ 上下文生成 ============

// GetMemoryContext 获取记忆上下文（用于 system prompt）
func (m *MemoryStore) GetMemoryContext() string {
	var parts []string

	// 长期记忆
	longTerm := m.ReadLongTerm()
	if longTerm != "" {
		parts = append(parts, "## Long-term Memory\n"+longTerm)
	}

	// 今日笔记
	today := m.ReadToday()
	if today != "" {
		parts = append(parts, "## Today's Notes\n"+today)
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n")
}
