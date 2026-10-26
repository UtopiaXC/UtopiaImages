<template>
  <div>
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px;">
      <div>
        <div style="font-size: 18px; font-weight: bold;">{{ $t('adminPolicies.title') }}</div>
        <div style="font-size: 13px; color: #888; margin-top: 4px;">{{ $t('adminPolicies.subtitle') }}</div>
      </div>
      <n-button type="primary" @click="openCreateModal">{{ $t('adminPolicies.new') }}</n-button>
    </div>

    <n-card style="border-radius: 12px;">
      <div class="table-responsive">
        <n-table :bordered="false" :single-line="false">
        <thead>
          <tr>
            <th>ID</th>
            <th>{{ $t('adminPolicies.policyName') }}</th>
            <th>{{ $t('adminPolicies.driver') }}</th>
            <th>{{ $t('adminPolicies.maxCapacity') }}</th>
            <th>{{ $t('adminPolicies.usedSpace') }}</th>
            <th>{{ $t('adminPolicies.pathPrefix') }}</th>
            <th>{{ $t('adminPolicies.namingRule') }}</th>
            <th>{{ $t('adminPolicies.permittedRoles') }}</th>
            <th>{{ $t('adminPolicies.isDefault') }}</th>
            <th style="text-align: right;">{{ $t('common.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="p in policies" :key="p.id">
            <td>{{ p.id }}</td>
            <td><strong>{{ p.name }}</strong></td>
            <td>
              <n-tag size="tiny" type="info">Local</n-tag>
            </td>
            <td>{{ p.max_capacity_bytes === 0 ? $t('common.unlimited') : formatBytes(p.max_capacity_bytes) }}</td>
            <td>{{ formatBytes(p.used_bytes) }}</td>
            <td><code>{{ formatPathPrefix(p.path_prefix) }}</code></td>
            <td><code>{{ p.naming_rule }}</code></td>
            <td>
              <div style="display: flex; flex-wrap: wrap; gap: 4px;">
                <n-tag v-for="r in getRoleTags(p.role_ids)" :key="r.id" size="tiny" type="success">
                  {{ r.display_name }}
                </n-tag>
                <span v-if="!p.role_ids || p.role_ids.length === 0" style="color: #666; font-size: 12px;">-</span>
              </div>
            </td>
            <td>
              <n-tag :type="p.is_default ? 'success' : 'default'" size="small">
                {{ p.is_default ? $t('common.yes') : $t('common.no') }}
              </n-tag>
            </td>
            <td style="text-align: right;">
              <div style="display: flex; gap: 8px; justify-content: flex-end;">
                <n-button size="tiny" quaternary type="primary" @click="openEditModal(p)">
                  {{ $t('common.edit') }}
                </n-button>
                <n-popconfirm @positive-click="handleDeletePolicy(p.id)">
                  <template #trigger>
                    <n-button size="tiny" quaternary type="error" :disabled="p.is_default && policies.length === 1">
                      {{ $t('common.delete') }}
                    </n-button>
                  </template>
                  {{ $t('adminPolicies.confirmDelete') }}
                </n-popconfirm>
              </div>
            </td>
          </tr>
        </tbody>
      </n-table>
      </div>
    </n-card>

    <!-- Create / Edit Policy Modal -->
    <n-modal
      v-model:show="showModal"
      preset="card"
      :title="isEditing ? $t('adminPolicies.editTitle') : $t('adminPolicies.new')"
      style="width: 580px; max-width: 95vw; border-radius: 12px;"
    >
      <n-form :model="policyForm" label-placement="left" label-width="130">
        <n-form-item :label="$t('adminPolicies.policyName')">
          <n-input v-model:value="policyForm.name" :placeholder="$t('adminPolicies.namePlaceholder')" />
        </n-form-item>

        <n-form-item :label="$t('adminPolicies.driver')">
          <n-select
            v-model:value="policyForm.driver"
            :options="driverOptions"
            disabled
          />
        </n-form-item>

        <n-form-item :label="$t('adminPolicies.maxCapacity')">
          <div style="display: flex; align-items: center; gap: 10px; width: 100%;">
            <n-input-number
              v-model:value="capacityGB"
              :min="0"
              :step="1"
              :placeholder="$t('adminPolicies.capacityPlaceholder')"
              style="flex: 1;"
            />
            <span style="color: #aaa; font-size: 13px;">GB (0 = {{ $t('common.unlimited') }})</span>
          </div>
        </n-form-item>

        <n-form-item :label="$t('adminPolicies.pathPrefix')">
          <div style="width: 100%;">
            <n-input v-model:value="policyForm.path_prefix" placeholder="留空则无前缀，默认 uploads" clearable />
            <div style="display: flex; flex-wrap: wrap; gap: 6px; margin-top: 8px; align-items: center;">
              <span style="font-size: 12px; color: #888;">{{ $t('adminPolicies.insertToken') }}:</span>
              <n-button size="tiny" secondary type="warning" @click="policyForm.path_prefix = ''">
                留空 (无前缀)
              </n-button>
              <n-button size="tiny" secondary type="info" @click="policyForm.path_prefix = 'uploads'">
                uploads (默认)
              </n-button>
              <n-button size="tiny" secondary type="info" @click="insertPathPrefixToken('{username}')">
                {username} · {{ $t('adminPolicies.tokenUsername') }}
              </n-button>
              <n-button size="tiny" secondary type="info" @click="insertPathPrefixToken('{uid}')">
                {uid} · {{ $t('adminPolicies.tokenUid') }}
              </n-button>
              <n-button size="tiny" secondary type="info" @click="insertPathPrefixToken('{role}')">
                {role} · {{ $t('adminPolicies.tokenRole') }}
              </n-button>
              <n-button size="tiny" secondary type="info" @click="insertPathPrefixToken('{year}')">
                {year} · {{ $t('adminPolicies.tokenYear') }}
              </n-button>
              <n-button size="tiny" secondary type="info" @click="insertPathPrefixToken('{month}')">
                {month} · {{ $t('adminPolicies.tokenMonth') }}
              </n-button>
              <n-button size="tiny" secondary type="info" @click="insertPathPrefixToken('{day}')">
                {day} · {{ $t('adminPolicies.tokenDay') }}
              </n-button>
              <n-button size="tiny" secondary type="info" @click="insertPathPrefixToken('{hour}')">
                {hour} · {{ $t('adminPolicies.tokenHour') }}
              </n-button>
            </div>
          </div>
        </n-form-item>

        <n-form-item :label="$t('adminPolicies.namingRule')">
          <div style="width: 100%;">
            <n-input v-model:value="policyForm.naming_rule" placeholder="{Y}/{m}/{d}/{random:16}.{ext}" />
            <div style="display: flex; flex-wrap: wrap; gap: 6px; margin-top: 8px; align-items: center;">
              <span style="font-size: 12px; color: #888;">{{ $t('adminPolicies.insertToken') }}:</span>
              <n-button size="tiny" secondary type="success" @click="insertNamingRuleToken('{Y}')">
                {Y} · {{ $t('adminPolicies.tokenYear') }}
              </n-button>
              <n-button size="tiny" secondary type="success" @click="insertNamingRuleToken('{m}')">
                {m} · {{ $t('adminPolicies.tokenMonth') }}
              </n-button>
              <n-button size="tiny" secondary type="success" @click="insertNamingRuleToken('{d}')">
                {d} · {{ $t('adminPolicies.tokenDay') }}
              </n-button>
              <n-button size="tiny" secondary type="success" @click="insertNamingRuleToken('{H}')">
                {H} · {{ $t('adminPolicies.tokenHour') }}
              </n-button>
              <n-button size="tiny" secondary type="success" @click="insertNamingRuleToken('{i}')">
                {i} · {{ $t('adminPolicies.tokenMinute') }}
              </n-button>
              <n-button size="tiny" secondary type="success" @click="insertNamingRuleToken('{s}')">
                {s} · {{ $t('adminPolicies.tokenSecond') }}
              </n-button>
              <n-button size="tiny" secondary type="success" @click="insertNamingRuleToken('{random:16}')">
                {random:16} · {{ $t('adminPolicies.tokenRandom') }}
              </n-button>
              <n-button size="tiny" secondary type="success" @click="insertNamingRuleToken('{filename}')">
                {filename} · {{ $t('adminPolicies.tokenFilename') }}
              </n-button>
              <n-button size="tiny" secondary type="success" @click="insertNamingRuleToken('{ext}')">
                {ext} · {{ $t('adminPolicies.tokenExt') }}
              </n-button>
              <n-button size="tiny" secondary type="success" @click="insertNamingRuleToken('{timestamp}')">
                {timestamp} · {{ $t('adminPolicies.tokenTimestamp') }}
              </n-button>
              <n-button size="tiny" secondary type="success" @click="insertNamingRuleToken('{sha256}')">
                {sha256} · {{ $t('adminPolicies.tokenSha256') }}
              </n-button>
            </div>
          </div>
        </n-form-item>

        <n-form-item :label="$t('adminPolicies.permittedRoles')">
          <n-select
            v-model:value="policyForm.role_ids"
            multiple
            :options="roleOptions"
            :placeholder="$t('adminPolicies.rolesPlaceholder')"
          />
        </n-form-item>

        <n-form-item :label="$t('adminPolicies.isDefault')">
          <n-switch v-model:value="policyForm.is_default" />
        </n-form-item>
      </n-form>

      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 12px;">
          <n-button @click="showModal = false">{{ $t('common.cancel') }}</n-button>
          <n-button type="primary" :loading="saving" @click="handleSavePolicy">{{ $t('common.save') }}</n-button>
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
const policies = ref<any[]>([])
const roles = ref<any[]>([])

const showModal = ref(false)
const isEditing = ref(false)
const editTargetId = ref<number | null>(null)
const saving = ref(false)
const capacityGB = ref<number | null>(0)

const policyForm = ref({
  name: '',
  driver: 'local',
  path_prefix: 'uploads',
  naming_rule: '{Y}/{m}/{d}/{random:16}.{ext}',
  is_default: false,
  role_ids: [] as number[]
})

const driverOptions = [
  { label: 'Local · 本地存储', value: 'local' }
]

const roleOptions = computed(() => {
  return roles.value.map(r => ({
    label: `${r.display_name} · ${r.name}`,
    value: r.id
  }))
})

const insertPathPrefixToken = (token: string) => {
  if (!policyForm.value.path_prefix) {
    policyForm.value.path_prefix = token
  } else if (policyForm.value.path_prefix.endsWith('/')) {
    policyForm.value.path_prefix += token
  } else {
    policyForm.value.path_prefix += '/' + token
  }
}

const insertNamingRuleToken = (token: string) => {
  if (!policyForm.value.naming_rule) {
    policyForm.value.naming_rule = token
  } else {
    policyForm.value.naming_rule += token
  }
}

const getRoleTags = (roleIds: number[]) => {
  if (!roleIds) return []
  return roles.value.filter(r => roleIds.includes(r.id))
}

const loadData = async () => {
  try {
    const [pRes, rRes] = await Promise.all([
      api.get('/admin/policies'),
      api.get('/admin/roles')
    ])
    policies.value = pRes.data || []
    roles.value = rRes.data || []
  } catch {}
}

const formatPathPrefix = (prefix: string | undefined | null) => {
  if (prefix === undefined || prefix === null || prefix.trim() === '' || prefix.trim() === '/') {
    return '/ (无前缀)'
  }
  const clean = prefix.replace(/^\/+|\/+$/g, '')
  return '/' + clean
}

const openCreateModal = () => {
  isEditing.value = false
  editTargetId.value = null
  capacityGB.value = 0
  policyForm.value = {
    name: '',
    driver: 'local',
    path_prefix: 'uploads',
    naming_rule: '{Y}/{m}/{d}/{random:16}.{ext}',
    is_default: false,
    role_ids: roles.value.map(r => r.id)
  }
  showModal.value = true
}

const openEditModal = (p: any) => {
  isEditing.value = true
  editTargetId.value = p.id
  capacityGB.value = p.max_capacity_bytes > 0 ? Math.round(p.max_capacity_bytes / (1024 * 1024 * 1024)) : 0
  policyForm.value = {
    name: p.name,
    driver: p.driver || 'local',
    path_prefix: p.path_prefix !== undefined && p.path_prefix !== null ? p.path_prefix : '',
    naming_rule: p.naming_rule || '{Y}/{m}/{d}/{random:16}.{ext}',
    is_default: p.is_default,
    role_ids: p.role_ids || []
  }
  showModal.value = true
}

const handleSavePolicy = async () => {
  if (!policyForm.value.name) {
    message.warning(t('adminPolicies.nameRequired'))
    return
  }

  saving.value = true
  try {
    const maxCapacityBytes = (capacityGB.value || 0) * 1024 * 1024 * 1024
    const cleanPrefix = (policyForm.value.path_prefix || '').trim().replace(/^\/+|\/+$/g, '')
    const payload = {
      ...policyForm.value,
      path_prefix: cleanPrefix,
      max_capacity_bytes: maxCapacityBytes
    }

    if (isEditing.value && editTargetId.value) {
      await api.patch(`/admin/policies/${editTargetId.value}`, payload)
    } else {
      await api.post('/admin/policies', payload)
    }
    message.success(t('adminPolicies.saveSuccess'))
    showModal.value = false
    loadData()
  } catch (err: any) {
    message.error(err.response?.data?.error || t('common.save'))
  } finally {
    saving.value = false
  }
}

const handleDeletePolicy = async (id: number) => {
  try {
    await api.delete(`/admin/policies/${id}`)
    message.success(t('adminPolicies.deleteSuccess'))
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
