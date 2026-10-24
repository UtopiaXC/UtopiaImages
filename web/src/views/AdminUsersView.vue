<template>
  <div>
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px;">
      <div style="font-size: 18px; font-weight: bold;">用户体系管理</div>
      <n-button type="primary" @click="showCreateModal = true">添加新用户</n-button>
    </div>

    <n-card style="border-radius: 12px;">
      <n-table :bordered="false" :single-line="false">
        <thead>
          <tr>
            <th>ID</th>
            <th>用户名 (URL前缀)</th>
            <th>昵称</th>
            <th>角色 / 分组</th>
            <th>已用存储 / 配额</th>
            <th>状态</th>
            <th>创建时间</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="u in users" :key="u.id">
            <td>{{ u.id }}</td>
            <td><code>/{{ u.username }}/</code></td>
            <td>{{ u.nickname || '-' }}</td>
            <td>
              <n-tag :type="u.role === 'admin' ? 'warning' : 'info'" size="small">
                {{ u.role }} ({{ u.group ? u.group.name : '默认' }})
              </n-tag>
            </td>
            <td>{{ formatBytes(u.used_bytes) }} / {{ u.storage_limit ? formatBytes(u.storage_limit) : '无限制' }}</td>
            <td>
              <n-tag :type="u.status === 'active' ? 'success' : 'error'" size="small">
                {{ u.status }}
              </n-tag>
            </td>
            <td>{{ formatDate(u.created_at) }}</td>
          </tr>
        </tbody>
      </n-table>
    </n-card>

    <!-- Create User Modal -->
    <n-modal v-model:show="showCreateModal" preset="dialog" title="添加新用户">
      <n-form :model="newUserForm">
        <n-form-item label="用户名 (作为 URL 路径前缀，不可随意变动)">
          <n-input v-model:value="newUserForm.username" placeholder="如：alice" />
        </n-form-item>
        <n-form-item label="初始密码">
          <n-input v-model:value="newUserForm.password" type="password" placeholder="至少6位" />
        </n-form-item>
        <n-form-item label="显示昵称">
          <n-input v-model:value="newUserForm.nickname" placeholder="如：爱丽丝" />
        </n-form-item>
        <n-form-item label="邮箱">
          <n-input v-model:value="newUserForm.email" placeholder="alice@example.com" />
        </n-form-item>
        <n-form-item label="角色">
          <n-select
            v-model:value="newUserForm.role"
            :options="[
              { label: '普通上传者 (user)', value: 'user' },
              { label: '管理员 (admin)', value: 'admin' }
            ]"
          />
        </n-form-item>
      </n-form>
      <template #action>
        <n-button @click="showCreateModal = false">取消</n-button>
        <n-button type="primary" :loading="creating" @click="handleCreateUser">确认创建</n-button>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useMessage } from 'naive-ui'
import { api } from '../api/client'

const message = useMessage()
const users = ref<any[]>([])
const showCreateModal = ref(false)
const creating = ref(false)

const newUserForm = ref({
  username: '',
  password: '',
  nickname: '',
  email: '',
  role: 'user'
})

const loadUsers = async () => {
  try {
    const res = await api.get('/admin/users')
    users.value = res.data
  } catch {}
}

const handleCreateUser = async () => {
  if (!newUserForm.value.username || !newUserForm.value.password) {
    message.warning('请填写用户名与密码')
    return
  }

  creating.value = true
  try {
    await api.post('/admin/users', newUserForm.value)
    message.success('用户创建成功')
    showCreateModal.value = false
    newUserForm.value = { username: '', password: '', nickname: '', email: '', role: 'user' }
    loadUsers()
  } catch (err: any) {
    message.error(err.response?.data?.error || '创建失败')
  } finally {
    creating.value = false
  }
}

const formatBytes = (bytes: number) => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

const formatDate = (isoStr: string) => {
  if (!isoStr) return ''
  const d = new Date(isoStr)
  return `${d.getFullYear()}-${d.getMonth() + 1}-${d.getDate()}`
}

onMounted(() => {
  loadUsers()
})
</script>
