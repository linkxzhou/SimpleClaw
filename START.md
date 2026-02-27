# SimpleClaw 快速开始

## 环境要求

- Go 1.21+

## 编译

```bash
go build -o simpleclaw ./cmd/
```

## 首次初始化

```bash
simpleclaw onboard
```

执行后会：
1. 创建配置文件 `~/.simpleclaw/config.json`（全局）
2. 创建工作区 `~/.simpleclaw/workspace/`
3. 生成模板文件（`AGENTS.md`、`SOUL.md`、`USER.md`、`HEARTBEAT.md`、`memory/MEMORY.md`）

> 也可在项目目录下创建 `.simpleclaw/config.json` 覆盖全局配置（本地优先）。

初始化完成后，在配置文件中填入你的 API Key：

```bash
# 推荐 OpenRouter（聚合多家 LLM）
# 获取 Key: https://openrouter.ai/keys
```

## 配置文件

配置文件按优先级查找：
1. **本地目录** `.simpleclaw/config.json`（当前工作目录下，适用于项目级配置）
2. **用户目录** `~/.simpleclaw/config.json`（全局配置，兜底）

配置文件格式：

```json
{
  "agents": {
    "defaults": {
      "workspace": "~/.simpleclaw/workspace",
      "model": "anthropic/claude-opus-4-5",
      "maxTokens": 8192,
      "temperature": 0.7,
      "maxToolIterations": 20,
      "apiBase": "",
      "apiKey": ""
    }
  },
  "channels": {
    "telegram": { "enabled": false, "token": "", "allowFrom": [] },
    "whatsapp": { "enabled": false, "bridgeUrl": "ws://localhost:3001" }
  },
  "providers": {
    "anthropic": { "apiKey": "" },
    "openai": { "apiKey": "" },
    "openrouter": { "apiKey": "" }
  },
  "gateway": { "host": "0.0.0.0", "port": 18790 },
  "tools": { "web": { "search": { "apiKey": "", "maxResults": 5 } } }
}
```

API Key 优先级：OpenRouter > Anthropic > OpenAI > Gemini > Zhipu > Groq > VLLM

## 启动

### `simpleclaw agent` — 与 Agent 对话

**单消息模式：**

```bash
simpleclaw agent -m "你是什么模型？"
simpleclaw agent -m "今天天气怎么样？"
simpleclaw agent -m "帮我写一个快速排序" -s "cli:coding"
simpleclaw agent -m "分析当前磁盘大小"
```

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-m, --message` | 发送的消息内容 | 无（省略则进入交互模式） |
| `-s, --session` | Session ID | `cli:default` |

**交互模式（REPL）：**

```bash
simpleclaw agent
```

```
🤖 Interactive mode (type 'exit' or Ctrl+C to quit)

You: 你好，你能做什么？

🤖 我是 SimpleClaw，一个 AI Agent。我可以：
  - 读写文件
  - 执行 Shell 命令
  - 搜索网页
  - 动态生成并执行 Go 代码
  ...

You: 帮我用 Go 写一个斐波那契函数并运行
...
You: exit
Goodbye!
```

### `simpleclaw gateway` — 启动完整网关

启动全部服务：Agent + Channels + Cron + Heartbeat + Health。

```bash
simpleclaw gateway              # 正常启动
simpleclaw gateway -v           # 详细日志模式 (debug level)
```

| 参数 | 说明 |
|------|------|
| `-v, --verbose` | 启用 debug 级别日志 |

启动后：
- Agent 监听 MessageBus，处理所有入站消息
- Channel Manager 启动已配置的渠道（Telegram/WhatsApp）
- Cron Service 加载并执行定时任务
- Heartbeat 每 30 分钟检查 `HEARTBEAT.md`
- Health Server 提供 HTTP 健康检查端点
- `Ctrl+C` 优雅关闭所有服务

### `simpleclaw cron` — 管理定时任务

**列出任务：**

```bash
simpleclaw cron list             # 列出已启用的任务
simpleclaw cron list --all       # 包含已禁用的任务
```

**添加任务：**

```bash
# 每小时执行
simpleclaw cron add -n "hourly check" -m "检查系统状态" --every 3600

# Cron 表达式（每天早上 9 点）
simpleclaw cron add -n "morning" -m "早上好，今天有什么安排？" --cron "0 9 * * *"

# 一次性定时
simpleclaw cron add -n "reminder" -m "开会提醒" --at "2026-02-08T14:00:00"

# 执行结果投递到 WhatsApp
simpleclaw cron add -n "report" -m "生成日报" --every 86400 -d --to "user123" --channel whatsapp
```

| 参数 | 说明 |
|------|------|
| `-n, --name` | 任务名称（必填） |
| `-m, --message` | Agent 执行的消息（必填） |
| `--every <秒>` | 固定间隔（秒） |
| `--cron <表达式>` | Cron 表达式 |
| `--at <时间>` | 一次性定时（RFC3339 或 `2006-01-02T15:04:05`） |
| `-d, --deliver` | 投递结果到渠道 |
| `--to <接收者>` | 投递目标 |
| `--channel <渠道>` | 投递渠道（telegram/whatsapp） |

**管理任务：**

```bash
simpleclaw cron remove <job_id>           # 删除任务
simpleclaw cron enable <job_id>           # 启用任务
simpleclaw cron enable <job_id> --disable # 禁用任务
simpleclaw cron run <job_id>              # 手动执行
simpleclaw cron run <job_id> --force      # 强制执行（即使已禁用）
```

### `simpleclaw status` — 查看系统状态

```bash
simpleclaw status
```

### `simpleclaw channels` — 渠道管理

```bash
simpleclaw channels status     # 查看渠道状态
```

### 其他

```bash
simpleclaw --version           # 显示版本号
simpleclaw --help              # 显示帮助
```

## Workspace 约定

```text
~/.simpleclaw/
├── config.json             # 全局配置
├── sessions/               # 会话持久化 (JSONL)
└── workspace/              # Agent 工作区
    ├── AGENTS.md            # Agent 行为规则
    ├── SOUL.md              # 人格定义
    ├── IDENTITY.md          # 身份信息
    ├── TOOLS.md             # 工具使用指南
    ├── USER.md              # 用户偏好
    ├── HEARTBEAT.md         # 心跳任务清单
    ├── memory/              # 记忆存储
    │   ├── long_term.md     # 长期记忆
    │   └── 2026-02-08.md    # 每日笔记
    └── skills/              # 用户自定义技能
        └── my-skill/
            └── SKILL.md
```

## Gateway HTTP API

Gateway 启动后默认监听 `0.0.0.0:18790`（可通过 `config.json` 的 `gateway.host` / `gateway.port` 修改）。前端通过以下 HTTP 端点与后端通信。

### 基础端点

#### `GET /health` — 存活探针

始终返回 200，用于 Kubernetes/Docker 存活检测。

**响应：**

```json
{
  "status": "ok",
  "uptime": "2h30m15s"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `status` | string | 固定 `"ok"` |
| `uptime` | string | 服务运行时长（Go Duration 格式） |

#### `GET /ready` — 就绪探针

所有服务启动完成后返回 200，否则返回 503。

**响应（就绪）：** `200 OK`

```json
{ "status": "ready", "uptime": "2h30m15s" }
```

**响应（未就绪）：** `503 Service Unavailable`

```json
{ "status": "not_ready", "uptime": "0s" }
```

#### `GET /api/health` — 前端健康检查

前端每 10 秒轮询此端点检测网关状态。

**响应：**

```json
{
  "ok": true,
  "uptime": 3600,
  "version": "0.1.0",
  "error": ""
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `ok` | boolean | 网关是否正常运行 |
| `uptime` | number | 运行秒数 |
| `version` | string | 版本号（可选） |
| `error` | string | 错误信息（可选） |

---

### RPC 统一调用

所有业务 API 通过统一 RPC 端点调用：

```
POST /api/rpc
Content-Type: application/json
```

**请求格式：**

```json
{
  "method": "模块.方法名",
  "params": { ... }
}
```

**响应格式：**

```json
{
  "success": true,
  "result": { ... }
}
```

**错误响应：**

```json
{
  "success": false,
  "error": "错误描述"
}
```

---

### SSE 事件流

#### `GET /api/events?sessionKey={sessionKey}` — 实时消息推送

建立 Server-Sent Events 连接，接收 Agent 的流式响应。

| 参数 | 类型 | 说明 |
|------|------|------|
| `sessionKey` | string (query) | 会话标识，需 URL 编码 |

**每条 SSE `data` 的 JSON 格式：**

```json
{
  "runId": "run_abc123",
  "state": "delta",
  "message": { "role": "assistant", "content": "..." },
  "errorMessage": ""
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `runId` | string | 当前运行标识 |
| `state` | string | `delta`（增量）/ `final`（完成）/ `error`（错误）/ `aborted`（已中止） |
| `message` | object | 消息内容 |
| `errorMessage` | string | 错误时的说明 |

`message.content` 可以是字符串或 ContentBlock 数组：

```json
[
  { "type": "text", "text": "回复文本" },
  { "type": "thinking", "thinking": "思考过程..." },
  { "type": "tool_use", "name": "read_file", "input": { "path": "/tmp/a.txt" } }
]
```

---

### Chat RPC 方法

#### `chat.send` — 发送消息

向指定会话发送一条消息，触发 Agent 执行。

**参数：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `sessionKey` | string | 是 | 会话标识 |
| `message` | string | 是 | 消息内容 |
| `deliver` | boolean | 否 | 是否投递到渠道，默认 `false` |
| `media` | array | 否 | 附件列表 |
| `idempotencyKey` | string | 是 | 幂等键（UUID），防止重复提交 |

`media` 数组元素：

| 字段 | 类型 | 说明 |
|------|------|------|
| `filePath` | string | 文件路径 |
| `mimeType` | string | MIME 类型 |
| `fileName` | string | 文件名 |

**响应：**

```json
{ "runId": "run_abc123" }
```

#### `chat.history` — 获取聊天历史

**参数：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `sessionKey` | string | 是 | 会话标识 |
| `limit` | number | 否 | 返回条数，默认 200 |

**响应：**

```json
{
  "messages": [ ... ],
  "thinkingLevel": "normal"
}
```

#### `chat.abort` — 中止当前对话

**参数：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `sessionKey` | string | 是 | 会话标识 |

**响应：** 无（成功即可）

---

### Sessions RPC 方法

#### `sessions.list` — 列出会话

**参数：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `limit` | number | 否 | 返回条数，默认 50 |

**响应：**

```json
{
  "sessions": [
    {
      "key": "cli:default",
      "label": "Default Session",
      "displayName": "默认会话",
      "thinkingLevel": "normal",
      "model": "anthropic/claude-opus-4-5"
    }
  ]
}
```

---

### Provider RPC 方法

#### `provider.list` — 列出所有 Provider

**参数：** 无

**响应：**

```json
{
  "providers": [
    {
      "id": "prov_abc",
      "name": "OpenRouter",
      "type": "openrouter",
      "enabled": true,
      "hasKey": true,
      "keyMasked": "sk-or-...****"
    }
  ]
}
```

#### `provider.getDefault` — 获取默认 Provider

**参数：** 无

**响应：**

```json
{ "id": "prov_abc" }
```

#### `provider.save` — 新增或更新 Provider

**参数：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | 否 | 更新时提供，新增时为空 |
| `name` | string | 是 | 显示名称 |
| `type` | string | 是 | 类型：`openai` / `anthropic` / `openrouter` / `gemini` / `zhipu` / `groq` / `vllm` / `ollama` / `custom` |
| `baseUrl` | string | 否 | 自定义 API 地址 |
| `model` | string | 否 | 模型 ID |
| `enabled` | boolean | 否 | 是否启用 |
| `apiKey` | string | 否 | API Key |

**响应：** 无（成功即可）

#### `provider.delete` — 删除 Provider

**参数：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | 是 | Provider ID |

#### `provider.setDefault` — 设置默认 Provider

**参数：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | 是 | Provider ID |

#### `provider.validateKey` — 验证 API Key

**参数：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | 是 | Provider ID |
| `apiKey` | string | 是 | 待验证的 Key |

**响应：**

```json
{ "valid": true }
```

---

### Channels RPC 方法

#### `channels.status` — 查询渠道状态

**参数：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `probe` | boolean | 否 | 是否主动探测连接状态，默认 `true` |

**响应：**

```json
{
  "channels": [
    {
      "id": "ch_001",
      "type": "telegram",
      "name": "My Telegram Bot",
      "status": "connected",
      "connected": true,
      "error": "",
      "accountId": "@my_bot",
      "lastActivity": "2026-02-25T10:30:00Z"
    }
  ]
}
```

#### `channels.add` — 添加渠道

**参数：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `type` | string | 是 | 渠道类型：`telegram` / `discord` / `whatsapp` / `feishu` |
| `name` | string | 是 | 显示名称 |
| `config` | object | 是 | 渠道配置（如 `token`、`bridgeUrl` 等） |

#### `channels.delete` — 删除渠道

**参数：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | 是 | 渠道 ID |

#### `channels.connect` — 连接渠道

**参数：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | 是 | 渠道 ID |

#### `channels.disconnect` — 断开渠道

**参数：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | 是 | 渠道 ID |

---

### Cron RPC 方法

#### `cron.list` — 列出定时任务

**参数：** 无

**响应：**

```json
{
  "jobs": [
    {
      "id": "job_001",
      "name": "早间问候",
      "message": "早上好，今天有什么安排？",
      "schedule": "0 9 * * *",
      "target": {
        "channelType": "telegram",
        "channelId": "ch_001",
        "channelName": "My Telegram"
      },
      "enabled": true,
      "createdAt": "2026-02-20T08:00:00Z",
      "updatedAt": "2026-02-25T10:00:00Z",
      "lastRun": {
        "time": "2026-02-25T09:00:00Z",
        "success": true
      },
      "nextRun": "2026-02-26T09:00:00Z"
    }
  ]
}
```

#### `cron.create` — 创建定时任务

**参数：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 任务名称 |
| `message` | string | 是 | Agent 执行的消息 |
| `schedule` | string | 是 | Cron 表达式或调度描述 |
| `target` | object | 是 | 投递目标 |
| `target.channelType` | string | 是 | 渠道类型 |
| `target.channelId` | string | 是 | 渠道 ID |
| `target.channelName` | string | 否 | 渠道名称 |
| `enabled` | boolean | 否 | 是否立即启用，默认 `true` |

#### `cron.update` — 更新定时任务

**参数：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | 是 | 任务 ID |
| `name` | string | 否 | 任务名称 |
| `message` | string | 否 | Agent 执行的消息 |
| `schedule` | string | 否 | Cron 表达式 |
| `target` | object | 否 | 投递目标 |
| `enabled` | boolean | 否 | 是否启用 |

#### `cron.delete` — 删除定时任务

**参数：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | 是 | 任务 ID |

#### `cron.toggle` — 启用/禁用任务

**参数：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | 是 | 任务 ID |
| `enabled` | boolean | 是 | `true` 启用 / `false` 禁用 |

#### `cron.trigger` — 手动触发任务

**参数：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | 是 | 任务 ID |

---

### Skills RPC 方法

#### `skills.status` — 列出已安装技能

**参数：** 无

**响应：**

```json
{
  "skills": [
    {
      "id": "skill_001",
      "slug": "web-search",
      "name": "Web Search",
      "description": "搜索网页获取实时信息",
      "enabled": true,
      "icon": "🔍",
      "version": "1.0.0",
      "author": "SimpleClaw",
      "isCore": true,
      "isBundled": false,
      "configurable": false,
      "config": {},
      "dependencies": []
    }
  ]
}
```

#### `skills.update` — 启用/禁用技能

**参数：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | 是 | 技能 ID |
| `enabled` | boolean | 是 | `true` 启用 / `false` 禁用 |

#### `skills.search` — 搜索市场技能

**参数：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `query` | string | 是 | 搜索关键词 |

**响应：**

```json
{
  "skills": [
    {
      "slug": "code-review",
      "name": "Code Review",
      "description": "自动代码审查",
      "version": "0.2.0",
      "author": "community",
      "downloads": 1200,
      "stars": 45
    }
  ]
}
```

#### `skills.install` — 安装市场技能

**参数：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `slug` | string | 是 | 技能标识 |

#### `skills.uninstall` — 卸载技能

**参数：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `slug` | string | 是 | 技能标识 |
