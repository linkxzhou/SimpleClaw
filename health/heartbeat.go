// 心跳服务：Agent 的周期性唤醒机制。
// 定时读取工作区中的 HEARTBEAT.md 文件，如果发现待处理任务，
// 则唤醒 Agent 进行处理。Agent 回复 HEARTBEAT_OK 表示无事可做。

package health

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"
)

const (
	// DefaultHeartbeatInterval 默认心跳间隔（30 分钟）。
	DefaultHeartbeatInterval = 30 * time.Minute

	// HeartbeatPrompt 心跳触发时发送给 Agent 的提示词。
	HeartbeatPrompt = `Read HEARTBEAT.md in your workspace (if it exists).
Follow any instructions or tasks listed there.
If nothing needs attention, reply with just: HEARTBEAT_OK`

	// HeartbeatOKToken 表示"无事可做"的标记。
	HeartbeatOKToken = "HEARTBEAT_OK"
)

// OnHeartbeatFunc 心跳触发时的回调函数。
// 接收提示词，返回 Agent 的响应文本。
type OnHeartbeatFunc func(ctx context.Context, prompt string) (string, error)

// HeartbeatService 周期性心跳服务，定时唤醒 Agent 检查待办任务。
type HeartbeatService struct {
	workspace   string          // 工作区路径
	onHeartbeat OnHeartbeatFunc // 心跳回调
	interval    time.Duration   // 检查间隔
	enabled     bool            // 是否启用
	logger      *slog.Logger    // 日志记录器

	cancel context.CancelFunc // 用于停止服务
}

// HeartbeatConfig 心跳服务的配置参数。
type HeartbeatConfig struct {
	Workspace   string          // 工作区路径
	OnHeartbeat OnHeartbeatFunc // 心跳回调
	Interval    time.Duration   // 检查间隔
	Enabled     bool            // 是否启用
	Logger      *slog.Logger    // 日志记录器
}

// NewHeartbeatService 创建新的心跳服务实例。
func NewHeartbeatService(cfg HeartbeatConfig) *HeartbeatService {
	if cfg.Interval <= 0 {
		cfg.Interval = DefaultHeartbeatInterval
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &HeartbeatService{
		workspace:   cfg.Workspace,
		onHeartbeat: cfg.OnHeartbeat,
		interval:    cfg.Interval,
		enabled:     cfg.Enabled,
		logger:      cfg.Logger,
	}
}

// HeartbeatFile 返回 HEARTBEAT.md 文件的完整路径。
func (s *HeartbeatService) HeartbeatFile() string {
	return s.workspace + "/HEARTBEAT.md"
}

// Start 启动心跳服务。阻塞直到 Stop 被调用或 ctx 取消。
func (s *HeartbeatService) Start(ctx context.Context) {
	if !s.enabled {
		s.logger.Info("heartbeat disabled")
		return
	}

	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	s.logger.Info("heartbeat started", "interval", s.interval.String())

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.tick(ctx)
		case <-ctx.Done():
			s.logger.Info("heartbeat stopped")
			return
		}
	}
}

// Stop 停止心跳服务。
func (s *HeartbeatService) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

// TriggerNow 手动触发一次心跳检查。
func (s *HeartbeatService) TriggerNow(ctx context.Context) (string, error) {
	if s.onHeartbeat == nil {
		return "", nil
	}
	return s.onHeartbeat(ctx, HeartbeatPrompt)
}

// tick 执行一次心跳检查。
// 读取 HEARTBEAT.md，如果内容非空且有可执行任务，则唤醒 Agent。
func (s *HeartbeatService) tick(ctx context.Context) {
	content := s.readHeartbeatFile()
	if isHeartbeatEmpty(content) {
		s.logger.Debug("heartbeat: no tasks (HEARTBEAT.md empty)")
		return
	}

	s.logger.Info("heartbeat: checking for tasks...")

	if s.onHeartbeat == nil {
		return
	}

	response, err := s.onHeartbeat(ctx, HeartbeatPrompt)
	if err != nil {
		s.logger.Error("heartbeat execution failed", "error", err)
		return
	}

	// 判断 Agent 是否回复了 HEARTBEAT_OK（忽略下划线和大小写）
	upper := strings.ToUpper(strings.ReplaceAll(response, "_", ""))
	if strings.Contains(upper, "HEARTBEATOK") {
		s.logger.Info("heartbeat: OK (no action needed)")
	} else {
		s.logger.Info("heartbeat: completed task")
	}
}

// readHeartbeatFile 读取 HEARTBEAT.md 文件内容。
// 文件不存在或读取失败时返回空字符串。
func (s *HeartbeatService) readHeartbeatFile() string {
	data, err := os.ReadFile(s.HeartbeatFile())
	if err != nil {
		return ""
	}
	return string(data)
}

// isHeartbeatEmpty 判断 HEARTBEAT.md 是否没有可执行的内容。
// 空行、标题行（#）、HTML 注释行（<!--）以及空的 Markdown 待办项
// 都视为无内容。
func isHeartbeatEmpty(content string) bool {
	if content == "" {
		return true
	}
	skipPrefixes := []string{"- [ ]", "* [ ]", "- [x]", "* [x]"}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "<!--") {
			continue
		}
		skip := false
		for _, p := range skipPrefixes {
			if line == p {
				skip = true
				break
			}
		}
		if !skip {
			return false
		}
	}
	return true
}
