# config

SimpleClaw 的配置管理包，提供配置模式定义、加载/保存和默认值生成。

## 目录结构

```
config/
├── schema.go       # 根配置结构体及所有子模块配置定义
├── loader.go       # 配置文件加载、保存、命名转换（camelCase ↔ snake_case）
├── defaults.go     # 默认常量（AppName、DefaultModel、DefaultEndpoints）
└── config_test.go  # 测试
```

## 配置结构

```
Config
├── Agents           Agent 相关配置
│   ├── Defaults     默认参数（workspace、model、maxTokens、temperature 等）
│   └── List[]       多 Agent 列表（支持不同模型和 fallback）
├── Bindings[]       Agent 路由绑定规则（channel/peer/guild 匹配）
├── Channels         聊天渠道配置
│   ├── Telegram     Bot Token + 白名单
│   ├── WhatsApp     Bridge URL + 白名单
│   ├── Discord      Token（预留）
│   └── DingTalk     ClientID/Secret（预留）
├── Providers        LLM 提供商配置
│   ├── OpenRouter   API Key + Base URL
│   ├── Anthropic    API Key
│   ├── OpenAI       API Key
│   ├── Gemini       API Key
│   ├── Zhipu        API Key + Base URL
│   ├── Groq         API Key
│   ├── DeepSeek     API Key
│   └── VLLM         API Key + Base URL
├── Gateway          网关配置（Host + Port）
├── Tools            工具配置
│   ├── Web.Search   搜索引擎（API Key + 引擎类型 + 最大结果数）
│   └── Exec         Shell 执行（超时 + 最大输出）
└── Heartbeat        心跳配置（Enabled + IntervalMin）
```

## 配置文件

默认路径：`~/.simpleclaw/config.json`

```json
{
  "agents": {
    "defaults": {
      "workspace": "~/.simpleclaw/workspace",
      "model": "anthropic/claude-sonnet-4-20250514",
      "maxTokens": 8192,
      "temperature": 0.7,
      "maxToolIterations": 20,
      "restrictToWorkspace": true
    }
  },
  "providers": {
    "openrouter": { "apiKey": "sk-..." }
  },
  "channels": {
    "telegram": { "enabled": false, "token": "" },
    "whatsapp": { "enabled": false, "bridgeUrl": "ws://localhost:3001" }
  },
  "gateway": { "host": "0.0.0.0", "port": 18790 },
  "heartbeat": { "enabled": true, "intervalMin": 30 }
}
```

## 核心 API

| 函数/方法 | 说明 |
|-----------|------|
| `DefaultConfig()` | 返回带有合理默认值的配置 |
| `Load(path)` | 加载配置，文件不存在返回默认值 |
| `Save(cfg, path)` | 保存配置为格式化 JSON |
| `GetConfigPath()` | 返回默认配置路径 |
| `cfg.WorkspacePath()` | 展开 `~` 前缀的工作区路径 |
| `cfg.GetAPIKey()` | 按优先级返回第一个非空 API Key |
| `cfg.GetAPIBase()` | 返回当前活跃提供商的 API Base URL |
| `cfg.GetAPIKeys()` | 返回所有 vendor → apiKey 映射 |
| `cfg.GetEndpoints()` | 返回所有自定义 endpoint 映射 |
| `CamelToSnake(name)` | camelCase → snake_case |
| `SnakeToCamel(name)` | snake_case → camelCase |

## 支持的 LLM 提供商

| Vendor | 默认 Endpoint |
|--------|---------------|
| OpenAI | `https://api.openai.com/v1` |
| Anthropic | `https://api.anthropic.com/v1` |
| OpenRouter | `https://openrouter.ai/api/v1` |
| DeepSeek | `https://api.deepseek.com/v1` |
| Zhipu (智谱) | `https://open.bigmodel.cn/api/paas/v4` |
| Groq | `https://api.groq.com/openai/v1` |
| Gemini | `https://generativelanguage.googleapis.com/v1beta/openai` |
| Moonshot | `https://api.moonshot.cn/v1` |
| Qwen (通义) | `https://dashscope.aliyuncs.com/compatible-mode/v1` |
| Ollama | `http://localhost:11434/v1` |
| vLLM | `http://localhost:8000/v1` |
