<template>
  <div style="height: 100vh; display: flex; align-items: center; justify-content: center; background-color: #101014;">
    <n-card title="Utopia Images" style="max-width: 420px; border-radius: 12px; box-shadow: 0 8px 24px rgba(0,0,0,0.5);">
      <div v-if="!mfaRequired">
        <n-form :model="form" @submit.prevent="handleLogin">
          <n-form-item :label="$t('login.account')">
            <n-input v-model:value="form.username" :placeholder="$t('login.accountPlaceholder')" />
          </n-form-item>
          <n-form-item :label="$t('login.password')">
            <n-input v-model:value="form.password" type="password" show-password-on="click" :placeholder="$t('login.passwordPlaceholder')" />
          </n-form-item>
          <div style="margin-bottom: 16px;">
            <n-checkbox v-model:checked="form.remember_me">{{ $t('login.rememberMe') }}</n-checkbox>
          </div>
          <n-button type="primary" block :loading="loading" @click="handleLogin">
            {{ $t('login.submit') }}
          </n-button>
        </n-form>
      </div>

      <div v-else>
        <p style="color: #bbb; margin-bottom: 16px;">{{ $t('login.mfaPrompt') }}</p>
        <n-form-item :label="$t('login.mfaCode')">
          <n-input v-model:value="mfaCode" :placeholder="$t('login.mfaCodePlaceholder')" maxlength="6" />
        </n-form-item>
        <n-button type="primary" block :loading="loading" @click="handleVerifyMFA">
          {{ $t('login.mfaSubmit') }}
        </n-button>
        <n-button text block style="margin-top: 12px;" @click="mfaRequired = false">
          {{ $t('login.backToPassword') }}
        </n-button>
      </div>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'

const router = useRouter()
const message = useMessage()
const { t } = useI18n()
const loading = ref(false)

const form = ref({
  username: '',
  password: '',
  remember_me: true,
  captcha: ''
})

const mfaRequired = ref(false)
const mfaUserID = ref<number | null>(null)
const mfaCode = ref('')

const handleLogin = async () => {
  if (!form.value.username || !form.value.password) {
    message.warning(t('login.validation'))
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
    message.error(err.response?.data?.error || t('login.failed'))
  } finally {
    loading.value = false
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
    message.error(err.response?.data?.error || t('login.mfaFailed'))
  } finally {
    loading.value = false
  }
}
</script>
