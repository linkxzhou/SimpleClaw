package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/linkxzhou/SimpleClaw/config"
	"github.com/linkxzhou/SimpleClaw/utils"
)

// cmdOnboard 初始化 SimpleClaw 配置和工作区。
func cmdOnboard(args []string) {
	configPath := config.GetConfigPath()

	// 检查是否已存在
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("[WARN] Config already exists at %s\n", configPath)
		fmt.Print("Overwrite? (y/N): ")
		var answer string
		fmt.Scanln(&answer)
		if answer != "y" && answer != "Y" {
			return
		}
	}

	// 创建默认配置
	cfg := config.DefaultConfig()
	if err := config.Save(cfg, configPath); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Failed to save config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  ✓ Created config at %s\n", configPath)

	// 创建工作区
	workspace := cfg.WorkspacePath()
	if _, err := utils.EnsureDir(workspace); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Failed to create workspace: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  ✓ Created workspace at %s\n", workspace)

	// 创建模板文件
	createWorkspaceTemplates(workspace)

	fmt.Printf("\n%s SimpleClaw is ready!\n", logo)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Add your API key to ~/.simpleclaw/config.json")
	fmt.Println("     Get one at: https://openrouter.ai/keys")
	fmt.Println("  2. Chat: simpleclaw agent -m \"Hello!\"")
}

// createWorkspaceTemplates 在工作区中创建默认模板文件。
func createWorkspaceTemplates(workspace string) {
	templates := map[string]string{
		"AGENTS.md": `# Agent Instructions

You are a helpful AI assistant. Be concise, accurate, and friendly.

## Guidelines

- Always explain what you're doing before taking actions
- Ask for clarification when the request is ambiguous
- Use tools to help accomplish tasks
- Remember important information in your memory files
`,
		"SOUL.md": `# Soul

I am SimpleClaw, a lightweight AI agent framework.

## Personality

- Helpful and friendly
- Concise and to the point
- Curious and eager to learn

## Values

- Accuracy over speed
- User privacy and safety
- Transparency in actions
`,
		"USER.md": `# User

Information about the user goes here.

## Preferences

- Communication style: (casual/formal)
- Timezone: (your timezone)
- Language: (your preferred language)
`,
	}

	for filename, content := range templates {
		fp := filepath.Join(workspace, filename)
		if _, err := os.Stat(fp); err != nil {
			if err := os.WriteFile(fp, []byte(content), 0644); err == nil {
				fmt.Printf("    Created %s\n", filename)
			}
		}
	}

	// 创建 memory 目录和 MEMORY.md
	memoryDir := filepath.Join(workspace, "memory")
	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] Failed to create memory dir: %v\n", err)
	}
	memoryFile := filepath.Join(memoryDir, "MEMORY.md")
	if _, err := os.Stat(memoryFile); err != nil {
		content := `# Long-term Memory

This file stores important information that should persist across sessions.

## User Information

(Important facts about the user)

## Preferences

(User preferences learned over time)

## Important Notes

(Things to remember)
`
		if err := os.WriteFile(memoryFile, []byte(content), 0644); err == nil {
			fmt.Println("    Created memory/MEMORY.md")
		}
	}

	// 创建 HEARTBEAT.md
	heartbeatFile := filepath.Join(workspace, "HEARTBEAT.md")
	if _, err := os.Stat(heartbeatFile); err != nil {
		content := `# Heartbeat

<!-- This file is checked periodically by the agent. -->
<!-- Add tasks or reminders below, and the agent will process them. -->
`
		if err := os.WriteFile(heartbeatFile, []byte(content), 0644); err == nil {
			fmt.Println("    Created HEARTBEAT.md")
		}
	}
}
