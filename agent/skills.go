package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// SkillsLoader 技能加载器
// 技能是 Markdown 文件（SKILL.md），教 Agent 如何使用特定工具或执行特定任务
type SkillsLoader struct {
	workspace       string
	workspaceSkills string
	builtinSkills   string
}

// SkillInfo 技能信息
type SkillInfo struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Source string `json:"source"` // workspace 或 builtin
}

// SkillMetadata 技能元数据
type SkillMetadata struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Always      bool   `json:"always"`
	Metadata    string `json:"metadata"`
}

// SkillRequires 技能依赖
type SkillRequires struct {
	Bins []string `json:"bins"`
	Env  []string `json:"env"`
}

// NewSkillsLoader 创建技能加载器
func NewSkillsLoader(workspace string, builtinSkillsDir string) *SkillsLoader {
	return &SkillsLoader{
		workspace:       workspace,
		workspaceSkills: filepath.Join(workspace, "skills"),
		builtinSkills:   builtinSkillsDir,
	}
}

// ListSkills 列出所有可用技能
func (s *SkillsLoader) ListSkills(filterUnavailable bool) []SkillInfo {
	var skills []SkillInfo
	nameSet := make(map[string]bool)

	// Workspace 技能（优先）
	if entries, err := os.ReadDir(s.workspaceSkills); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			skillFile := filepath.Join(s.workspaceSkills, entry.Name(), "SKILL.md")
			if _, err := os.Stat(skillFile); err == nil {
				skills = append(skills, SkillInfo{
					Name:   entry.Name(),
					Path:   skillFile,
					Source: "workspace",
				})
				nameSet[entry.Name()] = true
			}
		}
	}

	// 内置技能
	if s.builtinSkills != "" {
		if entries, err := os.ReadDir(s.builtinSkills); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() || nameSet[entry.Name()] {
					continue
				}
				skillFile := filepath.Join(s.builtinSkills, entry.Name(), "SKILL.md")
				if _, err := os.Stat(skillFile); err == nil {
					skills = append(skills, SkillInfo{
						Name:   entry.Name(),
						Path:   skillFile,
						Source: "builtin",
					})
				}
			}
		}
	}

	// 过滤不可用的技能
	if filterUnavailable {
		var available []SkillInfo
		for _, skill := range skills {
			meta := s.getSkillMeta(skill.Name)
			if s.checkRequirements(meta) {
				available = append(available, skill)
			}
		}
		return available
	}

	return skills
}

// LoadSkill 按名称加载技能内容
func (s *SkillsLoader) LoadSkill(name string) string {
	// 先检查 workspace
	path := filepath.Join(s.workspaceSkills, name, "SKILL.md")
	if content, err := os.ReadFile(path); err == nil {
		return string(content)
	}

	// 再检查 builtin
	if s.builtinSkills != "" {
		path = filepath.Join(s.builtinSkills, name, "SKILL.md")
		if content, err := os.ReadFile(path); err == nil {
			return string(content)
		}
	}

	return ""
}

// LoadSkillsForContext 加载指定技能用于 Agent 上下文
func (s *SkillsLoader) LoadSkillsForContext(names []string) string {
	var parts []string
	for _, name := range names {
		content := s.LoadSkill(name)
		if content != "" {
			content = s.stripFrontmatter(content)
			parts = append(parts, fmt.Sprintf("### Skill: %s\n\n%s", name, content))
		}
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// BuildSkillsSummary 构建技能摘要（XML 格式，用于渐进式加载）
func (s *SkillsLoader) BuildSkillsSummary() string {
	allSkills := s.ListSkills(false)
	if len(allSkills) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, "<skills>")
	for _, skill := range allSkills {
		name := escapeXML(skill.Name)
		desc := escapeXML(s.getSkillDescription(skill.Name))
		meta := s.getSkillMeta(skill.Name)
		available := s.checkRequirements(meta)

		lines = append(lines, fmt.Sprintf(`  <skill available="%v">`, available))
		lines = append(lines, fmt.Sprintf("    <name>%s</name>", name))
		lines = append(lines, fmt.Sprintf("    <description>%s</description>", desc))
		lines = append(lines, fmt.Sprintf("    <location>%s</location>", skill.Path))

		if !available {
			missing := s.getMissingRequirements(meta)
			if missing != "" {
				lines = append(lines, fmt.Sprintf("    <requires>%s</requires>", escapeXML(missing)))
			}
		}

		lines = append(lines, "  </skill>")
	}
	lines = append(lines, "</skills>")
	return strings.Join(lines, "\n")
}

// GetAlwaysSkills 获取标记为 always 的技能
func (s *SkillsLoader) GetAlwaysSkills() []string {
	var result []string
	for _, skill := range s.ListSkills(true) {
		meta := s.GetSkillMetadata(skill.Name)
		if meta == nil {
			continue
		}
		skillMeta := s.getSkillMeta(skill.Name)
		if skillMeta.Bins != nil || meta.Always {
			result = append(result, skill.Name)
		}
		// 检查 nanobot metadata 中的 always 字段
		nbMeta := s.parseNanobotMetadata(meta.Metadata)
		if alwaysVal, ok := nbMeta["always"]; ok {
			if b, ok := alwaysVal.(bool); ok && b {
				result = append(result, skill.Name)
			}
		}
	}
	// 去重
	seen := make(map[string]bool)
	var unique []string
	for _, name := range result {
		if !seen[name] {
			seen[name] = true
			unique = append(unique, name)
		}
	}
	return unique
}

// GetSkillMetadata 从技能的 frontmatter 中获取元数据
func (s *SkillsLoader) GetSkillMetadata(name string) *SkillMetadata {
	content := s.LoadSkill(name)
	if content == "" {
		return nil
	}

	if !strings.HasPrefix(content, "---") {
		return nil
	}

	re := regexp.MustCompile(`(?s)^---\n(.*?)\n---`)
	match := re.FindStringSubmatch(content)
	if len(match) < 2 {
		return nil
	}

	// 简单的 YAML 解析
	meta := &SkillMetadata{}
	for _, line := range strings.Split(match[1], "\n") {
		if idx := strings.Index(line, ":"); idx >= 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			value = strings.Trim(value, "\"'")
			switch key {
			case "name":
				meta.Name = value
			case "description":
				meta.Description = value
			case "always":
				meta.Always = value == "true"
			case "metadata":
				meta.Metadata = value
			}
		}
	}
	return meta
}

// ============ 内部方法 ============

func (s *SkillsLoader) getSkillDescription(name string) string {
	meta := s.GetSkillMetadata(name)
	if meta != nil && meta.Description != "" {
		return meta.Description
	}
	return name
}

func (s *SkillsLoader) stripFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---") {
		return content
	}
	re := regexp.MustCompile(`(?s)^---\n.*?\n---\n`)
	return strings.TrimSpace(re.ReplaceAllString(content, ""))
}

func (s *SkillsLoader) parseNanobotMetadata(raw string) map[string]interface{} {
	if raw == "" {
		return nil
	}
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil
	}
	if nanobot, ok := data["nanobot"].(map[string]interface{}); ok {
		return nanobot
	}
	return nil
}

func (s *SkillsLoader) getSkillMeta(name string) *SkillRequires {
	meta := s.GetSkillMetadata(name)
	if meta == nil {
		return &SkillRequires{}
	}
	nbMeta := s.parseNanobotMetadata(meta.Metadata)
	if nbMeta == nil {
		return &SkillRequires{}
	}
	requires := &SkillRequires{}
	if reqMap, ok := nbMeta["requires"].(map[string]interface{}); ok {
		if bins, ok := reqMap["bins"].([]interface{}); ok {
			for _, b := range bins {
				if str, ok := b.(string); ok {
					requires.Bins = append(requires.Bins, str)
				}
			}
		}
		if envs, ok := reqMap["env"].([]interface{}); ok {
			for _, e := range envs {
				if str, ok := e.(string); ok {
					requires.Env = append(requires.Env, str)
				}
			}
		}
	}
	return requires
}

func (s *SkillsLoader) checkRequirements(requires *SkillRequires) bool {
	if requires == nil {
		return true
	}
	for _, b := range requires.Bins {
		if _, err := exec.LookPath(b); err != nil {
			return false
		}
	}
	for _, env := range requires.Env {
		if os.Getenv(env) == "" {
			return false
		}
	}
	return true
}

func (s *SkillsLoader) getMissingRequirements(requires *SkillRequires) string {
	if requires == nil {
		return ""
	}
	var missing []string
	for _, b := range requires.Bins {
		if _, err := exec.LookPath(b); err != nil {
			missing = append(missing, "CLI: "+b)
		}
	}
	for _, env := range requires.Env {
		if os.Getenv(env) == "" {
			missing = append(missing, "ENV: "+env)
		}
	}
	return strings.Join(missing, ", ")
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
