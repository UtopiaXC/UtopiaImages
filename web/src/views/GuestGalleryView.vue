<template>
  <div class="guest-gallery-wrapper">
    <PublicNavBar />

    <main class="guest-gallery-container">
      <div style="max-width: 1280px; margin: 0 auto; width: 100%;">
        <!-- Header Toolbar -->
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; flex-wrap: wrap; gap: 12px;">
          <div>
            <div style="font-size: 20px; font-weight: bold;">{{ $t('guest.galleryTitle') }}</div>
            <div style="font-size: 13px; color: #888; margin-top: 4px;">{{ $t('guest.galleryCount', { count: total }) }}</div>
          </div>

          <div style="display: flex; align-items: center; gap: 12px;">
            <n-input
              v-model:value="search"
              clearable
              :placeholder="$t('files.searchPlaceholder')"
              style="width: 240px;"
              @keyup.enter="handleSearch"
              @clear="handleSearch"
            >
              <template #prefix>
                <n-icon :component="SearchOutline" />
              </template>
            </n-input>
          </div>
        </div>

        <!-- Files Grid -->
        <n-spin :show="loading">
          <div v-if="files.length === 0 && !loading" style="text-align: center; padding: 60px 0; color: #777;">
            <p>{{ $t('guest.empty') }}</p>
          </div>

          <div class="gallery-grid">
            <div
              v-for="f in files"
              :key="f.id"
              class="gallery-card"
              @click="openDetails(f)"
            >
              <div class="gallery-img-wrapper">
                <img
                  v-if="f.is_image"
                  :src="f.links?.thumb_url || f.links?.url || ''"
                  loading="lazy"
                  class="gallery-img"
                />
                <div v-else class="gallery-placeholder">FILE</div>
              </div>
              <div class="gallery-card-info">
                <div class="gallery-card-name" :title="f.display_name">{{ f.display_name }}</div>
                <div class="gallery-card-meta">
                  <span>{{ formatBytes(f.size) }}</span>
                  <span v-if="f.width && f.height">{{ f.width }}×{{ f.height }}</span>
                </div>
              </div>
            </div>
          </div>

          <!-- Pagination -->
          <div v-if="total > pageSize" style="display: flex; justify-content: center; margin-top: 24px;">
            <n-pagination
              v-model:page="page"
              :page-size="pageSize"
              :item-count="total"
              @update:page="loadData"
            />
          </div>
        </n-spin>
      </div>
    </main>

    <!-- Details & Copy Links Modal -->
    <n-modal
      v-model:show="showDetailModal"
      preset="card"
      :title="selectedFile?.display_name || $t('files.detailsTitle')"
      style="width: 680px; max-width: 95vw; border-radius: 12px;"
    >
      <div v-if="selectedFile" style="display: flex; flex-direction: column; gap: 16px;">
        <div style="text-align: center; background: #0c0d12; padding: 12px; border-radius: 8px;">
          <img
            v-if="selectedFile.is_image"
            :src="selectedFile.links?.url || ''"
            style="max-width: 100%; max-height: 400px; object-fit: contain; border-radius: 4px;"
          />
        </div>

        <div style="display: flex; gap: 12px; font-size: 13px; color: #aaa;">
          <span>{{ formatBytes(selectedFile.size) }}</span>
          <span v-if="selectedFile.width && selectedFile.height">{{ selectedFile.width }} × {{ selectedFile.height }}</span>
          <span>{{ selectedFile.mime_type }}</span>
        </div>

        <!-- Read Only Links Panel -->
        <FileLinksPanel :file="selectedFile" :initial-url="selectedFile.links?.url" :hide-temp-links="true" />
      </div>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { SearchOutline } from '@vicons/ionicons5'
import { api } from '../api/client'
import PublicNavBar from '../components/PublicNavBar.vue'
import FileLinksPanel from '../components/FileLinksPanel.vue'

const loading = ref(false)
const files = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(24)
const search = ref('')

const showDetailModal = ref(false)
const selectedFile = ref<any>(null)

const formatBytes = (bytes: number): string => {
  if (!bytes || bytes <= 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

const loadData = async () => {
  loading.value = true
  try {
    const res = await api.get('/guest/gallery', {
      params: {
        page: page.value,
        page_size: pageSize.value,
        q: search.value.trim()
      }
    })
    files.value = res.data.items || []
    total.value = res.data.total || 0
  } catch {
    files.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

const handleSearch = () => {
  page.value = 1
  loadData()
}

const openDetails = (file: any) => {
  selectedFile.value = file
  showDetailModal.value = true
}

onMounted(() => {
  loadData()
})
</script>

<style scoped>
.guest-gallery-wrapper {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  background: #101014;
}

.guest-gallery-container {
  flex: 1;
  padding: 24px 20px;
}

.gallery-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 16px;
}

.gallery-card {
  background: #18181c;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 8px;
  overflow: hidden;
  cursor: pointer;
  transition: all 0.2s ease;
}

.gallery-card:hover {
  transform: translateY(-2px);
  border-color: rgba(99, 226, 183, 0.4);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.4);
}

.gallery-img-wrapper {
  height: 160px;
  background: #0c0d12;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
}

.gallery-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  transition: transform 0.3s ease;
}

.gallery-card:hover .gallery-img {
  transform: scale(1.04);
}

.gallery-placeholder {
  color: #555;
  font-size: 13px;
  font-weight: 500;
}

.gallery-card-info {
  padding: 10px 12px;
}

.gallery-card-name {
  font-size: 13px;
  font-weight: 500;
  color: #eee;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  margin-bottom: 4px;
}

.gallery-card-meta {
  font-size: 11px;
  color: #777;
  display: flex;
  justify-content: space-between;
}
</style>

