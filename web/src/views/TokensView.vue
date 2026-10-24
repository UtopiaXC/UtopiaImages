<template>
  <div style="max-width: 800px; margin: 0 auto;">
    <n-card title="API 个人访问令牌 (PAT)" style="border-radius: 12px; margin-bottom: 24px;">
      <p style="color: #999; margin-bottom: 20px;">
        可用于 PicGo、自动化脚本、外部系统调用 UtopiaImages 上传和管理接口。
      </p>

      <div style="display: flex; gap: 12px; margin-bottom: 20px;">
        <n-input v-model:value="newTokenName" placeholder="令牌备注名称（如：PicGo 客户端）" />
        <n-button type="primary" :loading="creating" @click="handleCreate">创建新令牌</n-button>
      </div>

      <n-alert v-if="justCreatedToken" type="success" title="请妥善复制并保存您的新令牌（仅显示一次）：" style="margin-bottom: 20px;">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 8px;">
          <code style="font-size: 14px; background: rgba(0,0,0,0.3); padding: 4px 8px; border-radius: 4px;">{{ justCreatedToken }}</code>
          <n-button size="small" type="primary" @click="copyToken(justCreatedToken)">复制</n-button>
        </div>
      </n-alert>

      <n-table :bordered="false" :single-line="false">
        <thead>
          <tr>
            <th>名称</th>
            <th>前缀</th>
            <th>创建时间</th>
            <th>最后使用</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="t in tokens" :key="t.id">
            <td>{{ t.name }}</td>
            <td><code>{{ t.prefix }}***</code></td>
            <td>{{ formatDate(t.created_at) }}</td>
            <td>{{ t.last_used_at ? formatDate(t.last_used_at) : '从未' }}</td>
            <td>
              <n-popconfirm @positive-click="deleteToken(t.id)">
                <template #trigger>
                  <n-button size="tiny" type="error" quaternary>吊销</n-button>
                </template>
                确认吊销此令牌？吊销后使用此令牌的客户端将无法访问。
              </n-popconfirm>
            </td>
          </tr>
        </tbody>
      </n-table>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useMessage } from 'naive-ui'
import { api } from '../api/client'

const message = useMessage()
const tokens = ref<any[]>([])
const newTokenName = ref('')
const creating = ref(false)
const justCreatedToken = ref('')

const loadTokens = async () => {
  try {
    const res = await api.get('/me/tokens')
    tokens.value = res.data
  } catch {}
}

const handleCreate = async () => {
  if (!newTokenName.value) {
    message.warning('请输入令牌名称')
    return
  }

  creating.value = true
  try {
    const res = await api.post('/me/tokens', {
      name: newTokenName.value,
      expires_days: 0 // permanent
    })
    justCreatedToken.value = res.data.token
    newTokenName.value = ''
    loadTokens()
    message.success('令牌创建成功')
  } catch (err: any) {
    message.error(err.response?.data?.error || '创建失败')
  } finally {
    creating.value = false
  }
}

const deleteToken = async (id: number) => {
  try {
    await api.delete(`/me/tokens/${id}`)
    message.success('已吊销')
    loadTokens()
  } catch {}
}

const copyToken = (text: string) => {
  navigator.clipboard.writeText(text)
  message.success('已复制到剪贴板')
}

const formatDate = (isoStr: string) => {
  if (!isoStr) return ''
  const d = new Date(isoStr)
  return `${d.getFullYear()}-${d.getMonth() + 1}-${d.getDate()}`
}

onMounted(() => {
  loadTokens()
})
</script>
