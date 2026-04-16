package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/linkxzhou/SimpleClaw/agent/tools"
)

// LoopConfig 配置 Agent Loop 的行为。
type LoopConfig struct {
	Provider      LLMProvider
	Model         string
	Registry      *tools.Registry
	MaxIterations int
	Logger        *slog.Logger // 日志实例（主 Agent 用 slog.Default(), 子 Agent 用 debug logger）
}

// RunLoop 执行通用 ReAct 循环，返回最终文本响应。
// 主 Agent 和子 Agent 共用此函数，仅通过 LoopConfig 差异化行为。
func RunLoop(ctx context.Context, cfg LoopConfig, messages []Message) (string, error) {
	toolDefs := convertToolDefs(cfg.Registry.GetDefinitions())
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	for i := 0; i < cfg.MaxIterations; i++ {
		logger.Info("loop iteration start", "iteration", i+1, "maxIterations", cfg.MaxIterations)

		response, err := cfg.Provider.Chat(ctx, messages, toolDefs, cfg.Model)
		if err != nil {
			return "", fmt.Errorf("LLM chat error: %w", err)
		}

		if !response.HasToolCalls {
			contentPreview := response.Content
			if len(contentPreview) > 200 {
				contentPreview = contentPreview[:200] + "..."
			}
			logger.Info("loop final response", "iteration", i+1, "contentLength", len(response.Content), "preview", contentPreview)
			return response.Content, nil
		}

		// 收集工具调用名称用于日志
		toolNames := make([]string, 0, len(response.ToolCalls))
		for _, tc := range response.ToolCalls {
			toolNames = append(toolNames, tc.Name)
		}
		logger.Info("loop tool calls", "iteration", i+1, "count", len(response.ToolCalls), "tools", strings.Join(toolNames, ", "))

		// 添加 assistant 消息（包含 tool calls）
		var toolCallEntries []ToolCallEntry
		for _, tc := range response.ToolCalls {
			toolCallEntries = append(toolCallEntries, ToolCallToEntry(tc))
		}
		messages = append(messages, Message{
			Role:      "assistant",
			Content:   response.Content,
			ToolCalls: toolCallEntries,
		})

		// 执行工具
		for _, tc := range response.ToolCalls {
			logger.Info("loop executing tool", "iteration", i+1, "tool", tc.Name, "args", MarshalToolCallArgs(tc.Arguments))
			result := cfg.Registry.Execute(ctx, tc.Name, tc.Arguments)
			resultPreview := result
			if len(resultPreview) > 300 {
				resultPreview = resultPreview[:300] + "..."
			}
			logger.Info("loop tool result", "iteration", i+1, "tool", tc.Name, "resultLength", len(result), "preview", resultPreview)
			messages = append(messages, Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Name:       tc.Name,
				Content:    result,
			})
		}
	}

	logger.Warn("loop reached max iterations", "maxIterations", cfg.MaxIterations)
	return "I've reached the maximum number of iterations.", nil
}
