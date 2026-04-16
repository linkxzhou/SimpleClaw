package agent

import (
	"encoding/base64"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleClaw/memory"
)

// ContextBuilder 上下文构建器
// 负责组装 system prompt + 对话消息
type ContextBuilder struct {
	workspace          string
	memory             *memory.MemoryStore
	structuredMemory   *memory.StructuredMemory // 结构化记忆（可选）
	skills             *SkillsLoader
	snippets           *SnippetStore            // 代码片段存储（可选）
	memoryTokenBudget  int // 记忆 token 预算（0 = 不限）
}

// 引导文件列表
var bootstrapFiles = []string{"AGENTS.md", "SOUL.md", "USER.md", "TOOLS.md", "IDENTITY.md"}

// NewContextBuilder 创建上下文构建器
func NewContextBuilder(workspace string) *ContextBuilder {
	memoryDir := filepath.Join(workspace, "memory")
	entriesPath := filepath.Join(memoryDir, "entries.jsonl")
	sm, _ := memory.NewStructuredMemory(entriesPath)

	return &ContextBuilder{
		workspace:         workspace,
		memory:            memory.NewMemoryStore(workspace),
		structuredMemory:  sm,
		skills:            NewSkillsLoader(workspace, ""),
		memoryTokenBudget: 4000, // 默认 4000 token 预算
	}
}

// BuildSystemPrompt 构建完整的 system prompt
func (c *ContextBuilder) BuildSystemPrompt(skillNames ...string) string {
	var parts []string

	// 1. 核心身份
	parts = append(parts, c.getIdentity())

	// 2. 引导文件
	if bootstrap := c.loadBootstrapFiles(); bootstrap != "" {
		parts = append(parts, bootstrap)
	}

	// 3. 记忆上下文（优先使用结构化记忆，回退到 Markdown）
	if memCtx := c.buildMemoryContext(); memCtx != "" {
		parts = append(parts, "# Memory\n\n"+memCtx)
	}

	// 4. 始终加载的技能（完整内容）
	alwaysSkills := c.skills.GetAlwaysSkills()
	if len(alwaysSkills) > 0 {
		if content := c.skills.LoadSkillsForContext(alwaysSkills); content != "" {
			parts = append(parts, "# Active Skills\n\n"+content)
		}
	}

	// 5. 可用技能摘要（渐进式加载）
	if summary := c.skills.BuildSkillsSummary(); summary != "" {
		parts = append(parts, fmt.Sprintf(`# Skills

The following skills extend your capabilities. To use a skill, read its SKILL.md file using the read_file tool.
Skills with available="false" need dependencies installed first - you can try installing them with apt/brew.

%s`, summary))
	}

	// 6. Go 代码生成策略（引导 Agent 主动使用 go_run/go_agent）
	advisor := &GoCodegenAdvisor{}
	parts = append(parts, advisor.BuildPrompt())

	// 7. 代码片段摘要（供 Agent 复用已保存片段）
	if c.snippets != nil {
		if summary := c.snippets.BuildSummary(10); summary != "" {
			parts = append(parts, summary)
		}
	}

	return strings.Join(parts, "\n\n---\n\n")
}

// getIdentity 生成核心身份描述
func (c *ContextBuilder) getIdentity() string {
	now := time.Now().Format("2006-01-02 15:04 (Monday)")
	absWorkspace, _ := filepath.Abs(c.workspace)

	return fmt.Sprintf(`# Agent

You are a helpful AI assistant. You have access to tools that allow you to:
- Read, write, and edit files
- Execute shell commands
- Search the web and fetch web pages
- Send messages to users on chat channels
- Spawn subagents for complex background tasks
- Execute Go code in a sandboxed interpreter (go_run / go_agent)

## Go Code Execution — IMPORTANT

When the user asks you to write, generate, or run Go code, you MUST prefer:
1. **go_run** — execute a Go code snippet directly and return the output. Use this for single-shot tasks (e.g. call an API, compute something, parse data).
2. **go_agent** — launch a long-running Go agent for more complex tasks.

Do NOT write Go code to a file and then use exec/shell to compile and run it. Use go_run instead — it is faster, safer, and does not pollute the workspace with temporary files.

## Current Time
%s

## Workspace
Your workspace is at: %s
- Memory files: %s/memory/MEMORY.md
- Daily notes: %s/memory/YYYY-MM-DD.md
- Custom skills: %s/skills/{skill-name}/SKILL.md

IMPORTANT: When responding to direct questions or conversations, reply directly with your text response.
Only use the 'message' tool when you need to send a message to a specific chat channel.
For normal conversation, just respond with text - do not call the message tool.

Always be helpful, accurate, and concise. When using tools, explain what you're doing.
When remembering something, write to %s/memory/MEMORY.md`, now, absWorkspace, absWorkspace, absWorkspace, absWorkspace, absWorkspace)
}

// buildMemoryContext 构建记忆上下文。
// 优先使用结构化记忆（按分数排序 + token 预算截断），无条目时回退到 Markdown 记忆。
func (c *ContextBuilder) buildMemoryContext() string {
	// 尝试结构化记忆
	if c.structuredMemory != nil && c.structuredMemory.Len() > 0 {
		entries := c.structuredMemory.All()
		memory.ApplyTimeDecay(entries)
		memory.SortByScore(entries)

		selected := memory.SelectByTokenBudget(entries, c.memoryTokenBudget)
		if len(selected) > 0 {
			var parts []string
			for _, e := range selected {
				dateStr := ""
				if len(e.Timestamp) >= 10 {
					dateStr = e.Timestamp[:10]
				}
				parts = append(parts, fmt.Sprintf("[%s|%s] %s", e.Category, dateStr, e.Content))
			}
			return strings.Join(parts, "\n")
		}
	}

	// 回退到 Markdown 记忆
	return c.memory.GetMemoryContext()
}

// GetStructuredMemory 返回结构化记忆存储（供外部写入）。
func (c *ContextBuilder) GetStructuredMemory() *memory.StructuredMemory {
	return c.structuredMemory
}

// SetSnippetStore 设置代码片段存储。
func (c *ContextBuilder) SetSnippetStore(store *SnippetStore) {
	c.snippets = store
}

// GetSnippetStore 返回代码片段存储。
func (c *ContextBuilder) GetSnippetStore() *SnippetStore {
	return c.snippets
}

// loadBootstrapFiles 加载引导文件
func (c *ContextBuilder) loadBootstrapFiles() string {
	var parts []string
	for _, filename := range bootstrapFiles {
		path := filepath.Join(c.workspace, filename)
		data, err := os.ReadFile(path)
		if err == nil {
			parts = append(parts, fmt.Sprintf("## %s\n\n%s", filename, string(data)))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n")
}

// BuildMessages 构建完整的消息列表
func (c *ContextBuilder) BuildMessages(
	history []Message,
	currentMessage string,
	media []MediaItem,
	skillNames ...string,
) []Message {
	var messages []Message

	// System prompt
	systemPrompt := c.BuildSystemPrompt(skillNames...)
	messages = append(messages, Message{Role: "system", Content: systemPrompt})

	// 历史消息
	messages = append(messages, history...)

	// 当前用户消息（可能包含图片附件）
	userContent := c.buildUserContent(currentMessage, media)
	messages = append(messages, Message{Role: "user", Content: userContent})

	return messages
}

// buildUserContent 构建用户消息内容（支持 base64 编码图片）
func (c *ContextBuilder) buildUserContent(text string, media []MediaItem) string {
	if len(media) == 0 {
		return text
	}

	// 将图片编码为 base64 并附加到消息中
	var imageParts []string
	for _, item := range media {
		// 对远程 URL 直接引用
		if strings.HasPrefix(item.URL, "http://") || strings.HasPrefix(item.URL, "https://") {
			imageParts = append(imageParts, fmt.Sprintf("[%s: %s]", item.Type, item.URL))
			continue
		}
		// 本地文件尝试 base64 编码
		absPath, err := filepath.Abs(item.URL)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		mimeType := item.MimeType
		if mimeType == "" {
			ext := filepath.Ext(absPath)
			mimeType = mime.TypeByExtension(ext)
		}
		if mimeType == "" || !strings.HasPrefix(mimeType, "image/") {
			// 非图片文件直接附加路径引用
			imageParts = append(imageParts, fmt.Sprintf("[%s: %s]", item.Type, absPath))
			continue
		}
		b64 := base64.StdEncoding.EncodeToString(data)
		imageParts = append(imageParts, fmt.Sprintf("[Image: data:%s;base64,%s]", mimeType, b64))
	}

	if len(imageParts) == 0 {
		return text
	}
	return strings.Join(imageParts, "\n") + "\n" + text
}

// AddToolResult 添加工具结果到消息列表
func (c *ContextBuilder) AddToolResult(messages []Message, toolCallID, toolName, result string) []Message {
	return append(messages, Message{
		Role:       "tool",
		ToolCallID: toolCallID,
		Name:       toolName,
		Content:    result,
	})
}

// AddAssistantMessage 添加助手消息到消息列表
func (c *ContextBuilder) AddAssistantMessage(messages []Message, content string, toolCalls []ToolCallEntry) []Message {
	msg := Message{
		Role:    "assistant",
		Content: content,
	}
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}
	return append(messages, msg)
}
