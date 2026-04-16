// 内置中间件：审计日志 + 内容过滤。

package bus

import (
	"log/slog"
	"regexp"
)

// ============ 审计中间件 ============

// AuditMiddleware 记录所有入站消息到日志。
type AuditMiddleware struct {
	logger *slog.Logger
}

// NewAuditMiddleware 创建审计中间件。
func NewAuditMiddleware(logger *slog.Logger) *AuditMiddleware {
	if logger == nil {
		logger = slog.Default()
	}
	return &AuditMiddleware{logger: logger}
}

func (a *AuditMiddleware) Name() string { return "audit" }

func (a *AuditMiddleware) ProcessInbound(msg *InboundMessage) *InboundMessage {
	a.logger.Info("[AUDIT] inbound",
		"channel", msg.Channel,
		"sender", msg.SenderID,
		"content_len", len(msg.Content),
	)
	return msg // 透传不修改
}

// ============ 内容过滤中间件 ============

// ContentFilterMiddleware 按正则过滤消息，匹配则丢弃。
type ContentFilterMiddleware struct {
	patterns []*regexp.Regexp
}

// NewContentFilterMiddleware 创建内容过滤中间件。
func NewContentFilterMiddleware(patterns []string) *ContentFilterMiddleware {
	var compiled []*regexp.Regexp
	for _, p := range patterns {
		if r, err := regexp.Compile(p); err == nil {
			compiled = append(compiled, r)
		}
	}
	return &ContentFilterMiddleware{patterns: compiled}
}

func (f *ContentFilterMiddleware) Name() string { return "content_filter" }

func (f *ContentFilterMiddleware) ProcessInbound(msg *InboundMessage) *InboundMessage {
	for _, pattern := range f.patterns {
		if pattern.MatchString(msg.Content) {
			return nil // 丢弃匹配的消息
		}
	}
	return msg
}
