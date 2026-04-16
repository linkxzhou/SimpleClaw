package utils

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

// LogLevel 日志级别（slog.Level 别名）
type LogLevel = slog.Level

// 预定义日志级别
const (
	LogLevelDebug = slog.LevelDebug
	LogLevelInfo  = slog.LevelInfo
	LogLevelWarn  = slog.LevelWarn
	LogLevelError = slog.LevelError
)

// LogConfig 日志配置
type LogConfig struct {
	ConsoleEnabled bool     `json:"console_enabled"`
	ConsoleLevel   LogLevel `json:"console_level"`

	FileEnabled bool     `json:"file_enabled"`
	FileLevel   LogLevel `json:"file_level"`
	Filename    string   `json:"filename"`
	MaxSize     int      `json:"max_size"`
	MaxBackups  int      `json:"max_backups"`
	MaxAge      int      `json:"max_age"`
	Compress    bool     `json:"compress"`
}

// DefaultLogConfig 默认配置
func DefaultLogConfig() *LogConfig {
	return &LogConfig{
		ConsoleEnabled: true,
		ConsoleLevel:   LogLevelInfo,
		FileEnabled:    true,
		FileLevel:      LogLevelDebug,
		Filename:       "logs/main.log",
		MaxSize:        100,
		MaxBackups:     10,
		MaxAge:         7,
		Compress:       true,
	}
}

// InitLog 初始化日志：设置 slog.SetDefault()，使全局 slog 调用生效。
// 可选同时写入控制台（TextHandler）和文件（JSONHandler + lumberjack rotation）。
func InitLog(cfg *LogConfig) {
	var handlers []slog.Handler

	if cfg.ConsoleEnabled {
		lv := new(slog.LevelVar)
		lv.Set(cfg.ConsoleLevel)
		handlers = append(handlers, slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lv}))
	}

	if cfg.FileEnabled && cfg.Filename != "" {
		dir := filepath.Dir(cfg.Filename)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				slog.Warn("log: failed to create log dir", "dir", dir, "error", err)
			}
		}

		w := &lumberjack.Logger{
			Filename:   cfg.Filename,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge,
			Compress:   cfg.Compress,
		}

		lv := new(slog.LevelVar)
		lv.Set(cfg.FileLevel)
		handlers = append(handlers, slog.NewJSONHandler(w, &slog.HandlerOptions{Level: lv}))
	}

	switch len(handlers) {
	case 0:
		// 保持 slog 默认
	case 1:
		slog.SetDefault(slog.New(handlers[0]))
	default:
		slog.SetDefault(slog.New(&multiHandler{handlers: handlers}))
	}
}

// multiHandler 将日志同时写入多个 Handler。
type multiHandler struct {
	handlers []slog.Handler
}

func (h *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, hh := range h.handlers {
		if hh.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, hh := range h.handlers {
		if hh.Enabled(ctx, r.Level) {
			if err := hh.Handle(ctx, r.Clone()); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	hs := make([]slog.Handler, len(h.handlers))
	for i, hh := range h.handlers {
		hs[i] = hh.WithAttrs(attrs)
	}
	return &multiHandler{handlers: hs}
}

func (h *multiHandler) WithGroup(name string) slog.Handler {
	hs := make([]slog.Handler, len(h.handlers))
	for i, hh := range h.handlers {
		hs[i] = hh.WithGroup(name)
	}
	return &multiHandler{handlers: hs}
}
