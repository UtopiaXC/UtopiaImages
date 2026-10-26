<template>
  <n-layout has-sider style="height: 100vh;">
    <n-layout-sider
      bordered
      collapse-mode="width"
      :collapsed-width="64"
      :width="240"
      style="background: #18181c;"
      class="sidebar-layout"
    >
      <div style="padding: 20px 16px; font-weight: bold; font-size: 18px; color: #63e2b7; text-align: center; letter-spacing: 0.5px;">
        Utopia Images
      </div>
      <div style="flex: 1; overflow-y: auto;">
        <n-menu
          :value="activeKey"
          :options="menuOptions"
          @update:value="handleMenuSelect"
        />
      </div>

      <!-- Storage Capacity Display at bottom of sider -->
      <div v-if="quotaInfo" class="sider-quota-container">
        <div style="display: flex; justify-content: space-between; align-items: center; font-size: 12px; margin-bottom: 6px;">
          <span style="color: #aaa;">{{ $t('sidebar.storageCapacity') }}</span>
          <span style="font-weight: 500; color: #eee; font-size: 11px;">
            {{ formatBytes(quotaInfo.used_bytes) }} / {{ quotaInfo.quota_bytes === 0 ? $t('common.unlimited') : formatBytes(quotaInfo.quota_bytes) }}
          </span>
        </div>
        <n-progress
          type="line"
          :percentage="quotaPercentage"
          :status="quotaPercentage >= 90 ? 'error' : quotaPercentage >= 75 ? 'warning' : 'success'"
          :show-indicator="false"
          style="margin-bottom: 6px;"
        />
        <div style="display: flex; justify-content: space-between; font-size: 11px; color: #777;">
          <span>{{ quotaPercentage }}%</span>
          <span v-if="quotaInfo.role_name" style="text-transform: capitalize;">{{ quotaInfo.role_name }}</span>
        </div>
      </div>
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
            {{ user.nickname || user.username }} · {{ user.role?.display_name || user.role?.name || user.role }}
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
import { ref, computed, h, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import {
  CloudUploadOutline,
  ImagesOutline,
  ColorWandOutline,
  KeyOutline,
  PersonOutline,
  PeopleOutline,
  ShieldCheckmarkOutline,
  ServerOutline,
  CogOutline,
  StatsChartOutline
} from '@vicons/ionicons5'
import { api } from '../api/client'

const router = useRouter()
const route = useRoute()
const { t, locale } = useI18n()

const user = ref<any>(null)
const quotaInfo = ref<any>(null)
const currentLocale = ref(locale.value)

const renderMenuIcon = (icon: any) => {
  return () => h(NIcon, null, { default: () => h(icon) })
}

const localeOptions = [
  { label: '中文', value: 'zh-CN' },
  { label: 'English', value: 'en-US' },
  { label: '日本語', value: 'ja-JP' }
]

const activeKey = computed(() => route.path)

const currentTitle = computed(() => {
  if (route.path.includes('/upload')) return t('nav.upload')
  if (route.path.includes('/files')) return t('nav.files')
  if (route.path.includes('/watermarks')) return t('nav.watermarks')
  if (route.path.includes('/tokens')) return t('nav.tokens')
  if (route.path.includes('/settings')) return t('nav.settings')
  if (route.path.includes('/admin/users')) return t('nav.adminUsers')
  if (route.path.includes('/admin/roles')) return t('nav.adminRoles')
  if (route.path.includes('/admin/policies')) return t('nav.adminPolicies')
  if (route.path.includes('/admin/settings')) return t('nav.adminSettings')
  if (route.path.includes('/admin/stats')) return t('nav.adminStats')
  return 'Utopia Images'
})

const isAdmin = computed(() => {
  if (!user.value) return false
  return user.value.role_id === 1 ||
    user.value.role === 'admin' ||
    (user.value.role && user.value.role.name === 'admin')
})

const menuOptions = computed(() => {
  const items: any[] = [
    { label: () => t('nav.upload'), key: '/console/upload', icon: renderMenuIcon(CloudUploadOutline) },
    { label: () => t('nav.files'), key: '/console/files', icon: renderMenuIcon(ImagesOutline) },
    { label: () => t('nav.watermarks'), key: '/console/watermarks', icon: renderMenuIcon(ColorWandOutline) },
    { label: () => t('nav.tokens'), key: '/console/tokens', icon: renderMenuIcon(KeyOutline) },
    { label: () => t('nav.settings'), key: '/console/settings', icon: renderMenuIcon(PersonOutline) }
  ]

  if (isAdmin.value) {
    items.push(
      { type: 'divider', key: 'd1' },
      { label: () => t('nav.adminUsers'), key: '/console/admin/users', icon: renderMenuIcon(PeopleOutline) },
      { label: () => t('nav.adminRoles'), key: '/console/admin/roles', icon: renderMenuIcon(ShieldCheckmarkOutline) },
      { label: () => t('nav.adminPolicies'), key: '/console/admin/policies', icon: renderMenuIcon(ServerOutline) },
      { label: () => t('nav.adminSettings'), key: '/console/admin/settings', icon: renderMenuIcon(CogOutline) },
      { label: () => t('nav.adminStats'), key: '/console/admin/stats', icon: renderMenuIcon(StatsChartOutline) }
    )
  }

  return items
})

const quotaPercentage = computed(() => {
  if (!quotaInfo.value || quotaInfo.value.quota_bytes === 0) return 0
  const pct = Math.round((quotaInfo.value.used_bytes / quotaInfo.value.quota_bytes) * 100)
  return Math.min(pct, 100)
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

const formatBytes = (bytes: number) => {
  if (!bytes || bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

const loadUserData = async () => {
  try {
    const [uRes, qRes] = await Promise.all([
      api.get('/me'),
      api.get('/me/quota')
    ])
    user.value = uRes.data
    quotaInfo.value = qRes.data
  } catch {}
}

onMounted(() => {
  loadUserData()
})
</script>

<style scoped>
:deep(.n-layout-sider-scroll-container) {
  display: flex !important;
  flex-direction: column !important;
  height: 100% !important;
}

.sider-quota-container {
  padding: 14px 16px;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(0, 0, 0, 0.2);
}
</style>
