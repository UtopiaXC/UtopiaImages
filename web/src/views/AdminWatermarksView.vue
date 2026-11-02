<template>
  <div>
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 24px; flex-wrap: wrap; gap: 12px;">
      <div>
        <h2 style="margin: 0; font-size: 20px;">{{ $t('adminWatermarks.title') }}</h2>
        <p style="margin: 4px 0 0; color: #888; font-size: 13px;">
          {{ $t('adminWatermarks.subtitle') }}
        </p>
      </div>
      <div>
        <n-button type="primary" @click="openCreateModal">
          {{ $t('adminWatermarks.createSystem') }}
        </n-button>
      </div>
    </div>

    <!-- Filter Bar -->
    <n-card size="small" style="border-radius: 8px; margin-bottom: 16px;">
      <div style="display: flex; align-items: center; gap: 12px; flex-wrap: wrap;">
        <n-input
          v-model:value="searchQuery"
          :placeholder="$t('adminWatermarks.searchPlaceholder')"
          clearable
          style="width: 240px;"
        />
        <n-select
          v-model:value="filterType"
          :options="typeFilterOptions"
          style="width: 150px;"
        />
        <div style="margin-left: auto; font-size: 13px; color: #888;">
          {{ $t('adminWatermarks.totalCount', { count: filteredWatermarks.length }) }}
        </div>
      </div>
    </n-card>

    <!-- Watermarks Table -->
    <n-card style="border-radius: 8px;">
      <n-spin :show="loading">
        <n-table :bordered="false" :single-line="false" size="small">
          <thead>
            <tr>
              <th style="width: 60px;">ID</th>
              <th style="min-width: 140px;">{{ $t('watermarks.name') }}</th>
              <th style="width: 110px;">{{ $t('adminWatermarks.type') }}</th>
              <th style="min-width: 120px;">{{ $t('adminWatermarks.owner') }}</th>
              <th style="min-width: 200px;">{{ $t('watermarks.content') }}</th>
              <th style="width: 150px;">{{ $t('common.updatedAt') }}</th>
              <th style="width: 200px; text-align: right;">{{ $t('common.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in filteredWatermarks" :key="item.id">
              <td>{{ item.id }}</td>
              <td style="font-weight: 500;">{{ item.name }}</td>
              <td>
                <n-tag v-if="item.is_system" type="primary" size="small">
                  {{ $t('adminWatermarks.systemType') }}
                </n-tag>
                <n-tag v-else type="default" size="small">
                  {{ $t('adminWatermarks.userType') }}
                </n-tag>
              </td>
              <td>
                <span v-if="!item.owner_id" style="color: #63e2b7; font-weight: 500;">
                  {{ $t('watermarks.global') }}
                </span>
                <span v-else>
                  {{ item.owner_nickname || item.owner_username }}
                </span>
              </td>
              <td>
                <div style="font-size: 12px; line-height: 1.6;">
                  <code style="background: rgba(255,255,255,0.08); padding: 1px 4px; border-radius: 3px; word-break: break-all;">
                    {{ parseLayers(item.layers).text }}
                  </code>
                  <div style="color: #888; margin-top: 2px;">
                    {{ parseLayers(item.layers).layout === 'tile' ? $t('watermarks.layoutTile') : $t('watermarks.layoutPosition') }}
                    · {{ Math.round((parseLayers(item.layers).opacity || 0.2) * 100) }}%
                    <n-tag v-if="parseLayers(item.layers).enable_blind_watermark" type="warning" size="tiny" style="margin-left: 6px;">
                      {{ $t('watermarks.blindWatermark') }}
                    </n-tag>
                  </div>
                </div>
              </td>
              <td style="font-size: 12px; color: #888;">
                {{ formatDate(item.updated_at) }}
              </td>
              <td>
                <div style="display: flex; justify-content: flex-end; align-items: center; gap: 8px;">
                  <n-button v-if="item.is_system" size="tiny" quaternary @click="openEditModal(item)">
                    {{ $t('watermarks.edit') }}
                  </n-button>
                  <n-popconfirm @positive-click="deleteWatermark(item.id)">
                    <template #trigger>
                      <n-button size="tiny" quaternary type="error">
                        {{ $t('common.delete') }}
                      </n-button>
                    </template>
                    {{ $t('adminWatermarks.deleteConfirm') }}
                  </n-popconfirm>
                </div>
              </td>
            </tr>
            <tr v-if="filteredWatermarks.length === 0">
              <td colspan="7" style="text-align: center; padding: 32px 0; color: #888;">
                {{ $t('common.noData') }}
              </td>
            </tr>
          </tbody>
        </n-table>
      </n-spin>
    </n-card>

    <!-- Create / Edit Modal -->
    <n-modal
      v-model:show="showModal"
      preset="card"
      :title="isEditing ? $t('watermarks.editTitle') : $t('adminWatermarks.createSystem')"
      style="width: 820px; max-width: 95vw; border-radius: 12px;"
    >
      <n-form label-placement="left" label-width="110">
        <n-form-item :label="$t('watermarks.name')">
          <n-input v-model:value="editForm.name" :placeholder="$t('watermarks.namePlaceholder')" />
        </n-form-item>



        <n-form-item :label="$t('watermarks.content')">
          <div style="width: 100%;">
            <n-input
              v-model:value="editForm.text"
              :placeholder="$t('watermarks.contentPlaceholder')"
              @input="debouncedPreview"
            />
            <div style="margin-top: 10px; display: flex; flex-wrap: wrap; gap: 6px;">
              <n-button v-for="tag in presetTags" :key="tag.value" size="tiny" quaternary @click="insertTag(tag.value)">
                {{ tag.label }}
              </n-button>
            </div>
          </div>
        </n-form-item>

        <n-form-item :label="$t('watermarks.layout')">
          <n-radio-group v-model:value="editForm.layout" @update:value="debouncedPreview">
            <n-radio-button value="tile">{{ $t('watermarks.layoutTile') }}</n-radio-button>
            <n-radio-button value="position">{{ $t('watermarks.layoutPosition') }}</n-radio-button>
          </n-radio-group>
        </n-form-item>

        <!-- Tile specific options -->
        <template v-if="editForm.layout === 'tile'">
          <n-form-item :label="$t('watermarks.angle')">
            <div style="width: 100%; display: flex; align-items: center; gap: 12px;">
              <n-slider v-model:value="editForm.angle" :min="-90" :max="90" style="flex: 1;" @update:value="debouncedPreview" />
              <span style="width: 45px; text-align: right;">{{ editForm.angle }}°</span>
            </div>
          </n-form-item>

          <n-grid cols="2" :x-gap="16">
            <n-grid-item>
              <n-form-item :label="$t('watermarks.spacingX')">
                <n-input-number v-model:value="editForm.spacing_x" :min="80" :max="600" :step="10" style="width: 100%;" @update:value="debouncedPreview" />
              </n-form-item>
            </n-grid-item>
            <n-grid-item>
              <n-form-item :label="$t('watermarks.spacingY')">
                <n-input-number v-model:value="editForm.spacing_y" :min="40" :max="400" :step="10" style="width: 100%;" @update:value="debouncedPreview" />
              </n-form-item>
            </n-grid-item>
          </n-grid>
        </template>

        <!-- Position specific options -->
        <template v-else>
          <n-form-item :label="$t('watermarks.position')">
            <n-select
              v-model:value="editForm.position"
              :options="positionOptions"
              @update:value="debouncedPreview"
            />
          </n-form-item>
        </template>

        <!-- Typography & Appearance -->
        <n-grid cols="1 s:2" :x-gap="16">
          <n-grid-item>
            <n-form-item :label="$t('watermarks.fontFamily')">
              <n-select
                v-model:value="editForm.font_family"
                :options="fontFamilyOptions"
                @update:value="debouncedPreview"
              />
            </n-form-item>
          </n-grid-item>
          <n-grid-item>
            <n-form-item :label="$t('watermarks.adaptiveScaling')">
              <div style="display: flex; align-items: center; gap: 8px; height: 34px;">
                <n-switch v-model:value="editForm.adaptive_scaling" @update:value="debouncedPreview" />
              </div>
            </n-form-item>
          </n-grid-item>
        </n-grid>

        <n-grid cols="1 s:2" :x-gap="16">
          <n-grid-item>
            <n-form-item :label="$t('watermarks.blindWatermark')">
              <div style="display: flex; align-items: center; gap: 8px; height: 34px;">
                <n-switch v-model:value="editForm.enable_blind_watermark" @update:value="debouncedPreview" />
              </div>
            </n-form-item>
          </n-grid-item>
          <n-grid-item v-if="editForm.enable_blind_watermark">
            <n-form-item :label="$t('watermarks.blindWatermarkText')">
              <n-input
                v-model:value="editForm.blind_watermark_text"
                :placeholder="$t('watermarks.blindWatermarkTextPlaceholder')"
                @update:value="debouncedPreview"
              />
            </n-form-item>
          </n-grid-item>
        </n-grid>

        <n-grid cols="1 s:2" :x-gap="16">
          <n-grid-item>
            <n-form-item :label="$t('watermarks.fontSize')">
              <n-input-number v-model:value="editForm.font_size" :min="12" :max="120" style="width: 100%;" @update:value="debouncedPreview" />
            </n-form-item>
          </n-grid-item>
          <n-grid-item>
            <n-form-item :label="$t('watermarks.opacity')">
              <div style="width: 100%; display: flex; align-items: center; gap: 12px;">
                <n-slider v-model:value="editForm.opacityPercent" :min="5" :max="100" style="flex: 1;" @update:value="debouncedPreview" />
                <span style="width: 45px; text-align: right;">{{ editForm.opacityPercent }}%</span>
              </div>
            </n-form-item>
          </n-grid-item>
        </n-grid>

        <n-grid cols="1 s:2" :x-gap="16">
          <n-grid-item>
            <n-form-item :label="$t('watermarks.adaptiveColor')">
              <div style="display: flex; align-items: center; gap: 8px; height: 34px;">
                <n-switch v-model:value="editForm.adaptive_color" @update:value="debouncedPreview" />
              </div>
            </n-form-item>
          </n-grid-item>
          <n-grid-item v-if="!editForm.adaptive_color">
            <n-form-item :label="$t('watermarks.fontColor')">
              <n-color-picker v-model:value="editForm.font_color" :show-alpha="false" @update:value="debouncedPreview" />
            </n-form-item>
          </n-grid-item>
        </n-grid>

        <!-- Preview Card -->
        <n-form-item :label="$t('watermarks.preview')">
          <div style="width: 100%; height: 200px; background: #222; border-radius: 8px; display: flex; align-items: center; justify-content: center; overflow: hidden; border: 1px dashed rgba(255,255,255,0.2);">
            <img v-if="previewUrl" :src="previewUrl" style="max-height: 100%; max-width: 100%; object-fit: contain;" />
            <span v-else style="color: #666;">{{ $t('watermarks.previewPlaceholder') }}</span>
          </div>
        </n-form-item>
      </n-form>

      <template #action>
        <div style="display: flex; justify-content: flex-end; gap: 12px;">
          <n-button @click="showModal = false">{{ $t('common.cancel') }}</n-button>
          <n-button type="primary" :loading="saving" @click="handleSave">
            {{ $t('common.save') }}
          </n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'

const message = useMessage()
const { t } = useI18n()

const watermarks = ref<any[]>([])
const loading = ref(false)
const searchQuery = ref('')
const filterType = ref('all')

const typeFilterOptions = computed(() => [
  { label: t('common.all'), value: 'all' },
  { label: t('adminWatermarks.systemType'), value: 'system' },
  { label: t('adminWatermarks.userType'), value: 'user' }
])

const parseLayers = (layersStr: any) => {
  if (!layersStr) return { text: 'Utopia Images', layout: 'tile', opacity: 0.2 }
  if (typeof layersStr === 'object') return layersStr
  try {
    const parsed = JSON.parse(layersStr)
    if (Array.isArray(parsed) && parsed.length > 0) return parsed[0]
    return parsed
  } catch {
    return { text: 'Utopia Images', layout: 'tile', opacity: 0.2 }
  }
}

const filteredWatermarks = computed(() => {
  return watermarks.value.filter(item => {
    if (filterType.value === 'system' && !item.is_system) return false
    if (filterType.value === 'user' && item.is_system) return false
    if (searchQuery.value) {
      const q = searchQuery.value.toLowerCase()
      const matchName = (item.name || '').toLowerCase().includes(q)
      const matchOwner = (item.owner_nickname || item.owner_username || '').toLowerCase().includes(q)
      const l = parseLayers(item.layers)
      const matchText = (l.text || '').toLowerCase().includes(q)
      if (!matchName && !matchOwner && !matchText) return false
    }
    return true
  })
})

const loadWatermarks = async () => {
  loading.value = true
  try {
    const res = await api.get('/admin/watermarks')
    watermarks.value = res.data || []
  } catch (err: any) {
    message.error(err.response?.data?.error || t('common.loadFailed'))
  } finally {
    loading.value = false
  }
}



const deleteWatermark = async (id: number) => {
  try {
    await api.delete(`/admin/watermarks/${id}`)
    message.success(t('watermarks.deleteSuccess'))
    loadWatermarks()
  } catch (err: any) {
    message.error(err.response?.data?.error || t('watermarks.deleteFailed'))
  }
}

// Modal State & Form
const showModal = ref(false)
const isEditing = ref(false)
const editTargetId = ref<number | null>(null)
const saving = ref(false)
const previewUrl = ref('')
let timer: any = null

const editForm = ref({
  name: '',
  is_system: true,
  text: 'Utopia Images',
  layout: 'tile',
  position: 'bottom-right',
  opacityPercent: 20,
  angle: -30,
  spacing_x: 260,
  spacing_y: 140,
  font_size: 24,
  font_family: 'default',
  font_color: '#ffffff',
  adaptive_color: false,
  stroke_color: '#000000',
  stroke_width: 1,
  adaptive_scaling: true,
  enable_blind_watermark: false,
  blind_watermark_text: ''
})

const presetTags = [
  { label: 'Site Name', value: '{site_name}' },
  { label: 'Uploader', value: '{uploader}' },
  { label: 'Date', value: '{date}' },
  { label: 'Filename', value: '{filename}' }
]

const insertTag = (tag: string) => {
  editForm.value.text += tag
  debouncedPreview()
}

const positionOptions = [
  { label: 'Center', value: 'center' },
  { label: 'Top Left', value: 'top-left' },
  { label: 'Top Right', value: 'top-right' },
  { label: 'Bottom Left', value: 'bottom-left' },
  { label: 'Bottom Right', value: 'bottom-right' }
]

const fontFamilyOptions = [
  { label: 'System Default', value: 'default' },
  { label: 'Sans-serif', value: 'sans-serif' },
  { label: 'Serif', value: 'serif' },
  { label: 'Monospace', value: 'monospace' }
]

const debouncedPreview = () => {
  clearTimeout(timer)
  timer = setTimeout(fetchPreview, 300)
}

const fetchPreview = async () => {
  try {
    const res = await api.post('/watermarks/preview', {
      type: 'text',
      text: editForm.value.text,
      layout: editForm.value.layout,
      position: editForm.value.position,
      opacity: editForm.value.opacityPercent / 100,
      angle: editForm.value.layout === 'position' ? 0 : editForm.value.angle,
      spacing_x: editForm.value.spacing_x,
      spacing_y: editForm.value.spacing_y,
      font_size: editForm.value.font_size,
      font_family: editForm.value.font_family,
      font_color: editForm.value.font_color,
      adaptive_color: editForm.value.adaptive_color,
      stroke_color: editForm.value.stroke_color,
      stroke_width: editForm.value.stroke_width,
      adaptive_scaling: editForm.value.adaptive_scaling,
      enable_blind_watermark: editForm.value.enable_blind_watermark,
      blind_watermark_text: editForm.value.blind_watermark_text
    })
    previewUrl.value = res.data.data_url
  } catch {}
}

const openCreateModal = () => {
  isEditing.value = false
  editTargetId.value = null
  editForm.value = {
    name: '系统全局水印',
    is_system: true,
    text: 'Utopia Images',
    layout: 'tile',
    position: 'bottom-right',
    opacityPercent: 20,
    angle: -30,
    spacing_x: 260,
    spacing_y: 140,
    font_size: 24,
    font_family: 'default',
    font_color: '#ffffff',
    adaptive_color: false,
    stroke_color: '#000000',
    stroke_width: 1,
    adaptive_scaling: true,
    enable_blind_watermark: false,
    blind_watermark_text: ''
  }
  showModal.value = true
  fetchPreview()
}

const openEditModal = (item: any) => {
  if (!item.is_system) return
  isEditing.value = true
  editTargetId.value = item.id
  const l = parseLayers(item.layers)
  editForm.value = {
    name: item.name,
    is_system: true,
    text: l.text || 'Utopia Images',
    layout: l.layout || 'tile',
    position: l.position || 'bottom-right',
    opacityPercent: Math.round((l.opacity || 0.2) * 100),
    angle: l.angle ?? -30,
    spacing_x: l.spacing_x || 260,
    spacing_y: l.spacing_y || 140,
    font_size: l.font_size || 24,
    font_family: l.font_family || 'default',
    font_color: l.font_color || '#ffffff',
    adaptive_color: !!l.adaptive_color,
    stroke_color: l.stroke_color || '#000000',
    stroke_width: l.stroke_width ?? 1,
    adaptive_scaling: l.adaptive_scaling !== false,
    enable_blind_watermark: !!l.enable_blind_watermark,
    blind_watermark_text: l.blind_watermark_text || ''
  }
  showModal.value = true
  fetchPreview()
}

const handleSave = async () => {
  const trimmedName = editForm.value.name.trim()
  if (!trimmedName) {
    message.warning(t('watermarks.nameRequired'))
    return
  }

  saving.value = true
  try {
    const layers = JSON.stringify([{
      type: 'text',
      text: editForm.value.text,
      layout: editForm.value.layout,
      position: editForm.value.position,
      opacity: editForm.value.opacityPercent / 100,
      angle: editForm.value.layout === 'position' ? 0 : editForm.value.angle,
      spacing_x: editForm.value.spacing_x,
      spacing_y: editForm.value.spacing_y,
      font_size: editForm.value.font_size,
      font_family: editForm.value.font_family,
      font_color: editForm.value.font_color,
      adaptive_color: editForm.value.adaptive_color,
      stroke_color: editForm.value.stroke_color,
      stroke_width: editForm.value.stroke_width,
      adaptive_scaling: editForm.value.adaptive_scaling,
      enable_blind_watermark: editForm.value.enable_blind_watermark,
      blind_watermark_text: editForm.value.blind_watermark_text
    }])

    if (isEditing.value && editTargetId.value) {
      await api.patch(`/admin/watermarks/${editTargetId.value}`, {
        name: trimmedName,
        layers: layers
      })
    } else {
      await api.post('/admin/watermarks', {
        name: trimmedName,
        layers: layers
      })
    }

    message.success(t('watermarks.saveSuccess'))
    showModal.value = false
    loadWatermarks()
  } catch (err: any) {
    message.error(err.response?.data?.error || t('common.save'))
  } finally {
    saving.value = false
  }
}

const formatDate = (isoStr: string) => {
  if (!isoStr) return '-'
  const d = new Date(isoStr)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

onMounted(loadWatermarks)
</script>

