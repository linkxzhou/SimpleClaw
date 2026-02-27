package channels

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/linkxzhou/SimpleClaw/bus"
	"github.com/linkxzhou/SimpleClaw/config"
)

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

func newTestBus() *bus.MessageBus {
	return bus.NewMessageBus(64, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// ---------------------------------------------------------------------------
// BaseChannel 测试（通过 Telegram 包装进行间接测试，因为字段未导出）
// ---------------------------------------------------------------------------

func TestBaseChannelNameViaTelegram(t *testing.T) {
	ch := NewTelegramChannel("token", nil, newTestBus(), testLogger())
	if ch.Name() != "telegram" {
		t.Errorf("期望 'telegram'，实际 %q", ch.Name())
	}
}

func TestBaseChannelIsRunningDefault(t *testing.T) {
	ch := NewTelegramChannel("token", nil, newTestBus(), testLogger())
	if ch.IsRunning() {
		t.Error("初始状态应为未运行")
	}
}

// ---------------------------------------------------------------------------
// IsAllowed 测试（通过 Telegram 包装）
// ---------------------------------------------------------------------------

func TestIsAllowedEmptyList(t *testing.T) {
	ch := NewTelegramChannel("token", nil, newTestBus(), testLogger())
	if !ch.IsAllowed("anyone") {
		t.Error("空白名单应允许所有人")
	}
}

func TestIsAllowedExactMatch(t *testing.T) {
	ch := NewTelegramChannel("token", []string{"user1", "user2"}, newTestBus(), testLogger())
	if !ch.IsAllowed("user1") {
		t.Error("user1 应被允许")
	}
	if !ch.IsAllowed("user2") {
		t.Error("user2 应被允许")
	}
	if ch.IsAllowed("user3") {
		t.Error("user3 不应被允许")
	}
}

func TestIsAllowedPipeID(t *testing.T) {
	ch := NewTelegramChannel("token", []string{"12345"}, newTestBus(), testLogger())
	if !ch.IsAllowed("12345|username") {
		t.Error("包含 12345 的管道 ID 应被允许")
	}
	if !ch.IsAllowed("other|12345") {
		t.Error("后段匹配也应被允许")
	}
	if ch.IsAllowed("67890|username") {
		t.Error("不包含 12345 的管道 ID 不应被允许")
	}
}

func TestIsAllowedPipeEmptyParts(t *testing.T) {
	ch := NewTelegramChannel("token", []string{"user1"}, newTestBus(), testLogger())
	if !ch.IsAllowed("user1|") {
		t.Error("user1| 应被允许")
	}
	if !ch.IsAllowed("|user1") {
		t.Error("|user1 应被允许")
	}
}

// ---------------------------------------------------------------------------
// HandleMessage 测试
// ---------------------------------------------------------------------------

func TestHandleMessagePublishes(t *testing.T) {
	mb := newTestBus()
	ch := NewTelegramChannel("token", nil, mb, testLogger())

	err := ch.HandleMessage(bus.InboundMessage{
		Channel:  "telegram",
		SenderID: "user1",
		ChatID:   "chat1",
		Content:  "hello",
	})
	if err != nil {
		t.Fatalf("HandleMessage 失败: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*testTimeout)
	defer cancel()
	msg, err := mb.ConsumeInbound(ctx)
	if err != nil {
		t.Fatalf("消费入站消息失败: %v", err)
	}
	if msg.Channel != "telegram" {
		t.Errorf("期望 channel=telegram，实际 %q", msg.Channel)
	}
	if msg.Content != "hello" {
		t.Errorf("期望 content=hello，实际 %q", msg.Content)
	}
}

// ---------------------------------------------------------------------------
// TelegramChannel 测试
// ---------------------------------------------------------------------------

func TestNewTelegramChannel(t *testing.T) {
	mb := newTestBus()
	ch := NewTelegramChannel("test-token", []string{"user1"}, mb, testLogger())

	if ch.Name() != "telegram" {
		t.Errorf("期望 name=telegram，实际 %q", ch.Name())
	}
	if ch.token != "test-token" {
		t.Errorf("期望 token=test-token，实际 %q", ch.token)
	}
}

func TestNewTelegramChannelNilLogger(t *testing.T) {
	ch := NewTelegramChannel("token", nil, newTestBus(), nil)
	if ch.logger == nil {
		t.Error("logger 不应为 nil")
	}
}

func TestTelegramStartNoToken(t *testing.T) {
	ch := NewTelegramChannel("", nil, newTestBus(), testLogger())
	err := ch.Start(context.Background())
	if err == nil {
		t.Error("无 token 时 Start 应返回错误")
	}
}

func TestTelegramStartWithToken(t *testing.T) {
	ch := NewTelegramChannel("token", nil, newTestBus(), testLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := ch.Start(ctx); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	if !ch.IsRunning() {
		t.Error("Start 后应为运行状态")
	}
	_ = ch.Stop()
}

func TestTelegramStop(t *testing.T) {
	ch := NewTelegramChannel("token", nil, newTestBus(), testLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = ch.Start(ctx)
	if err := ch.Stop(); err != nil {
		t.Fatalf("Stop 失败: %v", err)
	}
	if ch.IsRunning() {
		t.Error("Stop 后应为停止状态")
	}
}

func TestTelegramSend(t *testing.T) {
	ch := NewTelegramChannel("token", nil, newTestBus(), testLogger())
	// Send 到一个不存在的 API 不会 panic
	err := ch.Send(bus.OutboundMessage{
		Channel: "telegram",
		ChatID:  "123",
		Content: "hello",
	})
	// 因为 token 无效，预期有网络错误，但不应 panic
	_ = err
}

// ---------------------------------------------------------------------------
// MarkdownToTelegramHTML 测试（导出的 convertMarkdownToHTML）
// ---------------------------------------------------------------------------

func TestConvertMarkdownToHTMLEmpty(t *testing.T) {
	if got := convertMarkdownToHTML(""); got != "" {
		t.Errorf("空字符串应返回空，实际 %q", got)
	}
}

func TestConvertMarkdownToHTMLBold(t *testing.T) {
	got := convertMarkdownToHTML("**bold**")
	if got != "<b>bold</b>" {
		t.Errorf("期望 <b>bold</b>，实际 %q", got)
	}
}

func TestConvertMarkdownToHTMLInlineCode(t *testing.T) {
	got := convertMarkdownToHTML("use `fmt.Println`")
	expected := "use <code>fmt.Println</code>"
	if got != expected {
		t.Errorf("期望 %q，实际 %q", expected, got)
	}
}

func TestConvertMarkdownToHTMLCodeBlock(t *testing.T) {
	input := "```go\nfmt.Println(\"hello\")\n```"
	got := convertMarkdownToHTML(input)
	if !containsAll(got, "<pre><code>", "fmt.Println", "</code></pre>") {
		t.Errorf("代码块转换不正确: %q", got)
	}
}

func TestConvertMarkdownToHTMLEscapeHTML(t *testing.T) {
	got := convertMarkdownToHTML("a < b > c & d")
	if !containsAll(got, "&lt;", "&gt;", "&amp;") {
		t.Errorf("HTML 实体未转义: %q", got)
	}
}

// ---------------------------------------------------------------------------
// escapeHTML 测试
// ---------------------------------------------------------------------------

func TestEscapeHTML(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"<b>", "&lt;b&gt;"},
		{"a & b", "a &amp; b"},
		{"no special", "no special"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := escapeHTML(tc.input); got != tc.expected {
			t.Errorf("escapeHTML(%q) = %q，期望 %q", tc.input, got, tc.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// WhatsAppChannel 测试
// ---------------------------------------------------------------------------

func TestNewWhatsAppChannel(t *testing.T) {
	mb := newTestBus()
	ch := NewWhatsAppChannel("ws://localhost:3001", []string{"user1"}, mb, testLogger())

	if ch.Name() != "whatsapp" {
		t.Errorf("期望 name=whatsapp，实际 %q", ch.Name())
	}
	if ch.bridgeURL != "ws://localhost:3001" {
		t.Errorf("期望 bridgeURL=ws://localhost:3001，实际 %q", ch.bridgeURL)
	}
}

func TestNewWhatsAppChannelNilLogger(t *testing.T) {
	ch := NewWhatsAppChannel("ws://localhost:3001", nil, newTestBus(), nil)
	if ch.logger == nil {
		t.Error("logger 不应为 nil")
	}
}

func TestWhatsAppStartNoBridgeURL(t *testing.T) {
	ch := NewWhatsAppChannel("", nil, newTestBus(), testLogger())
	err := ch.Start(context.Background())
	if err == nil {
		t.Error("无 bridgeURL 时 Start 应返回错误")
	}
}

func TestWhatsAppStartStop(t *testing.T) {
	ch := NewWhatsAppChannel("ws://localhost:3001", nil, newTestBus(), testLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := ch.Start(ctx); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	if !ch.IsRunning() {
		t.Error("Start 后应为运行状态")
	}

	if err := ch.Stop(); err != nil {
		t.Fatalf("Stop 失败: %v", err)
	}
	if ch.IsRunning() {
		t.Error("Stop 后应为停止状态")
	}
}

func TestWhatsAppSendNotConnected(t *testing.T) {
	ch := NewWhatsAppChannel("ws://localhost:3001", nil, newTestBus(), testLogger())
	err := ch.Send(bus.OutboundMessage{Content: "hello"})
	if err == nil {
		t.Error("未连接时 Send 应返回错误")
	}
}

// ---------------------------------------------------------------------------
// Manager 测试
// ---------------------------------------------------------------------------

func TestNewManagerNoChannels(t *testing.T) {
	cfg := config.DefaultConfig()
	mb := newTestBus()
	m := NewManager(cfg, mb, testLogger())

	if len(m.channels) != 0 {
		t.Errorf("默认配置不应有渠道，实际 %d", len(m.channels))
	}
}

func TestNewManagerWithTelegram(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Channels.Telegram.Enabled = true
	cfg.Channels.Telegram.Token = "token"
	mb := newTestBus()
	m := NewManager(cfg, mb, testLogger())

	if _, ok := m.channels["telegram"]; !ok {
		t.Error("应注册 Telegram 渠道")
	}
}

func TestNewManagerWithWhatsApp(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Channels.WhatsApp.Enabled = true
	mb := newTestBus()
	m := NewManager(cfg, mb, testLogger())

	if _, ok := m.channels["whatsapp"]; !ok {
		t.Error("应注册 WhatsApp 渠道")
	}
}

func TestNewManagerNilLogger(t *testing.T) {
	cfg := config.DefaultConfig()
	m := NewManager(cfg, newTestBus(), nil)
	if m.logger == nil {
		t.Error("logger 不应为 nil")
	}
}

func TestManagerRegister(t *testing.T) {
	cfg := config.DefaultConfig()
	m := NewManager(cfg, newTestBus(), testLogger())

	mock := &mockChannel{channelName: "mock"}
	m.Register(mock)

	ch, ok := m.GetChannel("mock")
	if !ok || ch.Name() != "mock" {
		t.Error("注册后应能获取渠道")
	}
}

func TestManagerGetChannelNotFound(t *testing.T) {
	cfg := config.DefaultConfig()
	m := NewManager(cfg, newTestBus(), testLogger())

	_, ok := m.GetChannel("nonexistent")
	if ok {
		t.Error("不存在的渠道应返回 false")
	}
}

func TestManagerGetStatus(t *testing.T) {
	cfg := config.DefaultConfig()
	m := NewManager(cfg, newTestBus(), testLogger())
	mock := &mockChannel{channelName: "mock", isRunning: true}
	m.Register(mock)

	status := m.GetStatus()
	if s, ok := status["mock"]; !ok {
		t.Error("应包含 mock 渠道状态")
	} else {
		if !s.Running {
			t.Error("mock 应为运行状态")
		}
		if !s.Enabled {
			t.Error("已注册渠道应为已启用")
		}
	}
}

func TestManagerEnabledChannels(t *testing.T) {
	cfg := config.DefaultConfig()
	m := NewManager(cfg, newTestBus(), testLogger())
	m.Register(&mockChannel{channelName: "ch1"})
	m.Register(&mockChannel{channelName: "ch2"})

	names := m.EnabledChannels()
	if len(names) != 2 {
		t.Errorf("期望 2 个渠道，实际 %d", len(names))
	}
}

func TestManagerStartAllNoChannels(t *testing.T) {
	cfg := config.DefaultConfig()
	m := NewManager(cfg, newTestBus(), testLogger())

	err := m.StartAll(context.Background())
	if err != nil {
		t.Errorf("无渠道时 StartAll 不应出错: %v", err)
	}
}

func TestManagerStopAllNilCancel(t *testing.T) {
	cfg := config.DefaultConfig()
	m := NewManager(cfg, newTestBus(), testLogger())
	m.StopAll()
}

// ---------------------------------------------------------------------------
// 辅助 mock
// ---------------------------------------------------------------------------

// mockChannel 实现 Channel 接口用于测试。
type mockChannel struct {
	channelName string
	isRunning   bool
}

func (m *mockChannel) Name() string                             { return m.channelName }
func (m *mockChannel) Start(_ context.Context) error            { m.isRunning = true; return nil }
func (m *mockChannel) Stop() error                              { m.isRunning = false; return nil }
func (m *mockChannel) Send(_ bus.OutboundMessage) error         { return nil }
func (m *mockChannel) IsRunning() bool                          { return m.isRunning }

// containsAll 检查字符串是否包含所有子串。
func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !containsSub(s, sub) {
			return false
		}
	}
	return true
}

func containsSub(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// 测试超时常量
const testTimeout = 1000000 // 纳秒 = 1ms
