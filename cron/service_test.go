package cron

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

// newTestService 创建用于测试的 Service，存储路径指向临时目录。
func newTestService(t *testing.T, onJob OnJobFunc) *Service {
	t.Helper()
	dir := t.TempDir()
	return NewService(filepath.Join(dir, "cron.json"), onJob, nil)
}

// ---------------------------------------------------------------------------
// computeNextRun 测试
// ---------------------------------------------------------------------------

func TestComputeNextRunAt(t *testing.T) {
	now := int64(1000)
	// 目标时间在未来
	if got := computeNextRun(Schedule{Kind: ScheduleAt, AtMs: 2000}, now); got != 2000 {
		t.Errorf("期望 2000，实际 %d", got)
	}
	// 目标时间已过
	if got := computeNextRun(Schedule{Kind: ScheduleAt, AtMs: 500}, now); got != 0 {
		t.Errorf("期望 0，实际 %d", got)
	}
	// 目标时间等于当前
	if got := computeNextRun(Schedule{Kind: ScheduleAt, AtMs: 1000}, now); got != 0 {
		t.Errorf("期望 0（等于当前也不执行），实际 %d", got)
	}
}

func TestComputeNextRunEvery(t *testing.T) {
	now := int64(1000)
	if got := computeNextRun(Schedule{Kind: ScheduleEvery, EveryMs: 500}, now); got != 1500 {
		t.Errorf("期望 1500，实际 %d", got)
	}
	// 间隔为 0
	if got := computeNextRun(Schedule{Kind: ScheduleEvery, EveryMs: 0}, now); got != 0 {
		t.Errorf("间隔为 0 应返回 0，实际 %d", got)
	}
	// 负数间隔
	if got := computeNextRun(Schedule{Kind: ScheduleEvery, EveryMs: -100}, now); got != 0 {
		t.Errorf("负数间隔应返回 0，实际 %d", got)
	}
}

func TestComputeNextRunCron(t *testing.T) {
	// Cron 类型尚未实现，应返回 0
	if got := computeNextRun(Schedule{Kind: ScheduleCron, Expr: "0 * * * *"}, 1000); got != 0 {
		t.Errorf("cron 未实现应返回 0，实际 %d", got)
	}
}

func TestComputeNextRunUnknownKind(t *testing.T) {
	if got := computeNextRun(Schedule{Kind: "unknown"}, 1000); got != 0 {
		t.Errorf("未知类型应返回 0，实际 %d", got)
	}
}

// ---------------------------------------------------------------------------
// NewService 测试
// ---------------------------------------------------------------------------

func TestNewService(t *testing.T) {
	svc := NewService("/tmp/test.json", nil, nil)
	if svc.storePath != "/tmp/test.json" {
		t.Errorf("storePath 不匹配")
	}
	if svc.logger == nil {
		t.Error("logger 不应为 nil")
	}
}

// ---------------------------------------------------------------------------
// AddJob 测试
// ---------------------------------------------------------------------------

func TestAddJob(t *testing.T) {
	svc := newTestService(t, nil)
	job := svc.AddJob("test-job", Schedule{Kind: ScheduleEvery, EveryMs: 60000}, "hello", false, "", "", false)

	if job.Name != "test-job" {
		t.Errorf("期望 name=test-job，实际 %q", job.Name)
	}
	if !job.Enabled {
		t.Error("新任务应默认启用")
	}
	if job.Payload.Message != "hello" {
		t.Errorf("期望 message=hello，实际 %q", job.Payload.Message)
	}
	if job.Payload.Kind != PayloadAgentTurn {
		t.Errorf("期望 kind=agent_turn，实际 %q", job.Payload.Kind)
	}
	if job.ID == "" {
		t.Error("ID 不应为空")
	}
	if job.CreatedAtMs == 0 {
		t.Error("CreatedAtMs 不应为 0")
	}
}

func TestAddJobWithDelivery(t *testing.T) {
	svc := newTestService(t, nil)
	job := svc.AddJob("deliver-job", Schedule{Kind: ScheduleAt, AtMs: nowMs() + 100000}, "msg", true, "telegram", "chat123", true)

	if !job.Payload.Deliver {
		t.Error("Deliver 应为 true")
	}
	if job.Payload.Channel != "telegram" {
		t.Errorf("期望 channel=telegram，实际 %q", job.Payload.Channel)
	}
	if job.Payload.To != "chat123" {
		t.Errorf("期望 to=chat123，实际 %q", job.Payload.To)
	}
	if !job.DeleteAfterRun {
		t.Error("DeleteAfterRun 应为 true")
	}
}

func TestAddJobPersistence(t *testing.T) {
	svc := newTestService(t, nil)
	svc.AddJob("persist-job", Schedule{Kind: ScheduleEvery, EveryMs: 1000}, "msg", false, "", "", false)

	// 验证文件已写入
	data, err := os.ReadFile(svc.storePath)
	if err != nil {
		t.Fatalf("读取存储文件失败: %v", err)
	}
	var store Store
	if err := json.Unmarshal(data, &store); err != nil {
		t.Fatalf("解析存储文件失败: %v", err)
	}
	if len(store.Jobs) != 1 {
		t.Fatalf("期望 1 个任务，实际 %d", len(store.Jobs))
	}
	if store.Jobs[0].Name != "persist-job" {
		t.Errorf("持久化的任务名称不匹配")
	}
}

// ---------------------------------------------------------------------------
// ListJobs 测试
// ---------------------------------------------------------------------------

func TestListJobsEmpty(t *testing.T) {
	svc := newTestService(t, nil)
	jobs := svc.ListJobs(true)
	if len(jobs) != 0 {
		t.Errorf("期望 0 个任务，实际 %d", len(jobs))
	}
}

func TestListJobsFiltersDisabled(t *testing.T) {
	svc := newTestService(t, nil)
	svc.AddJob("enabled-job", Schedule{Kind: ScheduleEvery, EveryMs: 1000}, "msg", false, "", "", false)
	j2 := svc.AddJob("disabled-job", Schedule{Kind: ScheduleEvery, EveryMs: 1000}, "msg", false, "", "", false)
	svc.EnableJob(j2.ID, false)

	// includeDisabled=false 只返回启用的
	jobs := svc.ListJobs(false)
	if len(jobs) != 1 {
		t.Errorf("期望 1 个启用任务，实际 %d", len(jobs))
	}

	// includeDisabled=true 返回全部
	allJobs := svc.ListJobs(true)
	if len(allJobs) != 2 {
		t.Errorf("期望 2 个任务，实际 %d", len(allJobs))
	}
}

func TestListJobsSortOrder(t *testing.T) {
	svc := newTestService(t, nil)
	// 添加不同间隔的周期任务
	svc.AddJob("far", Schedule{Kind: ScheduleEvery, EveryMs: 100000}, "msg", false, "", "", false)
	time.Sleep(2 * time.Millisecond)
	svc.AddJob("near", Schedule{Kind: ScheduleEvery, EveryMs: 100}, "msg", false, "", "", false)

	jobs := svc.ListJobs(true)
	if len(jobs) < 2 {
		t.Fatalf("期望至少 2 个任务")
	}
	// 近的应排在前面
	if jobs[0].Name != "near" {
		t.Errorf("期望第一个任务为 'near'，实际 %q", jobs[0].Name)
	}
}

// ---------------------------------------------------------------------------
// RemoveJob 测试
// ---------------------------------------------------------------------------

func TestRemoveJob(t *testing.T) {
	svc := newTestService(t, nil)
	job := svc.AddJob("to-remove", Schedule{Kind: ScheduleEvery, EveryMs: 1000}, "msg", false, "", "", false)

	if !svc.RemoveJob(job.ID) {
		t.Error("删除存在的任务应返回 true")
	}
	jobs := svc.ListJobs(true)
	if len(jobs) != 0 {
		t.Errorf("删除后应无任务，实际 %d", len(jobs))
	}
}

func TestRemoveJobNonExistent(t *testing.T) {
	svc := newTestService(t, nil)
	if svc.RemoveJob("nonexistent") {
		t.Error("删除不存在的任务应返回 false")
	}
}

// ---------------------------------------------------------------------------
// EnableJob 测试
// ---------------------------------------------------------------------------

func TestEnableJobDisable(t *testing.T) {
	svc := newTestService(t, nil)
	job := svc.AddJob("toggle", Schedule{Kind: ScheduleEvery, EveryMs: 1000}, "msg", false, "", "", false)

	result := svc.EnableJob(job.ID, false)
	if result == nil {
		t.Fatal("EnableJob 不应返回 nil")
	}
	if result.Enabled {
		t.Error("任务应被禁用")
	}
	if result.State.NextRunAtMs != 0 {
		t.Error("禁用后 NextRunAtMs 应为 0")
	}
}

func TestEnableJobReEnable(t *testing.T) {
	svc := newTestService(t, nil)
	job := svc.AddJob("toggle", Schedule{Kind: ScheduleEvery, EveryMs: 5000}, "msg", false, "", "", false)
	svc.EnableJob(job.ID, false)

	result := svc.EnableJob(job.ID, true)
	if result == nil {
		t.Fatal("EnableJob 不应返回 nil")
	}
	if !result.Enabled {
		t.Error("任务应被重新启用")
	}
	if result.State.NextRunAtMs == 0 {
		t.Error("重新启用后 NextRunAtMs 不应为 0")
	}
}

func TestEnableJobNonExistent(t *testing.T) {
	svc := newTestService(t, nil)
	if svc.EnableJob("nonexistent", true) != nil {
		t.Error("不存在的任务应返回 nil")
	}
}

// ---------------------------------------------------------------------------
// RunJob 测试
// ---------------------------------------------------------------------------

func TestRunJob(t *testing.T) {
	var called int32
	svc := newTestService(t, func(ctx context.Context, job *Job) (string, error) {
		atomic.AddInt32(&called, 1)
		return "ok", nil
	})
	job := svc.AddJob("run-me", Schedule{Kind: ScheduleEvery, EveryMs: 60000}, "msg", false, "", "", false)

	if !svc.RunJob(context.Background(), job.ID, false) {
		t.Error("RunJob 应返回 true")
	}
	if atomic.LoadInt32(&called) != 1 {
		t.Errorf("回调应被调用 1 次，实际 %d", called)
	}
}

func TestRunJobNonExistent(t *testing.T) {
	svc := newTestService(t, nil)
	if svc.RunJob(context.Background(), "nonexistent", false) {
		t.Error("不存在的任务应返回 false")
	}
}

func TestRunJobDisabledNotForced(t *testing.T) {
	svc := newTestService(t, func(ctx context.Context, job *Job) (string, error) {
		t.Error("不应执行禁用的任务")
		return "", nil
	})
	job := svc.AddJob("disabled", Schedule{Kind: ScheduleEvery, EveryMs: 1000}, "msg", false, "", "", false)
	svc.EnableJob(job.ID, false)

	if svc.RunJob(context.Background(), job.ID, false) {
		t.Error("禁用且非强制应返回 false")
	}
}

func TestRunJobDisabledForced(t *testing.T) {
	var called int32
	svc := newTestService(t, func(ctx context.Context, job *Job) (string, error) {
		atomic.AddInt32(&called, 1)
		return "ok", nil
	})
	job := svc.AddJob("forced", Schedule{Kind: ScheduleEvery, EveryMs: 1000}, "msg", false, "", "", false)
	svc.EnableJob(job.ID, false)

	if !svc.RunJob(context.Background(), job.ID, true) {
		t.Error("强制运行应返回 true")
	}
	if atomic.LoadInt32(&called) != 1 {
		t.Error("强制运行应执行回调")
	}
}

// ---------------------------------------------------------------------------
// executeJob 测试
// ---------------------------------------------------------------------------

func TestExecuteJobSuccess(t *testing.T) {
	svc := newTestService(t, func(ctx context.Context, job *Job) (string, error) {
		return "done", nil
	})
	job := svc.AddJob("exec-ok", Schedule{Kind: ScheduleEvery, EveryMs: 5000}, "msg", false, "", "", false)

	svc.executeJob(context.Background(), job)

	if job.State.LastStatus != StatusOK {
		t.Errorf("期望 status=ok，实际 %q", job.State.LastStatus)
	}
	if job.State.LastError != "" {
		t.Errorf("成功时 LastError 应为空，实际 %q", job.State.LastError)
	}
	if job.State.LastRunAtMs == 0 {
		t.Error("LastRunAtMs 不应为 0")
	}
}

func TestExecuteJobError(t *testing.T) {
	svc := newTestService(t, func(ctx context.Context, job *Job) (string, error) {
		return "", errors.New("执行失败")
	})
	job := svc.AddJob("exec-err", Schedule{Kind: ScheduleEvery, EveryMs: 5000}, "msg", false, "", "", false)

	svc.executeJob(context.Background(), job)

	if job.State.LastStatus != StatusError {
		t.Errorf("期望 status=error，实际 %q", job.State.LastStatus)
	}
	if job.State.LastError != "执行失败" {
		t.Errorf("期望 LastError='执行失败'，实际 %q", job.State.LastError)
	}
}

func TestExecuteJobNilCallback(t *testing.T) {
	svc := newTestService(t, nil)
	job := svc.AddJob("nil-cb", Schedule{Kind: ScheduleEvery, EveryMs: 5000}, "msg", false, "", "", false)

	// 不应 panic
	svc.executeJob(context.Background(), job)
	if job.State.LastRunAtMs == 0 {
		t.Error("LastRunAtMs 应被设置")
	}
}

func TestExecuteJobAtDisablesAfterRun(t *testing.T) {
	svc := newTestService(t, func(ctx context.Context, job *Job) (string, error) {
		return "ok", nil
	})
	future := nowMs() + 100000
	job := svc.AddJob("at-job", Schedule{Kind: ScheduleAt, AtMs: future}, "msg", false, "", "", false)

	svc.executeJob(context.Background(), job)

	if job.Enabled {
		t.Error("一次性任务执行后应被禁用")
	}
	if job.State.NextRunAtMs != 0 {
		t.Error("禁用后 NextRunAtMs 应为 0")
	}
}

func TestExecuteJobAtDeleteAfterRun(t *testing.T) {
	svc := newTestService(t, func(ctx context.Context, job *Job) (string, error) {
		return "ok", nil
	})
	future := nowMs() + 100000
	job := svc.AddJob("at-delete", Schedule{Kind: ScheduleAt, AtMs: future}, "msg", false, "", "", true)

	svc.executeJob(context.Background(), job)

	// 任务应从 store 中删除
	jobs := svc.ListJobs(true)
	for _, j := range jobs {
		if j.ID == job.ID {
			t.Error("DeleteAfterRun 的任务应从 store 中删除")
		}
	}
}

func TestExecuteJobEveryRecomputes(t *testing.T) {
	svc := newTestService(t, func(ctx context.Context, job *Job) (string, error) {
		return "ok", nil
	})
	job := svc.AddJob("every-job", Schedule{Kind: ScheduleEvery, EveryMs: 5000}, "msg", false, "", "", false)
	oldNext := job.State.NextRunAtMs

	time.Sleep(2 * time.Millisecond)
	svc.executeJob(context.Background(), job)

	if job.State.NextRunAtMs <= oldNext {
		t.Error("周期任务执行后 NextRunAtMs 应增加")
	}
}

// ---------------------------------------------------------------------------
// Status 测试
// ---------------------------------------------------------------------------

func TestStatus(t *testing.T) {
	svc := newTestService(t, nil)
	svc.AddJob("j1", Schedule{Kind: ScheduleEvery, EveryMs: 1000}, "msg", false, "", "", false)
	svc.AddJob("j2", Schedule{Kind: ScheduleEvery, EveryMs: 2000}, "msg", false, "", "", false)

	status := svc.Status()
	if status["enabled"] != false {
		t.Error("未启动时 enabled 应为 false")
	}
	if status["jobs"] != 2 {
		t.Errorf("期望 2 个任务，实际 %v", status["jobs"])
	}
	if status["nextWakeAtMs"] == nil {
		t.Error("nextWakeAtMs 不应为 nil")
	}
}

// ---------------------------------------------------------------------------
// loadStore / saveStore 测试
// ---------------------------------------------------------------------------

func TestLoadStoreFileNotExist(t *testing.T) {
	svc := newTestService(t, nil)
	// 首次 loadStore 文件不存在时应创建默认 store
	svc.mu.Lock()
	svc.loadStore()
	svc.mu.Unlock()
	if svc.store == nil {
		t.Error("store 不应为 nil")
	}
	if svc.store.Version != 1 {
		t.Errorf("期望 version=1，实际 %d", svc.store.Version)
	}
}

func TestLoadStoreInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cron.json")
	os.WriteFile(path, []byte("not json"), 0644)

	svc := NewService(path, nil, nil)
	svc.mu.Lock()
	svc.loadStore()
	svc.mu.Unlock()

	if svc.store == nil {
		t.Error("无效 JSON 时应回退到默认 store")
	}
	if svc.store.Version != 1 {
		t.Errorf("期望 version=1，实际 %d", svc.store.Version)
	}
}

func TestSaveAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cron.json")

	// 第一个 service 添加任务
	svc1 := NewService(path, nil, nil)
	svc1.AddJob("reload-test", Schedule{Kind: ScheduleEvery, EveryMs: 1000}, "msg", false, "", "", false)

	// 第二个 service 从同一文件加载
	svc2 := NewService(path, nil, nil)
	jobs := svc2.ListJobs(true)
	if len(jobs) != 1 {
		t.Fatalf("期望 1 个任务，实际 %d", len(jobs))
	}
	if jobs[0].Name != "reload-test" {
		t.Errorf("任务名称不匹配")
	}
}

// ---------------------------------------------------------------------------
// Start / Stop 测试
// ---------------------------------------------------------------------------

func TestStartAndStop(t *testing.T) {
	svc := newTestService(t, func(ctx context.Context, job *Job) (string, error) {
		return "ok", nil
	})

	done := make(chan struct{})
	go func() {
		svc.Start(context.Background())
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	svc.Stop()

	select {
	case <-done:
		// 正常停止
	case <-time.After(2 * time.Second):
		t.Error("Stop 后 Start 应退出")
	}
}

func TestStopWithoutStart(t *testing.T) {
	svc := newTestService(t, nil)
	// 不应 panic
	svc.Stop()
}

// ---------------------------------------------------------------------------
// getNextWakeMs 测试
// ---------------------------------------------------------------------------

func TestGetNextWakeMsNoJobs(t *testing.T) {
	svc := newTestService(t, nil)
	svc.mu.Lock()
	svc.loadStore()
	result := svc.getNextWakeMs()
	svc.mu.Unlock()
	if result != 0 {
		t.Errorf("无任务时应返回 0，实际 %d", result)
	}
}

func TestGetNextWakeMsReturnsEarliest(t *testing.T) {
	svc := newTestService(t, nil)
	svc.AddJob("far", Schedule{Kind: ScheduleEvery, EveryMs: 100000}, "msg", false, "", "", false)
	svc.AddJob("near", Schedule{Kind: ScheduleEvery, EveryMs: 100}, "msg", false, "", "", false)

	svc.mu.Lock()
	result := svc.getNextWakeMs()
	svc.mu.Unlock()

	// 应返回最近的任务时间
	if result == 0 {
		t.Error("应有待执行任务")
	}
}

// ---------------------------------------------------------------------------
// 类型常量测试
// ---------------------------------------------------------------------------

func TestScheduleKindConstants(t *testing.T) {
	if ScheduleAt != "at" {
		t.Errorf("ScheduleAt 应为 'at'")
	}
	if ScheduleEvery != "every" {
		t.Errorf("ScheduleEvery 应为 'every'")
	}
	if ScheduleCron != "cron" {
		t.Errorf("ScheduleCron 应为 'cron'")
	}
}

func TestPayloadKindConstants(t *testing.T) {
	if PayloadSystemEvent != "system_event" {
		t.Errorf("PayloadSystemEvent 应为 'system_event'")
	}
	if PayloadAgentTurn != "agent_turn" {
		t.Errorf("PayloadAgentTurn 应为 'agent_turn'")
	}
}

func TestJobStatusConstants(t *testing.T) {
	if StatusOK != "ok" {
		t.Errorf("StatusOK 应为 'ok'")
	}
	if StatusError != "error" {
		t.Errorf("StatusError 应为 'error'")
	}
	if StatusSkipped != "skipped" {
		t.Errorf("StatusSkipped 应为 'skipped'")
	}
}
