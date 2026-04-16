# sop — SOP 标准操作流程

用 JSON + Markdown 定义可复用的多步骤工作流，支持多种触发方式和执行模式，让定时任务从"执行单条消息"升级为"编排多步骤流程"。

## 架构

```
~/.simpleclaw/workspace/sops/
├── deploy-pipeline/
│   ├── sop.json          # 元数据（名称、优先级、触发器、执行模式）
│   └── steps.md          # 步骤定义（Markdown 格式）
└── daily-report/
    ├── sop.json
    └── steps.md

                    ┌────────────────────┐
                    │    LoadSOPs()       │  扫描 sops/ 子目录
                    │  ├── sop.json      │  解析元数据
                    │  └── steps.md      │  解析步骤（Markdown）
                    └────────┬───────────┘
                             │
                    ┌────────▼───────────┐
                    │      Engine        │  执行引擎
                    │  ├── Auto          │  全自动执行
                    │  ├── Supervised    │  开始前确认
                    │  ├── StepByStep    │  每步确认
                    │  └── Deterministic │  无 LLM，纯工具链
                    └────────────────────┘
```

## 文件结构

| 文件 | 说明 |
|------|------|
| `types.go` | 数据模型（Sop、SopStep、SopRun、优先级、执行模式、触发器） |
| `loader.go` | SOP 加载器 + Markdown 步骤解析器 |
| `engine.go` | 执行引擎（四种模式、checkpoint 暂停、LLM/工具双路径） |
| `sop_test.go` | 单元测试（12 个用例） |

## 执行模式

| 模式 | 说明 | 确认行为 |
|------|------|---------|
| `auto` | 全自动 | 无确认，直接执行 |
| `supervised` | 监督模式 | 开始前确认一次 |
| `step_by_step` | 逐步确认 | 每个步骤前确认 |
| `deterministic` | 确定性模式 | 不调用 LLM，直接调用 SuggestedTools |

## SOP 定义格式

### sop.json

```json
{
  "name": "deploy-pipeline",
  "description": "标准部署流水线",
  "priority": "high",
  "execution_mode": "supervised",
  "cooldown_secs": 300,
  "max_concurrent": 1,
  "triggers": [
    { "type": "manual" },
    { "type": "cron", "expression": "0 2 * * *" }
  ]
}
```

### steps.md

```markdown
## Step 1: 拉取最新代码
tools: exec
执行 `git pull origin main`。

## Step 2: 运行测试 [checkpoint]
tools: exec
执行 `go test ./...`。如果有测试失败，停止。

## Step 3: 构建二进制
tools: exec
执行 `go build -o simpleclaw ./cmd/`
```

- `## Step N: Title` — 步骤标题（必需）
- `[checkpoint]` — 标记为暂停点，执行前需确认
- `tools: name1, name2` — 建议使用的工具列表
- 正文 — 步骤描述/指令

## 核心 API

```go
// 加载所有 SOP
sops, err := sop.LoadSOPs(workspaceDir)

// 创建执行引擎
engine := sop.NewEngine(sop.EngineConfig{
    ConfirmFunc: func(title, desc string) bool { return true },
    LLM:         myLLMExecutor,
    Tools:       myToolExecutor,
    Model:       "gpt-4o",
})

// 执行 SOP
run := engine.Execute(ctx, sops[0])
fmt.Println(run.Status)      // completed | failed
fmt.Println(run.StepResults) // 每步结果
```

## CLI 命令

```bash
simpleclaw sop list                        # 列出所有 SOP
simpleclaw sop run deploy-pipeline         # 执行 SOP
simpleclaw sop run deploy-pipeline --step-by-step  # 逐步确认模式
```

## 触发方式

| 类型 | 说明 |
|------|------|
| `manual` | 手动通过 CLI 触发 |
| `cron` | cron 表达式定时触发 |
| `webhook` | Webhook 路径触发（预留） |
