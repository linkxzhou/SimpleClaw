import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { GatewayStatus, GatewayHealth } from '@/types/gateway'
import { getGatewayStatus, getGatewayHealth, gatewayRpc } from '@/api/gateway'

export const useGatewayStore = defineStore('gateway', () => {
  const status = ref<GatewayStatus>({ state: 'stopped', port: 18790 })
  const health = ref<GatewayHealth | null>(null)
  const isInitialized = ref(false)
  const lastError = ref<string | null>(null)
  let pollingTimer: ReturnType<typeof setInterval> | null = null

  async function init() {
    if (isInitialized.value) return
    try {
      const s = await getGatewayStatus()
      status.value = s
      isInitialized.value = true
      startPolling()
    } catch (error) {
      lastError.value = String(error)
      status.value = { state: 'stopped', port: 18790 }
      isInitialized.value = true
    }
  }

  function startPolling() {
    if (pollingTimer) return
    pollingTimer = setInterval(async () => {
      try {
        const s = await getGatewayStatus()
        status.value = s
        lastError.value = null
      } catch {
        status.value = { ...status.value, state: 'stopped' }
      }
    }, 10000)
  }

  function stopPolling() {
    if (pollingTimer) {
      clearInterval(pollingTimer)
      pollingTimer = null
    }
  }

  async function checkHealth(): Promise<GatewayHealth> {
    try {
      const h = await getGatewayHealth()
      health.value = h
      return h
    } catch (error) {
      const h: GatewayHealth = { ok: false, error: String(error) }
      health.value = h
      return h
    }
  }

  async function rpc<T>(method: string, params?: unknown): Promise<T> {
    return gatewayRpc<T>(method, params)
  }

  function clearError() {
    lastError.value = null
  }

  return {
    status, health, isInitialized, lastError,
    init, checkHealth, rpc, clearError, startPolling, stopPolling,
  }
})
