<template>
  <div style="max-width: 900px; margin: 0 auto;">
    <n-card :title="$t('upload.title')" style="border-radius: 12px; margin-bottom: 24px;">
      <template #header-extra>
        <n-button size="small" secondary @click="showImportModal = true">
          {{ $t('upload.archiveImport') }}
        </n-button>
      </template>

      <!-- Options Bar -->
      <div class="upload-options-container">
        <div class="upload-options-row">
          <div class="upload-option-item">
            <span class="upload-option-label">{{ $t('upload.storagePolicy') }}</span>
            <span class="upload-option-colon">:</span>
            <PolicySelectButton
              v-model:value="selectedPolicyIds"
              :policies="policiesList"
              :min="1"
              size="small"
              :placeholder="$t('upload.storagePolicy')"
            />
          </div>

          <div class="upload-option-item">
            <span class="upload-option-label">{{ $t('upload.watermark') }}</span>
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-icon size="16" style="color: #888; cursor: pointer; vertical-align: middle;">
                  <InformationCircleOutline />
                </n-icon>
              </template>
              {{ $t('upload.watermarkTip') }}
            </n-tooltip>
            <span class="upload-option-colon">:</span>
            <n-select
              v-model:value="watermarkMode"
              :options="watermarkModeOptions"
              size="small"
              style="width: 120px;"
            />
          </div>

          <div v-if="watermarkMode === 'on'" class="upload-option-item">
            <span class="upload-option-label">{{ $t('upload.template') }}</span>
            <span class="upload-option-colon">:</span>
            <n-select
              v-model:value="selectedTemplateId"
              :options="templateOptions"
              size="small"
              :placeholder="$t('upload.defaultTemplate')"
              style="width: 150px;"
            />
          </div>

          <div class="upload-option-item">
            <span class="upload-option-label">{{ $t('files.publicMode') }}</span>
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-icon size="16" style="color: #888; cursor: pointer; vertical-align: middle;">
                  <InformationCircleOutline />
                </n-icon>
              </template>
              {{ $t('files.publicTip') }}
            </n-tooltip>
            <span class="upload-option-colon">:</span>
            <n-select
              v-model:value="publicMode"
              :options="publicModeOptions"
              size="small"
              style="width: 120px;"
            />
          </div>
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
              :src="item.links.url ? (item.links.url.includes('?') ? item.links.url + '&from=console' : item.links.url + '?from=console') : ''"
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
            <div style="font-weight: bold; font-size: 15px; margin-bottom: 10px; display: flex; align-items: center; gap: 8px; flex-wrap: wrap;">
              <span style="text-overflow: ellipsis; overflow: hidden; white-space: nowrap;">{{ item.file.display_name }}</span>
              <n-tag size="small" :bordered="false">{{ formatBytes(item.file.size) }}</n-tag>
              <n-tag v-if="item.file.width && item.file.height" size="small" type="info" :bordered="false">
                {{ item.file.width }} × {{ item.file.height }}
              </n-tag>
              <template v-if="item.links?.replicas">
                <n-tag
                  v-for="rep in item.links.replicas"
                  :key="rep.id"
                  size="small"
                  :type="rep.status === 'ready' ? 'success' : 'error'"
                >
                  {{ rep.policy_name }}
                </n-tag>
              </template>
            </div>
            
            <FileLinksPanel :file="item.file" :links="item.links" :initial-url="item.links.url" />
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
      <n-alert v-if="selectedZipFile && selectedZipFile.size > 20 * 1024 * 1024" type="info" :show-icon="false" style="margin-bottom: 16px; font-size: 12px; line-height: 1.5;">
        {{ $t('upload.chunkModeNotice') }}
      </n-alert>

      <p style="color: #aaa; font-size: 13px; margin-bottom: 20px;">
        {{ $t('upload.archiveImportTip') }}
      </p>

      <n-form label-placement="left" label-width="120">
        <n-form-item :label="$t('upload.selectArchiveFile')">
          <input type="file" accept=".zip,.tar,.gz,.tgz,.7z" @change="handleZipFileChange" style="color: #ccc;" />
        </n-form-item>

        <n-form-item :label="$t('upload.storagePolicy')">
          <PolicySelectButton
            v-model:value="importOptions.policy_ids"
            :policies="policiesList"
            :min="1"
            size="small"
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
          <n-button v-if="importing" type="error" ghost @click="cancelImport">{{ $t('upload.cancelImportAndClear') }}</n-button>
          <n-button v-else @click="showImportModal = false">{{ $t('common.cancel') }}</n-button>
          <n-button type="primary" :loading="importing" :disabled="!selectedZipFile" @click="handleImportZip">
            {{ $t('upload.startImport') }}
          </n-button>
        </div>
      </template>
    </n-modal>

    <!-- Batch Upload Policy Failures Modal -->
    <n-modal
      v-model:show="showUploadFailuresModal"
      preset="card"
      :title="$t('upload.failuresModalTitle')"
      style="width: 650px; max-width: 95vw; border-radius: 12px;"
    >
      <n-alert type="warning" :show-icon="false" style="margin-bottom: 16px;">
        {{ $t('upload.failuresModalDesc') }}
      </n-alert>
      <div class="table-responsive">
        <n-table size="small" :bordered="false" :single-line="false">
          <thead>
            <tr>
              <th>{{ $t('files.fileName') }}</th>
              <th style="width: 140px;">{{ $t('upload.storagePolicy') }}</th>
              <th>{{ $t('upload.failureReason') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(item, idx) in uploadFailures" :key="idx">
              <td style="font-weight: 500; word-break: break-all;">{{ item.fileName }}</td>
              <td>
                <n-tag size="small" type="error">{{ item.policyName }}</n-tag>
              </td>
              <td style="color: #e88080; font-size: 13px; word-break: break-all;">{{ item.error }}</td>
            </tr>
          </tbody>
        </n-table>
      </div>
      <template #footer>
        <div style="display: flex; justify-content: flex-end;">
          <n-button type="primary" size="small" @click="showUploadFailuresModal = false; uploadFailures = []">
            {{ $t('common.confirm') }}
          </n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, h } from 'vue'
import { useMessage, NTag, type UploadCustomRequestOptions } from 'naive-ui'
import { InformationCircleOutline } from '@vicons/ionicons5'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'
import { uploadInChunks } from '../utils/chunkUploader'
import FileLinksPanel from '../components/FileLinksPanel.vue'
import ImageEditorModal from '../components/ImageEditorModal.vue'
import PolicySelectButton from '../components/PolicySelectButton.vue'

const message = useMessage()
const { t } = useI18n()

const watermarkMode = ref('inherit')
const publicMode = ref('inherit')
const publicModeOptions = computed(() => [
  { label: t('files.publicInherit'), value: 'inherit' },
  { label: t('files.publicOn'), value: 'public' },
  { label: t('files.publicOff'), value: 'private' }
])
const selectedTemplateId = ref<string | null>(null)
const templateOptions = ref<any[]>([])
const results = ref<any[]>([])

const selectedPolicyIds = ref<number[]>([])
const policyOptions = ref<any[]>([])

const showImportModal = ref(false)
const selectedZipFile = ref<File | null>(null)
const importing = ref(false)
let importAbortController: AbortController | null = null

const cancelImport = () => {
  if (importAbortController) {
    importAbortController.abort()
    importAbortController = null
  }
  importing.value = false
  uploadStatus.value = 'warning'
  uploadStatusText.value = '导入任务已取消，临时缓存已清除'
  message.warning('任务已取消，已清空服务端分片缓存')
}

const importOptions = ref({
  policy_ids: [] as number[],
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
  importAbortController = new AbortController()

  const policiesToUse = importOptions.value.policy_ids.length > 0 ? importOptions.value.policy_ids : selectedPolicyIds.value

  try {
    // If archive > 20MB, use 20MB chunked streaming & SHA-256 verification
    if (selectedZipFile.value.size > 20 * 1024 * 1024) {
      const res = await uploadInChunks({
        file: selectedZipFile.value,
        uploadType: 'archive',
        params: {
          policy_ids: policiesToUse,
          watermark_mode: importOptions.value.watermark_mode,
          watermark_template_id: importOptions.value.watermark_template_id,
          hotlink_mode: importOptions.value.hotlink_mode,
          preserve_path: importOptions.value.preserve_path,
          strip_top_dir: importOptions.value.strip_top_dir,
          conflict_action: importOptions.value.conflict_action
        },
        abortController: importAbortController,
        onProgress: (p) => {
          uploadPercentage.value = p.percent
          uploadStatusText.value = p.text
        }
      })
      uploadStatus.value = 'success'
      message.success(`${t('upload.importSuccess')}: ${res.imported} / ${res.total}`)
      window.dispatchEvent(new CustomEvent('refresh-quota'))
      showImportModal.value = false
      selectedZipFile.value = null
      return
    }

    // Direct upload for small archives (<= 20MB)
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
    for (const pid of policiesToUse) {
      formData.append('policy_ids[]', String(pid))
    }
    if (policiesToUse.length > 0) {
      formData.append('policy_id', String(policiesToUse[0]))
    }

    const res = await api.post('/import/archive', formData, {
      timeout: 0,
      signal: importAbortController.signal,
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
    if (err.message === 'Upload cancelled by user' || importAbortController?.signal?.aborted) {
      uploadStatus.value = 'warning'
      uploadStatusText.value = '导入任务已取消，临时缓存已清除'
      return
    }
    uploadStatus.value = 'error'
    let errMsg = err.response?.data?.error
    if (!errMsg) {
      if (err.response?.status === 413) {
        errMsg = '上传文件过大被拦截: 413 Payload Too Large。若经过 Cloudflare 代理，免费或专业版单请求体上限为 100MB。'
      } else if (err.code === 'ECONNABORTED' || err.message?.includes('timeout') || err.response?.status === 524) {
        errMsg = '上传超时: 524 Timeout。'
      } else {
        errMsg = err.message || t('upload.importFailed')
      }
    }
    message.error(errMsg, { duration: 6000 })
  } finally {
    importing.value = false
    importAbortController = null
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
const selectedPolicies = computed(() => {
  return policiesList.value.filter((p: any) => selectedPolicyIds.value.includes(p.id))
})



const loadingPolicies = ref(false)

const loadPolicies = async () => {
  loadingPolicies.value = true
  try {
    const res = await api.get('/me/policies')
    const list = res.data || []
    policiesList.value = list
    policyOptions.value = list.map((item: any) => {
      const used = formatBytes(item.used_bytes || 0)
      const cap = item.max_capacity_bytes > 0 ? formatBytes(item.max_capacity_bytes) : t('common.unlimited')
      return {
        label: `${item.name} · 容量: ${used} / ${cap}`,
        shortLabel: item.name,
        value: item.id
      }
    })
    if (list.length > 0) {
      const def = list.find((p: any) => p.is_default) || list[0]
      if (selectedPolicyIds.value.length === 0) {
        selectedPolicyIds.value = [def.id]
      }
      if (importOptions.value.policy_ids.length === 0) {
        importOptions.value.policy_ids = [def.id]
      }
    }
  } catch (err) {
    console.error('Failed to load storage policies', err)
  } finally {
    loadingPolicies.value = false
  }
}

watch(showImportModal, (show) => {
  if (!show && importing.value) {
    cancelImport()
  }
  if (show) {
    if (importOptions.value.policy_ids.length === 0 && selectedPolicyIds.value.length > 0) {
      importOptions.value.policy_ids = [...selectedPolicyIds.value]
    } else if (importOptions.value.policy_ids.length === 0 && policyOptions.value.length > 0) {
      importOptions.value.policy_ids = [policyOptions.value[0].value]
    }
    if (policyOptions.value.length === 0) {
      loadPolicies()
    }
  }
})

interface UploadFailureRecord {
  fileName: string
  policyName: string
  error: string
}

const activeUploadCount = ref(0)
const uploadFailures = ref<UploadFailureRecord[]>([])
const showUploadFailuresModal = ref(false)
let uploadEndCheckTimer: any = null

const triggerUploadEndCheck = () => {
  clearTimeout(uploadEndCheckTimer)
  uploadEndCheckTimer = setTimeout(() => {
    if (activeUploadCount.value <= 0 && uploadFailures.value.length > 0) {
      showUploadFailuresModal.value = true
    }
  }, 300)
}

const checkFailuresInResponse = (data: any, fileName: string) => {
  if (data?.failed_policies && Array.isArray(data.failed_policies) && data.failed_policies.length > 0) {
    for (const fp of data.failed_policies) {
      uploadFailures.value.push({
        fileName,
        policyName: fp.policy_name || `策略 #${fp.policy_id}`,
        error: fp.error || t('common.failed')
      })
    }
  }
}

const handleCustomUpload = async ({ file, onFinish, onError }: UploadCustomRequestOptions) => {
  const rawFile = file.file as File
  if (!rawFile) {
    onError()
    return
  }

  activeUploadCount.value++
  clearTimeout(uploadEndCheckTimer)

  try {
    // If file > 20MB, use chunked upload with 20MB chunks
    if (rawFile.size > 20 * 1024 * 1024) {
      const data = await uploadInChunks({
        file: rawFile,
        uploadType: 'file',
        params: {
          watermark_mode: watermarkMode.value,
          watermark_template_id: selectedTemplateId.value,
          policy_ids: selectedPolicyIds.value
        }
      })
      results.value.unshift(data)
      checkFailuresInResponse(data, file.name)
      message.success(`${file.name} ${t('upload.uploadSuccess')}`)
      window.dispatchEvent(new CustomEvent('refresh-quota'))
      onFinish()
      return
    }

    const formData = new FormData()
    formData.append('file', rawFile)
    formData.append('watermark_mode', watermarkMode.value)
    formData.append('public_mode', publicMode.value)
    if (selectedTemplateId.value) {
      formData.append('watermark_template_id', selectedTemplateId.value)
    }
    for (const pid of selectedPolicyIds.value) {
      formData.append('policy_ids[]', String(pid))
    }
    if (selectedPolicyIds.value.length > 0) {
      formData.append('policy_id', String(selectedPolicyIds.value[0]))
    }

    const res = await api.post('/upload', formData)
    results.value.unshift(res.data)
    checkFailuresInResponse(res.data, file.name)
    message.success(`${file.name} ${t('upload.uploadSuccess')}`)
    window.dispatchEvent(new CustomEvent('refresh-quota'))
    onFinish()
  } catch (err: any) {
    uploadFailures.value.push({
      fileName: file.name,
      policyName: t('upload.allPolicies'),
      error: err.response?.data?.error || err.message || t('upload.uploadFailed')
    })
    onError()
  } finally {
    activeUploadCount.value--
    triggerUploadEndCheck()
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
  activeUploadCount.value++
  clearTimeout(uploadEndCheckTimer)

  const formData = new FormData()
  formData.append('file', blob, name)
  formData.append('watermark_mode', watermarkMode.value)
  if (selectedTemplateId.value) {
    formData.append('watermark_template_id', selectedTemplateId.value)
  }
  for (const pid of selectedPolicyIds.value) {
    formData.append('policy_ids[]', String(pid))
  }
  if (selectedPolicyIds.value.length > 0) {
    formData.append('policy_id', String(selectedPolicyIds.value[0]))
  }

  try {
    const res = await api.post('/upload', formData)
    results.value.unshift(res.data)
    checkFailuresInResponse(res.data, name)
    message.success(t('upload.clipboardSuccess'))
    window.dispatchEvent(new CustomEvent('refresh-quota'))
  } catch (err: any) {
    uploadFailures.value.push({
      fileName: name,
      policyName: t('upload.allPolicies'),
      error: err.response?.data?.error || t('upload.clipboardFailed')
    })
  } finally {
    activeUploadCount.value--
    triggerUploadEndCheck()
  }
}

const copyText = (text: string) => {
  navigator.clipboard.writeText(text)
  message.success(t('common.copied'))
}

const formatBytes = (bytes: number) => {
  if (!bytes || bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  const idx = Math.min(Math.max(0, i), sizes.length - 1)
  return parseFloat((bytes / Math.pow(k, idx)).toFixed(1)) + ' ' + sizes[idx]
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
.upload-options-container {
  margin-bottom: 20px;
}

.upload-options-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 16px;
}

.upload-option-item {
  display: flex;
  align-items: center;
  gap: 8px;
}

.upload-option-label {
  font-size: 13px;
  color: #aaa;
  white-space: nowrap;
}

.upload-option-colon {
  color: #666;
}

.upload-policy-select {
  min-width: 200px;
  max-width: 280px;
}

@media (max-width: 640px) {
  .upload-options-row {
    flex-direction: column;
    align-items: stretch;
    gap: 12px;
  }
  .upload-option-item {
    width: 100%;
    justify-content: space-between;
  }
  .upload-policy-select {
    width: 100% !important;
    max-width: none !important;
  }
}

@media (max-width: 640px) {
  .upload-result-item {
    flex-direction: column !important;
    align-items: stretch !important;
  }
}
</style>
