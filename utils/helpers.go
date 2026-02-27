// Package utils 提供 SimpleClaw 的通用工具函数。
// 包含目录管理、路径处理、字符串操作、时间格式化和会话键解析等功能。
package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// EnsureDir 确保目录存在，不存在则创建。
// 返回路径本身，便于链式调用。
func EnsureDir(path string) (string, error) {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", fmt.Errorf("ensure dir %s: %w", path, err)
	}
	return path, nil
}

// GetDataPath 返回 SimpleClaw 数据目录路径（~/.simpleclaw）。
// 如果目录不存在则自动创建。
func GetDataPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return EnsureDir(filepath.Join(home, ".simpleclaw"))
}

// GetWorkspacePath 返回工作区路径。
// workspace 为空时使用默认路径（~/.simpleclaw/workspace）。
// 支持 ~ 前缀展开为用户主目录。
func GetWorkspacePath(workspace string) (string, error) {
	if workspace != "" {
		// 展开 ~ 前缀
		path := ExpandHome(workspace)
		return EnsureDir(path)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return EnsureDir(filepath.Join(home, ".simpleclaw", "workspace"))
}

// GetSessionsPath 返回会话存储目录路径。
func GetSessionsPath() (string, error) {
	dataPath, err := GetDataPath()
	if err != nil {
		return "", err
	}
	return EnsureDir(filepath.Join(dataPath, "sessions"))
}

// GetMemoryPath 返回工作区中的记忆目录路径。
// workspace 为空时使用默认工作区路径。
func GetMemoryPath(workspace string) (string, error) {
	ws := workspace
	if ws == "" {
		var err error
		ws, err = GetWorkspacePath("")
		if err != nil {
			return "", err
		}
	}
	return EnsureDir(filepath.Join(ws, "memory"))
}

// GetSkillsPath 返回工作区中的技能目录路径。
// workspace 为空时使用默认工作区路径。
func GetSkillsPath(workspace string) (string, error) {
	ws := workspace
	if ws == "" {
		var err error
		ws, err = GetWorkspacePath("")
		if err != nil {
			return "", err
		}
	}
	return EnsureDir(filepath.Join(ws, "skills"))
}

// TodayDate 返回今天的日期（YYYY-MM-DD 格式）。
func TodayDate() string {
	return time.Now().Format("2006-01-02")
}

// Timestamp 返回当前时间戳（ISO 8601 / RFC3339 格式）。
func Timestamp() string {
	return time.Now().Format(time.RFC3339)
}

// TruncateString 截断字符串到 maxLen 长度，超出部分用 suffix 替代。
// suffix 为空时默认使用 "..."。
func TruncateString(s string, maxLen int, suffix string) string {
	if suffix == "" {
		suffix = "..."
	}
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= len(suffix) {
		return suffix[:maxLen]
	}
	return s[:maxLen-len(suffix)] + suffix
}

// SafeFilename 将字符串转换为安全的文件名。
// 替换不安全字符（<>:"/\|?*）为下划线，并去除首尾空格。
func SafeFilename(name string) string {
	const unsafe = `<>:"/\|?*`
	for _, ch := range unsafe {
		name = strings.ReplaceAll(name, string(ch), "_")
	}
	return strings.TrimSpace(name)
}

// ParseSessionKey 解析 "channel:chatID" 格式的会话键。
// 返回渠道名和聊天 ID，格式无效时返回错误。
func ParseSessionKey(key string) (channel, chatID string, err error) {
	idx := strings.Index(key, ":")
	if idx < 0 {
		return "", "", fmt.Errorf("invalid session key: %s", key)
	}
	return key[:idx], key[idx+1:], nil
}

// BuildSessionKey 从渠道名和聊天 ID 构建会话键。
func BuildSessionKey(channel, chatID string) string {
	return channel + ":" + chatID
}

// ExpandHome 将路径中的 ~ 前缀展开为用户主目录。
func ExpandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}

// ValidatePath 验证请求路径是否在允许范围内。
// 如果 restrictToWorkspace 为 true，路径必须在 workspace 目录内。
// 自动展开 ~ 前缀，解析符号链接防止逃逸。
func ValidatePath(requestedPath, workspace string, restrictToWorkspace bool) (string, error) {
	// 展开 ~ 前缀
	if strings.HasPrefix(requestedPath, "~/") || requestedPath == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot resolve home directory: %w", err)
		}
		requestedPath = filepath.Join(home, requestedPath[1:])
	}

	// 解析为绝对路径
	absPath, err := filepath.Abs(requestedPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	// 尝试解析符号链接（防止 symlink 逃逸）
	realPath, err := filepath.EvalSymlinks(absPath)
	if err == nil {
		absPath = realPath
	}
	// 文件不存在时 EvalSymlinks 会失败，这是正常的（新文件写入场景）

	// 工作区限制检查
	if restrictToWorkspace && workspace != "" {
		wsAbs, err := filepath.Abs(workspace)
		if err != nil {
			return "", fmt.Errorf("invalid workspace path: %w", err)
		}
		wsReal, err := filepath.EvalSymlinks(wsAbs)
		if err == nil {
			wsAbs = wsReal
		}

		if absPath != wsAbs && !strings.HasPrefix(absPath, wsAbs+string(filepath.Separator)) {
			return "", fmt.Errorf("access denied: %s is outside workspace %s", requestedPath, workspace)
		}
	}

	return absPath, nil
}
