<template>
  <div class="square-wrapper">
    <PublicNavBar />

    <main class="square-container">
      <div style="max-width: 1400px; margin: 0 auto; width: 100%;">
        <!-- Header -->
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 24px; flex-wrap: wrap; gap: 12px;">
          <div>
            <div style="font-size: 22px; font-weight: bold; letter-spacing: 0.5px;">{{ $t('square.title') }}</div>
            <div style="font-size: 13px; color: #888; margin-top: 4px;">{{ $t('square.photoWall') }} · {{ $t('square.wallCount', { count: total }) }}</div>
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

        <!-- Masonry Photo Wall -->
        <n-spin :show="loading">
          <div v-if="files.length === 0 && !loading" style="text-align: center; padding: 80px 0; color: #777;">
            <p>{{ $t('square.empty') }}</p>
          </div>

          <div class="masonry-wall">
            <div
              v-for="f in files"
              :key="f.id"
              class="masonry-item"
              @click="openDetails(f)"
            >
              <div class="masonry-img-container">
                <img
                  :src="f.links?.thumb_url || f.links?.url || ''"
                  :alt="f.display_name"
                  loading="lazy"
                  class="masonry-img"
                />
                <div class="masonry-overlay">
                  <div class="masonry-title">{{ f.display_name }}</div>
                  <div class="masonry-meta">
                    <span>{{ formatBytes(f.size) }}</span>
                    <span v-if="f.width && f.height">{{ f.width }}×{{ f.height }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Pagination -->
          <div v-if="total > pageSize" style="display: flex; justify-content: center; margin-top: 32px;">
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

    <!-- Photo Details & Copy Links Modal -->
    <n-modal
      v-model:show="showDetailModal"
      preset="card"
      :title="selectedFile?.display_name || $t('files.detailsTitle')"
      style="width: 720px; max-width: 95vw; border-radius: 12px;"
    >
      <div v-if="selectedFile" style="display: flex; flex-direction: column; gap: 16px;">
        <div style="text-align: center; background: #0c0d12; padding: 12px; border-radius: 8px;">
          <img
            :src="selectedFile.links?.url || ''"
            style="max-width: 100%; max-height: 480px; object-fit: contain; border-radius: 4px;"
          />
        </div>

        <div style="display: flex; gap: 16px; font-size: 13px; color: #aaa; flex-wrap: wrap;">
          <span>{{ formatBytes(selectedFile.size) }}</span>
          <span v-if="selectedFile.width && selectedFile.height">{{ selectedFile.width }} × {{ selectedFile.height }}</span>
          <span>{{ selectedFile.mime_type }}</span>
        </div>

        <!-- Read-Only Link Copying Panel -->
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
const pageSize = ref(30)
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
    const res = await api.get('/square/files', {
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
.square-wrapper {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  background: #101014;
}

.square-container {
  flex: 1;
  padding: 24px 20px;
}

.masonry-wall {
  column-count: 4;
  column-gap: 16px;
}

@media (max-width: 1200px) {
  .masonry-wall {
    column-count: 3;
  }
}

@media (max-width: 768px) {
  .masonry-wall {
    column-count: 2;
    column-gap: 10px;
  }
}

@media (max-width: 480px) {
  .masonry-wall {
    column-count: 1;
  }
}

.masonry-item {
  break-inside: avoid;
  margin-bottom: 16px;
  cursor: pointer;
}

.masonry-img-container {
  position: relative;
  border-radius: 8px;
  overflow: hidden;
  background: #18181c;
  border: 1px solid rgba(255, 255, 255, 0.06);
  transition: all 0.25s ease;
}

.masonry-item:hover .masonry-img-container {
  transform: translateY(-2px);
  border-color: rgba(99, 226, 183, 0.4);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.5);
}

.masonry-img {
  width: 100%;
  display: block;
  object-fit: cover;
  transition: transform 0.3s ease;
}

.masonry-item:hover .masonry-img {
  transform: scale(1.02);
}

.masonry-overlay {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  padding: 28px 12px 10px;
  background: linear-gradient(to top, rgba(0, 0, 0, 0.85) 0%, rgba(0, 0, 0, 0.4) 60%, transparent 100%);
  color: #fff;
  opacity: 0;
  transition: opacity 0.2s ease;
}

.masonry-item:hover .masonry-overlay {
  opacity: 1;
}

.masonry-title {
  font-size: 13px;
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.masonry-meta {
  font-size: 11px;
  color: #ccc;
  display: flex;
  justify-content: space-between;
  margin-top: 2px;
}
</style>

