<template>
  <div style="min-height: 100vh; display: flex; align-items: center; justify-content: center; background: radial-gradient(circle at 50% 20%, #1a1a24 0%, #0d0d12 100%); padding: 24px;">
    <n-card
      :title="cardTitle"
      style="max-width: 440px; width: 100%; border-radius: 16px; box-shadow: 0 12px 36px rgba(0,0,0,0.6); background: rgba(24, 24, 28, 0.85); backdrop-filter: blur(12px); border: 1px solid rgba(255,255,255,0.08);"
      :bordered="false"
    >
      <!-- Mode: Login -->
      <div v-if="mode === 'login'">
        <div v-if="!mfaRequired">
          <n-form :model="form" @submit.prevent="handleLogin">
            <n-form-item :label="$t('login.account')">
              <n-input v-model:value="form.username" :placeholder="$t('login.accountPlaceholder')" size="large" />
            </n-form-item>
            <n-form-item :label="$t('login.password')">
              <n-input
                v-model:value="form.password"
                type="password"
                show-password-on="click"
                :placeholder="$t('login.passwordPlaceholder')"
                size="large"
              />
            </n-form-item>

            <!-- Interactive CAPTCHA Challenge when triggered -->
            <div
              v-if="captchaRequired"
              style="margin-bottom: 16px; background: rgba(255,255,255,0.03); padding: 12px; border-radius: 8px; border: 1px solid rgba(255,255,255,0.08);"
            >
              <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px;">
                <span style="font-size: 13px; color: #ccc;">{{ $t('login.captchaTitle') }}</span>
                <n-button size="tiny" quaternary @click="fetchCaptchaChallenge">{{ $t('login.captchaRefresh') }}</n-button>
              </div>

              <div v-if="captchaSolved" style="color: #63e2b7; font-size: 13px; display: flex; align-items: center; gap: 6px;">
                <n-icon size="16">
                  <CheckmarkCircleOutline />
                </n-icon>
                <span>{{ $t('login.captchaVerified') }}</span>
              </div>

              <div v-else>
                <n-button size="small" type="warning" block ghost :loading="solvingCaptcha" @click="solveCaptcha">
                  {{ $t('login.captchaSolve') }}
                </n-button>
              </div>
            </div>

            <div style="margin-bottom: 16px; display: flex; justify-content: space-between; align-items: center;">
              <n-checkbox v-model:checked="form.remember_me">{{ $t('login.rememberMe') }}</n-checkbox>
              <n-button text type="primary" size="small" @click="switchMode('forgot')">
                {{ $t('login.forgotPassword') }}
              </n-button>
            </div>

            <n-button type="primary" block size="large" :loading="loading" @click="handleLogin">
              {{ $t('login.submit') }}
            </n-button>

            <!-- Passkey Login Alternative -->
            <div v-if="supportsPasskey" style="margin-top: 16px; text-align: center;">
              <n-divider style="margin: 12px 0; font-size: 12px; color: #555;">{{ $t('login.or') }}</n-divider>
              <n-button block secondary :loading="passkeyLoading" @click="handlePasskeyLogin">
                <template #icon>
                  <n-icon>
                    <FingerPrintOutline />
                  </n-icon>
                </template>
                {{ $t('login.passkeyLogin') }}
              </n-button>
            </div>

            <!-- Register Switcher if allowed -->
            <div v-if="allowRegistration" style="margin-top: 20px; text-align: center;">
              <span style="color: #777; font-size: 13px;">{{ $t('login.noAccount') }} </span>
              <n-button text type="primary" size="small" style="font-weight: 500;" @click="switchMode('register')">
                {{ $t('login.register') }}
              </n-button>
            </div>
          </n-form>
        </div>

        <div v-else>
          <p style="color: #bbb; margin-bottom: 16px;">{{ $t('login.mfaPrompt') }}</p>
          <n-form-item :label="$t('login.mfaCode')">
            <n-input v-model:value="mfaCode" :placeholder="$t('login.mfaCodePlaceholder')" maxlength="6" size="large" />
          </n-form-item>
          <n-button type="primary" block size="large" :loading="loading" @click="handleVerifyMFA">
            {{ $t('login.mfaSubmit') }}
          </n-button>
          <div style="margin-top: 14px; display: flex; justify-content: space-between; align-items: center;">
            <n-button text size="small" type="warning" @click="switchMode('forgot-2fa')">
              {{ $t('login.lost2FA') }}
            </n-button>
            <n-button text size="small" @click="mfaRequired = false">
              {{ $t('login.backToPassword') }}
            </n-button>
          </div>
        </div>
      </div>

      <!-- Mode: Register -->
      <div v-else-if="mode === 'register'">
        <n-form :model="registerForm" @submit.prevent="handleRegister">
          <n-form-item :label="$t('login.username')">
            <n-input v-model:value="registerForm.username" :placeholder="$t('login.usernamePlaceholder')" size="large" />
          </n-form-item>
          <n-form-item :label="$t('login.nickname')">
            <n-input v-model:value="registerForm.nickname" :placeholder="$t('login.nicknamePlaceholder')" size="large" />
          </n-form-item>
          <n-form-item :label="$t('login.email')">
            <n-input v-model:value="registerForm.email" :placeholder="$t('login.emailPlaceholder')" size="large" />
          </n-form-item>
          <n-form-item :label="$t('login.password')">
            <n-input
              v-model:value="registerForm.password"
              type="password"
              show-password-on="click"
              :placeholder="$t('login.newPasswordPlaceholder')"
              size="large"
            />
          </n-form-item>
          <n-form-item :label="$t('login.confirmNewPassword')">
            <n-input
              v-model:value="registerForm.confirm_password"
              type="password"
              show-password-on="click"
              :placeholder="$t('login.confirmNewPassword')"
              size="large"
            />
          </n-form-item>

          <n-button type="primary" block size="large" :loading="loading" @click="handleRegister">
            {{ $t('login.registerSubmit') }}
          </n-button>

          <div style="margin-top: 16px; text-align: center;">
            <n-button text type="primary" size="small" @click="switchMode('login')">
              {{ $t('login.haveAccount') }}
            </n-button>
          </div>
        </n-form>
      </div>

      <!-- Mode: Forgot Password -->
      <div v-else-if="mode === 'forgot'">
        <p style="color: #aaa; font-size: 13px; line-height: 1.6; margin-bottom: 16px;">
          {{ $t('login.sendResetEmailTip') }}
        </p>
        <n-form :model="forgotForm" @submit.prevent="handleForgotPassword">
          <n-form-item :label="$t('login.email')">
            <n-input v-model:value="forgotForm.email" :placeholder="$t('login.emailPlaceholder')" size="large" />
          </n-form-item>

          <n-button type="primary" block size="large" :loading="loading" @click="handleForgotPassword">
            {{ $t('login.sendResetEmail') }}
          </n-button>

          <div style="margin-top: 16px; display: flex; justify-content: space-between; align-items: center;">
            <n-button text size="small" @click="switchMode('reset')">
              {{ $t('login.resetPassword') }}
            </n-button>
            <n-button text type="primary" size="small" @click="switchMode('login')">
              {{ $t('login.backToLogin') }}
            </n-button>
          </div>
        </n-form>
      </div>

      <!-- Mode: Reset Password with Token -->
      <div v-else-if="mode === 'reset'">
        <p style="color: #aaa; font-size: 13px; line-height: 1.6; margin-bottom: 16px;">
          {{ $t('login.resetPasswordTip') }}
        </p>
        <n-form :model="resetForm" @submit.prevent="handleResetPassword">
          <n-form-item :label="$t('login.resetToken')">
            <n-input v-model:value="resetForm.token" :placeholder="$t('login.resetTokenPlaceholder')" size="large" />
          </n-form-item>
          <n-form-item :label="$t('login.newPassword')">
            <n-input
              v-model:value="resetForm.new_password"
              type="password"
              show-password-on="click"
              :placeholder="$t('login.newPasswordPlaceholder')"
              size="large"
            />
          </n-form-item>
          <n-form-item :label="$t('login.confirmNewPassword')">
            <n-input
              v-model:value="resetForm.confirm_password"
              type="password"
              show-password-on="click"
              :placeholder="$t('login.confirmNewPassword')"
              size="large"
            />
          </n-form-item>

          <n-button type="primary" block size="large" :loading="loading" @click="handleResetPassword">
            {{ $t('login.resetPassword') }}
          </n-button>

          <div style="margin-top: 16px; text-align: center;">
            <n-button text type="primary" size="small" @click="switchMode('login')">
              {{ $t('login.backToLogin') }}
            </n-button>
          </div>
        </n-form>
      </div>

      <!-- Mode: Forgot 2FA -->
      <div v-else-if="mode === 'forgot-2fa'">
        <p style="color: #aaa; font-size: 13px; line-height: 1.6; margin-bottom: 16px;">
          {{ $t('login.sendReset2FAEmailTip') }}
        </p>
        <n-form :model="forgot2FAForm" @submit.prevent="handleForgot2FA">
          <n-form-item :label="$t('login.email')">
            <n-input v-model:value="forgot2FAForm.email" :placeholder="$t('login.emailPlaceholder')" size="large" />
          </n-form-item>

          <n-button type="primary" block size="large" :loading="loading" @click="handleForgot2FA">
            {{ $t('login.sendReset2FAEmail') }}
          </n-button>

          <div style="margin-top: 16px; display: flex; justify-content: space-between; align-items: center;">
            <n-button text size="small" @click="switchMode('reset-2fa')">
              {{ $t('login.reset2FA') }}
            </n-button>
            <n-button text type="primary" size="small" @click="switchMode('login')">
              {{ $t('login.backToLogin') }}
            </n-button>
          </div>
        </n-form>
      </div>

      <!-- Mode: Reset 2FA with Token -->
      <div v-else-if="mode === 'reset-2fa'">
        <p style="color: #aaa; font-size: 13px; line-height: 1.6; margin-bottom: 16px;">
          {{ $t('login.reset2FATip') }}
        </p>
        <n-form :model="reset2FAForm" @submit.prevent="handleReset2FA">
          <n-form-item :label="$t('login.resetToken')">
            <n-input v-model:value="reset2FAForm.token" :placeholder="$t('login.reset2FATokenPlaceholder')" size="large" />
          </n-form-item>

          <n-button type="primary" block size="large" :loading="loading" @click="handleReset2FA">
            {{ $t('login.confirmReset2FA') }}
          </n-button>

          <div style="margin-top: 16px; text-align: center;">
            <n-button text type="primary" size="small" @click="switchMode('login')">
              {{ $t('login.backToLogin') }}
            </n-button>
          </div>
        </n-form>
      </div>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useMessage } from 'naive-ui'
import { FingerPrintOutline, CheckmarkCircleOutline } from '@vicons/ionicons5'
import { useI18n } from 'vue-i18n'
import { api, getApiErrorMessage } from '../api/client'

const router = useRouter()
const route = useRoute()
const message = useMessage()
const { t } = useI18n()

type Mode = 'login' | 'register' | 'forgot' | 'reset' | 'forgot-2fa' | 'reset-2fa'
const mode = ref<Mode>('login')

const loading = ref(false)
const passkeyLoading = ref(false)
const supportsPasskey = ref(false)
const allowRegistration = ref(false)

const cardTitle = computed(() => {
  if (mode.value === 'register') return t('login.register')
  if (mode.value === 'forgot') return t('login.forgotPassword')
  if (mode.value === 'reset') return t('login.resetPasswordTitle')
  if (mode.value === 'forgot-2fa') return t('login.lost2FATitle')
  if (mode.value === 'reset-2fa') return t('login.reset2FATitle')
  return 'Utopia Images'
})

const form = ref({
  username: '',
  password: '',
  remember_me: true,
  captcha: ''
})

const registerForm = ref({
  username: '',
  nickname: '',
  email: '',
  password: '',
  confirm_password: ''
})

const forgotForm = ref({
  email: ''
})

const resetForm = ref({
  token: '',
  new_password: '',
  confirm_password: ''
})

const forgot2FAForm = ref({
  email: ''
})

const reset2FAForm = ref({
  token: ''
})

const mfaRequired = ref(false)
const mfaUserID = ref<number | null>(null)
const mfaCode = ref('')

const captchaRequired = ref(false)
const captchaChallenge = ref<any>(null)
const captchaSolved = ref(false)
const solvingCaptcha = ref(false)

const switchMode = (newMode: Mode) => {
  mode.value = newMode
  captchaRequired.value = false
  captchaSolved.value = false
  form.value.captcha = ''
}

const checkSiteSettings = async () => {
  try {
    const res = await api.get('/site')
    if (res.data) {
      allowRegistration.value = !!res.data.allow_registration
    }
  } catch {}
}

const handleLogin = async () => {
  if (!form.value.username || !form.value.password) {
    message.warning(t('login.validation'))
    return
  }

  if (captchaRequired.value && !form.value.captcha) {
    message.warning(t('login.captchaRequiredTip'))
    return
  }

  loading.value = true
  try {
    const res = await api.post('/auth/login', form.value)
    if (res.data.requires_mfa) {
      mfaRequired.value = true
      mfaUserID.value = res.data.user_id
    } else {
      message.success(t('login.success'))
      router.push('/console/upload')
    }
  } catch (err: any) {
    const resData = err.response?.data
    if (resData?.requires_captcha) {
      captchaRequired.value = true
      captchaSolved.value = false
      form.value.captcha = ''
      fetchCaptchaChallenge()
    }
    message.error(getApiErrorMessage(err))
  } finally {
    loading.value = false
  }
}

const handleRegister = async () => {
  if (!registerForm.value.username || !registerForm.value.email || !registerForm.value.password) {
    message.warning(t('setup.fillRequired'))
    return
  }
  const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
  if (!emailRegex.test(registerForm.value.email)) {
    message.warning(t('setup.emailRequired'))
    return
  }
  if (registerForm.value.password.length < 8) {
    message.warning(t('errors.AUTH_PASSWORD_TOO_SHORT'))
    return
  }
  if (registerForm.value.password !== registerForm.value.confirm_password) {
    message.warning(t('errors.AUTH_PASSWORD_MISMATCH'))
    return
  }

  loading.value = true
  try {
    await api.post('/auth/register', {
      username: registerForm.value.username,
      nickname: registerForm.value.nickname,
      email: registerForm.value.email,
      password: registerForm.value.password
    })
    message.success(t('login.registerSuccessActivationTip'))
    form.value.username = registerForm.value.username
    form.value.password = ''
    switchMode('login')
  } catch (err: any) {
    message.error(getApiErrorMessage(err))
  } finally {
    loading.value = false
  }
}

const handleForgotPassword = async () => {
  if (!forgotForm.value.email) {
    message.warning(t('setup.fillRequired'))
    return
  }

  loading.value = true
  try {
    await api.post('/auth/forgot-password', {
      email: forgotForm.value.email
    })
    message.success(t('login.resetEmailSent'))
    switchMode('reset')
  } catch (err: any) {
    message.error(getApiErrorMessage(err))
  } finally {
    loading.value = false
  }
}

const handleResetPassword = async () => {
  if (!resetForm.value.token || !resetForm.value.new_password) {
    message.warning(t('setup.fillRequired'))
    return
  }
  if (resetForm.value.new_password.length < 6) {
    message.warning(t('errors.AUTH_PASSWORD_TOO_SHORT'))
    return
  }
  if (resetForm.value.new_password !== resetForm.value.confirm_password) {
    message.warning(t('errors.AUTH_PASSWORD_MISMATCH'))
    return
  }

  loading.value = true
  try {
    await api.post('/auth/reset-password', {
      token: resetForm.value.token,
      new_password: resetForm.value.new_password
    })
    message.success(t('login.resetSuccess'))
    switchMode('login')
  } catch (err: any) {
    message.error(getApiErrorMessage(err))
  } finally {
    loading.value = false
  }
}

const handleForgot2FA = async () => {
  if (!forgot2FAForm.value.email) {
    message.warning(t('setup.fillRequired'))
    return
  }

  loading.value = true
  try {
    await api.post('/auth/forgot-2fa', {
      email: forgot2FAForm.value.email
    })
    message.success(t('login.reset2FAEmailSent'))
    switchMode('reset-2fa')
  } catch (err: any) {
    message.error(getApiErrorMessage(err))
  } finally {
    loading.value = false
  }
}

const handleReset2FA = async () => {
  if (!reset2FAForm.value.token) {
    message.warning(t('setup.fillRequired'))
    return
  }

  loading.value = true
  try {
    await api.post('/auth/reset-2fa', {
      token: reset2FAForm.value.token
    })
    message.success(t('login.reset2FASuccess'))
    mfaRequired.value = false
    switchMode('login')
  } catch (err: any) {
    message.error(getApiErrorMessage(err))
  } finally {
    loading.value = false
  }
}

const fetchCaptchaChallenge = async () => {
  try {
    const res = await api.get('/auth/captcha')
    captchaChallenge.value = res.data
    captchaSolved.value = false
    form.value.captcha = ''
  } catch {}
}

// Client-side ALTCHA PoW solver
const solveCaptcha = async () => {
  if (!captchaChallenge.value) {
    await fetchCaptchaChallenge()
  }
  if (!captchaChallenge.value) return

  solvingCaptcha.value = true
  try {
    const { challenge, salt, maxnumber, algorithm, signature } = captchaChallenge.value
    const max = maxnumber || 50000
    const target = challenge.toLowerCase()

    let solvedNumber = -1
    const enc = new TextEncoder()

    // Solve SHA-256(salt + N) == challenge
    for (let n = 0; n <= max; n++) {
      const data = enc.encode(`${salt}${n}`)
      const hashBuffer = await crypto.subtle.digest('SHA-256', data)
      const hashArray = Array.from(new Uint8Array(hashBuffer))
      const hashHex = hashArray.map(b => b.toString(16).padStart(2, '0')).join('')

      if (hashHex === target) {
        solvedNumber = n
        break
      }
    }

    if (solvedNumber >= 0) {
      const solutionPayload = {
        algorithm: algorithm || 'SHA-256',
        challenge: challenge,
        number: solvedNumber,
        salt: salt,
        signature: signature
      }
      form.value.captcha = JSON.stringify(solutionPayload)
      captchaSolved.value = true
      message.success(t('login.captchaSuccess'))
    } else {
      message.error(t('login.captchaFailed'))
      fetchCaptchaChallenge()
    }
  } catch (err: any) {
    message.error(t('login.captchaFailed'))
  } finally {
    solvingCaptcha.value = false
  }
}

const handleVerifyMFA = async () => {
  if (!mfaCode.value || mfaCode.value.length !== 6) {
    message.warning(t('login.mfaValidation'))
    return
  }

  loading.value = true
  try {
    await api.post('/auth/mfa', {
      user_id: mfaUserID.value,
      code: mfaCode.value,
      remember_me: form.value.remember_me
    })
    message.success(t('login.success'))
    router.push('/console/upload')
  } catch (err: any) {
    message.error(getApiErrorMessage(err))
  } finally {
    loading.value = false
  }
}

// Passkey login helpers
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

const bufferToBase64URL = (buffer: ArrayBuffer): string => {
  const bytes = new Uint8Array(buffer)
  let binary = ''
  for (let i = 0; i < bytes.byteLength; i++) {
    binary += String.fromCharCode(bytes[i])
  }
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '')
}

const handlePasskeyLogin = async () => {
  if (!window.PublicKeyCredential) {
    message.error(t('settings.passkeyNotSupported'))
    return
  }

  passkeyLoading.value = true
  try {
    const beginRes = await api.post('/auth/passkey/login-begin', {
      username: form.value.username || ''
    })
    const { options, session_key } = beginRes.data
    const pkOptions = options.publicKey

    pkOptions.challenge = base64URLToBuffer(pkOptions.challenge)
    if (pkOptions.allowCredentials) {
      for (const cred of pkOptions.allowCredentials) {
        cred.id = base64URLToBuffer(cred.id)
      }
    }

    const assertion: any = await navigator.credentials.get({ publicKey: pkOptions })
    if (!assertion) {
      throw new Error('Passkey verification canceled')
    }

    const payload = {
      id: assertion.id,
      rawId: bufferToBase64URL(assertion.rawId),
      type: assertion.type,
      response: {
        authenticatorData: bufferToBase64URL(assertion.response.authenticatorData),
        clientDataJSON: bufferToBase64URL(assertion.response.clientDataJSON),
        signature: bufferToBase64URL(assertion.response.signature),
        userHandle: assertion.response.userHandle ? bufferToBase64URL(assertion.response.userHandle) : null
      }
    }

    await api.post(`/auth/passkey/login-finish?session_key=${session_key}`, payload)
    message.success(t('login.success'))
    router.push('/console/upload')
  } catch (err: any) {
    message.error(getApiErrorMessage(err))
  } finally {
    passkeyLoading.value = false
  }
}

const handleActivate = async (token: string) => {
  loading.value = true
  try {
    await api.post('/auth/activate', { token })
    message.success(t('login.accountActivated'))
    mode.value = 'login'
  } catch (err: any) {
    message.error(getApiErrorMessage(err))
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  checkSiteSettings()
  if (window.PublicKeyCredential) {
    supportsPasskey.value = true
  }
  if (route.query.action === 'activate' && route.query.token) {
    handleActivate(String(route.query.token))
  } else if (route.query.action === 'reset-2fa') {
    mode.value = 'reset-2fa'
    if (route.query.token) {
      reset2FAForm.value.token = String(route.query.token)
    }
  } else if (route.query.token) {
    mode.value = 'reset'
    resetForm.value.token = String(route.query.token)
  } else if (route.query.action === 'register') {
    mode.value = 'register'
  }
})
</script>
