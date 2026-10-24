<template>
  <div style="height: 100vh; display: flex; align-items: center; justify-content: center; background-color: #101014;">
    <n-card title="UtopiaImages 系统初始化" style="max-width: 440px; border-radius: 12px; box-shadow: 0 8px 24px rgba(0,0,0,0.5);">
      <p style="color: #999; margin-bottom: 20px;">欢迎使用 UtopiaImages！请创建首个系统管理员账号：</p>
      <n-form ref="formRef" :model="form" :rules="rules">
        <n-form-item label="管理员用户名" path="username">
          <n-input v-model:value="form.username" placeholder="建议使用 admin 或您的用户名" />
        </n-form-item>
        <n-form-item label="管理员密码" path="password">
          <n-input v-model:value="form.password" type="password" show-password-on="click" placeholder="至少8位字符" />
        </n-form-item>
        <n-form-item label="显示昵称" path="nickname">
          <n-input v-model:value="form.nickname" placeholder="例如：站长" />
        </n-form-item>
        <n-form-item label="邮箱（可选）" path="email">
          <n-input v-model:value="form.email" placeholder="admin@example.com" />
        </n-form-item>
        <n-button type="primary" block :loading="loading" @click="handleSetup">
          完成初始化并进入系统
        </n-button>
      </n-form>
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
  username: 'admin',
  password: '',
  nickname: '站长',
  email: ''
})

const rules = {
  username: { required: true, message: '请输入管理员用户名', trigger: 'blur' },
  password: { required: true, message: '请输入管理员密码', trigger: 'blur' }
}

const handleSetup = async () => {
  if (!form.value.username || !form.value.password) {
    message.warning('请填写完整的管理员信息')
    return
  }
  loading.value = true
  try {
    await api.post('/setup', form.value)
    message.success('管理员创建成功！请登录')
    router.push('/login')
  } catch (err: any) {
    message.error(err.response?.data?.error || '初始化失败')
  } finally {
    loading.value = false
  }
}
</script>
