<template>
  <div class="cron-page">
    <div class="page-header">
      <div>
        <h2>{{ t('cron.title') }}</h2>
        <p class="subtitle">{{ t('cron.subtitle') }}</p>
      </div>
      <a-space>
        <a-button @click="cronStore.fetchJobs">
          <template #icon><ReloadOutlined /></template>
          {{ t('cron.refresh') }}
        </a-button>
        <a-button type="primary" @click="openCreateDialog">
          <template #icon><PlusOutlined /></template>
          {{ t('cron.newTask') }}
        </a-button>
      </a-space>
    </div>

    <a-alert
      v-if="gatewayStore.status.state !== 'running'"
      type="warning"
      :message="t('cron.gatewayWarning')"
      show-icon
      style="margin-bottom: 16px"
    />

    <!-- Stats -->
    <a-row :gutter="[12, 12]" style="margin-bottom: 24px">
      <a-col :xs="12" :sm="6">
        <a-card><a-statistic :title="t('cron.stats.total')" :value="cronStore.jobs.length" /></a-card>
      </a-col>
      <a-col :xs="12" :sm="6">
        <a-card><a-statistic :title="t('cron.stats.active')" :value="activeCount" /></a-card>
      </a-col>
      <a-col :xs="12" :sm="6">
        <a-card><a-statistic :title="t('cron.stats.paused')" :value="pausedCount" /></a-card>
      </a-col>
      <a-col :xs="12" :sm="6">
        <a-card><a-statistic :title="t('cron.stats.failed')" :value="failedCount" /></a-card>
      </a-col>
    </a-row>

    <!-- Job list -->
    <a-spin :spinning="cronStore.loading">
      <a-empty v-if="cronStore.jobs.length === 0" :description="t('cron.empty.title')">
        <template #extra>
          <p>{{ t('cron.empty.description') }}</p>
          <a-button type="primary" @click="openCreateDialog">{{ t('cron.empty.create') }}</a-button>
        </template>
      </a-empty>

      <a-list v-else :data-source="cronStore.jobs">
        <template #renderItem="{ item }">
          <a-list-item>
            <a-list-item-meta>
              <template #title>
                <a-space>
                  {{ item.name }}
                  <a-badge :status="item.enabled ? 'success' : 'default'" :text="item.enabled ? t('status.active') : t('status.paused')" />
                </a-space>
              </template>
              <template #description>
                <div>{{ truncate(item.message, 100) }}</div>
                <div style="margin-top: 4px; font-size: 12px; color: #999;">
                  <span>📅 {{ formatSchedule(item.schedule) }}</span>
                  <span style="margin-left: 12px">📡 {{ item.target?.channelName || item.target?.channelType }}</span>
                </div>
                <div v-if="item.lastRun" style="font-size: 12px; margin-top: 2px">
                  <a-badge :status="item.lastRun.success ? 'success' : 'error'" />
                  {{ t('cron.card.last') }}: {{ formatRelativeTime(item.lastRun.time) }}
                </div>
              </template>
            </a-list-item-meta>
            <template #actions>
              <a-switch :checked="item.enabled" size="small" @change="(val: boolean) => cronStore.toggleJob(item.id, val)" />
              <a-button size="small" @click="cronStore.triggerJob(item.id)">{{ t('cron.card.runNow') }}</a-button>
              <a-button size="small" @click="openEditDialog(item)">
                <template #icon><EditOutlined /></template>
              </a-button>
              <a-popconfirm :title="t('cron.card.deleteConfirm')" @confirm="cronStore.deleteJob(item.id)">
                <a-button danger size="small"><template #icon><DeleteOutlined /></template></a-button>
              </a-popconfirm>
            </template>
          </a-list-item>
        </template>
      </a-list>
    </a-spin>

    <!-- Create/Edit Dialog -->
    <a-modal
      v-model:open="dialogVisible"
      :title="editingJob ? t('cron.dialog.editTitle') : t('cron.dialog.createTitle')"
      @ok="handleSaveJob"
      :confirm-loading="saving"
    >
      <a-form layout="vertical">
        <a-form-item :label="t('cron.dialog.taskName')" required>
          <a-input v-model:value="formData.name" :placeholder="t('cron.dialog.taskNamePlaceholder')" />
        </a-form-item>
        <a-form-item :label="t('cron.dialog.message')" required>
          <a-textarea v-model:value="formData.message" :rows="3" :placeholder="t('cron.dialog.messagePlaceholder')" />
        </a-form-item>
        <a-form-item :label="t('cron.dialog.schedule')" required>
          <a-radio-group v-model:value="scheduleMode" style="margin-bottom: 8px">
            <a-radio-button value="preset">{{ t('cron.dialog.schedule') }}</a-radio-button>
            <a-radio-button value="custom">Custom Cron</a-radio-button>
          </a-radio-group>
          <a-select v-if="scheduleMode === 'preset'" v-model:value="formData.schedule" style="width: 100%">
            <a-select-option value="* * * * *">{{ t('cron.presets.everyMinute') }}</a-select-option>
            <a-select-option value="*/5 * * * *">{{ t('cron.presets.every5Min') }}</a-select-option>
            <a-select-option value="*/15 * * * *">{{ t('cron.presets.every15Min') }}</a-select-option>
            <a-select-option value="0 * * * *">{{ t('cron.presets.everyHour') }}</a-select-option>
            <a-select-option value="0 9 * * *">{{ t('cron.presets.daily9am') }}</a-select-option>
            <a-select-option value="0 18 * * *">{{ t('cron.presets.daily6pm') }}</a-select-option>
            <a-select-option value="0 9 * * 1">{{ t('cron.presets.weeklyMon') }}</a-select-option>
            <a-select-option value="0 9 1 * *">{{ t('cron.presets.monthly1st') }}</a-select-option>
          </a-select>
          <a-input v-else v-model:value="formData.schedule" :placeholder="t('cron.dialog.cronPlaceholder')" />
        </a-form-item>
        <a-form-item :label="t('cron.dialog.targetChannel')" required>
          <a-select v-model:value="formData.targetChannelId" style="width: 100%">
            <a-select-option v-for="ch in channelsStore.channels" :key="ch.id" :value="ch.id">
              {{ getChannelIcon(ch.type) }} {{ ch.name }}
            </a-select-option>
          </a-select>
          <a-empty v-if="channelsStore.channels.length === 0" :description="t('cron.dialog.noChannels')" />
        </a-form-item>
        <a-form-item v-if="!editingJob">
          <a-checkbox v-model:checked="formData.enabled">{{ t('cron.dialog.enableImmediately') }}</a-checkbox>
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, reactive, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useCronStore } from '@/stores/cron'
import { useChannelsStore } from '@/stores/channels'
import { useGatewayStore } from '@/stores/gateway'
import type { CronJob, CronSchedule } from '@/types/cron'
import { CHANNEL_ICONS, type ChannelType } from '@/types/channel'
import { truncate, formatRelativeTime } from '@/lib/utils'
import { message } from 'ant-design-vue'
import { ReloadOutlined, PlusOutlined, EditOutlined, DeleteOutlined } from '@ant-design/icons-vue'

const { t } = useI18n()
const cronStore = useCronStore()
const channelsStore = useChannelsStore()
const gatewayStore = useGatewayStore()

const dialogVisible = ref(false)
const editingJob = ref<CronJob | null>(null)
const saving = ref(false)
const scheduleMode = ref('preset')
const formData = reactive({
  name: '',
  message: '',
  schedule: '0 9 * * *',
  targetChannelId: '',
  enabled: true,
})

const activeCount = computed(() => cronStore.jobs.filter(j => j.enabled).length)
const pausedCount = computed(() => cronStore.jobs.filter(j => !j.enabled).length)
const failedCount = computed(() => cronStore.jobs.filter(j => j.lastRun && !j.lastRun.success).length)

function getChannelIcon(type: string): string {
  return CHANNEL_ICONS[type as ChannelType] || '📡'
}

function formatSchedule(schedule: string | CronSchedule): string {
  if (typeof schedule === 'string') return schedule
  if (schedule.kind === 'cron') return schedule.expr
  if (schedule.kind === 'every') return `Every ${schedule.everyMs / 1000}s`
  if (schedule.kind === 'at') return `At ${schedule.at}`
  return String(schedule)
}

function openCreateDialog() {
  editingJob.value = null
  formData.name = ''
  formData.message = ''
  formData.schedule = '0 9 * * *'
  formData.targetChannelId = channelsStore.channels[0]?.id || ''
  formData.enabled = true
  scheduleMode.value = 'preset'
  dialogVisible.value = true
}

function openEditDialog(job: CronJob) {
  editingJob.value = job
  formData.name = job.name
  formData.message = job.message
  formData.schedule = typeof job.schedule === 'string' ? job.schedule : ''
  formData.targetChannelId = job.target?.channelId || ''
  formData.enabled = job.enabled
  scheduleMode.value = 'custom'
  dialogVisible.value = true
}

async function handleSaveJob() {
  if (!formData.name.trim() || !formData.message.trim() || !formData.schedule.trim()) {
    message.warning(t('cron.dialog.taskName'))
    return
  }
  saving.value = true
  try {
    const channel = channelsStore.channels.find(c => c.id === formData.targetChannelId)
    const target = {
      channelType: channel?.type || 'telegram',
      channelId: formData.targetChannelId,
      channelName: channel?.name || '',
    }
    if (editingJob.value) {
      await cronStore.updateJob(editingJob.value.id, {
        name: formData.name,
        message: formData.message,
        schedule: formData.schedule,
        target,
      })
    } else {
      await cronStore.createJob({
        name: formData.name,
        message: formData.message,
        schedule: formData.schedule,
        target,
        enabled: formData.enabled,
      })
    }
    dialogVisible.value = false
  } catch (err) {
    message.error(String(err))
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  if (gatewayStore.status.state === 'running') {
    cronStore.fetchJobs()
    channelsStore.fetchChannels()
  }
})
</script>

<style scoped>
.cron-page { max-width: 100%; }

@media (max-width: 767px) {
  .cron-page :deep(.ant-list-item-action) { flex-wrap: wrap; gap: 4px; }
  .cron-page :deep(.ant-list-item-action > li) { padding: 0 4px; }
}
</style>
