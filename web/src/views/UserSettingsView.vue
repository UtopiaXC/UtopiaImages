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

            <n-alert type="warning" :show-icon="false" style="margin-bottom: 20px; font-size: 13px;">
              <div style="display: flex; align-items: center; gap: 6px;">
                <n-icon size="14" style="color: #f0a020;">
                  <InformationCircleOutline />
                </n-icon>
                <span>{{ $t('settings.changeUsernameNotice') }}</span>
              </div>
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

      <!-- Tab 2: Security, TOTP & Passkeys -->
      <n-tab-pane name="security" :tab="$t('settings.tabSecurity')">
        <!-- TOTP Section -->
        <n-card :title="$t('settings.totpTitle')" style="border-radius: 8px; margin-top: 12px; margin-bottom: 16px;">
          <template #header-extra>
            <n-tag v-if="totpActive" type="success" size="small">{{ $t('settings.totpActive') }}</n-tag>
            <n-tag v-else type="default" size="small">{{ $t('settings.totpInactive') }}</n-tag>
          </template>

          <div style="color: #888; font-size: 13px; margin-bottom: 16px;">
            {{ $t('settings.totpDesc') }}
          </div>

          <div>
            <n-button v-if="!totpActive" type="primary" @click="startTotpSetup">
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

        <!-- Passkey Section -->
        <n-card :title="$t('settings.passkeyTitle')" style="border-radius: 8px;">
          <template #header-extra>
            <n-button type="primary" size="small" :loading="registeringPasskey" @click="handleCreatePasskey">
              {{ $t('settings.addPasskey') }}
            </n-button>
          </template>

          <div style="color: #888; font-size: 13px; margin-bottom: 16px;">
            {{ $t('settings.passkeyDesc') }}
          </div>

          <div v-if="passkeys.length > 0" class="table-responsive">
            <n-table :bordered="false" :single-line="false" size="small">
              <thead>
                <tr>
                  <th>{{ $t('settings.passkeyName') }}</th>
                  <th>{{ $t('settings.passkeyCreated') }}</th>
                  <th>{{ $t('settings.passkeyLastUsed') }}</th>
                  <th>{{ $t('common.actions') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="pk in passkeys" :key="pk.id">
                  <td><strong>{{ pk.name }}</strong></td>
                  <td>{{ formatDate(pk.created_at) }}</td>
                  <td>{{ pk.last_used_at ? formatDate(pk.last_used_at) : $t('tokens.never') }}</td>
                  <td>
                    <n-popconfirm @positive-click="deletePasskey(pk.id)">
                      <template #trigger>
                        <n-button size="tiny" quaternary type="error">{{ $t('common.delete') }}</n-button>
                      </template>
                      {{ $t('settings.confirmDeletePasskey') }}
                    </n-popconfirm>
                  </td>
                </tr>
              </tbody>
            </n-table>
          </div>

          <div v-else style="color: #777; font-size: 13px; padding: 12px 0;">
            {{ $t('settings.noPasskeys') }}
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

      <!-- Tab 4: Image Settings (Watermark + Hotlink Protection) -->
      <n-tab-pane name="imageSettings" :tab="$t('settings.tabImageSettings')">
        <n-card style="border-radius: 8px; margin-top: 12px;">
          <h3 style="font-size: 15px; margin-bottom: 16px; font-weight: 500;">{{ $t('settings.watermarkSectionTitle') }}</h3>
          <n-form label-placement="left" label-width="140">
            <n-form-item :label="$t('settings.defaultWatermark')">
              <n-radio-group v-model:value="preferences.watermark_mode">
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

            <n-divider style="margin: 16px 0;" />

            <h3 style="font-size: 15px; margin-bottom: 12px; font-weight: 500;">{{ $t('settings.hotlinkSectionTitle') }}</h3>
            <div style="font-size: 13px; color: #888; margin-bottom: 16px;">
              {{ $t('settings.hotlinkDesc') }}
            </div>

            <n-form-item :label="$t('settings.enableHotlink')">
              <n-switch v-model:value="hotlinkSettings.hotlink_enabled" />
            </n-form-item>

            <n-form-item v-if="hotlinkSettings.hotlink_enabled" :label="$t('settings.allowEmpty')">
              <n-switch v-model:value="hotlinkSettings.hotlink_allow_empty" />
            </n-form-item>

            <n-form-item v-if="hotlinkSettings.hotlink_enabled" :label="$t('settings.allowedDomains')">
              <n-input
                v-model:value="hotlinkSettings.hotlink_allowed_domains"
                type="textarea"
                :placeholder="$t('settings.allowedDomainsPlaceholder')"
                :rows="4"
                style="max-width: 480px;"
              />
            </n-form-item>

            <div style="display: flex; justify-content: flex-end;">
              <n-button type="primary" :loading="savingImageSettings" @click="saveImageSettings">
                {{ $t('common.save') }}
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
      style="width: 480px; max-width: 95vw; border-radius: 12px;"
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
      style="width: 440px; max-width: 95vw; border-radius: 12px;"
    >
      <div style="text-align: center;">
        <p style="font-size: 13px; color: #aaa;">{{ $t('settings.scanQr') }}</p>
        <div style="background: #ffffff; width: 190px; height: 190px; border-radius: 8px; margin: 12px auto; display: flex; align-items: center; justify-content: center; padding: 5px;">
          <img v-if="totpQr" :src="totpQr" style="width: 180px; height: 180px; display: block;" />
        </div>
        <div style="background: rgba(255,255,255,0.06); padding: 8px; border-radius: 4px; font-family: monospace; font-size: 13px; margin-bottom: 20px;">
          {{ $t('settings.totpSecret') }}: {{ totpSecret }}
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
import { InformationCircleOutline } from '@vicons/ionicons5'
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

const hotlinkSettings = ref({
  hotlink_enabled: false,
  hotlink_allow_empty: true,
  hotlink_allowed_domains: ''
})

const policyOptions = ref<any[]>([])
const watermarkOptions = ref<any[]>([])
const passkeys = ref<any[]>([])
const registeringPasskey = ref(false)

const linkFormatOptions = computed(() => [
  { label: 'Markdown', value: 'markdown' },
  { label: t('upload.directLink'), value: 'url' },
  { label: t('upload.html'), value: 'html' },
  { label: t('upload.bbcode'), value: 'bbcode' }
])

const savingProfile = ref(false)
const savingPassword = ref(false)
const savingPreferences = ref(false)
const savingHotlink = ref(false)

const showChangeUsernameModal = ref(false)
const newUsername = ref('')
const migratingUsername = ref(false)

const showTotpModal = ref(false)
const totpQr = ref('')
const totpSecret = ref('')
const totpCode = ref('')
const verifyingTotp = ref(false)
const totpActive = ref(false)

const loadData = async () => {
  try {
    const [userRes, prefRes] = await Promise.all([
      api.get('/me'),
      api.get('/me/settings')
    ])
    profile.value = userRes.data
    totpActive.value = Boolean(userRes.data?.totp_enabled || userRes.data?.has_totp)
    preferences.value.default_policy_id = prefRes.data.default_policy_id || '1'
    preferences.value.watermark_mode = prefRes.data.watermark_mode === 'on' ? 'on' : 'off'
    preferences.value.watermark_template_id = prefRes.data.watermark_template_id || ''
    preferences.value.default_link_format = prefRes.data.default_link_format || 'markdown'

    hotlinkSettings.value.hotlink_enabled = !!prefRes.data.hotlink_enabled
    hotlinkSettings.value.hotlink_allow_empty = prefRes.data.hotlink_allow_empty !== false
    hotlinkSettings.value.hotlink_allowed_domains = prefRes.data.hotlink_allowed_domains || ''

    policyOptions.value = (prefRes.data.available_policies || []).map((p: any) => ({
      label: p.name,
      value: String(p.id)
    }))

    watermarkOptions.value = (prefRes.data.available_watermarks || []).map((w: any) => ({
      label: w.name,
      value: String(w.id)
    }))

    loadPasskeys()
  } catch {}
}

const loadPasskeys = async () => {
  try {
    const res = await api.get('/me/passkeys')
    passkeys.value = res.data || []
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
    const rawQr = res.data.qr_base64
    totpQr.value = rawQr.startsWith('data:') ? rawQr : `data:image/png;base64,${rawQr}`
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
    totpActive.value = true
    if (profile.value) {
      profile.value.has_totp = true
      profile.value.totp_enabled = true
    }
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
    totpActive.value = false
    if (profile.value) {
      profile.value.has_totp = false
      profile.value.totp_enabled = false
    }
  } catch (err: any) {
    message.error(t('settings.disableTotp'))
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

const savingImageSettings = ref(false)
const saveImageSettings = async () => {
  savingImageSettings.value = true
  try {
    await api.post('/me/settings', {
      default_policy_id: preferences.value.default_policy_id,
      watermark_mode: preferences.value.watermark_mode,
      watermark_template_id: preferences.value.watermark_template_id,
      default_link_format: preferences.value.default_link_format,
      hotlink_enabled: hotlinkSettings.value.hotlink_enabled,
      hotlink_allow_empty: hotlinkSettings.value.hotlink_allow_empty,
      hotlink_allowed_domains: hotlinkSettings.value.hotlink_allowed_domains
    })
    message.success(t('settings.imageSettingsSaved'))
  } catch (err: any) {
    message.error(err.response?.data?.error || t('common.save'))
  } finally {
    savingImageSettings.value = false
  }
}

// Passkey creation helper
const bufferToBase64URL = (buffer: ArrayBuffer): string => {
  const bytes = new Uint8Array(buffer)
  let binary = ''
  for (let i = 0; i < bytes.byteLength; i++) {
    binary += String.fromCharCode(bytes[i])
  }
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '')
}

const base64URLToBuffer = (base64url: string): Uint8Array => {
  let base64 = base64url.replace(/-/g, '+').replace(/_/g, '/')
  while (base64.length % 4) {
    base64 += '='
  }
  const binary = atob(base64)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i)
  }
  return bytes
}

const handleCreatePasskey = async () => {
  if (!window.PublicKeyCredential) {
    message.error(t('settings.passkeyNotSupported'))
    return
  }

  registeringPasskey.value = true
  try {
    const beginRes = await api.post('/me/passkeys/register-begin')
    const { options, session_key } = beginRes.data
    const pkOptions = options.publicKey

    // Decode challenge and user ID
    pkOptions.challenge = base64URLToBuffer(pkOptions.challenge)
    pkOptions.user.id = base64URLToBuffer(pkOptions.user.id)
    if (pkOptions.excludeCredentials) {
      for (const cred of pkOptions.excludeCredentials) {
        cred.id = base64URLToBuffer(cred.id)
      }
    }

    const credential: any = await navigator.credentials.create({ publicKey: pkOptions })
    if (!credential) {
      throw new Error('Credential creation canceled')
    }

    const payload = {
      id: credential.id,
      rawId: bufferToBase64URL(credential.rawId),
      type: credential.type,
      response: {
        attestationObject: bufferToBase64URL(credential.response.attestationObject),
        clientDataJSON: bufferToBase64URL(credential.response.clientDataJSON)
      }
    }

    await api.post(`/me/passkeys/register-finish?session_key=${session_key}&name=Passkey`, payload)
    message.success(t('settings.passkeyCreatedSuccess'))
    loadPasskeys()
  } catch (err: any) {
    message.error(err.response?.data?.error || err.message || t('settings.passkeyFailed'))
  } finally {
    registeringPasskey.value = false
  }
}

const deletePasskey = async (id: number) => {
  try {
    await api.delete(`/me/passkeys/${id}`)
    message.success(t('settings.passkeyDeleted'))
    loadPasskeys()
  } catch (err: any) {
    message.error(err.response?.data?.error || t('common.delete'))
  }
}

const formatDate = (isoStr: string) => {
  if (!isoStr) return '-'
  const d = new Date(isoStr)
  return `${d.getFullYear()}-${d.getMonth() + 1}-${d.getDate()}`
}

onMounted(loadData)
</script>
