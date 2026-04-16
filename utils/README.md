# utils — 通用工具函数

SimpleClaw 的通用工具包，提供目录管理、路径处理、字符串操作、Token 估算和日志配置等基础功能。

## 文件结构

| 文件 | 说明 |
|------|------|
| `helpers.go` | 通用工具函数（目录、路径、字符串、时间、会话键解析） |
| `log.go` | 日志系统（slog + lumberjack 日志轮转） |
| `tokens.go` | Token 估算器（轻量级，不依赖外部 tokenizer） |
| `helpers_test.go` | 工具函数测试 |
| `log_test.go` | 日志测试 |
| `tokens_test.go` | Token 估算测试 |

## 核心功能

### 目录与路径

| 函数 | 说明 |
|------|------|
| `EnsureDir(path)` | 确保目录存在，不存在则创建 |
| `GetDataPath()` | 返回 `~/.simpleclaw` 数据目录 |
| `ExpandHome(path)` | 展开 `~` 前缀为用户主目录 |
| `ValidatePath(path, workspace, restrict)` | 路径安全验证（防止 workspace 外访问） |

### 字符串处理

| 函数 | 说明 |
|------|------|
| `Truncate(s, maxLen)` | 截断字符串，超长部分用 `...` 替代 |
| `TruncateRunes(s, maxRunes)` | 按 rune 截断（正确处理 CJK） |
| `ParseSessionKey(key)` | 解析 `channel:chatID` 格式的会话键 |

### 时间格式化

| 函数 | 说明 |
|------|------|
| `FormatTimestamp(t)` | 格式化为 `2006-01-02 15:04:05` |
| `NowDateString()` | 返回当天日期字符串 `YYYY-MM-DD` |

### Token 估算

轻量级实现，不依赖外部 tokenizer 库，误差目标 ±30%：

```go
tokens := utils.EstimateTokens("Hello 你好世界")
// 英文 ≈ 0.25 token/char, CJK ≈ 0.6 token/char, 其他 ≈ 0.5 token/char
```

| 函数 | 说明 |
|------|------|
| `EstimateTokens(text)` | 估算文本 token 数 |

### 日志系统

基于 `slog` + `lumberjack` 日志轮转：

```go
logger := utils.NewLogger(utils.LogConfig{
    Level:      utils.LevelInfo,
    Dir:        "~/.simpleclaw/logs",
    MaxSizeMB:  50,
    MaxBackups: 3,
    MaxAgeDays: 30,
})
```

| 函数/类型 | 说明 |
|-----------|------|
| `NewLogger(config)` | 创建带日志轮转的 slog.Logger |
| `LogConfig` | 日志配置（级别、目录、轮转参数） |
| `LevelDebug/Info/Warn/Error` | 预定义日志级别 |
