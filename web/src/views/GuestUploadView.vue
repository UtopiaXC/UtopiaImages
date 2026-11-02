<template>
  <div class="guest-page-wrapper">
    <PublicNavBar />

    <main class="guest-content-container">
      <div style="max-width: 900px; margin: 0 auto; width: 100%;">
        <n-card :title="$t('guest.uploadTitle')" style="border-radius: 12px; margin-bottom: 24px;">


          <!-- Upload Drop Zone -->
          <n-upload
            multiple
            directory-dnd
            :show-file-list="false"
            :custom-request="handleCustomUpload"
            accept="image/*,.jpg,.jpeg,.png,.gif,.webp,.heif,.heic,.bmp,.svg"
          >
            <n-upload-dragger style="padding: 48px 24px; background: rgba(255, 255, 255, 0.02); border-radius: 12px;">
              <div style="font-size: 16px; margin-bottom: 8px; font-weight: 500;">
                {{ $t('upload.dragTip') }}
              </div>
              <div style="font-size: 13px; color: #777;">
                {{ $t('guest.onlyImagesTip') }}
              </div>
            </n-upload-dragger>
          </n-upload>
        </n-card>

        <!-- Upload Results -->
        <div v-if="results.length > 0">
          <n-card
            v-for="(item, index) in results"
            :key="index"
            style="margin-bottom: 16px; border-radius: 12px;"
          >
            <div style="display: flex; gap: 20px; align-items: flex-start; flex-wrap: wrap;">
              <div style="display: flex; flex-direction: column; align-items: center; gap: 8px;">
                <img
                  v-if="item.file.is_image"
                  :src="item.links.url || ''"
                  style="width: 110px; height: 110px; object-fit: contain; background: #0c0d12; border-radius: 8px; border: 1px solid #333;"
                />
              </div>
              <div style="flex: 1; min-width: 260px;">
                <div style="font-weight: bold; font-size: 15px; margin-bottom: 10px; display: flex; align-items: center; gap: 8px;">
                  <span style="text-overflow: ellipsis; overflow: hidden; white-space: nowrap;">{{ item.file.display_name }}</span>
                  <n-tag size="small" :bordered="false">{{ formatBytes(item.file.size) }}</n-tag>
                  <n-tag v-if="item.file.width && item.file.height" size="small" type="info" :bordered="false">
                    {{ item.file.width }} × {{ item.file.height }}
                  </n-tag>
                </div>

                <!-- Only Read-Only Links Panel for guests -->
                <FileLinksPanel :file="item.file" :initial-url="item.links.url" :hide-temp-links="true" />
              </div>
            </div>
          </n-card>
        </div>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useMessage } from 'naive-ui'
import { InformationCircleOutline } from '@vicons/ionicons5'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'
import { siteInfo } from '../utils/site'
import PublicNavBar from '../components/PublicNavBar.vue'
import FileLinksPanel from '../components/FileLinksPanel.vue'

const message = useMessage()
const { t } = useI18n()



interface UploadResult {
  file: any
  links: any
}

const results = ref<UploadResult[]>([])

const formatBytes = (bytes: number): string => {
  if (!bytes || bytes <= 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

const handleCustomUpload = async ({ file, onFinish, onError }: any) => {
  const f = file.file as File
  if (!f) {
    onError()
    return
  }

  // Pre-validate file size (5MB default limit for guest)
  if (f.size > 5 * 1024 * 1024) {
    message.error(t('guest.onlyImagesTip'))
    onError()
    return
  }

  const formData = new FormData()
  formData.append('file', f)
  formData.append('public_mode', 'public')

  try {
    const res = await api.post('/guest/upload', formData)
    results.value.unshift({
      file: res.data.file,
      links: res.data.links
    })
    message.success(t('upload.uploadSuccess'))
    onFinish()
  } catch (err: any) {
    message.error(err.response?.data?.error || t('upload.uploadFailed'))
    onError()
  }
}
</script>

<style scoped>
.guest-page-wrapper {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  background: #101014;
}

.guest-content-container {
  flex: 1;
  padding: 32px 20px;
}
</style>

