# Plan 10: Sender Pairing 配对认证

## 来源

TinyAGI — `packages/core/src/pairing.ts`

## 目标

将 SimpleClaw 的 Channel 认证从静态 `AllowFrom` 白名单升级为动态配对码认证机制，支持新用户自助申请、管理员审批，无需重启服务。

## 现状分析

- `channels/base.go` 的 `BaseChannel` 通过 `AllowFrom` 字段做硬编码白名单校验
- 新增用户需要修改 `config.json` 的 `allowFrom` 数组并重启服务
- `AllowFrom` 支持 `|` 分隔的复合 ID，但只能精确匹配
- 无审批流程，无法追溯谁何时被授权

## 技术方案

### 阶段一：配对状态管理（1 周）

#### 1.1 数据模型

```go
// channels/pairing.go (新文件)
type PairingStatus string
const (
    PairingPending  PairingStatus = "pending"
    PairingApproved PairingStatus = "approved"
)

type PairingEntry struct {
    SenderID   string        `json:"sender_id"`
    Channel    string        `json:"channel"`
    Code       string        `json:"code"`
    Status     PairingStatus `json:"status"`
    RequestedAt string       `json:"requested_at"`
    ApprovedAt  *string      `json:"approved_at,omitempty"`
}

type PairingStore struct {
    filePath string // ~/.simpleclaw/pairing.json
    mu       sync.RWMutex
    pending  []PairingEntry
    approved []PairingEntry
}
```

#### 1.2 原子文件写入

```go
func (ps *PairingStore) save() error {
    ps.mu.RLock()
    defer ps.mu.RUnlock()

    data, err := json.MarshalIndent(struct {
        Pending  []PairingEntry `json:"pending"`
        Approved []PairingEntry `json:"approved"`
    }{ps.pending, ps.approved}, "", "  ")
    if err != nil {
        return err
    }

    // 原子写入：write tmp → rename
    tmpPath := ps.filePath + ".tmp"
    if err := os.WriteFile(tmpPath, data, 0644); err != nil {
        return err
    }
    return os.Rename(tmpPath, ps.filePath)
}
```

#### 1.3 配对码生成

```go
const pairingAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // 去除 I/O/0/1

func GeneratePairingCode() string {
    b := make([]byte, 8)
    if _, err := rand.Read(b); err != nil {
        panic(err)
    }
    code := make([]byte, 8)
    for i, v := range b {
        code[i] = pairingAlphabet[int(v)%len(pairingAlphabet)]
    }
    return string(code)
}
```

### 阶段二：认证流程（1 周）

#### 2.1 发送者验证

```go
type PairingResult struct {
    Approved     bool
    IsNewPending bool   // 首次申请
    Code         string // 配对码（未认证时返回）
}

func (ps *PairingStore) EnsureSenderPaired(channel, senderID string) PairingResult {
    ps.mu.Lock()
    defer ps.mu.Unlock()

    // 1. 检查已认证列表
    for _, entry := range ps.approved {
        if entry.Channel == channel && entry.SenderID == senderID {
            return PairingResult{Approved: true}
        }
    }

    // 2. 检查待审批列表
    for _, entry := range ps.pending {
        if entry.Channel == channel && entry.SenderID == senderID {
            return PairingResult{Approved: false, Code: entry.Code}
        }
    }

    // 3. 新用户，生成配对码
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
```

#### 2.2 管理员审批

```go
func (ps *PairingStore) ApproveByCode(code string) (*PairingEntry, error) {
    ps.mu.Lock()
    defer ps.mu.Unlock()

    for i, entry := range ps.pending {
        if entry.Code == code {
            // 从 pending 移除
            ps.pending = append(ps.pending[:i], ps.pending[i+1:]...)
            // 添加到 approved
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
```

### 阶段三：Channel 集成（0.5 周）

#### 3.1 BaseChannel 改造

```go
// channels/base.go — 修改 HandleMessage()
func (bc *BaseChannel) HandleMessage(senderID, chatID, content string) {
    // 优先检查传统 AllowFrom 白名单（向后兼容）
    if bc.isAllowedByConfig(senderID) {
        bc.forwardToAgent(senderID, chatID, content)
        return
    }

    // 启用配对认证
    if bc.pairingStore != nil {
        result := bc.pairingStore.EnsureSenderPaired(bc.Name(), senderID)
        if result.Approved {
            bc.forwardToAgent(senderID, chatID, content)
            return
        }

        // 未认证：返回配对码提示
        if result.IsNewPending {
            bc.Send(chatID, fmt.Sprintf(
                "🔐 You are not yet authorized. "+
                "Please share this pairing code with the admin: **%s**",
                result.Code,
            ))
        } else {
            bc.Send(chatID, fmt.Sprintf(
                "⏳ Your pairing request is pending. Code: **%s**",
                result.Code,
            ))
        }
        return
    }

    // 无配对 store 且不在白名单 → 拒绝
    bc.Send(chatID, "⛔ Access denied.")
}
```

### 阶段四：CLI 管理命令（0.5 周）

#### 4.1 命令接口

```bash
# 列出所有配对状态
simpleclaw pairing list
  CHANNEL     SENDER          STATUS      CODE        REQUESTED AT
  telegram    user12345       approved    -           2026-04-10
  telegram    user67890       pending     XYZW5678    2026-04-15

# 通过配对码审批
simpleclaw pairing approve XYZW5678
  ✓ Approved: telegram:user67890

# 撤销授权
simpleclaw pairing revoke --channel telegram --sender user67890
  ✓ Revoked: telegram:user67890

# 直接添加（跳过配对流程）
simpleclaw pairing add --channel telegram --sender user12345
  ✓ Added: telegram:user12345
```

#### 4.2 通过渠道消息审批（可选增强）

管理员（已在 approved 列表中的用户）可以直接在聊天中审批：

```
Admin: /approve XYZW5678
Bot: ✓ Approved: telegram:user67890
```

## 涉及文件

| 操作 | 文件 | 说明 |
|------|------|------|
| 新建 | `channels/pairing.go` | PairingStore + 配对逻辑 |
| 新建 | `channels/pairing_test.go` | 单元测试 |
| 新建 | `cmd/pairing.go` | CLI 管理命令 |
| 修改 | `channels/base.go` | HandleMessage 集成配对检查 |
| 修改 | `channels/manager.go` | 初始化 PairingStore |
| 修改 | `config/schema.go` | 新增 pairing.enabled 配置 |
| 修改 | `cmd/main.go` | 新增 pairing 子命令 |

## 配置扩展

```json
{
  "channels": {
    "pairing": {
      "enabled": true
    },
    "telegram": {
      "enabled": true,
      "token": "...",
      "allowFrom": ["admin123"]
    }
  }
}
```

当 `pairing.enabled = true` 时：
- `allowFrom` 中的用户仍然免配对（向后兼容）
- 不在 `allowFrom` 中的用户走配对流程
- 当 `pairing.enabled = false` 时，行为与当前完全一致

## 验收标准

- [ ] 新用户首次消息收到配对码提示
- [ ] 重复消息返回已有配对码（不重新生成）
- [ ] 管理员通过 CLI 审批后，用户立即获得访问权限
- [ ] 已审批用户无需再次配对
- [ ] `AllowFrom` 白名单仍然有效（向后兼容）
- [ ] 原子文件写入防止数据损坏
- [ ] 配对码不含易混淆字符（I/O/0/1）

## 工期估算

**总计：3 周（1 人）**

| 阶段 | 工期 |
|------|------|
| 配对状态管理 | 1 周 |
| 认证流程 | 1 周 |
| Channel 集成 | 0.5 周 |
| CLI 管理命令 | 0.5 周 |
