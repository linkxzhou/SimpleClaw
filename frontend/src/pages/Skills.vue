<template>
  <div class="skills-page">
    <div class="page-header">
      <div>
        <h2>{{ t('skills.title') }}</h2>
        <p class="subtitle">{{ t('skills.subtitle') }}</p>
      </div>
      <a-button @click="skillsStore.fetchSkills">
        <template #icon><ReloadOutlined /></template>
        {{ t('skills.refresh') }}
      </a-button>
    </div>

    <a-alert
      v-if="gatewayStore.status.state !== 'running'"
      type="warning"
      :message="t('skills.gatewayWarning')"
      show-icon
      style="margin-bottom: 16px"
    />

    <a-tabs v-model:activeKey="activeTab">
      <!-- Installed tab -->
      <a-tab-pane key="installed" :tab="t('skills.tabs.installed')">
        <div class="skills-toolbar">
          <a-input-search
            v-model:value="searchText"
            :placeholder="t('skills.search')"
            style="max-width: 300px"
          />
          <a-radio-group v-model:value="filterSource" button-style="solid" size="small">
            <a-radio-button value="all">{{ t('skills.filter.all', { count: filteredSkills.length }) }}</a-radio-button>
            <a-radio-button value="builtin">{{ t('skills.filter.builtIn', { count: builtinCount }) }}</a-radio-button>
            <a-radio-button value="marketplace">{{ t('skills.filter.marketplace', { count: marketplaceCount }) }}</a-radio-button>
          </a-radio-group>
        </div>

        <a-spin :spinning="skillsStore.loading">
          <a-empty v-if="filteredSkills.length === 0" :description="t('skills.noSkills')" />
          <a-row v-else :gutter="[16, 16]">
            <a-col v-for="skill in filteredSkills" :key="skill.id" :xs="24" :sm="12" :md="8" :lg="6">
              <a-card hoverable size="small" @click="openSkillDetail(skill)">
                <div class="skill-card-header">
                  <span class="skill-icon">{{ skill.icon || '📦' }}</span>
                  <a-switch
                    v-if="!skill.isCore"
                    :checked="skill.enabled"
                    size="small"
                    @change="(val: boolean) => toggleSkill(skill, val)"
                    @click.stop
                  />
                </div>
                <div class="skill-name">{{ skill.name }}</div>
                <div class="skill-desc">{{ skill.description }}</div>
                <div class="skill-meta">
                  <a-tag v-if="skill.isCore" size="small" color="purple">Core</a-tag>
                  <a-tag v-else-if="skill.isBundled" size="small" color="blue">Bundled</a-tag>
                  <a-tag v-if="skill.version" size="small">v{{ skill.version }}</a-tag>
                </div>
              </a-card>
            </a-col>
          </a-row>
        </a-spin>
      </a-tab-pane>

      <!-- Marketplace tab -->
      <a-tab-pane key="marketplace" :tab="t('skills.tabs.marketplace')">
        <div class="skills-toolbar">
          <a-input-search
            v-model:value="marketplaceSearch"
            :placeholder="t('skills.searchMarketplace')"
            :loading="skillsStore.searching"
            @search="handleMarketplaceSearch"
            style="max-width: 400px"
            enter-button
          />
        </div>

        <a-spin :spinning="skillsStore.searching">
          <a-empty
            v-if="skillsStore.searchResults.length === 0 && !skillsStore.searching"
            :description="marketplaceSearch ? t('skills.marketplace.noResults') : t('skills.marketplace.emptyPrompt')"
          />
          <a-list v-else :data-source="skillsStore.searchResults">
            <template #renderItem="{ item }">
              <a-list-item>
                <a-list-item-meta :title="item.name" :description="item.description">
                  <template #avatar>📦</template>
                </a-list-item-meta>
                <template #actions>
                  <a-button
                    v-if="!isInstalled(item.slug)"
                    type="primary"
                    size="small"
                    :loading="skillsStore.installing[item.slug]"
                    @click="skillsStore.installSkill(item.slug)"
                  >
                    {{ t('actions.install') }}
                  </a-button>
                  <a-button
                    v-else
                    danger
                    size="small"
                    @click="skillsStore.uninstallSkill(item.slug)"
                  >
                    {{ t('actions.uninstall') }}
                  </a-button>
                </template>
              </a-list-item>
            </template>
          </a-list>
        </a-spin>
      </a-tab-pane>
    </a-tabs>

    <!-- Skill detail drawer -->
    <a-drawer
      v-model:open="detailVisible"
      :title="selectedSkill?.name"
      :width="isMobile ? '85vw' : 480"
    >
      <template v-if="selectedSkill">
        <a-descriptions :column="1" bordered size="small">
          <a-descriptions-item :label="t('skills.detail.description')">
            {{ selectedSkill.description }}
          </a-descriptions-item>
          <a-descriptions-item v-if="selectedSkill.version" :label="t('skills.detail.version')">
            {{ selectedSkill.version }}
          </a-descriptions-item>
          <a-descriptions-item v-if="selectedSkill.author" :label="t('skills.detail.author')">
            {{ selectedSkill.author }}
          </a-descriptions-item>
          <a-descriptions-item :label="t('skills.detail.source')">
            <a-tag v-if="selectedSkill.isCore" color="purple">{{ t('skills.detail.coreSystem') }}</a-tag>
            <a-tag v-else-if="selectedSkill.isBundled" color="blue">{{ t('skills.detail.bundled') }}</a-tag>
            <a-tag v-else color="green">{{ t('skills.detail.userInstalled') }}</a-tag>
          </a-descriptions-item>
        </a-descriptions>

        <div style="margin-top: 16px">
          <span>{{ selectedSkill.enabled ? t('skills.detail.enabled') : t('skills.detail.disabled') }}</span>
          <a-switch
            v-if="!selectedSkill.isCore"
            :checked="selectedSkill.enabled"
            style="margin-left: 8px"
            @change="(val: boolean) => toggleSkill(selectedSkill!, val)"
          />
        </div>
      </template>
    </a-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useSkillsStore } from '@/stores/skills'
import { useGatewayStore } from '@/stores/gateway'
import { useSettingsStore } from '@/stores/settings'
import type { Skill } from '@/types/skill'
import { message } from 'ant-design-vue'
import { ReloadOutlined } from '@ant-design/icons-vue'

const { t } = useI18n()
const skillsStore = useSkillsStore()
const gatewayStore = useGatewayStore()
const settingsStore = useSettingsStore()

const isMobile = computed(() => settingsStore.isMobile)

const activeTab = ref('installed')
const searchText = ref('')
const filterSource = ref('all')
const marketplaceSearch = ref('')
const detailVisible = ref(false)
const selectedSkill = ref<Skill | null>(null)

const filteredSkills = computed(() => {
  let list = skillsStore.skills
  if (searchText.value) {
    const q = searchText.value.toLowerCase()
    list = list.filter(s => s.name.toLowerCase().includes(q) || s.description.toLowerCase().includes(q))
  }
  if (filterSource.value === 'builtin') list = list.filter(s => s.isCore || s.isBundled)
  if (filterSource.value === 'marketplace') list = list.filter(s => !s.isCore && !s.isBundled)
  return list
})

const builtinCount = computed(() => skillsStore.skills.filter(s => s.isCore || s.isBundled).length)
const marketplaceCount = computed(() => skillsStore.skills.filter(s => !s.isCore && !s.isBundled).length)

function isInstalled(slug: string): boolean {
  return skillsStore.skills.some(s => s.slug === slug || s.id === slug)
}

async function toggleSkill(skill: Skill, enabled: boolean) {
  try {
    if (enabled) {
      await skillsStore.enableSkill(skill.id)
    } else {
      await skillsStore.disableSkill(skill.id)
    }
  } catch (err) {
    message.error(String(err))
  }
}

function openSkillDetail(skill: Skill) {
  selectedSkill.value = skill
  detailVisible.value = true
}

function handleMarketplaceSearch(value: string) {
  if (value.trim()) {
    skillsStore.searchSkills(value.trim())
  }
}

onMounted(() => {
  if (gatewayStore.status.state === 'running') {
    skillsStore.fetchSkills()
  }
})
</script>

<style scoped>
.skills-page { max-width: 100%; }
.skills-toolbar { display: flex; gap: 12px; align-items: center; margin-bottom: 16px; flex-wrap: wrap; }
.skill-card-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px; }
.skill-icon { font-size: 28px; }
.skill-name { font-weight: 600; margin-bottom: 4px; }
.skill-desc {
  font-size: 12px; color: var(--text-secondary);
  display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical;
  overflow: hidden; margin-bottom: 8px;
}
.skill-meta { display: flex; gap: 4px; flex-wrap: wrap; }

@media (max-width: 767px) {
  .skills-toolbar { flex-direction: column; align-items: stretch; }
  .skills-toolbar :deep(.ant-input-search) { max-width: 100% !important; }
  .skills-toolbar :deep(.ant-radio-group) { display: flex; overflow-x: auto; white-space: nowrap; }
  .skill-icon { font-size: 22px; }
}
</style>
