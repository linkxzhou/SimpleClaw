// 结构化记忆条目：按类别（Core/Daily/Conversation/Procedural）分类存储，
// 支持时间衰减评分，在上下文受限时智能选择最相关的记忆。

package memory

// MemoryCategory 记忆类别。
type MemoryCategory string

const (
	CategoryCore         MemoryCategory = "core"         // 核心事实（永不衰减）：用户姓名、关键偏好
	CategoryDaily        MemoryCategory = "daily"        // 每日对话摘要
	CategoryConversation MemoryCategory = "conversation" // 对话上下文片段
	CategoryProcedural   MemoryCategory = "procedural"   // 操作步骤 / 工作流偏好
)

// MemoryEntry 一条结构化记忆。
type MemoryEntry struct {
	ID        string         `json:"id"`
	Key       string         `json:"key"`                 // 短标识（如 "user_name"）
	Content   string         `json:"content"`
	Category  MemoryCategory `json:"category"`
	Timestamp string         `json:"timestamp"`           // RFC3339
	Score     *float64       `json:"score,omitempty"`     // 衰减后分数（nil 表示未计算）
}

// CategoryHalfLifeDays 各类别的衰减半衰期（天）。
// Core 永不衰减（半衰期 = 0 表示跳过）。
var CategoryHalfLifeDays = map[MemoryCategory]float64{
	CategoryCore:         0,    // 永不衰减
	CategoryProcedural:   30.0, // 30 天
	CategoryDaily:        7.0,  // 7 天
	CategoryConversation: 3.0,  // 3 天
}

// CategoryInitialScore 各类别的初始分数。
var CategoryInitialScore = map[MemoryCategory]float64{
	CategoryCore:         1.0,
	CategoryProcedural:   0.8,
	CategoryDaily:        0.6,
	CategoryConversation: 0.4,
}

// InitialScore 返回该条目的初始分数。
func (e *MemoryEntry) InitialScore() float64 {
	if s, ok := CategoryInitialScore[e.Category]; ok {
		return s
	}
	return 0.5
}
