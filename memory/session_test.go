// memory 包的会话相关单元测试。
// 覆盖 Session 和 SessionManager 的所有导出方法，包括：
// - Session: NewSession、AddMessage、GetHistory、Clear
// - SessionManager: NewSessionManager、GetOrCreate、Save、Delete、ListSessions
// - 持久化: JSONL 格式的保存与加载往返测试
// - 并发安全: 多 goroutine 并发读写测试
// - 边界条件: 空会话、大量消息、不存在的会话等

package memory

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/linkxzhou/SimpleClaw/utils"
	"time"
)

// ============ Session 测试 ============

// TestNewSession 测试创建新会话。
func TestNewSession(t *testing.T) {
	s := NewSession("telegram:12345")

	if s.Key != "telegram:12345" {
		t.Errorf("Key = %q, want %q", s.Key, "telegram:12345")
	}
	if len(s.Messages) != 0 {
		t.Errorf("Messages should be empty, got %d", len(s.Messages))
	}
	if s.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if s.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero")
	}
	if s.Metadata == nil {
		t.Error("Metadata should not be nil")
	}
}

// TestAddMessage 测试添加消息。
func TestAddMessage(t *testing.T) {
	s := NewSession("test:1")
	before := s.UpdatedAt

	time.Sleep(time.Millisecond)
	s.AddMessage("user", "hello", nil)

	if len(s.Messages) != 1 {
		t.Fatalf("Messages count = %d, want 1", len(s.Messages))
	}
	msg := s.Messages[0]
	if msg["role"] != "user" {
		t.Errorf("role = %v, want user", msg["role"])
	}
	if msg["content"] != "hello" {
		t.Errorf("content = %v, want hello", msg["content"])
	}
	if msg["timestamp"] == nil {
		t.Error("timestamp should be set")
	}
	if !s.UpdatedAt.After(before) {
		t.Error("UpdatedAt should be updated")
	}
}

// TestAddMessageWithExtra 测试添加带额外字段的消息。
func TestAddMessageWithExtra(t *testing.T) {
	s := NewSession("test:1")
	extra := map[string]interface{}{
		"tool_call_id": "call_123",
		"name":         "search",
	}
	s.AddMessage("tool", "result", extra)

	if len(s.Messages) != 1 {
		t.Fatalf("Messages count = %d, want 1", len(s.Messages))
	}
	msg := s.Messages[0]
	if msg["tool_call_id"] != "call_123" {
		t.Errorf("tool_call_id = %v, want call_123", msg["tool_call_id"])
	}
	if msg["name"] != "search" {
		t.Errorf("name = %v, want search", msg["name"])
	}
}

// TestAddMultipleMessages 测试添加多条消息。
func TestAddMultipleMessages(t *testing.T) {
	s := NewSession("test:1")
	s.AddMessage("user", "hi", nil)
	s.AddMessage("assistant", "hello!", nil)
	s.AddMessage("user", "how are you?", nil)

	if len(s.Messages) != 3 {
		t.Errorf("Messages count = %d, want 3", len(s.Messages))
	}
	if s.Messages[0]["content"] != "hi" {
		t.Errorf("first message content = %v, want hi", s.Messages[0]["content"])
	}
	if s.Messages[2]["content"] != "how are you?" {
		t.Errorf("third message content = %v", s.Messages[2]["content"])
	}
}

// TestGetHistoryDefault 测试默认 maxMessages（<= 0 默认 50）。
func TestGetHistoryDefault(t *testing.T) {
	s := NewSession("test:1")
	for i := 0; i < 5; i++ {
		s.AddMessage("user", "msg", nil)
	}

	history := s.GetHistory(0)
	if len(history) != 5 {
		t.Errorf("history length = %d, want 5", len(history))
	}

	history = s.GetHistory(-1)
	if len(history) != 5 {
		t.Errorf("history length = %d, want 5", len(history))
	}
}

// TestGetHistoryTruncation 测试消息历史截断。
func TestGetHistoryTruncation(t *testing.T) {
	s := NewSession("test:1")
	for i := 0; i < 10; i++ {
		s.AddMessage("user", "msg", nil)
	}

	history := s.GetHistory(3)
	if len(history) != 3 {
		t.Errorf("history length = %d, want 3", len(history))
	}
}

// TestGetHistoryFormat 测试返回的消息格式（仅包含 role 和 content）。
func TestGetHistoryFormat(t *testing.T) {
	s := NewSession("test:1")
	s.AddMessage("user", "hello", map[string]interface{}{"extra": "data"})

	history := s.GetHistory(10)
	if len(history) != 1 {
		t.Fatalf("history length = %d, want 1", len(history))
	}
	msg := history[0]
	if msg["role"] != "user" {
		t.Errorf("role = %v, want user", msg["role"])
	}
	if msg["content"] != "hello" {
		t.Errorf("content = %v, want hello", msg["content"])
	}
	// extra 字段不应出现在历史中
	if _, ok := msg["extra"]; ok {
		t.Error("extra field should not be in history")
	}
	if _, ok := msg["timestamp"]; ok {
		t.Error("timestamp field should not be in history")
	}
}

// TestGetHistoryEmpty 测试空会话的历史。
func TestGetHistoryEmpty(t *testing.T) {
	s := NewSession("test:1")
	history := s.GetHistory(10)
	if len(history) != 0 {
		t.Errorf("history length = %d, want 0", len(history))
	}
}

// TestGetHistoryKeepsRecentMessages 测试截断时保留最新消息。
func TestGetHistoryKeepsRecentMessages(t *testing.T) {
	s := NewSession("test:1")
	s.AddMessage("user", "old", nil)
	s.AddMessage("user", "middle", nil)
	s.AddMessage("user", "recent", nil)

	history := s.GetHistory(2)
	if len(history) != 2 {
		t.Fatalf("history length = %d, want 2", len(history))
	}
	if history[0]["content"] != "middle" {
		t.Errorf("first = %v, want middle", history[0]["content"])
	}
	if history[1]["content"] != "recent" {
		t.Errorf("second = %v, want recent", history[1]["content"])
	}
}

// TestClear 测试清空会话。
func TestClear(t *testing.T) {
	s := NewSession("test:1")
	s.AddMessage("user", "hello", nil)
	s.AddMessage("assistant", "hi", nil)

	before := s.UpdatedAt
	time.Sleep(time.Millisecond)
	s.Clear()

	if len(s.Messages) != 0 {
		t.Errorf("Messages count = %d, want 0", len(s.Messages))
	}
	if !s.UpdatedAt.After(before) {
		t.Error("UpdatedAt should be updated after Clear")
	}
}

// ============ SessionManager 测试 ============

// newTestSessionManager 创建用于测试的 SessionManager。
func newTestSessionManager(t *testing.T) *SessionManager {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "sessions")
	m, err := NewSessionManager(dir, nil)
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}
	return m
}

// TestNewSessionManager 测试创建管理器。
func TestNewSessionManager(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	m, err := NewSessionManager(dir, nil)
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}
	if m == nil {
		t.Fatal("SessionManager should not be nil")
	}
	// 目录应已创建
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("sessions dir not created: %v", err)
	}
}

// TestNewSessionManagerInvalidDir 测试在无法创建的路径上创建管理器。
func TestNewSessionManagerInvalidDir(t *testing.T) {
	_, err := NewSessionManager("/dev/null/sessions", nil)
	if err == nil {
		t.Error("expected error for invalid dir, got nil")
	}
}

// TestGetOrCreateNew 测试获取不存在的会话（应创建新会话）。
func TestGetOrCreateNew(t *testing.T) {
	m := newTestSessionManager(t)
	s := m.GetOrCreate("telegram:user1")

	if s == nil {
		t.Fatal("session should not be nil")
	}
	if s.Key != "telegram:user1" {
		t.Errorf("Key = %q, want %q", s.Key, "telegram:user1")
	}
	if len(s.Messages) != 0 {
		t.Errorf("new session should have 0 messages, got %d", len(s.Messages))
	}
}

// TestGetOrCreateCached 测试从缓存获取已有会话。
func TestGetOrCreateCached(t *testing.T) {
	m := newTestSessionManager(t)
	s1 := m.GetOrCreate("telegram:user1")
	s1.AddMessage("user", "hello", nil)

	s2 := m.GetOrCreate("telegram:user1")
	if s1 != s2 {
		t.Error("should return the same cached session pointer")
	}
	if len(s2.Messages) != 1 {
		t.Errorf("cached session should have 1 message, got %d", len(s2.Messages))
	}
}

// TestSaveAndLoad 测试保存和重新加载会话（JSONL 往返测试）。
func TestSaveAndLoad(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	m1, err := NewSessionManager(dir, nil)
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}

	// 创建并保存会话
	s := m1.GetOrCreate("whatsapp:user2")
	s.AddMessage("user", "hello world", nil)
	s.AddMessage("assistant", "hi there!", nil)
	s.Metadata["lang"] = "en"

	if err := m1.Save(s); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// 创建新的 SessionManager（不共享缓存），验证磁盘加载
	m2, err := NewSessionManager(dir, nil)
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}

	loaded := m2.GetOrCreate("whatsapp:user2")
	if loaded.Key != "whatsapp:user2" {
		t.Errorf("Key = %q, want %q", loaded.Key, "whatsapp:user2")
	}
	if len(loaded.Messages) != 2 {
		t.Fatalf("loaded messages count = %d, want 2", len(loaded.Messages))
	}
	if loaded.Messages[0]["content"] != "hello world" {
		t.Errorf("first message = %v, want 'hello world'", loaded.Messages[0]["content"])
	}
	if loaded.Messages[1]["role"] != "assistant" {
		t.Errorf("second role = %v, want assistant", loaded.Messages[1]["role"])
	}
	if loaded.Metadata["lang"] != "en" {
		t.Errorf("metadata lang = %v, want en", loaded.Metadata["lang"])
	}
}

// TestSaveCreatesFile 测试 Save 创建 JSONL 文件。
func TestSaveCreatesFile(t *testing.T) {
	m := newTestSessionManager(t)
	s := m.GetOrCreate("cli:test")
	s.AddMessage("user", "test", nil)

	if err := m.Save(s); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// 检查文件是否存在
	pattern := filepath.Join(m.sessionsDir, "*.jsonl")
	files, _ := filepath.Glob(pattern)
	if len(files) == 0 {
		t.Error("no JSONL file created")
	}
}

// TestSaveOverwrite 测试覆盖保存。
func TestSaveOverwrite(t *testing.T) {
	m := newTestSessionManager(t)
	s := m.GetOrCreate("cli:test")
	s.AddMessage("user", "first", nil)
	if err := m.Save(s); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// 添加更多消息后覆盖
	s.AddMessage("assistant", "second", nil)
	s.AddMessage("user", "third", nil)
	if err := m.Save(s); err != nil {
		t.Fatalf("Save (overwrite) error: %v", err)
	}

	// 重新加载验证
	dir := m.sessionsDir
	m2, _ := NewSessionManager(dir, nil)
	loaded := m2.GetOrCreate("cli:test")
	if len(loaded.Messages) != 3 {
		t.Errorf("loaded messages count = %d, want 3", len(loaded.Messages))
	}
}

// TestDelete 测试删除会话。
func TestDelete(t *testing.T) {
	m := newTestSessionManager(t)
	s := m.GetOrCreate("telegram:del1")
	s.AddMessage("user", "hello", nil)
	if err := m.Save(s); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// 删除
	ok := m.Delete("telegram:del1")
	if !ok {
		t.Error("Delete returned false, want true")
	}

	// 验证缓存中已删除
	s2 := m.GetOrCreate("telegram:del1")
	if len(s2.Messages) != 0 {
		t.Errorf("after delete, new session should have 0 messages, got %d", len(s2.Messages))
	}
}

// TestDeleteNonExistent 测试删除不存在的会话。
func TestDeleteNonExistent(t *testing.T) {
	m := newTestSessionManager(t)
	ok := m.Delete("nonexistent:key")
	if ok {
		t.Error("Delete returned true for non-existent session")
	}
}

// TestDeleteRemovesFile 测试删除会确实删除磁盘文件。
func TestDeleteRemovesFile(t *testing.T) {
	m := newTestSessionManager(t)
	s := m.GetOrCreate("cli:delfile")
	s.AddMessage("user", "msg", nil)
	m.Save(s)

	// 先确认文件存在
	files, _ := filepath.Glob(filepath.Join(m.sessionsDir, "*.jsonl"))
	if len(files) == 0 {
		t.Fatal("file should exist before delete")
	}

	m.Delete("cli:delfile")

	// 文件应已被删除
	files, _ = filepath.Glob(filepath.Join(m.sessionsDir, "*.jsonl"))
	if len(files) != 0 {
		t.Errorf("file should be deleted, but found %d files", len(files))
	}
}

// TestListSessions 测试列出会话。
func TestListSessions(t *testing.T) {
	m := newTestSessionManager(t)

	// 空目录
	sessions := m.ListSessions()
	if len(sessions) != 0 {
		t.Errorf("empty dir should return 0 sessions, got %d", len(sessions))
	}

	// 创建多个会话
	for _, key := range []string{"telegram:a", "whatsapp:b", "cli:c"} {
		s := m.GetOrCreate(key)
		s.AddMessage("user", "hi", nil)
		if err := m.Save(s); err != nil {
			t.Fatalf("Save error: %v", err)
		}
		time.Sleep(10 * time.Millisecond) // 确保时间戳不同
	}

	sessions = m.ListSessions()
	if len(sessions) != 3 {
		t.Errorf("sessions count = %d, want 3", len(sessions))
	}

	// 验证每个 session 有基本信息
	for _, info := range sessions {
		if info.Key == "" {
			t.Error("session key should not be empty")
		}
		if info.CreatedAt == "" {
			t.Error("created_at should not be empty")
		}
		if info.Path == "" {
			t.Error("path should not be empty")
		}
	}
}

// TestListSessionsSortOrder 测试列出会话按更新时间倒序。
func TestListSessionsSortOrder(t *testing.T) {
	m := newTestSessionManager(t)

	keys := []string{"a:1", "b:2", "c:3"}
	for _, key := range keys {
		s := m.GetOrCreate(key)
		s.AddMessage("user", "hi", nil)
		m.Save(s)
		time.Sleep(20 * time.Millisecond)
	}

	sessions := m.ListSessions()
	if len(sessions) != 3 {
		t.Fatalf("sessions count = %d, want 3", len(sessions))
	}

	for i := 1; i < len(sessions); i++ {
		if sessions[i].UpdatedAt > sessions[i-1].UpdatedAt {
			t.Errorf("sessions not sorted descending: [%d].UpdatedAt=%s > [%d].UpdatedAt=%s",
				i, sessions[i].UpdatedAt, i-1, sessions[i-1].UpdatedAt)
		}
	}
}

// TestListSessionsIgnoresNonJSONL 测试列出会话时忽略非 .jsonl 文件。
func TestListSessionsIgnoresNonJSONL(t *testing.T) {
	m := newTestSessionManager(t)

	s := m.GetOrCreate("test:valid")
	s.AddMessage("user", "hi", nil)
	m.Save(s)

	os.WriteFile(filepath.Join(m.sessionsDir, "notes.txt"), []byte("not a session"), 0644)
	os.Mkdir(filepath.Join(m.sessionsDir, "subdir"), 0755)

	sessions := m.ListSessions()
	if len(sessions) != 1 {
		t.Errorf("sessions count = %d, want 1 (should ignore non-jsonl)", len(sessions))
	}
}

// ============ 并发测试 ============

// TestConcurrentGetOrCreate 测试并发获取/创建会话。
func TestConcurrentGetOrCreate(t *testing.T) {
	m := newTestSessionManager(t)

	var wg sync.WaitGroup
	results := make([]*Session, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = m.GetOrCreate("shared:key")
		}(i)
	}

	wg.Wait()

	for i := 1; i < len(results); i++ {
		if results[i] != results[0] {
			t.Errorf("goroutine %d got different session pointer", i)
		}
	}
}

// TestConcurrentSave 测试并发保存不同会话。
func TestConcurrentSave(t *testing.T) {
	m := newTestSessionManager(t)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := "ch:" + string(rune('a'+idx))
			s := m.GetOrCreate(key)
			s.AddMessage("user", "msg", nil)
			if err := m.Save(s); err != nil {
				t.Errorf("Save error for %s: %v", key, err)
			}
		}(i)
	}

	wg.Wait()

	sessions := m.ListSessions()
	if len(sessions) != 10 {
		t.Errorf("sessions count = %d, want 10", len(sessions))
	}
}

// ============ 边界条件测试 ============

// TestEmptySessionSaveAndLoad 测试保存和加载空会话（无消息）。
func TestEmptySessionSaveAndLoad(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	m1, _ := NewSessionManager(dir, nil)

	s := m1.GetOrCreate("cli:empty")
	if err := m1.Save(s); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	m2, _ := NewSessionManager(dir, nil)
	loaded := m2.GetOrCreate("cli:empty")
	if len(loaded.Messages) != 0 {
		t.Errorf("empty session should have 0 messages after reload, got %d", len(loaded.Messages))
	}
}

// TestLargeSession 测试大量消息的会话。
func TestLargeSession(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	m1, _ := NewSessionManager(dir, nil)

	s := m1.GetOrCreate("cli:large")
	for i := 0; i < 500; i++ {
		s.AddMessage("user", "message content that is reasonably long for testing purposes", nil)
	}

	if err := m1.Save(s); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	m2, _ := NewSessionManager(dir, nil)
	loaded := m2.GetOrCreate("cli:large")
	if len(loaded.Messages) != 500 {
		t.Errorf("loaded messages count = %d, want 500", len(loaded.Messages))
	}
}

// TestSessionWithSpecialCharKey 测试包含特殊字符的会话键。
func TestSessionWithSpecialCharKey(t *testing.T) {
	m := newTestSessionManager(t)

	s := m.GetOrCreate("slack:channel/thread")
	s.AddMessage("user", "test", nil)
	if err := m.Save(s); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	dir := m.sessionsDir
	m2, _ := NewSessionManager(dir, nil)
	loaded := m2.GetOrCreate("slack:channel/thread")
	if len(loaded.Messages) != 1 {
		t.Errorf("loaded messages count = %d, want 1", len(loaded.Messages))
	}
}

// TestSessionMetadataPreserved 测试元数据在持久化后被保留。
func TestSessionMetadataPreserved(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	m1, _ := NewSessionManager(dir, nil)

	s := m1.GetOrCreate("test:meta")
	s.Metadata["user_name"] = "Alice"
	s.Metadata["preference"] = "dark_mode"
	s.AddMessage("user", "hi", nil)
	m1.Save(s)

	m2, _ := NewSessionManager(dir, nil)
	loaded := m2.GetOrCreate("test:meta")
	if loaded.Metadata["user_name"] != "Alice" {
		t.Errorf("metadata user_name = %v, want Alice", loaded.Metadata["user_name"])
	}
	if loaded.Metadata["preference"] != "dark_mode" {
		t.Errorf("metadata preference = %v, want dark_mode", loaded.Metadata["preference"])
	}
}

// TestSessionCreatedAtPreserved 测试创建时间在持久化后被保留。
func TestSessionCreatedAtPreserved(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	m1, _ := NewSessionManager(dir, nil)

	s := m1.GetOrCreate("test:time")
	originalCreated := s.CreatedAt
	s.AddMessage("user", "hi", nil)
	m1.Save(s)

	time.Sleep(10 * time.Millisecond)

	m2, _ := NewSessionManager(dir, nil)
	loaded := m2.GetOrCreate("test:time")

	if loaded.CreatedAt.Unix() != originalCreated.Unix() {
		t.Errorf("CreatedAt changed: got %v, want %v", loaded.CreatedAt, originalCreated)
	}
}

// ============ safeFilename 测试 ============

// TestSafeFilename 测试文件名安全转换。
func TestSafeFilename(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"normal", "normal"},
		{"hello world", "hello world"},
		{`a<b>c:d"e`, "a_b_c_d_e"},
		{`x/y\z|w?v*u`, "x_y_z_w_v_u"},
		{"  spaces  ", "spaces"},
		{"no-change", "no-change"},
	}
	for _, tt := range tests {
		got := utils.SafeFilename(tt.input)
		if got != tt.want {
			t.Errorf("SafeFilename(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestSafeFilenameEmpty 测试空字符串。
func TestSafeFilenameEmpty(t *testing.T) {
	got := utils.SafeFilename("")
	if got != "" {
		t.Errorf("SafeFilename(\"\") = %q, want \"\"", got)
	}
}
