<template>
  <div class="admin-files-page">
    <div style="margin-bottom: 20px;">
      <h2 style="margin: 0 0 6px; font-weight: 600; font-size: 20px;">{{ $t('adminFiles.title') }}</h2>
      <p style="margin: 0; color: #888; font-size: 13px;">{{ $t('adminFiles.subtitle') }}</p>
    </div>

    <!-- Filters & Actions Toolbar -->
    <n-card size="small" style="margin-bottom: 16px; border-radius: 8px;">
      <div style="display: flex; flex-wrap: wrap; gap: 12px; align-items: center; justify-content: space-between;">
        <div style="display: flex; flex-wrap: wrap; gap: 10px; align-items: center; flex: 1 1 300px;">
          <!-- Search -->
          <n-input
            v-model:value="search"
            :placeholder="$t('adminFiles.searchPlaceholder')"
            clearable
            size="small"
            style="min-width: 200px; max-width: 280px;"
            @update:value="handleSearchChange"
          >
            <template #prefix>
              <n-icon :component="SearchOutline" />
            </template>
          </n-input>

          <!-- Format Filter -->
          <n-select
            v-model:value="formatFilter"
            :options="formatOptions"
            size="small"
            style="width: 120px;"
            @update:value="handleFilterChange"
          />

          <!-- Storage Policy Filter -->
          <n-select
            v-model:value="policyFilter"
            :options="policyOptions"
            size="small"
            style="width: 130px;"
            @update:value="handleFilterChange"
          />

          <!-- Time Range Filter -->
          <n-select
            v-model:value="timeRangeFilter"
            :options="timeRangeOptions"
            size="small"
            style="width: 120px;"
            @update:value="handleFilterChange"
          />

          <!-- Temp Link filter -->
          <n-checkbox v-model:checked="tempLinkOnly" @update:checked="handleFilterChange">
            {{ $t('adminFiles.filterTempLinks') }}
          </n-checkbox>
        </div>

        <!-- Batch Delete Action -->
        <div style="display: flex; gap: 10px; align-items: center;">
          <n-popconfirm @positive-click="handleBatchDelete">
            <template #trigger>
              <n-button
                type="error"
                size="small"
                :disabled="selectedIds.length === 0"
                :loading="batchDeleting"
              >
                {{ $t('adminFiles.batchDelete') }} ({{ selectedIds.length }})
              </n-button>
            </template>
            {{ $t('adminFiles.confirmBatchDelete', { count: selectedIds.length }) }}
          </n-popconfirm>
        </div>
      </div>
    </n-card>

    <!-- Files Table -->
    <n-card size="small" style="border-radius: 8px;">
      <div class="table-responsive">
        <n-table :bordered="false" :single-line="false" size="small">
          <thead>
            <tr>
              <th style="width: 40px;">
                <n-checkbox
                  :checked="isAllSelected"
                  :indeterminate="isIndeterminate"
                  @update:checked="toggleSelectAll"
                />
              </th>
              <th style="width: 64px;">{{ $t('files.preview') }}</th>
              <th>{{ $t('files.fileName') }}</th>
              <th>{{ $t('adminFiles.uploader') }}</th>
              <th>{{ $t('files.fileSize') }}</th>
              <th>{{ $t('files.dimensions') }}</th>
              <th>{{ $t('files.mimeType') }}</th>
              <th>{{ $t('adminFiles.tempLinks') }}</th>
              <th>{{ $t('common.createdAt') }}</th>
              <th style="text-align: right;">{{ $t('common.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="loading">
              <td colspan="10" style="text-align: center; padding: 36px 0; color: #888;">
                <n-spin size="small" />
              </td>
            </tr>
            <tr v-else-if="files.length === 0">
              <td colspan="10" style="text-align: center; padding: 40px 0; color: #777;">
                {{ $t('files.noFiles') }}
              </td>
            </tr>
            <tr v-else v-for="file in files" :key="file.id">
              <td>
                <n-checkbox
                  :checked="selectedIds.includes(file.id)"
                  @update:checked="(val: boolean) => toggleSelect(file.id, val)"
                />
              </td>
              <td>
                <img
                  v-if="file.is_image"
                  :src="resolveFileThumbUrl(file)"
                  style="width: 48px; height: 48px; object-fit: cover; border-radius: 4px; cursor: pointer; border: 1px solid rgba(255, 255, 255, 0.1);"
                  @click="openPreviewModal(file)"
                />
                <div
                  v-else
                  style="width: 48px; height: 48px; background: rgba(255, 255, 255, 0.05); display: flex; align-items: center; justify-content: center; border-radius: 4px; color: #888; font-size: 11px; font-weight: bold;"
                >
                  FILE
                </div>
              </td>
              <td>
                <div style="font-weight: 500; word-break: break-all;">
                  {{ file.display_name }}
                </div>
                <div style="color: #777; font-size: 11px; margin-top: 2px; word-break: break-all;">
                  {{ file.public_path }}
                </div>
              </td>
              <td>
                <div style="display: flex; align-items: center; gap: 6px;">
                  <n-tag size="tiny" type="info">#{{ file.user?.id }}</n-tag>
                  <span style="font-size: 13px;">{{ file.user?.nickname || file.user?.username || '-' }}</span>
                </div>
                <div style="color: #666; font-size: 11px; margin-top: 2px;">
                  {{ file.user?.username }}
                </div>
              </td>
              <td>{{ formatBytes(file.size) }}</td>
              <td>{{ file.width && file.height ? `${file.width} × ${file.height}` : '-' }}</td>
              <td>
                <n-tag size="tiny" :type="file.is_image ? 'success' : 'default'">
                  {{ file.mime_type }}
                </n-tag>
              </td>
              <td>
                <n-tag v-if="file.temp_link_count > 0" type="warning" size="tiny">
                  {{ $t('adminFiles.hasTempLinks') }} ({{ file.temp_link_count }})
                </n-tag>
                <span v-else style="color: #666; font-size: 12px;">{{ $t('adminFiles.noTempLinksShort') }}</span>
              </td>
              <td style="white-space: nowrap; font-size: 12px; color: #888;">
                {{ formatDate(file.created_at) }}
              </td>
              <td style="text-align: right; white-space: nowrap;">
                <div style="display: flex; gap: 6px; justify-content: flex-end;">
                  <n-button size="tiny" quaternary type="primary" @click="openDetails(file.id)">
                    {{ $t('common.details') }}
                  </n-button>
                  <n-popconfirm @positive-click="deleteFile(file.id)">
                    <template #trigger>
                      <n-button size="tiny" quaternary type="error">
                        {{ $t('common.delete') }}
                      </n-button>
                    </template>
                    {{ $t('adminFiles.confirmDelete') }}
                  </n-popconfirm>
                </div>
              </td>
            </tr>
          </tbody>
        </n-table>
      </div>

      <!-- Pagination -->
      <div v-if="total > pageSize" style="display: flex; justify-content: flex-end; margin-top: 16px;">
        <n-pagination
          v-model:page="page"
          :page-size="pageSize"
          :item-count="total"
          @update:page="loadFiles"
        />
      </div>
    </n-card>

    <!-- Image Preview Modal -->
    <n-modal v-model:show="showPreviewModal" preset="card" style="max-width: 800px; width: 95vw;" :title="previewFile?.display_name">
      <div style="text-align: center; max-height: 70vh; overflow: auto;">
        <img :src="resolveFileUrl(previewFile)" style="max-width: 100%; max-height: 65vh; object-fit: contain; border-radius: 6px;" />
      </div>
    </n-modal>

    <!-- File Details Drawer / Modal -->
    <n-modal v-model:show="showDetailsModal" preset="card" style="max-width: 720px; width: 95vw;" :title="$t('adminFiles.fileInfo')">
      <div v-if="detailsLoading" style="text-align: center; padding: 40px 0;">
        <n-spin size="medium" />
      </div>
      <div v-else-if="detailsData">
        <!-- Preview & Basic Info Grid -->
        <div style="display: flex; gap: 16px; margin-bottom: 20px; flex-wrap: wrap;">
          <div v-if="detailsData.file?.is_image" style="flex: 0 0 160px; text-align: center;">
            <img
              :src="resolveFileUrl(detailsData.file)"
              style="width: 160px; height: 160px; object-fit: cover; border-radius: 8px; border: 1px solid rgba(255,255,255,0.1);"
            />
          </div>
          <div style="flex: 1 1 300px;">
            <h3 style="margin: 0 0 10px; font-size: 16px; font-weight: 600;">{{ detailsData.file?.display_name }}</h3>
            <n-descriptions :column="2" size="small" bordered>
              <n-descriptions-item :label="$t('files.fileSize')">
                {{ formatBytes(detailsData.file?.size) }}
              </n-descriptions-item>
              <n-descriptions-item :label="$t('files.dimensions')">
                {{ detailsData.file?.width && detailsData.file?.height ? `${detailsData.file.width} × ${detailsData.file.height}` : '-' }}
              </n-descriptions-item>
              <n-descriptions-item :label="$t('files.mimeType')">
                {{ detailsData.file?.mime_type }}
              </n-descriptions-item>
              <n-descriptions-item :label="$t('adminFiles.filterPolicy')">
                {{ detailsData.policy?.name || '-' }} ({{ detailsData.policy?.driver }})
              </n-descriptions-item>
              <n-descriptions-item :label="$t('adminFiles.uploader')">
                {{ detailsData.user?.nickname || detailsData.user?.username }} (ID: {{ detailsData.user?.id }})
              </n-descriptions-item>
              <n-descriptions-item :label="$t('common.createdAt')">
                {{ formatDate(detailsData.file?.created_at) }}
              </n-descriptions-item>
            </n-descriptions>
          </div>
        </div>

        <!-- Copyable Links & Temporary Links -->
        <div style="margin-bottom: 20px;">
          <h4 style="margin: 0 0 10px; font-size: 14px; font-weight: 600;">{{ $t('adminFiles.links') }}</h4>
          <FileLinksPanel
            v-if="detailsData.file"
            :file="detailsData.file"
            :initial-url="resolveFileUrl(detailsData.file)"
          />
        </div>

        <!-- Temporary Links List -->
        <div>
          <h4 style="margin: 0 0 10px; font-size: 14px; font-weight: 600;">
            {{ $t('adminFiles.tempLinks') }} ({{ (detailsData.temp_links || []).length }})
          </h4>
          <div v-if="(detailsData.temp_links || []).length === 0" style="color: #777; font-size: 13px;">
            {{ $t('adminFiles.noTempLinks') }}
          </div>
          <div v-else style="display: flex; flex-direction: column; gap: 6px;">
            <n-card v-for="tl in detailsData.temp_links" :key="tl.id" size="small" style="background: rgba(255,255,255,0.02);">
              <div style="display: flex; justify-content: space-between; align-items: center; font-size: 12px;">
                <div>
                  <span style="font-weight: 500;">Token: <code>{{ tl.token }}</code></span>
                  <div style="color: #888; margin-top: 4px;">
                    {{ $t('common.createdAt') }}: {{ formatDate(tl.created_at) }} · 过期时间: {{ formatDate(tl.expires_at) }}
                  </div>
                </div>
                <n-tag :type="tl.is_revoked ? 'error' : 'success'" size="tiny">
                  {{ tl.is_revoked ? '已吊销' : '有效' }}
                </n-tag>
              </div>
            </n-card>
          </div>
        </div>
      </div>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { SearchOutline } from '@vicons/ionicons5'
import { api, getApiErrorMessage } from '../api/client'
import FileLinksPanel from '../components/FileLinksPanel.vue'

const message = useMessage()
const { t } = useI18n()

const isBrowserOnline = () => {
  return typeof window !== 'undefined' &&
    window.location?.origin &&
    !window.location.origin.includes('localhost') &&
    !window.location.origin.includes('127.0.0.1')
}

const resolveFileThumbUrl = (file: any) => {
  if (!file) return ''
  const cleanPath = (file.public_path || '').replace(/^\/+/, '')
  const hasLocalhost = file.thumbnail_url && (file.thumbnail_url.includes('localhost') || file.thumbnail_url.includes('127.0.0.1'))
  if (!file.thumbnail_url || (hasLocalhost && isBrowserOnline())) {
    return `/${cleanPath}?s=thumb`
  }
  return file.thumbnail_url
}

const resolveFileUrl = (file: any) => {
  if (!file) return ''
  const cleanPath = (file.public_path || '').replace(/^\/+/, '')
  const hasLocalhost = file.url && (file.url.includes('localhost') || file.url.includes('127.0.0.1'))
  if (!file.url || (hasLocalhost && isBrowserOnline())) {
    return `/${cleanPath}`
  }
  return file.url
}

const loading = ref(false)
const batchDeleting = ref(false)
const files = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)

// Filters
const search = ref('')
const formatFilter = ref('all')
const policyFilter = ref('all')
const timeRangeFilter = ref('all')
const tempLinkOnly = ref(false)

// Selections
const selectedIds = ref<number[]>([])

// Details and preview
const showPreviewModal = ref(false)
const previewFile = ref<any>(null)
const showDetailsModal = ref(false)
const detailsLoading = ref(false)
const detailsData = ref<any>(null)

// Storage Policies
const policyOptions = ref<{ label: string; value: string }[]>([
  { label: t('adminFiles.filterPolicy') + ': ' + t('common.all'), value: 'all' }
])

const formatOptions = computed(() => [
  { label: t('adminFiles.filterFormat') + ': ' + t('common.all'), value: 'all' },
  { label: 'JPEG / JPG', value: 'jpeg' },
  { label: 'PNG', value: 'png' },
  { label: 'WebP', value: 'webp' },
  { label: 'GIF', value: 'gif' },
  { label: 'SVG', value: 'svg' },
  { label: '其他文件', value: 'other' }
])

const timeRangeOptions = computed(() => [
  { label: t('adminFiles.filterTime') + ': ' + t('common.all'), value: 'all' },
  { label: t('files.timeToday'), value: 'today' },
  { label: t('files.time7d'), value: '7d' },
  { label: t('files.time30d'), value: '30d' }
])

const isAllSelected = computed(() => {
  return files.value.length > 0 && files.value.every(f => selectedIds.value.includes(f.id))
})

const isIndeterminate = computed(() => {
  return selectedIds.value.length > 0 && !isAllSelected.value
})

const toggleSelectAll = (checked: boolean) => {
  if (checked) {
    const curIds = files.value.map(f => f.id)
    const combined = Array.from(new Set([...selectedIds.value, ...curIds]))
    selectedIds.value = combined
  } else {
    const curIds = files.value.map(f => f.id)
    selectedIds.value = selectedIds.value.filter(id => !curIds.includes(id))
  }
}

const toggleSelect = (id: number, checked: boolean) => {
  if (checked) {
    selectedIds.value.push(id)
  } else {
    selectedIds.value = selectedIds.value.filter(i => i !== id)
  }
}

let searchTimer: any = null
const handleSearchChange = () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    page.value = 1
    loadFiles()
  }, 350)
}

const handleFilterChange = () => {
  page.value = 1
  loadFiles()
}

const loadPolicies = async () => {
  try {
    const res = await api.get('/admin/policies')
    const list = res.data || []
    policyOptions.value = [
      { label: t('adminFiles.filterPolicy') + ': ' + t('common.all'), value: 'all' },
      ...list.map((p: any) => ({ label: p.name, value: String(p.id) }))
    ]
  } catch {}
}

const loadFiles = async () => {
  loading.value = true
  try {
    const params: any = {
      page: page.value,
      page_size: pageSize.value
    }
    if (search.value.trim()) params.search = search.value.trim()
    if (formatFilter.value !== 'all') params.format = formatFilter.value
    if (policyFilter.value !== 'all') params.policy_id = policyFilter.value
    if (timeRangeFilter.value !== 'all') params.time_range = timeRangeFilter.value
    if (tempLinkOnly.value) params.has_temp_links = 'true'

    const res = await api.get('/admin/files', { params })
    files.value = res.data.items || []
    total.value = res.data.total || 0
  } catch (err: any) {
    message.error(getApiErrorMessage(err))
  } finally {
    loading.value = false
  }
}

const openPreviewModal = (file: any) => {
  previewFile.value = file
  showPreviewModal.value = true
}

const openDetails = async (id: number) => {
  showDetailsModal.value = true
  detailsLoading.value = true
  try {
    const res = await api.get(`/admin/files/${id}/details`)
    detailsData.value = res.data
  } catch (err: any) {
    message.error(getApiErrorMessage(err))
  } finally {
    detailsLoading.value = false
  }
}

const deleteFile = async (id: number) => {
  try {
    await api.delete(`/admin/files/${id}`)
    message.success(t('adminFiles.deleteSuccess'))
    selectedIds.value = selectedIds.value.filter(i => i !== id)
    window.dispatchEvent(new CustomEvent('refresh-quota'))
    loadFiles()
  } catch (err: any) {
    message.error(getApiErrorMessage(err))
  }
}

const handleBatchDelete = async () => {
  if (selectedIds.value.length === 0) return
  batchDeleting.value = true
  try {
    const res = await api.post('/admin/files/batch-delete', {
      ids: selectedIds.value
    })
    message.success(t('adminFiles.batchDeleteSuccess', { count: res.data.deleted || selectedIds.value.length }))
    selectedIds.value = []
    window.dispatchEvent(new CustomEvent('refresh-quota'))
    loadFiles()
  } catch (err: any) {
    message.error(getApiErrorMessage(err))
  } finally {
    batchDeleting.value = false
  }
}

const copyText = (val: string) => {
  navigator.clipboard.writeText(val)
  message.success(t('common.copied'))
}

const formatBytes = (bytes?: number) => {
  if (!bytes || bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

const formatDate = (dateStr?: string) => {
  if (!dateStr) return '-'
  const d = new Date(dateStr)
  return d.toLocaleString()
}

onMounted(() => {
  loadPolicies()
  loadFiles()
})
</script>

<style scoped>
.admin-files-page {
  max-width: 1400px;
  margin: 0 auto;
}
</style>
