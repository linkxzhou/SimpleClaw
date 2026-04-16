// Bracket Tag 解析器：从 LLM 响应中提取 [@agent: message] 和 [#team: message] 标签。
// 支持嵌套括号、逗号分隔多目标、自引用过滤。

package agent

import (
	"strings"
	"unicode"
)

// Mention 一条 @agent 提及。
type Mention struct {
	AgentIDs []string // 目标 Agent ID（逗号分隔时多个）
	Message  string   // 要发送的消息
}

// ChatRoomMessage 一条 #team 广播。
type ChatRoomMessage struct {
	TeamID  string
	Message string
}

// ParseResult 标签解析结果。
type ParseResult struct {
	Mentions  []Mention
	ChatRooms []ChatRoomMessage
	CleanText string // 去除标签后的文本
}

// ParseBracketTags 解析 LLM 响应中的 bracket tags。
// 格式：[@agent: msg]、[@agent1,agent2: msg]、[#team: msg]
// selfID 为当前 Agent 的 ID，自引用会被过滤。
func ParseBracketTags(response string, selfID string, registry *AgentRegistry) ParseResult {
	var result ParseResult
	var clean strings.Builder
	runes := []rune(response)
	i := 0

	for i < len(runes) {
		// 检查是否是标签开始
		if runes[i] == '[' && i+1 < len(runes) && (runes[i+1] == '@' || runes[i+1] == '#') {
			tagType := runes[i+1] // '@' or '#'
			start := i

			// 查找冒号位置和关闭括号
			colonIdx := -1
			depth := 1
			j := i + 2

			for j < len(runes) && depth > 0 {
				switch runes[j] {
				case '[':
					depth++
				case ']':
					depth--
				case ':':
					if depth == 1 && colonIdx == -1 {
						colonIdx = j
					}
				}
				if depth > 0 {
					j++
				}
			}

			// 成功匹配完整标签
			if depth == 0 && colonIdx > 0 {
				targetStr := strings.TrimSpace(string(runes[i+2 : colonIdx]))
				message := strings.TrimSpace(string(runes[colonIdx+1 : j]))

				if tagType == '@' {
					mention := parseMention(targetStr, message, selfID, registry)
					if mention != nil {
						result.Mentions = append(result.Mentions, *mention)
						i = j + 1
						continue
					}
				} else if tagType == '#' {
					if registry != nil && registry.GetTeam(targetStr) != nil {
						result.ChatRooms = append(result.ChatRooms, ChatRoomMessage{
							TeamID:  targetStr,
							Message: message,
						})
						i = j + 1
						continue
					}
				}
			}

			// 解析失败，作为普通文本
			clean.WriteRune(runes[start])
			i = start + 1
			continue
		}

		clean.WriteRune(runes[i])
		i++
	}

	result.CleanText = strings.TrimSpace(clean.String())
	return result
}

// parseMention 解析 @mention 目标。
func parseMention(targetStr, message, selfID string, registry *AgentRegistry) *Mention {
	parts := strings.Split(targetStr, ",")
	var validIDs []string

	for _, p := range parts {
		id := strings.TrimSpace(p)
		if id == "" || id == selfID {
			continue // 过滤空值和自引用
		}
		// 验证 ID 合法性
		if !isValidAgentID(id) {
			continue
		}
		// 如果有 registry，验证 Agent 存在
		if registry != nil && !registry.HasAgent(id) {
			continue
		}
		validIDs = append(validIDs, id)
	}

	if len(validIDs) == 0 {
		return nil
	}

	return &Mention{
		AgentIDs: validIDs,
		Message:  message,
	}
}

// isValidAgentID 检查 Agent ID 是否合法（字母、数字、下划线、连字符）。
func isValidAgentID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' {
			return false
		}
	}
	return true
}
