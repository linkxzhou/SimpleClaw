import type { GatewayHealth, GatewayRpcResponse, GatewayStatus } from '@/types/gateway'

const BASE_URL = '/api'

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  if (!res.ok) {
    throw new Error(`HTTP ${res.status}: ${res.statusText}`)
  }
  return res.json()
}

export async function getGatewayStatus(): Promise<GatewayStatus> {
  try {
    const health = await getGatewayHealth()
    return {
      state: health.ok ? 'running' : 'error',
      port: 18790,
      uptime: health.uptime,
      version: health.version,
      error: health.error,
    }
  } catch {
    return { state: 'stopped', port: 18790 }
  }
}

export async function getGatewayHealth(): Promise<GatewayHealth> {
  return request<GatewayHealth>('/health')
}

export async function gatewayRpc<T>(method: string, params?: unknown): Promise<T> {
  const res = await request<GatewayRpcResponse<T>>('/rpc', {
    method: 'POST',
    body: JSON.stringify({ method, params }),
  })
  if (!res.success) {
    throw new Error(res.error || `RPC call failed: ${method}`)
  }
  return res.result as T
}

export async function sendChatMessage(sessionKey: string, message: string, media?: Array<{ filePath: string; mimeType: string; fileName: string }>): Promise<{ runId?: string }> {
  return gatewayRpc<{ runId?: string }>('chat.send', {
    sessionKey,
    message,
    deliver: false,
    media,
    idempotencyKey: crypto.randomUUID(),
  })
}

export async function getChatHistory(sessionKey: string, limit = 200): Promise<{ messages: unknown[]; thinkingLevel?: string }> {
  return gatewayRpc<{ messages: unknown[]; thinkingLevel?: string }>('chat.history', { sessionKey, limit })
}

export async function abortChat(sessionKey: string): Promise<void> {
  await gatewayRpc('chat.abort', { sessionKey })
}

export async function listSessions(limit = 50): Promise<{ sessions: unknown[] }> {
  return gatewayRpc<{ sessions: unknown[] }>('sessions.list', { limit })
}

export function createEventSource(sessionKey: string): EventSource | null {
  try {
    return new EventSource(`${BASE_URL}/events?sessionKey=${encodeURIComponent(sessionKey)}`)
  } catch {
    return null
  }
}
