# SimpleClaw 重构方案 — 实施路线图

## 1. 总体策略

### 1.1 核心原则

1. **渐进式重构**：不破坏现有功能，逐步增强
2. **可编译通过**：每个 Phase 完成后代码都能编译运行
3. **测试覆盖**：关键模块配套单元测试
4. **保持极简**：新增依赖严格审查，能不加就不加
5. **GoScript 优先**：保留并增强核心差异化特性

### 1.2 依赖策略

| 依赖 | 理由 | 替代方案 |
|------|------|---------|
| `gorilla/websocket` | WhatsApp Bridge 必需 | 标准库无 WebSocket |
| `adhocore/gronx` | Cron 表达式解析，零依赖 | 自实现（不推荐） |
| Bot SDK (可选) | 各 Channel 按需引入 | 直接 HTTP API |

**拒绝引入清单**：
- 不引入 Cobra/Viper/CLI 框架（保持 os.Args）
- 不引入 ORM/数据库（保持文件存储）
- 不引入 OAuth 库（自实现 PKCE < 200 行）
- 不引入 Web 框架（标准库 net/http 足够）

---

## 2. 分阶段实施路线

### Phase 0：准备工作（1天）

**目标**：整理代码结构，统一编码规范

- [ ] 统一日志框架：全部使用 `log/slog`，移除冗余的 `log` 包中自定义实现的 Printf/Errorf 等
- [ ] 创建 `Makefile`（build / build-all / test / lint / install）
- [ ] 创建 `.env.example` 文件
- [ ] 创建 `workspace/` 模板目录（AGENT.md, IDENTITY.md, SOUL.md, USER.md, MEMORY.md）
- [ ] 更新 `cmd/onboard.go` 使用 `go:embed` 嵌入 workspace 模板

---

### Phase 1：安全加固（3天）⭐ P0

**目标**：生产环境安全底线

**1.1 Shell 安全模型**（`agent/tools/shell.go`）
- [ ] 实现 ~40 条默认拒绝正则规则
- [ ] 实现路径遍历防护
- [ ] 实现白名单覆盖机制
- [ ] 实现进程组终止（SysProcAttr + Setpgid）
- [ ] 添加可配置的 `ExecToolConfig`
- [ ] 单元测试：覆盖所有拒绝/允许场景

**1.2 工作区沙箱**（`agent/tools/fs.go`）
- [ ] 实现 `validatePath` 函数（符号链接解析、路径遍历防护）
- [ ] 为所有文件系统工具添加沙箱检查
- [ ] 支持 `restrict_to_workspace` 配置
- [ ] 单元测试

---

### Phase 2：Provider 容错（5天）⭐ P0

**目标**：多 Provider 容错，LLM 服务高可用

**2.1 ErrorClassifier**（`providers/errors.go`）
- [ ] 实现 6 种错误分类 + ~40 条模式匹配
- [ ] 实现 HTTP 状态码分类
- [ ] 实现 `IsRetriable()` 判定
- [ ] 单元测试

**2.2 CooldownTracker**（`providers/cooldown.go`）
- [ ] 实现指数退避冷却（标准 + 计费两种策略）
- [ ] 实现 24h 无故障自动重置
- [ ] 实现 `MarkSuccess` / `MarkFailure` / `IsOnCooldown`
- [ ] 并发安全（sync.RWMutex）
- [ ] 单元测试

**2.3 ProviderFactory**（`providers/factory.go`）
- [ ] 实现 `vendor/model` 格式解析
- [ ] 实现默认 endpoint 表（17+ 厂商）
- [ ] 复用 `OpenAIProvider` 构造
- [ ] 单元测试

**2.4 FallbackChain**（`providers/fallback.go`）
- [ ] 实现候选链解析（Primary + Fallbacks）
- [ ] 实现容错执行循环（冷却检查 → 请求 → 错误分类 → 重试/放弃）
- [ ] 实现 `FallbackExhaustedError`（含完整尝试记录）
- [ ] 集成测试

**2.5 Provider 增强**（`providers/openai.go`）
- [ ] 增强错误返回（包含 HTTP 状态码）
- [ ] 支持可配置的 baseURL / apiKey / model

---

### Phase 3：核心功能完善（5天）⭐ P0

**目标**：完成 Channel 和 Session 核心功能

**3.1 Bus 事件增强**（`bus/events.go`）
- [ ] 添加 `Media` / `Metadata` 字段到 InboundMessage
- [ ] 添加 `ReplyTo` / `Media` 字段到 OutboundMessage
- [ ] 更新 `cmd/adapter.go` 的类型桥接

**3.2 Telegram Channel 完整实现**（`channels/telegram.go`）
- [ ] Long Polling 消息接收
- [ ] 文本消息发送（HTML 格式）
- [ ] 消息分片（>4096 字符）
- [ ] 图片/文档接收
- [ ] 语音消息接收 + 转录集成
- [ ] Bot Commands 设置（/start, /clear）
- [ ] 集成测试

**3.3 Session 管理统一**
- [ ] 移除 `agent/types.go` 中的轻量 Session
- [ ] 在 AgentLoop 中使用 `session.Manager`
- [ ] 统一消息格式（含 Timestamp / ToolCalls）
- [ ] 确保 gateway 正确集成

**3.4 Config 增强**（`config/schema.go`）
- [ ] 添加 `Bindings` 配置（多 Agent 路由绑定）
- [ ] 添加 `ModelList` 配置（含 Fallbacks）
- [ ] 添加 `Tools.Exec` 安全配置
- [ ] 添加 `Devices` / `Auth` / `Health` 配置
- [ ] 创建 `config/defaults.go`（默认值常量）
- [ ] 更新 `config.example.json`

---

### Phase 4：多 Agent 架构（4天）⭐ P1

**目标**：支持多 Agent + 路由

**4.1 Agent 实例化**
- [ ] 拆分 `agent/agent.go` → `agent/loop.go` + `agent/instance.go`
- [ ] 每个 AgentInstance 独立：Model、Workspace、ToolRegistry、Session
- [ ] 实现 `AgentRegistry`

**4.2 路由系统**（`routing/route.go`）
- [ ] 实现 `RouteResolver`（7 级优先级）
- [ ] 实现 `SessionKey` 构建
- [ ] 集成到 `AgentLoop`

**4.3 State 管理**（`state/state.go`）
- [ ] 实现原子持久化（temp + rename）
- [ ] 实现 `LastChannel` / `LastChatID` 追踪
- [ ] 集成到 AgentLoop

**4.4 健康检查**（`health/server.go`）
- [ ] 实现 `/health` + `/ready` 端点
- [ ] 集成到 gateway

---

### Phase 5：扩展功能（5天）⭐ P1-P2

**目标**：增强工具和技能体系

**5.1 Cron 表达式**（`cron/service.go`）
- [ ] 集成 `gronx` 库
- [ ] 实现 cron 表达式调度
- [ ] 实现 Cron 管理工具（`agent/tools/cron_tool.go`）

**5.2 技能系统增强**
- [ ] 实现三级加载器（workspace > global > builtin）
- [ ] 创建 `workspace/` 嵌入模板（weather, github, skill-creator, gocode）
- [ ] 实现 `go:embed` 嵌入到 onboard 命令

**5.3 Web 搜索增强**
- [ ] 添加 DuckDuckGo 搜索（免费，无需 Key）
- [ ] 自动选择搜索引擎（有 Key 用 Brave，否则 DuckDuckGo）

**5.4 WhatsApp WebSocket**（`channels/whatsapp.go`）
- [ ] 引入 `gorilla/websocket`
- [ ] 实现 WebSocket 连接 + 自动重连
- [ ] 实现 Bridge 消息协议处理
- [ ] 测试

**5.5 GoScript 增强**
- [ ] 新增标准库注册：`os`, `io`, `encoding/json`
- [ ] 执行资源限制（内存上限、超时更精细）
- [ ] Go Agent 增强：支持上下文变量传入

---

### Phase 6：生态扩展（可选，按需实施）⭐ P2-P3

**6.1 更多 Channel**
- [ ] Discord Bot
- [ ] DingTalk Bot
- [ ] Feishu Bot
- [ ] WeCom Bot

**6.2 认证系统**（`auth/`）
- [ ] OAuth 2.0 + PKCE 实现
- [ ] Token 存储（`~/.simpleclaw/auth.json`）
- [ ] `cmd/auth.go` 命令

**6.3 设备集成**（`devices/`）
- [ ] USB 热插拔监控（Linux udevadm）
- [ ] 非 Linux 平台 Stub
- [ ] 事件通知到 Channel

**6.4 技能市场**（`skills/registry.go`）
- [ ] 远程注册中心接口
- [ ] GitHub Skills 安装
- [ ] 搜索缓存

**6.5 容器化**
- [ ] Dockerfile
- [ ] docker-compose.yml
- [ ] `.dockerignore`

---

## 3. 时间估算汇总

| Phase | 名称 | 工作量 | 优先级 | 累计 |
|-------|------|--------|--------|------|
| 0 | 准备工作 | 1天 | P0 | 1天 |
| 1 | 安全加固 | 3天 | P0 | 4天 |
| 2 | Provider 容错 | 5天 | P0 | 9天 |
| 3 | 核心功能完善 | 5天 | P0 | 14天 |
| 4 | 多 Agent 架构 | 4天 | P1 | 18天 |
| 5 | 扩展功能 | 5天 | P1-P2 | 23天 |
| 6 | 生态扩展 | 8天 | P2-P3 | 31天 |

**MVP 里程碑**（Phase 0-3）：**14 天** — 安全、可靠、功能完整的单 Agent 系统
**完整版**（Phase 0-5）：**23 天** — 多 Agent、完整工具链、多 Channel
**全功能版**（Phase 0-6）：**31 天** — 认证、设备、容器化

---

## 4. 风险与缓解

| 风险 | 影响 | 缓解策略 |
|------|------|---------|
| Telegram Bot API 变更 | Channel 不可用 | 直接 HTTP API，不依赖第三方 SDK |
| WhatsApp Bridge 不稳定 | Channel 不可用 | 自动重连 + 5s 退避 |
| LLM Provider 全部不可用 | 服务中断 | FallbackChain + 友好错误提示 |
| GoScript 安全漏洞 | 代码执行风险 | 沙箱限制 + 资源上限 |
| 配置迁移兼容性 | 用户困扰 | 保持向后兼容 + migrate 命令 |
| 多 Agent 复杂度 | 调试困难 | 结构化日志 + 路由追踪 |

---

## 5. 成功指标

### 5.1 性能指标

| 指标 | 目标 |
|------|------|
| 启动时间 | < 1s |
| 内存占用（空闲） | < 20MB |
| 消息处理延迟（不含 LLM） | < 100ms |
| 二进制大小 | < 15MB（strip） |

### 5.2 可靠性指标

| 指标 | 目标 |
|------|------|
| Provider 故障转移时间 | < 5s |
| Channel 重连时间 | < 10s |
| Session 数据丢失率 | 0%（原子保存） |
| Shell 命令安全覆盖率 | 100%（已知危险命令） |

### 5.3 功能完整性

| 指标 | 目标 |
|------|------|
| LLM Provider 支持 | 17+ |
| Channel 支持 | 6+（Phase 5 后） |
| 内置工具 | 13+ |
| 内置技能 | 5+ |

---

## 6. 与 PicoClaw / Nanobot 的对比优势

完成重构后，SimpleClaw 将具备以下独特优势：

| 特性 | SimpleClaw | PicoClaw | Nanobot |
|------|-----------|----------|---------|
| **GoScript 解释器** | ✅ (独有) | ❌ | ❌ |
| **go_run / go_agent** | ✅ (独有) | ❌ | ❌ |
| **极简依赖** | ✅ (~5 直接) | ❌ (~20) | ❌ (~10) |
| **Provider 容错** | ✅ | ✅ | ❌ |
| **多 Agent 路由** | ✅ | ✅ | ❌ |
| **Shell 安全** | ✅ | ✅ | ❌ |
| **文件存储** | ✅ (无外部依赖) | ✅ | ✅ |
| **Go 原生** | ✅ | ✅ | ❌ (Python) |
| **单二进制部署** | ✅ | ✅ | ❌ |
| **跨平台编译** | ✅ | ✅ | ❌ |

**核心差异化**：SimpleClaw 是唯一能让 AI Agent **动态编写并即时执行 Go 代码**的框架（通过 GoScript SSA 解释器），结合 PicoClaw 级别的生产能力（容错、安全、多 Agent），同时保持极简的 Go 依赖哲学。
