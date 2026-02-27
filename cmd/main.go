// Package main 提供 SimpleClaw 的 CLI 入口。
// 使用纯 os.Args 实现命令行解析，不依赖第三方 CLI 库。
// 支持的子命令：onboard、gateway、agent、cron、channels、status。
package main

import (
	"fmt"
	"os"
)

const version = "0.1.0" // 版本号
const logo = "🤖"       // 显示用 Logo

// main 是程序入口，根据第一个参数分派到对应的子命令处理函数。
func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "onboard":
		cmdOnboard(args)
	case "gateway":
		cmdGateway(args)
	case "agent":
		cmdAgent(args)
	case "cron":
		cmdCron(args)
	case "channels":
		cmdChannels(args)
	case "status":
		cmdStatus(args)
	case "--version", "-v", "version":
		fmt.Printf("%s SimpleClaw v%s\n", logo, version)
	case "--help", "-h", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

// printUsage 输出命令行使用帮助。
func printUsage() {
	fmt.Printf(`%s SimpleClaw - Personal AI Agent

Usage:
  simpleclaw <command> [options]

Commands:
  onboard     初始化配置和工作区
  gateway     启动完整网关（Agent + Channels + Cron + Heartbeat）
  agent       与 Agent 交互（单消息或交互模式）
  cron        管理定时任务（list、add、remove、enable、run）
  channels    管理聊天渠道（status、login）
  status      显示系统状态

Options:
  -v, --version   显示版本号
  -h, --help      显示帮助

Examples:
  simpleclaw onboard                           # 首次初始化
  simpleclaw agent -m "Hello!"                 # 单消息模式
  simpleclaw agent                             # 交互模式
  simpleclaw gateway                           # 启动完整服务
  simpleclaw cron list                         # 列出定时任务
  simpleclaw status                            # 显示配置状态
`, logo)
}
