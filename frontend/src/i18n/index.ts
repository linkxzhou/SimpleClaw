import { createI18n } from 'vue-i18n'
import en from './locales/en'
import zh from './locales/zh'
import ja from './locales/ja'

function getDefaultLanguage(): string {
  const lang = navigator.language.toLowerCase()
  if (lang.startsWith('zh')) return 'zh'
  if (lang.startsWith('ja')) return 'ja'
  return 'en'
}

const i18n = createI18n({
  legacy: false,
  locale: getDefaultLanguage(),
  fallbackLocale: 'en',
  messages: { en, zh, ja },
})

export default i18n
