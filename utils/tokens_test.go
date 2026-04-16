package utils

import (
	"strings"
	"testing"
)

func TestEstimateTokensEmpty(t *testing.T) {
	if got := EstimateTokens(""); got != 0 {
		t.Errorf("空字符串应返回 0，实际 %d", got)
	}
}

func TestEstimateTokensEnglish(t *testing.T) {
	// "Hello world" = 10 alpha + 1 space → 10*0.25 + 1*0.5 = 3
	tokens := EstimateTokens("Hello world")
	if tokens < 2 || tokens > 5 {
		t.Errorf("英文估算异常，实际 %d", tokens)
	}
}

func TestEstimateTokensChinese(t *testing.T) {
	// 10 个汉字 → 10*0.6 = 6
	tokens := EstimateTokens("你好世界我是一个人工智能")
	if tokens < 4 || tokens > 10 {
		t.Errorf("中文估算异常，实际 %d", tokens)
	}
}

func TestEstimateTokensCode(t *testing.T) {
	code := `func main() { fmt.Println("hello") }`
	tokens := EstimateTokens(code)
	if tokens < 5 || tokens > 20 {
		t.Errorf("代码估算异常，实际 %d", tokens)
	}
}

func TestEstimateTokensLong(t *testing.T) {
	// 1000 个英文单词，每个约 5 字符 + 1 空格
	words := make([]string, 1000)
	for i := range words {
		words[i] = "hello"
	}
	text := strings.Join(words, " ")
	tokens := EstimateTokens(text)
	// 预期约 1000*5*0.25 + 999*0.5 = 1250 + 499 = 1749
	if tokens < 1000 || tokens > 2500 {
		t.Errorf("长文本估算异常，实际 %d", tokens)
	}
}

func TestEstimateTokensMinimum(t *testing.T) {
	// 单字符应返回至少 1
	if got := EstimateTokens("a"); got < 1 {
		t.Errorf("单字符应返回至少 1，实际 %d", got)
	}
}

func TestTruncateContentNoTruncation(t *testing.T) {
	text := "hello world"
	got := TruncateContent(text, 100)
	if got != text {
		t.Errorf("不应截断，期望 %q，实际 %q", text, got)
	}
}

func TestTruncateContentDisabled(t *testing.T) {
	text := "hello world"
	if got := TruncateContent(text, 0); got != text {
		t.Error("maxRunes=0 不应截断")
	}
	if got := TruncateContent(text, -1); got != text {
		t.Error("maxRunes=-1 不应截断")
	}
}

func TestTruncateContentTruncates(t *testing.T) {
	text := "hello world"
	got := TruncateContent(text, 5)
	if !strings.HasPrefix(got, "hello") {
		t.Errorf("应保留前 5 个 rune，实际 %q", got)
	}
	if !strings.HasSuffix(got, "[truncated]") {
		t.Errorf("应添加截断标记，实际 %q", got)
	}
}

func TestTruncateContentCJK(t *testing.T) {
	text := "你好世界测试"
	got := TruncateContent(text, 3)
	if !strings.HasPrefix(got, "你好世") {
		t.Errorf("CJK 截断异常，实际 %q", got)
	}
}
