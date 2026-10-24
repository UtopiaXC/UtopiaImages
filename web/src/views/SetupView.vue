<template>
  <div style="height: 100vh; display: flex; align-items: center; justify-content: center; background-color: #101014;">
    <n-card :title="'Utopia Images ' + $t('setup.title')" style="max-width: 440px; border-radius: 12px; box-shadow: 0 8px 24px rgba(0,0,0,0.5);">
      <p style="color: #999; margin-bottom: 20px;">{{ $t('setup.welcome') }}</p>
      <n-form ref="formRef" :model="form" :rules="rules">
        <n-form-item :label="$t('setup.adminUsername')" path="username">
          <n-input v-model:value="form.username" :placeholder="$t('setup.adminUsernamePlaceholder')" />
        </n-form-item>
        <n-form-item :label="$t('setup.adminPassword')" path="password">
          <n-input v-model:value="form.password" type="password" show-password-on="click" :placeholder="$t('setup.adminPasswordPlaceholder')" />
        </n-form-item>
        <n-form-item :label="$t('setup.nickname')" path="nickname">
          <n-input v-model:value="form.nickname" :placeholder="$t('setup.nicknamePlaceholder')" />
        </n-form-item>
        <n-form-item :label="$t('setup.email')" path="email">
          <n-input v-model:value="form.email" :placeholder="$t('setup.emailPlaceholder')" />
        </n-form-item>
        <n-button type="primary" block :loading="loading" @click="handleSetup">
          {{ $t('setup.submit') }}
        </n-button>
      </n-form>
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
  username: 'admin',
  password: '',
  nickname: '',
  email: ''
})

const rules = {
  username: { required: true, message: t('setup.adminUsernamePlaceholder'), trigger: 'blur' },
  password: { required: true, message: t('setup.adminPasswordPlaceholder'), trigger: 'blur' }
}

const handleSetup = async () => {
  if (!form.value.username || !form.value.password) {
    message.warning(t('setup.validation'))
    return
  }
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
</script>
