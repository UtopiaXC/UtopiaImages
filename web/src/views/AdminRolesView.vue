<template>
  <div>
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px;">
      <div>
        <div style="font-size: 18px; font-weight: bold;">{{ $t('adminRoles.title') }}</div>
        <div style="font-size: 13px; color: #888; margin-top: 4px;">{{ $t('adminRoles.subtitle') }}</div>
      </div>
      <n-button type="primary" @click="openCreateModal">{{ $t('adminRoles.new') }}</n-button>
    </div>

    <n-card style="border-radius: 12px;">
      <div class="table-responsive">
        <n-table :bordered="false" :single-line="false">
        <thead>
          <tr>
            <th>ID</th>
            <th>{{ $t('adminRoles.roleName') }}</th>
            <th>{{ $t('adminRoles.displayName') }}</th>
            <th>{{ $t('adminRoles.userQuota') }}</th>
            <th>{{ $t('adminRoles.permittedPolicies') }}</th>
            <th>{{ $t('adminRoles.userCount') }}</th>
            <th style="text-align: right;">{{ $t('common.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in roles" :key="r.id">
            <td>{{ r.id }}</td>
            <td>
              <code>{{ r.name }}</code>
              <n-tag v-if="r.is_system || r.name === 'admin'" size="tiny" type="warning" style="margin-left: 6px;">
                {{ $t('adminRoles.systemRole') }}
              </n-tag>
            </td>
            <td><strong>{{ r.display_name }}</strong></td>
            <td>{{ r.quota_bytes === 0 ? $t('common.unlimited') : formatBytes(r.quota_bytes) }}</td>
            <td>
              <div style="display: flex; flex-wrap: wrap; gap: 4px;">
                <n-tag v-for="p in getPolicyTags(r.policy_ids)" :key="p.id" size="tiny" type="info">
                  {{ p.name }}
                </n-tag>
                <span v-if="!r.policy_ids || r.policy_ids.length === 0" style="color: #666; font-size: 12px;">-</span>
              </div>
            </td>
            <td>{{ r.user_count }}</td>
            <td style="text-align: right;">
              <div style="display: flex; gap: 8px; justify-content: flex-end;">
                <n-button size="tiny" quaternary type="primary" @click="openEditModal(r)">
                  {{ $t('common.edit') }}
                </n-button>
                <n-popconfirm @positive-click="handleDeleteRole(r.id)">
                  <template #trigger>
                    <n-button
                      size="tiny"
                      quaternary
                      type="error"
                      :disabled="r.is_system || r.name === 'admin' || r.user_count > 0"
                    >
                      {{ $t('common.delete') }}
                    </n-button>
                  </template>
                  {{ $t('adminRoles.confirmDelete') }}
                </n-popconfirm>
              </div>
            </td>
          </tr>
        </tbody>
      </n-table>
      </div>
    </n-card>

    <!-- Create Role Modal -->
    <n-modal
      v-model:show="showCreateModal"
      preset="card"
      :title="$t('adminRoles.new')"
      style="width: 540px; max-width: 95vw; border-radius: 12px;"
    >
      <n-form :model="createForm" label-placement="left" label-width="130">
        <n-form-item :label="$t('adminRoles.roleName')">
          <n-input v-model:value="createForm.name" :placeholder="$t('adminRoles.namePlaceholder')" />
        </n-form-item>

        <n-form-item :label="$t('adminRoles.displayName')">
          <n-input v-model:value="createForm.display_name" :placeholder="$t('adminRoles.displayNamePlaceholder')" />
        </n-form-item>

        <n-form-item :label="$t('adminRoles.userQuota')">
          <div style="display: flex; align-items: center; gap: 10px; width: 100%;">
            <n-input-number
              v-model:value="createQuotaGB"
              :min="0"
              :step="1"
              :placeholder="$t('adminRoles.quotaPlaceholder')"
              style="flex: 1;"
            />
            <span style="color: #aaa; font-size: 13px;">GB (0 = {{ $t('common.unlimited') }})</span>
          </div>
        </n-form-item>

        <n-form-item :label="$t('adminRoles.permittedPolicies')">
          <n-select
            v-model:value="createForm.policy_ids"
            multiple
            :options="policyOptions"
            :placeholder="$t('adminRoles.policiesPlaceholder')"
          />
        </n-form-item>
      </n-form>

      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 12px;">
          <n-button @click="showCreateModal = false">{{ $t('common.cancel') }}</n-button>
          <n-button type="primary" :loading="saving" @click="handleCreateRole">{{ $t('common.save') }}</n-button>
        </div>
      </template>
    </n-modal>

    <!-- Edit Role Modal -->
    <n-modal
      v-model:show="showEditModal"
      preset="card"
      :title="$t('adminRoles.editTitle')"
      style="width: 540px; max-width: 95vw; border-radius: 12px;"
    >
      <n-form :model="editForm" label-placement="left" label-width="130">
        <n-form-item :label="$t('adminRoles.roleName')">
          <n-input :value="editingRole?.name" disabled />
        </n-form-item>

        <n-form-item :label="$t('adminRoles.displayName')">
          <n-input v-model:value="editForm.display_name" />
        </n-form-item>

        <n-form-item :label="$t('adminRoles.userQuota')">
          <div style="display: flex; align-items: center; gap: 10px; width: 100%;">
            <n-input-number
              v-model:value="editQuotaGB"
              :min="0"
              :step="1"
              style="flex: 1;"
            />
            <span style="color: #aaa; font-size: 13px;">GB (0 = {{ $t('common.unlimited') }})</span>
          </div>
        </n-form-item>

        <n-form-item :label="$t('adminRoles.permittedPolicies')">
          <n-select
            v-model:value="editForm.policy_ids"
            multiple
            :options="policyOptions"
            :placeholder="$t('adminRoles.policiesPlaceholder')"
          />
        </n-form-item>
      </n-form>

      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 12px;">
          <n-button @click="showEditModal = false">{{ $t('common.cancel') }}</n-button>
          <n-button type="primary" :loading="saving" @click="handleUpdateRole">{{ $t('common.save') }}</n-button>
        </div>
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

const roles = ref<any[]>([])
const policies = ref<any[]>([])
const saving = ref(false)

const showCreateModal = ref(false)
const createQuotaGB = ref<number | null>(0)
const createForm = ref({
  name: '',
  display_name: '',
  policy_ids: [] as number[]
})

const showEditModal = ref(false)
const editingRole = ref<any>(null)
const editQuotaGB = ref<number | null>(0)
const editForm = ref({
  display_name: '',
  policy_ids: [] as number[]
})

const policyOptions = computed(() => {
  return policies.value.map(p => ({
    label: `${p.name} (${p.driver})`,
    value: p.id
  }))
})

const getPolicyTags = (ids: number[]) => {
  if (!ids) return []
  return policies.value.filter(p => ids.includes(p.id))
}

const loadData = async () => {
  try {
    const [rRes, pRes] = await Promise.all([
      api.get('/admin/roles'),
      api.get('/admin/policies')
    ])
    roles.value = rRes.data || []
    policies.value = pRes.data || []
  } catch {}
}

const openCreateModal = () => {
  createQuotaGB.value = 0
  createForm.value = {
    name: '',
    display_name: '',
    policy_ids: policies.value.filter(p => p.is_default).map(p => p.id)
  }
  showCreateModal.value = true
}

const handleCreateRole = async () => {
  if (!createForm.value.name || !createForm.value.display_name) {
    message.warning(t('adminRoles.nameRequired'))
    return
  }

  saving.value = true
  try {
    const quotaBytes = (createQuotaGB.value || 0) * 1024 * 1024 * 1024
    await api.post('/admin/roles', {
      name: createForm.value.name,
      display_name: createForm.value.display_name,
      quota_bytes: quotaBytes,
      policy_ids: createForm.value.policy_ids
    })
    message.success(t('adminRoles.saveSuccess'))
    showCreateModal.value = false
    loadData()
  } catch (err: any) {
    message.error(err.response?.data?.error || t('common.save'))
  } finally {
    saving.value = false
  }
}

const openEditModal = (r: any) => {
  editingRole.value = r
  editQuotaGB.value = r.quota_bytes > 0 ? Math.round(r.quota_bytes / (1024 * 1024 * 1024)) : 0
  editForm.value = {
    display_name: r.display_name,
    policy_ids: r.policy_ids || []
  }
  showEditModal.value = true
}

const handleUpdateRole = async () => {
  if (!editingRole.value) return
  saving.value = true
  try {
    const quotaBytes = (editQuotaGB.value || 0) * 1024 * 1024 * 1024
    await api.patch(`/admin/roles/${editingRole.value.id}`, {
      display_name: editForm.value.display_name,
      quota_bytes: quotaBytes,
      policy_ids: editForm.value.policy_ids
    })
    message.success(t('adminRoles.saveSuccess'))
    showEditModal.value = false
    loadData()
  } catch (err: any) {
    message.error(err.response?.data?.error || t('common.save'))
  } finally {
    saving.value = false
  }
}

const handleDeleteRole = async (id: number) => {
  try {
    await api.delete(`/admin/roles/${id}`)
    message.success(t('adminRoles.deleteSuccess'))
    loadData()
  } catch (err: any) {
    message.error(err.response?.data?.error || t('common.delete'))
  }
}

const formatBytes = (bytes: number) => {
  if (!bytes || bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

onMounted(loadData)
</script>
