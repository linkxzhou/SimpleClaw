# SimpleClaw 重构方案 — 多 Agent 路由与安全体系

## 1. 多 Agent 路由系统

### 1.1 现状分析

当前 SimpleClaw 是**单 Agent 架构**：
- `agent/agent.go` 中的 `Agent` 直接处理所有消息
- 无路由机制，所有 Channel 的消息走同一个处理流程
- 无法为不同场景（工作/生活/编程）分配不同的 Agent 人格

PicoClaw 实现了完整的 7 级路由，Nanobot 保持了单 Agent 简约设计。

### 1.2 设计方案：渐进式多 Agent

**设计原则**：先实现基础路由框架，保持单 Agent 兼容，支持未来扩展到多 Agent。

#### 1.2.1 Agent 实例化

```go
// agent/instance.go

type AgentInstance struct {
    ID              string
    Model           string           // "vendor/model"
    Workspace       string           // 工作区路径
    MaxTokens       int
    Temperature     float64
    MaxIterations   int
    RestrictToWS    bool             // 工作区沙箱
    IsDefault       bool

    contextBuilder  *ContextBuilder
    toolRegistry    *tools.Registry
    sessionManager  *session.Manager
    memoryStore     *MemoryStore
    skillsLoader    *SkillsLoader
}
```

#### 1.2.2 Agent 注册表

```go
// agent/registry.go

type AgentRegistry struct {
    mu     sync.RWMutex
    agents map[string]*AgentInstance
}

func (r *AgentRegistry) Register(agent *AgentInstance)
func (r *AgentRegistry) Get(id string) (*AgentInstance, bool)
func (r *AgentRegistry) GetDefault() *AgentInstance
func (r *AgentRegistry) List() []*AgentInstance
```

#### 1.2.3 路由解析器

```go
// routing/route.go

type RouteResolver struct {
    bindings []BindingConfig
    agents   *AgentRegistry
}

type ResolvedRoute struct {
    AgentID    string
    SessionKey string
    MatchedBy  string   // 匹配方式描述
}

// 7 级优先级路由（与 PicoClaw 一致）
func (r *RouteResolver) Resolve(channel, accountID, chatID string) *ResolvedRoute {
    // 优先级: peer > parent_peer > guild > team > account > channel_wildcard > default
    for _, binding := range r.bindings {
        if match := r.matchBinding(binding, channel, accountID, chatID); match != nil {
            return match
        }
    }
    // 回退到默认 Agent
    return &ResolvedRoute{
        AgentID:    r.agents.GetDefault().ID,
        SessionKey: buildSessionKey(channel, chatID),
        MatchedBy:  "default",
    }
}
```

#### 1.2.4 配置结构

```json
{
  "agents": {
    "defaults": {
      "model": "anthropic/claude-sonnet-4-20250514",
      "workspace": "~/.simpleclaw/workspace",
      "max_tokens": 8192,
      "max_tool_iterations": 20,
      "restrict_to_workspace": true
    },
    "list": [
      {
        "id": "coder",
        "model": "anthropic/claude-sonnet-4-20250514",
        "workspace": "~/projects",
        "restrict_to_workspace": false
      },
      {
        "id": "assistant",
        "model": "zhipu/glm-4",
        "default": true
      }
    ]
  },
  "bindings": [
    {
      "agent": "coder",
      "channel": "telegram",
      "peer": "123456789"
    },
    {
      "agent": "assistant",
      "channel": "*"
    }
  ]
}
```

### 1.3 AgentLoop 重构

```go
// agent/loop.go (重构后)

type AgentLoop struct {
    bus      MessageBus
    registry *AgentRegistry
    router   *RouteResolver
    fallback *FallbackChain
    state    *StateManager
}

func (l *AgentLoop) Run(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case msg := <-l.bus.Inbound():
            go l.processMessage(ctx, msg)
        }
    }
}

func (l *AgentLoop) processMessage(ctx context.Context, msg InboundMessage) {
    // 1. 路由到 Agent
    route := l.router.Resolve(msg.Channel, msg.SenderID, msg.ChatID)

    // 2. 获取 Agent 实例
    agent, _ := l.registry.Get(route.AgentID)

    // 3. 获取/创建 Session
    session := agent.sessionManager.GetOrCreate(route.SessionKey)

    // 4. 构建上下文
    messages := agent.contextBuilder.BuildMessages(session, msg)

    // 5. 执行 Agent 循环（含 FallbackChain）
    reply := l.agentLoop(ctx, agent, messages)

    // 6. 保存 Session
    session.AddMessage(Message{Role: "user", Content: msg.Content})
    session.AddMessage(Message{Role: "assistant", Content: reply})
    agent.sessionManager.Save(session)

    // 7. 更新状态
    l.state.Update(msg.Channel, msg.ChatID)

    // 8. 发送回复
    l.bus.PublishOutbound(OutboundMessage{
        Channel: msg.Channel,
        ChatID:  msg.ChatID,
        Content: reply,
    })
}
```

---

## 2. Shell 命令安全模型

### 2.1 现状分析

当前 `agent/tools/shell.go` 仅实现了基础的超时控制（60s），无任何命令安全检查。这在生产环境中存在严重风险。

### 2.2 安全防护设计

参考 PicoClaw 的 ~40 条拒绝规则，设计分层安全模型：

#### 2.2.1 命令拒绝列表

```go
// agent/tools/shell.go

var defaultDenyPatterns = []*regexp.Regexp{
    // === 文件删除 ===
    regexp.MustCompile(`(?i)\brm\s+.*-[rR]`),          // rm -r/-R
    regexp.MustCompile(`(?i)\brm\s+.*-f`),              // rm -f
    regexp.MustCompile(`(?i)\brmdir\s+/`),              // rmdir /
    regexp.MustCompile(`(?i)\bdel\s+/[fFsS]`),          // Windows del

    // === 磁盘操作 ===
    regexp.MustCompile(`(?i)\bdd\s+if=`),               // dd
    regexp.MustCompile(`(?i)\bmkfs\b`),                  // mkfs
    regexp.MustCompile(`(?i)\bformat\s+[a-z]:`),         // Windows format

    // === 系统操作 ===
    regexp.MustCompile(`(?i)\bshutdown\b`),
    regexp.MustCompile(`(?i)\breboot\b`),
    regexp.MustCompile(`(?i)\binit\s+[06]\b`),

    // === 权限提升 ===
    regexp.MustCompile(`(?i)\bsudo\b`),
    regexp.MustCompile(`(?i)\bsu\s+-`),
    regexp.MustCompile(`(?i)\bchmod\s+.*[0-7]{3,4}`),
    regexp.MustCompile(`(?i)\bchown\b`),

    // === 命令注入 ===
    regexp.MustCompile(`\$\(`),                          // $(...)
    regexp.MustCompile("`.+`"),                          // `...`
    regexp.MustCompile(`(?i)\|\s*(sh|bash|zsh|fish)\b`), // | sh
    regexp.MustCompile(`(?i)\beval\b`),

    // === 进程管理 ===
    regexp.MustCompile(`(?i)\bkill\s+-9`),
    regexp.MustCompile(`(?i)\bkillall\b`),
    regexp.MustCompile(`(?i)\bpkill\b`),

    // === 网络危险操作 ===
    regexp.MustCompile(`(?i)curl.*\|\s*(sh|bash)`),      // curl | sh
    regexp.MustCompile(`(?i)wget.*\|\s*(sh|bash)`),      // wget | sh
    regexp.MustCompile(`(?i)\bssh\b`),

    // === 包管理全局安装 ===
    regexp.MustCompile(`(?i)\bnpm\s+install\s+-g`),
    regexp.MustCompile(`(?i)\bpip\s+install\s+--user`),
    regexp.MustCompile(`(?i)\bapt\s+(install|remove)`),

    // === 容器操作 ===
    regexp.MustCompile(`(?i)\bdocker\s+(run|exec)`),

    // === Git 危险操作 ===
    regexp.MustCompile(`(?i)\bgit\s+push\b`),
    regexp.MustCompile(`(?i)\bgit\s+.*--force`),

    // === Fork bomb ===
    regexp.MustCompile(`:\(\)\s*\{`),

    // === 路径遍历 ===
    regexp.MustCompile(`\.\.\/`),
}
```

#### 2.2.2 路径遍历防护

```go
func (t *ExecTool) validateWorkspace(command, workspace string) error {
    // 检测绝对路径是否在工作区外
    absPathRegex := regexp.MustCompile(`(?:^|\s)\/[^\s]+`)
    for _, path := range absPathRegex.FindAllString(command, -1) {
        path = strings.TrimSpace(path)
        if !strings.HasPrefix(path, workspace) &&
           !strings.HasPrefix(path, "/tmp") &&
           !strings.HasPrefix(path, "/usr/bin") &&
           !strings.HasPrefix(path, "/usr/local/bin") {
            return fmt.Errorf("path %s is outside workspace", path)
        }
    }
    return nil
}
```

#### 2.2.3 可配置白名单

```go
type ExecToolConfig struct {
    DenyPatterns  []*regexp.Regexp  // 自定义拒绝模式
    AllowPatterns []*regexp.Regexp  // 白名单（覆盖拒绝）
    Workspace     string            // 工作区根路径
    Timeout       time.Duration     // 超时（默认 60s）
    MaxOutput     int               // 最大输出（默认 10000 字符）
}
```

#### 2.2.4 执行流程

```go
func (t *ExecTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
    command := args["command"].(string)

    // 1. 白名单检查（优先通过）
    if t.matchesAllowList(command) {
        return t.executeCommand(ctx, command)
    }

    // 2. 拒绝列表检查
    if reason := t.matchesDenyList(command); reason != "" {
        return "", fmt.Errorf("command blocked: %s", reason)
    }

    // 3. 工作区路径检查
    if t.config.Workspace != "" {
        if err := t.validateWorkspace(command, t.config.Workspace); err != nil {
            return "", err
        }
    }

    // 4. 执行
    return t.executeCommand(ctx, command)
}
```

#### 2.2.5 进程树终止

```go
func (t *ExecTool) executeCommand(ctx context.Context, command string) (string, error) {
    ctx, cancel := context.WithTimeout(ctx, t.config.Timeout)
    defer cancel()

    cmd := exec.CommandContext(ctx, "sh", "-c", command)
    cmd.Dir = t.config.Workspace
    cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // 进程组

    // ... 执行 ...

    // 超时时终止整个进程树
    if ctx.Err() == context.DeadlineExceeded {
        syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
        time.AfterFunc(2*time.Second, func() {
            syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
        })
    }
}
```

---

## 3. 工作区沙箱

### 3.1 设计目标

限制文件系统工具（read_file, write_file, edit_file, list_dir）只能访问工作区目录内的文件。

### 3.2 路径验证

```go
// agent/tools/fs.go

func validatePath(requestedPath, workspace string, restrictToWorkspace bool) (string, error) {
    // 1. 展开 ~ 为 home 目录
    if strings.HasPrefix(requestedPath, "~") {
        home, _ := os.UserHomeDir()
        requestedPath = filepath.Join(home, requestedPath[1:])
    }

    // 2. 解析为绝对路径
    absPath, err := filepath.Abs(requestedPath)
    if err != nil {
        return "", err
    }

    // 3. 解析符号链接（防止 symlink 逃逸）
    realPath, err := filepath.EvalSymlinks(absPath)
    if err != nil && !os.IsNotExist(err) {
        return "", err
    }
    if realPath != "" {
        absPath = realPath
    }

    // 4. 工作区限制检查
    if restrictToWorkspace {
        wsAbs, _ := filepath.Abs(workspace)
        if !strings.HasPrefix(absPath, wsAbs+string(filepath.Separator)) && absPath != wsAbs {
            return "", fmt.Errorf("access denied: %s is outside workspace %s", requestedPath, workspace)
        }
    }

    return absPath, nil
}
```

### 3.3 配置控制

```json
{
  "agents": {
    "defaults": {
      "restrict_to_workspace": true
    },
    "list": [
      {
        "id": "coder",
        "restrict_to_workspace": false,
        "workspace": "~/projects"
      }
    ]
  }
}
```

---

## 4. State 管理（原子持久化）

### 4.1 设计目标

持久化追踪最近活跃的 Channel/ChatID，供心跳服务和设备事件使用。

### 4.2 实现

```go
// state/state.go

type StateManager struct {
    mu       sync.RWMutex
    filePath string
    state    State
}

type State struct {
    LastChannel string    `json:"last_channel"`
    LastChatID  string    `json:"last_chat_id"`
    Timestamp   time.Time `json:"timestamp"`
}

// 原子保存：temp file + rename（POSIX 原子性保证）
func (sm *StateManager) Save() error {
    sm.mu.RLock()
    defer sm.mu.RUnlock()

    data, _ := json.MarshalIndent(sm.state, "", "  ")

    tmpPath := sm.filePath + ".tmp"
    if err := os.WriteFile(tmpPath, data, 0644); err != nil {
        return err
    }
    return os.Rename(tmpPath, sm.filePath) // 原子操作
}

func (sm *StateManager) Update(channel, chatID string) {
    sm.mu.Lock()
    defer sm.mu.Unlock()
    sm.state.LastChannel = channel
    sm.state.LastChatID = chatID
    sm.state.Timestamp = time.Now()
    go sm.Save() // 异步保存
}
```

---

## 5. 健康检查服务

### 5.1 HTTP 端点

```go
// health/server.go

type HealthServer struct {
    addr      string
    startTime time.Time
    ready     atomic.Bool
    checks    map[string]func() bool
}

// GET /health — 始终 200（存活探针）
func (h *HealthServer) healthHandler(w http.ResponseWriter, r *http.Request) {
    json.NewEncoder(w).Encode(map[string]any{
        "status": "ok",
        "uptime": time.Since(h.startTime).String(),
    })
}

// GET /ready — 就绪探针（检查所有依赖）
func (h *HealthServer) readyHandler(w http.ResponseWriter, r *http.Request) {
    allReady := h.ready.Load()
    for name, check := range h.checks {
        if !check() {
            allReady = false
            // 记录不健康的检查项
        }
    }
    if allReady {
        w.WriteHeader(http.StatusOK)
    } else {
        w.WriteHeader(http.StatusServiceUnavailable)
    }
}
```

---

## 6. 实施计划

| 阶段 | 模块 | 工作量 | 优先级 |
|------|------|--------|--------|
| Phase 1 | Shell 安全模型 | 2天 | P0 (安全) |
| Phase 2 | 工作区沙箱 (validatePath) | 1天 | P0 (安全) |
| Phase 3 | State 管理器 | 0.5天 | P1 |
| Phase 4 | Agent 实例化 + 注册表 | 2天 | P1 |
| Phase 5 | Route 解析器 | 1.5天 | P1 |
| Phase 6 | AgentLoop 重构 | 2天 | P1 |
| Phase 7 | 健康检查服务 | 0.5天 | P2 |
| Phase 8 | 单元测试 | 2天 | P1 |

**总计**：~11.5 天
