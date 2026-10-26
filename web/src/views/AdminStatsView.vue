<template>
  <div style="max-width: 1200px; margin: 0 auto; display: flex; flex-direction: column; gap: 20px;">
    <!-- Header -->
    <div style="display: flex; justify-content: space-between; align-items: center;">
      <div>
        <h2 style="margin: 0; font-size: 20px; font-weight: bold;">{{ $t('adminStats.title') }}</h2>
        <p style="margin: 4px 0 0; color: #888; font-size: 13px;">{{ $t('adminStats.systemOverview') }}</p>
      </div>
      <n-button secondary size="small" :loading="loading" @click="fetchStats">
        <template #icon>
          <n-icon>
            <RefreshOutline />
          </n-icon>
        </template>
        {{ $t('login.captchaRefresh') }}
      </n-button>
    </div>

    <!-- 1. Top Summary Counters -->
    <n-grid :cols="5" :x-gap="16" :y-gap="16" responsive="screen" :item-responsive="true">
      <n-grid-item span="5 m:2 l:1">
        <n-card style="border-radius: 12px; background: rgba(255,255,255,0.02); border: 1px solid rgba(255,255,255,0.06);">
          <n-statistic :label="$t('adminStats.userCount')" :value="stats.users_count">
            <template #prefix>
              <n-icon size="18" style="color: #63e2b7; margin-right: 4px;">
                <PeopleOutline />
              </n-icon>
            </template>
          </n-statistic>
        </n-card>
      </n-grid-item>

      <n-grid-item span="5 m:2 l:1">
        <n-card style="border-radius: 12px; background: rgba(255,255,255,0.02); border: 1px solid rgba(255,255,255,0.06);">
          <n-statistic :label="$t('adminStats.fileCount')" :value="stats.files_count">
            <template #prefix>
              <n-icon size="18" style="color: #70c0e8; margin-right: 4px;">
                <ImagesOutline />
              </n-icon>
            </template>
          </n-statistic>
        </n-card>
      </n-grid-item>

      <n-grid-item span="5 m:2 l:1">
        <n-card style="border-radius: 12px; background: rgba(255,255,255,0.02); border: 1px solid rgba(255,255,255,0.06);">
          <n-statistic :label="$t('adminStats.storageUsed')" :value="formatBytes(stats.total_bytes)">
            <template #prefix>
              <n-icon size="18" style="color: #f2c97d; margin-right: 4px;">
                <CloudDoneOutline />
              </n-icon>
            </template>
          </n-statistic>
        </n-card>
      </n-grid-item>

      <n-grid-item span="5 m:2 l:1">
        <n-card style="border-radius: 12px; background: rgba(255,255,255,0.02); border: 1px solid rgba(255,255,255,0.06);">
          <n-statistic :label="$t('adminStats.roleCount')" :value="stats.roles_count">
            <template #prefix>
              <n-icon size="18" style="color: #e88080; margin-right: 4px;">
                <ShieldCheckmarkOutline />
              </n-icon>
            </template>
          </n-statistic>
        </n-card>
      </n-grid-item>

      <n-grid-item span="5 m:2 l:1">
        <n-card style="border-radius: 12px; background: rgba(255,255,255,0.02); border: 1px solid rgba(255,255,255,0.06);">
          <n-statistic :label="$t('adminStats.policyCount')" :value="stats.policies_count">
            <template #prefix>
              <n-icon size="18" style="color: #a0cfff; margin-right: 4px;">
                <ServerOutline />
              </n-icon>
            </template>
          </n-statistic>
        </n-card>
      </n-grid-item>
    </n-grid>

    <!-- 2. Activity Trends & Distribution -->
    <n-grid :cols="3" :x-gap="16" :y-gap="16" responsive="screen">
      <!-- 14-Day Activity Chart -->
      <n-grid-item span="3 m:2">
        <n-card
          :title="$t('adminStats.uploadTrend')"
          style="border-radius: 12px; height: 100%;"
        >
          <template #header-extra>
            <span style="font-size: 12px; color: #888;">{{ $t('adminStats.past14Days') }}</span>
          </template>

          <div style="height: 180px; display: flex; align-items: flex-end; gap: 8px; padding-top: 20px; border-bottom: 1px solid rgba(255,255,255,0.08);">
            <div
              v-for="d in stats.daily_trends"
              :key="d.date"
              style="flex: 1; display: flex; flex-direction: column; align-items: center; height: 100%; justify-content: flex-end;"
            >
              <n-tooltip trigger="hover">
                <template #trigger>
                  <div
                    style="width: 100%; max-width: 24px; border-radius: 4px 4px 0 0; background: linear-gradient(180deg, #63e2b7 0%, rgba(99, 226, 183, 0.2) 100%); transition: height 0.3s;"
                    :style="{ height: calcBarHeight(d.files_count) + '%' }"
                  />
                </template>
                <div>
                  <div style="font-weight: bold; margin-bottom: 2px;">{{ d.date }}</div>
                  <div>{{ d.files_count }} {{ $t('adminStats.fileCount') }}</div>
                  <div>{{ formatBytes(d.bytes_count) }}</div>
                </div>
              </n-tooltip>
              <div style="font-size: 10px; color: #666; margin-top: 6px; white-space: nowrap; transform: scale(0.85);">
                {{ d.date.slice(5) }}
              </div>
            </div>
          </div>
          <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 10px; font-size: 12px; color: #888;">
            <span>14 {{ $t('common.days') }}</span>
            <span style="display: flex; align-items: center; gap: 6px;">
              <span style="display: inline-block; width: 10px; height: 10px; border-radius: 2px; background: #63e2b7;"></span>
              {{ $t('adminStats.fileCount') }}
            </span>
          </div>
        </n-card>
      </n-grid-item>

      <!-- Format Distribution -->
      <n-grid-item span="3 m:1">
        <n-card
          :title="$t('adminStats.formatDistribution')"
          style="border-radius: 12px; height: 100%;"
        >
          <div v-if="stats.mime_distribution && stats.mime_distribution.length > 0" style="display: flex; flex-direction: column; gap: 12px;">
            <div v-for="m in stats.mime_distribution" :key="m.mime">
              <div style="display: flex; justify-content: space-between; font-size: 12px; margin-bottom: 4px;">
                <span style="color: #eee; font-family: monospace;">{{ m.mime || 'unknown' }}</span>
                <span style="color: #888;">{{ m.count }} ({{ formatBytes(m.total_size) }})</span>
              </div>
              <n-progress
                type="line"
                :percentage="calcMimePercentage(m.count)"
                :show-indicator="false"
                status="info"
                style="height: 6px;"
              />
            </div>
          </div>
          <div v-else style="color: #666; font-size: 13px; text-align: center; padding: 40px 0;">
            -
          </div>
        </n-card>
      </n-grid-item>
    </n-grid>

    <!-- 3. Server Runtime Status -->
    <n-card v-if="stats.system_info" :title="$t('adminStats.systemRuntime')" style="border-radius: 12px;">
      <n-grid :cols="4" :x-gap="16" :y-gap="16" responsive="screen">
        <n-grid-item span="4 m:1">
          <div style="color: #888; font-size: 12px; margin-bottom: 4px;">{{ $t('adminStats.goVersion') }}</div>
          <div style="font-weight: 500; font-family: monospace;">{{ stats.system_info.go_version }}</div>
        </n-grid-item>
        <n-grid-item span="4 m:1">
          <div style="color: #888; font-size: 12px; margin-bottom: 4px;">{{ $t('adminStats.osArch') }}</div>
          <div style="font-weight: 500; text-transform: uppercase;">{{ stats.system_info.os }} / {{ stats.system_info.arch }}</div>
        </n-grid-item>
        <n-grid-item span="4 m:1">
          <div style="color: #888; font-size: 12px; margin-bottom: 4px;">{{ $t('adminStats.serverUptime') }}</div>
          <div style="font-weight: 500; color: #63e2b7;">{{ formatUptime(stats.system_info.server_uptime_seconds) }}</div>
        </n-grid-item>
        <n-grid-item span="4 m:1">
          <div style="color: #888; font-size: 12px; margin-bottom: 4px;">{{ $t('adminStats.goroutines') }}</div>
          <div style="font-weight: 500;">{{ stats.system_info.goroutines }} · GC: {{ stats.system_info.num_gc }}</div>
        </n-grid-item>
        <n-grid-item span="4 m:1">
          <div style="color: #888; font-size: 12px; margin-bottom: 4px;">{{ $t('adminStats.memoryAlloc') }}</div>
          <div style="font-weight: 500;">{{ formatBytes(stats.system_info.alloc_bytes) }}</div>
        </n-grid-item>
        <n-grid-item span="4 m:1">
          <div style="color: #888; font-size: 12px; margin-bottom: 4px;">{{ $t('adminStats.memorySys') }}</div>
          <div style="font-weight: 500;">{{ formatBytes(stats.system_info.sys_bytes) }}</div>
        </n-grid-item>
        <n-grid-item span="4 m:1">
          <div style="color: #888; font-size: 12px; margin-bottom: 4px;">{{ $t('adminSettings.logLevel') }}</div>
          <div>
            <n-tag size="small" type="info">{{ stats.system_info.log_level }}</n-tag>
          </div>
        </n-grid-item>
        <n-grid-item span="4 m:1">
          <div style="color: #888; font-size: 12px; margin-bottom: 4px;">{{ $t('adminSettings.logPermanentLevel') }}</div>
          <div>
            <n-tag size="small" type="error">{{ stats.system_info.log_permanent_level }}</n-tag>
          </div>
        </n-grid-item>
      </n-grid>
    </n-card>

    <!-- 4. Tables: Recent Files & Recent Users -->
    <n-grid :cols="2" :x-gap="16" :y-gap="16" responsive="screen">
      <!-- Recent Files Table -->
      <n-grid-item span="2 m:1">
        <n-card :title="$t('adminStats.recentFiles')" style="border-radius: 12px;">
          <div class="table-responsive">
            <n-table size="small" :single-line="false" :bordered="false">
              <thead>
                <tr>
                  <th>{{ $t('adminStats.fileName') }}</th>
                  <th>{{ $t('adminStats.fileSize') }}</th>
                  <th>{{ $t('adminStats.uploader') }}</th>
                  <th>{{ $t('common.createdAt') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="f in stats.recent_files" :key="f.id">
                  <td style="max-width: 140px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
                    <strong>{{ f.display_name }}</strong>
                  </td>
                  <td>{{ formatBytes(f.size) }}</td>
                  <td>
                    <n-tag size="tiny" type="default">{{ f.uploader }}</n-tag>
                  </td>
                  <td style="font-size: 11px; color: #888;">{{ formatDate(f.created_at) }}</td>
                </tr>
                <tr v-if="!stats.recent_files || stats.recent_files.length === 0">
                  <td colspan="4" style="text-align: center; color: #666; padding: 20px;">-</td>
                </tr>
              </tbody>
            </n-table>
          </div>
        </n-card>
      </n-grid-item>

      <!-- Recent Users Table -->
      <n-grid-item span="2 m:1">
        <n-card :title="$t('adminStats.recentUsers')" style="border-radius: 12px;">
          <div class="table-responsive">
            <n-table size="small" :single-line="false" :bordered="false">
              <thead>
                <tr>
                  <th>{{ $t('adminStats.user') }}</th>
                  <th>{{ $t('adminStats.role') }}</th>
                  <th>{{ $t('common.status') }}</th>
                  <th>{{ $t('common.createdAt') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="u in stats.recent_users" :key="u.id">
                  <td>
                    <strong>{{ u.nickname || u.username }}</strong>
                    <div v-if="u.nickname" style="font-size: 11px; color: #666;">@{{ u.username }}</div>
                  </td>
                  <td>
                    <n-tag size="tiny" type="info">{{ u.role?.display_name || u.role?.name || 'User' }}</n-tag>
                  </td>
                  <td>
                    <n-tag v-if="u.status === 'banned'" size="tiny" type="error">{{ $t('adminUsers.statusBanned') }}</n-tag>
                    <n-tag v-else-if="u.status === 'frozen'" size="tiny" type="warning">{{ $t('adminUsers.statusFrozen') }}</n-tag>
                    <n-tag v-else size="tiny" type="success">{{ $t('adminUsers.statusActive') }}</n-tag>
                  </td>
                  <td style="font-size: 11px; color: #888;">{{ formatDate(u.created_at) }}</td>
                </tr>
                <tr v-if="!stats.recent_users || stats.recent_users.length === 0">
                  <td colspan="4" style="text-align: center; color: #666; padding: 20px;">-</td>
                </tr>
              </tbody>
            </n-table>
          </div>
        </n-card>
      </n-grid-item>
    </n-grid>

    <!-- 5. Real-time Logs & Error Box -->
    <n-grid :cols="2" :x-gap="16" :y-gap="16" responsive="screen">
      <!-- Recent General Logs -->
      <n-grid-item span="2 m:1">
        <n-card :title="$t('adminStats.recentLogs')" style="border-radius: 12px;">
          <div
            style="background: #0d0d12; border: 1px solid rgba(255,255,255,0.08); border-radius: 8px; padding: 12px; height: 260px; overflow-y: auto; font-family: Consolas, Monaco, monospace; font-size: 11px; line-height: 1.5; color: #ccc;"
          >
            <div v-for="(line, idx) in stats.recent_logs" :key="idx" style="margin-bottom: 4px; word-break: break-all;">
              <span style="color: #666; user-select: none; margin-right: 8px;">{{ String(idx + 1).padStart(2, '0') }}</span>
              <span :style="getLogLineStyle(line)">{{ line }}</span>
            </div>
            <div v-if="!stats.recent_logs || stats.recent_logs.length === 0" style="color: #555; text-align: center; padding-top: 100px;">
              {{ $t('adminStats.noLogs') }}
            </div>
          </div>
        </n-card>
      </n-grid-item>

      <!-- Recent Error Logs -->
      <n-grid-item span="2 m:1">
        <n-card :title="$t('adminStats.recentErrors')" style="border-radius: 12px;">
          <div
            style="background: #0d0d12; border: 1px solid rgba(255,255,255,0.08); border-radius: 8px; padding: 12px; height: 260px; overflow-y: auto; font-family: Consolas, Monaco, monospace; font-size: 11px; line-height: 1.5; color: #ff7875;"
          >
            <div v-for="(errLine, idx) in stats.recent_errors" :key="idx" style="margin-bottom: 4px; word-break: break-all;">
              <span style="color: #8c2a2a; user-select: none; margin-right: 8px;">{{ String(idx + 1).padStart(2, '0') }}</span>
              <span>{{ errLine }}</span>
            </div>
            <div v-if="!stats.recent_errors || stats.recent_errors.length === 0" style="color: #52c41a; text-align: center; padding-top: 100px;">
              {{ $t('adminStats.noErrors') }}
            </div>
          </div>
        </n-card>
      </n-grid-item>
    </n-grid>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import {
  PeopleOutline,
  ImagesOutline,
  CloudDoneOutline,
  ShieldCheckmarkOutline,
  ServerOutline,
  RefreshOutline
} from '@vicons/ionicons5'
import { api } from '../api/client'

const loading = ref(false)

const stats = ref<any>({
  users_count: 0,
  files_count: 0,
  total_bytes: 0,
  roles_count: 0,
  policies_count: 0,
  daily_trends: [],
  mime_distribution: [],
  recent_files: [],
  recent_users: [],
  system_info: null,
  recent_logs: [],
  recent_errors: []
})

const fetchStats = async () => {
  loading.value = true
  try {
    const res = await api.get('/admin/stats')
    stats.value = res.data
  } catch {}
  finally {
    loading.value = false
  }
}

const calcBarHeight = (count: number) => {
  if (!stats.value.daily_trends || stats.value.daily_trends.length === 0) return 5
  const max = Math.max(...stats.value.daily_trends.map((d: any) => d.files_count), 1)
  return Math.max(8, Math.round((count / max) * 100))
}

const calcMimePercentage = (count: number) => {
  if (!stats.value.files_count || stats.value.files_count === 0) return 0
  return Math.min(100, Math.round((count / stats.value.files_count) * 100))
}

const formatBytes = (bytes: number) => {
  if (!bytes || bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

const formatDate = (isoStr: string) => {
  if (!isoStr) return ''
  const d = new Date(isoStr)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

const formatUptime = (seconds: number) => {
  if (!seconds) return '0m'
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (d > 0) return `${d}d ${h}h ${m}m`
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

const getLogLineStyle = (line: string) => {
  if (line.includes('[ERROR]')) return { color: '#ff7875' }
  if (line.includes('[WARN]')) return { color: '#fadb14' }
  if (line.includes('[INFO]')) return { color: '#70c0e8' }
  return { color: '#aaa' }
}

onMounted(fetchStats)
</script>
