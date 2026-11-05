import { createI18n } from 'vue-i18n'
import zhCN from './locales/zh-CN.json'
import jaJP from './locales/ja-JP.json'
import enUS from './locales/en-US.json'

const savedLocale = localStorage.getItem('uimg_locale') || 'zh-CN'

export const i18n = createI18n({
  legacy: false,
  locale: savedLocale,
  fallbackLocale: 'zh-CN',
  messages: {
    'zh-CN': zhCN,
    'ja-JP': jaJP,
    'en-US': enUS,
  },
})
