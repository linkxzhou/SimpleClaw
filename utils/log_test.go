package utils

import (
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

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
	if LogLevelDebug >= LogLevelInfo {
		t.Error("LogLevelDebug should be less than LogLevelInfo")
	}
}

// TestInitLogConsoleOnly 测试仅控制台初始化
func TestInitLogConsoleOnly(t *testing.T) {
	cfg := &LogConfig{
		ConsoleEnabled: true,
		ConsoleLevel:   LogLevelInfo,
		FileEnabled:    false,
	}
	// 不应 panic
	InitLog(cfg)

	// slog.Default() 应可用
	slog.Info("test console only")
}

// TestInitLogWithFile 测试带文件输出的初始化
func TestInitLogWithFile(t *testing.T) {
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

	InitLog(cfg)
	slog.Info("test file log")

	// 验证文件已创建（lumberjack 懒创建，写入后才有文件）
	info, err := os.Stat(logFile)
	if err != nil {
		t.Fatalf("Log file should be created: %v", err)
	}
	if info.Size() == 0 {
		t.Error("Log file should not be empty")
	}
}

// TestInitLogBothOutputs 测试同时启用控制台和文件
func TestInitLogBothOutputs(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "both.log")

	cfg := &LogConfig{
		ConsoleEnabled: true,
		ConsoleLevel:   LogLevelInfo,
		FileEnabled:    true,
		FileLevel:      LogLevelDebug,
		Filename:       logFile,
		MaxSize:        1,
		MaxBackups:     1,
		MaxAge:         1,
		Compress:       false,
	}

	InitLog(cfg)
	slog.Info("dual output test")

	// 文件应有内容
	info, err := os.Stat(logFile)
	if err != nil {
		t.Fatalf("Log file should be created: %v", err)
	}
	if info.Size() == 0 {
		t.Error("Log file should not be empty")
	}
}

// TestInitLogNestedDir 测试日志目录自动创建
func TestInitLogNestedDir(t *testing.T) {
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

	InitLog(cfg)
	slog.Info("nested dir test")

	if _, err := os.Stat(nestedDir); os.IsNotExist(err) {
		t.Error("Nested directory should be created")
	}
}

// TestInitLogNone 测试两个都禁用
func TestInitLogNone(t *testing.T) {
	cfg := &LogConfig{
		ConsoleEnabled: false,
		FileEnabled:    false,
	}
	// 不应 panic，保持 slog 默认
	InitLog(cfg)
	slog.Info("should use slog default")
}

// TestConcurrentLogging 测试并发日志写入
func TestConcurrentLogging(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "concurrent.log")

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

	InitLog(cfg)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				slog.Info("concurrent", "goroutine", id, "iter", j)
			}
		}(i)
	}
	wg.Wait()

	info, err := os.Stat(logFile)
	if err != nil {
		t.Fatalf("Log file should exist: %v", err)
	}
	if info.Size() == 0 {
		t.Error("Log file should not be empty")
	}
}
