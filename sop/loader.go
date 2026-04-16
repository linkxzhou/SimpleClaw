// SOP 加载器：从 workspace/sops/ 目录加载 SOP 定义。
// 每个 SOP 是一个子目录，包含 sop.json（元数据）和 steps.md（步骤定义）。

package sop

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// stepHeaderRegex 匹配 "## Step N: Title [checkpoint]"
var stepHeaderRegex = regexp.MustCompile(`^##\s+Step\s+(\d+)\s*:\s*(.+)$`)

// LoadSOPs 从 workspace 目录加载所有 SOP。
func LoadSOPs(workspaceDir string) ([]Sop, error) {
	sopsDir := filepath.Join(workspaceDir, "sops")
	entries, err := os.ReadDir(sopsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read sops dir: %w", err)
	}

	var sops []Sop
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sopDir := filepath.Join(sopsDir, entry.Name())
		sop, err := loadSOP(sopDir)
		if err != nil {
			continue // 跳过无效 SOP
		}
		sops = append(sops, *sop)
	}
	return sops, nil
}

// loadSOP 从单个目录加载 SOP。
func loadSOP(dir string) (*Sop, error) {
	// 读取 sop.json
	jsonPath := filepath.Join(dir, "sop.json")
	jsonData, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("read sop.json: %w", err)
	}

	var sop Sop
	if err := json.Unmarshal(jsonData, &sop); err != nil {
		return nil, fmt.Errorf("parse sop.json: %w", err)
	}

	// 读取 steps.md
	stepsPath := filepath.Join(dir, "steps.md")
	stepsData, err := os.ReadFile(stepsPath)
	if err != nil {
		return nil, fmt.Errorf("read steps.md: %w", err)
	}

	sop.Steps = ParseSteps(string(stepsData))

	// 如果 name 为空，用目录名
	if sop.Name == "" {
		sop.Name = filepath.Base(dir)
	}

	return &sop, nil
}

// ParseSteps 解析 Markdown 步骤定义。
// 格式：
//
//	## Step 1: Title [checkpoint]
//	tools: exec, write_file
//	步骤描述内容...
func ParseSteps(markdown string) []SopStep {
	lines := strings.Split(markdown, "\n")
	var steps []SopStep
	var current *SopStep
	var bodyLines []string

	flush := func() {
		if current != nil {
			current.Body = strings.TrimSpace(strings.Join(bodyLines, "\n"))
			steps = append(steps, *current)
			current = nil
			bodyLines = nil
		}
	}

	for _, line := range lines {
		matches := stepHeaderRegex.FindStringSubmatch(line)
		if matches != nil {
			flush()

			num, _ := strconv.Atoi(matches[1])
			title := strings.TrimSpace(matches[2])

			isCheckpoint := false
			if strings.Contains(strings.ToLower(title), "[checkpoint]") {
				isCheckpoint = true
				title = strings.TrimSpace(strings.Replace(title, "[checkpoint]", "", 1))
			}

			current = &SopStep{
				Number:       num,
				Title:        title,
				IsCheckpoint: isCheckpoint,
			}
			continue
		}

		// 解析 tools: 行
		if current != nil && strings.HasPrefix(strings.TrimSpace(line), "tools:") {
			toolsStr := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "tools:"))
			for _, t := range strings.Split(toolsStr, ",") {
				t = strings.TrimSpace(t)
				if t != "" {
					current.SuggestedTools = append(current.SuggestedTools, t)
				}
			}
			continue
		}

		if current != nil {
			bodyLines = append(bodyLines, line)
		}
	}

	flush()
	return steps
}
