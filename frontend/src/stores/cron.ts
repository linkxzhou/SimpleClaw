import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { CronJob, CronJobCreateInput, CronJobUpdateInput } from '@/types/cron'
import { useGatewayStore } from './gateway'

export const useCronStore = defineStore('cron', () => {
  const jobs = ref<CronJob[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function fetchJobs() {
    loading.value = true
    error.value = null
    try {
      const gateway = useGatewayStore()
      const result = await gateway.rpc<{ jobs: CronJob[] }>('cron.list')
      jobs.value = result.jobs || []
    } catch (err) {
      error.value = String(err)
    } finally {
      loading.value = false
    }
  }

  async function createJob(input: CronJobCreateInput) {
    const gateway = useGatewayStore()
    await gateway.rpc('cron.create', input)
    await fetchJobs()
  }

  async function updateJob(id: string, input: CronJobUpdateInput) {
    const gateway = useGatewayStore()
    await gateway.rpc('cron.update', { id, ...input })
    await fetchJobs()
  }

  async function deleteJob(id: string) {
    const gateway = useGatewayStore()
    await gateway.rpc('cron.delete', { id })
    await fetchJobs()
  }

  async function toggleJob(id: string, enabled: boolean) {
    const gateway = useGatewayStore()
    await gateway.rpc('cron.toggle', { id, enabled })
    await fetchJobs()
  }

  async function triggerJob(id: string) {
    const gateway = useGatewayStore()
    await gateway.rpc('cron.trigger', { id })
    await fetchJobs()
  }

  return {
    jobs, loading, error,
    fetchJobs, createJob, updateJob, deleteJob, toggleJob, triggerJob,
  }
})
