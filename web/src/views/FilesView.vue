<template>
  <div>
    <!-- Top toolbar -->
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px;">
      <n-input
        v-model:value="search"
        :placeholder="$t('common.search')"
        style="max-width: 300px;"
        clearable
        @update:value="loadFiles"
      />
      <div>
        <n-button
          type="error"
          size="small"
          :disabled="selectedIds.length === 0"
          @click="handleBatchDelete"
        >
          <span>{{ $t('files.batchDelete') }}</span>
          <span v-if="selectedIds.length > 0" style="margin-left: 6px;">· {{ selectedIds.length }}</span>
        </n-button>
      </div>
    </div>

    <!-- Gallery Grid -->
    <n-grid :cols="4" :x-gap="16" :y-gap="16">
      <n-grid-item v-for="file in files" :key="file.id">
        <n-card
          size="small"
          style="border-radius: 8px; overflow: hidden;"
          :header-style="{ padding: '8px' }"
          :content-style="{ padding: '0' }"
        >
          <div style="height: 180px; background: #000; display: flex; align-items: center; justify-content: center; position: relative;">
            <img
              v-if="file.is_image"
              :src="baseURL + '/' + file.public_path + '?s=thumb'"
              style="max-width: 100%; max-height: 100%; object-fit: contain;"
            />
            <div v-else style="font-size: 14px; color: #888;">
              {{ file.mime_type }}
            </div>
            <div style="position: absolute; top: 8px; left: 8px;">
              <n-checkbox
                :checked="selectedIds.includes(file.id)"
                @update:checked="(val: boolean) => toggleSelect(file.id, val)"
              />
            </div>
          </div>

          <div style="padding: 10px;">
            <div style="display: flex; align-items: center; justify-content: space-between; gap: 8px;">
              <span style="font-weight: 500; font-size: 13px; text-overflow: ellipsis; overflow: hidden; white-space: nowrap;">
                {{ file.display_name }}
              </span>
              <n-tag v-if="file.watermark_mode === 'on'" type="warning" size="tiny">
                {{ $t('files.watermarked') }}
              </n-tag>
            </div>
            <div style="font-size: 12px; color: #777; margin-top: 4px; display: flex; justify-content: space-between;">
              <span>{{ formatBytes(file.size) }}</span>
              <span>{{ formatDate(file.created_at) }}</span>
            </div>
            
            <div style="margin-top: 10px; display: flex; flex-wrap: wrap; gap: 6px; justify-content: flex-end;">
              <n-button size="tiny" quaternary @click="copyLink(baseURL + '/' + file.public_path)">
                {{ $t('common.copy') }}
              </n-button>
              <n-button v-if="file.watermark_mode === 'on'" size="tiny" quaternary type="warning" @click="openTempLinkModal(file)">
                {{ $t('files.tempLink') }}
              </n-button>
              <n-button size="tiny" quaternary @click="openRename(file)">
                {{ $t('files.rename') }}
              </n-button>
              <n-popconfirm @positive-click="deleteFile(file.id)">
                <template #trigger>
                  <n-button size="tiny" quaternary type="error">{{ $t('common.delete') }}</n-button>
                </template>
                {{ $t('files.confirmDelete') }}
              </n-popconfirm>
            </div>
          </div>
        </n-card>
      </n-grid-item>
    </n-grid>

    <!-- Pagination -->
    <div style="display: flex; justify-content: center; margin-top: 24px;">
      <n-pagination
        v-model:page="page"
        :page-size="pageSize"
        :item-count="total"
        @update:page="loadFiles"
      />
    </div>

    <!-- Rename Modal -->
    <n-modal v-model:show="showRenameModal" preset="dialog" :title="$t('files.renameTitle')">
      <n-input v-model:value="renameName" :placeholder="$t('files.renamePlaceholder')" />
      <template #action>
        <n-button @click="showRenameModal = false">{{ $t('common.cancel') }}</n-button>
        <n-button type="primary" @click="confirmRename">{{ $t('common.save') }}</n-button>
      </template>
    </n-modal>

    <!-- Temp Link Modal -->
    <n-modal v-model:show="showTempLinkModal" preset="card" :title="$t('files.tempLinkTitle')" style="width: 500px; border-radius: 12px;">
      <p style="color: #aaa; font-size: 13px;">
        {{ $t('files.tempLinkDesc') }}
      </p>

      <n-form label-placement="left" label-width="90">
        <n-form-item :label="$t('files.duration')">
          <n-select v-model:value="tempExpiryHours" :options="expiryOptions" />
        </n-form-item>
        <n-form-item v-if="generatedTempUrl" :label="$t('files.tempLink')">
          <n-input-group>
            <n-input :value="generatedTempUrl" readonly />
            <n-button type="primary" @click="copyLink(generatedTempUrl)">{{ $t('common.copy') }}</n-button>
          </n-input-group>
        </n-form-item>
      </n-form>

      <template #footer>
        <div style="display: flex; justify-content: space-between; align-items: center;">
          <n-popconfirm @positive-click="revokeAllTempLinks">
            <template #trigger>
              <n-button type="error" ghost size="small">{{ $t('files.revokeLinks') }}</n-button>
            </template>
            {{ $t('files.confirmRevoke') }}
          </n-popconfirm>
          <n-button type="primary" :loading="generatingTemp" @click="generateTempLink">{{ $t('files.generateLink') }}</n-button>
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

const files = ref<any[]>([])
const baseURL = ref(window.location.origin)
const search = ref('')
const page = ref(1)
const pageSize = ref(24)
const total = ref(0)
const selectedIds = ref<number[]>([])

const showRenameModal = ref(false)
const renameTargetId = ref<number | null>(null)
const renameName = ref('')

const showTempLinkModal = ref(false)
const tempTargetFile = ref<any>(null)
const tempExpiryHours = ref(24)
const generatedTempUrl = ref('')
const generatingTemp = ref(false)

const expiryOptions = computed(() => [
  { label: t('files.hour1'), value: 1 },
  { label: t('files.hour24'), value: 24 },
  { label: t('files.day7'), value: 168 }
])

const openTempLinkModal = (file: any) => {
  tempTargetFile.value = file
  generatedTempUrl.value = ''
  showTempLinkModal.value = true
}

const generateTempLink = async () => {
  if (!tempTargetFile.value) return
  generatingTemp.value = true
  try {
    const res = await api.post(`/files/${tempTargetFile.value.id}/temp-link`, {
      expires_hours: tempExpiryHours.value
    })
    generatedTempUrl.value = res.data.temp_url
    message.success(t('files.tempLink'))
  } catch {
    message.error(t('files.deleteFailed'))
  } finally {
    generatingTemp.value = false
  }
}

const revokeAllTempLinks = async () => {
  if (!tempTargetFile.value) return
  try {
    await api.post(`/files/${tempTargetFile.value.id}/revoke-links`)
    message.success(t('files.revokeSuccess'))
    generatedTempUrl.value = ''
  } catch {
    message.error(t('files.deleteFailed'))
  }
}

const loadFiles = async () => {
  try {
    const res = await api.get('/files', {
      params: {
        page: page.value,
        page_size: pageSize.value,
        search: search.value
      }
    })
    files.value = res.data.items
    total.value = res.data.total
    if (res.data.base_url) {
      baseURL.value = res.data.base_url
    }
  } catch {}
}

const toggleSelect = (id: number, checked: boolean) => {
  if (checked) {
    selectedIds.value.push(id)
  } else {
    selectedIds.value = selectedIds.value.filter(item => item !== id)
  }
}

const copyLink = (url: string) => {
  navigator.clipboard.writeText(url)
  message.success(t('common.copied'))
}

const openRename = (file: any) => {
  renameTargetId.value = file.id
  renameName.value = file.display_name
  showRenameModal.value = true
}

const confirmRename = async () => {
  if (!renameTargetId.value || !renameName.value) return
  try {
    await api.patch(`/files/${renameTargetId.value}`, {
      display_name: renameName.value
    })
    message.success(t('files.renameSuccess'))
    showRenameModal.value = false
    loadFiles()
  } catch (err: any) {
    message.error(err.response?.data?.error || t('files.renameFailed'))
  }
}

const deleteFile = async (id: number) => {
  try {
    await api.delete(`/files/${id}`)
    message.success(t('files.deleteSuccess'))
    loadFiles()
  } catch (err: any) {
    message.error(err.response?.data?.error || t('files.deleteFailed'))
  }
}

const handleBatchDelete = async () => {
  if (selectedIds.value.length === 0) return
  try {
    await api.post('/files/batch-delete', { ids: selectedIds.value })
    message.success(`${t('files.deleteSuccess')} ${selectedIds.value.length}`)
    selectedIds.value = []
    loadFiles()
  } catch (err: any) {
    message.error(t('files.deleteFailed'))
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
  return `${d.getMonth() + 1}/${d.getDate()}`
}

onMounted(() => {
  loadFiles()
})
</script>
