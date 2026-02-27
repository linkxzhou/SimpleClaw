export interface GatewayStatus {
  state: 'stopped' | 'starting' | 'running' | 'error' | 'reconnecting'
  port: number
  pid?: number
  uptime?: number
  error?: string
  connectedAt?: number
  version?: string
  reconnectAttempts?: number
}

export interface GatewayRpcResponse<T = unknown> {
  success: boolean
  result?: T
  error?: string
}

export interface GatewayHealth {
  ok: boolean
  error?: string
  uptime?: number
  version?: string
}

export interface GatewayNotification {
  method: string
  params?: unknown
}

export interface ProviderConfig {
  id: string
  name: string
  type: 'openai' | 'anthropic' | 'ollama' | 'custom'
  apiKey?: string
  baseUrl?: string
  model?: string
  enabled: boolean
}
