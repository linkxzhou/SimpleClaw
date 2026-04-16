// cmd/sop.go — SOP CLI 子命令：list、run、status、history。

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleClaw/config"
	"github.com/linkxzhou/SimpleClaw/sop"
)

func cmdSop(args []string) {
	if len(args) == 0 {
		printSopUsage()
		return
	}

	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		cmdSopList(cfg)
	case "run":
		cmdSopRun(cfg, args[1:])
	case "help", "--help", "-h":
		printSopUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown sop command: %s\n", args[0])
		printSopUsage()
		os.Exit(1)
	}
}

func cmdSopList(cfg *config.Config) {
	sops, err := sop.LoadSOPs(cfg.WorkspacePath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load SOPs: %v\n", err)
		os.Exit(1)
	}

	if len(sops) == 0 {
		fmt.Println("No SOPs found. Create a SOP directory in workspace/sops/")
		return
	}

	fmt.Printf("%-20s %-12s %-14s %-8s %s\n", "NAME", "PRIORITY", "MODE", "STEPS", "TRIGGERS")
	fmt.Println(strings.Repeat("-", 80))
	for _, s := range sops {
		triggers := make([]string, 0, len(s.Triggers))
		for _, t := range s.Triggers {
			if t.Expression != "" {
				triggers = append(triggers, fmt.Sprintf("%s(%s)", t.Type, t.Expression))
			} else {
				triggers = append(triggers, t.Type)
			}
		}
		fmt.Printf("%-20s %-12s %-14s %-8d %s\n",
			s.Name, s.Priority, s.ExecutionMode, len(s.Steps), strings.Join(triggers, ", "))
	}
}

func cmdSopRun(cfg *config.Config, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: simpleclaw sop run <name> [--step-by-step]")
		os.Exit(1)
	}

	sopName := args[0]
	stepByStep := false
	for _, a := range args[1:] {
		if a == "--step-by-step" {
			stepByStep = true
		}
	}

	sops, err := sop.LoadSOPs(cfg.WorkspacePath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load SOPs: %v\n", err)
		os.Exit(1)
	}

	var target *sop.Sop
	for i := range sops {
		if sops[i].Name == sopName {
			target = &sops[i]
			break
		}
	}
	if target == nil {
		fmt.Fprintf(os.Stderr, "SOP not found: %s\n", sopName)
		os.Exit(1)
	}

	if stepByStep {
		target.ExecutionMode = sop.ModeStepByStep
	}

	engine := sop.NewEngine(sop.EngineConfig{
		ConfirmFunc: cliConfirmSop,
		Model:       cfg.Agents.Defaults.Model,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	fmt.Printf("🚀 Running SOP: %s (%d steps, mode=%s)\n\n", target.Name, len(target.Steps), target.ExecutionMode)
	run := engine.Execute(ctx, *target)

	// 输出结果
	fmt.Println()
	for _, r := range run.StepResults {
		icon := "✅"
		if r.Status == "failed" {
			icon = "❌"
		}
		fmt.Printf("  %s Step %d: %s (%s)\n", icon, r.StepNumber, r.Status, r.Duration.Round(time.Millisecond))
		if r.Output != "" {
			// 缩进输出
			for _, line := range strings.Split(r.Output, "\n") {
				fmt.Printf("     %s\n", line)
			}
		}
	}

	fmt.Printf("\nResult: %s (run_id=%s, duration=%s)\n",
		run.Status, run.RunID, run.CompletedAt.Sub(run.StartedAt).Round(time.Millisecond))
}

func cliConfirmSop(title, description string) bool {
	fmt.Printf("⏸️  %s\n", title)
	if description != "" {
		fmt.Printf("   %s\n", description)
	}
	fmt.Print("   Continue? [Y/n]: ")
	var input string
	fmt.Scanln(&input)
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "" || input == "y" || input == "yes"
}

func printSopUsage() {
	fmt.Println(`SOP — Standard Operating Procedures

Usage:
  simpleclaw sop <command>

Commands:
  list                      列出所有 SOP
  run <name>                执行 SOP
  run <name> --step-by-step 逐步确认模式

Examples:
  simpleclaw sop list
  simpleclaw sop run deploy-pipeline
  simpleclaw sop run daily-report --step-by-step`)
}
