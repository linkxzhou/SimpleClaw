// Token 估算器：不依赖外部 tokenizer 库的轻量级实现。
// 英文按字符/4 估算，CJK 按字符*0.6，其他按字符*0.5。
// 误差目标：±30% 以内。

package utils

import "unicode"

// EstimateTokens 估算文本的 token 数量。
// 使用经验系数：CJK ≈ 0.6 token/char，英文 ≈ 0.25 token/char，其他 ≈ 0.5 token/char。
func EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}

	var cjk, alphaNum, other int
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Han, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hiragana, r):
			cjk++
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			alphaNum++
		default:
			other++
		}
	}

	tokens := int(float64(cjk)*0.6 + float64(alphaNum)*0.25 + float64(other)*0.5)
	if tokens == 0 {
		tokens = 1
	}
	return tokens
}

// TruncateContent 按 rune 数截断内容，超出部分添加截断标记。
// maxRunes <= 0 时不截断。
func TruncateContent(content string, maxRunes int) string {
	if maxRunes <= 0 {
		return content
	}
	runes := []rune(content)
	if len(runes) <= maxRunes {
		return content
	}
	return string(runes[:maxRunes]) + "\n... [truncated]"
}
