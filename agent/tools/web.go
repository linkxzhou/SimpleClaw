package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_7_2) AppleWebKit/537.36"

// stripTags 去除 HTML 标签并解码实体
func stripTags(text string) string {
	// 移除 script 和 style 标签
	reScript := regexp.MustCompile(`(?is)<script[\s\S]*?</script>`)
	reStyle := regexp.MustCompile(`(?is)<style[\s\S]*?</style>`)
	text = reScript.ReplaceAllString(text, "")
	text = reStyle.ReplaceAllString(text, "")
	// 移除所有标签
	reTags := regexp.MustCompile(`<[^>]+>`)
	text = reTags.ReplaceAllString(text, "")
	return strings.TrimSpace(html.UnescapeString(text))
}

// normalize 规范化空白
func normalize(text string) string {
	reSpaces := regexp.MustCompile(`[ \t]+`)
	text = reSpaces.ReplaceAllString(text, " ")
	reNewlines := regexp.MustCompile(`\n{3,}`)
	text = reNewlines.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

// ============ WebSearchTool ============

// WebSearchTool 使用 Brave Search API 搜索网页
type WebSearchTool struct {
	APIKey     string
	MaxResults int
}

// NewWebSearchTool 创建网页搜索工具
func NewWebSearchTool(apiKey string) *WebSearchTool {
	if apiKey == "" {
		apiKey = os.Getenv("BRAVE_API_KEY")
	}
	return &WebSearchTool{
		APIKey:     apiKey,
		MaxResults: 5,
	}
}

func (t *WebSearchTool) Name() string        { return "web_search" }
func (t *WebSearchTool) Description() string { return "Search the web. Returns titles, URLs, and snippets." }
func (t *WebSearchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query",
			},
			"count": map[string]interface{}{
				"type":        "integer",
				"description": "Number of results (1-10)",
				"minimum":     1,
				"maximum":     10,
			},
		},
		"required": []string{"query"},
	}
}

func (t *WebSearchTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.APIKey == "" {
		return "Error: BRAVE_API_KEY not configured", nil
	}

	query, _ := params["query"].(string)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}

	count := t.MaxResults
	if c, ok := params["count"].(float64); ok {
		count = int(c)
		if count < 1 {
			count = 1
		}
		if count > 10 {
			count = 10
		}
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.search.brave.com/res/v1/web/search", nil)
	if err != nil {
		return "", err
	}
	q := req.URL.Query()
	q.Set("q", query)
	q.Set("count", fmt.Sprintf("%d", count))
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", t.APIKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Sprintf("Error: %s", err.Error()), nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Sprintf("Error parsing response: %s", err.Error()), nil
	}

	results := result.Web.Results
	if len(results) == 0 {
		return fmt.Sprintf("No results for: %s", query), nil
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Results for: %s\n", query))
	for i, item := range results {
		if i >= count {
			break
		}
		lines = append(lines, fmt.Sprintf("%d. %s\n   %s", i+1, item.Title, item.URL))
		if item.Description != "" {
			lines = append(lines, fmt.Sprintf("   %s", item.Description))
		}
	}
	return strings.Join(lines, "\n"), nil
}

// ============ WebFetchTool ============

// WebFetchTool 获取 URL 内容并提取可读文本
type WebFetchTool struct {
	MaxChars int
}

// NewWebFetchTool 创建网页获取工具
func NewWebFetchTool() *WebFetchTool {
	return &WebFetchTool{MaxChars: 50000}
}

func (t *WebFetchTool) Name() string        { return "web_fetch" }
func (t *WebFetchTool) Description() string {
	return "Fetch URL and extract readable content (HTML to text)."
}
func (t *WebFetchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "URL to fetch",
			},
			"max_chars": map[string]interface{}{
				"type":    "integer",
				"minimum": 100,
			},
		},
		"required": []string{"url"},
	}
}

func (t *WebFetchTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	url, _ := params["url"].(string)
	if url == "" {
		return "", fmt.Errorf("url is required")
	}

	maxChars := t.MaxChars
	if mc, ok := params["max_chars"].(float64); ok && int(mc) > 100 {
		maxChars = int(mc)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return jsonError(url, err.Error()), nil
	}
	req.Header.Set("User-Agent", userAgent)

	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return jsonError(url, err.Error()), nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return jsonError(url, err.Error()), nil
	}

	contentType := resp.Header.Get("Content-Type")
	var text, extractor string

	switch {
	case strings.Contains(contentType, "application/json"):
		// 格式化 JSON
		var jsonData interface{}
		if err := json.Unmarshal(body, &jsonData); err == nil {
			formatted, _ := json.MarshalIndent(jsonData, "", "  ")
			text = string(formatted)
		} else {
			text = string(body)
		}
		extractor = "json"

	case strings.Contains(contentType, "text/html"),
		strings.HasPrefix(strings.TrimSpace(strings.ToLower(string(body[:min(256, len(body))])), ), "<!doctype"),
		strings.HasPrefix(strings.TrimSpace(strings.ToLower(string(body[:min(256, len(body))])), ), "<html"):
		// 简单的 HTML -> text 转换
		text = htmlToText(string(body))
		extractor = "html"

	default:
		text = string(body)
		extractor = "raw"
	}

	truncated := len(text) > maxChars
	if truncated {
		text = text[:maxChars]
	}

	result, _ := json.Marshal(map[string]interface{}{
		"url":       url,
		"finalUrl":  resp.Request.URL.String(),
		"status":    resp.StatusCode,
		"extractor": extractor,
		"truncated": truncated,
		"length":    len(text),
		"text":      text,
	})
	return string(result), nil
}

// htmlToText 简单的 HTML 转文本
func htmlToText(htmlContent string) string {
	// 转换链接
	reLink := regexp.MustCompile(`(?is)<a\s+[^>]*href=["']([^"']+)["'][^>]*>([\s\S]*?)</a>`)
	text := reLink.ReplaceAllStringFunc(htmlContent, func(match string) string {
		sub := reLink.FindStringSubmatch(match)
		if len(sub) >= 3 {
			return fmt.Sprintf("[%s](%s)", stripTags(sub[2]), sub[1])
		}
		return match
	})

	// 转换标题
	reHeading := regexp.MustCompile(`(?is)<h([1-6])[^>]*>([\s\S]*?)</h[1-6]>`)
	text = reHeading.ReplaceAllStringFunc(text, func(match string) string {
		sub := reHeading.FindStringSubmatch(match)
		if len(sub) >= 3 {
			level := sub[1][0] - '0'
			return "\n" + strings.Repeat("#", int(level)) + " " + stripTags(sub[2]) + "\n"
		}
		return match
	})

	// 转换列表项
	reLi := regexp.MustCompile(`(?is)<li[^>]*>([\s\S]*?)</li>`)
	text = reLi.ReplaceAllStringFunc(text, func(match string) string {
		sub := reLi.FindStringSubmatch(match)
		if len(sub) >= 2 {
			return "\n- " + stripTags(sub[1])
		}
		return match
	})

	// 段落和换行
	reBlock := regexp.MustCompile(`(?i)</(p|div|section|article)>`)
	text = reBlock.ReplaceAllString(text, "\n\n")
	reBr := regexp.MustCompile(`(?i)<(br|hr)\s*/?>`)
	text = reBr.ReplaceAllString(text, "\n")

	return normalize(stripTags(text))
}

func jsonError(url, errMsg string) string {
	result, _ := json.Marshal(map[string]interface{}{
		"error": errMsg,
		"url":   url,
	})
	return string(result)
}
