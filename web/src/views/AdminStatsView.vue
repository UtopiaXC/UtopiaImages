<template>
  <div>
    <div style="font-size: 18px; font-weight: bold; margin-bottom: 20px;">{{ $t('adminStats.title') }}</div>
    <n-grid :cols="3" :x-gap="16" :y-gap="16">
      <n-grid-item>
        <n-card style="border-radius: 12px; text-align: center;">
          <n-statistic :label="$t('adminStats.userCount')" :value="stats.users_count" />
        </n-card>
      </n-grid-item>
      <n-grid-item>
        <n-card style="border-radius: 12px; text-align: center;">
          <n-statistic :label="$t('adminStats.fileCount')" :value="stats.files_count" />
        </n-card>
      </n-grid-item>
      <n-grid-item>
        <n-card style="border-radius: 12px; text-align: center;">
          <n-statistic :label="$t('adminStats.storageUsed')" :value="formatBytes(stats.total_bytes)" />
        </n-card>
      </n-grid-item>
    </n-grid>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api } from '../api/client'

const stats = ref({
  users_count: 0,
  files_count: 0,
  total_bytes: 0
})

const formatBytes = (bytes: number) => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

onMounted(async () => {
  try {
    const res = await api.get('/admin/stats')
    stats.value = res.data
  } catch {}
})
</script>
