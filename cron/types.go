// Package cron 提供 SimpleClaw 的定时任务管理功能。
// 支持三种调度方式：一次性（at）、周期性（every）、cron 表达式（cron）。
// 任务通过 JSON 文件持久化存储。
package cron

// ScheduleKind 定义调度类型。
type ScheduleKind string

const (
	ScheduleAt    ScheduleKind = "at"    // 一次性：在指定时间执行
	ScheduleEvery ScheduleKind = "every" // 周期性：按固定间隔重复执行
	ScheduleCron  ScheduleKind = "cron"  // Cron 表达式：按 cron 语法调度
)

// Schedule 定义任务的调度计划。
type Schedule struct {
	Kind    ScheduleKind `json:"kind"`              // 调度类型
	AtMs    int64        `json:"atMs,omitempty"`    // "at" 类型：目标时间戳（毫秒）
	EveryMs int64        `json:"everyMs,omitempty"` // "every" 类型：间隔时长（毫秒）
	Expr    string       `json:"expr,omitempty"`    // "cron" 类型：cron 表达式
	TZ      string       `json:"tz,omitempty"`      // cron 表达式的时区
}

// PayloadKind 定义任务执行时的动作类型。
type PayloadKind string

const (
	PayloadSystemEvent PayloadKind = "system_event" // 系统事件
	PayloadAgentTurn   PayloadKind = "agent_turn"   // Agent 对话轮次
)

// Payload 定义任务执行时的具体动作。
type Payload struct {
	Kind    PayloadKind `json:"kind"`              // 动作类型
	Message string      `json:"message"`           // 发送给 Agent 的消息
	Deliver bool        `json:"deliver"`           // 是否将响应投递到频道
	Channel string      `json:"channel,omitempty"` // 目标频道（如 "whatsapp"）
	To      string      `json:"to,omitempty"`      // 目标接收者（如聊天 ID）
}

// JobStatus 表示任务上次执行的结果状态。
type JobStatus string

const (
	StatusOK      JobStatus = "ok"      // 执行成功
	StatusError   JobStatus = "error"   // 执行失败
	StatusSkipped JobStatus = "skipped" // 已跳过
)

// JobState 保存任务的运行时状态。
type JobState struct {
	NextRunAtMs int64     `json:"nextRunAtMs,omitempty"` // 下次执行时间（毫秒时间戳）
	LastRunAtMs int64     `json:"lastRunAtMs,omitempty"` // 上次执行时间（毫秒时间戳）
	LastStatus  JobStatus `json:"lastStatus,omitempty"`  // 上次执行状态
	LastError   string    `json:"lastError,omitempty"`   // 上次执行的错误信息
}

// Job 表示一个定时任务。
type Job struct {
	ID             string   `json:"id"`             // 任务唯一标识
	Name           string   `json:"name"`           // 任务名称
	Enabled        bool     `json:"enabled"`        // 是否启用
	Schedule       Schedule `json:"schedule"`       // 调度计划
	Payload        Payload  `json:"payload"`        // 执行内容
	State          JobState `json:"state"`          // 运行时状态
	CreatedAtMs    int64    `json:"createdAtMs"`    // 创建时间（毫秒时间戳）
	UpdatedAtMs    int64    `json:"updatedAtMs"`    // 更新时间（毫秒时间戳）
	DeleteAfterRun bool     `json:"deleteAfterRun"` // 执行后是否自动删除
}

// Store 是定时任务的持久化存储结构。
type Store struct {
	Version int    `json:"version"` // 存储格式版本
	Jobs    []*Job `json:"jobs"`    // 任务列表
}
