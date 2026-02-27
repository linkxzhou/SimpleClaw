<template>
  <div class="setup-page">
    <!-- Progress steps -->
    <a-steps :current="currentStep" :direction="isMobile ? 'vertical' : 'horizontal'" :size="isMobile ? 'small' : 'default'" style="max-width: 600px; margin: 0 auto 40px">
      <a-step :title="t('setup.steps.welcome.title')" />
      <a-step :title="t('setup.steps.provider.title')" />
      <a-step :title="t('setup.steps.channel.title')" />
      <a-step :title="t('setup.steps.complete.title')" />
    </a-steps>

    <div class="setup-content">
      <!-- Step 0: Welcome -->
      <div v-if="currentStep === 0" class="step-welcome">
        <div class="welcome-logo">🤖</div>
        <h1>{{ t('setup.welcome.title') }}</h1>
        <p class="welcome-desc">{{ t('setup.welcome.description') }}</p>

        <a-row :gutter="[16, 16]" style="max-width: 500px; margin: 24px auto">
          <a-col :xs="12" :sm="12">
            <a-card size="small">⚡ {{ t('setup.welcome.features.lightweight') }}</a-card>
          </a-col>
          <a-col :xs="12" :sm="12">
            <a-card size="small">🎨 {{ t('setup.welcome.features.modernUI') }}</a-card>
          </a-col>
          <a-col :xs="12" :sm="12">
            <a-card size="small">🌐 {{ t('setup.welcome.features.crossPlatform') }}</a-card>
          </a-col>
          <a-col :xs="12" :sm="12">
            <a-card size="small">🧩 {{ t('setup.welcome.features.extensible') }}</a-card>
          </a-col>
        </a-row>

        <div style="margin-top: 16px">
          <span>{{ t('settings.appearance.language') }}: </span>
          <a-select
            :value="settings.language"
            @change="(val: string) => { settings.setLanguage(val); locale = val }"
            style="width: 140px"
          >
            <a-select-option value="en">English</a-select-option>
            <a-select-option value="zh">中文</a-select-option>
            <a-select-option value="ja">日本語</a-select-option>
          </a-select>
        </div>
      </div>

      <!-- Step 1: Provider -->
      <div v-if="currentStep === 1" class="step-provider">
        <h2>{{ t('setup.provider.label') }}</h2>
        <a-form layout="vertical" style="max-width: 500px; margin: 0 auto">
          <a-form-item :label="t('setup.provider.label')">
            <a-select v-model:value="providerType" :placeholder="t('setup.provider.selectPlaceholder')" @change="onSetupProviderChange">
              <a-select-option v-for="info in PROVIDER_TYPE_INFO" :key="info.id" :value="info.id">
                {{ info.icon }} {{ info.name }} {{ info.model ? `(${info.model})` : '' }}
              </a-select-option>
            </a-select>
          </a-form-item>

          <template v-if="providerType">
            <a-form-item v-if="setupProviderInfo?.requiresApiKey" :label="t('setup.provider.apiKey')">
              <a-input-password v-model:value="providerApiKey" :placeholder="setupProviderInfo?.placeholder" />
              <div class="field-hint">{{ t('setup.provider.storedLocally') }}</div>
            </a-form-item>
            <a-form-item v-if="setupProviderInfo?.showBaseUrl || setupProviderInfo?.defaultBaseUrl" :label="t('setup.provider.baseUrl')">
              <a-input v-model:value="providerBaseUrl" />
            </a-form-item>
            <a-form-item v-if="setupProviderInfo?.showModelId || setupProviderInfo?.defaultModelId" :label="t('setup.provider.modelId')">
              <a-input v-model:value="providerModelId" :placeholder="setupProviderInfo?.modelIdPlaceholder" />
            </a-form-item>

            <a-alert v-if="providerSaved" type="success" :message="t('setup.provider.valid')" show-icon style="margin-bottom: 16px" />

            <a-button type="primary" :loading="savingProvider" @click="handleSaveProvider" block>
              {{ t('setup.provider.validateSave') }}
            </a-button>
          </template>
        </a-form>
      </div>

      <!-- Step 2: Channel -->
      <div v-if="currentStep === 2" class="step-channel">
        <h2>{{ t('setup.channel.title') }}</h2>
        <p>{{ t('setup.channel.subtitle') }}</p>
        <a-row :gutter="[16, 16]" style="max-width: 600px; margin: 0 auto">
          <a-col v-for="type in primaryChannels" :key="type" :xs="24" :sm="12">
            <a-card
              hoverable
              :class="{ 'channel-selected': selectedChannel === type }"
              @click="selectedChannel = type"
            >
              <div class="channel-option">
                <span class="channel-icon">{{ CHANNEL_ICONS[type] }}</span>
                <span class="channel-label">{{ CHANNEL_NAMES[type] }}</span>
              </div>
            </a-card>
          </a-col>
        </a-row>

        <div v-if="channelConfigured" style="margin-top: 16px; text-align: center">
          <a-alert type="success" :message="t('setup.channel.connectedDesc')" show-icon />
        </div>
      </div>

      <!-- Step 3: Complete -->
      <div v-if="currentStep === 3" class="step-complete">
        <a-result
          status="success"
          :title="t('setup.complete.title')"
          :sub-title="t('setup.complete.subtitle')"
        >
          <template #extra>
            <a-descriptions :column="1" bordered style="max-width: 400px; margin: 0 auto 24px">
              <a-descriptions-item :label="t('setup.complete.provider')">
                {{ providerType ? PROVIDER_TYPE_INFO.find(p => p.id === providerType)?.name : '-' }}
              </a-descriptions-item>
              <a-descriptions-item :label="t('setup.complete.gateway')">
                <a-badge
                  :status="gatewayStore.status.state === 'running' ? 'success' : 'default'"
                  :text="gatewayStore.status.state === 'running' ? t('setup.complete.running') : gatewayStore.status.state"
                />
              </a-descriptions-item>
            </a-descriptions>
            <p style="color: #999">{{ t('setup.complete.footer') }}</p>
          </template>
        </a-result>
      </div>
    </div>

    <!-- Navigation -->
    <div class="setup-nav">
      <a-button v-if="currentStep > 0 && currentStep < 3" @click="currentStep--">
        {{ t('setup.nav.back') }}
      </a-button>
      <div v-else />

      <a-space>
        <a-button v-if="currentStep < 3 && currentStep > 0" @click="handleSkip">
          {{ t('setup.nav.skipStep') }}
        </a-button>
        <a-button v-if="currentStep === 0" @click="handleSkipSetup" type="text">
          {{ t('setup.nav.skipSetup') }}
        </a-button>
        <a-button
          type="primary"
          @click="handleNext"
        >
          {{ currentStep === 3 ? t('setup.nav.getStarted') : t('setup.nav.next') }}
        </a-button>
      </a-space>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useSettingsStore } from '@/stores/settings'
import { useGatewayStore } from '@/stores/gateway'
import { useProviderStore } from '@/stores/providers'
import { PROVIDER_TYPE_INFO, getProviderTypeInfo, type ProviderType } from '@/lib/providers'
import { CHANNEL_ICONS, CHANNEL_NAMES, getPrimaryChannels } from '@/types/channel'
import { message } from 'ant-design-vue'

const { t, locale } = useI18n()
const router = useRouter()
const settings = useSettingsStore()
const gatewayStore = useGatewayStore()
const providerStore = useProviderStore()

const isMobile = computed(() => settings.isMobile)

const currentStep = ref(0)
const providerType = ref<ProviderType | ''>('')
const providerApiKey = ref('')
const providerBaseUrl = ref('')
const providerModelId = ref('')
const providerSaved = ref(false)
const savingProvider = ref(false)
const selectedChannel = ref('')
const channelConfigured = ref(false)
const primaryChannels = getPrimaryChannels()

const setupProviderInfo = computed(() => providerType.value ? getProviderTypeInfo(providerType.value as ProviderType) : null)

function onSetupProviderChange(type: ProviderType) {
  const info = getProviderTypeInfo(type)
  if (info) {
    providerBaseUrl.value = info.defaultBaseUrl || ''
    providerModelId.value = info.defaultModelId || ''
    providerApiKey.value = ''
    providerSaved.value = false
  }
}

async function handleSaveProvider() {
  if (!providerType.value) return
  savingProvider.value = true
  try {
    const info = getProviderTypeInfo(providerType.value as ProviderType)
    await providerStore.addProvider({
      name: info?.name || providerType.value,
      type: providerType.value,
      baseUrl: providerBaseUrl.value || undefined,
      model: providerModelId.value || undefined,
      enabled: true,
    }, providerApiKey.value || undefined)
    providerSaved.value = true
    message.success(t('setup.provider.valid'))
  } catch (err) {
    message.error(String(err))
  } finally {
    savingProvider.value = false
  }
}

function handleNext() {
  if (currentStep.value === 3) {
    settings.markSetupComplete()
    router.push('/')
    return
  }
  currentStep.value++
}

function handleSkip() {
  currentStep.value++
}

function handleSkipSetup() {
  settings.markSetupComplete()
  router.push('/')
}
</script>

<style scoped>
.setup-page {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  padding: 40px;
  max-width: 800px;
  margin: 0 auto;
}
.setup-content {
  flex: 1;
  display: flex;
  justify-content: center;
  align-items: flex-start;
}
.setup-content > div {
  width: 100%;
  text-align: center;
}
.step-welcome .welcome-logo {
  font-size: 64px;
  margin-bottom: 16px;
}
.welcome-desc {
  color: var(--text-secondary);
  font-size: 16px;
  max-width: 500px;
  margin: 0 auto;
}
.setup-nav {
  display: flex;
  justify-content: space-between;
  padding-top: 24px;
  border-top: 1px solid var(--border-color);
  margin-top: 24px;
}
.channel-option {
  display: flex;
  align-items: center;
  gap: 12px;
}
.channel-icon {
  font-size: 32px;
}
.channel-label {
  font-weight: 500;
  font-size: 16px;
}
.channel-selected {
  border-color: var(--accent);
  box-shadow: 0 0 0 2px var(--accent-dim);
}
.field-hint {
  font-size: 12px;
  color: var(--text-tertiary);
  margin-top: 4px;
}

@media (max-width: 767px) {
  .setup-page {
    padding: 20px 16px;
  }
  .step-welcome .welcome-logo {
    font-size: 48px;
  }
  .welcome-desc {
    font-size: 14px;
  }
  .setup-nav {
    flex-wrap: wrap;
    gap: 12px;
  }
  .channel-icon {
    font-size: 24px;
  }
  .channel-label {
    font-size: 14px;
  }
}
</style>
