<template>
  <div style="max-width: 900px; margin: 0 auto;">
    <n-card :title="$t('upload.title')" style="border-radius: 12px; margin-bottom: 24px;">
      <template #header-extra>
        <n-button size="small" secondary @click="showImportModal = true">
          {{ $t('upload.archiveImport') }}
        </n-button>
      </template>

      <!-- Options Bar -->
      <div style="display: flex; flex-wrap: wrap; gap: 20px; margin-bottom: 20px; align-items: center;">
        <div style="display: flex; align-items: center; gap: 8px;">
          <span style="font-size: 13px; color: #aaa;">{{ $t('upload.storagePolicy') }}</span>
          <span style="color: #666;">:</span>
          <n-select
            v-model:value="selectedPolicyId"
            :options="policyOptions"
            size="small"
            :loading="loadingPolicies"
            :placeholder="$t('upload.storagePolicy')"
            style="width: 170px;"
          />
          <!-- Storage Policy Remaining Capacity Indicator (Requirement 6) -->
          <div
            v-if="currentPolicy"
            style="display: flex; align-items: center; background: rgba(255,255,255,0.03); padding: 4px 10px; border-radius: 6px; border: 1px solid rgba(255,255,255,0.06);"
          >
            <div style="display: flex; flex-direction: column; min-width: 140px; width: auto;">
              <div style="display: flex; justify-content: space-between; align-items: center; gap: 10px; font-size: 11px; margin-bottom: 3px; white-space: nowrap;">
                <span style="color: #888;">{{ $t('upload.policyRemaining') }}</span>
                <span style="color: #63e2b7; font-weight: 500;">
                  {{ currentPolicy.max_capacity_bytes === 0 ? $t('common.unlimited') : formatBytes(Math.max(0, currentPolicy.max_capacity_bytes - currentPolicy.used_bytes)) }}
                </span>
              </div>
              <n-progress
                type="line"
                :percentage="currentPolicyPercentage"
                :status="currentPolicyPercentage >= 90 ? 'error' : currentPolicyPercentage >= 75 ? 'warning' : 'success'"
                :show-indicator="false"
              />
            </div>
          </div>
        </div>

        <div style="display: flex; align-items: center; gap: 8px;">
          <span style="font-size: 13px; color: #aaa;">{{ $t('upload.watermark') }}</span>
          <n-tooltip trigger="hover">
            <template #trigger>
              <n-icon size="16" style="color: #888; cursor: pointer; vertical-align: middle;">
                <InformationCircleOutline />
              </n-icon>
            </template>
            {{ $t('upload.watermarkTip') }}
          </n-tooltip>
          <span style="color: #666;">:</span>
          <n-select
            v-model:value="watermarkMode"
            :options="watermarkModeOptions"
            size="small"
            style="width: 140px;"
          />
        </div>

        <div v-if="watermarkMode === 'on'" style="display: flex; align-items: center; gap: 8px;">
          <span style="font-size: 13px; color: #aaa;">{{ $t('upload.template') }}</span>
          <span style="color: #666;">:</span>
          <n-select
            v-model:value="selectedTemplateId"
            :options="templateOptions"
            size="small"
            :placeholder="$t('upload.defaultTemplate')"
            style="width: 180px;"
          />
        </div>
      </div>

      <!-- Drag & Drop / Click Upload Zone -->
      <n-upload
        multiple
        directory-dnd
        :show-file-list="false"
        :custom-request="handleCustomUpload"
      >
        <n-upload-dragger style="padding: 40px; background: rgba(255,255,255,0.02); border-radius: 12px;">
          <div style="font-size: 16px; margin-bottom: 8px; font-weight: 500;">
            {{ $t('upload.dragTip') }}
          </div>
          <div style="font-size: 13px; color: #777;">
            {{ $t('upload.alwaysOriginalTip') }}
          </div>
        </n-upload-dragger>
      </n-upload>
    </n-card>

    <!-- Upload Results List -->
    <div v-if="results.length > 0">
      <n-card
        v-for="(item, index) in results"
        :key="index"
        style="margin-bottom: 16px; border-radius: 12px;"
      >
        <div class="upload-result-item" style="display: flex; gap: 20px; align-items: flex-start;">
          <div style="display: flex; flex-direction: column; align-items: center; gap: 8px;">
            <img
              v-if="item.file.is_image"
              :src="item.links.url"
              style="width: 110px; height: 110px; object-fit: contain; background: #0c0d12; border-radius: 8px; border: 1px solid #333;"
            />
            <n-button
              v-if="item.file.is_image"
              size="tiny"
              type="info"
              quaternary
              style="width: 110px;"
              @click="openImageEditor(item.file, index)"
            >
              {{ $t('files.editImage') }}
            </n-button>
          </div>
          <div style="flex: 1; min-width: 0;">
            <div style="font-weight: bold; font-size: 15px; margin-bottom: 10px; display: flex; align-items: center; gap: 8px;">
              <span style="text-overflow: ellipsis; overflow: hidden; white-space: nowrap;">{{ item.file.display_name }}</span>
              <n-tag size="small" :bordered="false">{{ formatBytes(item.file.size) }}</n-tag>
              <n-tag v-if="item.file.width && item.file.height" size="small" type="info" :bordered="false">
                {{ item.file.width }} × {{ item.file.height }}
              </n-tag>
            </div>
            
            <FileLinksPanel :file="item.file" :initial-url="item.links.url" />
          </div>
        </div>
      </n-card>
    </div>

    <!-- Image Editor Modal -->
    <ImageEditorModal
      v-model:show="showEditorModal"
      :file="editingFile"
      @saved="handleEditorSaved"
    />

    <!-- ZIP Archive Import Modal -->
    <n-modal v-model:show="showImportModal" preset="card" :title="$t('upload.archiveImport')" style="width: 520px; max-width: 95vw; border-radius: 12px;">
      <n-alert type="warning" :show-icon="false" style="margin-bottom: 16px; font-size: 12px; line-height: 1.5;">
        {{ $t('upload.cfUploadWarning') }}
      </n-alert>

      <p style="color: #aaa; font-size: 13px; margin-bottom: 20px;">
        {{ $t('upload.archiveImportTip') }}
      </p>

      <n-form label-placement="left" label-width="120">
        <n-form-item :label="$t('upload.selectArchiveFile')">
          <input type="file" accept=".zip,.tar,.gz,.tgz,.7z" @change="handleZipFileChange" style="color: #ccc;" />
        </n-form-item>

        <n-form-item :label="$t('upload.storagePolicy')">
          <n-select
            v-model:value="importOptions.policy_id"
            :options="policyOptions"
            :loading="loadingPolicies"
            :placeholder="$t('upload.storagePolicy')"
          />
        </n-form-item>

        <n-form-item :label="$t('upload.watermark')">
          <n-select v-model:value="importOptions.watermark_mode" :options="watermarkModeOptions" />
        </n-form-item>

        <n-form-item v-if="importOptions.watermark_mode === 'on'" :label="$t('upload.template')">
          <n-select v-model:value="importOptions.watermark_template_id" :options="templateOptions" clearable :placeholder="$t('upload.defaultTemplate')" />
        </n-form-item>

        <n-form-item :label="$t('files.hotlinkSetting')">
          <n-select v-model:value="importOptions.hotlink_mode" :options="hotlinkModeOptions" />
        </n-form-item>

        <n-form-item :label="$t('upload.preservePath')">
          <n-switch v-model:value="importOptions.preserve_path" />
        </n-form-item>

        <n-form-item :label="$t('upload.stripTopDir')">
          <n-switch v-model:value="importOptions.strip_top_dir" />
        </n-form-item>

        <n-form-item :label="$t('upload.conflictAction')">
          <n-radio-group v-model:value="importOptions.conflict_action">
            <n-radio value="skip">{{ $t('upload.conflictSkip') }}</n-radio>
            <n-radio value="overwrite">{{ $t('upload.conflictOverwrite') }}</n-radio>
          </n-radio-group>
        </n-form-item>
      </n-form>

      <!-- Upload Progress Indicator -->
      <div v-if="importing" style="margin-top: 16px; padding: 12px; background: rgba(255,255,255,0.03); border-radius: 6px; border: 1px solid rgba(255,255,255,0.08);">
        <div style="display: flex; justify-content: space-between; font-size: 13px; margin-bottom: 6px;">
          <span>{{ uploadStatusText }}</span>
          <span v-if="uploadPercentage < 100">{{ uploadPercentage }}%</span>
        </div>
        <n-progress
          type="line"
          :percentage="uploadPercentage"
          :processing="uploadPercentage === 100"
          :status="uploadStatus"
        />
      </div>

      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 12px;">
          <n-button @click="showImportModal = false">{{ $t('common.cancel') }}</n-button>
          <n-button type="primary" :loading="importing" :disabled="!selectedZipFile" @click="handleImportZip">
            {{ $t('upload.startImport') }}
          </n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useMessage, type UploadCustomRequestOptions } from 'naive-ui'
import { InformationCircleOutline } from '@vicons/ionicons5'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'
import FileLinksPanel from '../components/FileLinksPanel.vue'
import ImageEditorModal from '../components/ImageEditorModal.vue'

const message = useMessage()
const { t } = useI18n()

const watermarkMode = ref('inherit')
const selectedTemplateId = ref<string | null>(null)
const templateOptions = ref<any[]>([])
const results = ref<any[]>([])

const selectedPolicyId = ref<number | null>(null)
const policyOptions = ref<any[]>([])

const showImportModal = ref(false)
const selectedZipFile = ref<File | null>(null)
const importing = ref(false)
const importOptions = ref({
  policy_id: null as number | null,
  watermark_mode: 'inherit',
  watermark_template_id: null as string | null,
  hotlink_mode: 'inherit',
  preserve_path: true,
  strip_top_dir: false,
  conflict_action: 'skip'
})

const showEditorModal = ref(false)
const editingFile = ref<any>(null)
const editingIndex = ref<number>(-1)

const openImageEditor = (file: any, index: number) => {
  editingFile.value = file
  editingIndex.value = index
  showEditorModal.value = true
}

const handleEditorSaved = (updatedFile: any) => {
  if (editingIndex.value >= 0 && results.value[editingIndex.value]) {
    results.value[editingIndex.value].file = updatedFile
    if (results.value[editingIndex.value].links?.url) {
      const orig = results.value[editingIndex.value].links.url.split('?')[0]
      results.value[editingIndex.value].links.url = `${orig}?v=${Date.now()}`
    }
  }
}

const handleZipFileChange = (e: Event) => {
  const target = e.target as HTMLInputElement
  if (target.files && target.files.length > 0) {
    selectedZipFile.value = target.files[0]
  }
}

const hotlinkModeOptions = computed(() => [
  { label: t('files.hotlinkInherit'), value: 'inherit' },
  { label: t('files.hotlinkOn'), value: 'on' },
  { label: t('files.hotlinkOff'), value: 'off' }
])

const uploadPercentage = ref(0)
const uploadStatus = ref<'default' | 'success' | 'error' | 'warning' | 'info'>('default')
const uploadStatusText = ref('')

const handleImportZip = async () => {
  if (!selectedZipFile.value) return
  importing.value = true
  uploadPercentage.value = 0
  uploadStatus.value = 'default'
  uploadStatusText.value = t('upload.archiveUploading')

  const formData = new FormData()
  formData.append('archive_file', selectedZipFile.value)
  formData.append('preserve_path', importOptions.value.preserve_path ? 'true' : 'false')
  formData.append('strip_top_dir', importOptions.value.strip_top_dir ? 'true' : 'false')
  formData.append('conflict_action', importOptions.value.conflict_action)
  formData.append('watermark_mode', importOptions.value.watermark_mode)
  formData.append('hotlink_mode', importOptions.value.hotlink_mode)
  if (importOptions.value.watermark_template_id) {
    formData.append('watermark_template_id', importOptions.value.watermark_template_id)
  }
  const policyToUse = importOptions.value.policy_id || selectedPolicyId.value
  if (policyToUse) {
    formData.append('policy_id', String(policyToUse))
  }

  try {
    const res = await api.post('/import/archive', formData, {
      timeout: 0,
      onUploadProgress: (progressEvent) => {
        if (progressEvent.total) {
          const percent = Math.min(Math.round((progressEvent.loaded * 100) / progressEvent.total), 99)
          uploadPercentage.value = percent
          if (percent >= 99) {
            uploadPercentage.value = 100
            uploadStatusText.value = t('upload.archiveProcessing')
          }
        }
      }
    })
    uploadStatus.value = 'success'
    message.success(`${t('upload.importSuccess')}: ${res.data.imported} / ${res.data.total}`)
    window.dispatchEvent(new CustomEvent('refresh-quota'))
    showImportModal.value = false
    selectedZipFile.value = null
  } catch (err: any) {
    uploadStatus.value = 'error'
    let errMsg = err.response?.data?.error
    if (!errMsg) {
      if (err.response?.status === 413) {
        errMsg = '上传文件过大被拦截 (413 Payload Too Large)。若经过 Cloudflare 代理，免费/专业版单请求体上限为 100MB。请拆包导入或使用灰云朵直连域名。'
      } else if (err.code === 'ECONNABORTED' || err.message?.includes('timeout') || err.response?.status === 524) {
        errMsg = '上传超时 (524 Timeout)。若经过 Cloudflare 代理或反向代理，长时间大文件请求会超时中断。建议分卷拆包导入或直连解析。'
      } else {
        errMsg = err.message || t('upload.importFailed')
      }
    }
    message.error(errMsg, { duration: 6000 })
  } finally {
    importing.value = false
  }
}

const watermarkModeOptions = computed(() => [
  { label: t('upload.watermarkInherit'), value: 'inherit' },
  { label: t('upload.watermarkOn'), value: 'on' },
  { label: t('upload.watermarkOff'), value: 'off' }
])

const loadTemplates = async () => {
  try {
    const res = await api.get('/watermarks')
    templateOptions.value = res.data.map((item: any) => ({
      label: item.name,
      value: String(item.id)
    }))
  } catch {}
}

const policiesList = ref<any[]>([])
const currentPolicy = computed(() => {
  return policiesList.value.find((p: any) => p.id === selectedPolicyId.value)
})

const currentPolicyPercentage = computed(() => {
  if (!currentPolicy.value || !currentPolicy.value.max_capacity_bytes) return 0
  return Math.min(100, Math.round((currentPolicy.value.used_bytes / currentPolicy.value.max_capacity_bytes) * 100))
})

const loadingPolicies = ref(false)

const loadPolicies = async () => {
  loadingPolicies.value = true
  try {
    const res = await api.get('/me/policies')
    const list = res.data || []
    policiesList.value = list
    policyOptions.value = list.map((item: any) => ({
      label: item.name,
      value: item.id
    }))
    if (list.length > 0) {
      const def = list.find((p: any) => p.is_default) || list[0]
      if (!selectedPolicyId.value) {
        selectedPolicyId.value = def.id
      }
      if (!importOptions.value.policy_id) {
        importOptions.value.policy_id = def.id
      }
    }
  } catch (err) {
    console.error('Failed to load storage policies', err)
  } finally {
    loadingPolicies.value = false
  }
}

watch(showImportModal, (show) => {
  if (show) {
    if (!importOptions.value.policy_id && selectedPolicyId.value) {
      importOptions.value.policy_id = selectedPolicyId.value
    } else if (!importOptions.value.policy_id && policyOptions.value.length > 0) {
      importOptions.value.policy_id = policyOptions.value[0].value
    }
    if (policyOptions.value.length === 0) {
      loadPolicies()
    }
  }
})

const handleCustomUpload = async ({ file, onFinish, onError }: UploadCustomRequestOptions) => {
  const formData = new FormData()
  formData.append('file', file.file as File)
  formData.append('watermark_mode', watermarkMode.value)
  if (selectedTemplateId.value) {
    formData.append('watermark_template_id', selectedTemplateId.value)
  }
  if (selectedPolicyId.value) {
    formData.append('policy_id', String(selectedPolicyId.value))
  }

  try {
    const res = await api.post('/upload', formData)
    results.value.unshift(res.data)
    message.success(`${file.name} ${t('upload.uploadSuccess')}`)
    window.dispatchEvent(new CustomEvent('refresh-quota'))
    onFinish()
  } catch (err: any) {
    message.error(err.response?.data?.error || `${file.name} ${t('upload.uploadFailed')}`)
    onError()
  }
}

// Paste from clipboard support
const handlePaste = (e: ClipboardEvent) => {
  const items = e.clipboardData?.items
  if (!items) return

  for (let i = 0; i < items.length; i++) {
    if (items[i].type.indexOf('image') !== -1) {
      const blob = items[i].getAsFile()
      if (blob) {
        uploadBlob(blob, `clipboard_${Date.now()}.png`)
      }
    }
  }
}

const uploadBlob = async (blob: File, name: string) => {
  const formData = new FormData()
  formData.append('file', blob, name)
  formData.append('watermark_mode', watermarkMode.value)
  if (selectedTemplateId.value) {
    formData.append('watermark_template_id', selectedTemplateId.value)
  }
  if (selectedPolicyId.value) {
    formData.append('policy_id', String(selectedPolicyId.value))
  }

  try {
    const res = await api.post('/upload', formData)
    results.value.unshift(res.data)
    message.success(t('upload.clipboardSuccess'))
    window.dispatchEvent(new CustomEvent('refresh-quota'))
  } catch (err: any) {
    message.error(err.response?.data?.error || t('upload.clipboardFailed'))
  }
}

const copyText = (text: string) => {
  navigator.clipboard.writeText(text)
  message.success(t('common.copied'))
}

const formatBytes = (bytes: number) => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

onMounted(() => {
  loadTemplates()
  loadPolicies()
  window.addEventListener('paste', handlePaste)
})

onUnmounted(() => {
  window.removeEventListener('paste', handlePaste)
})
</script>

<style scoped>
@media (max-width: 640px) {
  .upload-result-item {
    flex-direction: column !important;
    align-items: stretch !important;
  }
}
</style>
