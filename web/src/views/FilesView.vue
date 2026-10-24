<template>
  <div>
    <!-- Top toolbar -->
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px;">
      <n-input
        v-model:value="search"
        :placeholder="$t('common.search') + '...'"
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
          批量删除 ({{ selectedIds.length }})
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
            <div v-else style="font-size: 36px; color: #666;">
              📄
            </div>
            <div style="position: absolute; top: 8px; left: 8px;">
              <n-checkbox
                :checked="selectedIds.includes(file.id)"
                @update:checked="(val: boolean) => toggleSelect(file.id, val)"
              />
            </div>
          </div>

          <div style="padding: 10px;">
            <div style="font-weight: 500; font-size: 13px; text-overflow: ellipsis; overflow: hidden; white-space: nowrap;">
              {{ file.display_name }}
            </div>
            <div style="font-size: 12px; color: #777; margin-top: 4px; display: flex; justify-content: space-between;">
              <span>{{ formatBytes(file.size) }}</span>
              <span>{{ formatDate(file.created_at) }}</span>
            </div>
            
            <div style="margin-top: 10px; display: flex; gap: 8px; justify-content: flex-end;">
              <n-button size="tiny" quaternary @click="copyLink(baseURL + '/' + file.public_path)">
                复制
              </n-button>
              <n-button size="tiny" quaternary @click="openRename(file)">
                改名
              </n-button>
              <n-popconfirm @positive-click="deleteFile(file.id)">
                <template #trigger>
                  <n-button size="tiny" quaternary type="error">删除</n-button>
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
    <n-modal v-model:show="showRenameModal" preset="dialog" title="修改图片显示名称">
      <n-input v-model:value="renameName" placeholder="输入新的显示名称" />
      <template #action>
        <n-button @click="showRenameModal = false">取消</n-button>
        <n-button type="primary" @click="confirmRename">保存</n-button>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useMessage } from 'naive-ui'
import { api } from '../api/client'

const message = useMessage()
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
  message.success('链接已复制')
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
    message.success('修改成功')
    showRenameModal.value = false
    loadFiles()
  } catch (err: any) {
    message.error(err.response?.data?.error || '修改失败')
  }
}

const deleteFile = async (id: number) => {
  try {
    await api.delete(`/files/${id}`)
    message.success('已删除')
    loadFiles()
  } catch (err: any) {
    message.error(err.response?.data?.error || '删除失败')
  }
}

const handleBatchDelete = async () => {
  if (selectedIds.value.length === 0) return
  try {
    await api.post('/files/batch-delete', { ids: selectedIds.value })
    message.success(`成功删除 ${selectedIds.value.length} 个文件`)
    selectedIds.value = []
    loadFiles()
  } catch (err: any) {
    message.error('批量删除失败')
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
