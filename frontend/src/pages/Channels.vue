<template>
  <div class="channels-page">
    <div class="page-header">
      <div>
        <h2>{{ t('channels.title') }}</h2>
        <p class="subtitle">{{ t('channels.subtitle') }}</p>
      </div>
      <a-space>
        <a-button @click="channelsStore.fetchChannels">
          <template #icon><ReloadOutlined /></template>
          {{ t('channels.refresh') }}
        </a-button>
        <a-button type="primary" @click="showAddDialog = true">
          <template #icon><PlusOutlined /></template>
          {{ t('channels.addChannel') }}
        </a-button>
      </a-space>
    </div>

    <a-alert
      v-if="gatewayStore.status.state !== 'running'"
      type="warning"
      :message="t('channels.gatewayWarning')"
      show-icon
      style="margin-bottom: 16px"
    />

    <!-- Stats -->
    <a-row :gutter="[16, 12]" style="margin-bottom: 24px">
      <a-col :xs="8" :sm="8">
        <a-card><a-statistic :title="t('channels.stats.total')" :value="channelsStore.channels.length" /></a-card>
      </a-col>
      <a-col :xs="8" :sm="8">
        <a-card><a-statistic :title="t('channels.stats.connected')" :value="connectedCount" /></a-card>
      </a-col>
      <a-col :xs="8" :sm="8">
        <a-card><a-statistic :title="t('channels.stats.disconnected')" :value="channelsStore.channels.length - connectedCount" /></a-card>
      </a-col>
    </a-row>

    <!-- Configured channels -->
    <a-card v-if="channelsStore.channels.length > 0" style="margin-bottom: 24px">
      <a-list :data-source="channelsStore.channels" :loading="channelsStore.loading">
        <template #renderItem="{ item }">
          <a-list-item>
            <a-list-item-meta>
              <template #avatar>
                <span style="font-size: 28px">{{ getChannelIcon(item.type) }}</span>
              </template>
              <template #title>{{ item.name }}</template>
              <template #description>
                <a-badge
                  :status="item.status === 'connected' ? 'success' : item.status === 'error' ? 'error' : 'default'"
                  :text="item.status"
                />
              </template>
            </a-list-item-meta>
            <template #actions>
              <a-popconfirm
                :title="t('actions.delete') + '?'"
                @confirm="channelsStore.deleteChannel(item.id)"
              >
                <a-button danger type="text" size="small">
                  <template #icon><DeleteOutlined /></template>
                </a-button>
              </a-popconfirm>
            </template>
          </a-list-item>
        </template>
      </a-list>
    </a-card>

    <!-- Available channels -->
    <a-card :title="t('channels.available')">
      <p class="subtitle">{{ t('channels.availableDesc') }}</p>
      <a-row :gutter="[16, 16]">
        <a-col v-for="meta in availableChannels" :key="meta.id" :xs="12" :sm="8" :md="6">
          <a-card
            hoverable
            size="small"
            @click="selectChannel(meta.id)"
            class="channel-card"
          >
            <div class="channel-card-content">
              <span class="channel-icon">{{ meta.icon }}</span>
              <div class="channel-name">{{ meta.name }}</div>
              <a-tag v-if="meta.isPlugin" size="small">{{ t('channels.pluginBadge') }}</a-tag>
            </div>
          </a-card>
        </a-col>
      </a-row>
    </a-card>

    <!-- Add channel dialog -->
    <a-modal
      v-model:open="showAddDialog"
      :title="selectedMeta ? t('channels.dialog.configureTitle', { name: selectedMeta.name }) : t('channels.dialog.addTitle')"
      :footer="null"
      :width="isMobile ? '95vw' : 600"
      @cancel="resetDialog"
    >
      <!-- Channel type selection -->
      <template v-if="!selectedMeta">
        <p>{{ t('channels.dialog.selectDesc') }}</p>
        <a-row :gutter="[12, 12]">
          <a-col v-for="meta in allChannelMeta" :key="meta.id" :xs="12" :sm="8">
            <a-card hoverable size="small" @click="selectChannel(meta.id)" class="channel-card">
              <div class="channel-card-content">
                <span class="channel-icon">{{ meta.icon }}</span>
                <div class="channel-name">{{ meta.name }}</div>
              </div>
            </a-card>
          </a-col>
        </a-row>
      </template>

      <!-- Configuration form -->
      <template v-else>
        <a-button type="link" @click="selectedMeta = null" style="padding: 0; margin-bottom: 12px">
          ← {{ t('actions.back') }}
        </a-button>

        <!-- Instructions -->
        <a-collapse v-if="selectedMeta.instructions.length > 0" :bordered="false" style="margin-bottom: 16px">
          <a-collapse-panel :header="t('channels.dialog.howToConnect')">
            <a-steps direction="vertical" size="small" :current="-1">
              <a-step
                v-for="(inst, i) in selectedMeta.instructions"
                :key="i"
                :title="resolveI18n(inst)"
              />
            </a-steps>
          </a-collapse-panel>
        </a-collapse>

        <a-form layout="vertical">
          <a-form-item
            v-for="field in selectedMeta.configFields"
            :key="field.key"
            :label="resolveI18n(field.label)"
            :required="field.required"
          >
            <a-input
              v-if="field.type === 'text'"
              v-model:value="configValues[field.key]"
              :placeholder="resolveI18n(field.placeholder || '')"
            />
            <a-input-password
              v-else-if="field.type === 'password'"
              v-model:value="configValues[field.key]"
              :placeholder="resolveI18n(field.placeholder || '')"
            />
            <p v-if="field.description" class="field-desc">{{ resolveI18n(field.description) }}</p>
            <p v-if="field.envVar" class="field-desc">{{ t('channels.dialog.envVar', { var: field.envVar }) }}</p>
          </a-form-item>

          <a-form-item>
            <a-button type="primary" :loading="saving" @click="handleSaveChannel" block>
              {{ t('channels.dialog.saveAndConnect') }}
            </a-button>
          </a-form-item>
        </a-form>
      </template>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, reactive } from 'vue'
import { useI18n } from 'vue-i18n'
import { useChannelsStore } from '@/stores/channels'
import { useGatewayStore } from '@/stores/gateway'
import { useSettingsStore } from '@/stores/settings'
import { CHANNEL_META, CHANNEL_ICONS, getAllChannels, type ChannelType, type ChannelMeta } from '@/types/channel'
import { message } from 'ant-design-vue'
import { ReloadOutlined, PlusOutlined, DeleteOutlined } from '@ant-design/icons-vue'

const { t } = useI18n()
const channelsStore = useChannelsStore()
const gatewayStore = useGatewayStore()
const settingsStore = useSettingsStore()

const isMobile = computed(() => settingsStore.isMobile)

const showAddDialog = ref(false)
const selectedMeta = ref<ChannelMeta | null>(null)
const configValues = reactive<Record<string, string>>({})
const saving = ref(false)

const connectedCount = computed(() => channelsStore.channels.filter(c => c.status === 'connected').length)
const allChannelMeta = computed(() => getAllChannels().map(id => CHANNEL_META[id]))
const availableChannels = computed(() => {
  const configured = new Set(channelsStore.channels.map(c => c.type))
  return allChannelMeta.value.filter(m => !configured.has(m.id))
})

function getChannelIcon(type: string): string {
  return CHANNEL_ICONS[type as ChannelType] || '📡'
}

function resolveI18n(key: string): string {
  if (!key) return ''
  if (key.includes(':')) {
    // Try to resolve as i18n key
    const resolved = t(key.replace(':', '.'))
    if (resolved !== key.replace(':', '.')) return resolved
  }
  return key
}

function selectChannel(id: ChannelType) {
  selectedMeta.value = CHANNEL_META[id]
  Object.keys(configValues).forEach(k => delete configValues[k])
  showAddDialog.value = true
}

function resetDialog() {
  selectedMeta.value = null
  Object.keys(configValues).forEach(k => delete configValues[k])
}

async function handleSaveChannel() {
  if (!selectedMeta.value) return
  saving.value = true
  try {
    await channelsStore.addChannel({
      type: selectedMeta.value.id,
      name: selectedMeta.value.name,
      config: { ...configValues },
    })
    message.success(t('channels.title'))
    showAddDialog.value = false
    resetDialog()
  } catch (err) {
    message.error(String(err))
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  if (gatewayStore.status.state === 'running') {
    channelsStore.fetchChannels()
  }
})
</script>

<style scoped>
.channels-page { max-width: 100%; }
.channel-card { cursor: pointer; text-align: center; }
.channel-card-content { display: flex; flex-direction: column; align-items: center; gap: 4px; }
.channel-icon { font-size: 32px; }
.channel-name { font-weight: 500; font-size: 13px; }
.field-desc { font-size: 12px; color: var(--text-tertiary); margin: 4px 0 0; }

@media (max-width: 767px) {
  .channel-icon { font-size: 24px; }
  .channel-name { font-size: 12px; }
  .channels-page :deep(.ant-statistic-title) { font-size: 12px; }
}
</style>
