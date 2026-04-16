package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/linkxzhou/SimpleClaw/cron"
	"github.com/linkxzhou/SimpleClaw/utils"
)

// cmdCron 管理定时任务：list / add / remove / enable / run。
func cmdCron(args []string) {
	if len(args) == 0 {
		printCronUsage()
		return
	}

	subcmd := args[0]
	subArgs := args[1:]

	switch subcmd {
	case "list":
		cronList(subArgs)
	case "add":
		cronAdd(subArgs)
	case "remove":
		cronRemove(subArgs)
	case "enable":
		cronEnable(subArgs)
	case "run":
		cronRun(subArgs)
	default:
		fmt.Fprintf(os.Stderr, "Unknown cron subcommand: %s\n\n", subcmd)
		printCronUsage()
		os.Exit(1)
	}
}

// printCronUsage 输出 cron 子命令的使用帮助。
func printCronUsage() {
	fmt.Println(`Usage: simpleclaw cron <subcommand> [options]

Subcommands:
  list                 列出所有定时任务
  add                  添加定时任务
  remove <job_id>      删除任务
  enable <job_id>      启用/禁用任务
  run    <job_id>      手动执行任务

Examples:
  simpleclaw cron list
  simpleclaw cron list --all
  simpleclaw cron add -n "daily report" -m "Generate daily report" --every 3600
  simpleclaw cron add -n "morning check" -m "Good morning" --cron "0 9 * * *"
  simpleclaw cron remove abc123
  simpleclaw cron enable abc123
  simpleclaw cron enable abc123 --disable
  simpleclaw cron run abc123`)
}

// getCronService 创建一个 cron.Service 实例（用于 CLI 命令）。
func getCronService() *cron.Service {
	dataPath, _ := utils.GetDataPath()
	storePath := filepath.Join(dataPath, "cron", "jobs.json")
	return cron.NewService(storePath, nil, slog.Default())
}

// cronList 列出所有定时任务。
func cronList(args []string) {
	includeDisabled := false
	for _, a := range args {
		if a == "--all" || a == "-a" {
			includeDisabled = true
		}
	}

	svc := getCronService()
	jobs := svc.ListJobs(includeDisabled)

	if len(jobs) == 0 {
		fmt.Println("No scheduled jobs.")
		return
	}

	// 表格输出
	fmt.Printf("%-12s %-20s %-18s %-10s %-20s\n", "ID", "Name", "Schedule", "Status", "Next Run")
	fmt.Println("------------ -------------------- ------------------ ---------- --------------------")

	for _, job := range jobs {
		// 格式化 schedule
		var sched string
		switch job.Schedule.Kind {
		case cron.ScheduleEvery:
			sched = fmt.Sprintf("every %ds", job.Schedule.EveryMs/1000)
		case cron.ScheduleCron:
			sched = job.Schedule.Expr
		case cron.ScheduleAt:
			sched = "one-time"
		}

		// 格式化 next run
		var nextRun string
		if job.State.NextRunAtMs > 0 {
			t := time.UnixMilli(job.State.NextRunAtMs)
			nextRun = t.Format("2006-01-02 15:04")
		}

		status := "enabled"
		if !job.Enabled {
			status = "disabled"
		}

		// 截断显示
		id := utils.TruncateString(job.ID, 10, "..")
		name := utils.TruncateString(job.Name, 18, "..")

		fmt.Printf("%-12s %-20s %-18s %-10s %-20s\n", id, name, sched, status, nextRun)
	}
}

// cronAdd 添加定时任务。
func cronAdd(args []string) {
	var name, message, cronExpr, at string
	var every int
	var deliver bool
	var to, channel string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-n", "--name":
			if i+1 < len(args) {
				i++
				name = args[i]
			}
		case "-m", "--message":
			if i+1 < len(args) {
				i++
				message = args[i]
			}
		case "-e", "--every":
			if i+1 < len(args) {
				i++
				var err error
				every, err = strconv.Atoi(args[i])
				if err != nil {
					fmt.Fprintf(os.Stderr, "[ERROR] Invalid --every value: %s\n", args[i])
					os.Exit(1)
				}
			}
		case "-c", "--cron":
			if i+1 < len(args) {
				i++
				cronExpr = args[i]
			}
		case "--at":
			if i+1 < len(args) {
				i++
				at = args[i]
			}
		case "-d", "--deliver":
			deliver = true
		case "--to":
			if i+1 < len(args) {
				i++
				to = args[i]
			}
		case "--channel":
			if i+1 < len(args) {
				i++
				channel = args[i]
			}
		}
	}

	if name == "" || message == "" {
		fmt.Fprintln(os.Stderr, "[ERROR] --name and --message are required")
		os.Exit(1)
	}

	// 确定 schedule
	var sched cron.Schedule
	if every > 0 {
		sched = cron.Schedule{Kind: cron.ScheduleEvery, EveryMs: int64(every) * 1000}
	} else if cronExpr != "" {
		sched = cron.Schedule{Kind: cron.ScheduleCron, Expr: cronExpr}
	} else if at != "" {
		t, err := time.Parse(time.RFC3339, at)
		if err != nil {
			// 尝试简单格式
			t, err = time.Parse("2006-01-02T15:04:05", at)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[ERROR] Invalid --at time format: %v\n", err)
				os.Exit(1)
			}
		}
		sched = cron.Schedule{Kind: cron.ScheduleAt, AtMs: t.UnixMilli()}
	} else {
		fmt.Fprintln(os.Stderr, "[ERROR] Must specify --every, --cron, or --at")
		os.Exit(1)
	}

	svc := getCronService()
	job := svc.AddJob(name, sched, message, deliver, channel, to, false)
	fmt.Printf("  ✓ Added job '%s' (%s)\n", job.Name, job.ID)
}

// cronRemove 删除定时任务。
func cronRemove(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "[ERROR] Job ID required")
		os.Exit(1)
	}

	svc := getCronService()
	if svc.RemoveJob(args[0]) {
		fmt.Printf("  ✓ Removed job %s\n", args[0])
	} else {
		fmt.Fprintf(os.Stderr, "Job %s not found\n", args[0])
		os.Exit(1)
	}
}

// cronEnable 启用/禁用定时任务。
func cronEnable(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "[ERROR] Job ID required")
		os.Exit(1)
	}

	jobID := args[0]
	disable := false
	for _, a := range args[1:] {
		if a == "--disable" {
			disable = true
		}
	}

	svc := getCronService()
	job := svc.EnableJob(jobID, !disable)
	if job != nil {
		action := "enabled"
		if disable {
			action = "disabled"
		}
		fmt.Printf("  ✓ Job '%s' %s\n", job.Name, action)
	} else {
		fmt.Fprintf(os.Stderr, "Job %s not found\n", jobID)
		os.Exit(1)
	}
}

// cronRun 手动运行一个任务。
func cronRun(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "[ERROR] Job ID required")
		os.Exit(1)
	}

	jobID := args[0]
	force := false
	for _, a := range args[1:] {
		if a == "-f" || a == "--force" {
			force = true
		}
	}

	svc := getCronService()
	ctx := context.Background()
	if svc.RunJob(ctx, jobID, force) {
		fmt.Println("  ✓ Job executed")
	} else {
		fmt.Fprintf(os.Stderr, "Failed to run job %s (not found or no handler)\n", jobID)
		os.Exit(1)
	}
}
