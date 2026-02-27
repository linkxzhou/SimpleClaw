package utils

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"
)

// LogLevel 日志级别
type LogLevel = slog.Level

// 预定义日志级别
const (
	LogLevelDebug = slog.LevelDebug
	LogLevelInfo  = slog.LevelInfo
	LogLevelWarn  = slog.LevelWarn
	LogLevelError = slog.LevelError
)

// Logger 日志接口
type Logger interface {
	Debug(args ...interface{})
	Debugf(format string, args ...interface{})
	Info(args ...interface{})
	Infof(format string, args ...interface{})
	Warn(args ...interface{})
	Warnf(format string, args ...interface{})
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
	SetLevel(output string, level LogLevel)
	GetLevel(output string) LogLevel
	Flush()
}

// LogConfig 日志配置
type LogConfig struct {
	// 控制台配置
	ConsoleEnabled bool     `json:"console_enabled"`
	ConsoleLevel   LogLevel `json:"console_level"`

	// 文件配置
	FileEnabled bool     `json:"file_enabled"`
	FileLevel   LogLevel `json:"file_level"`
	Filename    string   `json:"filename"`
	MaxSize     int      `json:"max_size"`    // 单个文件最大大小（MB）
	MaxBackups  int      `json:"max_backups"` // 保留的旧文件数量
	MaxAge      int      `json:"max_age"`     // 保留的最大天数
	Compress    bool     `json:"compress"`    // 是否压缩旧文件
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

// slogLogger 基于 slog 的日志实现
type slogLogger struct {
	mu            sync.RWMutex
	consoleLogger *slog.Logger
	fileLogger    *slog.Logger
	consoleLevel  *slog.LevelVar
	fileLevel     *slog.LevelVar
	fileWriter    io.WriteCloser
}

var (
	DefaultLogger Logger
)

func init() {
	DefaultLogger = NewLogger(DefaultLogConfig())
}

// NewLogger 创建新的日志实例。
// 同时将控制台 handler 设置为 slog.SetDefault()，使标准库 slog 和 utils.Log* 共享同一输出。
func NewLogger(cfg *LogConfig) Logger {
	l := &slogLogger{
		consoleLevel: new(slog.LevelVar),
		fileLevel:    new(slog.LevelVar),
	}

	// 设置控制台日志
	if cfg.ConsoleEnabled {
		l.consoleLevel.Set(cfg.ConsoleLevel)
		consoleHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: l.consoleLevel,
		})
		l.consoleLogger = slog.New(consoleHandler)
		// 同步到 slog.Default()，让通过 slog.Info(...) 等调用的日志也走同一输出
		slog.SetDefault(l.consoleLogger)
	}

	// 设置文件日志
	if cfg.FileEnabled && cfg.Filename != "" {
		// 确保日志目录存在
		dir := filepath.Dir(cfg.Filename)
		if dir != "" && dir != "." {
			_ = os.MkdirAll(dir, 0755)
		}

		l.fileWriter = &lumberjack.Logger{
			Filename:   cfg.Filename,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge,
			Compress:   cfg.Compress,
		}

		l.fileLevel.Set(cfg.FileLevel)
		l.fileLogger = slog.New(slog.NewJSONHandler(l.fileWriter, &slog.HandlerOptions{
			Level: l.fileLevel,
		}))
	}

	return l
}

// InitLog 初始化日志配置
func InitLog(cfg *LogConfig) {
	DefaultLogger = NewLogger(cfg)
}

// SetLogger 设置默认 logger
func SetLogger(logger Logger) {
	DefaultLogger = logger
}

// LogSetLevel 设置日志级别
func LogSetLevel(output string, level LogLevel) {
	DefaultLogger.SetLevel(output, level)
}

// LogGetLevel 获取日志级别
func LogGetLevel(output string) LogLevel {
	return DefaultLogger.GetLevel(output)
}

// LogDebug 记录 DEBUG 级别日志
func LogDebug(args ...interface{}) {
	DefaultLogger.Debug(args...)
}

// LogDebugf 记录 DEBUG 级别日志（格式化）
func LogDebugf(format string, args ...interface{}) {
	DefaultLogger.Debugf(format, args...)
}

// LogInfo 记录 INFO 级别日志
func LogInfo(args ...interface{}) {
	DefaultLogger.Info(args...)
}

// LogInfof 记录 INFO 级别日志（格式化）
func LogInfof(format string, args ...interface{}) {
	DefaultLogger.Infof(format, args...)
}

// LogWarn 记录 WARN 级别日志
func LogWarn(args ...interface{}) {
	DefaultLogger.Warn(args...)
}

// LogWarnf 记录 WARN 级别日志（格式化）
func LogWarnf(format string, args ...interface{}) {
	DefaultLogger.Warnf(format, args...)
}

// LogError 记录 ERROR 级别日志
func LogError(args ...interface{}) {
	DefaultLogger.Error(args...)
}

// LogErrorf 记录 ERROR 级别日志（格式化）
func LogErrorf(format string, args ...interface{}) {
	DefaultLogger.Errorf(format, args...)
}

// LogFatal 记录 FATAL 级别日志并退出程序
func LogFatal(args ...interface{}) {
	DefaultLogger.Fatal(args...)
}

// LogFatalf 记录 FATAL 级别日志并退出程序（格式化）
func LogFatalf(format string, args ...interface{}) {
	DefaultLogger.Fatalf(format, args...)
}

// LogFlush 刷新日志缓冲
func LogFlush() {
	DefaultLogger.Flush()
}

// ============ slogLogger 实现 ============

func (l *slogLogger) log(level slog.Level, msg string) {
	ctx := context.Background()
	if l.consoleLogger != nil {
		l.consoleLogger.Log(ctx, level, msg)
	}
	if l.fileLogger != nil {
		l.fileLogger.Log(ctx, level, msg)
	}
}

func (l *slogLogger) Debug(args ...interface{}) {
	l.log(slog.LevelDebug, fmt.Sprint(args...))
}

func (l *slogLogger) Debugf(format string, args ...interface{}) {
	l.log(slog.LevelDebug, fmt.Sprintf(format, args...))
}

func (l *slogLogger) Info(args ...interface{}) {
	l.log(slog.LevelInfo, fmt.Sprint(args...))
}

func (l *slogLogger) Infof(format string, args ...interface{}) {
	l.log(slog.LevelInfo, fmt.Sprintf(format, args...))
}

func (l *slogLogger) Warn(args ...interface{}) {
	l.log(slog.LevelWarn, fmt.Sprint(args...))
}

func (l *slogLogger) Warnf(format string, args ...interface{}) {
	l.log(slog.LevelWarn, fmt.Sprintf(format, args...))
}

func (l *slogLogger) Error(args ...interface{}) {
	l.log(slog.LevelError, fmt.Sprint(args...))
}

func (l *slogLogger) Errorf(format string, args ...interface{}) {
	l.log(slog.LevelError, fmt.Sprintf(format, args...))
}

func (l *slogLogger) Fatal(args ...interface{}) {
	l.log(slog.LevelError, fmt.Sprint(args...))
	l.Flush()
	os.Exit(1)
}

func (l *slogLogger) Fatalf(format string, args ...interface{}) {
	l.log(slog.LevelError, fmt.Sprintf(format, args...))
	l.Flush()
	os.Exit(1)
}

func (l *slogLogger) SetLevel(output string, level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()

	switch output {
	case "console":
		l.consoleLevel.Set(level)
	case "file":
		l.fileLevel.Set(level)
	default:
		// 设置所有输出的级别
		l.consoleLevel.Set(level)
		l.fileLevel.Set(level)
	}
}

func (l *slogLogger) GetLevel(output string) LogLevel {
	l.mu.RLock()
	defer l.mu.RUnlock()

	switch output {
	case "console":
		return l.consoleLevel.Level()
	case "file":
		return l.fileLevel.Level()
	default:
		return l.consoleLevel.Level()
	}
}

func (l *slogLogger) Flush() {
	// lumberjack.Logger 写入是同步的，无需显式刷新
	// 如果需要关闭文件，可以调用 Close
}

// Close 关闭日志文件
func (l *slogLogger) Close() error {
	if l.fileWriter != nil {
		return l.fileWriter.Close()
	}
	return nil
}
