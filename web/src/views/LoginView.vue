<template>
  <div style="height: 100vh; display: flex; align-items: center; justify-content: center; background-color: #101014;">
    <n-card title="UtopiaImages" style="max-width: 420px; border-radius: 12px; box-shadow: 0 8px 24px rgba(0,0,0,0.5);">
      <div v-if="!mfaRequired">
        <n-form :model="form" @submit.prevent="handleLogin">
          <n-form-item label="用户名 / 邮箱">
            <n-input v-model:value="form.username" placeholder="请输入用户名" />
          </n-form-item>
          <n-form-item label="密码">
            <n-input v-model:value="form.password" type="password" show-password-on="click" placeholder="请输入密码" />
          </n-form-item>
          <div style="margin-bottom: 16px;">
            <n-checkbox v-model:checked="form.remember_me">记住我 (30天内免密登录)</n-checkbox>
          </div>
          <n-button type="primary" block :loading="loading" @click="handleLogin">
            登 录
          </n-button>
        </n-form>
      </div>

      <div v-else>
        <p style="color: #bbb; margin-bottom: 16px;">已启用两步验证 (2FA)，请输入验证器 App 中的 6 位验证码：</p>
        <n-form-item label="2FA 验证码">
          <n-input v-model:value="mfaCode" placeholder="6 位动态验证码" maxlength="6" />
        </n-form-item>
        <n-button type="primary" block :loading="loading" @click="handleVerifyMFA">
          确认验证并登录
        </n-button>
        <n-button text block style="margin-top: 12px;" @click="mfaRequired = false">
          返回密码登录
        </n-button>
      </div>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useMessage } from 'naive-ui'
import { api } from '../api/client'

const router = useRouter()
const message = useMessage()
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
    message.warning('请输入用户名与密码')
    return
  }

  loading.value = true
  try {
    const res = await api.post('/auth/login', form.value)
    if (res.data.requires_mfa) {
      mfaRequired.value = true
      mfaUserID.value = res.data.user_id
    } else {
      message.success('登录成功')
      router.push('/console/upload')
    }
  } catch (err: any) {
    message.error(err.response?.data?.error || '登录失败')
  } finally {
    loading.value = false
  }
}

const handleVerifyMFA = async () => {
  if (!mfaCode.value || mfaCode.value.length !== 6) {
    message.warning('请输入 6 位有效动态验证码')
    return
  }

  loading.value = true
  try {
    await api.post('/auth/mfa', {
      user_id: mfaUserID.value,
      code: mfaCode.value,
      remember_me: form.value.remember_me
    })
    message.success('验证成功')
    router.push('/console/upload')
  } catch (err: any) {
    message.error(err.response?.data?.error || '2FA 验证失败')
  } finally {
    loading.value = false
  }
}
</script>
