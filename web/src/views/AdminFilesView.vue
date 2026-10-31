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

          <!-- Advanced Filter Button -->
          <n-button
            size="small"
            :type="activeFilterCount > 0 ? 'primary' : 'default'"
            :secondary="activeFilterCount === 0"
            @click="showFilterModal = true"
          >
            <template #icon>
              <n-icon :component="FilterOutline" />
            </template>
            <span>{{ $t('filter.button') }}</span>
            <n-badge
              v-if="activeFilterCount > 0"
              :value="activeFilterCount"
              type="info"
              style="margin-left: 6px;"
            />
          </n-button>

          <!-- Clear Filters Button -->
          <n-button
            v-if="activeFilterCount > 0"
            size="small"
            quaternary
            @click="clearAllFilters"
          >
            {{ $t('filter.clear') }}
          </n-button>
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

        <!-- Active Filter Tags / Chips -->
        <div v-if="activeFilterChips.length > 0" style="width: 100%; display: flex; flex-wrap: wrap; gap: 6px; align-items: center; margin-top: 10px; padding-top: 10px; border-top: 1px solid rgba(255, 255, 255, 0.05);">
          <span style="font-size: 12px; color: #888;">{{ $t('filter.activeConditions') }}:</span>
          <template v-for="chip in activeFilterChips" :key="chip.index">
            <span v-if="chip.connector" style="font-size: 11px; font-weight: 700; color: #63e2b7; margin: 0 2px;">
              {{ chip.connector }}
            </span>
            <n-tag
              size="small"
              closable
              type="info"
              @close="removeSingleFilter(chip.index)"
            >
              {{ chip.label }}
            </n-tag>
          </template>
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
              <th>{{ $t('adminFiles.callStats') }}</th>
              <th>{{ $t('adminFiles.tempLinks') }}</th>
              <th>{{ $t('common.createdAt') }}</th>
              <th style="text-align: right;">{{ $t('common.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="loading">
              <td colspan="11" style="text-align: center; padding: 36px 0; color: #888;">
                <n-spin size="small" />
              </td>
            </tr>
            <tr v-else-if="files.length === 0">
              <td colspan="11" style="text-align: center; padding: 40px 0; color: #777;">
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
                <n-tag size="tiny" type="info">
                  {{ file.total_calls || 0 }} PV · {{ file.total_uv || 0 }} UV
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

        <!-- Call Statistics Section -->
        <div style="margin-bottom: 20px;">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px;">
            <h4 style="margin: 0; font-size: 14px; font-weight: 600;">{{ $t('adminFiles.callStats') }}</h4>
            <div style="display: flex; gap: 8px;">
              <n-tag type="info" size="small">
                PV: {{ detailsData.total_calls || 0 }}
              </n-tag>
              <n-tag type="success" size="small">
                UV: {{ detailsData.total_uv || 0 }}
              </n-tag>
            </div>
          </div>
          <div v-if="(detailsData.call_stats || []).length > 0" class="table-responsive">
            <table style="width: 100%; border-collapse: collapse; font-size: 13px; background: rgba(255,255,255,0.02); border-radius: 6px;">
              <thead>
                <tr style="border-bottom: 1px solid rgba(255,255,255,0.08); color: #888; text-align: left;">
                  <th style="padding: 8px 12px;">{{ $t('adminFiles.statFormat') }}</th>
                  <th style="padding: 8px 12px;">{{ $t('adminFiles.statSize') }}</th>
                  <th style="padding: 8px 12px; text-align: right;">PV</th>
                  <th style="padding: 8px 12px; text-align: right;">UV</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="cs in detailsData.call_stats" :key="cs.id" style="border-bottom: 1px solid rgba(255,255,255,0.04);">
                  <td style="padding: 8px 12px; font-family: monospace;">
                    <n-tag size="tiny" type="success">{{ cs.format }}</n-tag>
                  </td>
                  <td style="padding: 8px 12px; color: #ccc;">{{ cs.size }}</td>
                  <td style="padding: 8px 12px; text-align: right; font-weight: 600; color: #63e2b7;">{{ cs.count }}</td>
                  <td style="padding: 8px 12px; text-align: right; font-weight: 600; color: #70c0e8;">{{ cs.uv || 0 }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <div v-else style="color: #777; font-size: 13px; padding: 8px 0;">
            {{ $t('adminFiles.noCallStats') }}
          </div>
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

    <!-- Advanced Filter Modal -->
    <FileFilterModal
      v-model:show="showFilterModal"
      is-admin
      :policies="policyOptions"
      :initial-payload="filterPayload"
      @apply="onApplyFilter"
      @reset="onResetFilter"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { SearchOutline, FilterOutline } from '@vicons/ionicons5'
import { api, getApiErrorMessage } from '../api/client'
import FileLinksPanel from '../components/FileLinksPanel.vue'
import FileFilterModal, { type AdvancedFilterPayload } from '../components/FileFilterModal.vue'
import { formatFilterRuleLabel } from '../utils/filterLabel'

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
  const base = file.thumbnail_url || `/${(file.public_path || '').replace(/^\/+/, '')}?s=thumb`
  return base.includes('?') ? `${base}&from=console` : `${base}?from=console`
}

const resolveFileUrl = (file: any) => {
  if (!file) return ''
  const base = file.url || `/${(file.public_path || '').replace(/^\/+/, '')}`
  return base.includes('?') ? `${base}&from=console` : `${base}?from=console`
}

const loading = ref(false)
const batchDeleting = ref(false)
const files = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)

// Filters
const search = ref('')
const showFilterModal = ref(false)
const filterPayload = ref<AdvancedFilterPayload>({ logic: 'and', rules: [] })

const activeFilterCount = computed(() => {
  return filterPayload.value.rules ? filterPayload.value.rules.length : 0
})

const activeFilterChips = computed(() => {
  if (!filterPayload.value || !filterPayload.value.rules) return []
  return filterPayload.value.rules.map((r, index) => {
    const pre = r.prefix_parens ? `${r.prefix_parens} ` : ''
    const suf = r.suffix_parens ? ` ${r.suffix_parens}` : ''
    const core = formatFilterRuleLabel(r, policyOptions.value, t)
    return {
      index,
      connector: index > 0 ? (filterPayload.value.rules[index - 1]?.connector || 'AND') : '',
      label: `${pre}${core}${suf}`
    }
  })
})

const onApplyFilter = (payload: AdvancedFilterPayload) => {
  filterPayload.value = payload
  page.value = 1
  loadFiles()
}

const onResetFilter = () => {
  filterPayload.value = { logic: 'and', rules: [] }
  page.value = 1
  loadFiles()
}

const removeSingleFilter = (index: number) => {
  if (filterPayload.value && filterPayload.value.rules) {
    filterPayload.value.rules.splice(index, 1)
    page.value = 1
    loadFiles()
  }
}

const clearAllFilters = () => {
  filterPayload.value = { logic: 'and', rules: [] }
  page.value = 1
  loadFiles()
}

// Selections
const selectedIds = ref<number[]>([])

// Details and preview
const showPreviewModal = ref(false)
const previewFile = ref<any>(null)
const showDetailsModal = ref(false)
const detailsLoading = ref(false)
const detailsData = ref<any>(null)

// Storage Policies
const policyOptions = ref<{ label: string; value: string }[]>([])

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

const loadPolicies = async () => {
  try {
    const res = await api.get('/admin/policies')
    const list = res.data || []
    policyOptions.value = [
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
    if (filterPayload.value.rules && filterPayload.value.rules.length > 0) {
      params.filter_rules = JSON.stringify(filterPayload.value)
    }

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
