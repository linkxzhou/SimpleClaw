import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { Skill, MarketplaceSkill } from '@/types/skill'
import { useGatewayStore } from './gateway'

export const useSkillsStore = defineStore('skills', () => {
  const skills = ref<Skill[]>([])
  const searchResults = ref<MarketplaceSkill[]>([])
  const loading = ref(false)
  const searching = ref(false)
  const searchError = ref<string | null>(null)
  const installing = ref<Record<string, boolean>>({})
  const error = ref<string | null>(null)

  async function fetchSkills() {
    loading.value = true
    error.value = null
    try {
      const gateway = useGatewayStore()
      const result = await gateway.rpc<{ skills: Skill[] }>('skills.status')
      skills.value = result.skills || []
    } catch (err) {
      error.value = String(err)
    } finally {
      loading.value = false
    }
  }

  async function enableSkill(id: string) {
    const gateway = useGatewayStore()
    await gateway.rpc('skills.update', { id, enabled: true })
    await fetchSkills()
  }

  async function disableSkill(id: string) {
    const gateway = useGatewayStore()
    await gateway.rpc('skills.update', { id, enabled: false })
    await fetchSkills()
  }

  async function searchSkills(query: string) {
    searching.value = true
    searchError.value = null
    try {
      const gateway = useGatewayStore()
      const result = await gateway.rpc<{ skills: MarketplaceSkill[] }>('skills.search', { query })
      searchResults.value = result.skills || []
    } catch (err) {
      searchError.value = String(err)
    } finally {
      searching.value = false
    }
  }

  async function installSkill(slug: string) {
    installing.value = { ...installing.value, [slug]: true }
    try {
      const gateway = useGatewayStore()
      await gateway.rpc('skills.install', { slug })
      await fetchSkills()
    } finally {
      const next = { ...installing.value }
      delete next[slug]
      installing.value = next
    }
  }

  async function uninstallSkill(slug: string) {
    const gateway = useGatewayStore()
    await gateway.rpc('skills.uninstall', { slug })
    await fetchSkills()
  }

  return {
    skills, searchResults, loading, searching, searchError, installing, error,
    fetchSkills, enableSkill, disableSkill, searchSkills, installSkill, uninstallSkill,
  }
})
