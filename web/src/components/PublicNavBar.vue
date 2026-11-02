<template>
  <header class="public-nav-header">
    <div class="public-nav-container">
      <!-- Left: Logo & Site Name & Nav Links -->
      <div style="display: flex; align-items: center; gap: 24px;">
        <router-link to="/" class="public-nav-brand">
          <img v-if="siteInfo.favicon && !siteInfo.favicon.startsWith('<svg')" :src="siteInfo.favicon" class="public-nav-logo" />
          <span class="public-nav-title">{{ siteInfo.site_name || 'Utopia Images' }}</span>
        </router-link>

        <nav class="public-nav-links">
          <router-link
            v-if="siteInfo.enable_guest_upload"
            to="/usr/upload"
            class="public-nav-link"
            :class="{ active: currentPath === '/usr/upload' || currentPath === '/' }"
          >
            {{ $t('guest.uploadTitle') }}
          </router-link>

          <router-link
            v-if="siteInfo.enable_square"
            to="/usr/square"
            class="public-nav-link"
            :class="{ active: currentPath === '/usr/square' }"
          >
            {{ $t('square.title') }}
          </router-link>

          <router-link
            v-if="siteInfo.enable_guest_upload"
            to="/usr/gallery"
            class="public-nav-link"
            :class="{ active: currentPath === '/usr/gallery' }"
          >
            {{ $t('guest.galleryTitle') }}
          </router-link>
        </nav>
      </div>

      <!-- Right: Locale & Auth Entry -->
      <div style="display: flex; align-items: center; gap: 12px;">
        <n-select
          v-model:value="currentLocale"
          :options="localeOptions"
          size="small"
          style="width: 100px;"
          @update:value="changeLocale"
        />

        <template v-if="currentUser">
          <n-button size="small" type="primary" secondary @click="goToConsole">
            {{ $t('guest.consoleBtn') }}
          </n-button>
        </template>
        <template v-else>
          <n-button
            v-if="siteInfo.allow_registration"
            size="small"
            secondary
            @click="goToRegister"
          >
            {{ $t('login.register') }}
          </n-button>
          <n-button size="small" type="primary" @click="goToLogin">
            {{ $t('guest.loginBtn') }}
          </n-button>
        </template>
      </div>
    </div>
  </header>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { siteInfo } from '../utils/site'
import { currentUser } from '../utils/auth'

const router = useRouter()
const route = useRoute()
const { locale } = useI18n()

const currentPath = computed(() => route.path)

const currentLocale = computed({
  get: () => locale.value,
  set: (val: string) => {
    locale.value = val
    localStorage.setItem('utopia_locale', val)
  }
})

const localeOptions = [
  { label: '中文', value: 'zh-CN' },
  { label: 'English', value: 'en-US' },
  { label: '日本語', value: 'ja-JP' }
]

const changeLocale = (val: string) => {
  locale.value = val
  localStorage.setItem('utopia_locale', val)
}

const goToLogin = () => {
  router.push('/usr/login')
}

const goToRegister = () => {
  router.push({ path: '/usr/login', query: { action: 'register' } })
}

const goToConsole = () => {
  router.push('/dsh/upload')
}
</script>

<style scoped>
.public-nav-header {
  height: 60px;
  background: #18181c;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  position: sticky;
  top: 0;
  z-index: 100;
}

.public-nav-container {
  max-width: 1280px;
  height: 100%;
  margin: 0 auto;
  padding: 0 20px;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.public-nav-brand {
  display: flex;
  align-items: center;
  gap: 10px;
  text-decoration: none;
  font-weight: bold;
  font-size: 17px;
  color: #63e2b7;
  letter-spacing: 0.5px;
}

.public-nav-logo {
  width: 28px;
  height: 28px;
  object-fit: contain;
}

.public-nav-links {
  display: flex;
  align-items: center;
  gap: 6px;
}

.public-nav-link {
  text-decoration: none;
  font-size: 14px;
  color: #aaa;
  padding: 6px 14px;
  border-radius: 6px;
  transition: all 0.2s ease;
}

.public-nav-link:hover {
  color: #eee;
  background: rgba(255, 255, 255, 0.05);
}

.public-nav-link.active {
  color: #63e2b7;
  background: rgba(99, 226, 183, 0.12);
  font-weight: 500;
}
</style>

