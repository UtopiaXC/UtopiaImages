<template>
  <div style="max-width: 900px; margin: 0 auto;">
    <n-card :title="$t('upload.title')" style="border-radius: 12px; margin-bottom: 24px;">
      <!-- Options Bar -->
      <div style="display: flex; gap: 24px; margin-bottom: 20px; align-items: center;">
        <n-checkbox v-model:checked="convertToWebP">
          {{ $t('upload.convertToWebP') }}
        </n-checkbox>
        <n-checkbox v-model:checked="watermark">
          {{ $t('upload.watermark') }}
        </n-checkbox>
        <n-checkbox v-model:checked="keepOriginal">
          保留原图备份
        </n-checkbox>
      </div>

      <!-- Drag & Drop / Click Upload Zone -->
      <n-upload
        multiple
        directory-dnd
        :show-file-list="false"
        :custom-request="handleCustomUpload"
      >
        <n-upload-dragger style="padding: 40px; background: rgba(255,255,255,0.02); border-radius: 12px;">
          <div style="font-size: 48px; margin-bottom: 12px;">📤</div>
          <n-text style="font-size: 16px;">
            {{ $t('upload.dragTip') }}
          </n-text>
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
            <div style="font-weight: bold; font-size: 15px; margin-bottom: 10px;">
              {{ item.file.display_name }} ({{ formatBytes(item.file.size) }})
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
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useMessage, type UploadCustomRequestOptions } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'

const message = useMessage()
const { t } = useI18n()

const convertToWebP = ref(true)
const watermark = ref(false)
const keepOriginal = ref(true)
const results = ref<any[]>([])

const handleCustomUpload = async ({ file, onFinish, onError }: UploadCustomRequestOptions) => {
  const formData = new FormData()
  formData.append('file', file.file as File)
  formData.append('convert_webp', convertToWebP.value ? 'true' : 'false')
  formData.append('keep_original', keepOriginal.value ? 'true' : 'false')
  formData.append('watermark_mode', watermark.value ? 'on' : 'off')

  try {
    const res = await api.post('/upload', formData)
    results.value.unshift(res.data)
    message.success(`${file.name} 上传成功`)
    onFinish()
  } catch (err: any) {
    message.error(err.response?.data?.error || `${file.name} 上传失败`)
    onError()
  }
}

// Paste from clipboard support (Ctrl+V)
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
  formData.append('watermark_mode', watermark.value ? 'on' : 'off')

  try {
    const res = await api.post('/upload', formData)
    results.value.unshift(res.data)
    message.success('已自动上传剪贴板图片')
  } catch (err: any) {
    message.error(err.response?.data?.error || '上传剪贴板图片失败')
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
  window.addEventListener('paste', handlePaste)
})

onUnmounted(() => {
  window.removeEventListener('paste', handlePaste)
})
</script>
