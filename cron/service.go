package cron

// 定时任务服务：管理、调度和执行定时任务。
// 使用 time.AfterFunc 实现单定时器驱动的调度，
// 每次定时器触发时检查所有到期任务并执行。

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleClaw/utils"
)

// nowMs 返回当前时间的毫秒时间戳。
func nowMs() int64 {
	return time.Now().UnixMilli()
}

// computeNextRun 根据调度计划计算下次执行时间（毫秒时间戳）。
// 返回 0 表示不再执行。
func computeNextRun(sched Schedule, now int64) int64 {
	switch sched.Kind {
	case ScheduleAt:
		// 一次性任务：只在目标时间之后执行
		if sched.AtMs > now {
			return sched.AtMs
		}
		return 0

	case ScheduleEvery:
		// 周期性任务：当前时间加上间隔
		if sched.EveryMs <= 0 {
			return 0
		}
		return now + sched.EveryMs

	case ScheduleCron:
		// Cron 表达式需要 cron 解析器库支持。
		// TODO: 集成 cron 解析器（如 github.com/robfig/cron/v3）
		return 0
	}
	return 0
}

// OnJobFunc 任务到期时的回调函数。返回响应文本或错误。
type OnJobFunc func(ctx context.Context, job *Job) (string, error)

// Service 管理和执行定时任务。
// 采用单定时器模式：只维护一个指向最近到期任务的定时器，
// 触发后重新计算并设置下一个定时器。
type Service struct {
	storePath string       // 任务存储文件路径
	onJob     OnJobFunc    // 任务执行回调
	logger    *slog.Logger // 日志记录器

	mu      sync.Mutex     // 保护内部状态
	store   *Store         // 任务存储
	cancel  context.CancelFunc // 用于停止服务
	running bool           // 是否正在运行
	timer   *time.Timer    // 下次唤醒定时器
}

// NewService 创建新的定时任务服务。
func NewService(storePath string, onJob OnJobFunc, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		storePath: storePath,
		onJob:     onJob,
		logger:    logger,
	}
}

// Start 启动定时任务服务。阻塞直到 ctx 取消。
func (s *Service) Start(ctx context.Context) {
	s.mu.Lock()
	s.running = true
	s.loadStore()
	s.recomputeNextRuns()
	s.saveStore()
	s.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	s.armTimer(ctx)
	s.logger.Info("cron service started", "jobs", len(s.store.Jobs))

	<-ctx.Done()
	s.logger.Info("cron service stopped")
}

// Stop 停止定时任务服务。
func (s *Service) Stop() {
	s.mu.Lock()
	s.running = false
	if s.timer != nil {
		s.timer.Stop()
	}
	s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}
}

// ListJobs 返回所有任务，可选是否包含已禁用的任务。
// 返回结果按下次执行时间升序排列。
func (s *Service) ListJobs(includeDisabled bool) []*Job {
	s.mu.Lock()
	s.loadStore()
	s.mu.Unlock()

	var jobs []*Job
	for _, j := range s.store.Jobs {
		if includeDisabled || j.Enabled {
			jobs = append(jobs, j)
		}
	}
	// 按下次执行时间排序（NextRunAtMs == 0 排最后）
	sort.Slice(jobs, func(i, k int) bool {
		a := jobs[i].State.NextRunAtMs
		b := jobs[k].State.NextRunAtMs
		if a == 0 {
			a = math.MaxInt64
		}
		if b == 0 {
			b = math.MaxInt64
		}
		return a < b
	})
	return jobs
}

// AddJob 添加一个新的定时任务。
func (s *Service) AddJob(name string, sched Schedule, message string, deliver bool, channel, to string, deleteAfterRun bool) *Job {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.loadStore()
	now := nowMs()

	job := &Job{
		ID:      utils.TruncateString(fmt.Sprintf("%d", now), 8, ""),
		Name:    name,
		Enabled: true,
		Schedule: sched,
		Payload: Payload{
			Kind:    PayloadAgentTurn,
			Message: message,
			Deliver: deliver,
			Channel: channel,
			To:      to,
		},
		State:          JobState{NextRunAtMs: computeNextRun(sched, now)},
		CreatedAtMs:    now,
		UpdatedAtMs:    now,
		DeleteAfterRun: deleteAfterRun,
	}

	s.store.Jobs = append(s.store.Jobs, job)
	s.saveStore()
	s.logger.Info("cron: added job", "name", name, "id", job.ID)
	return job
}

// RemoveJob 根据 ID 删除一个任务。删除成功返回 true。
func (s *Service) RemoveJob(jobID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.loadStore()
	before := len(s.store.Jobs)
	filtered := make([]*Job, 0, before)
	for _, j := range s.store.Jobs {
		if j.ID != jobID {
			filtered = append(filtered, j)
		}
	}
	s.store.Jobs = filtered
	removed := len(s.store.Jobs) < before

	if removed {
		s.saveStore()
		s.logger.Info("cron: removed job", "id", jobID)
	}
	return removed
}

// EnableJob 启用或禁用一个任务。返回修改后的任务，未找到返回 nil。
func (s *Service) EnableJob(jobID string, enabled bool) *Job {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.loadStore()
	for _, j := range s.store.Jobs {
		if j.ID == jobID {
			j.Enabled = enabled
			j.UpdatedAtMs = nowMs()
			if enabled {
				j.State.NextRunAtMs = computeNextRun(j.Schedule, nowMs())
			} else {
				j.State.NextRunAtMs = 0
			}
			s.saveStore()
			return j
		}
	}
	return nil
}

// RunJob 手动运行一个任务。force 为 true 时忽略启用状态。
func (s *Service) RunJob(ctx context.Context, jobID string, force bool) bool {
	s.mu.Lock()
	var target *Job
	s.loadStore()
	for _, j := range s.store.Jobs {
		if j.ID == jobID {
			target = j
			break
		}
	}
	s.mu.Unlock()

	if target == nil {
		return false
	}
	if !force && !target.Enabled {
		return false
	}

	s.executeJob(ctx, target)

	s.mu.Lock()
	s.saveStore()
	s.mu.Unlock()

	return true
}

// Status 返回服务的运行状态摘要。
func (s *Service) Status() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.loadStore()
	return map[string]interface{}{
		"enabled":      s.running,
		"jobs":         len(s.store.Jobs),
		"nextWakeAtMs": s.getNextWakeMs(),
	}
}

// ---- 内部方法 ----

// loadStore 从磁盘加载任务存储。仅在首次访问时加载（惰性加载）。
func (s *Service) loadStore() {
	if s.store != nil {
		return
	}

	data, err := os.ReadFile(s.storePath)
	if err != nil {
		s.store = &Store{Version: 1}
		return
	}

	var store Store
	if err := json.Unmarshal(data, &store); err != nil {
		s.logger.Warn("failed to load cron store", "error", err)
		s.store = &Store{Version: 1}
		return
	}
	s.store = &store
}

// saveStore 将任务存储持久化到磁盘。
func (s *Service) saveStore() {
	if s.store == nil {
		return
	}

	dir := filepath.Dir(s.storePath)
	os.MkdirAll(dir, 0o755)

	data, err := json.MarshalIndent(s.store, "", "  ")
	if err != nil {
		s.logger.Error("failed to marshal cron store", "error", err)
		return
	}
	if err := os.WriteFile(s.storePath, data, 0o644); err != nil {
		s.logger.Error("failed to save cron store", "error", err)
	}
}

// recomputeNextRuns 重新计算所有已启用任务的下次执行时间。
func (s *Service) recomputeNextRuns() {
	if s.store == nil {
		return
	}
	now := nowMs()
	for _, j := range s.store.Jobs {
		if j.Enabled {
			j.State.NextRunAtMs = computeNextRun(j.Schedule, now)
		}
	}
}

// getNextWakeMs 返回最近一个待执行任务的时间戳。
// 返回 0 表示没有待执行的任务。
func (s *Service) getNextWakeMs() int64 {
	if s.store == nil {
		return 0
	}
	var earliest int64
	for _, j := range s.store.Jobs {
		if j.Enabled && j.State.NextRunAtMs > 0 {
			if earliest == 0 || j.State.NextRunAtMs < earliest {
				earliest = j.State.NextRunAtMs
			}
		}
	}
	return earliest
}

// armTimer 设置定时器，在最近到期任务的时间点触发。
func (s *Service) armTimer(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.timer != nil {
		s.timer.Stop()
	}

	nextWake := s.getNextWakeMs()
	if nextWake == 0 || !s.running {
		return
	}

	delay := time.Duration(nextWake-nowMs()) * time.Millisecond
	if delay < 0 {
		delay = 0
	}

	s.timer = time.AfterFunc(delay, func() {
		if !s.running {
			return
		}
		s.onTimer(ctx)
	})
}

// onTimer 定时器触发时的处理函数。
// 收集所有到期任务并依次执行，然后重新设置定时器。
func (s *Service) onTimer(ctx context.Context) {
	s.mu.Lock()
	if s.store == nil {
		s.mu.Unlock()
		return
	}

	// 收集所有到期任务
	now := nowMs()
	var dueJobs []*Job
	for _, j := range s.store.Jobs {
		if j.Enabled && j.State.NextRunAtMs > 0 && now >= j.State.NextRunAtMs {
			dueJobs = append(dueJobs, j)
		}
	}
	s.mu.Unlock()

	// 逐个执行到期任务
	for _, j := range dueJobs {
		s.executeJob(ctx, j)
	}

	s.mu.Lock()
	s.saveStore()
	s.mu.Unlock()

	// 重新设置定时器
	s.armTimer(ctx)
}

// executeJob 执行一个任务。
// 调用 onJob 回调，更新任务状态。
// 对于一次性任务（ScheduleAt），执行后根据 DeleteAfterRun 决定删除还是禁用。
func (s *Service) executeJob(ctx context.Context, job *Job) {
	startMs := nowMs()
	s.logger.Info("cron: executing job", "name", job.Name, "id", job.ID)

	if s.onJob != nil {
		_, err := s.onJob(ctx, job)
		if err != nil {
			job.State.LastStatus = StatusError
			job.State.LastError = err.Error()
			s.logger.Error("cron: job failed", "name", job.Name, "error", err)
		} else {
			job.State.LastStatus = StatusOK
			job.State.LastError = ""
			s.logger.Info("cron: job completed", "name", job.Name)
		}
	}

	job.State.LastRunAtMs = startMs
	job.UpdatedAtMs = nowMs()

	// 处理一次性任务
	if job.Schedule.Kind == ScheduleAt {
		if job.DeleteAfterRun {
			// 执行后删除
			s.mu.Lock()
			filtered := make([]*Job, 0, len(s.store.Jobs))
			for _, j := range s.store.Jobs {
				if j.ID != job.ID {
					filtered = append(filtered, j)
				}
			}
			s.store.Jobs = filtered
			s.mu.Unlock()
		} else {
			// 执行后禁用
			job.Enabled = false
			job.State.NextRunAtMs = 0
		}
	} else {
		// 周期性任务：计算下次执行时间
		job.State.NextRunAtMs = computeNextRun(job.Schedule, nowMs())
	}
}
