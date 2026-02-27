<template>
  <div class="dashboard-page">
    <h2>{{ t('sidebar.dashboard') }}</h2>

    <!-- Status cards -->
    <a-row :gutter="[16, 12]" style="margin-bottom: 24px">
      <a-col :xs="12" :sm="12" :md="6">
        <a-card>
          <a-statistic
            :title="t('dashboard.gateway')"
            :value="gatewayStore.status.state === 'running' ? t('status.running') : t('status.stopped')"
          >
            <template #prefix>
              <a-badge :status="gatewayStore.status.state === 'running' ? 'success' : 'default'" />
            </template>
          </a-statistic>
          <div v-if="gatewayStore.status.state === 'running'" class="stat-extra">
            {{ t('dashboard.port', { port: gatewayStore.status.port }) }}
          </div>
        </a-card>
      </a-col>
      <a-col :xs="12" :sm="12" :md="6">
        <a-card>
          <a-statistic
            :title="t('dashboard.channels')"
            :value="connectedChannels"
          >
            <template #suffix>/ {{ totalChannels }}</template>
          </a-statistic>
        </a-card>
      </a-col>
      <a-col :xs="12" :sm="12" :md="6">
        <a-card>
          <a-statistic
            :title="t('dashboard.skills')"
            :value="enabledSkills"
          >
            <template #suffix>/ {{ totalSkills }}</template>
          </a-statistic>
        </a-card>
      </a-col>
      <a-col :xs="12" :sm="12" :md="6">
        <a-card>
          <a-statistic
            :title="t('dashboard.uptime')"
            :value="uptimeStr"
          />
        </a-card>
      </a-col>
    </a-row>

    <!-- Quick actions -->
    <a-card :title="t('dashboard.quickActions.title')" style="margin-bottom: 24px">
      <a-space :size="12" wrap>
        <a-button @click="$router.push('/channels')">
          <template #icon><WifiOutlined /></template>
          {{ t('dashboard.quickActions.addChannel') }}
        </a-button>
        <a-button @click="$router.push('/skills')">
          <template #icon><AppstoreOutlined /></template>
          {{ t('dashboard.quickActions.browseSkills') }}
        </a-button>
        <a-button type="primary" @click="$router.push('/')">
          <template #icon><MessageOutlined /></template>
          {{ t('dashboard.quickActions.openChat') }}
        </a-button>
        <a-button @click="$router.push('/settings')">
          <template #icon><SettingOutlined /></template>
          {{ t('dashboard.quickActions.settings') }}
        </a-button>
      </a-space>
    </a-card>

    <a-row :gutter="[16, 16]">
      <!-- Connected Channels -->
      <a-col :xs="24" :md="12">
        <a-card :title="t('dashboard.connectedChannels')">
          <a-empty v-if="channelsStore.channels.length === 0" :description="t('dashboard.noChannels')">
            <a-button type="primary" @click="$router.push('/channels')">{{ t('dashboard.addFirst') }}</a-button>
          </a-empty>
          <a-list v-else :data-source="channelsStore.channels.slice(0, 5)" size="small">
            <template #renderItem="{ item }">
              <a-list-item>
                <a-list-item-meta :title="item.name">
                  <template #avatar>
                    <span style="font-size: 20px">{{ getChannelIcon(item.type) }}</span>
                  </template>
                  <template #description>
                    <a-badge :status="item.status === 'connected' ? 'success' : 'default'" :text="item.status" />
                  </template>
                </a-list-item-meta>
              </a-list-item>
            </template>
          </a-list>
        </a-card>
      </a-col>

      <!-- Active Skills -->
      <a-col :xs="24" :md="12">
        <a-card :title="t('dashboard.activeSkills')">
          <a-empty v-if="skillsStore.skills.filter(s => s.enabled).length === 0" :description="t('dashboard.noSkills')">
            <a-button type="primary" @click="$router.push('/skills')">{{ t('dashboard.enableSome') }}</a-button>
          </a-empty>
          <div v-else class="skills-grid">
            <a-tag
              v-for="skill in skillsStore.skills.filter(s => s.enabled).slice(0, 12)"
              :key="skill.id"
              color="blue"
            >
              {{ skill.icon || '📦' }} {{ skill.name }}
            </a-tag>
            <a-tag v-if="skillsStore.skills.filter(s => s.enabled).length > 12">
              {{ t('dashboard.more', { count: skillsStore.skills.filter(s => s.enabled).length - 12 }) }}
            </a-tag>
          </div>
        </a-card>
      </a-col>
    </a-row>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useGatewayStore } from '@/stores/gateway'
import { useChannelsStore } from '@/stores/channels'
import { useSkillsStore } from '@/stores/skills'
import { CHANNEL_ICONS, type ChannelType } from '@/types/channel'
import { formatUptime } from '@/lib/utils'
import {
  WifiOutlined, AppstoreOutlined, MessageOutlined, SettingOutlined,
} from '@ant-design/icons-vue'

const { t } = useI18n()
const gatewayStore = useGatewayStore()
const channelsStore = useChannelsStore()
const skillsStore = useSkillsStore()

const totalChannels = computed(() => channelsStore.channels.length)
const connectedChannels = computed(() => channelsStore.channels.filter(c => c.status === 'connected').length)
const totalSkills = computed(() => skillsStore.skills.length)
const enabledSkills = computed(() => skillsStore.skills.filter(s => s.enabled).length)
const uptimeStr = computed(() => {
  const uptime = gatewayStore.status.uptime
  if (!uptime) return '-'
  return formatUptime(uptime)
})

function getChannelIcon(type: string): string {
  return CHANNEL_ICONS[type as ChannelType] || '📡'
}

onMounted(() => {
  if (gatewayStore.status.state === 'running') {
    channelsStore.fetchChannels()
    skillsStore.fetchSkills()
  }
})
</script>

<style scoped>
.dashboard-page { max-width: 100%; }
.stat-extra { font-size: 12px; color: var(--text-tertiary); margin-top: 4px; }
.skills-grid { display: flex; flex-wrap: wrap; gap: 8px; }
</style>
