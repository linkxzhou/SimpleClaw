# Plan 0 — 现有代码优化与简化

> **定位**: 前置步骤，在引入新功能（plan1-plan10）之前，先清理和优化当前代码库，降低后续迭代的认知负担和改动成本。

---

## 一、现有代码架构总览

### 模块清单

| 包 | 核心文件 | 行数 | 职责 |
|---|---------|------|------|
| `cmd/` | main, gateway, agent, cron, adapter, onboard, status | ~910 | CLI 入口 + 适配器层 |
| `agent/` | agent, context, skills, subagent, types | ~1300 | Agent 核心循环 + 上下文构建 |
| `agent/tools/` | registry, fs, shell, web, gocode, message, spawn | ~1370 | 工具实现 |
| `bus/` | queue, events, errors | ~193 | 消息总线 |
| `channels/` | base, manager, telegram, whatsapp | ~744 | 聊天渠道 |
| `config/` | loader, schema | ~426 | 配置加载与 schema |
| `cron/` | service, types | ~514 | 定时任务 |
| `health/` | heartbeat, server | ~299 | 心跳 + HTTP 健康检查 |
| `memory/` | manager, session, state, store | ~550 | 会话/状态/记忆持久化 |
| `providers/` | base, openai, fallback, cooldown, errors, factory, transcription | ~765 | LLM Provider 抽象层 |
| `routing/` | route | ~93 | 多 Agent 路由 |
| `utils/` | helpers, log | ~498 | 通用工具 + 日志 |
| `goscript/` | *(解释器，独立子系统)* | ~大量 | Go SSA 解释器 |

**总计（不含 goscript/thirdparty/tests）**: 约 **7,600 行**

---

## 二、问题发现与优化项

### O1. adapter.go 重复类型转换链（高优先级）

**问题**: `cmd/adapter.go` 用 277 行实现了 BusAdapter、ProviderAdapter、FallbackProviderAdapter、SessionStoreAdapter 四个适配器，大量代码是机械性的 `map[string]interface{}` ↔ 强类型转换。

**根因**: `agent` 包通过接口定义 `MessageBus`/`LLMProvider`，而 `bus`/`providers` 包各自有独立类型。两套类型除了包名不同，结构完全一致。

**当前代码**:
- `agent/types.go` 定义了 `InboundMessage`/`OutboundMessage` 为 `bus` 包的 type alias（已优化）
- 但 `LLMProvider` 接口使用 `agent.Message`/`agent.ToolDef`，而 `providers` 使用 `providers.ChatRequest`，二者无法直接兼容
- `buildChatRequest()` 将强类型 → `map[string]interface{}`，`convertLLMResponse()` 再反向转换

**优化方案**:
1. 将 `providers.ChatRequest` 改为直接接受强类型 messages/tools，消除 `map[string]interface{}` 中间层
2. 统一 `agent.ToolCall` 与 `providers.ToolCallRequest` 为同一类型（后者移到共享 types 包）
3. adapter.go 从 277 行缩减到 ~80 行（仅保留真正需要的胶水代码）

**涉及文件**:
- `cmd/adapter.go` — 重构适配逻辑
- `providers/base.go` — `ChatRequest` 改为强类型
- `agent/types.go` — 提取共享类型

**预期收益**: 减少 ~200 行适配代码，消除运行时 map 序列化开销

---

### O2. agent 与 subagent 重复 Agent Loop（高优先级）

**问题**: `agent/agent.go:298-347` 和 `agent/subagent.go:113-148` 有两套几乎相同的 Agent 循环逻辑（LLM 调用 → 工具执行 → 消息追加），仅在迭代次数和日志级别上有差异。

**当前代码**:
```go
// agent.go: agentLoop — 20 次迭代
for i := 0; i < a.maxIterations; i++ {
    response, err := a.provider.Chat(ctx, messages, toolDefs, a.model)
    // ... 工具执行 ...
}

// subagent.go: runSubagent — 15 次迭代（内联复制）
for i := 0; i < maxIterations; i++ {
    response, err := m.provider.Chat(ctx, messages, toolDefs, m.model)
    // ... 工具执行（几乎相同）...
}
```

**优化方案**:
1. 提取公共 `RunLoop(ctx, provider, messages, toolDefs, model, registry, maxIter) (string, error)` 函数
2. `agent.go` 和 `subagent.go` 都调用此公共函数
3. 循环内的日志通过 `slog.Logger` 实例差异化（主 Agent 用 Info，子 Agent 用 Debug）

**涉及文件**:
- `agent/loop.go`（新文件） — 提取公共循环
- `agent/agent.go` — 调用公共循环
- `agent/subagent.go` — 调用公共循环

**预期收益**: 消除 ~50 行重复代码，后续 plan2（SubTurn）和 plan8（上下文截断）只需改一处

---

### O3. 工具注册重复（中优先级）

**问题**: `agent/agent.go:88-120` 和 `agent/subagent.go:87-98` 分别注册工具集，子 Agent 的注册是主 Agent 的子集，但以硬编码方式重复。

**当前代码**:
```go
// agent.go: registerDefaultTools — 12 行
a.tools.Register(&tools.ReadFileTool{})
a.tools.Register(&tools.WriteFileTool{})
// ...

// subagent.go: runSubagent — 9 行（手动重复子集）
registry.Register(&tools.ReadFileTool{})
registry.Register(&tools.WriteFileTool{})
// ...
```

**优化方案**:
1. 在 `agent/tools/` 中增加 `RegisterCommonTools(registry, workspace, braveAPIKey)` 公共函数
2. 增加 `RegisterAgentOnlyTools(registry, bus, subagents)` 仅注册主 Agent 特有的工具（message, spawn）
3. 子 Agent 只调用 `RegisterCommonTools`

**涉及文件**:
- `agent/tools/registry.go` — 添加批量注册函数
- `agent/agent.go` — 调用公共注册
- `agent/subagent.go` — 调用公共注册

**预期收益**: 消除重复，新增工具时只改一处

---

### O4. skills.go 重复加载与解析（中优先级）

**问题**: `agent/skills.go` 中 `GetAlwaysSkills()` 对每个 skill 调用了 `GetSkillMetadata()` → `LoadSkill()` → `os.ReadFile()` → 正则解析，同时又调用 `getSkillMeta()` → `GetSkillMetadata()` → 再次读文件。单个 skill 最多被读取 3 次。

**当前代码**:
```go
func (s *SkillsLoader) GetAlwaysSkills() []string {
    for _, skill := range s.ListSkills(true) {   // 第1次: listSkills → getSkillMeta → GetSkillMetadata → LoadSkill(ReadFile)
        meta := s.GetSkillMetadata(skill.Name)     // 第2次: ReadFile + 正则解析
        skillMeta := s.getSkillMeta(skill.Name)    // 第3次: GetSkillMetadata → ReadFile + 正则解析
    }
}
```

同时 `GetSkillMetadata()` 使用手写 YAML 解析器（逐行 split + indexOf ":"），对多行值和嵌套结构无法正确处理。

**优化方案**:
1. 引入 skill metadata 缓存 `map[string]*SkillMetadata`，首次加载后缓存
2. 合并 `GetSkillMetadata` 和 `getSkillMeta` 为一个方法，统一返回 `SkillFullMeta`（含 name, description, always, requires）
3. `ListSkills` 返回值中附带已解析的 metadata，避免下游二次加载
4. 简易 YAML 解析保持不变（足够当前场景），但修复多值 bins/env 的解析问题

**涉及文件**:
- `agent/skills.go` — 重构缓存和合并方法

**预期收益**: 启动时 IO 次数从 O(3n) 降到 O(n)，代码减少 ~40 行

---

### O5. config.GetAPIKeys/GetEndpoints 重复遍历（低优先级）

**问题**: `config/schema.go` 中 `GetAPIKeys()` 和 `GetEndpoints()` 都以硬编码方式逐个检查 8 个 Provider 字段。每新增一个 Provider 需要改 3 处（结构体定义、GetAPIKeys、GetEndpoints）。

**当前代码**:
```go
func (c *Config) GetAPIKeys() map[string]string {
    if c.Providers.OpenRouter.APIKey != "" { keys["openrouter"] = ... }
    if c.Providers.Anthropic.APIKey != ""  { keys["anthropic"] = ... }
    // ... 重复 8 次
}
```

**优化方案**:
1. 将 `ProvidersConfig` 改为 `map[string]ProviderConfig`（或保留结构体但增加 `All() map[string]ProviderConfig` 方法）
2. `GetAPIKeys` / `GetEndpoints` 改为遍历 map，消除硬编码

**涉及文件**:
- `config/schema.go` — 重构 Provider 遍历

**预期收益**: 新增 Provider 只需改 1 处，减少 ~50 行重复代码

---

### O6. Session 存储 map[string]interface{} 类型不安全（中优先级）

**问题**: `memory/session.go` 中 `Session.Messages` 类型为 `[]map[string]interface{}`，在 `AddMessage` / `GetHistory` 中需要手动字符串 key 取值、类型断言。`cmd/adapter.go` 的 `SessionStoreAdapter` 还需要在 `map[string]interface{}` 和 `agent.Message` 之间来回转换。

**当前代码**:
```go
// session.go
type Session struct {
    Messages []map[string]interface{}  // 弱类型
}

// adapter.go
func (a *SessionStoreAdapter) GetOrCreateSession(key string) []agent.Message {
    for _, m := range history {
        role, _ := m["role"].(string)      // 手动断言
        content, _ := m["content"].(string) // 手动断言
    }
}
```

**优化方案**:
1. 定义 `memory.ChatMessage` 强类型结构体（role, content, timestamp, extra）
2. `Session.Messages` 改为 `[]ChatMessage`
3. JSONL 序列化/反序列化直接使用 `json.Marshal/Unmarshal`
4. adapter 层无需再做 `map → struct` 转换

**涉及文件**:
- `memory/session.go` — 引入强类型
- `memory/manager.go` — 适配新类型
- `cmd/adapter.go` — 简化 SessionStoreAdapter

**预期收益**: 消除运行时类型断言，IDE 补全友好，减少 ~30 行适配代码

---

### O7. utils/log.go 过度封装（低优先级）

**问题**: `utils/log.go` 定义了 308 行的自定义 `Logger` 接口 + `slogLogger` 实现 + 12 个全局函数（LogDebug/LogDebugf/LogInfo/...）。但实际项目中几乎所有模块直接使用 `slog.Info/slog.Error` 等标准库调用，自定义 Logger 接口仅在 `utils.InitLog()` 初始化时使用。

**现状**:
- `agent/`, `bus/`, `channels/`, `cron/`, `health/` 全部使用 `slog.Info/slog.Error`
- `utils.LogInfo` 等包级函数在业务代码中 **未被使用**
- `Logger` 接口（Debug/Debugf/Info/Infof/...）在业务代码中 **未被使用**

**优化方案**:
1. 删除 `Logger` 接口和全部 `LogXxx` 包级函数
2. 保留 `InitLog(cfg)` 和 `slogLogger` 内部实现（负责初始化 slog.Default）
3. 业务代码继续使用 `slog.Info/slog.Error`

**涉及文件**:
- `utils/log.go` — 精简到 ~80 行
- `utils/log_test.go` — 适配测试

**预期收益**: 减少 ~220 行未使用代码，消除维护负担

---

### O8. channels/manager 双重出站分发（低优先级）

**问题**: `bus/queue.go` 提供了 `SubscribeOutbound` + `DispatchOutbound` 订阅者模式，但 `channels/manager.go` 并未使用此机制，而是自己实现了 `dispatchOutbound` 方法直接消费 `ConsumeOutbound`。这导致 bus 的订阅者机制成为死代码。

**当前代码**:
```go
// bus/queue.go — 订阅者模式（未使用）
func (b *MessageBus) SubscribeOutbound(channel string, handler OutboundHandler) { ... }
func (b *MessageBus) DispatchOutbound(ctx context.Context) { ... }

// channels/manager.go — 自行消费（实际使用）
func (m *Manager) dispatchOutbound(ctx context.Context) {
    msg, err := m.bus.ConsumeOutbound(ctx)
    ch := m.channels[msg.Channel]
    ch.Send(msg)
}
```

**优化方案**:
两个方向任选其一：
- **A**: 删除 bus 的 `SubscribeOutbound` + `DispatchOutbound`，仅保留 channel 消费方式（简化）
- **B**: channels/manager 改为使用 bus 的订阅者模式（为 plan9 插件管道做铺垫）

**推荐方向 B**（考虑后续 plan9），但如果不实施 plan9 则选 A。

**涉及文件**:
- `bus/queue.go` — 根据方向决定
- `channels/manager.go` — 适配

**预期收益**: 消除二义性，后续开发不会困惑该用哪种分发方式

---

### O9. routing 包未接入主流程（低优先级）

**问题**: `routing/route.go` 实现了完整的 7 级优先级路由解析，但 `cmd/gateway.go` 和 `agent/agent.go` 的主流程中 **完全没有使用** 该路由。所有消息都直接发给唯一的 Agent 实例。`config/schema.go` 定义了 `BindingConfig` 但 gateway 启动时未读取。

**优化方案**:
1. 如果短期不需要多 Agent 路由，将 `routing/` 包标记为 experimental 或移到 `internal/` 下
2. 在 `gateway.go` 中为单 Agent 模式添加注释说明为何未使用 routing
3. 后续 plan3（多 Agent 团队协作）时正式接入

**涉及文件**:
- `cmd/gateway.go` — 添加 TODO 注释
- `routing/route.go` — 保持不变

**预期收益**: 代码意图明确，不会误导维护者

---

### O10. 错误处理一致性（中优先级）

**问题**: 项目中错误处理风格不一致：
- 部分函数返回格式化字符串（`"Error: xxx"`）而非 error 类型（如 `tools/fs.go` 的 `ReadFileTool.Execute`）
- 部分函数静默吞掉错误（如 `state.go` 的 `save()` 中 `_ = os.Rename`）
- `cron/service.go` 中 `saveStore()` 的错误只打日志不上报

**具体案例**:
```go
// tools/fs.go — 业务错误返回 (string, nil) 而非 ("", error)
if os.IsNotExist(err) {
    return fmt.Sprintf("Error: File not found: %s", path), nil  // 调用方无法区分
}

// memory/state.go — 静默忽略
_ = os.Rename(tmpPath, m.filePath)  // 重命名失败 → 数据丢失
```

**优化方案**:
1. 工具的 `Execute` 方法保持 `(string, nil)` 模式用于"预期内的业务失败"（文件不存在等），真正的系统错误用 `("", error)` — 当前模式合理，但需**统一**
2. `state.go` 的 `save()` 添加错误日志而非静默忽略
3. `cron/service.go` 的 `saveStore()` 错误传播给调用方

**涉及文件**:
- `memory/state.go` — 添加错误日志
- `cron/service.go` — 评估错误传播

**预期收益**: 减少隐蔽 bug，排障效率提升

---

## 三、优先级排序与实施计划

### 第一批（高优先级，建议在 plan1-10 之前完成）

| 编号 | 优化项 | 改动量 | 风险 | 收益 |
|------|--------|--------|------|------|
| **O2** | 提取公共 Agent Loop | 中 | 低 | 消除重复，plan2/plan8 的基础 |
| **O1** | 统一类型，简化 adapter | 大 | 中 | 减少 200 行，消除 map 开销 |
| **O3** | 工具注册统一 | 小 | 低 | 新增工具只改 1 处 |

### 第二批（中优先级，可与 plan1-10 穿插）

| 编号 | 优化项 | 改动量 | 风险 | 收益 |
|------|--------|--------|------|------|
| **O4** | Skills 加载缓存 | 中 | 低 | 启动 IO 降 3 倍 |
| **O6** | Session 强类型化 | 中 | 中 | 类型安全 + 减少适配 |
| **O10** | 错误处理一致性 | 小 | 低 | 减少隐蔽 bug |

### 第三批（低优先级，空闲时清理）

| 编号 | 优化项 | 改动量 | 风险 | 收益 |
|------|--------|--------|------|------|
| **O7** | 精简 utils/log.go | 小 | 低 | 减少 220 行死代码 |
| **O8** | 出站分发统一 | 小 | 低 | 消除二义性 |
| **O5** | Config Provider 遍历 | 中 | 中 | 扩展性提升 |
| **O9** | routing 标记 experimental | 小 | 无 | 代码意图明确 |

---

## 四、技术方案细节

### O2 详细设计 — 提取公共 Agent Loop

```
新文件: agent/loop.go

// LoopConfig 配置 Agent Loop 的行为
type LoopConfig struct {
    Provider      LLMProvider
    Model         string
    Registry      *tools.Registry
    MaxIterations int
    Logger        *slog.Logger      // 日志实例（主 Agent 用 slog.Default(), 子 Agent 用 debug logger）
    OnToolCall    func(iteration int, tc ToolCall)       // 可选钩子（用于 plan4 成本追踪）
    OnIteration   func(iteration int, response *LLMResponse) // 可选钩子
}

// RunLoop 执行 ReAct 循环，返回最终文本响应
func RunLoop(ctx context.Context, cfg LoopConfig, messages []Message) (string, error) {
    toolDefs := convertToolDefs(cfg.Registry.GetDefinitions())
    for i := 0; i < cfg.MaxIterations; i++ {
        response, err := cfg.Provider.Chat(ctx, messages, toolDefs, cfg.Model)
        if err != nil { return "", err }

        if cfg.OnIteration != nil { cfg.OnIteration(i, response) }

        if !response.HasToolCalls {
            return response.Content, nil
        }

        // 添加 assistant + 执行工具 + 添加 tool results
        messages = appendToolMessages(messages, response, cfg.Registry, ctx)

        if cfg.OnToolCall != nil {
            for _, tc := range response.ToolCalls { cfg.OnToolCall(i, tc) }
        }
    }
    return "I've reached the maximum number of iterations.", nil
}
```

### O1 详细设计 — 统一类型层

```
方案: 引入 types/ 共享包（或在 agent/types.go 中定义，providers 通过接口引用）

步骤:
1. providers.ChatRequest.Messages 从 []map[string]interface{} 改为 []Message 强类型
2. providers.ChatRequest.Tools 从 []map[string]interface{} 改为 []ToolDef 强类型
3. providers.OpenAIProvider.Chat() 内部在发送 HTTP 前才做 JSON 序列化
4. 删除 adapter.go 中的 buildChatRequest() 和 convertLLMResponse()
5. ProviderAdapter / FallbackProviderAdapter 直接透传类型
```

---

## 五、验收标准

1. `go build ./...` 编译通过
2. `go test ./...` 所有既有测试通过
3. 优化后总代码量减少 ≥ 400 行（不含注释/空行）
4. 无新增外部依赖
5. gateway / agent / cron 子命令功能不变
6. 每个优化项独立提交，commit message 标注 `[plan0-Ox]`

---

## 六、工期估算

| 批次 | 内容 | 预估工期 |
|------|------|---------|
| 第一批 | O1 + O2 + O3 | 3 天 |
| 第二批 | O4 + O6 + O10 | 2 天 |
| 第三批 | O5 + O7 + O8 + O9 | 1 天 |
| **合计** | | **~1 周** |

---

## 七、风险与缓解

| 风险 | 影响 | 缓解 |
|------|------|------|
| O1 改动范围大，可能引入回归 | 中 | 每步改完跑全量测试，分 3 个 commit |
| O6 Session 格式变更导致已有 .jsonl 文件不兼容 | 低 | 新格式兼容旧格式读取（fallback 解析） |
| O5 Config Provider 改为 map 后 JSON schema 变化 | 中 | 提供 migration 脚本或保持向后兼容 |

---

## 八、与后续 Plan 的关系

| 优化项 | 直接受益的后续 Plan |
|--------|-------------------|
| O2 公共 Agent Loop | plan2（SubTurn）、plan4（成本追踪钩子）、plan8（上下文截断） |
| O1 统一类型 | plan4（LLM 响应中的 Usage 直传）、plan3（多 Agent 共享类型） |
| O3 工具注册统一 | plan9（插件系统新增工具）、plan6（审批网关工具） |
| O4 Skills 缓存 | plan7（SOP 工作流大量加载 skill） |
| O8 出站分发统一 | plan9（插件管道中间件链） |
