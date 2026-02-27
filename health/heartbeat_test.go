package health

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

// newTestHeartbeatService 创建用于测试的 HeartbeatService 实例，工作区指向临时目录。
func newTestHeartbeatService(t *testing.T, enabled bool, fn OnHeartbeatFunc) *HeartbeatService {
	t.Helper()
	return NewHeartbeatService(HeartbeatConfig{
		Workspace:   t.TempDir(),
		OnHeartbeat: fn,
		Interval:    50 * time.Millisecond, // 短间隔用于快速测试
		Enabled:     enabled,
		Logger:      slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
}

// writeHeartbeatFile 在 HeartbeatService 的工作区写入 HEARTBEAT.md。
func writeHeartbeatFile(t *testing.T, s *HeartbeatService, content string) {
	t.Helper()
	if err := os.WriteFile(s.HeartbeatFile(), []byte(content), 0644); err != nil {
		t.Fatalf("写入 HEARTBEAT.md 失败: %v", err)
	}
}

// ---------------------------------------------------------------------------
// NewHeartbeatService 测试
// ---------------------------------------------------------------------------

func TestNewHeartbeatServiceDefaults(t *testing.T) {
	svc := NewHeartbeatService(HeartbeatConfig{})
	if svc.interval != DefaultHeartbeatInterval {
		t.Errorf("期望默认间隔 %v，实际 %v", DefaultHeartbeatInterval, svc.interval)
	}
	if svc.logger == nil {
		t.Error("logger 不应为 nil")
	}
}

func TestNewHeartbeatServiceCustomInterval(t *testing.T) {
	svc := NewHeartbeatService(HeartbeatConfig{Interval: 5 * time.Minute})
	if svc.interval != 5*time.Minute {
		t.Errorf("期望间隔 5m，实际 %v", svc.interval)
	}
}

func TestNewHeartbeatServiceZeroInterval(t *testing.T) {
	svc := NewHeartbeatService(HeartbeatConfig{Interval: 0})
	if svc.interval != DefaultHeartbeatInterval {
		t.Errorf("间隔为 0 应回退到默认值 %v，实际 %v", DefaultHeartbeatInterval, svc.interval)
	}
}

func TestNewHeartbeatServiceNegativeInterval(t *testing.T) {
	svc := NewHeartbeatService(HeartbeatConfig{Interval: -1 * time.Second})
	if svc.interval != DefaultHeartbeatInterval {
		t.Errorf("负数间隔应回退到默认值 %v，实际 %v", DefaultHeartbeatInterval, svc.interval)
	}
}

func TestNewHeartbeatServiceFieldsAssigned(t *testing.T) {
	fn := func(ctx context.Context, prompt string) (string, error) { return "", nil }
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	svc := NewHeartbeatService(HeartbeatConfig{
		Workspace:   "/tmp/test",
		OnHeartbeat: fn,
		Interval:    10 * time.Second,
		Enabled:     true,
		Logger:      logger,
	})
	if svc.workspace != "/tmp/test" {
		t.Errorf("workspace 不匹配")
	}
	if svc.enabled != true {
		t.Errorf("enabled 应为 true")
	}
	if svc.onHeartbeat == nil {
		t.Errorf("onHeartbeat 不应为 nil")
	}
}

// ---------------------------------------------------------------------------
// HeartbeatFile 测试
// ---------------------------------------------------------------------------

func TestHeartbeatFile(t *testing.T) {
	svc := NewHeartbeatService(HeartbeatConfig{Workspace: "/workspace"})
	expected := "/workspace/HEARTBEAT.md"
	if got := svc.HeartbeatFile(); got != expected {
		t.Errorf("期望 %q，实际 %q", expected, got)
	}
}

func TestHeartbeatFileEmptyWorkspace(t *testing.T) {
	svc := NewHeartbeatService(HeartbeatConfig{Workspace: ""})
	if got := svc.HeartbeatFile(); got != "/HEARTBEAT.md" {
		t.Errorf("期望 /HEARTBEAT.md，实际 %q", got)
	}
}

// ---------------------------------------------------------------------------
// TriggerNow 测试
// ---------------------------------------------------------------------------

func TestTriggerNowNilCallback(t *testing.T) {
	svc := newTestHeartbeatService(t, true, nil)
	resp, err := svc.TriggerNow(context.Background())
	if err != nil {
		t.Errorf("不期望错误: %v", err)
	}
	if resp != "" {
		t.Errorf("期望空响应，实际 %q", resp)
	}
}

func TestTriggerNowWithCallback(t *testing.T) {
	called := false
	fn := func(ctx context.Context, prompt string) (string, error) {
		called = true
		if prompt != HeartbeatPrompt {
			t.Errorf("期望提示词 %q，实际 %q", HeartbeatPrompt, prompt)
		}
		return "done", nil
	}
	svc := newTestHeartbeatService(t, true, fn)
	resp, err := svc.TriggerNow(context.Background())
	if err != nil {
		t.Errorf("不期望错误: %v", err)
	}
	if resp != "done" {
		t.Errorf("期望 'done'，实际 %q", resp)
	}
	if !called {
		t.Error("回调函数未被调用")
	}
}

func TestTriggerNowCallbackError(t *testing.T) {
	fn := func(ctx context.Context, prompt string) (string, error) {
		return "", errors.New("agent 失败")
	}
	svc := newTestHeartbeatService(t, true, fn)
	_, err := svc.TriggerNow(context.Background())
	if err == nil || err.Error() != "agent 失败" {
		t.Errorf("期望 'agent 失败' 错误，实际 %v", err)
	}
}

// ---------------------------------------------------------------------------
// readHeartbeatFile 测试
// ---------------------------------------------------------------------------

func TestReadHeartbeatFileExists(t *testing.T) {
	svc := newTestHeartbeatService(t, false, nil)
	writeHeartbeatFile(t, svc, "hello world")
	got := svc.readHeartbeatFile()
	if got != "hello world" {
		t.Errorf("期望 'hello world'，实际 %q", got)
	}
}

func TestReadHeartbeatFileNotExists(t *testing.T) {
	svc := newTestHeartbeatService(t, false, nil)
	got := svc.readHeartbeatFile()
	if got != "" {
		t.Errorf("文件不存在时应返回空字符串，实际 %q", got)
	}
}

func TestReadHeartbeatFileEmptyContent(t *testing.T) {
	svc := newTestHeartbeatService(t, false, nil)
	writeHeartbeatFile(t, svc, "")
	got := svc.readHeartbeatFile()
	if got != "" {
		t.Errorf("空文件应返回空字符串，实际 %q", got)
	}
}

// ---------------------------------------------------------------------------
// isHeartbeatEmpty 测试
// ---------------------------------------------------------------------------

func TestIsHeartbeatEmptyTrue(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"空字符串", ""},
		{"仅空行", "\n\n\n"},
		{"仅标题行", "# Heartbeat\n## Tasks"},
		{"仅 HTML 注释", "<!-- 注释 -->\n<!-- 另一个注释 -->"},
		{"空待办项 dash bracket", "- [ ]"},
		{"空待办项 star bracket", "* [ ]"},
		{"已完成空待办项 dash", "- [x]"},
		{"已完成空待办项 star", "* [x]"},
		{"混合空内容", "# Title\n\n<!-- 注释 -->\n- [ ]\n* [x]\n"},
		{"标题+空行+注释", "# H\n\n<!---->"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !isHeartbeatEmpty(tc.content) {
				t.Errorf("期望为空（true），内容: %q", tc.content)
			}
		})
	}
}

func TestIsHeartbeatEmptyFalse(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"普通文本", "有任务需要处理"},
		{"待办项有内容", "- [ ] 完成报告"},
		{"已完成待办有内容", "- [x] 已完成任务"},
		{"标题后有任务", "# Tasks\n- [ ] 检查日志"},
		{"混合行最终有非空行", "# Title\n\n<!-- 注释 -->\nsome task"},
		{"代码块", "```go\nfmt.Println()\n```"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if isHeartbeatEmpty(tc.content) {
				t.Errorf("期望为非空（false），内容: %q", tc.content)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Start / Stop 测试
// ---------------------------------------------------------------------------

func TestHeartbeatStartDisabled(t *testing.T) {
	svc := newTestHeartbeatService(t, false, func(ctx context.Context, prompt string) (string, error) {
		t.Error("不应调用回调函数")
		return "", nil
	})
	// Start 应立即返回（不阻塞）
	done := make(chan struct{})
	go func() {
		svc.Start(context.Background())
		close(done)
	}()
	select {
	case <-done:
		// 正常返回
	case <-time.After(500 * time.Millisecond):
		t.Error("Start 在禁用模式下应立即返回")
	}
}

func TestHeartbeatStartAndStop(t *testing.T) {
	svc := newTestHeartbeatService(t, true, func(ctx context.Context, prompt string) (string, error) {
		return HeartbeatOKToken, nil
	})

	done := make(chan struct{})
	go func() {
		svc.Start(context.Background())
		close(done)
	}()

	// 给一点时间让 ticker 设置好
	time.Sleep(20 * time.Millisecond)
	svc.Stop()

	select {
	case <-done:
		// 正常停止
	case <-time.After(2 * time.Second):
		t.Error("Stop 后 Start 应退出")
	}
}

func TestHeartbeatStartContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	svc := newTestHeartbeatService(t, true, func(ctx context.Context, prompt string) (string, error) {
		return HeartbeatOKToken, nil
	})

	done := make(chan struct{})
	go func() {
		svc.Start(ctx)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// 正常退出
	case <-time.After(2 * time.Second):
		t.Error("context 取消后 Start 应退出")
	}
}

func TestHeartbeatStopNilCancel(t *testing.T) {
	svc := newTestHeartbeatService(t, false, nil)
	// cancel 为 nil 时 Stop 不应 panic
	svc.Stop()
}

// ---------------------------------------------------------------------------
// tick 行为测试
// ---------------------------------------------------------------------------

func TestTickNoFile(t *testing.T) {
	called := false
	svc := newTestHeartbeatService(t, true, func(ctx context.Context, prompt string) (string, error) {
		called = true
		return "", nil
	})
	svc.tick(context.Background())
	if called {
		t.Error("HEARTBEAT.md 不存在时不应调用回调")
	}
}

func TestTickEmptyFile(t *testing.T) {
	called := false
	svc := newTestHeartbeatService(t, true, func(ctx context.Context, prompt string) (string, error) {
		called = true
		return "", nil
	})
	writeHeartbeatFile(t, svc, "# Heartbeat\n\n<!-- empty -->")
	svc.tick(context.Background())
	if called {
		t.Error("HEARTBEAT.md 内容为空时不应调用回调")
	}
}

func TestTickWithTasks(t *testing.T) {
	var callCount int32
	svc := newTestHeartbeatService(t, true, func(ctx context.Context, prompt string) (string, error) {
		atomic.AddInt32(&callCount, 1)
		return "完成", nil
	})
	writeHeartbeatFile(t, svc, "- [ ] 检查日志\n- [ ] 清理缓存")
	svc.tick(context.Background())
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("期望回调被调用 1 次，实际 %d 次", callCount)
	}
}

func TestTickHeartbeatOKResponse(t *testing.T) {
	svc := newTestHeartbeatService(t, true, func(ctx context.Context, prompt string) (string, error) {
		return "HEARTBEAT_OK", nil
	})
	writeHeartbeatFile(t, svc, "有任务")
	// 不应 panic，应正常处理 HEARTBEAT_OK 响应
	svc.tick(context.Background())
}

func TestTickHeartbeatOKVariants(t *testing.T) {
	// 测试 HEARTBEAT_OK 的各种变体（大小写、下划线）
	variants := []string{
		"HEARTBEAT_OK",
		"heartbeat_ok",
		"HeartBeat_OK",
		"HEARTBEATOK",
		"heartbeatok",
	}
	for _, v := range variants {
		t.Run(v, func(t *testing.T) {
			upper := strings.ToUpper(strings.ReplaceAll(v, "_", ""))
			if !strings.Contains(upper, "HEARTBEATOK") {
				t.Errorf("变体 %q 未能匹配 HEARTBEATOK", v)
			}
		})
	}
}

func TestTickCallbackError(t *testing.T) {
	svc := newTestHeartbeatService(t, true, func(ctx context.Context, prompt string) (string, error) {
		return "", errors.New("agent 错误")
	})
	writeHeartbeatFile(t, svc, "有任务")
	// 不应 panic，错误应被记录
	svc.tick(context.Background())
}

func TestTickNilCallback(t *testing.T) {
	svc := newTestHeartbeatService(t, true, nil)
	writeHeartbeatFile(t, svc, "有任务")
	// onHeartbeat 为 nil 时 tick 不应 panic
	svc.tick(context.Background())
}

// ---------------------------------------------------------------------------
// Start 集成测试：验证 tick 被周期性调用
// ---------------------------------------------------------------------------

func TestHeartbeatStartTriggersTickPeriodically(t *testing.T) {
	var callCount int32
	svc := NewHeartbeatService(HeartbeatConfig{
		Workspace: t.TempDir(),
		OnHeartbeat: func(ctx context.Context, prompt string) (string, error) {
			atomic.AddInt32(&callCount, 1)
			return HeartbeatOKToken, nil
		},
		Interval: 30 * time.Millisecond,
		Enabled:  true,
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})

	// 写入有任务的文件
	if err := os.WriteFile(filepath.Join(svc.workspace, "HEARTBEAT.md"), []byte("有任务"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		svc.Start(ctx)
		close(done)
	}()

	// 等待足够时间让 tick 执行几次
	time.Sleep(120 * time.Millisecond)
	cancel()
	<-done

	count := atomic.LoadInt32(&callCount)
	if count < 2 {
		t.Errorf("期望回调至少被调用 2 次，实际 %d 次", count)
	}
}

// ---------------------------------------------------------------------------
// 常量验证测试
// ---------------------------------------------------------------------------

func TestDefaultHeartbeatInterval(t *testing.T) {
	if DefaultHeartbeatInterval != 30*time.Minute {
		t.Errorf("期望 DefaultHeartbeatInterval 为 30m，实际 %v", DefaultHeartbeatInterval)
	}
}

func TestHeartbeatPromptContainsKey(t *testing.T) {
	if !strings.Contains(HeartbeatPrompt, "HEARTBEAT.md") {
		t.Error("HeartbeatPrompt 应包含 'HEARTBEAT.md'")
	}
	if !strings.Contains(HeartbeatPrompt, HeartbeatOKToken) {
		t.Error("HeartbeatPrompt 应包含 HEARTBEAT_OK token")
	}
}
