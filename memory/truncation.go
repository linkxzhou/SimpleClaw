// 智能上下文截断：在安全切割点归档旧消息，保持语义完整性。
// 安全切割点 = assistant 回复之后、且后续非 tool 消息处。
// system 消息始终保留，tool_use → tool_result 对不会被拆分。

package memory

import (
	"fmt"

	"github.com/linkxzhou/SimpleClaw/utils"
)

// TruncMessage 用于截断算法的消息接口（避免依赖 agent 包）。
type TruncMessage struct {
	Role    string
	Content string
}

// CutPoint 一个安全切割点。
type CutPoint struct {
	Index       int // 切割位置（该索引之前的消息将被归档）
	TokensSaved int // 截断后可节省的 token 数
}

// FindSafeCutPoints 扫描消息列表，返回所有安全切割点（从前向后累计 token）。
// 安全切割条件：
//   - 不在 system 消息上切割（始终保留）
//   - 不在 tool 消息上切割（必须和前面的 assistant tool_use 一起）
//   - 在 assistant 回复之后、且下一条不是 tool 时切割
func FindSafeCutPoints(messages []TruncMessage) []CutPoint {
	var points []CutPoint
	tokensSoFar := 0

	for i, msg := range messages {
		tokensSoFar += utils.EstimateTokens(msg.Content) + 4 // +4 为 role/分隔符开销

		// system 消息始终保留，不作为切割点
		if msg.Role == "system" {
			continue
		}

		// tool 消息不能单独切割（必须跟着 assistant 的 tool_use）
		if msg.Role == "tool" {
			continue
		}

		// 在 assistant 回复之后是安全切割点
		if msg.Role == "assistant" {
			// 但如果下一条是 tool（说明 assistant 的 tool_use 还没得到 result），跳过
			if i+1 < len(messages) && messages[i+1].Role == "tool" {
				continue
			}
			points = append(points, CutPoint{
				Index:       i + 1, // 切割点在该消息之后
				TokensSaved: tokensSoFar,
			})
		}

		// user 消息之后也是一个次优切割点（语义上一轮已结束）
		if msg.Role == "user" && i > 0 {
			// 只在前一条不是 system 时才考虑
			if messages[i-1].Role != "system" {
				points = append(points, CutPoint{
					Index:       i,
					TokensSaved: tokensSoFar - utils.EstimateTokens(msg.Content) - 4,
				})
			}
		}
	}

	return points
}

// PickCutPoint 选择能节省 targetTokens 的最小切割点。
// 返回 nil 表示无可用切割点。
func PickCutPoint(points []CutPoint, targetTokens int) *CutPoint {
	for i := range points {
		if points[i].TokensSaved >= targetTokens {
			return &points[i]
		}
	}
	// 无法满足目标，选最后一个（最大切割）
	if len(points) > 0 {
		return &points[len(points)-1]
	}
	return nil
}

// TruncateMessages 对消息列表执行智能截断。
// budget 为可用 token 预算。返回截断后的消息列表和被归档的消息数。
// 如果不需要截断则原样返回。
func TruncateMessages(messages []TruncMessage, budget int) (result []TruncMessage, archivedCount int) {
	// 估算总 token 数
	totalTokens := 0
	for _, m := range messages {
		totalTokens += utils.EstimateTokens(m.Content) + 4
	}

	if totalTokens <= budget {
		return messages, 0 // 无需截断
	}

	// 目标：压缩到预算的 75%，留余量给后续 tool 调用
	targetSave := totalTokens - budget*3/4

	// 找到安全切割点
	cutPoints := FindSafeCutPoints(messages)
	cut := PickCutPoint(cutPoints, targetSave)

	if cut == nil || cut.Index <= 1 {
		// 无安全切割点或只有 system 消息，降级为保留尾部
		return keepRecentMessages(messages, budget), len(messages) - countRecentMessages(messages, budget)
	}

	// 找到第一条 system 消息的结束位置
	systemEnd := 0
	for i, m := range messages {
		if m.Role == "system" {
			systemEnd = i + 1
		} else {
			break
		}
	}

	// 归档 [systemEnd, cut.Index) 的消息
	archivedCount = cut.Index - systemEnd

	// 重组：system + 摘要占位符 + 保留消息
	result = make([]TruncMessage, 0, len(messages)-archivedCount+1)
	result = append(result, messages[:systemEnd]...) // system 消息
	result = append(result, TruncMessage{
		Role:    "system",
		Content: fmt.Sprintf("[Earlier conversation (%d messages) archived to save context space]", archivedCount),
	})
	result = append(result, messages[cut.Index:]...) // 保留的消息

	return result, archivedCount
}

// keepRecentMessages 保留尾部消息（降级策略）。
func keepRecentMessages(messages []TruncMessage, budget int) []TruncMessage {
	// 始终保留第一条 system 消息
	if len(messages) == 0 {
		return messages
	}

	var systemMsgs []TruncMessage
	systemTokens := 0
	rest := messages
	for len(rest) > 0 && rest[0].Role == "system" {
		systemTokens += utils.EstimateTokens(rest[0].Content) + 4
		systemMsgs = append(systemMsgs, rest[0])
		rest = rest[1:]
	}

	remaining := budget - systemTokens
	if remaining <= 0 {
		return systemMsgs
	}

	// 从尾部开始累加
	var kept []TruncMessage
	for i := len(rest) - 1; i >= 0; i-- {
		tokens := utils.EstimateTokens(rest[i].Content) + 4
		if remaining < tokens {
			break
		}
		remaining -= tokens
		kept = append([]TruncMessage{rest[i]}, kept...)
	}

	result := make([]TruncMessage, 0, len(systemMsgs)+1+len(kept))
	result = append(result, systemMsgs...)
	if len(kept) < len(rest) {
		result = append(result, TruncMessage{
			Role:    "system",
			Content: fmt.Sprintf("[Earlier conversation (%d messages) truncated]", len(rest)-len(kept)),
		})
	}
	result = append(result, kept...)
	return result
}

// countRecentMessages 计算能放入 budget 的尾部消息数。
func countRecentMessages(messages []TruncMessage, budget int) int {
	tokens := 0
	count := 0
	for i := len(messages) - 1; i >= 0; i-- {
		t := utils.EstimateTokens(messages[i].Content) + 4
		if tokens+t > budget {
			break
		}
		tokens += t
		count++
	}
	return count
}
