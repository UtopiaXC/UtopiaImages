<template>
  <div style="background: rgba(255, 255, 255, 0.02); border-radius: 8px; padding: 14px;">
    <!-- Top toolbar: Image Size Selector & Domain Selector -->
    <div style="display: flex; align-items: center; justify-content: space-between; margin-bottom: 12px; flex-wrap: wrap; gap: 8px;">
      <div style="display: flex; align-items: center; gap: 12px; flex-wrap: wrap;">
        <!-- Domain Selector (Multi-Domain Support) -->
        <div v-if="domainOptions.length > 1" style="display: flex; align-items: center; gap: 6px;">
          <span style="font-size: 13px; color: #aaa;">{{ $t('upload.domainSelect') }}</span>
          <n-select
            v-model:value="selectedDomain"
            :options="domainOptions"
            size="small"
            style="width: 210px;"
          />
        </div>

        <div style="display: flex; align-items: center; gap: 6px;">
          <span style="font-size: 13px; color: #aaa;">{{ $t('upload.imageSize') }}</span>
          <n-select
            v-model:value="selectedSize"
            :options="sizeOptions"
            size="small"
            style="width: 150px;"
          />
        </div>
      </div>
      <div v-if="file.is_image && file.width && file.height" style="font-size: 12px; color: #777;">
        {{ file.width }} × {{ file.height }} px
      </div>
    </div>

    <!-- Format Tabs (Direct, Markdown, HTML, BBCode) -->
    <n-tabs v-model:value="activeTab" type="line" size="small">
      <n-tab-pane name="direct" :tab="$t('upload.directLink')">
        <div style="display: flex; flex-direction: column; gap: 8px; margin-top: 6px;">
          <div v-for="r in visibleFormatRows" :key="r.key" style="display: flex; align-items: center; gap: 8px;">
            <n-tag size="small" :bordered="false" style="width: 110px; justify-content: center;">
              {{ r.label }}
            </n-tag>
            <n-input :value="formatContent(r.url)" size="small" readonly style="flex: 1;" />
            <n-button size="small" type="primary" quaternary @click="copyText(formatContent(r.url))">
              {{ $t('common.copy') }}
            </n-button>
          </div>
        </div>
      </n-tab-pane>

      <n-tab-pane name="markdown" :tab="$t('upload.markdown')">
        <div style="display: flex; flex-direction: column; gap: 8px; margin-top: 6px;">
          <div v-for="r in visibleFormatRows" :key="r.key" style="display: flex; align-items: center; gap: 8px;">
            <n-tag size="small" :bordered="false" style="width: 110px; justify-content: center;">
              {{ r.label }}
            </n-tag>
            <n-input :value="formatContent(r.url)" size="small" readonly style="flex: 1;" />
            <n-button size="small" type="primary" quaternary @click="copyText(formatContent(r.url))">
              {{ $t('common.copy') }}
            </n-button>
          </div>
        </div>
      </n-tab-pane>

      <n-tab-pane name="html" :tab="$t('upload.html')">
        <div style="display: flex; flex-direction: column; gap: 8px; margin-top: 6px;">
          <div v-for="r in visibleFormatRows" :key="r.key" style="display: flex; align-items: center; gap: 8px;">
            <n-tag size="small" :bordered="false" style="width: 110px; justify-content: center;">
              {{ r.label }}
            </n-tag>
            <n-input :value="formatContent(r.url)" size="small" readonly style="flex: 1;" />
            <n-button size="small" type="primary" quaternary @click="copyText(formatContent(r.url))">
              {{ $t('common.copy') }}
            </n-button>
          </div>
        </div>
      </n-tab-pane>

      <n-tab-pane name="bbcode" :tab="$t('upload.bbcode')">
        <div style="display: flex; flex-direction: column; gap: 8px; margin-top: 6px;">
          <div v-for="r in visibleFormatRows" :key="r.key" style="display: flex; align-items: center; gap: 8px;">
            <n-tag size="small" :bordered="false" style="width: 110px; justify-content: center;">
              {{ r.label }}
            </n-tag>
            <n-input :value="formatContent(r.url)" size="small" readonly style="flex: 1;" />
            <n-button size="small" type="primary" quaternary @click="copyText(formatContent(r.url))">
              {{ $t('common.copy') }}
            </n-button>
          </div>
        </div>
      </n-tab-pane>
    </n-tabs>

    <!-- Temporary Link Management Section (Requirement 4) -->
    <div v-if="isWatermarked" style="margin-top: 14px; padding-top: 12px; border-top: 1px solid rgba(255, 255, 255, 0.08);">
      <div style="display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px;">
        <div style="display: flex; align-items: center; gap: 8px;">
          <n-tag size="small" type="warning" :bordered="false" style="width: 110px; justify-content: center;">
            {{ $t('upload.tempLink') }}
          </n-tag>
          <span style="font-size: 12px; color: #888;">
            {{ activeLink ? $t('files.linkExpires') + ': ' + formatDate(activeLink.expires_at) : $t('upload.tempLinkTip') }}
          </span>
        </div>
        <div style="display: flex; gap: 6px;">
          <n-button size="tiny" quaternary @click="showHistoryModal = true">
            {{ $t('files.tempLinkHistory') }} ({{ tempLinks.length }})
          </n-button>
        </div>
      </div>

      <!-- Active link input or creation controls -->
      <div v-if="activeLink" style="display: flex; align-items: center; gap: 8px;">
        <n-input :value="formatContent(applySelectedDomain(activeLink.url))" size="small" readonly style="flex: 1;" />
        <n-button size="small" type="primary" quaternary @click="copyText(formatContent(applySelectedDomain(activeLink.url)))">
          {{ $t('common.copy') }}
        </n-button>
        <n-popconfirm @positive-click="revokeSingleLink(activeLink.id)">
          <template #trigger>
            <n-button size="small" quaternary type="error">
              {{ $t('files.revokeLink') }}
            </n-button>
          </template>
          {{ $t('files.confirmRevokeOne') }}
        </n-popconfirm>
      </div>

      <div v-else style="display: flex; align-items: center; gap: 8px;">
        <span style="font-size: 12px; color: #888;">{{ $t('files.duration') }}:</span>
        <n-select
          v-model:value="selectedDurationHours"
          :options="durationOptions"
          size="small"
          style="width: 130px;"
        />
        <n-button size="small" type="warning" :loading="generatingTemp" @click="handleGenerateTempLink">
          {{ $t('upload.generateTempLink') }}
        </n-button>
      </div>
    </div>

    <!-- Temporary Link History Modal -->
    <n-modal
      v-model:show="showHistoryModal"
      preset="card"
      :title="$t('files.tempLinkHistory')"
      style="width: 700px; max-width: 95vw; border-radius: 12px;"
    >
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px;">
        <div style="display: flex; align-items: center; gap: 8px;">
          <span style="font-size: 12px; color: #aaa;">{{ $t('files.duration') }}:</span>
          <n-select
            v-model:value="selectedDurationHours"
            :options="durationOptions"
            size="small"
            style="width: 130px;"
          />
          <n-button size="small" type="primary" :loading="generatingTemp" @click="handleGenerateTempLink">
            {{ $t('upload.generateTempLink') }}
          </n-button>
        </div>
        <n-popconfirm @positive-click="revokeAllLinks">
          <template #trigger>
            <n-button size="small" quaternary type="error" :disabled="activeLinksCount === 0">
              {{ $t('files.revokeLinks') }}
            </n-button>
          </template>
          {{ $t('files.confirmRevoke') }}
        </n-popconfirm>
      </div>

      <n-table size="small" :bordered="false" :single-line="false">
        <thead>
          <tr>
            <th style="width: 90px;">{{ $t('common.status') }}</th>
            <th>{{ $t('files.linkExpires') }}</th>
            <th>{{ $t('files.linkCreated') }}</th>
            <th style="text-align: right;">{{ $t('common.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in tempLinks" :key="item.id">
            <td>
              <n-tag v-if="item.is_active" type="success" size="tiny">
                {{ $t('files.statusActive') }}
              </n-tag>
              <n-tag v-else-if="item.is_revoked" type="error" size="tiny">
                {{ $t('files.statusRevoked') }}
              </n-tag>
              <n-tag v-else size="tiny">
                {{ $t('files.statusExpired') }}
              </n-tag>
            </td>
            <td style="font-size: 12px;">{{ formatDate(item.expires_at) }}</td>
            <td style="font-size: 12px;">{{ formatDate(item.created_at) }}</td>
            <td style="text-align: right;">
              <div style="display: flex; gap: 4px; justify-content: flex-end;">
                <n-button size="tiny" quaternary type="primary" @click="copyText(applySelectedDomain(item.url))">
                  {{ $t('common.copy') }}
                </n-button>
                <n-popconfirm v-if="item.is_active" @positive-click="revokeSingleLink(item.id)">
                  <template #trigger>
                    <n-button size="tiny" quaternary type="error">
                      {{ $t('files.revokeLink') }}
                    </n-button>
                  </template>
                  {{ $t('files.confirmRevokeOne') }}
                </n-popconfirm>
              </div>
            </td>
          </tr>
          <tr v-if="tempLinks.length === 0">
            <td colspan="4" style="text-align: center; color: #777; padding: 20px 0;">
              {{ $t('watermarks.empty') }}
            </td>
          </tr>
        </tbody>
      </n-table>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'

const props = defineProps<{
  file: any
  initialUrl?: string
}>()

const message = useMessage()
const { t } = useI18n()

const activeTab = ref('direct')
const selectedSize = ref('original')
const selectedDurationHours = ref(24)
const generatingTemp = ref(false)
const showHistoryModal = ref(false)
const tempLinks = ref<any[]>([])

const configuredDomains = ref<string[]>([])
const selectedDomain = ref<string>(window.location.origin)

const loadSiteDomains = async () => {
  try {
    const res = await api.get('/site')
    if (res.data.base_urls && Array.isArray(res.data.base_urls) && res.data.base_urls.length > 0) {
      configuredDomains.value = res.data.base_urls
    } else if (res.data.base_url) {
      configuredDomains.value = [res.data.base_url]
    }
  } catch {}
}

const domainOptions = computed(() => {
  const set = new Set<string>()
  const isOnline = window.location.origin && !window.location.origin.includes('localhost') && !window.location.origin.includes('127.0.0.1')
  if (window.location.origin) {
    set.add(window.location.origin)
  }
  const addDomain = (d: string) => {
    if (!d) return
    const clean = d.replace(/\/+$/, '')
    if (isOnline && (clean.includes('localhost') || clean.includes('127.0.0.1'))) {
      return // skip localhost on online environments
    }
    set.add(clean)
  }
  if (props.file?.base_urls && Array.isArray(props.file.base_urls)) {
    props.file.base_urls.forEach(addDomain)
  }
  configuredDomains.value.forEach(addDomain)
  return Array.from(set).map((d) => ({
    label: d,
    value: d
  }))
})

const applySelectedDomain = (url: string) => {
  if (!url) return ''
  const domain = selectedDomain.value || window.location.origin
  if (url.startsWith('/')) {
    return `${domain}${url}`
  }
  return url.replace(/^https?:\/\/[^\/]+/, domain)
}

const durationOptions = computed(() => [
  { label: t('files.hour1'), value: 1 },
  { label: t('files.hour24'), value: 24 },
  { label: t('files.day7'), value: 168 },
  { label: '30 ' + t('common.days'), value: 720 },
])

const sizeOptions = computed(() => [
  { label: t('upload.sizeOriginal'), value: 'original' },
  { label: t('upload.sizeLarge'), value: 'large' },
  { label: t('upload.sizeMedium'), value: 'medium' },
  { label: t('upload.sizeSmall'), value: 'small' },
  { label: t('upload.sizeThumb'), value: 'thumb' }
])

const isWatermarked = computed(() => {
  return props.file.watermark_mode === 'on' || (props.file.watermark_mode !== 'off' && props.file.is_image)
})

const activeLink = computed(() => {
  return tempLinks.value.find((l) => l.is_active)
})

const activeLinksCount = computed(() => {
  return tempLinks.value.filter((l) => l.is_active).length
})

const loadTempLinks = async () => {
  if (!props.file?.id) return
  try {
    const res = await api.get(`/files/${props.file.id}/temp-links`)
    tempLinks.value = res.data.links || []
  } catch (e) {
    // Ignore if not permitted
  }
}

onMounted(() => {
  loadTempLinks()
  loadSiteDomains()
})

watch(() => props.file?.id, () => {
  loadTempLinks()
})

const getRawUrl = () => {
  if (props.initialUrl) {
    return applySelectedDomain(props.initialUrl)
  }
  const domain = selectedDomain.value || window.location.origin
  const path = props.file?.public_path ? props.file.public_path.replace(/^\/+/, '') : ''
  return `${domain}/${path}`
}

const originalExt = computed(() => {
  const parts = props.file.public_path.split('.')
  if (parts.length > 1) {
    return parts.pop()?.toLowerCase() || ''
  }
  return ''
})

const sizeSuffix = computed(() => {
  if (selectedSize.value === 'large') return '?size=large'
  if (selectedSize.value === 'medium') return '?size=medium'
  if (selectedSize.value === 'small') return '?size=small'
  if (selectedSize.value === 'thumb') return '?size=thumb'
  return ''
})

const visibleFormatRows = computed(() => {
  const rawUrl = getRawUrl()
  const baseNoExt = rawUrl.replace(/\.[^.]+$/, '')
  const ext = originalExt.value
  const isImage = props.file.is_image
  const suffix = sizeSuffix.value

  const rows: Array<{ key: string; label: string; url: string }> = [
    { key: 'raw', label: t('upload.fmtOriginal'), url: `${rawUrl}${suffix}` }
  ]

  if (!isImage) {
    return rows
  }

  // PNG
  if (ext !== 'png') {
    rows.push({
      key: 'png',
      label: t('upload.fmtPng'),
      url: `${baseNoExt}.png${suffix}`
    })
  }

  // JPG
  if (ext !== 'jpg' && ext !== 'jpeg') {
    rows.push({
      key: 'jpg',
      label: t('upload.fmtJpg'),
      url: `${baseNoExt}.jpg${suffix}`
    })
  }

  // WebP
  if (ext !== 'webp') {
    rows.push({
      key: 'webp',
      label: t('upload.fmtWebp'),
      url: `${baseNoExt}.webp${suffix}`
    })
  }

  // HEIF
  if (ext !== 'heif' && ext !== 'heic') {
    rows.push({
      key: 'heif',
      label: t('upload.fmtHeif'),
      url: `${baseNoExt}.heif${suffix}`
    })
  }

  return rows
})

const formatContent = (url: string) => {
  const name = props.file.display_name || 'image'
  switch (activeTab.value) {
    case 'markdown': {
      const safeMd = name.replace(/\[/g, '\\[').replace(/\]/g, '\\]')
      return `![${safeMd}](${url})`
    }
    case 'html': {
      const safeHtml = name
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;')
      return `<img src="${url}" alt="${safeHtml}" />`
    }
    case 'bbcode':
      return `[img]${url}[/img]`
    default:
      return url
  }
}

const copyText = (text: string) => {
  navigator.clipboard.writeText(text)
  message.success(t('common.copied'))
}

const formatDate = (dateStr: string) => {
  if (!dateStr) return '-'
  const d = new Date(dateStr)
  return d.toLocaleString()
}

const handleGenerateTempLink = async () => {
  generatingTemp.value = true
  try {
    await api.post(`/files/${props.file.id}/temp-links`, {
      expires_hours: selectedDurationHours.value
    })
    await loadTempLinks()
    message.success(t('upload.tempLinkGenerated'))
  } catch (err: any) {
    message.error(err.response?.data?.error || t('upload.tempLinkFailed'))
  } finally {
    generatingTemp.value = false
  }
}

const revokeSingleLink = async (linkID: number) => {
  try {
    await api.delete(`/files/${props.file.id}/temp-links/${linkID}`)
    await loadTempLinks()
    message.success(t('files.revokeSuccess'))
  } catch (err: any) {
    message.error(err.response?.data?.error || t('common.failed'))
  }
}

const revokeAllLinks = async () => {
  try {
    await api.delete(`/files/${props.file.id}/temp-links`)
    await loadTempLinks()
    message.success(t('files.revokeSuccess'))
  } catch (err: any) {
    message.error(err.response?.data?.error || t('common.failed'))
  }
}
</script>
