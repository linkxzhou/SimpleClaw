import { defineStore } from 'pinia'
import { ref, onMounted, onUnmounted } from 'vue'

type Theme = 'light' | 'dark'
type UpdateChannel = 'stable' | 'beta' | 'dev'

function getDefaultLanguage(): string {
  const lang = navigator.language.toLowerCase()
  if (lang.startsWith('zh')) return 'zh'
  if (lang.startsWith('ja')) return 'ja'
  return 'en'
}

export const useSettingsStore = defineStore('settings', () => {
  const theme = ref<Theme>('light')
  const language = ref(getDefaultLanguage())
  const startMinimized = ref(false)
  const launchAtStartup = ref(false)
  const gatewayAutoStart = ref(true)
  const gatewayPort = ref(18790)
  const updateChannel = ref<UpdateChannel>('stable')
  const autoCheckUpdate = ref(true)
  const autoDownloadUpdate = ref(false)
  const sidebarCollapsed = ref(false)
  const devModeUnlocked = ref(false)
  const setupComplete = ref(false)
  const isMobile = ref(false)

  let _mql: MediaQueryList | null = null

  function _checkMobile() {
    isMobile.value = window.innerWidth < 768
  }

  function initMobileDetect() {
    _checkMobile()
    _mql = window.matchMedia('(max-width: 767px)')
    const handler = (e: MediaQueryListEvent) => { isMobile.value = e.matches }
    _mql.addEventListener('change', handler)
    return () => _mql?.removeEventListener('change', handler)
  }

  function setTheme(val: Theme) {
    theme.value = val
    applyTheme(val)
  }

  function setLanguage(val: string) {
    language.value = val
  }

  function markSetupComplete() {
    setupComplete.value = true
  }

  function resetSettings() {
    theme.value = 'light'
    language.value = getDefaultLanguage()
    startMinimized.value = false
    launchAtStartup.value = false
    gatewayAutoStart.value = true
    gatewayPort.value = 18790
    updateChannel.value = 'stable'
    autoCheckUpdate.value = true
    autoDownloadUpdate.value = false
    sidebarCollapsed.value = false
    devModeUnlocked.value = false
    setupComplete.value = false
  }

  return {
    theme, language, startMinimized, launchAtStartup,
    gatewayAutoStart, gatewayPort,
    updateChannel, autoCheckUpdate, autoDownloadUpdate,
    sidebarCollapsed, devModeUnlocked, setupComplete,
    isMobile, initMobileDetect,
    setTheme, setLanguage, markSetupComplete, resetSettings,
  }
}, {
  persist: {
    key: 'simpleclaw-settings',
  },
})

function applyTheme(theme: Theme) {
  const root = document.documentElement
  root.classList.remove('light', 'dark')
  root.classList.add(theme)
}
