export const PROVIDER_TYPES = [
  'anthropic',
  'openai',
  'google',
  'openrouter',
  'moonshot',
  'siliconflow',
  'ollama',
  'custom',
] as const

export type ProviderType = (typeof PROVIDER_TYPES)[number]

export interface ProviderConfig {
  id: string
  name: string
  type: ProviderType
  baseUrl?: string
  model?: string
  enabled: boolean
  createdAt: string
  updatedAt: string
}

export interface ProviderWithKeyInfo extends ProviderConfig {
  hasKey: boolean
  keyMasked: string | null
}

export interface ProviderTypeInfo {
  id: ProviderType
  name: string
  icon: string
  placeholder: string
  model?: string
  requiresApiKey: boolean
  defaultBaseUrl?: string
  showBaseUrl?: boolean
  showModelId?: boolean
  modelIdPlaceholder?: string
  defaultModelId?: string
}

export const PROVIDER_TYPE_INFO: ProviderTypeInfo[] = [
  { id: 'anthropic', name: 'Anthropic', icon: '🤖', placeholder: 'sk-ant-api03-...', model: 'Claude', requiresApiKey: true },
  { id: 'openai', name: 'OpenAI', icon: '💚', placeholder: 'sk-proj-...', model: 'GPT', requiresApiKey: true },
  { id: 'google', name: 'Google', icon: '🔷', placeholder: 'AIza...', model: 'Gemini', requiresApiKey: true },
  { id: 'openrouter', name: 'OpenRouter', icon: '🌐', placeholder: 'sk-or-v1-...', model: 'Multi-Model', requiresApiKey: true },
  { id: 'moonshot', name: 'Moonshot (CN)', icon: '🌙', placeholder: 'sk-...', model: 'Kimi', requiresApiKey: true, defaultBaseUrl: 'https://api.moonshot.cn/v1', defaultModelId: 'kimi-k2.5' },
  { id: 'siliconflow', name: 'SiliconFlow (CN)', icon: '🌊', placeholder: 'sk-...', model: 'Multi-Model', requiresApiKey: true, defaultBaseUrl: 'https://api.siliconflow.cn/v1', defaultModelId: 'Pro/moonshotai/Kimi-K2.5' },
  { id: 'ollama', name: 'Ollama', icon: '🦙', placeholder: 'Not required', requiresApiKey: false, defaultBaseUrl: 'http://localhost:11434', showBaseUrl: true, showModelId: true, modelIdPlaceholder: 'qwen3:latest' },
  { id: 'custom', name: 'Custom', icon: '⚙️', placeholder: 'API key...', requiresApiKey: true, showBaseUrl: true, showModelId: true, modelIdPlaceholder: 'your-provider/model-id' },
]

export const SETUP_PROVIDERS = PROVIDER_TYPE_INFO

export function getProviderTypeInfo(type: ProviderType): ProviderTypeInfo | undefined {
  return PROVIDER_TYPE_INFO.find((t) => t.id === type)
}
