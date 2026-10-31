<template>
  <div style="max-width: 800px; margin: 0 auto;">
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 24px;">
      <div>
        <h2 style="margin: 0; font-size: 20px;">{{ $t('adminSettings.title') }}</h2>
        <p style="margin: 4px 0 0; color: #888; font-size: 13px;">
          {{ $t('adminSettings.subtitle') }}
        </p>
      </div>
      <div style="display: flex; align-items: center; gap: 8px;">
        <span v-if="saving" style="font-size: 13px; color: #63e2b7; display: flex; align-items: center; gap: 6px;">
          <n-spin size="small" />
          {{ $t('adminSettings.autoSaving') }}
        </span>
        <span v-else style="font-size: 12px; color: #666;">
          {{ $t('adminSettings.autoSaved') }}
        </span>
      </div>
    </div>

    <!-- Section 1: Site Info -->
    <n-card :title="$t('adminSettings.siteInfo')" style="border-radius: 8px; margin-bottom: 16px;">
      <n-form label-placement="left" label-width="150">
        <n-form-item :label="$t('adminSettings.siteName')">
          <n-input v-model:value="settings.site_name" placeholder="Utopia Images" style="max-width: 400px;" />
        </n-form-item>
        <n-form-item :label="$t('adminSettings.siteDesc')">
          <n-input v-model:value="settings.site_description" style="max-width: 400px;" />
        </n-form-item>
        <n-form-item :label="$t('adminSettings.baseUrl')">
          <div style="display: flex; flex-direction: column; gap: 4px; width: 100%; max-width: 500px;">
            <n-input
              v-model:value="settings.base_url"
              type="textarea"
              :autosize="{ minRows: 2, maxRows: 4 }"
              placeholder=""
            />
            <span style="font-size: 12px; color: #888;">
              {{ $t('adminSettings.baseUrlTip') }}
            </span>
          </div>
        </n-form-item>
        <n-form-item :label="$t('adminSettings.restrictDomains')">
          <div style="display: flex; flex-direction: column; gap: 6px; max-width: 500px;">
            <div style="display: flex; align-items: center; gap: 10px;">
              <n-switch v-model:value="restrictDomains" />
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-icon size="16" style="color: #888; cursor: pointer;">
                  <InformationCircleOutline />
                </n-icon>
              </template>
              {{ $t('adminSettings.restrictDomainsDesc') }}
            </n-tooltip>
            </div>
          </div>
        </n-form-item>
        <n-form-item :label="$t('adminSettings.faviconTitle')">
          <div style="display: flex; align-items: center; gap: 16px;">
            <div
              style="width: 48px; height: 48px; border-radius: 50%; background: #ffffff; border: 1px solid rgba(255,255,255,0.15); display: flex; align-items: center; justify-content: center; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.3);"
            >
              <img
                :src="faviconUrl"
                style="width: 38px; height: 38px; object-fit: contain;"
              />
            </div>
            <div style="display: flex; gap: 10px; align-items: center;">
              <n-upload
                :custom-request="handleFaviconUpload"
                :show-file-list="false"
                accept=".ico,.png,.svg,.jpg,.jpeg,.webp"
              >
                <n-button size="small" type="primary" secondary :loading="uploadingFavicon">
                  {{ $t('adminSettings.uploadFavicon') }}
                </n-button>
              </n-upload>
              <n-button size="small" quaternary @click="handleRestoreDefaultFavicon" :loading="restoringFavicon">
                {{ $t('adminSettings.restoreDefaultFavicon') }}
              </n-button>
            </div>
          </div>
        </n-form-item>
      </n-form>
    </n-card>

    <!-- Section 2: Technical Limits & Formats -->
    <n-card :title="$t('adminSettings.limitsAndConversion')" style="border-radius: 8px; margin-bottom: 16px;">
      <n-form label-placement="left" label-width="150">
        <n-form-item :label="$t('adminSettings.enableFormatConversion')">
          <div style="display: flex; align-items: center; gap: 10px;">
            <n-switch v-model:value="enableFormatConversion" />
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-icon size="16" style="color: #888; cursor: pointer;">
                  <InformationCircleOutline />
                </n-icon>
              </template>
              {{ $t('adminSettings.enableFormatConversionTip') }}
            </n-tooltip>
          </div>
        </n-form-item>

        <n-form-item v-if="enableFormatConversion" :label="$t('adminSettings.defaultFormat')">
          <n-select
            v-model:value="settings.default_convert_format"
            :options="formatOptions"
            style="max-width: 200px;"
          />
        </n-form-item>

        <n-form-item v-if="enableFormatConversion" :label="$t('adminSettings.convertQuality')">
          <div style="display: flex; align-items: center; gap: 16px; width: 100%; max-width: 320px;">
            <n-slider v-model:value="convertQuality" :min="1" :max="100" style="flex: 1;" />
            <span style="width: 45px; text-align: right;">{{ convertQuality }}%</span>
          </div>
        </n-form-item>

        <n-form-item :label="$t('adminSettings.maxFileSize')">
          <div style="display: flex; align-items: center; gap: 10px;">
            <n-input-number v-model:value="maxFileSize" :min="0" :max="102400" style="max-width: 200px;" />
            <span style="color: #888; font-size: 13px;">MB (填 0 为无限制)</span>
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-icon size="16" style="color: #888; cursor: pointer;">
                  <InformationCircleOutline />
                </n-icon>
              </template>
              {{ $t('adminSettings.maxFileSizeTip') }}
            </n-tooltip>
          </div>
        </n-form-item>

        <n-form-item :label="$t('adminSettings.maxProcessSize')">
          <div style="display: flex; align-items: center; gap: 10px;">
            <n-input-number v-model:value="maxProcessSize" :min="1" :max="100" style="max-width: 200px;" />
            <span style="color: #888; font-size: 13px;">MB</span>
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-icon size="16" style="color: #888; cursor: pointer;">
                  <InformationCircleOutline />
                </n-icon>
              </template>
              {{ $t('adminSettings.maxProcessSizeTip') }}
            </n-tooltip>
          </div>
        </n-form-item>
      </n-form>
    </n-card>

    <!-- Section 3: Security -->
    <n-card :title="$t('adminSettings.security')" style="border-radius: 8px; margin-bottom: 16px;">
      <n-form label-placement="left" label-width="150">
        <n-form-item :label="$t('adminSettings.captchaProvider')">
          <n-select
            v-model:value="settings.captcha_provider"
            :options="captchaOptions"
            style="max-width: 300px;"
          />
        </n-form-item>
        <n-form-item :label="$t('adminSettings.sessionTimeout')">
          <div style="display: flex; align-items: center; gap: 10px;">
            <n-input-number v-model:value="sessionDays" :min="1" :max="90" style="max-width: 200px;" />
            <span style="color: #888; font-size: 13px;">{{ $t('adminSettings.daysUnit') }}</span>
          </div>
        </n-form-item>
        <n-form-item :label="$t('adminSettings.force2fa')">
          <div style="display: flex; align-items: center; gap: 10px;">
            <n-switch v-model:value="require2FA" />
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-icon size="16" style="color: #888; cursor: pointer;">
                  <InformationCircleOutline />
                </n-icon>
              </template>
              {{ $t('adminSettings.force2faTip') }}
            </n-tooltip>
          </div>
        </n-form-item>
      </n-form>
    </n-card>

    <!-- Section 3.5: Hotlink Protection -->
    <n-card :title="$t('adminSettings.hotlinkTitle')" style="border-radius: 8px; margin-bottom: 16px;">
      <n-form label-placement="left" label-width="150">
        <n-form-item :label="$t('adminSettings.enableHotlink')">
          <div style="display: flex; align-items: center; gap: 10px;">
            <n-switch v-model:value="enableHotlink" />
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-icon size="16" style="color: #888; cursor: pointer;">
                  <InformationCircleOutline />
                </n-icon>
              </template>
              {{ $t('adminSettings.enableHotlinkTip') }}
            </n-tooltip>
          </div>
        </n-form-item>

        <n-form-item v-if="enableHotlink" :label="$t('adminSettings.allowEmptyReferer')">
          <div style="display: flex; align-items: center; gap: 10px;">
            <n-switch v-model:value="allowEmptyReferer" />
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-icon size="16" style="color: #888; cursor: pointer;">
                  <InformationCircleOutline />
                </n-icon>
              </template>
              {{ $t('adminSettings.allowEmptyRefererTip') }}
            </n-tooltip>
          </div>
        </n-form-item>

        <n-form-item v-if="enableHotlink" :label="$t('adminSettings.allowedDomains')">
          <n-input
            v-model:value="settings.hotlink_allowed_domains"
            type="textarea"
            :rows="3"
            :placeholder="$t('adminSettings.allowedDomainsPlaceholder')"
            style="max-width: 500px;"
          />
        </n-form-item>

        <n-form-item v-if="enableHotlink" :label="$t('adminSettings.hotlinkAction')">
          <n-select
            v-model:value="settings.hotlink_action"
            :options="hotlinkActionOptions"
            style="max-width: 300px;"
          />
        </n-form-item>
      </n-form>
    </n-card>

    <!-- Section 3.6: User Registration & Permissions -->
    <n-card :title="$t('adminSettings.registrationTitle')" style="border-radius: 8px; margin-bottom: 16px;">
      <n-form label-placement="left" label-width="150">
        <n-form-item :label="$t('adminSettings.allowRegistration')">
          <div style="display: flex; align-items: center; gap: 10px;">
            <n-switch v-model:value="allowRegistration" />
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-icon size="16" style="color: #888; cursor: pointer;">
                  <InformationCircleOutline />
                </n-icon>
              </template>
              {{ $t('adminSettings.allowRegistrationTip') }}
            </n-tooltip>
          </div>
        </n-form-item>

        <n-form-item v-if="allowRegistration" :label="$t('adminSettings.defaultRegisterRole')">
          <div style="display: flex; align-items: center; gap: 10px;">
            <n-select
              v-model:value="defaultRegisterRoleId"
              :options="roleOptions"
              style="width: 280px;"
            />
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-icon size="16" style="color: #888; cursor: pointer;">
                  <InformationCircleOutline />
                </n-icon>
              </template>
              {{ $t('adminSettings.defaultRegisterRoleTip') }}
            </n-tooltip>
          </div>
        </n-form-item>
      </n-form>
    </n-card>

    <!-- Section 3.7: Mail Notification Service -->
    <n-card :title="$t('adminSettings.smtpTitle')" style="border-radius: 8px; margin-bottom: 16px;">
      <p style="font-size: 13px; color: #aaa; margin-top: 0; margin-bottom: 16px;">
        {{ $t('adminSettings.smtpDesc') }}
      </p>
      <n-form label-placement="left" label-width="150">
        <n-form-item :label="$t('adminSettings.enableSmtp')">
          <div style="display: flex; align-items: center; gap: 10px;">
            <n-switch v-model:value="smtpEnabled" />
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-icon size="16" style="color: #888; cursor: pointer;">
                  <InformationCircleOutline />
                </n-icon>
              </template>
              {{ $t('adminSettings.smtpOptionalTip') }}
            </n-tooltip>
          </div>
        </n-form-item>

        <template v-if="smtpEnabled">
          <n-form-item :label="$t('setup.smtpHost')">
            <n-input v-model:value="settings.smtp_host" placeholder="smtp.example.com" style="max-width: 400px;" />
          </n-form-item>

          <n-form-item :label="$t('setup.smtpPort')">
            <n-input-number v-model:value="smtpPort" :min="1" :max="65535" style="max-width: 200px;" />
          </n-form-item>

          <n-form-item :label="$t('setup.smtpEncryption')">
            <n-select v-model:value="settings.smtp_encryption" :options="encryptionOptions" style="max-width: 200px;" />
          </n-form-item>

          <n-form-item :label="$t('setup.smtpFrom')">
            <n-input v-model:value="settings.smtp_from" placeholder="noreply@example.com" style="max-width: 400px;" />
          </n-form-item>

          <n-form-item :label="$t('setup.smtpFromName')">
            <n-input v-model:value="settings.smtp_from_name" placeholder="Utopia Images" style="max-width: 400px;" />
          </n-form-item>

          <n-form-item :label="$t('setup.smtpUser')">
            <n-input v-model:value="settings.smtp_user" placeholder="username@example.com" style="max-width: 400px;" />
          </n-form-item>

          <n-form-item :label="$t('setup.smtpPassword')">
            <n-input v-model:value="settings.smtp_password" type="password" show-password-on="click" placeholder="••••••••" style="max-width: 400px;" />
          </n-form-item>

          <n-form-item label=" ">
            <n-button type="info" ghost :loading="testingSmtp" @click="handleTestSmtp">
              {{ $t('setup.testSmtp') }}
            </n-button>
          </n-form-item>
        </template>
      </n-form>
    </n-card>


    <!-- Section 5: Logging Settings -->
    <n-card :title="$t('adminSettings.loggingTitle')" style="border-radius: 8px; margin-bottom: 16px;">
      <p style="font-size: 13px; color: #aaa; margin-top: 0; margin-bottom: 16px;">
        {{ $t('adminSettings.loggingDesc') }}
      </p>
      <n-form label-placement="left" label-width="150">
        <n-form-item :label="$t('adminSettings.logLevel')">
          <div style="display: flex; align-items: center; gap: 10px;">
            <n-select
              v-model:value="settings.log_level"
              :options="logLevelOptions"
              style="width: 280px;"
            />
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-icon size="16" style="color: #888; cursor: pointer;">
                  <InformationCircleOutline />
                </n-icon>
              </template>
              {{ $t('adminSettings.logLevelTip') }}
            </n-tooltip>
          </div>
        </n-form-item>

        <n-form-item :label="$t('adminSettings.logPermanentLevel')">
          <div style="display: flex; align-items: center; gap: 10px;">
            <n-select
              v-model:value="settings.log_permanent_level"
              :options="permanentLogLevelOptions"
              style="width: 280px;"
            />
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-icon size="16" style="color: #888; cursor: pointer;">
                  <InformationCircleOutline />
                </n-icon>
              </template>
              {{ $t('adminSettings.logPermanentLevelTip') }}
            </n-tooltip>
          </div>
        </n-form-item>

        <n-form-item :label="$t('adminSettings.logMaxSize')">
          <div style="display: flex; align-items: center; gap: 10px;">
            <n-input-number v-model:value="logMaxSize" :min="1" :max="500" style="max-width: 200px;" />
            <span style="color: #888; font-size: 13px;">MB</span>
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-icon size="16" style="color: #888; cursor: pointer;">
                  <InformationCircleOutline />
                </n-icon>
              </template>
              {{ $t('adminSettings.logMaxSizeTip') }}
            </n-tooltip>
          </div>
        </n-form-item>

        <n-form-item :label="$t('adminSettings.logMaxBackups')">
          <div style="display: flex; align-items: center; gap: 10px;">
            <n-input-number v-model:value="logMaxBackups" :min="1" :max="100" style="max-width: 200px;" />
            <span style="color: #888; font-size: 13px;">{{ $t('adminSettings.backupUnits') }}</span>
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-icon size="16" style="color: #888; cursor: pointer;">
                  <InformationCircleOutline />
                </n-icon>
              </template>
              {{ $t('adminSettings.logMaxBackupsTip') }}
            </n-tooltip>
          </div>
        </n-form-item>

        <n-form-item :label="$t('adminSettings.logMaxAge')">
          <div style="display: flex; align-items: center; gap: 10px;">
            <n-input-number v-model:value="logMaxAge" :min="1" :max="365" style="max-width: 200px;" />
            <span style="color: #888; font-size: 13px;">{{ $t('adminSettings.daysUnit') }}</span>
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-icon size="16" style="color: #888; cursor: pointer;">
                  <InformationCircleOutline />
                </n-icon>
              </template>
              {{ $t('adminSettings.logMaxAgeTip') }}
            </n-tooltip>
          </div>
        </n-form-item>
      </n-form>
    </n-card>

    <!-- Section 6: Cache Management -->
    <n-card :title="$t('adminSettings.cacheTitle')" style="border-radius: 8px;">
      <p style="font-size: 13px; color: #aaa; margin-top: 0;">
        {{ $t('adminSettings.cacheDesc') }}
      </p>

      <!-- Cache Usage Progress -->
      <div style="margin-bottom: 24px; padding: 16px; background-color: var(--card-color, rgba(255,255,255,0.03)); border-radius: 6px; border: 1px solid var(--border-color, rgba(255,255,255,0.08));">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px;">
          <div style="font-weight: 500;">
            {{ $t('adminSettings.cacheUsage') }}
          </div>
          <div style="font-size: 13px; color: #888;">
            <span style="font-weight: 600; color: var(--text-color);">{{ formatBytes(cacheStats.usage_bytes) }}</span>
            <span v-if="cacheLimitBytes > 0"> / {{ formatBytes(cacheLimitBytes) }} ({{ cacheUsagePercent }}%)</span>
            <span v-else> ({{ $t('adminSettings.cacheUsageUnlimited') }})</span>
            <span style="margin-left: 12px; color: #888;">{{ cacheStats.file_count }} {{ $t('adminSettings.cacheFilesCount') }}</span>
          </div>
        </div>
        <n-progress
          type="line"
          :percentage="cacheLimitBytes > 0 ? cacheUsagePercent : 0"
          :status="cacheProgressStatus"
          :show-indicator="cacheLimitBytes > 0"
          :height="10"
          border-radius="5px"
        />
      </div>

      <n-form label-placement="left" label-width="160" :model="settings">
        <n-form-item :label="$t('adminSettings.cacheMaxSize')">
          <div style="display: flex; flex-direction: column; gap: 4px;">
            <n-input
              v-model:value="settings.derived_cache_max_size_mb"
              placeholder="0"
              style="max-width: 400px;"
            >
              <template #suffix>MB</template>
            </n-input>
            <span style="font-size: 12px; color: #888;">
              {{ $t('adminSettings.cacheMaxSizeTip') }}
            </span>
          </div>
        </n-form-item>

        <n-form-item :label="$t('adminSettings.cacheTTL')">
          <div style="display: flex; flex-direction: column; gap: 4px;">
            <n-input
              v-model:value="settings.derived_cache_ttl_days"
              placeholder="0"
              style="max-width: 400px;"
            >
              <template #suffix>{{ $t('adminSettings.cacheDaysUnit') }}</template>
            </n-input>
            <span style="font-size: 12px; color: #888;">
              {{ $t('adminSettings.cacheTTLTip') }}
            </span>
          </div>
        </n-form-item>

        <n-form-item :label="$t('adminSettings.globalCallStat')">
          <div style="display: flex; flex-direction: column; gap: 6px; max-width: 500px;">
            <div style="display: flex; align-items: center; gap: 10px;">
              <n-switch v-model:value="globalCallStat" />
              <n-tooltip trigger="hover">
                <template #trigger>
                  <n-icon size="16" style="color: #888; cursor: pointer;">
                    <InformationCircleOutline />
                  </n-icon>
                </template>
                {{ $t('adminSettings.globalCallStatDesc') }}
              </n-tooltip>
            </div>
            <span style="font-size: 12px; color: #888;">
              {{ $t('adminSettings.globalCallStatTip') }}
            </span>
          </div>
        </n-form-item>
      </n-form>

      <div style="display: flex; gap: 12px; margin-top: 8px;">
        <n-button type="primary" secondary :loading="pruningCache" @click="pruneDerivedCache">
          {{ $t('adminSettings.pruneCache') }}
        </n-button>
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
import { ref, computed, onMounted, watch, nextTick } from 'vue'
import { useMessage, useDialog } from 'naive-ui'
import { InformationCircleOutline } from '@vicons/ionicons5'
import { useI18n } from 'vue-i18n'
import { api, getApiErrorMessage } from '../api/client'
import { setSiteInfo, updateDocumentTitle } from '../utils/site'

const message = useMessage()
const dialog = useDialog()
const { t } = useI18n()
const saving = ref(false)
const clearingCache = ref(false)

const settings = ref<any>({
  site_name: 'Utopia Images',
  site_description: '',
  base_url: (typeof window !== 'undefined' && window.location?.origin) ? window.location.origin : '',
  max_single_file_size: '50',
  max_process_image_size: '5',
  captcha_provider: 'altcha',
  session_timeout_days: '7',
  require_2fa: 'false',
  enable_format_conversion: 'true',
  default_convert_format: 'webp',
  convert_quality: '85',
  log_level: 'info',
  log_permanent_level: 'ERROR',
  log_max_size_mb: '20',
  log_max_backups: '10',
  log_max_age_days: '30',
  derived_cache_max_size_mb: '5120',
  derived_cache_ttl_days: '0',
  allow_registration: 'false',
  default_register_role_id: '2',
  restrict_domains: 'false',
  global_call_stat: 'false',
  smtp_enabled: 'false',
  smtp_host: '',
  smtp_port: '587',
  smtp_encryption: 'none',
  smtp_from: '',
  smtp_from_name: 'Utopia Images',
  smtp_user: '',
  smtp_password: ''
})

const faviconTimestamp = ref(Date.now())
const faviconUrl = computed(() => `/favicon.ico?v=${faviconTimestamp.value}`)
const uploadingFavicon = ref(false)
const restoringFavicon = ref(false)

const handleFaviconUpload = async ({ file, onFinish, onError }: any) => {
  uploadingFavicon.value = true
  const formData = new FormData()
  formData.append('file', file.file as File)
  try {
    await api.post('/admin/favicon', formData)
    faviconTimestamp.value = Date.now()
    message.success(t('adminSettings.faviconUpdated'))
    onFinish()
  } catch (err: any) {
    message.error(err.response?.data?.error || t('common.save'))
    onError()
  } finally {
    uploadingFavicon.value = false
  }
}

const handleRestoreDefaultFavicon = async () => {
  restoringFavicon.value = true
  try {
    await api.delete('/admin/favicon')
    faviconTimestamp.value = Date.now()
    message.success(t('adminSettings.faviconRestored'))
  } catch (err: any) {
    message.error(err.response?.data?.error || t('common.save'))
  } finally {
    restoringFavicon.value = false
  }
}

const rolesList = ref<any[]>([])
const roleOptions = computed(() => {
  return rolesList.value.map(r => ({
    label: `${r.display_name} (${r.name})`,
    value: r.id
  }))
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

const convertQuality = computed({
  get: () => parseInt(settings.value.convert_quality) || 85,
  set: (v) => { settings.value.convert_quality = String(v) }
})

const require2FA = computed({
  get: () => settings.value.require_2fa === 'true',
  set: (v) => { settings.value.require_2fa = v ? 'true' : 'false' }
})

const enableFormatConversion = computed({
  get: () => settings.value.enable_format_conversion !== 'false' && settings.value.enable_format_conversion !== '0',
  set: (v) => { settings.value.enable_format_conversion = v ? 'true' : 'false' }
})

const enableHotlink = computed({
  get: () => settings.value.enable_hotlink === 'true',
  set: (v) => { settings.value.enable_hotlink = v ? 'true' : 'false' }
})

const allowEmptyReferer = computed({
  get: () => settings.value.hotlink_allow_empty !== 'false',
  set: (v) => { settings.value.hotlink_allow_empty = v ? 'true' : 'false' }
})

const hotlinkActionOptions = computed(() => [
  { label: t('adminSettings.actionForbidden'), value: 'forbidden' }
])

const captchaOptions = computed(() => [
  { label: t('adminSettings.captchaAltcha'), value: 'altcha' },
  { label: t('adminSettings.captchaTurnstile'), value: 'turnstile' },
  { label: t('adminSettings.captchaNone'), value: 'none' }
])

const formatOptions = computed(() => [
  { label: 'WebP', value: 'webp' },
  { label: 'PNG', value: 'png' },
  { label: 'JPEG', value: 'jpeg' }
])

const logMaxSize = computed({
  get: () => parseInt(settings.value.log_max_size_mb) || 20,
  set: (v) => { settings.value.log_max_size_mb = String(v) }
})

const logMaxBackups = computed({
  get: () => parseInt(settings.value.log_max_backups) || 10,
  set: (v) => { settings.value.log_max_backups = String(v) }
})

const logMaxAge = computed({
  get: () => parseInt(settings.value.log_max_age_days) || 30,
  set: (v) => { settings.value.log_max_age_days = String(v) }
})

const allowRegistration = computed({
  get: () => settings.value.allow_registration === 'true',
  set: (v) => { settings.value.allow_registration = v ? 'true' : 'false' }
})

const defaultRegisterRoleId = computed({
  get: () => parseInt(settings.value.default_register_role_id) || 2,
  set: (v) => { settings.value.default_register_role_id = String(v) }
})

const smtpEnabled = computed({
  get: () => settings.value.smtp_enabled === 'true',
  set: (v) => { settings.value.smtp_enabled = v ? 'true' : 'false' }
})

const restrictDomains = computed({
  get: () => settings.value.restrict_domains === 'true',
  set: (v) => { settings.value.restrict_domains = v ? 'true' : 'false' }
})

const globalCallStat = computed({
  get: () => settings.value.global_call_stat === 'true',
  set: (v) => { settings.value.global_call_stat = v ? 'true' : 'false' }
})

const smtpPort = computed({
  get: () => parseInt(settings.value.smtp_port) || 587,
  set: (v) => { settings.value.smtp_port = String(v) }
})

const encryptionOptions = computed(() => [
  { label: 'SSL / TLS', value: 'ssl' },
  { label: 'STARTTLS', value: 'tls' },
  { label: t('common.disabled'), value: 'none' }
])

const testingSmtp = ref(false)
const handleTestSmtp = async () => {
  if (!settings.value.smtp_host) {
    message.warning(t('setup.fillSmtpHost'))
    return
  }
  testingSmtp.value = true
  try {
    const res = await api.post('/admin/smtp/test', {
      host: settings.value.smtp_host,
      port: parseInt(settings.value.smtp_port) || 25,
      from: settings.value.smtp_from,
      user: settings.value.smtp_user,
      password: settings.value.smtp_password,
      encryption: settings.value.smtp_encryption || 'none'
    })
    message.success(res.data?.message || t('setup.testSmtpSuccess'))
  } catch (err: any) {
    message.error(getApiErrorMessage(err) || t('setup.testSmtpFailed'))
  } finally {
    testingSmtp.value = false
  }
}

const logLevelOptions = computed(() => [
  { label: 'DEBUG (调试详细)', value: 'debug' },
  { label: 'INFO (标准信息)', value: 'info' },
  { label: 'WARN (异常告警)', value: 'warn' },
  { label: 'ERROR (故障错误)', value: 'error' }
])

const permanentLogLevelOptions = computed(() => [
  { label: 'ERROR (仅持久化错误日志)', value: 'ERROR' },
  { label: 'WARN (持久化告警与错误)', value: 'WARN' },
  { label: t('adminSettings.logLevelNone'), value: 'none' }
])

const loadRoles = async () => {
  try {
    const res = await api.get('/admin/roles')
    rolesList.value = res.data || []
  } catch {}
}

const loadSettings = async () => {
  try {
    const res = await api.get('/admin/settings')
    settings.value = { ...settings.value, ...res.data }
    const curOrigin = typeof window !== 'undefined' ? window.location.origin : ''
    if (curOrigin && !curOrigin.includes('localhost') && !curOrigin.includes('127.0.0.1')) {
      if (!settings.value.base_url || settings.value.base_url.includes('localhost') || settings.value.base_url.includes('127.0.0.1')) {
        settings.value.base_url = curOrigin
      }
    }
    loadRoles()
    await nextTick()
    isLoaded = true
  } catch (err: any) {
    message.error(getApiErrorMessage(err))
  }
}

let autoSaveTimer: any = null
let isLoaded = false

const autoSave = (immediate = false) => {
  if (!isLoaded) return
  if (autoSaveTimer) clearTimeout(autoSaveTimer)
  const delay = immediate ? 0 : 400
  autoSaveTimer = setTimeout(async () => {
    saving.value = true
    try {
      await api.post('/admin/settings', settings.value)
      if (settings.value.site_name) {
        setSiteInfo({ site_name: settings.value.site_name })
        updateDocumentTitle(t('nav.adminSettings'))
      }
    } catch (err: any) {
      message.error(getApiErrorMessage(err))
      if (err.response?.data?.code === 'ERR_REGISTRATION_SMTP_REQUIRED') {
        settings.value.allow_registration = 'false'
      }
    } finally {
      saving.value = false
    }
  }, delay)
}

watch(
  settings,
  () => {
    autoSave(false)
  },
  { deep: true }
)

const cacheStats = ref({
  usage_bytes: 0,
  file_count: 0,
  max_size_mb: 5120,
  ttl_days: 0
})
const loadingCacheStats = ref(false)
const pruningCache = ref(false)

const formatBytes = (bytes: number): string => {
  if (!bytes || bytes <= 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

const cacheLimitBytes = computed(() => {
  const mb = Number(settings.value.derived_cache_max_size_mb) || 0
  return mb > 0 ? mb * 1024 * 1024 : 0
})

const cacheUsagePercent = computed(() => {
  if (cacheLimitBytes.value <= 0) return 0
  const pct = Math.round((cacheStats.value.usage_bytes / cacheLimitBytes.value) * 100)
  return Math.min(100, Math.max(0, pct))
})

const cacheProgressStatus = computed(() => {
  if (cacheLimitBytes.value <= 0) return 'info'
  if (cacheUsagePercent.value >= 90) return 'error'
  if (cacheUsagePercent.value >= 75) return 'warning'
  return 'success'
})

const loadCacheStats = async () => {
  loadingCacheStats.value = true
  try {
    const res = await api.get('/admin/cache/stats')
    cacheStats.value = res.data
  } catch {}
  finally {
    loadingCacheStats.value = false
  }
}

const pruneDerivedCache = async () => {
  pruningCache.value = true
  try {
    const res = await api.post('/admin/cache/prune')
    message.success(`${t('adminSettings.cachePruned')}: ${res.data.cleared_count} (${formatBytes(res.data.freed_bytes)})`)
    await loadCacheStats()
  } catch {
    message.error(t('common.save'))
  } finally {
    pruningCache.value = false
  }
}

const clearDerivedCache = async () => {
  clearingCache.value = true
  try {
    const res = await api.post('/admin/cache/clear')
    message.success(`${t('adminSettings.cacheCleared')}: ${res.data.cleared}`)
    await loadCacheStats()
  } catch {
    message.error(t('common.save'))
  } finally {
    clearingCache.value = false
  }
}

onMounted(() => {
  loadSettings()
  loadCacheStats()
})
</script>
