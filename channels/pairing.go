// Sender Pairing 配对认证：将静态 AllowFrom 白名单升级为动态配对码认证。
// 新用户发消息时自动生成配对码，管理员通过 CLI 审批后即获得访问权限。

package channels

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// PairingStatus 配对状态。
type PairingStatus string

const (
	PairingPending  PairingStatus = "pending"
	PairingApproved PairingStatus = "approved"
)

// PairingEntry 一条配对记录。
type PairingEntry struct {
	SenderID    string        `json:"sender_id"`
	Channel     string        `json:"channel"`
	Code        string        `json:"code"`
	Status      PairingStatus `json:"status"`
	RequestedAt string        `json:"requested_at"`
	ApprovedAt  *string       `json:"approved_at,omitempty"`
}

// PairingResult 验证结果。
type PairingResult struct {
	Approved     bool
	IsNewPending bool   // 首次申请
	Code         string // 配对码
}

// PairingStore 配对状态管理。
type PairingStore struct {
	filePath string
	mu       sync.RWMutex
	pending  []PairingEntry
	approved []PairingEntry
}

// NewPairingStore 创建配对存储。
func NewPairingStore(dir string) (*PairingStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	ps := &PairingStore{
		filePath: filepath.Join(dir, "pairing.json"),
	}
	ps.load()
	return ps, nil
}

// EnsureSenderPaired 检查发送者是否已配对。
// 未配对则自动生成配对码并加入 pending。
func (ps *PairingStore) EnsureSenderPaired(channel, senderID string) PairingResult {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// 检查已认证
	for _, e := range ps.approved {
		if e.Channel == channel && e.SenderID == senderID {
			return PairingResult{Approved: true}
		}
	}

	// 检查待审批
	for _, e := range ps.pending {
		if e.Channel == channel && e.SenderID == senderID {
			return PairingResult{Approved: false, Code: e.Code}
		}
	}

	// 新用户
	code := GeneratePairingCode()
	ps.pending = append(ps.pending, PairingEntry{
		SenderID:    senderID,
		Channel:     channel,
		Code:        code,
		Status:      PairingPending,
		RequestedAt: time.Now().Format(time.RFC3339),
	})
	ps.save()

	return PairingResult{Approved: false, IsNewPending: true, Code: code}
}

// ApproveByCode 通过配对码审批。
func (ps *PairingStore) ApproveByCode(code string) (*PairingEntry, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	for i, entry := range ps.pending {
		if entry.Code == code {
			ps.pending = append(ps.pending[:i], ps.pending[i+1:]...)
			now := time.Now().Format(time.RFC3339)
			entry.Status = PairingApproved
			entry.ApprovedAt = &now
			ps.approved = append(ps.approved, entry)
			ps.save()
			return &entry, nil
		}
	}
	return nil, fmt.Errorf("pairing code %q not found", code)
}

// Revoke 撤销授权。
func (ps *PairingStore) Revoke(channel, senderID string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	for i, entry := range ps.approved {
		if entry.Channel == channel && entry.SenderID == senderID {
			ps.approved = append(ps.approved[:i], ps.approved[i+1:]...)
			ps.save()
			return nil
		}
	}
	return fmt.Errorf("sender %s:%s not found in approved list", channel, senderID)
}

// AddDirect 直接添加授权（跳过配对流程）。
func (ps *PairingStore) AddDirect(channel, senderID string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// 检查是否已存在
	for _, e := range ps.approved {
		if e.Channel == channel && e.SenderID == senderID {
			return
		}
	}

	now := time.Now().Format(time.RFC3339)
	ps.approved = append(ps.approved, PairingEntry{
		SenderID:    senderID,
		Channel:     channel,
		Code:        "",
		Status:      PairingApproved,
		RequestedAt: now,
		ApprovedAt:  &now,
	})
	ps.save()
}

// ListPending 返回所有待审批条目。
func (ps *PairingStore) ListPending() []PairingEntry {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	cp := make([]PairingEntry, len(ps.pending))
	copy(cp, ps.pending)
	return cp
}

// ListApproved 返回所有已授权条目。
func (ps *PairingStore) ListApproved() []PairingEntry {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	cp := make([]PairingEntry, len(ps.approved))
	copy(cp, ps.approved)
	return cp
}

// ============ 配对码生成 ============

const pairingAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // 去除 I/O/0/1

// GeneratePairingCode 生成 8 位配对码（不含易混淆字符）。
func GeneratePairingCode() string {
	b := make([]byte, 8)
	rand.Read(b)
	code := make([]byte, 8)
	for i, v := range b {
		code[i] = pairingAlphabet[int(v)%len(pairingAlphabet)]
	}
	return string(code)
}

// ============ 文件 IO ============

type pairingData struct {
	Pending  []PairingEntry `json:"pending"`
	Approved []PairingEntry `json:"approved"`
}

func (ps *PairingStore) load() {
	data, err := os.ReadFile(ps.filePath)
	if err != nil {
		return
	}
	var pd pairingData
	if err := json.Unmarshal(data, &pd); err != nil {
		return
	}
	ps.pending = pd.Pending
	ps.approved = pd.Approved
}

func (ps *PairingStore) save() error {
	data, err := json.MarshalIndent(pairingData{
		Pending:  ps.pending,
		Approved: ps.approved,
	}, "", "  ")
	if err != nil {
		return err
	}
	// 原子写入
	tmpPath := ps.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, ps.filePath)
}
