<template>
  <div class="settings-page">
    <h2>{{ t('settings.title') }}</h2>
    <p class="subtitle">{{ t('settings.subtitle') }}</p>

    <a-space direction="vertical" :size="24" style="width: 100%">
      <!-- Appearance -->
      <a-card :title="t('settings.appearance.title')">
        <p class="card-desc">{{ t('settings.appearance.description') }}</p>
        <div class="setting-item">
          <span>{{ t('settings.appearance.theme') }}</span>
          <a-radio-group
            :value="settings.theme"
            @change="(e: any) => settings.setTheme(e.target.value)"
            button-style="solid"
            size="small"
          >
            <a-radio-button value="light">{{ t('settings.appearance.light') }}</a-radio-button>
            <a-radio-button value="dark">{{ t('settings.appearance.dark') }}</a-radio-button>
          </a-radio-group>
        </div>
        <div class="setting-item">
          <span>{{ t('settings.appearance.language') }}</span>
          <a-select
            :value="settings.language"
            @change="(val: string) => { settings.setLanguage(val); locale = val }"
            style="width: 160px"
          >
            <a-select-option value="en">English</a-select-option>
            <a-select-option value="zh">中文</a-select-option>
            <a-select-option value="ja">日本語</a-select-option>
          </a-select>
        </div>
      </a-card>

      <!-- AI Providers -->
      <a-card :title="t('settings.aiProviders.title')">
        <template #extra>
          <a-button type="primary" size="small" @click="showAddProvider = true">
            <template #icon><PlusOutlined /></template>
            {{ t('settings.aiProviders.add') }}
          </a-button>
        </template>
        <p class="card-desc">{{ t('settings.aiProviders.description') }}</p>

        <a-spin :spinning="providerStore.loading">
          <a-empty
            v-if="providerStore.providers.length === 0"
            :description="t('settings.aiProviders.empty.title')"
          />
          <a-list v-else :data-source="providerStore.providers" size="small">
            <template #renderItem="{ item }">
              <a-list-item>
                <a-list-item-meta>
                  <template #avatar>
                    <span style="font-size: 24px">{{ getProviderIcon(item.type) }}</span>
                  </template>
                  <template #title>
                    <a-space>
                      {{ item.name }}
                      <a-tag v-if="item.id === providerStore.defaultProviderId" color="gold" size="small">
                        {{ t('settings.aiProviders.card.default') }}
                      </a-tag>
                    </a-space>
                  </template>
                  <template #description>
                    {{ item.hasKey ? t('settings.aiProviders.card.configured') : t('settings.aiProviders.card.noKey') }}
                    {{ item.keyMasked ? ` • ${item.keyMasked}` : '' }}
                  </template>
                </a-list-item-meta>
                <template #actions>
                  <a-button
                    v-if="item.id !== providerStore.defaultProviderId"
                    size="small"
                    @click="providerStore.setDefaultProvider(item.id)"
                  >⭐</a-button>
                  <a-popconfirm
                    :title="t('settings.aiProviders.card.delete') + '?'"
                    @confirm="providerStore.deleteProvider(item.id)"
                  >
                    <a-button danger size="small">
                      <template #icon><DeleteOutlined /></template>
                    </a-button>
                  </a-popconfirm>
                </template>
              </a-list-item>
            </template>
          </a-list>
        </a-spin>
      </a-card>

      <!-- Gateway -->
      <a-card :title="t('settings.gateway.title')">
        <p class="card-desc">{{ t('settings.gateway.description') }}</p>
        <div class="setting-item">
          <span>{{ t('settings.gateway.status') }}</span>
          <a-badge
            :status="gatewayStore.status.state === 'running' ? 'success' : 'default'"
            :text="gatewayStore.status.state"
          />
        </div>
        <div class="setting-item">
          <span>{{ t('settings.gateway.port') }}</span>
          <a-input-number v-model:value="settings.gatewayPort" :min="1024" :max="65535" style="width: 120px" />
        </div>
        <div class="setting-item">
          <span>{{ t('settings.gateway.autoStart') }}</span>
          <a-switch v-model:checked="settings.gatewayAutoStart" />
        </div>
      </a-card>

      <!-- Advanced -->
      <a-card :title="t('settings.advanced.title')">
        <p class="card-desc">{{ t('settings.advanced.description') }}</p>
        <div class="setting-item">
          <div>
            <div>{{ t('settings.advanced.devMode') }}</div>
            <div class="setting-desc">{{ t('settings.advanced.devModeDesc') }}</div>
          </div>
          <a-switch v-model:checked="settings.devModeUnlocked" />
        </div>
      </a-card>

      <!-- About -->
      <a-card :title="t('settings.about.title')">
        <div class="about-section">
          <h3>🤖 {{ t('settings.about.appName') }}</h3>
          <p>{{ t('settings.about.tagline') }}</p>
        </div>
      </a-card>
    </a-space>

    <!-- Add Provider Dialog -->
    <a-modal
      v-model:open="showAddProvider"
      :title="t('settings.aiProviders.dialog.title')"
      @ok="handleAddProvider"
      :confirm-loading="addingProvider"
    >
      <a-form layout="vertical">
        <a-form-item label="Provider Type" required>
          <a-select v-model:value="newProvider.type" @change="onProviderTypeChange">
            <a-select-option v-for="info in PROVIDER_TYPE_INFO" :key="info.id" :value="info.id">
              {{ info.icon }} {{ info.name }}
            </a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item :label="t('settings.aiProviders.dialog.displayName')" required>
          <a-input v-model:value="newProvider.name" />
        </a-form-item>
        <a-form-item v-if="selectedProviderInfo?.requiresApiKey" :label="t('settings.aiProviders.dialog.apiKey')">
          <a-input-password v-model:value="newProvider.apiKey" :placeholder="selectedProviderInfo?.placeholder" />
          <div class="setting-desc">{{ t('settings.aiProviders.dialog.apiKeyStored') }}</div>
        </a-form-item>
        <a-form-item v-if="selectedProviderInfo?.showBaseUrl || selectedProviderInfo?.defaultBaseUrl" :label="t('settings.aiProviders.dialog.baseUrl')">
          <a-input v-model:value="newProvider.baseUrl" />
        </a-form-item>
        <a-form-item v-if="selectedProviderInfo?.showModelId || selectedProviderInfo?.defaultModelId" :label="t('settings.aiProviders.dialog.modelId')">
          <a-input v-model:value="newProvider.modelId" :placeholder="selectedProviderInfo?.modelIdPlaceholder" />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useSettingsStore } from '@/stores/settings'
import { useGatewayStore } from '@/stores/gateway'
import { useProviderStore } from '@/stores/providers'
import { PROVIDER_TYPE_INFO, getProviderTypeInfo, type ProviderType } from '@/lib/providers'
import { message } from 'ant-design-vue'
import { PlusOutlined, DeleteOutlined } from '@ant-design/icons-vue'

const { t, locale } = useI18n()
const settings = useSettingsStore()
const gatewayStore = useGatewayStore()
const providerStore = useProviderStore()

const showAddProvider = ref(false)
const addingProvider = ref(false)

const newProvider = reactive({
  type: 'openai' as ProviderType,
  name: 'OpenAI',
  apiKey: '',
  baseUrl: '',
  modelId: '',
})

const selectedProviderInfo = computed(() => getProviderTypeInfo(newProvider.type))

function getProviderIcon(type: string): string {
  const info = PROVIDER_TYPE_INFO.find(p => p.id === type)
  return info?.icon || '⚙️'
}

function onProviderTypeChange(type: ProviderType) {
  const info = getProviderTypeInfo(type)
  if (info) {
    newProvider.name = info.name
    newProvider.baseUrl = info.defaultBaseUrl || ''
    newProvider.modelId = info.defaultModelId || ''
    newProvider.apiKey = ''
  }
}

async function handleAddProvider() {
  if (!newProvider.name.trim()) return
  addingProvider.value = true
  try {
    await providerStore.addProvider({
      name: newProvider.name,
      type: newProvider.type,
      baseUrl: newProvider.baseUrl || undefined,
      model: newProvider.modelId || undefined,
      enabled: true,
    }, newProvider.apiKey || undefined)
    message.success(t('settings.aiProviders.toast.added'))
    showAddProvider.value = false
  } catch (err) {
    message.error(String(err))
  } finally {
    addingProvider.value = false
  }
}

onMounted(() => {
  providerStore.fetchProviders()
})
</script>

<style scoped>
.settings-page { max-width: 100%; }
.subtitle { color: var(--text-secondary); margin-bottom: 24px; }
.card-desc { color: var(--text-tertiary); margin-bottom: 16px; }
.setting-item {
  display: flex; justify-content: space-between; align-items: center;
  padding: 8px 0; border-bottom: 1px solid var(--border-color); gap: 12px;
}
.setting-item:last-child { border-bottom: none; }
.setting-desc { font-size: 12px; color: var(--text-tertiary); margin-top: 2px; }
.about-section { text-align: center; padding: 16px; }
.about-section h3 { color: var(--accent); }

@media (max-width: 767px) {
  .setting-item { flex-direction: column; align-items: flex-start; gap: 8px; }
  .setting-item :deep(.ant-radio-group) { align-self: flex-start; }
  .setting-item :deep(.ant-select) { width: 100% !important; }
  .setting-item :deep(.ant-input-number) { width: 100% !important; }
}
</style>
