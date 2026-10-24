<template>
  <n-layout has-sider style="height: 100vh;">
    <n-layout-sider
      bordered
      collapse-mode="width"
      :collapsed-width="64"
      :width="240"
      style="background: #18181c;"
    >
      <div style="padding: 20px; font-weight: bold; font-size: 18px; color: #63e2b7; text-align: center;">
        Utopia Images
      </div>
      <n-menu
        :value="activeKey"
        :options="menuOptions"
        @update:value="handleMenuSelect"
      />
    </n-layout-sider>

    <n-layout>
      <n-layout-header bordered style="height: 60px; padding: 0 24px; display: flex; align-items: center; justify-content: space-between; background: #18181c;">
        <div style="font-weight: 500; font-size: 16px;">
          {{ currentTitle }}
        </div>
        <div style="display: flex; align-items: center; gap: 16px;">
          <!-- Language Selector -->
          <n-select
            v-model:value="currentLocale"
            :options="localeOptions"
            size="small"
            style="width: 110px;"
            @update:value="changeLocale"
          />

          <!-- User info -->
          <n-tag type="success" size="small" v-if="user">
            {{ user.nickname || user.username }} · {{ user.role }}
          </n-tag>

          <n-button size="small" quaternary @click="handleLogout">
            {{ $t('common.logout') }}
          </n-button>
        </div>
      </n-layout-header>

      <n-layout-content style="padding: 24px; background: #101014; overflow-y: auto;">
        <router-view />
      </n-layout-content>
    </n-layout>
  </n-layout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'

const router = useRouter()
const route = useRoute()
const { t, locale } = useI18n()

const user = ref<any>(null)
const currentLocale = ref(locale.value)

const localeOptions = [
  { label: '中文', value: 'zh-CN' },
  { label: '日本語', value: 'ja-JP' },
  { label: 'English', value: 'en-US' }
]

const activeKey = computed(() => route.path)

const currentTitle = computed(() => {
  if (route.path.includes('/upload')) return t('nav.upload')
  if (route.path.includes('/files')) return t('nav.files')
  if (route.path.includes('/watermarks')) return t('nav.watermarks')
  if (route.path.includes('/tokens')) return t('nav.tokens')
  if (route.path.includes('/settings')) return t('nav.settings')
  if (route.path.includes('/admin/users')) return t('nav.adminUsers')
  if (route.path.includes('/admin/policies')) return t('nav.adminPolicies')
  if (route.path.includes('/admin/settings')) return t('nav.adminSettings')
  if (route.path.includes('/admin/stats')) return t('nav.adminStats')
  return 'Utopia Images'
})

const menuOptions = computed(() => {
  const items: any[] = [
    { label: () => t('nav.upload'), key: '/console/upload' },
    { label: () => t('nav.files'), key: '/console/files' },
    { label: () => t('nav.watermarks'), key: '/console/watermarks' },
    { label: () => t('nav.tokens'), key: '/console/tokens' },
    { label: () => t('nav.settings'), key: '/console/settings' }
  ]

  if (user.value && user.value.role === 'admin') {
    items.push(
      { type: 'divider', key: 'd1' },
      { label: () => t('nav.adminUsers'), key: '/console/admin/users' },
      { label: () => t('nav.adminPolicies'), key: '/console/admin/policies' },
      { label: () => t('nav.adminSettings'), key: '/console/admin/settings' },
      { label: () => t('nav.adminStats'), key: '/console/admin/stats' }
    )
  }

  return items
})

const handleMenuSelect = (key: string) => {
  router.push(key)
}

const changeLocale = (val: string) => {
  locale.value = val
  localStorage.setItem('uimg_locale', val)
}

const handleLogout = async () => {
  try {
    await api.post('/auth/logout')
  } finally {
    router.push('/login')
  }
}

onMounted(async () => {
  try {
    const res = await api.get('/me')
    user.value = res.data
  } catch {}
})
</script>
