import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { ProviderWithKeyInfo } from '@/lib/providers'
import { useGatewayStore } from './gateway'

export const useProviderStore = defineStore('providers', () => {
  const providers = ref<ProviderWithKeyInfo[]>([])
  const defaultProviderId = ref<string | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function fetchProviders() {
    loading.value = true
    error.value = null
    try {
      const gateway = useGatewayStore()
      const [list, defaultRes] = await Promise.all([
        gateway.rpc<{ providers: ProviderWithKeyInfo[] }>('provider.list'),
        gateway.rpc<{ id: string }>('provider.getDefault').catch(() => ({ id: '' })),
      ])
      providers.value = list.providers || []
      defaultProviderId.value = defaultRes.id || null
    } catch (err) {
      error.value = String(err)
    } finally {
      loading.value = false
    }
  }

  async function addProvider(config: Record<string, unknown>, apiKey?: string) {
    const gateway = useGatewayStore()
    await gateway.rpc('provider.save', { ...config, apiKey })
    await fetchProviders()
  }

  async function updateProvider(id: string, updates: Record<string, unknown>, apiKey?: string) {
    const gateway = useGatewayStore()
    await gateway.rpc('provider.save', { id, ...updates, apiKey })
    await fetchProviders()
  }

  async function deleteProvider(id: string) {
    const gateway = useGatewayStore()
    await gateway.rpc('provider.delete', { id })
    await fetchProviders()
  }

  async function setDefaultProvider(id: string) {
    const gateway = useGatewayStore()
    await gateway.rpc('provider.setDefault', { id })
    defaultProviderId.value = id
  }

  async function validateApiKey(id: string, apiKey: string): Promise<boolean> {
    const gateway = useGatewayStore()
    const result = await gateway.rpc<{ valid: boolean }>('provider.validateKey', { id, apiKey })
    return result.valid
  }

  return {
    providers, defaultProviderId, loading, error,
    fetchProviders, addProvider, updateProvider, deleteProvider,
    setDefaultProvider, validateApiKey,
  }
})
