<template>
  <n-layout :has-sider="!isMobile" style="height: 100vh;">
    <!-- Desktop Sidebar -->
    <n-layout-sider
      v-if="!isMobile"
      bordered
      collapse-mode="width"
      :collapsed-width="64"
      :width="240"
      style="background: #18181c;"
      class="sidebar-layout"
    >
      <div style="padding: 20px 16px; font-weight: bold; font-size: 18px; color: #63e2b7; text-align: center; letter-spacing: 0.5px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
        {{ siteInfo.site_name || 'Utopia Images' }}
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

    <!-- Mobile Navigation Drawer -->
    <n-drawer
      v-if="isMobile"
      v-model:show="showMobileDrawer"
      placement="left"
      :width="260"
      style="background: #18181c;"
    >
      <n-drawer-content :body-content-style="{ padding: '0', display: 'flex', flexDirection: 'column', height: '100%', background: '#18181c' }">
        <div style="padding: 18px 16px; font-weight: bold; font-size: 18px; color: #63e2b7; text-align: center; border-bottom: 1px solid rgba(255,255,255,0.06); overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
          {{ siteInfo.site_name || 'Utopia Images' }}
        </div>
        <div style="flex: 1; overflow-y: auto;">
          <n-menu
            :value="activeKey"
            :options="menuOptions"
            @update:value="(key: string) => { handleMenuSelect(key); showMobileDrawer = false; }"
          />
        </div>
        <!-- Storage Capacity Display inside drawer -->
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
      </n-drawer-content>
    </n-drawer>

    <!-- Main Content Area -->
    <n-layout>
      <n-layout-header
        bordered
        :style="{
          height: '60px',
          padding: isMobile ? '0 12px' : '0 24px',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          background: '#18181c'
        }"
      >
        <div style="display: flex; align-items: center; gap: 10px; min-width: 0;">
          <!-- Mobile Menu Toggle Button -->
          <n-button v-if="isMobile" quaternary circle size="medium" @click="showMobileDrawer = true">
            <template #icon>
              <n-icon size="20">
                <MenuOutline />
              </n-icon>
            </template>
          </n-button>
          <div style="font-weight: 500; font-size: 16px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
            {{ currentTitle }}
          </div>
        </div>

        <div style="display: flex; align-items: center; gap: 8px;">
          <!-- Language Selector -->
          <n-select
            v-model:value="currentLocale"
            :options="localeOptions"
            size="small"
            :style="{ width: isMobile ? '86px' : '110px' }"
            @update:value="changeLocale"
          />

          <!-- User info -->
          <n-tag type="success" size="small" v-if="user && !isMobile">
            {{ user.nickname || user.username }} · {{ user.role?.display_name || user.role?.name || user.role }}
          </n-tag>
          <n-tag type="success" size="small" v-else-if="user && isMobile">
            {{ user.nickname || user.username }}
          </n-tag>

          <n-button size="small" quaternary @click="handleLogout">
            {{ $t('common.logout') }}
          </n-button>
        </div>
      </n-layout-header>

      <!-- Main Scroll Container with Permanent Footer inside n-layout-scroll-container -->
      <n-layout-content
        :content-style="{
          minHeight: '100%',
          display: 'flex',
          flexDirection: 'column',
          justifyContent: 'space-between',
          padding: isMobile ? '14px 12px' : '24px',
          background: '#101014'
        }"
      >
        <div style="flex: 1 0 auto; width: 100%;">
          <router-view />
        </div>

        <!-- Permanent Footer inside n-layout-scroll-container -->
        <footer class="app-footer">
          <span>
            Powered by
            <a
              href="https://github.com/UtopiaXC/UtopiaImages"
              target="_blank"
              rel="noopener noreferrer"
              class="footer-link"
            >
              Utopia Images
            </a>
          </span>
        </footer>
      </n-layout-content>
    </n-layout>
  </n-layout>
</template>

<script setup lang="ts">
import { ref, computed, h, watch, onMounted, onBeforeUnmount } from 'vue'
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
  StatsChartOutline,
  MenuOutline,
  FolderOpenOutline
} from '@vicons/ionicons5'
import { api } from '../api/client'
import { siteInfo } from '../utils/site'
import { clearCurrentUser, currentUser } from '../utils/auth'

const router = useRouter()
const route = useRoute()
const { t, locale } = useI18n()

const user = ref<any>(null)
const quotaInfo = ref<any>(null)
const currentLocale = ref(locale.value)

// Mobile responsiveness state
const isMobile = ref(false)
const showMobileDrawer = ref(false)

const handleResize = () => {
  isMobile.value = window.innerWidth <= 768
}

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
  if (route.path.includes('/admin/files')) return t('nav.adminFiles')
  if (route.path.includes('/admin/users')) return t('nav.adminUsers')
  if (route.path.includes('/admin/roles')) return t('nav.adminRoles')
  if (route.path.includes('/admin/policies')) return t('nav.adminPolicies')
  if (route.path.includes('/admin/settings')) return t('nav.adminSettings')
  if (route.path.includes('/admin/stats')) return t('nav.adminStats')
  return siteInfo.value.site_name || 'Utopia Images'
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
      { label: () => t('nav.adminFiles'), key: '/console/admin/files', icon: renderMenuIcon(FolderOpenOutline) },
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
    clearCurrentUser()
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

watch(() => route.path, () => {
  loadUserData()
})

onMounted(() => {
  handleResize()
  window.addEventListener('resize', handleResize)
  window.addEventListener('refresh-quota', loadUserData)
  loadUserData()
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleResize)
  window.removeEventListener('refresh-quota', loadUserData)
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

.app-footer {
  margin-top: 36px;
  padding: 18px 0 8px;
  text-align: center;
  font-size: 13px;
  color: #777;
  border-top: 1px solid rgba(255, 255, 255, 0.05);
}

.footer-link {
  color: #63e2b7;
  text-decoration: none;
  font-weight: 500;
  transition: color 0.2s ease;
}

.footer-link:hover {
  color: #7fe7c4;
  text-decoration: underline;
}
</style>
