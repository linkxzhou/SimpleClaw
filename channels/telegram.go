// Telegram 渠道完整实现。
// 使用 Bot HTTP API 进行 Long Polling 消息接收和发送。
// 支持文本消息、Markdown→HTML 转换、长消息自动分片。

package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleClaw/bus"
)

// TelegramChannel 实现 Telegram Bot 渠道。
type TelegramChannel struct {
	BaseChannel
	token  string
	client *http.Client
	offset int64
	cancel context.CancelFunc
	logger *slog.Logger
}

// NewTelegramChannel 创建 Telegram 渠道。
func NewTelegramChannel(token string, allowList []string, msgBus *bus.MessageBus, logger *slog.Logger) *TelegramChannel {
	if logger == nil {
		logger = slog.Default()
	}
	return &TelegramChannel{
		BaseChannel: BaseChannel{
			name:      "telegram",
			bus:       msgBus,
			allowList: allowList,
		},
		token:  token,
		client: &http.Client{Timeout: 35 * time.Second},
		logger: logger,
	}
}

func (t *TelegramChannel) Start(ctx context.Context) error {
	if t.token == "" {
		return fmt.Errorf("telegram bot token not configured")
	}
	ctx, t.cancel = context.WithCancel(ctx)
	t.running.Store(true)
	go t.pollLoop(ctx)
	t.logger.Info("telegram channel started")
	return nil
}

func (t *TelegramChannel) Stop() error {
	if t.cancel != nil {
		t.cancel()
	}
	t.running.Store(false)
	t.logger.Info("telegram channel stopped")
	return nil
}

func (t *TelegramChannel) Send(msg bus.OutboundMessage) error {
	html := convertMarkdownToHTML(msg.Content)

	// Telegram 消息上限 4096 字符
	if len(html) <= 4096 {
		return t.sendMessage(msg.ChatID, html, "HTML")
	}

	// 分片发送
	return t.sendChunked(msg.ChatID, html, 4096)
}

// pollLoop Long Polling 消息接收循环。
func (t *TelegramChannel) pollLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			updates, err := t.getUpdates(ctx, t.offset, 30)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				t.logger.Error("telegram getUpdates failed", "error", err)
				time.Sleep(5 * time.Second)
				continue
			}
			for _, update := range updates {
				t.offset = update.UpdateID + 1
				t.handleUpdate(update)
			}
		}
	}
}

// handleUpdate 处理单个 Telegram Update。
func (t *TelegramChannel) handleUpdate(update tgUpdate) {
	msg := update.Message
	if msg == nil {
		return
	}

	senderID := fmt.Sprint(msg.From.ID)
	chatID := fmt.Sprint(msg.Chat.ID)
	content := msg.Text

	// 检查白名单
	if !t.IsAllowed(senderID) {
		t.logger.Debug("telegram message from unlisted sender", "sender", senderID)
		return
	}

	if content == "" {
		if msg.Caption != "" {
			content = msg.Caption
		} else {
			return
		}
	}

	t.HandleMessage(bus.InboundMessage{
		Channel:   "telegram",
		SenderID:  senderID,
		ChatID:    chatID,
		Content:   content,
		Timestamp: time.Unix(int64(msg.Date), 0),
	})
}

// === Telegram Bot API 调用 ===

func (t *TelegramChannel) getUpdates(ctx context.Context, offset int64, timeout int) ([]tgUpdate, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=%d&allowed_updates=[\"message\"]",
		t.token, offset, timeout)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		OK     bool       `json:"ok"`
		Result []tgUpdate `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if !result.OK {
		return nil, fmt.Errorf("telegram API returned ok=false: %s", string(body))
	}
	return result.Result, nil
}

func (t *TelegramChannel) sendMessage(chatID, text, parseMode string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.token)

	payload := map[string]interface{}{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": parseMode,
	}
	data, _ := json.Marshal(payload)

	resp, err := t.client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		// HTML 解析失败时回退为纯文本
		if strings.Contains(string(body), "can't parse entities") {
			return t.sendMessage(chatID, stripHTMLTags(text), "")
		}
		return fmt.Errorf("telegram sendMessage failed (status %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

func (t *TelegramChannel) sendChunked(chatID, text string, maxLen int) error {
	for len(text) > 0 {
		chunk := text
		if len(chunk) > maxLen {
			// 尝试在换行处截断
			cutAt := strings.LastIndex(chunk[:maxLen], "\n")
			if cutAt < maxLen/2 {
				cutAt = maxLen
			}
			chunk = text[:cutAt]
			text = text[cutAt:]
		} else {
			text = ""
		}
		if err := t.sendMessage(chatID, chunk, "HTML"); err != nil {
			return err
		}
	}
	return nil
}

// === Telegram 数据类型 ===

type tgUpdate struct {
	UpdateID int64    `json:"update_id"`
	Message  *tgMessage `json:"message"`
}

type tgMessage struct {
	MessageID int    `json:"message_id"`
	From      tgUser `json:"from"`
	Chat      tgChat `json:"chat"`
	Date      int    `json:"date"`
	Text      string `json:"text"`
	Caption   string `json:"caption"`
}

type tgUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type tgChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// === Markdown → Telegram HTML 转换 ===

var (
	// 代码块（```lang\ncode\n```）
	reCodeBlock = regexp.MustCompile("(?s)```(?:\\w*)\n?(.*?)```")
	// 行内代码（`code`）
	reInlineCode = regexp.MustCompile("`([^`]+)`")
	// 粗体（**text**）
	reBold = regexp.MustCompile(`\*\*(.+?)\*\*`)
	// 斜体（*text* 或 _text_）
	reItalic = regexp.MustCompile(`(?:^|[^*])\*([^*]+)\*(?:[^*]|$)`)
)

// convertMarkdownToHTML 将 Markdown 转换为 Telegram HTML。
func convertMarkdownToHTML(md string) string {
	// 保护代码块
	var codeBlocks []string
	result := reCodeBlock.ReplaceAllStringFunc(md, func(m string) string {
		sub := reCodeBlock.FindStringSubmatch(m)
		code := escapeHTML(sub[1])
		placeholder := fmt.Sprintf("__CODE_BLOCK_%d__", len(codeBlocks))
		codeBlocks = append(codeBlocks, "<pre><code>"+code+"</code></pre>")
		return placeholder
	})

	// 保护行内代码
	var inlineCodes []string
	result = reInlineCode.ReplaceAllStringFunc(result, func(m string) string {
		sub := reInlineCode.FindStringSubmatch(m)
		code := escapeHTML(sub[1])
		placeholder := fmt.Sprintf("__INLINE_CODE_%d__", len(inlineCodes))
		inlineCodes = append(inlineCodes, "<code>"+code+"</code>")
		return placeholder
	})

	// 转义 HTML 实体
	result = escapeHTML(result)

	// 恢复占位符（占位符中的 HTML 不会被转义，因为它们不包含 < > &）
	for i, block := range codeBlocks {
		result = strings.Replace(result, escapeHTML(fmt.Sprintf("__CODE_BLOCK_%d__", i)), block, 1)
	}
	for i, code := range inlineCodes {
		result = strings.Replace(result, escapeHTML(fmt.Sprintf("__INLINE_CODE_%d__", i)), code, 1)
	}

	// 格式转换
	result = reBold.ReplaceAllString(result, "<b>$1</b>")

	return result
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func stripHTMLTags(s string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	return re.ReplaceAllString(s, "")
}
