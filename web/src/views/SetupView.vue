<template>
  <div class="setup-container">
    <div class="setup-card-wrapper">
      <!-- Top header bar with Brand & Language Selector -->
      <div class="setup-header">
        <div class="setup-brand">
          <div class="setup-logo">UI</div>
          <div>
            <div class="setup-title">Utopia Images</div>
            <div class="setup-subtitle">{{ $t('setup.title') }}</div>
          </div>
        </div>
        <div>
          <n-select
            v-model:value="currentLocale"
            :options="localeOptions"
            size="small"
            style="width: 110px;"
            @update:value="changeLocale"
          />
        </div>
      </div>

      <!-- Step Indicator -->
      <n-card class="setup-main-card">
        <n-steps :current="currentStep" size="small" style="margin-bottom: 24px;">
          <n-step :title="$t('setup.stepSite')" />
          <n-step :title="$t('setup.stepDb')" />
          <n-step :title="$t('setup.stepSmtp')" />
          <n-step :title="$t('setup.stepAdmin')" />
        </n-steps>

        <!-- Step 1: Site Info -->
        <div v-if="currentStep === 1">
          <div class="step-guide">{{ $t('setup.welcome') }}</div>
          <n-form label-placement="left" label-width="140" style="margin-top: 16px;">
            <n-form-item :label="$t('setup.siteName')">
              <n-input v-model:value="form.site.name" placeholder="Utopia Images" />
            </n-form-item>
            <n-form-item :label="$t('setup.siteDesc')">
              <n-input v-model:value="form.site.description" />
            </n-form-item>
            <n-form-item :label="$t('setup.baseUrl')">
              <n-input v-model:value="form.site.base_url" placeholder="http://localhost:8080" />
            </n-form-item>
            <n-form-item :label="$t('setup.defaultLocale')">
              <n-select v-model:value="form.site.locale" :options="localeOptions" />
            </n-form-item>
          </n-form>
        </div>

        <!-- Step 2: Database Configuration -->
        <div v-else-if="currentStep === 2">
          <div class="step-guide">{{ $t('setup.dbDriver') }}</div>
          <n-form label-placement="left" label-width="140" style="margin-top: 16px;">
            <n-form-item :label="$t('setup.dbDriver')">
              <n-radio-group v-model:value="form.database.driver">
                <n-radio-button value="sqlite">SQLite</n-radio-button>
                <n-radio-button value="postgres">PostgreSQL</n-radio-button>
                <n-radio-button value="mysql">MySQL</n-radio-button>
              </n-radio-group>
            </n-form-item>

            <!-- SQLite Settings -->
            <template v-if="form.database.driver === 'sqlite'">
              <n-alert type="info" :show-icon="false" style="margin-bottom: 16px; font-size: 13px;">
                {{ $t('setup.dbSqliteDesc') }}
              </n-alert>
              <n-form-item :label="$t('setup.dbPath')">
                <n-input v-model:value="dbFields.sqlitePath" placeholder="data/db/utopiaimages.db" />
              </n-form-item>
            </template>

            <!-- PostgreSQL Settings -->
            <template v-else-if="form.database.driver === 'postgres'">
              <n-alert type="info" :show-icon="false" style="margin-bottom: 16px; font-size: 13px;">
                {{ $t('setup.dbPostgresDesc') }}
              </n-alert>
              <n-grid cols="2" :x-gap="12">
                <n-grid-item>
                  <n-form-item :label="$t('setup.dbHost')">
                    <n-input v-model:value="dbFields.host" placeholder="localhost" />
                  </n-form-item>
                </n-grid-item>
                <n-grid-item>
                  <n-form-item :label="$t('setup.dbPort')">
                    <n-input-number v-model:value="dbFields.pgPort" :min="1" :max="65535" style="width: 100%;" />
                  </n-form-item>
                </n-grid-item>
              </n-grid>
              <n-form-item :label="$t('setup.dbName')">
                <n-input v-model:value="dbFields.dbName" placeholder="utopiaimages" />
              </n-form-item>
              <n-grid cols="2" :x-gap="12">
                <n-grid-item>
                  <n-form-item :label="$t('setup.dbUser')">
                    <n-input v-model:value="dbFields.user" placeholder="postgres" />
                  </n-form-item>
                </n-grid-item>
                <n-grid-item>
                  <n-form-item :label="$t('setup.dbPassword')">
                    <n-input v-model:value="dbFields.password" type="password" show-password-on="click" />
                  </n-form-item>
                </n-grid-item>
              </n-grid>
              <n-form-item :label="$t('setup.dbSslMode')">
                <n-select v-model:value="dbFields.sslmode" :options="sslModeOptions" />
              </n-form-item>
            </template>

            <!-- MySQL Settings -->
            <template v-else-if="form.database.driver === 'mysql'">
              <n-alert type="info" :show-icon="false" style="margin-bottom: 16px; font-size: 13px;">
                {{ $t('setup.dbMysqlDesc') }}
              </n-alert>
              <n-grid cols="2" :x-gap="12">
                <n-grid-item>
                  <n-form-item :label="$t('setup.dbHost')">
                    <n-input v-model:value="dbFields.host" placeholder="localhost" />
                  </n-form-item>
                </n-grid-item>
                <n-grid-item>
                  <n-form-item :label="$t('setup.dbPort')">
                    <n-input-number v-model:value="dbFields.mysqlPort" :min="1" :max="65535" style="width: 100%;" />
                  </n-form-item>
                </n-grid-item>
              </n-grid>
              <n-form-item :label="$t('setup.dbName')">
                <n-input v-model:value="dbFields.dbName" placeholder="utopiaimages" />
              </n-form-item>
              <n-grid cols="2" :x-gap="12">
                <n-grid-item>
                  <n-form-item :label="$t('setup.dbUser')">
                    <n-input v-model:value="dbFields.user" placeholder="root" />
                  </n-form-item>
                </n-grid-item>
                <n-grid-item>
                  <n-form-item :label="$t('setup.dbPassword')">
                    <n-input v-model:value="dbFields.password" type="password" show-password-on="click" />
                  </n-form-item>
                </n-grid-item>
              </n-grid>
              <n-form-item :label="$t('setup.dbCharset')">
                <n-input v-model:value="dbFields.charset" placeholder="utf8mb4" />
              </n-form-item>
            </template>

            <div style="display: flex; justify-content: flex-end; margin-top: 8px;">
              <n-button secondary size="small" :loading="testingDb" @click="handleTestDb">
                {{ $t('setup.testDb') }}
              </n-button>
            </div>
          </n-form>
        </div>

        <!-- Step 3: SMTP Mail Settings (Optional) -->
        <div v-else-if="currentStep === 3">
          <n-alert type="warning" :show-icon="false" style="margin-bottom: 16px; font-size: 13px;">
            {{ $t('setup.smtpOptionalTip') }}
          </n-alert>

          <n-form label-placement="left" label-width="140">
            <n-form-item :label="$t('setup.enableSmtp')">
              <n-switch v-model:value="form.smtp.enabled" />
            </n-form-item>

            <template v-if="form.smtp.enabled">
              <n-grid cols="2" :x-gap="12">
                <n-grid-item>
                  <n-form-item :label="$t('setup.smtpHost')">
                    <n-input v-model:value="form.smtp.host" placeholder="smtp.example.com" />
                  </n-form-item>
                </n-grid-item>
                <n-grid-item>
                  <n-form-item :label="$t('setup.smtpPort')">
                    <n-input-number v-model:value="form.smtp.port" :min="1" :max="65535" style="width: 100%;" />
                  </n-form-item>
                </n-grid-item>
              </n-grid>
              <n-grid cols="2" :x-gap="12">
                <n-grid-item>
                  <n-form-item :label="$t('setup.smtpFrom')">
                    <n-input v-model:value="form.smtp.from" placeholder="noreply@example.com" />
                  </n-form-item>
                </n-grid-item>
                <n-grid-item>
                  <n-form-item :label="$t('setup.smtpFromName')">
                    <n-input v-model:value="form.smtp.from_name" placeholder="Utopia Images" />
                  </n-form-item>
                </n-grid-item>
              </n-grid>
              <n-grid cols="2" :x-gap="12">
                <n-grid-item>
                  <n-form-item :label="$t('setup.smtpUser')">
                    <n-input v-model:value="form.smtp.user" placeholder="username@example.com" />
                  </n-form-item>
                </n-grid-item>
                <n-grid-item>
                  <n-form-item :label="$t('setup.smtpPassword')">
                    <n-input v-model:value="form.smtp.password" type="password" show-password-on="click" />
                  </n-form-item>
                </n-grid-item>
              </n-grid>
              <n-form-item :label="$t('setup.smtpEncryption')">
                <n-select v-model:value="form.smtp.encryption" :options="encryptionOptions" />
              </n-form-item>

              <div style="display: flex; justify-content: flex-end; margin-top: 8px;">
                <n-button secondary size="small" :loading="testingSmtp" @click="handleTestSmtp">
                  {{ $t('setup.testSmtp') }}
                </n-button>
              </div>
            </template>
          </n-form>
        </div>

        <!-- Step 4: Administrator Account -->
        <div v-else-if="currentStep === 4">
          <n-alert type="info" :show-icon="false" style="margin-bottom: 16px; font-size: 13px;">
            {{ $t('setup.adminNotice') }}
          </n-alert>

          <n-form label-placement="left" label-width="140">
            <n-form-item :label="$t('setup.adminUsername') + ' *'">
              <n-input v-model:value="form.admin.username" placeholder="admin" />
            </n-form-item>
            <n-form-item :label="$t('setup.adminNickname') + ' *'">
              <n-input v-model:value="form.admin.nickname" :placeholder="$t('setup.nicknamePlaceholder')" />
            </n-form-item>
            <n-form-item :label="$t('setup.adminEmail') + ' *'">
              <n-input v-model:value="form.admin.email" :placeholder="$t('setup.adminEmailPlaceholder')" />
            </n-form-item>
            <n-form-item :label="$t('setup.adminPassword') + ' *'">
              <n-input v-model:value="form.admin.password" type="password" show-password-on="click" :placeholder="$t('setup.adminPasswordPlaceholder')" />
            </n-form-item>
            <n-form-item :label="$t('setup.confirmPassword') + ' *'">
              <n-input v-model:value="adminConfirmPassword" type="password" show-password-on="click" :placeholder="$t('setup.confirmPassword')" />
            </n-form-item>
          </n-form>
        </div>

        <!-- Footer Navigation Buttons -->
        <div class="setup-actions">
          <n-button v-if="currentStep > 1" @click="prevStep">
            {{ $t('setup.prevStep') }}
          </n-button>
          <div style="flex: 1;"></div>
          <n-button v-if="currentStep < 4" type="primary" @click="nextStep">
            {{ $t('setup.nextStep') }}
          </n-button>
          <n-button v-else type="primary" :loading="loading" @click="handleFinishSetup">
            {{ $t('setup.finish') }}
          </n-button>
        </div>
      </n-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'

const router = useRouter()
const message = useMessage()
const { t, locale } = useI18n()

const currentLocale = ref(locale.value)
const currentStep = ref(1)
const loading = ref(false)
const testingDb = ref(false)
const testingSmtp = ref(false)

const localeOptions = [
  { label: '中文', value: 'zh-CN' },
  { label: 'English', value: 'en-US' },
  { label: '日本語', value: 'ja-JP' }
]

const sslModeOptions = computed(() => [
  { label: 'disable', value: 'disable' },
  { label: 'require', value: 'require' }
])

const encryptionOptions = computed(() => [
  { label: 'SSL / TLS', value: 'ssl' },
  { label: 'STARTTLS', value: 'tls' },
  { label: t('common.disabled'), value: 'none' }
])

const adminConfirmPassword = ref('')

const form = ref({
  site: {
    name: 'Utopia Images',
    description: '又一个Utopia Images自部署图床',
    base_url: window.location.origin,
    locale: locale.value
  },
  database: {
    driver: 'sqlite',
    dsn: 'data/db/utopiaimages.db'
  },
  smtp: {
    enabled: false,
    host: '',
    port: 465,
    from: '',
    from_name: 'Utopia Images',
    user: '',
    password: '',
    encryption: 'ssl'
  },
  admin: {
    username: 'admin',
    nickname: '',
    email: '',
    password: ''
  }
})

const dbFields = ref({
  sqlitePath: 'data/db/utopiaimages.db',
  host: 'localhost',
  pgPort: 5432,
  mysqlPort: 3306,
  dbName: 'utopiaimages',
  user: 'postgres',
  password: '',
  sslmode: 'disable',
  charset: 'utf8mb4'
})

const changeLocale = (val: string) => {
  locale.value = val
  localStorage.setItem('uimg_locale', val)
  form.value.site.locale = val
}

const buildDsn = () => {
  const driver = form.value.database.driver
  if (driver === 'sqlite') {
    return dbFields.value.sqlitePath || 'data/db/utopiaimages.db'
  }
  if (driver === 'postgres') {
    return `host=${dbFields.value.host} port=${dbFields.value.pgPort} user=${dbFields.value.user} password=${dbFields.value.password} dbname=${dbFields.value.dbName} sslmode=${dbFields.value.sslmode}`
  }
  if (driver === 'mysql') {
    return `${dbFields.value.user}:${dbFields.value.password}@tcp(${dbFields.value.host}:${dbFields.value.mysqlPort})/${dbFields.value.dbName}?charset=${dbFields.value.charset}&parseTime=True&loc=Local`
  }
  return ''
}

const handleTestDb = async () => {
  testingDb.value = true
  const dsn = buildDsn()
  try {
    const res = await api.post('/setup/test-db', {
      driver: form.value.database.driver,
      dsn: dsn
    })
    message.success(res.data.message || t('setup.testDbSuccess'))
  } catch (err: any) {
    message.error(err.response?.data?.error || t('setup.testDbFailed'))
  } finally {
    testingDb.value = false
  }
}

const handleTestSmtp = async () => {
  if (!form.value.smtp.host) {
    message.warning(t('setup.fillSmtpHost'))
    return
  }
  testingSmtp.value = true
  try {
    const res = await api.post('/setup/test-smtp', {
      host: form.value.smtp.host,
      port: form.value.smtp.port,
      from: form.value.smtp.from,
      user: form.value.smtp.user,
      password: form.value.smtp.password,
      encryption: form.value.smtp.encryption
    })
    message.success(res.data.message || t('setup.testSmtpSuccess'))
  } catch (err: any) {
    message.error(err.response?.data?.error || t('setup.testSmtpFailed'))
  } finally {
    testingSmtp.value = false
  }
}

const nextStep = () => {
  if (currentStep.value === 1) {
    if (!form.value.site.name || !form.value.site.base_url) {
      message.warning(t('setup.fillRequired'))
      return
    }
  } else if (currentStep.value === 2) {
    form.value.database.dsn = buildDsn()
  }
  currentStep.value++
}

const prevStep = () => {
  if (currentStep.value > 1) {
    currentStep.value--
  }
}

const handleFinishSetup = async () => {
  if (!form.value.admin.username || !form.value.admin.password || !form.value.admin.nickname) {
    message.warning(t('setup.validation'))
    return
  }
  if (!form.value.admin.email || !form.value.admin.email.includes('@')) {
    message.warning(t('setup.emailRequired'))
    return
  }
  if (form.value.admin.password.length < 8) {
    message.warning(t('setup.adminPasswordTip'))
    return
  }
  if (form.value.admin.password !== adminConfirmPassword.value) {
    message.warning(t('setup.passwordMismatch'))
    return
  }

  form.value.database.dsn = buildDsn()

  loading.value = true
  try {
    await api.post('/setup', form.value)
    message.success(t('setup.success'))
    router.push('/login')
  } catch (err: any) {
    message.error(err.response?.data?.error || t('setup.failed'))
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  try {
    const res = await api.get('/setup/status')
    if (res.data.base_url) {
      form.value.site.base_url = res.data.base_url
    }
    if (res.data.site_name) {
      form.value.site.name = res.data.site_name
    }
  } catch {}
})
</script>

<style scoped>
.setup-container {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background-color: #101014;
  padding: 24px;
}

.setup-card-wrapper {
  width: 100%;
  max-width: 680px;
}

.setup-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.setup-brand {
  display: flex;
  align-items: center;
  gap: 12px;
}

.setup-logo {
  width: 40px;
  height: 40px;
  border-radius: 8px;
  background: linear-gradient(135deg, #63e2b7 0%, #18a058 100%);
  color: #0c0d12;
  font-weight: 800;
  font-size: 18px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.setup-title {
  font-weight: 700;
  font-size: 18px;
  color: #fff;
}

.setup-subtitle {
  font-size: 12px;
  color: #888;
}

.setup-main-card {
  border-radius: 12px;
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.6);
  background: #18181c;
}

.step-guide {
  color: #bbb;
  font-size: 13px;
  margin-bottom: 16px;
  line-height: 1.6;
}

.setup-actions {
  display: flex;
  align-items: center;
  margin-top: 28px;
  padding-top: 16px;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
}
</style>
