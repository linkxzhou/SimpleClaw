package channels

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// GeneratePairingCode 测试
// ---------------------------------------------------------------------------

func TestGeneratePairingCode(t *testing.T) {
	code := GeneratePairingCode()
	if len(code) != 8 {
		t.Errorf("配对码应为 8 位，实际 %d", len(code))
	}
	// 检查不含易混淆字符
	for _, c := range code {
		if c == 'I' || c == 'O' || c == '0' || c == '1' {
			t.Errorf("配对码不应包含易混淆字符，发现: %c", c)
		}
	}
}

func TestGeneratePairingCodeUnique(t *testing.T) {
	codes := make(map[string]bool)
	for i := 0; i < 100; i++ {
		code := GeneratePairingCode()
		if codes[code] {
			t.Errorf("生成了重复的配对码: %s", code)
		}
		codes[code] = true
	}
}

// ---------------------------------------------------------------------------
// PairingStore 测试
// ---------------------------------------------------------------------------

func newTestPairingStore(t *testing.T) *PairingStore {
	dir := t.TempDir()
	store, err := NewPairingStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestPairingNewUser(t *testing.T) {
	store := newTestPairingStore(t)

	result := store.EnsureSenderPaired("telegram", "user123")
	if result.Approved {
		t.Error("新用户不应被授权")
	}
	if !result.IsNewPending {
		t.Error("新用户应为 NewPending")
	}
	if len(result.Code) != 8 {
		t.Errorf("配对码应为 8 位，实际 %d", len(result.Code))
	}
}

func TestPairingRepeatRequest(t *testing.T) {
	store := newTestPairingStore(t)

	r1 := store.EnsureSenderPaired("telegram", "user123")
	r2 := store.EnsureSenderPaired("telegram", "user123")

	if r2.IsNewPending {
		t.Error("重复请求不应为 NewPending")
	}
	if r1.Code != r2.Code {
		t.Error("重复请求应返回相同配对码")
	}
}

func TestPairingApprove(t *testing.T) {
	store := newTestPairingStore(t)

	r := store.EnsureSenderPaired("telegram", "user123")
	entry, err := store.ApproveByCode(r.Code)
	if err != nil {
		t.Fatal(err)
	}
	if entry.SenderID != "user123" {
		t.Errorf("审批后应返回正确 sender，实际 %q", entry.SenderID)
	}

	// 审批后应能访问
	r2 := store.EnsureSenderPaired("telegram", "user123")
	if !r2.Approved {
		t.Error("审批后应被授权")
	}
}

func TestPairingApproveInvalidCode(t *testing.T) {
	store := newTestPairingStore(t)

	_, err := store.ApproveByCode("INVALID1")
	if err == nil {
		t.Error("无效配对码应返回错误")
	}
}

func TestPairingRevoke(t *testing.T) {
	store := newTestPairingStore(t)

	store.AddDirect("telegram", "user123")
	r := store.EnsureSenderPaired("telegram", "user123")
	if !r.Approved {
		t.Fatal("直接添加后应被授权")
	}

	err := store.Revoke("telegram", "user123")
	if err != nil {
		t.Fatal(err)
	}

	r2 := store.EnsureSenderPaired("telegram", "user123")
	if r2.Approved {
		t.Error("撤销后不应被授权")
	}
}

func TestPairingRevokeNotFound(t *testing.T) {
	store := newTestPairingStore(t)

	err := store.Revoke("telegram", "unknown")
	if err == nil {
		t.Error("撤销不存在的用户应返回错误")
	}
}

func TestPairingAddDirect(t *testing.T) {
	store := newTestPairingStore(t)

	store.AddDirect("telegram", "admin")
	r := store.EnsureSenderPaired("telegram", "admin")
	if !r.Approved {
		t.Error("直接添加后应被授权")
	}

	// 重复添加不应报错
	store.AddDirect("telegram", "admin")
	approved := store.ListApproved()
	count := 0
	for _, e := range approved {
		if e.Channel == "telegram" && e.SenderID == "admin" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("重复添加不应创建多条记录，实际 %d", count)
	}
}

func TestPairingList(t *testing.T) {
	store := newTestPairingStore(t)

	store.EnsureSenderPaired("telegram", "user1")
	store.EnsureSenderPaired("telegram", "user2")
	store.AddDirect("telegram", "admin")

	pending := store.ListPending()
	if len(pending) != 2 {
		t.Errorf("pending 应有 2 条，实际 %d", len(pending))
	}

	approved := store.ListApproved()
	if len(approved) != 1 {
		t.Errorf("approved 应有 1 条，实际 %d", len(approved))
	}
}

func TestPairingPersistence(t *testing.T) {
	dir := t.TempDir()

	store1, _ := NewPairingStore(dir)
	store1.AddDirect("telegram", "admin")
	r := store1.EnsureSenderPaired("telegram", "user1")

	// 重新加载
	store2, _ := NewPairingStore(dir)
	approved := store2.ListApproved()
	if len(approved) != 1 || approved[0].SenderID != "admin" {
		t.Error("持久化后应能恢复 approved")
	}
	pending := store2.ListPending()
	if len(pending) != 1 || pending[0].Code != r.Code {
		t.Error("持久化后应能恢复 pending")
	}
}

func TestPairingAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewPairingStore(dir)

	store.AddDirect("telegram", "admin")

	// 检查不应有 .tmp 文件残留
	files, _ := os.ReadDir(dir)
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".tmp" {
			t.Errorf("不应有 .tmp 残留文件: %s", f.Name())
		}
	}
}
