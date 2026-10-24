<template>
  <div>
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px;">
      <div style="font-size: 18px; font-weight: bold;">{{ $t('adminUsers.title') }}</div>
      <n-button type="primary" @click="showCreateModal = true">{{ $t('adminUsers.new') }}</n-button>
    </div>

    <n-card style="border-radius: 12px;">
      <n-table :bordered="false" :single-line="false">
        <thead>
          <tr>
            <th>ID</th>
            <th>{{ $t('adminUsers.urlNamespace') }}</th>
            <th>{{ $t('adminUsers.nickname') }}</th>
            <th>{{ $t('adminUsers.role') }}</th>
            <th>{{ $t('adminUsers.quota') }}</th>
            <th>{{ $t('common.status') }}</th>
            <th>{{ $t('common.createdAt') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="u in users" :key="u.id">
            <td>{{ u.id }}</td>
            <td><code>/{{ u.username }}/</code></td>
            <td>{{ u.nickname || '-' }}</td>
            <td>
              <n-tag :type="u.role === 'admin' ? 'warning' : 'info'" size="small">
                {{ u.role === 'admin' ? $t('adminUsers.roleAdmin') : $t('adminUsers.roleUser') }}
              </n-tag>
            </td>
            <td>{{ formatBytes(u.used_bytes) }} / {{ u.storage_limit ? formatBytes(u.storage_limit) : $t('adminUsers.unlimited') }}</td>
            <td>
              <n-tag :type="u.status === 'active' ? 'success' : 'error'" size="small">
                {{ u.status === 'active' ? $t('common.enabled') : $t('common.disabled') }}
              </n-tag>
            </td>
            <td>{{ formatDate(u.created_at) }}</td>
          </tr>
        </tbody>
      </n-table>
    </n-card>

    <!-- Create User Modal -->
    <n-modal v-model:show="showCreateModal" preset="dialog" :title="$t('adminUsers.createTitle')">
      <n-form :model="newUserForm">
        <n-form-item :label="$t('adminUsers.username')">
          <n-input v-model:value="newUserForm.username" :placeholder="$t('adminUsers.usernameTip')" />
        </n-form-item>
        <n-form-item :label="$t('adminUsers.initialPassword')">
          <n-input v-model:value="newUserForm.password" type="password" :placeholder="$t('adminUsers.passwordPlaceholder')" />
        </n-form-item>
        <n-form-item :label="$t('adminUsers.nickname')">
          <n-input v-model:value="newUserForm.nickname" :placeholder="$t('adminUsers.nicknamePlaceholder')" />
        </n-form-item>
        <n-form-item :label="$t('setup.email')">
          <n-input v-model:value="newUserForm.email" :placeholder="$t('adminUsers.emailPlaceholder')" />
        </n-form-item>
        <n-form-item :label="$t('adminUsers.quota')">
          <n-input-number v-model:value="newUserQuotaMB" :placeholder="$t('adminUsers.quotaPlaceholder')" style="width: 100%;" :min="1" />
        </n-form-item>
        <n-form-item :label="$t('adminUsers.role')">
          <n-select
            v-model:value="newUserForm.role"
            :options="roleOptions"
          />
        </n-form-item>
      </n-form>
      <template #action>
        <n-button @click="showCreateModal = false">{{ $t('common.cancel') }}</n-button>
        <n-button type="primary" :loading="creating" @click="handleCreateUser">{{ $t('common.confirm') }}</n-button>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'

const message = useMessage()
const { t } = useI18n()
const users = ref<any[]>([])
const showCreateModal = ref(false)
const creating = ref(false)
const newUserQuotaMB = ref<number | null>(null)

const newUserForm = ref({
  username: '',
  password: '',
  nickname: '',
  email: '',
  role: 'user'
})

const roleOptions = computed(() => [
  { label: t('adminUsers.roleUser'), value: 'user' },
  { label: t('adminUsers.roleAdmin'), value: 'admin' }
])

const loadUsers = async () => {
  try {
    const res = await api.get('/admin/users')
    users.value = res.data
  } catch {}
}

const handleCreateUser = async () => {
  if (!newUserForm.value.username || !newUserForm.value.password) {
    message.warning(t('setup.validation'))
    return
  }

  creating.value = true
  try {
    const payload: any = { ...newUserForm.value }
    if (newUserQuotaMB.value && newUserQuotaMB.value > 0) {
      payload.storage_limit = newUserQuotaMB.value * 1024 * 1024
    }
    await api.post('/admin/users', payload)
    message.success(t('adminUsers.createSuccess'))
    showCreateModal.value = false
    newUserForm.value = { username: '', password: '', nickname: '', email: '', role: 'user' }
    newUserQuotaMB.value = null
    loadUsers()
  } catch (err: any) {
    message.error(err.response?.data?.error || t('common.save'))
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
