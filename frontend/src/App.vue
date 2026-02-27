<template>
  <a-config-provider :locale="antLocale" :theme="antTheme">
    <router-view />
  </a-config-provider>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, watch, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useSettingsStore } from '@/stores/settings'
import { useGatewayStore } from '@/stores/gateway'
import { theme } from 'ant-design-vue'
import enUS from 'ant-design-vue/es/locale/en_US'
import zhCN from 'ant-design-vue/es/locale/zh_CN'
import jaJP from 'ant-design-vue/es/locale/ja_JP'

const router = useRouter()
const { locale } = useI18n()
const settings = useSettingsStore()
const gateway = useGatewayStore()

let cleanupMobile: (() => void) | undefined

const antLocale = computed(() => {
  switch (settings.language) {
    case 'zh': return zhCN
    case 'ja': return jaJP
    default: return enUS
  }
})

const antTheme = computed(() => ({
  algorithm: settings.theme === 'dark' ? theme.darkAlgorithm : theme.defaultAlgorithm,
}))

// Sync i18n locale with settings
watch(() => settings.language, (lang) => {
  locale.value = lang
}, { immediate: true })

// Apply theme class to html
watch(() => settings.theme, (t) => {
  const root = document.documentElement
  root.classList.remove('light', 'dark')
  root.classList.add(t)
}, { immediate: true })

// Initialize gateway + mobile detection
onMounted(async () => {
  cleanupMobile = settings.initMobileDetect()
  await gateway.init()

  // Redirect to setup if not complete
  if (!settings.setupComplete && !window.location.pathname.startsWith('/setup')) {
    router.push('/setup')
  }
})

onUnmounted(() => {
  cleanupMobile?.()
})
</script>

<style>
* { box-sizing: border-box; }

/* ===== Accent & shared ===== */
html {
  --accent: #00b8d4;
  --accent-light: #00e5ff;
  --accent-dim: rgba(0, 184, 212, 0.15);
  --accent-glow: 0 0 12px rgba(0, 184, 212, 0.35);
  --glass-blur: blur(12px);
  --radius-sm: 6px;
  --radius-md: 10px;
  --radius-lg: 16px;
}

/* ===== Light theme (default) ===== */
html, html.light {
  --bg-primary: #f8fafc;
  --bg-secondary: #f0f4f8;
  --bg-tertiary: #e8edf2;
  --bg-hover: rgba(0, 184, 212, 0.06);
  --text-primary: #1a2332;
  --text-secondary: #5a6a7e;
  --text-tertiary: #8a96a6;
  --border-color: rgba(0, 184, 212, 0.12);
  --border-color-strong: rgba(0, 184, 212, 0.25);
  --shadow-color: rgba(0, 30, 60, 0.08);
  --assistant-bubble-bg: #eef4f8;
  --assistant-bubble-text: #1a2332;
  --system-bubble-bg: #e6f7ff;
  --system-bubble-text: #5a6a7e;
  --tool-status-bg: #f0f7fa;
  --code-bg: rgba(0, 40, 80, 0.05);
  --input-bg: #ffffff;
  --card-bg: #ffffff;
  --glass-bg: rgba(255, 255, 255, 0.72);
  --glass-border: rgba(0, 184, 212, 0.15);
  --sidebar-bg: rgba(248, 250, 252, 0.9);
  --user-bubble-bg: linear-gradient(135deg, #00b8d4, #0091ea);
}

/* ===== Dark theme ===== */
html.dark {
  --bg-primary: #0a0f1a;
  --bg-secondary: #111827;
  --bg-tertiary: #1a2236;
  --bg-hover: rgba(0, 229, 255, 0.08);
  --text-primary: rgba(255, 255, 255, 0.92);
  --text-secondary: #7a8ba0;
  --text-tertiary: #4a5568;
  --border-color: rgba(0, 229, 255, 0.1);
  --border-color-strong: rgba(0, 229, 255, 0.2);
  --shadow-color: rgba(0, 0, 0, 0.4);
  --assistant-bubble-bg: rgba(26, 34, 54, 0.8);
  --assistant-bubble-text: rgba(255, 255, 255, 0.92);
  --system-bubble-bg: rgba(0, 184, 212, 0.08);
  --system-bubble-text: #7a8ba0;
  --tool-status-bg: rgba(17, 24, 39, 0.8);
  --code-bg: rgba(0, 229, 255, 0.06);
  --input-bg: rgba(17, 24, 39, 0.9);
  --card-bg: rgba(17, 24, 39, 0.6);
  --glass-bg: rgba(10, 15, 26, 0.75);
  --glass-border: rgba(0, 229, 255, 0.12);
  --sidebar-bg: rgba(10, 15, 26, 0.92);
  --user-bubble-bg: linear-gradient(135deg, #0091ea, #00b8d4);
}

html, body {
  margin: 0;
  padding: 0;
  height: 100%;
  width: 100%;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  -webkit-text-size-adjust: 100%;
  overscroll-behavior: none;
  background: var(--bg-primary);
  color: var(--text-primary);
  transition: background-color 0.3s, color 0.3s;
}
#app {
  height: 100%;
  width: 100%;
  overflow: hidden;
}

/* ===== Global page utility classes ===== */
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 16px;
}
.subtitle {
  color: var(--text-secondary);
  margin: 0;
}

/* Mobile global overrides */
@media (max-width: 767px) {
  .ant-card { border-radius: 8px !important; }
  .ant-modal { max-width: calc(100vw - 32px) !important; margin: 16px auto !important; }
  .ant-modal .ant-modal-content { border-radius: 12px !important; }
  .ant-drawer-content-wrapper { max-width: 85vw !important; }
  .ant-list-item-action { flex-wrap: wrap; gap: 4px; }
  .page-header { flex-direction: column !important; gap: 12px !important; }
  .page-header > div:last-child,
  .page-header > .ant-space { align-self: flex-start !important; }
}
</style>
