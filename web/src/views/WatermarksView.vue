<template>
  <div>
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 24px;">
      <div>
        <h2 style="margin: 0; font-size: 20px;">{{ $t('watermarks.title') }}</h2>
        <p style="margin: 4px 0 0; color: #888; font-size: 13px;">
          {{ $t('watermarks.subtitle') }}
        </p>
      </div>
      <n-button type="primary" @click="openCreateModal">
        {{ $t('watermarks.new') }}
      </n-button>
    </div>

    <!-- Template List Grid -->
    <n-grid cols="1 s:2 m:3" :x-gap="16" :y-gap="16">
      <n-grid-item v-for="t in templates" :key="t.id">
        <n-card :title="t.name" size="small" hoverable style="border-radius: 8px;">
          <template #header-extra>
            <n-tag v-if="!t.owner_id" type="info" size="small">{{ $t('watermarks.global') }}</n-tag>
            <n-tag v-else type="success" size="small">{{ $t('watermarks.personal') }}</n-tag>
          </template>

          <div style="font-size: 13px; color: #aaa; margin-bottom: 12px; line-height: 1.8;">
            <div>
              <strong>{{ $t('watermarks.layout') }}: </strong>
              {{ parseLayers(t.layers).layout === 'tile' ? $t('watermarks.layoutTile') : $t('watermarks.layoutPosition') }}
            </div>
            <div>
              <strong>{{ $t('watermarks.content') }}: </strong>
              <code style="background: rgba(255,255,255,0.08); padding: 2px 4px; border-radius: 4px; word-break: break-all;">
                {{ parseLayers(t.layers).text }}
              </code>
            </div>
            <div style="display: flex; gap: 16px;">
              <span><strong>{{ $t('watermarks.opacity') }}: </strong>{{ Math.round((parseLayers(t.layers).opacity || 0.2) * 100) }}%</span>
              <span v-if="parseLayers(t.layers).font_size"><strong>{{ $t('watermarks.fontSize') }}: </strong>{{ parseLayers(t.layers).font_size }}px</span>
              <span v-if="parseLayers(t.layers).layout === 'tile'"><strong>{{ $t('watermarks.angle') }}: </strong>{{ parseLayers(t.layers).angle ?? -30 }}°</span>
            </div>
          </div>

          <template #action>
            <div style="display: flex; justify-content: flex-end; gap: 8px;">
              <n-button size="tiny" quaternary @click="previewExisting(t)">{{ $t('watermarks.edit') }}</n-button>
              <n-popconfirm @positive-click="deleteTemplate(t.id)">
                <template #trigger>
                  <n-button size="tiny" quaternary type="error">{{ $t('common.delete') }}</n-button>
                </template>
                {{ $t('watermarks.confirmDelete') }}
              </n-popconfirm>
            </div>
          </template>
        </n-card>
      </n-grid-item>
    </n-grid>

    <div v-if="templates.length === 0" style="text-align: center; padding: 48px 0; color: #666;">
      {{ $t('watermarks.empty') }}
    </div>

    <!-- Create / Edit Modal -->
    <n-modal
      v-model:show="showModal"
      preset="card"
      :title="isEditing ? $t('watermarks.editTitle') : $t('watermarks.new')"
      style="width: 820px; max-width: 95vw; border-radius: 12px;"
    >
      <n-form label-placement="left" label-width="110">
        <n-form-item :label="$t('watermarks.name')">
          <n-input v-model:value="editForm.name" :placeholder="$t('watermarks.namePlaceholder')" />
        </n-form-item>

        <n-form-item v-if="isAdmin" :label="$t('watermarks.global')">
          <div style="display: flex; align-items: center; gap: 10px;">
            <n-switch v-model:value="editForm.is_global" />
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-icon size="16" style="color: #888; cursor: pointer;">
                  <InformationCircleOutline />
                </n-icon>
              </template>
              {{ $t('watermarks.globalTip') }}
            </n-tooltip>
          </div>
        </n-form-item>

        <n-form-item :label="$t('watermarks.content')">
          <div style="width: 100%;">
            <n-input
              v-model:value="editForm.text"
              :placeholder="$t('watermarks.contentPlaceholder')"
              @input="debouncedPreview"
            />

            <!-- Presets Section -->
            <div style="margin-top: 10px;">
              <div style="font-size: 12px; color: #888; margin-bottom: 6px; display: flex; align-items: center; gap: 6px;">
                <span>{{ $t('watermarks.presetTags') }}</span>
                <n-tooltip trigger="hover">
                  <template #trigger>
                    <n-icon size="14" style="color: #888; cursor: pointer;">
                      <InformationCircleOutline />
                    </n-icon>
                  </template>
                  {{ $t('watermarks.dynamicWarningTooltip') }}
                </n-tooltip>
              </div>

              <div style="display: flex; flex-wrap: wrap; gap: 6px;">
                <n-button v-for="tag in presetTags" :key="tag.value" size="tiny" quaternary @click="insertTag(tag.value)">
                  {{ tag.label }}
                </n-button>
              </div>
            </div>

            <!-- Warning notice for dynamic items -->
            <n-alert v-if="hasDynamicTags" type="warning" :show-icon="false" style="margin-top: 10px; font-size: 12px; padding: 6px 12px;">
              <div style="display: flex; align-items: center; gap: 6px;">
                <n-icon size="14" style="color: #f0a020;">
                  <InformationCircleOutline />
                </n-icon>
                <span>{{ $t('watermarks.dynamicWarning') }}</span>
              </div>
            </n-alert>
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
              <div style="display: flex; gap: 4px;">
                <n-button size="tiny" quaternary @click="setAngle(-45)">-45°</n-button>
                <n-button size="tiny" quaternary @click="setAngle(-30)">-30°</n-button>
                <n-button size="tiny" quaternary @click="setAngle(0)">0°</n-button>
                <n-button size="tiny" quaternary @click="setAngle(30)">30°</n-button>
                <n-button size="tiny" quaternary @click="setAngle(45)">45°</n-button>
              </div>
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
                <n-tooltip trigger="hover">
                  <template #trigger>
                    <n-icon size="16" style="color: #888; cursor: pointer;">
                      <InformationCircleOutline />
                    </n-icon>
                  </template>
                  {{ $t('watermarks.adaptiveScalingTip') }}
                </n-tooltip>
              </div>
            </n-form-item>
          </n-grid-item>
        </n-grid>

        <n-grid cols="1 s:2" :x-gap="16">
          <n-grid-item>
            <n-form-item :label="$t('watermarks.fontSize')">
              <n-input-number v-model:value="editForm.font_size" :min="10" :max="96" style="width: 100%;" @update:value="debouncedPreview" />
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
            <n-form-item :label="$t('watermarks.fontColor')">
              <div style="display: flex; gap: 8px; width: 100%; align-items: center;">
                <n-color-picker
                  v-model:value="editForm.font_color"
                  :show-alpha="false"
                  :disabled="editForm.adaptive_color"
                  style="flex: 1;"
                  @update:value="debouncedPreview"
                />
                <div style="display: flex; align-items: center; gap: 4px; white-space: nowrap;">
                  <n-checkbox v-model:checked="editForm.adaptive_color" @update:checked="debouncedPreview">
                    {{ $t('watermarks.adaptiveColor') }}
                  </n-checkbox>
                  <n-tooltip trigger="hover">
                    <template #trigger>
                      <n-icon size="14" style="color: #888; cursor: pointer;">
                        <InformationCircleOutline />
                      </n-icon>
                    </template>
                    {{ $t('watermarks.adaptiveColorTip') }}
                  </n-tooltip>
                </div>
              </div>
            </n-form-item>
          </n-grid-item>
          <n-grid-item>
            <n-form-item :label="$t('watermarks.strokeColor')">
              <div style="display: flex; gap: 12px; width: 100%;">
                <n-color-picker v-model:value="editForm.stroke_color" :show-alpha="false" style="flex: 1;" @update:value="debouncedPreview" />
                <n-select
                  v-model:value="editForm.stroke_width"
                  :options="strokeWidthOptions"
                  style="width: 100px;"
                  @update:value="debouncedPreview"
                />
              </div>
            </n-form-item>
          </n-grid-item>
        </n-grid>

        <!-- Live Preview Box -->
        <n-divider dashed title-placement="left" style="font-size: 13px; margin: 16px 0 12px;">{{ $t('watermarks.preview') }}</n-divider>
        <div style="text-align: center; background: #0c0d12; padding: 16px; border-radius: 8px;">
          <img
            v-if="previewUrl"
            :src="previewUrl"
            style="max-width: 100%; height: auto; border-radius: 6px; box-shadow: 0 4px 16px rgba(0,0,0,0.5);"
          />
          <div v-else style="color: #666; padding: 30px;">{{ $t('watermarks.previewLoading') }}</div>
        </div>
      </n-form>

      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 12px;">
          <n-button @click="showModal = false">{{ $t('common.cancel') }}</n-button>
          <n-button type="primary" :loading="saving" @click="handleSaveTemplate">{{ $t('watermarks.save') }}</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useMessage } from 'naive-ui'
import { InformationCircleOutline } from '@vicons/ionicons5'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'

const message = useMessage()
const { t } = useI18n()
const templates = ref<any[]>([])
const showModal = ref(false)
const saving = ref(false)
const isAdmin = ref(false)
const isEditing = ref(false)
const editTargetId = ref<number | null>(null)
const previewUrl = ref('')

const positionOptions = computed(() => [
  { label: t('watermarks.posTopLeft'), value: 'top-left' },
  { label: t('watermarks.posTopCenter'), value: 'top-center' },
  { label: t('watermarks.posTopRight'), value: 'top-right' },
  { label: t('watermarks.posCenterLeft'), value: 'center-left' },
  { label: t('watermarks.posCenter'), value: 'center' },
  { label: t('watermarks.posCenterRight'), value: 'center-right' },
  { label: t('watermarks.posBottomLeft'), value: 'bottom-left' },
  { label: t('watermarks.posBottomCenter'), value: 'bottom-center' },
  { label: t('watermarks.posBottomRight'), value: 'bottom-right' }
])

const strokeWidthOptions = computed(() => [
  { label: t('watermarks.strokeNone'), value: 0 },
  { label: '1px', value: 1 },
  { label: '2px', value: 2 },
  { label: '3px', value: 3 }
])

const presetTags = computed(() => [
  { label: t('watermarks.tagYear'), value: '{year}' },
  { label: t('watermarks.tagDate'), value: '{date}' },
  { label: t('watermarks.tagTime'), value: '{time}' },
  { label: t('watermarks.tagDatetime'), value: '{datetime}' },
  { label: t('watermarks.tagFilename'), value: '{filename}' },
  { label: t('watermarks.tagDimensions'), value: '{dimensions}' },
  { label: t('watermarks.tagFilesize'), value: '{filesize}' },
  { label: t('watermarks.tagOwner'), value: '{owner}' },
  { label: t('watermarks.tagIp'), value: '{ip}' },
  { label: t('watermarks.tagDomain'), value: '{domain}' },
  { label: t('watermarks.tagCopyright'), value: '{copyright}' },
  { label: t('watermarks.tagConfidential'), value: '{confidential}' },
  { label: t('watermarks.tagPreviewOnly'), value: '{preview_only}' }
])

const fontFamilyOptions = computed(() => [
  { label: t('watermarks.fontDefault'), value: 'default' },
  { label: t('watermarks.fontYahei'), value: 'yahei' },
  { label: t('watermarks.fontSimhei'), value: 'simhei' },
  { label: t('watermarks.fontSimsun'), value: 'simsun' },
  { label: 'Arial', value: 'arial' }
])

const editForm = ref({
  name: t('watermarks.defaultName'),
  is_global: false,
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
  adaptive_scaling: true
})

const hasDynamicTags = computed(() => {
  return editForm.value.text.includes('{') && editForm.value.text.includes('}')
})

const setAngle = (deg: number) => {
  editForm.value.angle = deg
  debouncedPreview()
}

const parseLayers = (jsonStr: string) => {
  try {
    return JSON.parse(jsonStr)
  } catch {
    return { text: 'Utopia Images', layout: 'tile', opacity: 0.2, angle: -30, font_size: 24 }
  }
}

const loadTemplates = async () => {
  try {
    const res = await api.get('/watermarks')
    templates.value = res.data
  } catch (err: any) {
    message.error(t('watermarks.deleteFailed'))
  }
}

const checkAdmin = async () => {
  try {
    const res = await api.get('/me')
    isAdmin.value = res.data.role === 'admin'
  } catch {}
}

const openCreateModal = () => {
  isEditing.value = false
  editTargetId.value = null
  editForm.value = {
    name: t('watermarks.tileName'),
    is_global: isAdmin.value,
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
    adaptive_scaling: true
  }
  showModal.value = true
  fetchPreview()
}

const insertTag = (tag: string) => {
  editForm.value.text += ' ' + tag
  fetchPreview()
}

let timer: any = null
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
      angle: editForm.value.angle,
      spacing_x: editForm.value.spacing_x,
      spacing_y: editForm.value.spacing_y,
      font_size: editForm.value.font_size,
      font_family: editForm.value.font_family,
      font_color: editForm.value.font_color,
      adaptive_color: editForm.value.adaptive_color,
      stroke_color: editForm.value.stroke_color,
      stroke_width: editForm.value.stroke_width,
      adaptive_scaling: editForm.value.adaptive_scaling
    })
    previewUrl.value = res.data.data_url
  } catch {}
}

const handleSaveTemplate = async () => {
  if (!editForm.value.name) {
    message.warning(t('watermarks.namePlaceholder'))
    return
  }

  saving.value = true
  try {
    const layers = JSON.stringify({
      type: 'text',
      text: editForm.value.text,
      layout: editForm.value.layout,
      position: editForm.value.position,
      opacity: editForm.value.opacityPercent / 100,
      angle: editForm.value.angle,
      spacing_x: editForm.value.spacing_x,
      spacing_y: editForm.value.spacing_y,
      font_size: editForm.value.font_size,
      font_family: editForm.value.font_family,
      font_color: editForm.value.font_color,
      adaptive_color: editForm.value.adaptive_color,
      stroke_color: editForm.value.stroke_color,
      stroke_width: editForm.value.stroke_width,
      adaptive_scaling: editForm.value.adaptive_scaling
    })

    if (isEditing.value && editTargetId.value) {
      await api.patch(`/watermarks/${editTargetId.value}`, {
        name: editForm.value.name,
        layers: layers
      })
    } else {
      await api.post('/watermarks', {
        name: editForm.value.name,
        layers: layers,
        is_global: editForm.value.is_global
      })
    }

    message.success(t('watermarks.saveSuccess'))
    showModal.value = false
    loadTemplates()
  } catch (err: any) {
    message.error(err.response?.data?.error || t('watermarks.deleteFailed'))
  } finally {
    saving.value = false
  }
}

const previewExisting = (tItem: any) => {
  isEditing.value = true
  editTargetId.value = tItem.id
  const l = parseLayers(tItem.layers)
  editForm.value = {
    name: tItem.name,
    is_global: !tItem.owner_id,
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
    adaptive_scaling: l.adaptive_scaling !== false
  }
  showModal.value = true
  fetchPreview()
}

const deleteTemplate = async (id: number) => {
  try {
    await api.delete(`/watermarks/${id}`)
    message.success(t('watermarks.deleteSuccess'))
    loadTemplates()
  } catch (err: any) {
    message.error(err.response?.data?.error || t('watermarks.deleteFailed'))
  }
}

onMounted(() => {
  loadTemplates()
  checkAdmin()
})
</script>
