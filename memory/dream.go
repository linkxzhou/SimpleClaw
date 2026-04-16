// Dream 两阶段记忆整合服务：
// Phase 1 — 分析：LLM 分析新对话，提取事实/偏好/过时信息
// Phase 2 — 编辑：将分析结果写入结构化记忆和 MEMORY.md
//
// 由 Cron 每小时触发，通过 cursor 增量处理。

package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// DreamLLMProvider Dream 使用的 LLM 调用接口（避免循环依赖 agent 包）。
type DreamLLMProvider interface {
	ChatSimple(ctx context.Context, systemPrompt, userPrompt, model string) (string, error)
}

// DreamConfig Dream 服务配置。
type DreamConfig struct {
	Enabled   bool   `json:"enabled"`
	Model     string `json:"model,omitempty"`    // 默认使用廉价模型
	BatchSize int    `json:"batchSize,omitempty"` // 每批处理条数，默认 20
}

// DreamService 两阶段记忆整合服务。
type DreamService struct {
	store     *MemoryStore
	history   *HistoryStore
	memory    *StructuredMemory
	provider  DreamLLMProvider
	config    DreamConfig
	logger    *slog.Logger
}

// NewDreamService 创建 Dream 服务。
func NewDreamService(
	store *MemoryStore,
	history *HistoryStore,
	memory *StructuredMemory,
	provider DreamLLMProvider,
	config DreamConfig,
	logger *slog.Logger,
) *DreamService {
	if logger == nil {
		logger = slog.Default()
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 20
	}
	if config.Model == "" {
		config.Model = "gpt-4o-mini"
	}
	return &DreamService{
		store:    store,
		history:  history,
		memory:   memory,
		provider: provider,
		config:   config,
		logger:   logger,
	}
}

// DreamResult Dream 运行结果。
type DreamResult struct {
	Processed int    // 处理的历史条数
	NewFacts  int    // 新增/更新的记忆条数
	Cursor    int64  // 新游标位置
	Error     error  // 错误（nil = 成功）
}

// Run 执行一次 Dream 整合。返回是否有新内容被处理。
func (d *DreamService) Run(ctx context.Context) (*DreamResult, error) {
	cursor := d.history.GetDreamCursor()

	// 读取新对话
	entries, err := d.history.ReadSince(cursor)
	if err != nil {
		return nil, fmt.Errorf("read history: %w", err)
	}

	if len(entries) == 0 {
		d.logger.Debug("dream: no new entries")
		return &DreamResult{Cursor: cursor}, nil
	}

	// 按 batch 截取
	if len(entries) > d.config.BatchSize {
		entries = entries[:d.config.BatchSize]
	}

	d.logger.Info("dream: processing entries",
		"count", len(entries),
		"cursor_from", cursor,
		"cursor_to", entries[len(entries)-1].Cursor,
	)

	// Phase 1: 分析
	analysis, err := d.phase1Analyze(ctx, entries)
	if err != nil {
		d.logger.Warn("dream phase1 failed", "error", err)
		return &DreamResult{Error: fmt.Errorf("phase1: %w", err)}, nil
	}

	// Phase 2: 编辑记忆
	newFacts, err := d.phase2Edit(analysis)
	if err != nil {
		d.logger.Warn("dream phase2 failed, advancing cursor anyway", "error", err)
	}

	// 推进游标（Phase 2 失败也推进，避免重复分析）
	newCursor := entries[len(entries)-1].Cursor
	if err := d.history.SetDreamCursor(newCursor); err != nil {
		d.logger.Warn("dream: failed to set cursor", "error", err)
	}

	return &DreamResult{
		Processed: len(entries),
		NewFacts:  newFacts,
		Cursor:    newCursor,
	}, nil
}

// ============ Phase 1: 分析 ============

// phase1AnalysisResult Phase 1 LLM 输出格式。
type phase1AnalysisResult struct {
	NewFacts []struct {
		Key      string `json:"key"`
		Content  string `json:"content"`
		Category string `json:"category"` // "core", "procedural", "daily"
	} `json:"new_facts"`
	Obsolete []string `json:"obsolete"` // 需要删除的 key 列表
}

const phase1SystemPrompt = `You are a memory consolidation engine. Analyze the recent conversation history and current memory state, then output a JSON object:

{
  "new_facts": [
    {"key": "short_identifier", "content": "the fact or preference", "category": "core|procedural|daily"}
  ],
  "obsolete": ["key_to_remove"]
}

Rules:
- "core": permanent facts (user name, important decisions, key preferences)
- "procedural": workflow preferences, tool usage patterns
- "daily": daily conversation summaries
- Only extract genuinely new or updated information
- Mark obsolete keys that are contradicted by new info
- If nothing new, return {"new_facts": [], "obsolete": []}
- Output ONLY valid JSON, no markdown fences`

func (d *DreamService) phase1Analyze(ctx context.Context, entries []HistoryEntry) (*phase1AnalysisResult, error) {
	// 格式化对话历史
	var historyParts []string
	for _, e := range entries {
		historyParts = append(historyParts, fmt.Sprintf("[%s] User: %s\nAssistant: %s",
			e.Timestamp, truncDream(e.UserMsg, 300), truncDream(e.AssistMsg, 300)))
	}

	// 读取当前记忆状态
	currentMemory := d.store.ReadLongTerm()
	var structuredParts []string
	if d.memory != nil {
		for _, e := range d.memory.All() {
			structuredParts = append(structuredParts, fmt.Sprintf("[%s] %s: %s", e.Category, e.Key, e.Content))
		}
	}

	userPrompt := fmt.Sprintf("## Recent Conversations\n%s\n\n## Current MEMORY.md\n%s\n\n## Structured Memory\n%s",
		strings.Join(historyParts, "\n\n"),
		truncDream(currentMemory, 2000),
		strings.Join(structuredParts, "\n"),
	)

	resp, err := d.provider.ChatSimple(ctx, phase1SystemPrompt, userPrompt, d.config.Model)
	if err != nil {
		return nil, err
	}

	// 解析 JSON
	resp = strings.TrimSpace(resp)
	// 去除可能的 markdown fences
	resp = strings.TrimPrefix(resp, "```json")
	resp = strings.TrimPrefix(resp, "```")
	resp = strings.TrimSuffix(resp, "```")
	resp = strings.TrimSpace(resp)

	var result phase1AnalysisResult
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		return nil, fmt.Errorf("parse analysis: %w (response: %.200s)", err, resp)
	}

	return &result, nil
}

// ============ Phase 2: 编辑记忆 ============

func (d *DreamService) phase2Edit(analysis *phase1AnalysisResult) (int, error) {
	if analysis == nil {
		return 0, nil
	}

	newFacts := 0

	// 删除过时记忆
	if d.memory != nil {
		for _, key := range analysis.Obsolete {
			for _, e := range d.memory.All() {
				if e.Key == key {
					d.memory.Delete(e.ID)
					d.logger.Info("dream: removed obsolete memory", "key", key)
					break
				}
			}
		}
	}

	// 写入新事实
	for _, fact := range analysis.NewFacts {
		cat := CategoryDaily // 默认
		switch fact.Category {
		case "core":
			cat = CategoryCore
		case "procedural":
			cat = CategoryProcedural
		case "daily":
			cat = CategoryDaily
		}

		if d.memory != nil {
			if err := d.memory.Upsert(fact.Key, fact.Content, cat); err != nil {
				d.logger.Warn("dream: failed to upsert", "key", fact.Key, "error", err)
				continue
			}
		}
		newFacts++
		d.logger.Info("dream: upserted memory", "key", fact.Key, "category", cat)
	}

	// 同步到 MEMORY.md（人可读视图）
	if newFacts > 0 {
		d.syncToMarkdown()
	}

	return newFacts, nil
}

// syncToMarkdown 将结构化记忆同步到 MEMORY.md。
func (d *DreamService) syncToMarkdown() {
	if d.memory == nil {
		return
	}

	entries := d.memory.All()
	ApplyTimeDecay(entries)
	SortByScore(entries)

	var sections = map[MemoryCategory][]string{
		CategoryCore:         nil,
		CategoryProcedural:   nil,
		CategoryDaily:        nil,
		CategoryConversation: nil,
	}

	for _, e := range entries {
		dateStr := ""
		if len(e.Timestamp) >= 10 {
			dateStr = e.Timestamp[:10]
		}
		line := fmt.Sprintf("- [%s] %s", dateStr, e.Content)
		sections[e.Category] = append(sections[e.Category], line)
	}

	var md strings.Builder
	md.WriteString(fmt.Sprintf("# Memory\n\n_Auto-generated by Dream at %s_\n\n", time.Now().Format("2006-01-02 15:04")))

	if items := sections[CategoryCore]; len(items) > 0 {
		md.WriteString("## Core Facts\n\n")
		md.WriteString(strings.Join(items, "\n") + "\n\n")
	}
	if items := sections[CategoryProcedural]; len(items) > 0 {
		md.WriteString("## Procedures & Preferences\n\n")
		md.WriteString(strings.Join(items, "\n") + "\n\n")
	}
	if items := sections[CategoryDaily]; len(items) > 0 {
		md.WriteString("## Daily Notes\n\n")
		md.WriteString(strings.Join(items, "\n") + "\n\n")
	}

	d.store.WriteLongTerm(md.String())
}

// ============ 辅助 ============

func truncDream(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
