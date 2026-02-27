// utils 包的单元测试。
// 覆盖所有导出函数：EnsureDir、GetDataPath、GetWorkspacePath、GetSessionsPath、
// GetMemoryPath、GetSkillsPath、TodayDate、Timestamp、TruncateString、
// SafeFilename、ParseSessionKey、BuildSessionKey、expandHome。

package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- EnsureDir 测试 ---

// TestEnsureDir 测试创建多级目录。
func TestEnsureDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b", "c")
	got, err := EnsureDir(dir)
	if err != nil {
		t.Fatalf("EnsureDir(%q) error: %v", dir, err)
	}
	if got != dir {
		t.Errorf("EnsureDir returned %q, want %q", got, dir)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat after EnsureDir: %v", err)
	}
	if !info.IsDir() {
		t.Error("EnsureDir did not create a directory")
	}
}

// TestEnsureDirExisting 测试对已存在的目录调用 EnsureDir。
func TestEnsureDirExisting(t *testing.T) {
	dir := t.TempDir()
	got, err := EnsureDir(dir)
	if err != nil {
		t.Fatalf("EnsureDir on existing dir error: %v", err)
	}
	if got != dir {
		t.Errorf("got %q, want %q", got, dir)
	}
}

// --- GetDataPath 测试 ---

// TestGetDataPath 测试获取数据目录路径。
func TestGetDataPath(t *testing.T) {
	path, err := GetDataPath()
	if err != nil {
		t.Fatalf("GetDataPath error: %v", err)
	}
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".simpleclaw")
	if path != expected {
		t.Errorf("GetDataPath = %q, want %q", path, expected)
	}
}

// --- GetWorkspacePath 测试 ---

// TestGetWorkspacePathDefault 测试默认工作区路径。
func TestGetWorkspacePathDefault(t *testing.T) {
	path, err := GetWorkspacePath("")
	if err != nil {
		t.Fatalf("GetWorkspacePath error: %v", err)
	}
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".simpleclaw", "workspace")
	if path != expected {
		t.Errorf("GetWorkspacePath(\"\") = %q, want %q", path, expected)
	}
}

// TestGetWorkspacePathCustom 测试自定义工作区路径。
func TestGetWorkspacePathCustom(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "myworkspace")
	path, err := GetWorkspacePath(dir)
	if err != nil {
		t.Fatalf("GetWorkspacePath error: %v", err)
	}
	if path != dir {
		t.Errorf("GetWorkspacePath(%q) = %q", dir, path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("directory not created: %v", err)
	}
}

// TestGetWorkspacePathTilde 测试 ~ 前缀展开。
func TestGetWorkspacePathTilde(t *testing.T) {
	path, err := GetWorkspacePath("~/test_ws_simpleclaw")
	if err != nil {
		t.Fatalf("GetWorkspacePath error: %v", err)
	}
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, "test_ws_simpleclaw")
	if path != expected {
		t.Errorf("got %q, want %q", path, expected)
	}
	// 清理
	os.RemoveAll(path)
}

// --- GetSessionsPath 测试 ---

// TestGetSessionsPath 测试获取会话存储目录路径。
func TestGetSessionsPath(t *testing.T) {
	path, err := GetSessionsPath()
	if err != nil {
		t.Fatalf("GetSessionsPath error: %v", err)
	}
	if !strings.HasSuffix(path, filepath.Join(".simpleclaw", "sessions")) {
		t.Errorf("unexpected sessions path: %s", path)
	}
}

// --- GetMemoryPath 测试 ---

// TestGetMemoryPathDefault 测试默认记忆目录路径。
func TestGetMemoryPathDefault(t *testing.T) {
	path, err := GetMemoryPath("")
	if err != nil {
		t.Fatalf("GetMemoryPath error: %v", err)
	}
	if !strings.HasSuffix(path, "memory") {
		t.Errorf("expected path ending with 'memory', got %s", path)
	}
}

// TestGetMemoryPathCustom 测试自定义工作区的记忆目录路径。
func TestGetMemoryPathCustom(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "ws")
	path, err := GetMemoryPath(ws)
	if err != nil {
		t.Fatalf("GetMemoryPath error: %v", err)
	}
	expected := filepath.Join(ws, "memory")
	if path != expected {
		t.Errorf("got %q, want %q", path, expected)
	}
}

// --- GetSkillsPath 测试 ---

// TestGetSkillsPathDefault 测试默认技能目录路径。
func TestGetSkillsPathDefault(t *testing.T) {
	path, err := GetSkillsPath("")
	if err != nil {
		t.Fatalf("GetSkillsPath error: %v", err)
	}
	if !strings.HasSuffix(path, "skills") {
		t.Errorf("expected path ending with 'skills', got %s", path)
	}
}

// TestGetSkillsPathCustom 测试自定义工作区的技能目录路径。
func TestGetSkillsPathCustom(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "ws")
	path, err := GetSkillsPath(ws)
	if err != nil {
		t.Fatalf("GetSkillsPath error: %v", err)
	}
	expected := filepath.Join(ws, "skills")
	if path != expected {
		t.Errorf("got %q, want %q", path, expected)
	}
}

// --- TodayDate 测试 ---

// TestTodayDate 测试日期格式（YYYY-MM-DD）。
func TestTodayDate(t *testing.T) {
	got := TodayDate()
	expected := time.Now().Format("2006-01-02")
	if got != expected {
		t.Errorf("TodayDate() = %q, want %q", got, expected)
	}
	// 验证格式
	if len(got) != 10 || got[4] != '-' || got[7] != '-' {
		t.Errorf("TodayDate format invalid: %q", got)
	}
}

// --- Timestamp 测试 ---

// TestTimestamp 测试时间戳格式（RFC3339）。
func TestTimestamp(t *testing.T) {
	got := Timestamp()
	_, err := time.Parse(time.RFC3339, got)
	if err != nil {
		t.Errorf("Timestamp() = %q, not valid RFC3339: %v", got, err)
	}
}

// --- TruncateString 测试 ---

// TestTruncateStringNoTruncation 测试不需要截断的情况。
func TestTruncateStringNoTruncation(t *testing.T) {
	got := TruncateString("hello", 10, "")
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

// TestTruncateStringExactLength 测试长度恰好等于 maxLen 的情况。
func TestTruncateStringExactLength(t *testing.T) {
	got := TruncateString("hello", 5, "")
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

// TestTruncateStringTruncated 测试需要截断的情况。
func TestTruncateStringTruncated(t *testing.T) {
	got := TruncateString("hello world", 8, "...")
	// 8 - 3 = 5 个字符 + "..." = "hello..."
	if got != "hello..." {
		t.Errorf("got %q, want %q", got, "hello...")
	}
}

// TestTruncateStringCustomSuffix 测试自定义后缀。
func TestTruncateStringCustomSuffix(t *testing.T) {
	got := TruncateString("abcdefghij", 7, "~~")
	// 7 - 2 = 5 个字符 + "~~" = "abcde~~"
	if got != "abcde~~" {
		t.Errorf("got %q, want %q", got, "abcde~~")
	}
}

// TestTruncateStringDefaultSuffix 测试默认后缀（...）。
func TestTruncateStringDefaultSuffix(t *testing.T) {
	got := TruncateString("abcdefghij", 6, "")
	// 默认后缀 "..."，6 - 3 = 3 个字符 + "..." = "abc..."
	if got != "abc..." {
		t.Errorf("got %q, want %q", got, "abc...")
	}
}

// TestTruncateStringMaxLenSmallerThanSuffix 测试 maxLen 小于后缀长度的情况。
func TestTruncateStringMaxLenSmallerThanSuffix(t *testing.T) {
	got := TruncateString("abcdefghij", 2, "...")
	if got != ".." {
		t.Errorf("got %q, want %q", got, "..")
	}
}

// --- SafeFilename 测试 ---

// TestSafeFilename 测试文件名安全转换。
func TestSafeFilename(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"normal.txt", "normal.txt"},
		{"hello world.txt", "hello world.txt"},
		{"file<name>.txt", "file_name_.txt"},
		{`a:b"c/d\e|f?g*h`, "a_b_c_d_e_f_g_h"},
		{"  spaces  ", "spaces"},
		{"no-change", "no-change"},
	}
	for _, tt := range tests {
		got := SafeFilename(tt.input)
		if got != tt.want {
			t.Errorf("SafeFilename(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- ParseSessionKey 测试 ---

// TestParseSessionKey 测试正常的会话键解析。
func TestParseSessionKey(t *testing.T) {
	ch, id, err := ParseSessionKey("telegram:12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ch != "telegram" || id != "12345" {
		t.Errorf("got (%q, %q), want (telegram, 12345)", ch, id)
	}
}

// TestParseSessionKeyWithColonInID 测试 ID 中包含冒号的情况。
func TestParseSessionKeyWithColonInID(t *testing.T) {
	ch, id, err := ParseSessionKey("slack:channel:thread")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ch != "slack" || id != "channel:thread" {
		t.Errorf("got (%q, %q), want (slack, channel:thread)", ch, id)
	}
}

// TestParseSessionKeyInvalid 测试无效的会话键。
func TestParseSessionKeyInvalid(t *testing.T) {
	_, _, err := ParseSessionKey("nocolon")
	if err == nil {
		t.Error("expected error for invalid key, got nil")
	}
}

// TestParseSessionKeyEmpty 测试空的会话键。
func TestParseSessionKeyEmpty(t *testing.T) {
	_, _, err := ParseSessionKey("")
	if err == nil {
		t.Error("expected error for empty key, got nil")
	}
}

// --- BuildSessionKey 测试 ---

// TestBuildSessionKey 测试构建会话键。
func TestBuildSessionKey(t *testing.T) {
	got := BuildSessionKey("telegram", "12345")
	if got != "telegram:12345" {
		t.Errorf("got %q, want %q", got, "telegram:12345")
	}
}

// TestBuildAndParseRoundTrip 测试构建与解析的往返一致性。
func TestBuildAndParseRoundTrip(t *testing.T) {
	key := BuildSessionKey("whatsapp", "user-99")
	ch, id, err := ParseSessionKey(key)
	if err != nil {
		t.Fatalf("round-trip error: %v", err)
	}
	if ch != "whatsapp" || id != "user-99" {
		t.Errorf("round-trip failed: got (%q, %q)", ch, id)
	}
}

// --- expandHome 测试 ---

// TestExpandHome 测试 ~ 路径展开。
func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		input string
		want  string
	}{
		{"~/docs", filepath.Join(home, "docs")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"~", home},
	}
	for _, tt := range tests {
		got := ExpandHome(tt.input)
		if got != tt.want {
			t.Errorf("ExpandHome(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
