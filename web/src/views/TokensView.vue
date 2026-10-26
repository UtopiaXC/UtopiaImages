<template>
  <div style="max-width: 800px; margin: 0 auto;">
    <n-card :title="$t('tokens.title')" style="border-radius: 12px; margin-bottom: 24px;">
      <p style="color: #999; margin-bottom: 20px;">
        {{ $t('tokens.subtitle') }}
      </p>

      <div style="display: flex; gap: 12px; margin-bottom: 20px;">
        <n-input v-model:value="newTokenName" :placeholder="$t('tokens.namePlaceholder')" />
        <n-button type="primary" :loading="creating" @click="handleCreate">{{ $t('tokens.create') }}</n-button>
      </div>

      <n-alert v-if="justCreatedToken" type="success" :title="$t('tokens.createdNotice')" style="margin-bottom: 20px;">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 8px;">
          <code style="font-size: 14px; background: rgba(0,0,0,0.3); padding: 4px 8px; border-radius: 4px;">{{ justCreatedToken }}</code>
          <n-button size="small" type="primary" @click="copyToken(justCreatedToken)">{{ $t('common.copy') }}</n-button>
        </div>
      </n-alert>

      <div class="table-responsive">
        <n-table :bordered="false" :single-line="false">
          <thead>
            <tr>
              <th>{{ $t('tokens.name') }}</th>
              <th>{{ $t('tokens.prefix') }}</th>
              <th>{{ $t('common.createdAt') }}</th>
              <th>{{ $t('tokens.lastUsed') }}</th>
              <th>{{ $t('common.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="t in tokens" :key="t.id">
              <td>{{ t.name }}</td>
              <td><code>{{ t.prefix }}***</code></td>
              <td>{{ formatDate(t.created_at) }}</td>
              <td>{{ t.last_used_at ? formatDate(t.last_used_at) : $t('tokens.never') }}</td>
              <td>
                <n-popconfirm @positive-click="deleteToken(t.id)">
                  <template #trigger>
                    <n-button size="tiny" type="error" quaternary>{{ $t('tokens.revoke') }}</n-button>
                  </template>
                  {{ $t('tokens.confirmRevoke') }}
                </n-popconfirm>
              </td>
            </tr>
          </tbody>
        </n-table>
      </div>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'

const message = useMessage()
const { t } = useI18n()
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
    message.warning(t('tokens.namePlaceholder'))
    return
  }

  creating.value = true
  try {
    const res = await api.post('/me/tokens', {
      name: newTokenName.value,
      expires_days: 0
    })
    justCreatedToken.value = res.data.token
    newTokenName.value = ''
    loadTokens()
    message.success(t('tokens.createSuccess'))
  } catch (err: any) {
    message.error(err.response?.data?.error || t('common.delete'))
  } finally {
    creating.value = false
  }
}

const deleteToken = async (id: number) => {
  try {
    await api.delete(`/me/tokens/${id}`)
    message.success(t('tokens.revokeSuccess'))
    loadTokens()
  } catch {}
}

const copyToken = (text: string) => {
  navigator.clipboard.writeText(text)
  message.success(t('common.copied'))
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
