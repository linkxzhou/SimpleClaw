# SimpleClaw 重构方案 — Channel / 工具 / 技能增强

## 1. Channel 层完善

### 1.1 现状

当前 `channels/` 目录已有基础框架：
- `base.go`: `Channel` 接口 + `BaseChannel` 通用实现（白名单、HandleMessage）
- `manager.go`: 通道管理器（初始化、启动、出站分发）
- `telegram.go`: Markdown→HTML 转换已实现，Bot API 集成 **TODO**
- `whatsapp.go`: Bridge 消息协议已定义，WebSocket 连接 **TODO**

### 1.2 Telegram 完整实现

#### 1.2.1 依赖选择

参考 PicoClaw 直接使用 Telegram Bot HTTP API（无第三方 SDK），保持零依赖：

```go
// channels/telegram.go

type TelegramChannel struct {
    BaseChannel
    token   string
    bus     MessageBus
    client  *http.Client
    offset  int64       // long polling offset
    cancel  context.CancelFunc
}
```

#### 1.2.2 Long Polling 消息接收

```go
func (t *TelegramChannel) Start(ctx context.Context) error {
    ctx, t.cancel = context.WithCancel(ctx)
    go t.pollLoop(ctx)
    return nil
}

func (t *TelegramChannel) pollLoop(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        default:
            updates, err := t.getUpdates(ctx, t.offset, 30) // 30s long poll
            if err != nil {
                time.Sleep(5 * time.Second)
                continue
            }
            for _, update := range updates {
                t.offset = update.UpdateID + 1
                t.handleUpdate(update)
            }
        }
    }
}

func (t *TelegramChannel) getUpdates(ctx context.Context, offset int64, timeout int) ([]Update, error) {
    url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=%d",
        t.token, offset, timeout)
    // HTTP GET + JSON 解码
}
```

#### 1.2.3 消息发送

```go
func (t *TelegramChannel) Send(msg OutboundMessage) error {
    // 将 Markdown 转为 Telegram HTML（已有 convertMarkdownToHTML）
    html := convertMarkdownToHTML(msg.Content)

    body := map[string]any{
        "chat_id":    msg.ChatID,
        "text":       html,
        "parse_mode": "HTML",
    }

    url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.token)
    // HTTP POST + JSON 编码

    // 消息过长时自动分片（Telegram 限制 4096 字符）
    if len(html) > 4096 {
        return t.sendChunked(msg.ChatID, html, 4096)
    }
}
```

#### 1.2.4 媒体支持

```go
func (t *TelegramChannel) handleUpdate(update Update) {
    msg := update.Message

    var media []MediaItem
    var content string

    switch {
    case msg.Photo != nil:
        // 获取最大尺寸照片
        photo := msg.Photo[len(msg.Photo)-1]
        fileURL := t.getFileURL(photo.FileID)
        media = append(media, MediaItem{Type: "image", URL: fileURL})
        content = msg.Caption

    case msg.Voice != nil:
        // 下载语音 → Groq Whisper 转录
        fileURL := t.getFileURL(msg.Voice.FileID)
        transcript := t.transcribe(fileURL)
        content = fmt.Sprintf("[voice transcript] %s", transcript)

    case msg.Document != nil:
        // 下载文档到 media/ 目录
        fileURL := t.getFileURL(msg.Document.FileID)
        localPath := t.downloadFile(fileURL, msg.Document.FileName)
        content = fmt.Sprintf("[file: %s] %s", localPath, msg.Caption)

    default:
        content = msg.Text
    }

    if content == "" && len(media) == 0 {
        return
    }

    t.HandleMessage(InboundMessage{
        Channel:  "telegram",
        SenderID: fmt.Sprint(msg.From.ID),
        ChatID:   fmt.Sprint(msg.Chat.ID),
        Content:  content,
        Media:    media,
    })
}
```

### 1.3 WhatsApp WebSocket Bridge

#### 1.3.1 架构（与 Nanobot 一致）

```
SimpleClaw ←WebSocket→ Node.js Bridge (baileys) ←→ WhatsApp Server
```

#### 1.3.2 实现

```go
// channels/whatsapp.go

import "github.com/gorilla/websocket"

type WhatsAppChannel struct {
    BaseChannel
    bus       MessageBus
    bridgeURL string        // ws://localhost:3001
    conn      *websocket.Conn
    cancel    context.CancelFunc
}

func (w *WhatsAppChannel) Start(ctx context.Context) error {
    ctx, w.cancel = context.WithCancel(ctx)
    go w.connectLoop(ctx)
    return nil
}

func (w *WhatsAppChannel) connectLoop(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        default:
            conn, _, err := websocket.DefaultDialer.DialContext(ctx, w.bridgeURL, nil)
            if err != nil {
                slog.Error("whatsapp connect failed", "error", err)
                time.Sleep(5 * time.Second)
                continue
            }
            w.conn = conn
            w.readLoop(ctx)  // 阻塞直到断开
        }
    }
}

func (w *WhatsAppChannel) readLoop(ctx context.Context) {
    defer w.conn.Close()
    for {
        _, data, err := w.conn.ReadMessage()
        if err != nil {
            slog.Error("whatsapp read error", "error", err)
            return // 触发重连
        }

        var bridgeMsg BridgeMessage
        json.Unmarshal(data, &bridgeMsg)

        switch bridgeMsg.Type {
        case "message":
            w.HandleMessage(InboundMessage{
                Channel:  "whatsapp",
                SenderID: bridgeMsg.From,
                ChatID:   bridgeMsg.ChatID,
                Content:  bridgeMsg.Content,
            })
        case "qr":
            slog.Info("WhatsApp QR code", "data", bridgeMsg.QR)
        case "status":
            slog.Info("WhatsApp status", "status", bridgeMsg.Status)
        }
    }
}
```

### 1.4 更多 Channel 扩展（按优先级）

| Channel | 依赖 | 优先级 | 备注 |
|---------|------|--------|------|
| Discord | discordgo | P2 | Bot API |
| DingTalk | dingtalk-stream SDK | P2 | Outgoing Bot |
| Feishu | oapi-sdk-go | P2 | Bot Webhook |
| WeCom | 直接 HTTP API | P3 | 企业微信机器人 |
| QQ | botgo | P3 | QQ 开放平台 |
| Slack | slack-go | P3 | Slack Bot |
| OneBot | gorilla/websocket | P3 | OneBot v11 协议 |
| LINE | 直接 HTTP API | P4 | LINE Messaging API |

每个 Channel 遵循相同模式：
1. 实现 `Channel` 接口
2. 嵌入 `BaseChannel`
3. 在 `manager.go` 中注册

### 1.5 Bus 事件模型增强

```go
// bus/events.go (增强)

type MediaItem struct {
    Type    string `json:"type"`     // "image", "audio", "video", "file"
    URL     string `json:"url"`      // 远程 URL 或本地路径
    Name    string `json:"name"`     // 文件名
    MimeType string `json:"mime_type"`
}

type InboundMessage struct {
    Channel   string            `json:"channel"`
    SenderID  string            `json:"sender_id"`
    ChatID    string            `json:"chat_id"`
    Content   string            `json:"content"`
    Media     []MediaItem       `json:"media,omitempty"`      // [新增]
    Metadata  map[string]string `json:"metadata,omitempty"`   // [新增]
    Timestamp time.Time         `json:"timestamp"`
}

type OutboundMessage struct {
    Channel string      `json:"channel"`
    ChatID  string      `json:"chat_id"`
    Content string      `json:"content"`
    ReplyTo string      `json:"reply_to,omitempty"`  // [新增] 回复特定消息
    Media   []MediaItem `json:"media,omitempty"`     // [新增] 发送媒体
}
```

---

## 2. 工具系统增强

### 2.1 新增工具

#### 2.1.1 Cron 管理工具

允许 Agent 在对话中动态创建/管理定时任务：

```go
// agent/tools/cron_tool.go

type CronTool struct {
    cronService *cron.Service
}

func (t *CronTool) Name() string { return "cron" }

func (t *CronTool) Parameters() map[string]any {
    return map[string]any{
        "type": "object",
        "properties": map[string]any{
            "action": map[string]any{
                "type": "string",
                "enum": []string{"list", "add", "remove", "enable", "disable"},
            },
            "name":     map[string]any{"type": "string"},
            "schedule": map[string]any{"type": "string", "description": "Cron expression or 'every N seconds'"},
            "task":     map[string]any{"type": "string", "description": "Task description for agent"},
        },
        "required": []string{"action"},
    }
}

func (t *CronTool) Execute(ctx context.Context, args map[string]any) (string, error) {
    action := args["action"].(string)
    switch action {
    case "list":
        return t.cronService.ListJobs()
    case "add":
        return t.cronService.AddJob(args["name"].(string), args["schedule"].(string), args["task"].(string))
    // ...
    }
}
```

#### 2.1.2 技能搜索/安装工具

```go
// agent/tools/skills_tool.go

type FindSkillsTool struct {
    registry *skills.RegistryManager
}

func (t *FindSkillsTool) Name() string { return "find_skills" }

func (t *FindSkillsTool) Execute(ctx context.Context, args map[string]any) (string, error) {
    query := args["query"].(string)
    results, err := t.registry.SearchAll(ctx, query)
    // 返回搜索结果摘要
}

type InstallSkillTool struct {
    registry *skills.RegistryManager
}

func (t *InstallSkillTool) Name() string { return "install_skill" }

func (t *InstallSkillTool) Execute(ctx context.Context, args map[string]any) (string, error) {
    name := args["name"].(string)
    source := args["source"].(string) // "github" 或 "clawhub"
    return t.registry.Install(ctx, name, source)
}
```

#### 2.1.3 Web 搜索增强

当前仅支持 Brave Search，增加 DuckDuckGo（免费无 Key）：

```go
// agent/tools/web.go (增强)

func (t *WebSearchTool) Execute(ctx context.Context, args map[string]any) (string, error) {
    query := args["query"].(string)

    switch t.config.Engine {
    case "brave":
        return t.searchBrave(ctx, query)
    case "duckduckgo":
        return t.searchDDG(ctx, query)
    case "perplexity":
        return t.searchPerplexity(ctx, query)
    default:
        // 自动选择：有 Brave Key 用 Brave，否则 DuckDuckGo
        if t.config.BraveAPIKey != "" {
            return t.searchBrave(ctx, query)
        }
        return t.searchDDG(ctx, query)
    }
}

func (t *WebSearchTool) searchDDG(ctx context.Context, query string) (string, error) {
    // DuckDuckGo Instant Answer API（免费，无需 Key）
    url := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&no_html=1", url.QueryEscape(query))
    // HTTP GET + 解析响应
}
```

### 2.2 工具接口增强

参考 PicoClaw 的 `ContextualTool` 和 `AsyncTool`：

```go
// agent/tools/registry.go (增强)

// ContextualTool — 需要知道调用上下文（channel/chatID）的工具
type ContextualTool interface {
    Tool
    SetContext(channel, chatID string)
}

// AsyncTool — 异步执行的工具（如 spawn）
type AsyncTool interface {
    Tool
    SetCallback(fn func(result string))
}
```

---

## 3. 技能系统增强

### 3.1 三级加载体系

参考 PicoClaw 的三级覆盖设计，增强当前的双级加载：

```
优先级：Workspace Skills > Global Skills > Builtin Skills
路径：   ~/.simpleclaw/workspace/skills/  >  ~/.simpleclaw/skills/  >  embedded://skills/
```

```go
// skills/loader.go

type SkillsLoader struct {
    workspacePath string   // ~/.simpleclaw/workspace/skills/
    globalPath    string   // ~/.simpleclaw/skills/
    builtinFS     embed.FS // 编译时嵌入
}

func (l *SkillsLoader) LoadAll() ([]*Skill, error) {
    seen := make(map[string]bool)
    var skills []*Skill

    // 1. Workspace skills（最高优先级）
    for _, s := range l.scanDir(l.workspacePath) {
        seen[s.Name] = true
        skills = append(skills, s)
    }

    // 2. Global skills
    for _, s := range l.scanDir(l.globalPath) {
        if !seen[s.Name] {
            seen[s.Name] = true
            skills = append(skills, s)
        }
    }

    // 3. Builtin skills
    for _, s := range l.scanEmbed(l.builtinFS) {
        if !seen[s.Name] {
            skills = append(skills, s)
        }
    }

    return skills, nil
}
```

### 3.2 技能市场（远程注册表）

```go
// skills/registry.go

type SkillRegistry interface {
    Search(ctx context.Context, query string) ([]SkillSearchResult, error)
    GetMeta(ctx context.Context, name string) (*SkillMeta, error)
    Install(ctx context.Context, name, destDir string) error
}

type RegistryManager struct {
    registries []SkillRegistry
    cache      *searchCache
}

// 并发扇出搜索所有注册中心
func (rm *RegistryManager) SearchAll(ctx context.Context, query string) ([]SkillSearchResult, error) {
    // 检查缓存
    if cached := rm.cache.Get(query); cached != nil {
        return cached, nil
    }

    var wg sync.WaitGroup
    resultCh := make(chan []SkillSearchResult, len(rm.registries))

    for _, reg := range rm.registries {
        wg.Add(1)
        go func(r SkillRegistry) {
            defer wg.Done()
            results, _ := r.Search(ctx, query)
            resultCh <- results
        }(reg)
    }

    wg.Wait()
    close(resultCh)

    // 合并 + 排序
    var all []SkillSearchResult
    for results := range resultCh {
        all = append(all, results...)
    }
    sort.Slice(all, func(i, j int) bool { return all[i].Score > all[j].Score })

    rm.cache.Set(query, all)
    return all, nil
}
```

### 3.3 内置技能

嵌入到二进制中的默认技能：

```
workspace/skills/
├── weather/
│   └── SKILL.md        # 天气查询（wttr.in + Open-Meteo，无需 Key）
├── github/
│   └── SKILL.md        # GitHub 操作（依赖 gh CLI）
├── skill-creator/
│   └── SKILL.md        # 技能创建指南
├── summarize/
│   └── SKILL.md        # URL/文件/视频摘要
└── gocode/
    └── SKILL.md        # [SimpleClaw 独有] Go 代码编写最佳实践
```

---

## 4. Session 管理统一

### 4.1 现状问题

当前存在两套 Session：
1. `agent/types.go` 中的轻量 `Session`（纯内存，50 条上限）
2. `session/manager.go` 中的 `SessionManager`（JSONL 持久化）

两者未集成，gateway 中使用的是方案 1，session 包实际未被使用。

### 4.2 统一方案

将 `session/manager.go` 作为唯一的 Session 管理方案，替换 agent 内部的轻量 Session：

```go
// agent/loop.go 中改用 session.Manager

func (l *AgentLoop) processMessage(ctx context.Context, msg InboundMessage) {
    route := l.router.Resolve(msg.Channel, msg.SenderID, msg.ChatID)
    agent, _ := l.registry.Get(route.AgentID)

    // 使用统一的 SessionManager
    sess, err := agent.sessionManager.GetOrCreate(route.SessionKey)
    if err != nil {
        sess = agent.sessionManager.Create(route.SessionKey)
    }

    // 获取历史消息（最近 50 条）
    history := sess.GetHistory(50)

    // 构建上下文
    messages := agent.contextBuilder.BuildMessages(history, msg)

    // ... Agent 循环 ...

    // 保存到持久化 Session
    sess.AddMessage(session.Message{Role: "user", Content: msg.Content, Timestamp: time.Now()})
    sess.AddMessage(session.Message{Role: "assistant", Content: reply, Timestamp: time.Now()})
    agent.sessionManager.Save(sess)
}
```

### 4.3 Session 消息格式统一

```go
// session/session.go (统一格式)

type Message struct {
    Role       string    `json:"role"`
    Content    string    `json:"content"`
    ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
    ToolCallID string    `json:"tool_call_id,omitempty"`
    Timestamp  time.Time `json:"timestamp"`
}
```

---

## 5. Cron 表达式支持

### 5.1 当前状态

`cron/service.go` 已实现 `at`（一次性）和 `every`（周期）调度，cron 表达式为 TODO。

### 5.2 集成 gronx

```go
// go.mod 新增
// github.com/adhocore/gronx v1.x

// cron/service.go (增强)

import "github.com/adhocore/gronx"

func (s *CronService) scheduleCronJob(job *CronJob) {
    g := gronx.New()

    go func() {
        for {
            if !job.Enabled {
                return
            }

            // 计算下次执行时间
            nextRun, err := g.NextTick(job.Schedule, false)
            if err != nil {
                slog.Error("invalid cron expression", "schedule", job.Schedule, "error", err)
                return
            }

            delay := time.Until(nextRun)
            timer := time.NewTimer(delay)

            select {
            case <-s.ctx.Done():
                timer.Stop()
                return
            case <-timer.C:
                s.executeJob(job)
            }
        }
    }()
}
```

---

## 6. 语音转录增强

### 6.1 当前状态

`providers/transcription.go` 已实现 Groq Whisper API 调用。

### 6.2 集成到 Channel

```go
// voice/transcriber.go (独立包，便于 Channel 层调用)

type Transcriber struct {
    apiKey  string
    client  *http.Client
    baseURL string
}

func (t *Transcriber) IsAvailable() bool {
    return t.apiKey != ""
}

func (t *Transcriber) Transcribe(ctx context.Context, audioPath string) (string, error) {
    // multipart upload to Groq Whisper API
    // 返回转录文本
}
```

在 Telegram Channel 中使用：

```go
// channels/telegram.go

func (t *TelegramChannel) handleVoice(msg TelegramMessage) {
    if t.transcriber == nil || !t.transcriber.IsAvailable() {
        t.HandleMessage(InboundMessage{Content: "[voice message - transcription unavailable]"})
        return
    }

    // 下载语音文件
    localPath := t.downloadFile(msg.Voice.FileID)
    defer os.Remove(localPath)

    // 转录
    text, err := t.transcriber.Transcribe(context.Background(), localPath)
    if err != nil {
        text = "[voice transcription failed]"
    }

    t.HandleMessage(InboundMessage{
        Content: text,
        Media:   []MediaItem{{Type: "audio", URL: localPath}},
    })
}
```

---

## 7. 实施计划

| 阶段 | 模块 | 工作量 | 优先级 |
|------|------|--------|--------|
| Phase 1 | Bus 事件模型增强（Media/Metadata） | 0.5天 | P0 |
| Phase 2 | Telegram Channel 完整实现 | 3天 | P0 |
| Phase 3 | Session 管理统一 | 1.5天 | P0 |
| Phase 4 | WhatsApp WebSocket Bridge | 2天 | P1 |
| Phase 5 | Cron 表达式支持 | 1天 | P1 |
| Phase 6 | Web 搜索增强（DuckDuckGo） | 0.5天 | P1 |
| Phase 7 | Cron/Skills 工具 | 1天 | P2 |
| Phase 8 | 三级技能加载 | 1.5天 | P2 |
| Phase 9 | 技能市场（远程注册表） | 2天 | P3 |
| Phase 10 | 语音转录集成 | 1天 | P2 |
| Phase 11 | Discord/DingTalk/Feishu Channel | 3天 | P3 |

**总计**：~17 天
