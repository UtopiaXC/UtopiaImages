import { ref } from 'vue'
import { api } from '../api/client'

export interface SiteInfo {
  site_name: string
  description?: string
  version?: string
  base_url?: string
  base_urls?: string[]
  allow_registration?: boolean
  captcha_provider?: string
  favicon?: string
}

export const siteInfo = ref<SiteInfo>({
  site_name: 'Utopia Images',
  description: '',
})

export const siteLoaded = ref(false)

export async function fetchSiteInfo(force = false): Promise<SiteInfo> {
  if (siteLoaded.value && !force) {
    return siteInfo.value
  }
  try {
    const res = await api.get('/site')
    if (res.data) {
      siteInfo.value = {
        ...siteInfo.value,
        ...res.data,
      }
      siteLoaded.value = true
    }
  } catch (err) {
    // Keep default site info on error
  }
  return siteInfo.value
}

export function setSiteInfo(info: Partial<SiteInfo>) {
  siteInfo.value = {
    ...siteInfo.value,
    ...info,
  }
}

export function updateDocumentTitle(pageTitle?: string) {
  const name = siteInfo.value.site_name || 'Utopia Images'
  if (pageTitle && pageTitle.trim() && pageTitle.trim() !== name) {
    document.title = `${pageTitle.trim()} - ${name}`
  } else {
    document.title = name
  }
}
