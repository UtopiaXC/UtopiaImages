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
        <n-checkbox v-model:checked="convertToWebP">
          {{ $t('upload.convertToWebP') }}
        </n-checkbox>

        <n-checkbox v-model:checked="keepOriginal">
          {{ $t('upload.keepOriginal') }}
        </n-checkbox>

        <div style="display: flex; align-items: center; gap: 8px;">
          <span style="font-size: 13px; color: #aaa;">{{ $t('upload.watermark') }}：</span>
          <n-select
            v-model:value="watermarkMode"
            :options="watermarkModeOptions"
            size="small"
            style="width: 140px;"
          />
        </div>

        <div v-if="watermarkMode === 'on'" style="display: flex; align-items: center; gap: 8px;">
          <span style="font-size: 13px; color: #aaa;">{{ $t('upload.template') }}：</span>
          <n-select
            v-model:value="selectedTemplateId"
            :options="templateOptions"
            size="small"
            :placeholder="$t('upload.defaultTemplate')"
            style="width: 160px;"
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
        <div style="display: flex; gap: 20px; align-items: center;">
          <img
            v-if="item.file.is_image"
            :src="item.links.url"
            style="width: 100px; height: 100px; object-fit: cover; border-radius: 8px; border: 1px solid #333;"
          />
          <div style="flex: 1;">
            <div style="font-weight: bold; font-size: 15px; margin-bottom: 10px; display: flex; align-items: center; gap: 8px;">
              <span>{{ item.file.display_name }}</span>
              <n-tag size="small" :bordered="false">{{ formatBytes(item.file.size) }}</n-tag>
            </div>
            
            <n-tabs type="line" size="small">
              <n-tab-pane name="url" :tab="$t('upload.directLink')">
                <n-input-group>
                  <n-input :value="item.links.url" readonly />
                  <n-button type="primary" @click="copyText(item.links.url)">{{ $t('common.copy') }}</n-button>
                </n-input-group>
              </n-tab-pane>
              <n-tab-pane name="markdown" :tab="$t('upload.markdown')">
                <n-input-group>
                  <n-input :value="item.links.markdown" readonly />
                  <n-button type="primary" @click="copyText(item.links.markdown)">{{ $t('common.copy') }}</n-button>
                </n-input-group>
              </n-tab-pane>
              <n-tab-pane name="html" :tab="$t('upload.html')">
                <n-input-group>
                  <n-input :value="item.links.html" readonly />
                  <n-button type="primary" @click="copyText(item.links.html)">{{ $t('common.copy') }}</n-button>
                </n-input-group>
              </n-tab-pane>
              <n-tab-pane name="bbcode" :tab="$t('upload.bbcode')">
                <n-input-group>
                  <n-input :value="item.links.bbcode" readonly />
                  <n-button type="primary" @click="copyText(item.links.bbcode)">{{ $t('common.copy') }}</n-button>
                </n-input-group>
              </n-tab-pane>
            </n-tabs>
          </div>
        </div>
      </n-card>
    </div>

    <!-- ZIP Archive Import Modal -->
    <n-modal v-model:show="showImportModal" preset="card" :title="$t('upload.archiveImport')" style="width: 520px; border-radius: 12px;">
      <p style="color: #aaa; font-size: 13px; margin-bottom: 20px;">
        {{ $t('upload.archiveImportTip') }}
      </p>

      <n-form label-placement="left" label-width="120">
        <n-form-item :label="$t('upload.selectZipFile')">
          <input type="file" accept=".zip" @change="handleZipFileChange" style="color: #ccc;" />
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
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useMessage, type UploadCustomRequestOptions } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'

const message = useMessage()
const { t } = useI18n()

const convertToWebP = ref(true)
const keepOriginal = ref(true)
const watermarkMode = ref('inherit')
const selectedTemplateId = ref<string | null>(null)
const templateOptions = ref<any[]>([])
const results = ref<any[]>([])

const showImportModal = ref(false)
const selectedZipFile = ref<File | null>(null)
const importing = ref(false)
const importOptions = ref({
  preserve_path: true,
  strip_top_dir: false,
  conflict_action: 'skip'
})

const handleZipFileChange = (e: Event) => {
  const target = e.target as HTMLInputElement
  if (target.files && target.files.length > 0) {
    selectedZipFile.value = target.files[0]
  }
}

const handleImportZip = async () => {
  if (!selectedZipFile.value) return
  importing.value = true
  const formData = new FormData()
  formData.append('file', selectedZipFile.value)
  formData.append('preserve_path', importOptions.value.preserve_path ? 'true' : 'false')
  formData.append('strip_top_dir', importOptions.value.strip_top_dir ? 'true' : 'false')
  formData.append('conflict_action', importOptions.value.conflict_action)

  try {
    const res = await api.post('/import/zip', formData)
    message.success(`${t('upload.importSuccess')}: ${res.data.imported} / ${res.data.total}`)
    showImportModal.value = false
    selectedZipFile.value = null
  } catch (err: any) {
    message.error(err.response?.data?.error || t('upload.importFailed'))
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

const handleCustomUpload = async ({ file, onFinish, onError }: UploadCustomRequestOptions) => {
  const formData = new FormData()
  formData.append('file', file.file as File)
  formData.append('convert_webp', convertToWebP.value ? 'true' : 'false')
  formData.append('keep_original', keepOriginal.value ? 'true' : 'false')
  formData.append('watermark_mode', watermarkMode.value)
  if (selectedTemplateId.value) {
    formData.append('watermark_template_id', selectedTemplateId.value)
  }

  try {
    const res = await api.post('/upload', formData)
    results.value.unshift(res.data)
    message.success(`${file.name} ${t('upload.uploadSuccess')}`)
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
  formData.append('convert_webp', convertToWebP.value ? 'true' : 'false')
  formData.append('keep_original', keepOriginal.value ? 'true' : 'false')
  formData.append('watermark_mode', watermarkMode.value)
  if (selectedTemplateId.value) {
    formData.append('watermark_template_id', selectedTemplateId.value)
  }

  try {
    const res = await api.post('/upload', formData)
    results.value.unshift(res.data)
    message.success(t('upload.clipboardSuccess'))
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
  window.addEventListener('paste', handlePaste)
})

onUnmounted(() => {
  window.removeEventListener('paste', handlePaste)
})
</script>
