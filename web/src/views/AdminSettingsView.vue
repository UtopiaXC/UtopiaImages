<template>
  <div style="max-width: 800px; margin: 0 auto;">
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 24px;">
      <div>
        <h2 style="margin: 0; font-size: 20px;">{{ $t('adminSettings.title') }}</h2>
        <p style="margin: 4px 0 0; color: #888; font-size: 13px;">
          {{ $t('adminSettings.subtitle') }}
        </p>
      </div>
      <n-button type="primary" :loading="saving" @click="saveSettings">
        {{ $t('adminSettings.saveAll') }}
      </n-button>
    </div>

    <!-- Section 1: Site Info -->
    <n-card :title="$t('adminSettings.siteInfo')" style="border-radius: 8px; margin-bottom: 16px;">
      <n-form label-placement="left" label-width="140">
        <n-form-item :label="$t('adminSettings.siteName')">
          <n-input v-model:value="settings.site_name" placeholder="Utopia Images" style="max-width: 400px;" />
        </n-form-item>
        <n-form-item :label="$t('adminSettings.siteDesc')">
          <n-input v-model:value="settings.site_description" style="max-width: 400px;" />
        </n-form-item>
        <n-form-item :label="$t('adminSettings.baseUrl')">
          <n-input v-model:value="settings.base_url" placeholder="https://imgs.example.com" style="max-width: 400px;" />
        </n-form-item>
      </n-form>
    </n-card>

    <!-- Section 2: Technical Limits -->
    <n-card :title="$t('adminSettings.limits')" style="border-radius: 8px; margin-bottom: 16px;">
      <n-form label-placement="left" label-width="140">
        <n-form-item :label="$t('adminSettings.maxFileSize')">
          <n-input-number v-model:value="maxFileSize" :min="1" :max="1024" style="max-width: 200px;" />
          <span style="margin-left: 10px; color: #888; font-size: 12px;">{{ $t('adminSettings.maxFileSizeTip') }}</span>
        </n-form-item>
        <n-form-item :label="$t('adminSettings.maxProcessSize')">
          <n-input-number v-model:value="maxProcessSize" :min="1" :max="100" style="max-width: 200px;" />
          <span style="margin-left: 10px; color: #888; font-size: 12px;">{{ $t('adminSettings.maxProcessSizeTip') }}</span>
        </n-form-item>
      </n-form>
    </n-card>

    <!-- Section 3: Security -->
    <n-card :title="$t('adminSettings.security')" style="border-radius: 8px; margin-bottom: 16px;">
      <n-form label-placement="left" label-width="140">
        <n-form-item :label="$t('adminSettings.captchaProvider')">
          <n-select
            v-model:value="settings.captcha_provider"
            :options="captchaOptions"
            style="max-width: 300px;"
          />
        </n-form-item>
        <n-form-item :label="$t('adminSettings.sessionTimeout')">
          <n-input-number v-model:value="sessionDays" :min="1" :max="90" style="max-width: 200px;" />
          <span style="margin-left: 10px; color: #888; font-size: 12px;">{{ $t('adminSettings.daysUnit') }}</span>
        </n-form-item>
        <n-form-item :label="$t('adminSettings.force2fa')">
          <n-switch v-model:value="require2FA" />
          <span style="margin-left: 10px; color: #888; font-size: 12px;">{{ $t('adminSettings.force2faTip') }}</span>
        </n-form-item>
      </n-form>
    </n-card>

    <!-- Section 4: Cache Management -->
    <n-card :title="$t('adminSettings.cacheTitle')" style="border-radius: 8px;">
      <p style="font-size: 13px; color: #aaa; margin-top: 0;">
        {{ $t('adminSettings.cacheDesc') }}
      </p>
      <div style="display: flex; gap: 12px;">
        <n-popconfirm @positive-click="clearDerivedCache">
          <template #trigger>
            <n-button type="warning" ghost :loading="clearingCache">
              {{ $t('adminSettings.clearCache') }}
            </n-button>
          </template>
          {{ $t('adminSettings.confirmClearCache') }}
        </n-popconfirm>
      </div>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'

const message = useMessage()
const { t } = useI18n()
const saving = ref(false)
const clearingCache = ref(false)

const settings = ref<any>({
  site_name: 'Utopia Images',
  site_description: '',
  base_url: 'http://localhost:8080',
  max_single_file_size: '50',
  max_process_image_size: '5',
  captcha_provider: 'altcha',
  session_timeout_days: '7',
  require_2fa: 'false'
})

const maxFileSize = computed({
  get: () => parseInt(settings.value.max_single_file_size) || 50,
  set: (v) => { settings.value.max_single_file_size = String(v) }
})

const maxProcessSize = computed({
  get: () => parseInt(settings.value.max_process_image_size) || 5,
  set: (v) => { settings.value.max_process_image_size = String(v) }
})

const sessionDays = computed({
  get: () => parseInt(settings.value.session_timeout_days) || 7,
  set: (v) => { settings.value.session_timeout_days = String(v) }
})

const require2FA = computed({
  get: () => settings.value.require_2fa === 'true',
  set: (v) => { settings.value.require_2fa = v ? 'true' : 'false' }
})

const captchaOptions = computed(() => [
  { label: t('adminSettings.captchaAltcha'), value: 'altcha' },
  { label: t('adminSettings.captchaTurnstile'), value: 'turnstile' },
  { label: t('adminSettings.captchaNone'), value: 'none' }
])

const loadSettings = async () => {
  try {
    const res = await api.get('/admin/settings')
    settings.value = { ...settings.value, ...res.data }
  } catch (err: any) {
    message.error(t('common.save'))
  }
}

const saveSettings = async () => {
  saving.value = true
  try {
    await api.post('/admin/settings', settings.value)
    message.success(t('adminSettings.saveSuccess'))
  } catch (err: any) {
    message.error(err.response?.data?.error || t('common.save'))
  } finally {
    saving.value = false
  }
}

const clearDerivedCache = async () => {
  clearingCache.value = true
  try {
    const res = await api.post('/admin/cache/clear')
    message.success(`${t('adminSettings.cacheCleared')}：${res.data.cleared}`)
  } catch {
    message.error(t('common.save'))
  } finally {
    clearingCache.value = false
  }
}

onMounted(loadSettings)
</script>
