import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { Channel } from '@/types/channel'
import { useGatewayStore } from './gateway'

export const useChannelsStore = defineStore('channels', () => {
  const channels = ref<Channel[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function fetchChannels() {
    loading.value = true
    error.value = null
    try {
      const gateway = useGatewayStore()
      const result = await gateway.rpc<Record<string, unknown>>('channels.status', { probe: true })
      const channelsList = result.channels as Record<string, unknown>[] ?? []
      channels.value = channelsList.map((ch) => ({
        id: String(ch.id || ch.type || ''),
        type: String(ch.type || '') as Channel['type'],
        name: String(ch.name || ch.type || ''),
        status: inferStatus(ch),
        accountId: ch.accountId ? String(ch.accountId) : undefined,
        lastActivity: ch.lastActivity ? String(ch.lastActivity) : undefined,
        error: ch.error ? String(ch.error) : undefined,
      }))
    } catch (err) {
      error.value = String(err)
    } finally {
      loading.value = false
    }
  }

  function inferStatus(ch: Record<string, unknown>): Channel['status'] {
    if (ch.status) return String(ch.status) as Channel['status']
    if (ch.connected === true) return 'connected'
    if (ch.error) return 'error'
    const lastActivity = ch.lastActivity ? new Date(String(ch.lastActivity)).getTime() : 0
    if (lastActivity > 0 && Date.now() - lastActivity < 10 * 60 * 1000) return 'connected'
    return 'disconnected'
  }

  async function addChannel(params: Record<string, unknown>) {
    const gateway = useGatewayStore()
    await gateway.rpc('channels.add', params)
    await fetchChannels()
  }

  async function deleteChannel(id: string) {
    const gateway = useGatewayStore()
    await gateway.rpc('channels.delete', { id })
    await fetchChannels()
  }

  async function connectChannel(id: string) {
    const gateway = useGatewayStore()
    await gateway.rpc('channels.connect', { id })
    await fetchChannels()
  }

  async function disconnectChannel(id: string) {
    const gateway = useGatewayStore()
    await gateway.rpc('channels.disconnect', { id })
    await fetchChannels()
  }

  function clearError() {
    error.value = null
  }

  return {
    channels, loading, error,
    fetchChannels, addChannel, deleteChannel,
    connectChannel, disconnectChannel, clearError,
  }
})
