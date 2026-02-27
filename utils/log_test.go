package utils

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// testWriter 用于捕获日志输出的测试 writer
type testWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *testWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *testWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func (w *testWriter) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Reset()
}

// newTestLogger 创建用于测试的日志实例
func newTestLogger(w io.Writer, level LogLevel) *slogLogger {
	levelVar := new(slog.LevelVar)
	levelVar.Set(level)
	return &slogLogger{
		consoleLevel: levelVar,
		fileLevel:    new(slog.LevelVar),
		consoleLogger: slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
			Level: levelVar,
		})),
	}
}

// TestDefaultLogConfig 测试默认配置
func TestDefaultLogConfig(t *testing.T) {
	cfg := DefaultLogConfig()

	if !cfg.ConsoleEnabled {
		t.Error("ConsoleEnabled should be true by default")
	}
	if cfg.ConsoleLevel != LogLevelInfo {
		t.Errorf("ConsoleLevel should be LogLevelInfo, got %v", cfg.ConsoleLevel)
	}
	if !cfg.FileEnabled {
		t.Error("FileEnabled should be true by default")
	}
	if cfg.FileLevel != LogLevelDebug {
		t.Errorf("FileLevel should be LogLevelDebug, got %v", cfg.FileLevel)
	}
	if cfg.Filename != "logs/main.log" {
		t.Errorf("Filename should be 'logs/main.log', got %s", cfg.Filename)
	}
	if cfg.MaxSize != 100 {
		t.Errorf("MaxSize should be 100, got %d", cfg.MaxSize)
	}
	if cfg.MaxBackups != 10 {
		t.Errorf("MaxBackups should be 10, got %d", cfg.MaxBackups)
	}
	if cfg.MaxAge != 7 {
		t.Errorf("MaxAge should be 7, got %d", cfg.MaxAge)
	}
	if !cfg.Compress {
		t.Error("Compress should be true by default")
	}
}

// TestNewLogger 测试创建日志实例
func TestNewLogger(t *testing.T) {
	// 测试仅控制台
	cfg := &LogConfig{
		ConsoleEnabled: true,
		ConsoleLevel:   LogLevelInfo,
		FileEnabled:    false,
	}
	logger := NewLogger(cfg)
	if logger == nil {
		t.Fatal("NewLogger should return a logger")
	}

	sl := logger.(*slogLogger)
	if sl.consoleLogger == nil {
		t.Error("consoleLogger should not be nil")
	}
	if sl.fileLogger != nil {
		t.Error("fileLogger should be nil when FileEnabled is false")
	}
}

// TestNewLoggerWithFile 测试创建带文件输出的日志实例
func TestNewLoggerWithFile(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	cfg := &LogConfig{
		ConsoleEnabled: false,
		FileEnabled:    true,
		FileLevel:      LogLevelDebug,
		Filename:       logFile,
		MaxSize:        1,
		MaxBackups:     1,
		MaxAge:         1,
		Compress:       false,
	}

	logger := NewLogger(cfg)
	sl := logger.(*slogLogger)

	if sl.consoleLogger != nil {
		t.Error("consoleLogger should be nil when ConsoleEnabled is false")
	}
	if sl.fileLogger == nil {
		t.Error("fileLogger should not be nil")
	}
	if sl.fileWriter == nil {
		t.Error("fileWriter should not be nil")
	}

	// 写入日志
	logger.Info("test message")

	// 关闭并验证文件
	sl.Close()

	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if !strings.Contains(string(content), "test message") {
		t.Error("Log file should contain 'test message'")
	}
}

// TestLogLevels 测试各个日志级别
func TestLogLevels(t *testing.T) {
	tests := []struct {
		name     string
		level    LogLevel
		logFunc  func(Logger)
		contains string
	}{
		{
			name:     "Debug",
			level:    LogLevelDebug,
			logFunc:  func(l Logger) { l.Debug("debug message") },
			contains: "debug message",
		},
		{
			name:     "Debugf",
			level:    LogLevelDebug,
			logFunc:  func(l Logger) { l.Debugf("debug %s %d", "test", 123) },
			contains: "debug test 123",
		},
		{
			name:     "Info",
			level:    LogLevelDebug,
			logFunc:  func(l Logger) { l.Info("info message") },
			contains: "info message",
		},
		{
			name:     "Infof",
			level:    LogLevelDebug,
			logFunc:  func(l Logger) { l.Infof("info %s", "formatted") },
			contains: "info formatted",
		},
		{
			name:     "Warn",
			level:    LogLevelDebug,
			logFunc:  func(l Logger) { l.Warn("warn message") },
			contains: "warn message",
		},
		{
			name:     "Warnf",
			level:    LogLevelDebug,
			logFunc:  func(l Logger) { l.Warnf("warn %d", 456) },
			contains: "warn 456",
		},
		{
			name:     "Error",
			level:    LogLevelDebug,
			logFunc:  func(l Logger) { l.Error("error message") },
			contains: "error message",
		},
		{
			name:     "Errorf",
			level:    LogLevelDebug,
			logFunc:  func(l Logger) { l.Errorf("error %v", "details") },
			contains: "error details",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &testWriter{}
			logger := newTestLogger(w, tt.level)
			tt.logFunc(logger)

			output := w.String()
			if !strings.Contains(output, tt.contains) {
				t.Errorf("Expected output to contain %q, got %q", tt.contains, output)
			}
		})
	}
}

// TestLogLevelFiltering 测试日志级别过滤
func TestLogLevelFiltering(t *testing.T) {
	w := &testWriter{}
	logger := newTestLogger(w, LogLevelWarn)

	// Debug 和 Info 应该被过滤
	logger.Debug("debug should be filtered")
	logger.Info("info should be filtered")

	output := w.String()
	if strings.Contains(output, "debug should be filtered") {
		t.Error("Debug message should be filtered at Warn level")
	}
	if strings.Contains(output, "info should be filtered") {
		t.Error("Info message should be filtered at Warn level")
	}

	// Warn 和 Error 应该输出
	logger.Warn("warn should appear")
	logger.Error("error should appear")

	output = w.String()
	if !strings.Contains(output, "warn should appear") {
		t.Error("Warn message should appear at Warn level")
	}
	if !strings.Contains(output, "error should appear") {
		t.Error("Error message should appear at Warn level")
	}
}

// TestLogSetLevel 测试动态设置日志级别
func TestLogSetLevel(t *testing.T) {
	w := &testWriter{}
	logger := newTestLogger(w, LogLevelDebug)

	// 初始级别为 Debug
	logger.Debug("debug visible")
	if !strings.Contains(w.String(), "debug visible") {
		t.Error("Debug should be visible at Debug level")
	}

	w.Reset()

	// 提高级别到 Error
	logger.SetLevel("console", LogLevelError)

	logger.Debug("debug hidden")
	logger.Info("info hidden")
	logger.Warn("warn hidden")

	output := w.String()
	if strings.Contains(output, "debug hidden") ||
		strings.Contains(output, "info hidden") ||
		strings.Contains(output, "warn hidden") {
		t.Error("Lower level messages should be hidden after SetLevel")
	}

	// Error 应该可见
	logger.Error("error visible")
	if !strings.Contains(w.String(), "error visible") {
		t.Error("Error should be visible at Error level")
	}
}

// TestLogGetLevel 测试获取日志级别
func TestLogGetLevel(t *testing.T) {
	w := &testWriter{}
	logger := newTestLogger(w, LogLevelInfo)

	level := logger.GetLevel("console")
	if level != LogLevelInfo {
		t.Errorf("Expected LogLevelInfo, got %v", level)
	}

	logger.SetLevel("console", LogLevelWarn)
	level = logger.GetLevel("console")
	if level != LogLevelWarn {
		t.Errorf("Expected LogLevelWarn after SetLevel, got %v", level)
	}
}

// TestDefaultLoggerInit 测试默认日志实例
func TestDefaultLoggerInit(t *testing.T) {
	if DefaultLogger == nil {
		t.Fatal("DefaultLogger should not be nil after init")
	}
}

// TestInitLog 测试 InitLog 函数
func TestInitLog(t *testing.T) {
	oldLogger := DefaultLogger
	defer func() { DefaultLogger = oldLogger }()

	cfg := &LogConfig{
		ConsoleEnabled: true,
		ConsoleLevel:   LogLevelError,
		FileEnabled:    false,
	}
	InitLog(cfg)

	if DefaultLogger == nil {
		t.Error("DefaultLogger should not be nil after InitLog")
	}
}

// TestSetLoggerFunc 测试 SetLogger 函数
func TestSetLoggerFunc(t *testing.T) {
	oldLogger := DefaultLogger
	defer func() { DefaultLogger = oldLogger }()

	w := &testWriter{}
	newLog := newTestLogger(w, LogLevelDebug)
	SetLogger(newLog)

	if DefaultLogger != newLog {
		t.Error("SetLogger should update DefaultLogger")
	}
}

// TestGlobalLogFunctions 测试全局日志函数
func TestGlobalLogFunctions(t *testing.T) {
	oldLogger := DefaultLogger
	defer func() { DefaultLogger = oldLogger }()

	w := &testWriter{}
	SetLogger(newTestLogger(w, LogLevelDebug))

	// 测试各个全局函数
	LogDebug("global debug")
	if !strings.Contains(w.String(), "global debug") {
		t.Error("Global LogDebug should work")
	}

	w.Reset()
	LogDebugf("global debugf %d", 1)
	if !strings.Contains(w.String(), "global debugf 1") {
		t.Error("Global LogDebugf should work")
	}

	w.Reset()
	LogInfo("global info")
	if !strings.Contains(w.String(), "global info") {
		t.Error("Global LogInfo should work")
	}

	w.Reset()
	LogInfof("global infof %s", "test")
	if !strings.Contains(w.String(), "global infof test") {
		t.Error("Global LogInfof should work")
	}

	w.Reset()
	LogWarn("global warn")
	if !strings.Contains(w.String(), "global warn") {
		t.Error("Global LogWarn should work")
	}

	w.Reset()
	LogWarnf("global warnf %v", true)
	if !strings.Contains(w.String(), "global warnf true") {
		t.Error("Global LogWarnf should work")
	}

	w.Reset()
	LogError("global error")
	if !strings.Contains(w.String(), "global error") {
		t.Error("Global LogError should work")
	}

	w.Reset()
	LogErrorf("global errorf %f", 3.14)
	if !strings.Contains(w.String(), "global errorf") {
		t.Error("Global LogErrorf should work")
	}
}

// TestGlobalLogSetLevel 测试全局 LogSetLevel/LogGetLevel
func TestGlobalLogSetLevel(t *testing.T) {
	oldLogger := DefaultLogger
	defer func() { DefaultLogger = oldLogger }()

	w := &testWriter{}
	SetLogger(newTestLogger(w, LogLevelDebug))

	LogSetLevel("console", LogLevelWarn)
	level := LogGetLevel("console")
	if level != LogLevelWarn {
		t.Errorf("Expected LogLevelWarn, got %v", level)
	}
}

// TestLogFlush 测试 LogFlush 函数
func TestLogFlush(t *testing.T) {
	oldLogger := DefaultLogger
	defer func() { DefaultLogger = oldLogger }()

	w := &testWriter{}
	SetLogger(newTestLogger(w, LogLevelDebug))

	// LogFlush 应该不会 panic
	LogFlush()
}

// TestLogClose 测试 Close 函数
func TestLogClose(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "close_test.log")

	cfg := &LogConfig{
		ConsoleEnabled: false,
		FileEnabled:    true,
		FileLevel:      LogLevelDebug,
		Filename:       logFile,
		MaxSize:        1,
		MaxBackups:     1,
		MaxAge:         1,
		Compress:       false,
	}

	logger := NewLogger(cfg)
	sl := logger.(*slogLogger)

	logger.Info("before close")
	err := sl.Close()
	if err != nil {
		t.Errorf("Close should not return error: %v", err)
	}
}

// TestLogCloseNilWriter 测试 Close 无文件 writer 的情况
func TestLogCloseNilWriter(t *testing.T) {
	cfg := &LogConfig{
		ConsoleEnabled: true,
		FileEnabled:    false,
	}

	logger := NewLogger(cfg)
	sl := logger.(*slogLogger)

	err := sl.Close()
	if err != nil {
		t.Errorf("Close should return nil for nil writer: %v", err)
	}
}

// TestFileLogJSONFormat 测试文件日志 JSON 格式
func TestFileLogJSONFormat(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "json_test.log")

	cfg := &LogConfig{
		ConsoleEnabled: false,
		FileEnabled:    true,
		FileLevel:      LogLevelDebug,
		Filename:       logFile,
		MaxSize:        1,
		MaxBackups:     1,
		MaxAge:         1,
		Compress:       false,
	}

	logger := NewLogger(cfg)
	logger.Info("json format test")
	logger.(*slogLogger).Close()

	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	// 验证是有效的 JSON
	var logEntry map[string]interface{}
	if err := json.Unmarshal(content, &logEntry); err != nil {
		t.Errorf("Log entry should be valid JSON: %v, content: %s", err, content)
	}

	// 验证包含必要字段
	if _, ok := logEntry["time"]; !ok {
		t.Error("JSON log should contain 'time' field")
	}
	if _, ok := logEntry["level"]; !ok {
		t.Error("JSON log should contain 'level' field")
	}
	if _, ok := logEntry["msg"]; !ok {
		t.Error("JSON log should contain 'msg' field")
	}
}

// TestConcurrentLogging 测试并发日志写入
func TestConcurrentLogging(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "concurrent_test.log")

	cfg := &LogConfig{
		ConsoleEnabled: false,
		FileEnabled:    true,
		FileLevel:      LogLevelDebug,
		Filename:       logFile,
		MaxSize:        10,
		MaxBackups:     1,
		MaxAge:         1,
		Compress:       false,
	}

	logger := NewLogger(cfg)

	var wg sync.WaitGroup
	numGoroutines := 10
	numLogs := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numLogs; j++ {
				logger.Infof("goroutine %d log %d", id, j)
			}
		}(i)
	}

	wg.Wait()
	logger.(*slogLogger).Close()

	// 验证文件存在且有内容
	info, err := os.Stat(logFile)
	if err != nil {
		t.Fatalf("Log file should exist: %v", err)
	}
	if info.Size() == 0 {
		t.Error("Log file should not be empty")
	}
}

// TestConcurrentLogSetLevel 测试并发设置日志级别
func TestConcurrentLogSetLevel(t *testing.T) {
	w := &testWriter{}
	logger := newTestLogger(w, LogLevelDebug)

	var wg sync.WaitGroup
	levels := []LogLevel{LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError}

	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for _, level := range levels {
				logger.SetLevel("console", level)
			}
		}()
		go func() {
			defer wg.Done()
			for range levels {
				_ = logger.GetLevel("console")
			}
		}()
	}

	wg.Wait()
}

// TestLogDirectoryCreation 测试日志目录自动创建
func TestLogDirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "a", "b", "c")
	logFile := filepath.Join(nestedDir, "nested.log")

	cfg := &LogConfig{
		ConsoleEnabled: false,
		FileEnabled:    true,
		FileLevel:      LogLevelDebug,
		Filename:       logFile,
		MaxSize:        1,
		MaxBackups:     1,
		MaxAge:         1,
		Compress:       false,
	}

	logger := NewLogger(cfg)
	logger.Info("test nested directory")
	logger.(*slogLogger).Close()

	// 验证目录和文件已创建
	if _, err := os.Stat(nestedDir); os.IsNotExist(err) {
		t.Error("Nested directory should be created")
	}
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Error("Log file should be created")
	}
}

// TestLogLevelConstants 测试日志级别常量
func TestLogLevelConstants(t *testing.T) {
	if LogLevelDebug != slog.LevelDebug {
		t.Error("LogLevelDebug should equal slog.LevelDebug")
	}
	if LogLevelInfo != slog.LevelInfo {
		t.Error("LogLevelInfo should equal slog.LevelInfo")
	}
	if LogLevelWarn != slog.LevelWarn {
		t.Error("LogLevelWarn should equal slog.LevelWarn")
	}
	if LogLevelError != slog.LevelError {
		t.Error("LogLevelError should equal slog.LevelError")
	}

	// 验证级别顺序
	if LogLevelDebug >= LogLevelInfo {
		t.Error("LogLevelDebug should be less than LogLevelInfo")
	}
	if LogLevelInfo >= LogLevelWarn {
		t.Error("LogLevelInfo should be less than LogLevelWarn")
	}
	if LogLevelWarn >= LogLevelError {
		t.Error("LogLevelWarn should be less than LogLevelError")
	}
}

// TestLogSetLevelAllOutputs 测试设置所有输出的级别
func TestLogSetLevelAllOutputs(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "all_outputs.log")

	cfg := &LogConfig{
		ConsoleEnabled: true,
		ConsoleLevel:   LogLevelDebug,
		FileEnabled:    true,
		FileLevel:      LogLevelDebug,
		Filename:       logFile,
		MaxSize:        1,
		MaxBackups:     1,
		MaxAge:         1,
		Compress:       false,
	}

	logger := NewLogger(cfg)
	sl := logger.(*slogLogger)

	// 使用空字符串设置所有输出
	logger.SetLevel("", LogLevelError)

	if sl.consoleLevel.Level() != LogLevelError {
		t.Error("Console level should be Error")
	}
	if sl.fileLevel.Level() != LogLevelError {
		t.Error("File level should be Error")
	}

	sl.Close()
}

// TestLogGetLevelDefault 测试 GetLevel 默认返回值
func TestLogGetLevelDefault(t *testing.T) {
	w := &testWriter{}
	logger := newTestLogger(w, LogLevelWarn)

	// 使用未知输出类型应返回控制台级别
	level := logger.GetLevel("unknown")
	if level != LogLevelWarn {
		t.Errorf("GetLevel with unknown output should return console level, got %v", level)
	}
}

// BenchmarkLogInfo 基准测试 Info 日志
func BenchmarkLogInfo(b *testing.B) {
	w := &testWriter{}
	logger := newTestLogger(w, LogLevelDebug)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("benchmark test message")
	}
}

// BenchmarkLogInfof 基准测试 Infof 日志
func BenchmarkLogInfof(b *testing.B) {
	w := &testWriter{}
	logger := newTestLogger(w, LogLevelDebug)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Infof("benchmark test %s %d", "message", i)
	}
}

// BenchmarkLogFiltered 基准测试被过滤的日志
func BenchmarkLogFiltered(b *testing.B) {
	w := &testWriter{}
	logger := newTestLogger(w, LogLevelError)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Debug("this will be filtered")
	}
}

// BenchmarkConcurrentLog 基准测试并发日志
func BenchmarkConcurrentLog(b *testing.B) {
	w := &testWriter{}
	logger := newTestLogger(w, LogLevelDebug)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info("concurrent benchmark message")
		}
	})
}

// ExampleNewLogger 示例：创建自定义日志实例
func ExampleNewLogger() {
	cfg := &LogConfig{
		ConsoleEnabled: true,
		ConsoleLevel:   LogLevelInfo,
		FileEnabled:    false,
	}
	logger := NewLogger(cfg)
	logger.Info("Hello, world!")
}

// ExampleInitLog 示例：初始化默认日志
func ExampleInitLog() {
	cfg := &LogConfig{
		ConsoleEnabled: true,
		ConsoleLevel:   LogLevelDebug,
		FileEnabled:    true,
		FileLevel:      LogLevelInfo,
		Filename:       "logs/app.log",
		MaxSize:        50,
		MaxBackups:     5,
		MaxAge:         30,
		Compress:       true,
	}
	InitLog(cfg)
	LogInfo("Application started")
}

// TestLogMultipleArguments 测试多参数日志
func TestLogMultipleArguments(t *testing.T) {
	w := &testWriter{}
	logger := newTestLogger(w, LogLevelDebug)

	logger.Debug("arg1", "arg2", "arg3")
	output := w.String()
	if !strings.Contains(output, "arg1arg2arg3") {
		t.Errorf("Multiple arguments should be concatenated, got: %s", output)
	}
}

// TestLogEmptyMessage 测试空消息
func TestLogEmptyMessage(t *testing.T) {
	w := &testWriter{}
	logger := newTestLogger(w, LogLevelDebug)

	// 空消息不应该 panic
	logger.Info()
	logger.Infof("")
}

// TestLogSpecialCharacters 测试特殊字符
func TestLogSpecialCharacters(t *testing.T) {
	w := &testWriter{}
	logger := newTestLogger(w, LogLevelDebug)

	specialChars := "特殊字符 !@#$%^&*() 中文 日本語 🎉"
	logger.Info(specialChars)

	if !strings.Contains(w.String(), specialChars) {
		t.Error("Special characters should be preserved in log output")
	}
}

// TestLogTiming 测试日志时间戳
func TestLogTiming(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "timing.log")

	cfg := &LogConfig{
		ConsoleEnabled: false,
		FileEnabled:    true,
		FileLevel:      LogLevelDebug,
		Filename:       logFile,
		MaxSize:        1,
		MaxBackups:     1,
		MaxAge:         1,
		Compress:       false,
	}

	before := time.Now()
	logger := NewLogger(cfg)
	logger.Info("timing test")
	after := time.Now()
	logger.(*slogLogger).Close()

	content, _ := os.ReadFile(logFile)
	var logEntry map[string]interface{}
	json.Unmarshal(content, &logEntry)

	timeStr, ok := logEntry["time"].(string)
	if !ok {
		t.Fatal("Log should contain time field as string")
	}

	logTime, err := time.Parse(time.RFC3339Nano, timeStr)
	if err != nil {
		t.Fatalf("Failed to parse log time: %v", err)
	}

	if logTime.Before(before) || logTime.After(after) {
		t.Error("Log time should be between before and after timestamps")
	}
}
