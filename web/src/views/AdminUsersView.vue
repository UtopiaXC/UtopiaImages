<template>
  <div>
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px;">
      <div>
        <div style="font-size: 18px; font-weight: bold;">{{ $t('adminUsers.title') }}</div>
        <div style="font-size: 13px; color: #888; margin-top: 4px;">{{ $t('adminUsers.subtitle') }}</div>
      </div>
      <n-button type="primary" @click="openCreateModal">{{ $t('adminUsers.new') }}</n-button>
    </div>

    <n-card style="border-radius: 12px;">
      <div class="table-responsive">
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
            <th style="text-align: right;">{{ $t('common.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="u in users" :key="u.id">
            <td>{{ u.id }}</td>
            <td><code>/{{ u.username }}/</code></td>
            <td>{{ u.nickname || '-' }}</td>
            <td>
              <n-tag :type="u.role?.name === 'admin' ? 'warning' : 'info'" size="small">
                {{ u.role?.display_name || u.role?.name || $t('adminUsers.roleUser') }}
              </n-tag>
            </td>
            <td>
              {{ formatBytes(u.used_bytes) }} / {{ u.role?.quota_bytes ? formatBytes(u.role.quota_bytes) : $t('adminUsers.unlimited') }}
            </td>
            <td>
              <n-tag v-if="u.status === 'banned'" type="error" size="small">
                {{ $t('adminUsers.statusBanned') }}
              </n-tag>
              <n-tag v-else-if="u.status === 'frozen' || (u.lockout_until && new Date(u.lockout_until) > new Date())" type="warning" size="small">
                {{ $t('adminUsers.statusFrozen') }}
              </n-tag>
              <n-tag v-else type="success" size="small">
                {{ $t('adminUsers.statusActive') }}
              </n-tag>
            </td>
            <td>{{ formatDate(u.created_at) }}</td>
            <td style="text-align: right;">
              <div style="display: flex; gap: 8px; justify-content: flex-end;">
                <n-button size="tiny" quaternary type="primary" @click="openEditModal(u)">
                  {{ $t('common.edit') }}
                </n-button>
                <n-popconfirm
                  v-if="u.status === 'active' && u.id !== 1 && u.role?.name !== 'admin'"
                  @positive-click="handleBanUser(u.id)"
                >
                  <template #trigger>
                    <n-button size="tiny" quaternary type="warning">
                      {{ $t('adminUsers.ban') }}
                    </n-button>
                  </template>
                  {{ $t('adminUsers.confirmBan') }}
                </n-popconfirm>
                <n-popconfirm
                  v-else-if="u.status === 'banned' || u.status === 'frozen' || (u.lockout_until && new Date(u.lockout_until) > new Date())"
                  @positive-click="handleUnfreezeUser(u.id)"
                >
                  <template #trigger>
                    <n-button size="tiny" quaternary type="success">
                      {{ $t('adminUsers.unfreeze') }}
                    </n-button>
                  </template>
                  {{ $t('adminUsers.confirmUnfreeze') }}
                </n-popconfirm>
                <n-popconfirm @positive-click="handleDeleteUser(u.id)">
                  <template #trigger>
                    <n-button size="tiny" quaternary type="error" :disabled="u.id === 1 || u.role?.name === 'admin'">
                      {{ $t('common.delete') }}
                    </n-button>
                  </template>
                  {{ $t('adminUsers.confirmDelete') }}
                </n-popconfirm>
              </div>
            </td>
          </tr>
        </tbody>
      </n-table>
      </div>
    </n-card>

    <!-- Create User Modal -->
    <n-modal v-model:show="showCreateModal" preset="card" :title="$t('adminUsers.createTitle')" style="width: 520px; max-width: 95vw; border-radius: 12px;">
      <n-form :model="newUserForm" label-placement="left" label-width="110">
        <n-form-item :label="$t('adminUsers.username')">
          <n-input v-model:value="newUserForm.username" :placeholder="$t('adminUsers.usernameTip')" />
        </n-form-item>
        <n-form-item :label="$t('adminUsers.initialPassword')">
          <n-input v-model:value="newUserForm.password" type="password" show-password-on="click" :placeholder="$t('adminUsers.passwordPlaceholder')" />
        </n-form-item>
        <n-form-item :label="$t('adminUsers.nickname')">
          <n-input v-model:value="newUserForm.nickname" :placeholder="$t('adminUsers.nicknamePlaceholder')" />
        </n-form-item>
        <n-form-item :label="$t('setup.email')">
          <n-input v-model:value="newUserForm.email" :placeholder="$t('adminUsers.emailPlaceholder')" />
        </n-form-item>
        <n-form-item :label="$t('adminUsers.role')">
          <n-select
            v-model:value="newUserForm.role_id"
            :options="roleOptions"
          />
        </n-form-item>
      </n-form>
      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 12px;">
          <n-button @click="showCreateModal = false">{{ $t('common.cancel') }}</n-button>
          <n-button type="primary" :loading="creating" @click="handleCreateUser">{{ $t('common.confirm') }}</n-button>
        </div>
      </template>
    </n-modal>

    <!-- Edit User Modal -->
    <n-modal v-model:show="showEditModal" preset="card" :title="$t('adminUsers.editTitle')" style="width: 520px; max-width: 95vw; border-radius: 12px;">
      <n-form :model="editUserForm" label-placement="left" label-width="110">
        <n-form-item :label="$t('adminUsers.username')">
          <n-input :value="editTargetUser?.username" disabled />
        </n-form-item>
        <n-form-item :label="$t('adminUsers.nickname')">
          <n-input v-model:value="editUserForm.nickname" />
        </n-form-item>
        <n-form-item :label="$t('setup.email')">
          <n-input v-model:value="editUserForm.email" />
        </n-form-item>
        <n-form-item :label="$t('adminUsers.role')">
          <n-select
            v-model:value="editUserForm.role_id"
            :options="roleOptions"
          />
        </n-form-item>
        <n-form-item :label="$t('common.status')">
          <n-select
            v-model:value="editUserForm.status"
            :options="statusOptions"
          />
        </n-form-item>
        <n-form-item :label="$t('adminUsers.resetPassword')">
          <n-input v-model:value="editUserForm.password" type="password" show-password-on="click" :placeholder="$t('adminUsers.resetPasswordPlaceholder')" />
        </n-form-item>
      </n-form>
      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 12px;">
          <n-button @click="showEditModal = false">{{ $t('common.cancel') }}</n-button>
          <n-button type="primary" :loading="savingEdit" @click="handleSaveEditUser">{{ $t('common.save') }}</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { api, getApiErrorMessage } from '../api/client'

const message = useMessage()
const { t } = useI18n()
const users = ref<any[]>([])
const roles = ref<any[]>([])

const showCreateModal = ref(false)
const creating = ref(false)

const newUserForm = ref({
  username: '',
  password: '',
  nickname: '',
  email: '',
  role_id: 2
})

const showEditModal = ref(false)
const savingEdit = ref(false)
const editTargetUser = ref<any>(null)

const editUserForm = ref({
  nickname: '',
  email: '',
  role_id: 2,
  status: 'active',
  password: ''
})

const roleOptions = computed(() => {
  return roles.value.map(r => ({
    label: `${r.display_name} · ${r.name}`,
    value: r.id
  }))
})

const statusOptions = computed(() => [
  { label: t('adminUsers.statusActive'), value: 'active' },
  { label: t('adminUsers.statusFrozen'), value: 'frozen' },
  { label: t('adminUsers.statusBanned'), value: 'banned' }
])

const loadData = async () => {
  try {
    const [uRes, rRes] = await Promise.all([
      api.get('/admin/users'),
      api.get('/admin/roles')
    ])
    users.value = uRes.data || []
    roles.value = rRes.data || []
  } catch {}
}

const openCreateModal = () => {
  newUserForm.value = {
    username: '',
    password: '',
    nickname: '',
    email: '',
    role_id: roles.value.find(r => r.name === 'user')?.id || roles.value[0]?.id || 2
  }
  showCreateModal.value = true
}

const handleCreateUser = async () => {
  if (!newUserForm.value.username || !newUserForm.value.password) {
    message.warning(t('setup.validation'))
    return
  }

  creating.value = true
  try {
    await api.post('/admin/users', newUserForm.value)
    message.success(t('adminUsers.createSuccess'))
    showCreateModal.value = false
    loadData()
  } catch (err: any) {
    message.error(err.response?.data?.error || t('common.save'))
  } finally {
    creating.value = false
  }
}

const openEditModal = (u: any) => {
  editTargetUser.value = u
  editUserForm.value = {
    nickname: u.nickname || '',
    email: u.email || '',
    role_id: u.role_id || u.role?.id || 2,
    status: u.status || 'active',
    password: ''
  }
  showEditModal.value = true
}

const handleSaveEditUser = async () => {
  if (!editTargetUser.value) return
  savingEdit.value = true
  try {
    await api.patch(`/admin/users/${editTargetUser.value.id}`, editUserForm.value)
    message.success(t('adminUsers.updateSuccess'))
    showEditModal.value = false
    loadData()
  } catch (err: any) {
    message.error(err.response?.data?.error || t('common.save'))
  } finally {
    savingEdit.value = false
  }
}

const handleBanUser = async (id: number) => {
  try {
    await api.post(`/admin/users/${id}/ban`)
    message.success(t('adminUsers.banSuccess'))
    loadData()
  } catch (err: any) {
    message.error(getApiErrorMessage(err))
  }
}

const handleUnfreezeUser = async (id: number) => {
  try {
    await api.post(`/admin/users/${id}/unfreeze`)
    message.success(t('adminUsers.unfreezeSuccess'))
    loadData()
  } catch (err: any) {
    message.error(getApiErrorMessage(err))
  }
}

const handleDeleteUser = async (id: number) => {
  try {
    await api.delete(`/admin/users/${id}`)
    message.success(t('adminUsers.deleteSuccess'))
    loadData()
  } catch (err: any) {
    message.error(getApiErrorMessage(err))
  }
}

const formatBytes = (bytes: number) => {
  if (!bytes || bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

const formatDate = (isoStr: string) => {
  if (!isoStr) return ''
  const d = new Date(isoStr)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

onMounted(loadData)
</script>
