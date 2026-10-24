<template>
  <div style="max-width: 800px; margin: 0 auto;">
    <h2 style="margin-bottom: 24px; font-size: 20px;">{{ $t('settings.title') }}</h2>

    <n-tabs type="line" animated>
      <!-- Tab 1: Profile -->
      <n-tab-pane name="profile" :tab="$t('settings.tabProfile')">
        <n-card style="border-radius: 8px; margin-top: 12px;">
          <n-form label-placement="left" label-width="120">
            <n-form-item :label="$t('settings.currentUsername')">
              <div style="display: flex; align-items: center; gap: 12px; width: 100%;">
                <n-input :value="profile.username" disabled style="max-width: 320px;" />
                <n-button type="warning" ghost size="small" @click="showChangeUsernameModal = true">
                  {{ $t('settings.changeUsername') }}
                </n-button>
              </div>
            </n-form-item>

            <n-alert type="warning" style="margin-bottom: 20px; font-size: 13px;">
              {{ $t('settings.changeUsernameNotice') }}
            </n-alert>

            <n-form-item :label="$t('settings.nickname')">
              <n-input v-model:value="profile.nickname" :placeholder="$t('settings.nicknamePlaceholder')" style="max-width: 320px;" />
            </n-form-item>

            <n-form-item :label="$t('settings.email')">
              <n-input v-model:value="profile.email" placeholder="admin@example.com" style="max-width: 320px;" />
            </n-form-item>

            <div style="display: flex; justify-content: flex-end;">
              <n-button type="primary" :loading="savingProfile" @click="saveProfile">
                {{ $t('settings.saveProfile') }}
              </n-button>
            </div>
          </n-form>

          <n-divider />

          <h3 style="font-size: 16px; margin-bottom: 16px;">{{ $t('settings.passwordChange') }}</h3>
          <n-form label-placement="left" label-width="120">
            <n-form-item :label="$t('settings.oldPassword')">
              <n-input v-model:value="passwordForm.old_password" type="password" show-password-on="click" :placeholder="$t('settings.oldPasswordPlaceholder')" style="max-width: 320px;" />
            </n-form-item>
            <n-form-item :label="$t('settings.newPassword')">
              <n-input v-model:value="passwordForm.new_password" type="password" show-password-on="click" :placeholder="$t('settings.newPasswordPlaceholder')" style="max-width: 320px;" />
            </n-form-item>
            <div style="display: flex; justify-content: flex-end;">
              <n-button type="primary" :loading="savingPassword" @click="changePassword">
                {{ $t('settings.updatePassword') }}
              </n-button>
            </div>
          </n-form>
        </n-card>
      </n-tab-pane>

      <!-- Tab 2: Security & 2FA -->
      <n-tab-pane name="security" :tab="$t('settings.tabSecurity')">
        <n-card style="border-radius: 8px; margin-top: 12px;">
          <div style="display: flex; justify-content: space-between; align-items: center;">
            <div>
              <div style="font-weight: bold; font-size: 16px; margin-bottom: 6px;">
                {{ $t('settings.totpTitle') }}
              </div>
              <div style="color: #888; font-size: 13px;">
                {{ $t('settings.totpDesc') }}
              </div>
            </div>
            <div>
              <n-tag v-if="profile.require_2fa" type="success" size="medium">{{ $t('settings.totpActive') }}</n-tag>
              <n-tag v-else type="default" size="medium">{{ $t('settings.totpInactive') }}</n-tag>
            </div>
          </div>

          <div style="margin-top: 24px;">
            <n-button v-if="!profile.require_2fa" type="primary" @click="startTotpSetup">
              {{ $t('settings.enableTotp') }}
            </n-button>
            <n-popconfirm v-else @positive-click="disableTotp">
              <template #trigger>
                <n-button type="error" ghost>{{ $t('settings.disableTotp') }}</n-button>
              </template>
              {{ $t('settings.confirmDisableTotp') }}
            </n-popconfirm>
          </div>
        </n-card>
      </n-tab-pane>

      <!-- Tab 3: Upload Preferences -->
      <n-tab-pane name="preferences" :tab="$t('settings.tabPreferences')">
        <n-card style="border-radius: 8px; margin-top: 12px;">
          <n-form label-placement="left" label-width="140">
            <n-form-item :label="$t('settings.defaultPolicy')">
              <n-select
                v-model:value="preferences.default_policy_id"
                :options="policyOptions"
                style="max-width: 320px;"
              />
            </n-form-item>

            <n-form-item :label="$t('settings.defaultWatermark')">
              <n-radio-group v-model:value="preferences.watermark_mode">
                <n-radio-button value="inherit">{{ $t('upload.watermarkInherit') }}</n-radio-button>
                <n-radio-button value="on">{{ $t('upload.watermarkOn') }}</n-radio-button>
                <n-radio-button value="off">{{ $t('upload.watermarkOff') }}</n-radio-button>
              </n-radio-group>
            </n-form-item>

            <n-form-item v-if="preferences.watermark_mode === 'on'" :label="$t('settings.defaultTemplate')">
              <n-select
                v-model:value="preferences.watermark_template_id"
                :options="watermarkOptions"
                style="max-width: 320px;"
              />
            </n-form-item>

            <n-form-item :label="$t('settings.defaultLinkFormat')">
              <n-select
                v-model:value="preferences.default_link_format"
                :options="linkFormatOptions"
                style="max-width: 320px;"
              />
            </n-form-item>

            <div style="display: flex; justify-content: flex-end;">
              <n-button type="primary" :loading="savingPreferences" @click="savePreferences">
                {{ $t('settings.savePreferences') }}
              </n-button>
            </div>
          </n-form>
        </n-card>
      </n-tab-pane>
    </n-tabs>

    <!-- Change Username Modal -->
    <n-modal
      v-model:show="showChangeUsernameModal"
      preset="card"
      :title="$t('settings.changeUsernameModalTitle')"
      style="width: 480px; border-radius: 12px;"
    >
      <p style="color: #bbb; font-size: 13px;">
        {{ $t('settings.changeUsernameModalDesc') }}
      </p>
      <n-input v-model:value="newUsername" :placeholder="$t('settings.newUsernamePlaceholder')" />
      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 12px;">
          <n-button @click="showChangeUsernameModal = false">{{ $t('common.cancel') }}</n-button>
          <n-button type="warning" :loading="migratingUsername" @click="handleMigrateUsername">{{ $t('settings.confirmMigrate') }}</n-button>
        </div>
      </template>
    </n-modal>

    <!-- TOTP Setup Modal -->
    <n-modal
      v-model:show="showTotpModal"
      preset="card"
      :title="$t('settings.totpModalTitle')"
      style="width: 440px; border-radius: 12px;"
    >
      <div style="text-align: center;">
        <p style="font-size: 13px; color: #aaa;">{{ $t('settings.scanQr') }}</p>
        <img v-if="totpQr" :src="totpQr" style="width: 180px; height: 180px; border-radius: 8px; margin: 12px auto;" />
        <div style="background: rgba(255,255,255,0.06); padding: 8px; border-radius: 4px; font-family: monospace; font-size: 13px; margin-bottom: 20px;">
          {{ $t('settings.totpSecret') }}：{{ totpSecret }}
        </div>
        <n-input v-model:value="totpCode" :placeholder="$t('settings.totpCodePlaceholder')" style="text-align: center;" />
      </div>
      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 12px;">
          <n-button @click="showTotpModal = false">{{ $t('common.cancel') }}</n-button>
          <n-button type="primary" :loading="verifyingTotp" @click="verifyAndEnableTotp">{{ $t('settings.verifyAndEnable') }}</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'

const message = useMessage()
const { t } = useI18n()

const profile = ref<any>({
  username: '',
  nickname: '',
  email: '',
  require_2fa: false
})

const passwordForm = ref({
  old_password: '',
  new_password: ''
})

const preferences = ref({
  default_policy_id: '1',
  watermark_mode: 'inherit',
  watermark_template_id: '',
  default_link_format: 'markdown'
})

const policyOptions = ref<any[]>([])
const watermarkOptions = ref<any[]>([])

const linkFormatOptions = computed(() => [
  { label: 'Markdown', value: 'markdown' },
  { label: t('upload.directLink'), value: 'url' },
  { label: t('upload.html'), value: 'html' },
  { label: t('upload.bbcode'), value: 'bbcode' }
])

const savingProfile = ref(false)
const savingPassword = ref(false)
const savingPreferences = ref(false)

const showChangeUsernameModal = ref(false)
const newUsername = ref('')
const migratingUsername = ref(false)

const showTotpModal = ref(false)
const totpQr = ref('')
const totpSecret = ref('')
const totpCode = ref('')
const verifyingTotp = ref(false)

const loadData = async () => {
  try {
    const [userRes, prefRes] = await Promise.all([
      api.get('/me'),
      api.get('/me/settings')
    ])
    profile.value = userRes.data
    preferences.value.default_policy_id = prefRes.data.default_policy_id || '1'
    preferences.value.watermark_mode = prefRes.data.watermark_mode || 'inherit'
    preferences.value.watermark_template_id = prefRes.data.watermark_template_id || ''
    preferences.value.default_link_format = prefRes.data.default_link_format || 'markdown'

    policyOptions.value = (prefRes.data.available_policies || []).map((p: any) => ({
      label: p.name,
      value: String(p.id)
    }))

    watermarkOptions.value = (prefRes.data.available_watermarks || []).map((w: any) => ({
      label: w.name,
      value: String(w.id)
    }))
  } catch {}
}

const saveProfile = async () => {
  savingProfile.value = true
  try {
    await api.patch('/me/profile', {
      nickname: profile.value.nickname,
      email: profile.value.email
    })
    message.success(t('settings.profileSaved'))
  } catch (err: any) {
    message.error(err.response?.data?.error || t('settings.profileSaved'))
  } finally {
    savingProfile.value = false
  }
}

const changePassword = async () => {
  if (!passwordForm.value.old_password || !passwordForm.value.new_password) {
    message.warning(t('settings.oldPasswordPlaceholder'))
    return
  }
  savingPassword.value = true
  try {
    await api.post('/me/password', passwordForm.value)
    message.success(t('settings.passwordSuccess'))
    passwordForm.value.old_password = ''
    passwordForm.value.new_password = ''
  } catch (err: any) {
    message.error(err.response?.data?.error || t('common.save'))
  } finally {
    savingPassword.value = false
  }
}

const handleMigrateUsername = async () => {
  if (!newUsername.value) {
    message.warning(t('settings.newUsernamePlaceholder'))
    return
  }
  migratingUsername.value = true
  try {
    const res = await api.patch('/me/username', { new_username: newUsername.value })
    message.success(res.data.message || t('settings.confirmMigrate'))
    profile.value.username = newUsername.value
    showChangeUsernameModal.value = false
  } catch (err: any) {
    message.error(err.response?.data?.error || t('common.save'))
  } finally {
    migratingUsername.value = false
  }
}

const startTotpSetup = async () => {
  try {
    const res = await api.post('/me/totp/setup')
    totpQr.value = res.data.qr_base64
    totpSecret.value = res.data.secret
    totpCode.value = ''
    showTotpModal.value = true
  } catch (err: any) {
    message.error(t('settings.enableTotp'))
  }
}

const verifyAndEnableTotp = async () => {
  if (!totpCode.value) {
    message.warning(t('settings.totpCodePlaceholder'))
    return
  }
  verifyingTotp.value = true
  try {
    await api.post('/me/totp/enable', { code: totpCode.value })
    message.success(t('settings.totpEnabled'))
    profile.value.require_2fa = true
    showTotpModal.value = false
  } catch (err: any) {
    message.error(err.response?.data?.error || t('login.mfaFailed'))
  } finally {
    verifyingTotp.value = false
  }
}

const disableTotp = async () => {
  try {
    await api.post('/me/totp/disable')
    message.success(t('settings.totpDisabled'))
    profile.value.require_2fa = false
  } catch (err: any) {
    message.error(t('common.save'))
  }
}

const savePreferences = async () => {
  savingPreferences.value = true
  try {
    await api.post('/me/settings', preferences.value)
    message.success(t('settings.preferencesSaved'))
  } catch (err: any) {
    message.error(t('common.save'))
  } finally {
    savingPreferences.value = false
  }
}

onMounted(loadData)
</script>
