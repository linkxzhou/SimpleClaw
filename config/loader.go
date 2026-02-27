// 配置文件的加载、保存和命名转换。
// 支持从 JSON 文件加载配置，文件不存在时返回默认值。

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// GetConfigPath 按优先级返回配置文件路径：
//  1. 当前工作目录下 .simpleclaw/config.json（本地项目级配置）
//  2. ~/.simpleclaw/config.json（用户全局配置）
func GetConfigPath() string {
	// 优先使用本地目录配置
	localPath := filepath.Join(".simpleclaw", "config.json")
	if _, err := os.Stat(localPath); err == nil {
		if abs, err := filepath.Abs(localPath); err == nil {
			return abs
		}
		return localPath
	}
	// 回退到用户主目录配置
	return filepath.Join(homeDir(), ".simpleclaw", "config.json")
}

// Load 从指定路径加载配置。
// configPath 为空时使用默认路径。
// 文件不存在时返回默认配置。
// JSON 解析失败时输出警告并返回默认配置。
func Load(configPath string) (*Config, error) {
	if configPath == "" {
		configPath = GetConfigPath()
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		fmt.Printf("Warning: failed to load config %s: %v\nUsing defaults.\n", configPath, err)
		return DefaultConfig(), nil
	}

	return cfg, nil
}

// Save 将配置保存到指定路径。
// configPath 为空时使用默认路径。
// 自动创建父目录，输出格式化的 JSON（带缩进）。
func Save(cfg *Config, configPath string) error {
	if configPath == "" {
		configPath = GetConfigPath()
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	return os.WriteFile(configPath, data, 0o644)
}

// homeDir 返回用户主目录，失败时 panic。
func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(fmt.Sprintf("cannot get home dir: %v", err))
	}
	return home
}
