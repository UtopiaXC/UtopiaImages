<template>
  <n-modal
    v-model:show="visible"
    preset="card"
    :title="$t('editor.title') + ': ' + (file?.display_name || '')"
    style="width: 1120px; max-width: 96vw; border-radius: 12px;"
    :mask-closable="false"
    @after-enter="initEditor"
  >
    <div style="display: flex; gap: 16px; height: 620px; max-height: 75vh;">
      <!-- Left: Tools Sidebar -->
      <div style="width: 250px; display: flex; flex-direction: column; gap: 12px; overflow-y: auto; padding-right: 8px;">
        <!-- Clean Dropdown to switch editing modules instead of cramped segmented tabs -->
        <div>
          <div style="font-size: 13px; color: #888; margin-bottom: 6px;">{{ $t('editor.selectMode') }}</div>
          <n-select
            v-model:value="activeToolTab"
            :options="toolTabOptions"
            size="medium"
          />
        </div>

        <!-- Tool Panel: Crop -->
        <div v-if="activeToolTab === 'crop'" style="display: flex; flex-direction: column; gap: 12px;">
          <div style="font-size: 13px; color: #aaa;">{{ $t('editor.aspectRatio') }}</div>
          <n-grid :cols="2" :x-gap="8" :y-gap="8">
            <n-grid-item v-for="ratio in cropRatios" :key="ratio.value">
              <n-button
                block
                size="small"
                :type="selectedCropRatio === ratio.value ? 'primary' : 'default'"
                @click="setCropRatio(ratio.value)"
              >
                {{ ratio.label }}
              </n-button>
            </n-grid-item>
          </n-grid>

          <div style="font-size: 12px; color: #888; line-height: 1.5; margin-top: 4px;">
            {{ $t('editor.cropDragTip') }}
          </div>

          <n-button type="primary" size="small" style="margin-top: 6px;" @click="applyCrop">
            {{ $t('editor.applyCrop') }}
          </n-button>
          <n-button v-if="cropBox" size="small" quaternary @click="cropBox = null">
            {{ $t('editor.resetCropBox') }}
          </n-button>
        </div>

        <!-- Tool Panel: Transform (Rotate & Flip) -->
        <div v-if="activeToolTab === 'transform'" style="display: flex; flex-direction: column; gap: 10px;">
          <div style="font-size: 13px; color: #aaa;">{{ $t('editor.rotation') }}</div>
          <div style="display: flex; gap: 8px;">
            <n-button size="small" style="flex: 1;" @click="rotateImage(-90)">
              {{ $t('editor.rotateLeft') }}
            </n-button>
            <n-button size="small" style="flex: 1;" @click="rotateImage(90)">
              {{ $t('editor.rotateRight') }}
            </n-button>
          </div>

          <div style="font-size: 13px; color: #aaa; margin-top: 8px;">{{ $t('editor.mirror') }}</div>
          <div style="display: flex; gap: 8px;">
            <n-button size="small" style="flex: 1;" @click="flipImage('h')">
              {{ $t('editor.flipHorizontal') }}
            </n-button>
            <n-button size="small" style="flex: 1;" @click="flipImage('v')">
              {{ $t('editor.flipVertical') }}
            </n-button>
          </div>
        </div>

        <!-- Tool Panel: Draw & Annotations -->
        <div v-if="activeToolTab === 'draw'" style="display: flex; flex-direction: column; gap: 12px;">
          <div style="font-size: 13px; color: #aaa;">{{ $t('editor.toolType') }}</div>
          <n-grid :cols="2" :x-gap="8" :y-gap="8">
            <n-grid-item>
              <n-button
                block
                size="small"
                :type="drawTool === 'brush' ? 'primary' : 'default'"
                @click="drawTool = 'brush'"
              >
                {{ $t('editor.toolBrush') }}
              </n-button>
            </n-grid-item>
            <n-grid-item>
              <n-button
                block
                size="small"
                :type="drawTool === 'rect' ? 'primary' : 'default'"
                @click="drawTool = 'rect'"
              >
                {{ $t('editor.toolRect') }}
              </n-button>
            </n-grid-item>
            <n-grid-item>
              <n-button
                block
                size="small"
                :type="drawTool === 'roundRect' ? 'primary' : 'default'"
                @click="drawTool = 'roundRect'"
              >
                {{ $t('editor.toolRoundRect') }}
              </n-button>
            </n-grid-item>
            <n-grid-item>
              <n-button
                block
                size="small"
                :type="drawTool === 'circle' ? 'primary' : 'default'"
                @click="drawTool = 'circle'"
              >
                {{ $t('editor.toolCircle') }}
              </n-button>
            </n-grid-item>
            <n-grid-item>
              <n-button
                block
                size="small"
                :type="drawTool === 'arrow' ? 'primary' : 'default'"
                @click="drawTool = 'arrow'"
              >
                {{ $t('editor.toolArrow') }}
              </n-button>
            </n-grid-item>
            <n-grid-item>
              <n-button
                block
                size="small"
                :type="drawTool === 'mosaic' ? 'primary' : 'default'"
                @click="drawTool = 'mosaic'"
              >
                {{ $t('editor.toolMosaic') }}
              </n-button>
            </n-grid-item>
            <n-grid-item>
              <n-button
                block
                size="small"
                :type="drawTool === 'blur' ? 'primary' : 'default'"
                @click="drawTool = 'blur'"
              >
                {{ $t('editor.toolBlur') }}
              </n-button>
            </n-grid-item>
            <n-grid-item>
              <n-button
                block
                size="small"
                :type="drawTool === 'text' ? 'primary' : 'default'"
                @click="drawTool = 'text'"
              >
                {{ $t('editor.toolText') }}
              </n-button>
            </n-grid-item>
          </n-grid>

          <!-- Mode selector for Mosaic and Blur: Brush vs Box -->
          <div v-if="drawTool === 'mosaic' || drawTool === 'blur'" style="display: flex; flex-direction: column; gap: 6px; background: rgba(255,255,255,0.03); padding: 8px; border-radius: 6px;">
            <div style="font-size: 12px; color: #aaa;">{{ $t('editor.applyMethod') }}</div>
            <n-radio-group v-model:value="mosaicBlurMode" size="small">
              <n-radio-button value="brush">{{ $t('editor.modeBrush') }}</n-radio-button>
              <n-radio-button value="box">{{ $t('editor.modeBox') }}</n-radio-button>
            </n-radio-group>
          </div>

          <div v-if="drawTool === 'text'" style="display: flex; flex-direction: column; gap: 6px;">
            <div style="font-size: 13px; color: #aaa;">{{ $t('editor.textContent') }}</div>
            <n-input v-model:value="annotationText" size="small" :placeholder="$t('editor.textPlaceholder')" />
          </div>

          <div style="font-size: 13px; color: #aaa;">{{ $t('editor.strokeWidth') }}: {{ strokeWidth }}px</div>
          <n-slider v-model:value="strokeWidth" :min="1" :max="40" />

          <div v-if="drawTool !== 'mosaic' && drawTool !== 'blur'" style="font-size: 13px; color: #aaa;">{{ $t('editor.color') }}</div>
          <n-color-picker v-if="drawTool !== 'mosaic' && drawTool !== 'blur'" v-model:value="strokeColor" :show-alpha="false" />
        </div>

        <!-- Tool Panel: Filters & Adjustments -->
        <div v-if="activeToolTab === 'filter'" style="display: flex; flex-direction: column; gap: 10px;">
          <div style="font-size: 13px; color: #aaa;">{{ $t('editor.presetFilters') }}</div>
          <n-grid :cols="2" :x-gap="8" :y-gap="8">
            <n-grid-item>
              <n-button size="small" block :type="activeFilterPreset === 'none' ? 'primary' : 'default'" @click="applyFilterPreset('none')">
                {{ $t('editor.filterNone') }}
              </n-button>
            </n-grid-item>
            <n-grid-item>
              <n-button size="small" block :type="activeFilterPreset === 'grayscale' ? 'primary' : 'default'" @click="applyFilterPreset('grayscale')">
                {{ $t('editor.filterGrayscale') }}
              </n-button>
            </n-grid-item>
            <n-grid-item>
              <n-button size="small" block :type="activeFilterPreset === 'sepia' ? 'primary' : 'default'" @click="applyFilterPreset('sepia')">
                {{ $t('editor.filterSepia') }}
              </n-button>
            </n-grid-item>
            <n-grid-item>
              <n-button size="small" block :type="activeFilterPreset === 'invert' ? 'primary' : 'default'" @click="applyFilterPreset('invert')">
                {{ $t('editor.filterInvert') }}
              </n-button>
            </n-grid-item>
          </n-grid>

          <div style="font-size: 13px; color: #aaa; margin-top: 6px;">{{ $t('editor.brightness') }}: {{ brightness }}%</div>
          <n-slider v-model:value="brightness" :min="-100" :max="100" @update:value="renderCanvas" />

          <div style="font-size: 13px; color: #aaa;">{{ $t('editor.contrast') }}: {{ contrast }}%</div>
          <n-slider v-model:value="contrast" :min="-100" :max="100" @update:value="renderCanvas" />

          <div style="font-size: 13px; color: #aaa;">{{ $t('editor.saturation') }}: {{ saturation }}%</div>
          <n-slider v-model:value="saturation" :min="-100" :max="100" @update:value="renderCanvas" />

          <div style="font-size: 13px; color: #aaa;">{{ $t('editor.blur') }}: {{ blurPx }}px</div>
          <n-slider v-model:value="blurPx" :min="0" :max="20" @update:value="renderCanvas" />

          <n-button size="small" quaternary @click="resetFilters" style="margin-top: 6px;">
            {{ $t('editor.resetFilters') }}
          </n-button>
        </div>

        <!-- History Actions -->
        <div style="margin-top: auto; padding-top: 12px; border-top: 1px solid rgba(255, 255, 255, 0.08); display: flex; gap: 8px;">
          <n-button size="small" :disabled="historyIndex <= 0" style="flex: 1;" @click="handleUndo">
            {{ $t('editor.undo') }}
          </n-button>
          <n-button size="small" :disabled="historyIndex >= historyStack.length - 1" style="flex: 1;" @click="handleRedo">
            {{ $t('editor.redo') }}
          </n-button>
          <n-button size="small" quaternary type="error" @click="handleResetAll">
            {{ $t('editor.resetAll') }}
          </n-button>
        </div>
      </div>

      <!-- Right: Interactive Canvas Area -->
      <div
        ref="canvasContainer"
        style="flex: 1; background: #0c0d12; border-radius: 8px; border: 1px solid rgba(255, 255, 255, 0.08); display: flex; align-items: center; justify-content: center; position: relative; overflow: hidden; user-select: none;"
      >
        <div style="position: relative; max-width: 100%; max-height: 100%; display: inline-flex;">
          <canvas
            ref="canvasRef"
            style="max-width: 100%; max-height: 100%; object-fit: contain; box-shadow: 0 4px 20px rgba(0, 0, 0, 0.6); cursor: crosshair; display: block;"
            @mousedown="handleMouseDown"
            @mousemove="handleMouseMove"
            @mouseup="handleMouseUp"
            @mouseleave="handleMouseUp"
          />

          <!-- Live Interactive Crop Selection Overlay -->
          <div
            v-if="activeToolTab === 'crop' && cropBoxStyle"
            :style="cropBoxStyle"
            style="position: absolute; border: 2px dashed #63e2b7; box-shadow: 0 0 0 9999px rgba(0, 0, 0, 0.55); cursor: move; user-select: none;"
            @mousedown.stop="handleCropBoxMouseDown($event, 'move')"
          >
            <!-- 4 Corner Handles -->
            <div
              style="position: absolute; top: -6px; left: -6px; width: 12px; height: 12px; background: #63e2b7; border-radius: 2px; cursor: nwse-resize; z-index: 10;"
              @mousedown.stop="handleCropBoxMouseDown($event, 'nw')"
            />
            <div
              style="position: absolute; top: -6px; right: -6px; width: 12px; height: 12px; background: #63e2b7; border-radius: 2px; cursor: nesw-resize; z-index: 10;"
              @mousedown.stop="handleCropBoxMouseDown($event, 'ne')"
            />
            <div
              style="position: absolute; bottom: -6px; right: -6px; width: 12px; height: 12px; background: #63e2b7; border-radius: 2px; cursor: nwse-resize; z-index: 10;"
              @mousedown.stop="handleCropBoxMouseDown($event, 'se')"
            />
            <div
              style="position: absolute; bottom: -6px; left: -6px; width: 12px; height: 12px; background: #63e2b7; border-radius: 2px; cursor: nesw-resize; z-index: 10;"
              @mousedown.stop="handleCropBoxMouseDown($event, 'sw')"
            />

            <!-- 4 Edge Handles -->
            <div
              style="position: absolute; top: -5px; left: calc(50% - 10px); width: 20px; height: 10px; background: #63e2b7; border-radius: 2px; cursor: ns-resize; z-index: 10;"
              @mousedown.stop="handleCropBoxMouseDown($event, 'n')"
            />
            <div
              style="position: absolute; bottom: -5px; left: calc(50% - 10px); width: 20px; height: 10px; background: #63e2b7; border-radius: 2px; cursor: ns-resize; z-index: 10;"
              @mousedown.stop="handleCropBoxMouseDown($event, 's')"
            />
            <div
              style="position: absolute; left: -5px; top: calc(50% - 10px); width: 10px; height: 20px; background: #63e2b7; border-radius: 2px; cursor: ew-resize; z-index: 10;"
              @mousedown.stop="handleCropBoxMouseDown($event, 'w')"
            />
            <div
              style="position: absolute; right: -5px; top: calc(50% - 10px); width: 10px; height: 20px; background: #63e2b7; border-radius: 2px; cursor: ew-resize; z-index: 10;"
              @mousedown.stop="handleCropBoxMouseDown($event, 'e')"
            />

            <!-- Rule-of-thirds grid lines -->
            <div style="position: absolute; left: 33.33%; top: 0; width: 1px; height: 100%; border-left: 1px dotted rgba(255,255,255,0.4); pointer-events: none;" />
            <div style="position: absolute; left: 66.66%; top: 0; width: 1px; height: 100%; border-left: 1px dotted rgba(255,255,255,0.4); pointer-events: none;" />
            <div style="position: absolute; top: 33.33%; left: 0; height: 1px; width: 100%; border-top: 1px dotted rgba(255,255,255,0.4); pointer-events: none;" />
            <div style="position: absolute; top: 66.66%; left: 0; height: 1px; width: 100%; border-top: 1px dotted rgba(255,255,255,0.4); pointer-events: none;" />
          </div>
        </div>

        <div style="position: absolute; bottom: 12px; right: 16px; background: rgba(0, 0, 0, 0.6); padding: 4px 10px; border-radius: 6px; font-size: 12px; color: #aaa;">
          {{ currentW }} × {{ currentH }} px
        </div>
      </div>
    </div>

    <!-- Modal Footer Actions -->
    <template #footer>
      <div style="display: flex; justify-content: space-between; align-items: center; width: 100%;">
        <div style="font-size: 13px; color: #888;">
          {{ $t('editor.saveNotice') }}
        </div>
        <div style="display: flex; gap: 12px;">
          <n-button @click="visible = false">{{ $t('common.cancel') }}</n-button>

          <!-- Save as copy -->
          <n-button type="info" :loading="savingCopy" @click="handleSave('copy')">
            {{ $t('editor.saveAsCopy') }}
          </n-button>

          <!-- Overwrite original -->
          <n-popconfirm @positive-click="handleSave('overwrite')">
            <template #trigger>
              <n-button type="primary" :loading="savingOverwrite">
                {{ $t('editor.overwriteOriginal') }}
              </n-button>
            </template>
            {{ $t('editor.confirmOverwrite') }}
          </n-popconfirm>
        </div>
      </div>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'

const props = defineProps<{
  show: boolean
  file: any
}>()

const emit = defineEmits<{
  (e: 'update:show', val: boolean): void
  (e: 'saved', file: any): void
}>()

const message = useMessage()
const { t } = useI18n()

const visible = computed({
  get: () => props.show,
  set: (val) => emit('update:show', val)
})

const canvasRef = ref<HTMLCanvasElement | null>(null)
const canvasContainer = ref<HTMLDivElement | null>(null)

// Mode dropdown options
const toolTabOptions = computed(() => [
  { label: t('editor.tabCrop'), value: 'crop' },
  { label: t('editor.tabTransform'), value: 'transform' },
  { label: t('editor.tabDraw'), value: 'draw' },
  { label: t('editor.tabFilter'), value: 'filter' }
])
const activeToolTab = ref('crop')

// Crop State
const selectedCropRatio = ref('free')
const cropRatios = computed(() => [
  { label: t('editor.ratioFree'), value: 'free' },
  { label: '1:1', value: '1:1' },
  { label: '4:3', value: '4:3' },
  { label: '16:9', value: '16:9' },
  { label: '3:2', value: '3:2' },
  { label: '9:16', value: '9:16' }
])
const cropBox = ref<{ x: number; y: number; w: number; h: number } | null>(null)
const isCroppingDrag = ref(false)

const cropBoxStyle = computed(() => {
  if (!cropBox.value || !canvasRef.value) return null
  const cw = canvasRef.value.width
  const ch = canvasRef.value.height
  if (cw <= 0 || ch <= 0) return null

  return {
    left: `${(cropBox.value.x / cw) * 100}%`,
    top: `${(cropBox.value.y / ch) * 100}%`,
    width: `${(cropBox.value.w / cw) * 100}%`,
    height: `${(cropBox.value.h / ch) * 100}%`
  }
})

// Draw State
type DrawTool = 'brush' | 'rect' | 'roundRect' | 'circle' | 'arrow' | 'mosaic' | 'blur' | 'text'
const drawTool = ref<DrawTool>('brush')
const mosaicBlurMode = ref<'brush' | 'box'>('brush')
const strokeWidth = ref(4)
const strokeColor = ref('#ff3b30')
const annotationText = ref('')

// Filters
const activeFilterPreset = ref('none')
const brightness = ref(0)
const contrast = ref(0)
const saturation = ref(0)
const blurPx = ref(0)

// Canvas Dimensions & Undo Stack
const currentW = ref(0)
const currentH = ref(0)
const originalImage = ref<HTMLImageElement | null>(null)
const historyStack = ref<ImageData[]>([])
const historyIndex = ref(-1)

// Mouse drag state
let isDrawing = false
let startX = 0
let startY = 0
let snapshotData: ImageData | null = null

// Saving state
const savingCopy = ref(false)
const savingOverwrite = ref(false)

watch(() => props.show, (newVal) => {
  if (newVal && props.file) {
    nextTick(() => {
      initEditor()
    })
  }
})

watch(() => props.file, (newVal) => {
  if (props.show && newVal) {
    nextTick(() => {
      initEditor()
    })
  }
})

watch(activeToolTab, (newTab) => {
  if (newTab === 'crop') {
    if (!cropBox.value || cropBox.value.w < 20 || cropBox.value.h < 20) {
      setCropRatio(selectedCropRatio.value)
    }
  }
})

let initRetryCount = 0

const initEditor = () => {
  if (!props.file) return
  if (!canvasRef.value) {
    if (initRetryCount < 20) {
      initRetryCount++
      setTimeout(initEditor, 50)
    }
    return
  }
  initRetryCount = 0

  historyStack.value = []
  historyIndex.value = -1
  resetFilters()
  activeToolTab.value = 'crop'
  cropBox.value = null

  const cleanPath = (props.file.public_path || props.file.url || '').replace(/^\/+/, '')
  const origin = window.location.origin
  const directUrl = cleanPath.startsWith('http')
    ? (cleanPath.includes('?') ? `${cleanPath}&from=console` : `${cleanPath}?from=console`)
    : `${origin}/${encodeURI(cleanPath)}?from=console&t=${Date.now()}`

  const loadFromImageElement = (sourceUrl: string, isFallback = false) => {
    const img = new Image()
    img.crossOrigin = 'anonymous'
    img.onload = () => {
      originalImage.value = img
      setupCanvas(img.width, img.height)
      const ctx = canvasRef.value?.getContext('2d')
      if (ctx) {
        ctx.clearRect(0, 0, img.width, img.height)
        ctx.drawImage(img, 0, 0)
        saveHistory()
        setCropRatio(selectedCropRatio.value)
      }
    }
    img.onerror = (err) => {
      console.warn('Direct image load failed for:', sourceUrl, err)
      if (!isFallback && props.file?.id) {
        // Fallback: fetch raw unwatermarked blob directly from file endpoint
        api.get(`/files/${props.file.id}/raw`, { responseType: 'blob' })
          .then((res) => {
            const blobUrl = URL.createObjectURL(res.data)
            loadFromImageElement(blobUrl, true)
          })
          .catch((e) => {
            console.error('Raw file fetch error:', e)
          })
      }
    }
    img.src = sourceUrl
  }

  loadFromImageElement(directUrl)
}

const setupCanvas = (w: number, h: number) => {
  if (!canvasRef.value) return
  canvasRef.value.width = w
  canvasRef.value.height = h
  currentW.value = w
  currentH.value = h
  if (!cropBox.value || cropBox.value.w < 20 || cropBox.value.h < 20) {
    cropBox.value = {
      x: Math.round(w * 0.05),
      y: Math.round(h * 0.05),
      w: Math.round(w * 0.9),
      h: Math.round(h * 0.9)
    }
  }
}

const saveHistory = () => {
  const ctx = canvasRef.value?.getContext('2d')
  if (!ctx || !canvasRef.value) return
  const data = ctx.getImageData(0, 0, canvasRef.value.width, canvasRef.value.height)
  if (historyIndex.value < historyStack.value.length - 1) {
    historyStack.value = historyStack.value.slice(0, historyIndex.value + 1)
  }
  historyStack.value.push(data)
  historyIndex.value = historyStack.value.length - 1
}

const handleUndo = () => {
  if (historyIndex.value > 0) {
    historyIndex.value--
    restoreHistory(historyIndex.value)
  }
}

const handleRedo = () => {
  if (historyIndex.value < historyStack.value.length - 1) {
    historyIndex.value++
    restoreHistory(historyIndex.value)
  }
}

const restoreHistory = (idx: number) => {
  const data = historyStack.value[idx]
  if (!data || !canvasRef.value) return
  setupCanvas(data.width, data.height)
  const ctx = canvasRef.value.getContext('2d')
  ctx?.putImageData(data, 0, 0)
}

const handleResetAll = () => {
  if (!originalImage.value || !canvasRef.value) return
  setupCanvas(originalImage.value.width, originalImage.value.height)
  const ctx = canvasRef.value.getContext('2d')
  ctx?.drawImage(originalImage.value, 0, 0)
  resetFilters()
  cropBox.value = null
  saveHistory()
}

// Filters Rendering
const resetFilters = () => {
  brightness.value = 0
  contrast.value = 0
  saturation.value = 0
  blurPx.value = 0
  activeFilterPreset.value = 'none'
}

const applyFilterPreset = (preset: string) => {
  activeFilterPreset.value = preset
  resetFilters()
  activeFilterPreset.value = preset

  if (preset === 'grayscale') {
    saturation.value = -100
  } else if (preset === 'sepia') {
    contrast.value = 15
    brightness.value = 10
  }
  renderCanvas()
}

const renderCanvas = () => {
  if (!canvasRef.value || historyIndex.value < 0) return
  const baseData = historyStack.value[historyIndex.value]
  if (!baseData) return

  const ctx = canvasRef.value.getContext('2d')
  if (!ctx) return

  ctx.putImageData(baseData, 0, 0)

  // CSS Filter overlay
  const filters: string[] = []
  if (brightness.value !== 0) filters.push(`brightness(${100 + brightness.value}%)`)
  if (contrast.value !== 0) filters.push(`contrast(${100 + contrast.value}%)`)
  if (saturation.value !== 0) filters.push(`saturate(${100 + saturation.value}%)`)
  if (blurPx.value > 0) filters.push(`blur(${blurPx.value}px)`)
  if (activeFilterPreset.value === 'sepia') filters.push('sepia(70%)')
  if (activeFilterPreset.value === 'invert') filters.push('invert(100%)')

  if (filters.length > 0) {
    const offscreen = document.createElement('canvas')
    offscreen.width = canvasRef.value.width
    offscreen.height = canvasRef.value.height
    const offCtx = offscreen.getContext('2d')
    if (offCtx) {
      offCtx.filter = filters.join(' ')
      offCtx.drawImage(canvasRef.value, 0, 0)
      ctx.clearRect(0, 0, canvasRef.value.width, canvasRef.value.height)
      ctx.drawImage(offscreen, 0, 0)
    }
  }
}

// Transforms (Rotate & Flip)
const rotateImage = (deg: number) => {
  if (!canvasRef.value) return
  const currentCanvas = canvasRef.value
  const offscreen = document.createElement('canvas')
  offscreen.width = currentCanvas.width
  offscreen.height = currentCanvas.height
  offscreen.getContext('2d')?.drawImage(currentCanvas, 0, 0)

  const newW = deg % 180 === 0 ? currentCanvas.width : currentCanvas.height
  const newH = deg % 180 === 0 ? currentCanvas.height : currentCanvas.width
  setupCanvas(newW, newH)

  const ctx = currentCanvas.getContext('2d')
  if (!ctx) return

  ctx.save()
  ctx.translate(newW / 2, newH / 2)
  ctx.rotate((deg * Math.PI) / 180)
  ctx.drawImage(offscreen, -offscreen.width / 2, -offscreen.height / 2)
  ctx.restore()

  saveHistory()
}

const flipImage = (dir: 'h' | 'v') => {
  if (!canvasRef.value) return
  const currentCanvas = canvasRef.value
  const offscreen = document.createElement('canvas')
  offscreen.width = currentCanvas.width
  offscreen.height = currentCanvas.height
  offscreen.getContext('2d')?.drawImage(currentCanvas, 0, 0)

  const ctx = currentCanvas.getContext('2d')
  if (!ctx) return

  ctx.save()
  if (dir === 'h') {
    ctx.translate(currentCanvas.width, 0)
    ctx.scale(-1, 1)
  } else {
    ctx.translate(0, currentCanvas.height)
    ctx.scale(1, -1)
  }
  ctx.drawImage(offscreen, 0, 0)
  ctx.restore()

  saveHistory()
}

// Crop functionality
const setCropRatio = (val: string) => {
  selectedCropRatio.value = val
  if (!canvasRef.value) return
  const w = canvasRef.value.width
  const h = canvasRef.value.height

  let targetRatio: number | null = null
  if (val === '1:1') targetRatio = 1
  else if (val === '4:3') targetRatio = 4 / 3
  else if (val === '16:9') targetRatio = 16 / 9
  else if (val === '3:2') targetRatio = 3 / 2
  else if (val === '9:16') targetRatio = 9 / 16

  if (targetRatio !== null) {
    let cropW = w * 0.85
    let cropH = cropW / targetRatio
    if (cropH > h * 0.85) {
      cropH = h * 0.85
      cropW = cropH * targetRatio
    }
    const cropX = Math.round((w - cropW) / 2)
    const cropY = Math.round((h - cropH) / 2)
    cropBox.value = { x: cropX, y: cropY, w: Math.round(cropW), h: Math.round(cropH) }
  } else {
    cropBox.value = { x: Math.round(w * 0.1), y: Math.round(h * 0.1), w: Math.round(w * 0.8), h: Math.round(h * 0.8) }
  }
}

// Crop drag and resize handlers
type CropDragHandle = 'move' | 'nw' | 'ne' | 'se' | 'sw' | 'n' | 's' | 'w' | 'e' | null
const activeCropHandle = ref<CropDragHandle>(null)
let cropStartMouseX = 0
let cropStartMouseY = 0
let cropInitBox = { x: 0, y: 0, w: 0, h: 0 }

const handleCropBoxMouseDown = (e: MouseEvent, handle: CropDragHandle) => {
  if (!cropBox.value || !canvasRef.value) return
  e.preventDefault()
  e.stopPropagation()
  activeCropHandle.value = handle
  cropStartMouseX = e.clientX
  cropStartMouseY = e.clientY
  cropInitBox = { ...cropBox.value }

  window.addEventListener('mousemove', onCropWindowMouseMove)
  window.addEventListener('mouseup', onCropWindowMouseUp)
}

const onCropWindowMouseMove = (e: MouseEvent) => {
  if (!activeCropHandle.value || !canvasRef.value || !cropBox.value) return
  e.preventDefault()
  const rect = canvasRef.value.getBoundingClientRect()
  if (rect.width <= 0 || rect.height <= 0) return
  const scaleX = canvasRef.value.width / rect.width
  const scaleY = canvasRef.value.height / rect.height

  const deltaX = (e.clientX - cropStartMouseX) * scaleX
  const deltaY = (e.clientY - cropStartMouseY) * scaleY
  const canvasW = canvasRef.value.width
  const canvasH = canvasRef.value.height

  if (activeCropHandle.value === 'move') {
    const maxX = Math.max(0, canvasW - cropInitBox.w)
    const maxY = Math.max(0, canvasH - cropInitBox.h)
    const newX = Math.round(Math.max(0, Math.min(maxX, cropInitBox.x + deltaX)))
    const newY = Math.round(Math.max(0, Math.min(maxY, cropInitBox.y + deltaY)))
    cropBox.value = {
      ...cropInitBox,
      x: newX,
      y: newY
    }
    return
  }

  let newX = cropInitBox.x
  let newY = cropInitBox.y
  let newW = cropInitBox.w
  let newH = cropInitBox.h

  const handle = activeCropHandle.value
  if (handle.includes('e')) {
    newW = Math.max(20, Math.min(canvasW - newX, cropInitBox.w + deltaX))
  }
  if (handle.includes('s')) {
    newH = Math.max(20, Math.min(canvasH - newY, cropInitBox.h + deltaY))
  }
  if (handle.includes('w')) {
    const maxDelta = cropInitBox.w - 20
    const clampedDelta = Math.max(-cropInitBox.x, Math.min(maxDelta, deltaX))
    newX = cropInitBox.x + clampedDelta
    newW = cropInitBox.w - clampedDelta
  }
  if (handle.includes('n')) {
    const maxDelta = cropInitBox.h - 20
    const clampedDelta = Math.max(-cropInitBox.y, Math.min(maxDelta, deltaY))
    newY = cropInitBox.y + clampedDelta
    newH = cropInitBox.h - clampedDelta
  }

  let targetRatio: number | null = null
  if (selectedCropRatio.value === '1:1') targetRatio = 1
  else if (selectedCropRatio.value === '4:3') targetRatio = 4 / 3
  else if (selectedCropRatio.value === '16:9') targetRatio = 16 / 9
  else if (selectedCropRatio.value === '3:2') targetRatio = 3 / 2
  else if (selectedCropRatio.value === '9:16') targetRatio = 9 / 16

  if (targetRatio !== null) {
    if (handle === 'n' || handle === 's') {
      newW = newH * targetRatio
    } else {
      newH = newW / targetRatio
    }
  }

  cropBox.value = {
    x: Math.round(newX),
    y: Math.round(newY),
    w: Math.round(newW),
    h: Math.round(newH)
  }
}

const onCropWindowMouseUp = () => {
  activeCropHandle.value = null
  window.removeEventListener('mousemove', onCropWindowMouseMove)
  window.removeEventListener('mouseup', onCropWindowMouseUp)
}

const applyCrop = () => {
  if (!canvasRef.value) return
  const w = canvasRef.value.width
  const h = canvasRef.value.height

  let box = cropBox.value
  if (!box || box.w <= 0 || box.h <= 0) {
    setCropRatio(selectedCropRatio.value)
    box = cropBox.value
  }
  if (!box) return

  const sx = Math.max(0, Math.min(w, box.x))
  const sy = Math.max(0, Math.min(h, box.y))
  const sw = Math.max(1, Math.min(w - sx, box.w))
  const sh = Math.max(1, Math.min(h - sy, box.h))

  const offscreen = document.createElement('canvas')
  offscreen.width = sw
  offscreen.height = sh
  const offCtx = offscreen.getContext('2d')
  if (!offCtx) return

  offCtx.drawImage(canvasRef.value, sx, sy, sw, sh, 0, 0, sw, sh)

  setupCanvas(sw, sh)
  const ctx = canvasRef.value.getContext('2d')
  if (ctx) {
    ctx.clearRect(0, 0, sw, sh)
    ctx.drawImage(offscreen, 0, 0)
    cropBox.value = null
    saveHistory()
    message.success(t('editor.cropApplied'))
  }
}

// Mouse coords calculation
const getCanvasCoords = (e: MouseEvent) => {
  if (!canvasRef.value) return { x: 0, y: 0 }
  const rect = canvasRef.value.getBoundingClientRect()
  const scaleX = canvasRef.value.width / rect.width
  const scaleY = canvasRef.value.height / rect.height
  return {
    x: Math.max(0, Math.min(canvasRef.value.width, (e.clientX - rect.left) * scaleX)),
    y: Math.max(0, Math.min(canvasRef.value.height, (e.clientY - rect.top) * scaleY))
  }
}

const handleMouseDown = (e: MouseEvent) => {
  if (!canvasRef.value) return
  const coords = getCanvasCoords(e)
  startX = coords.x
  startY = coords.y

  if (activeToolTab.value === 'crop') {
    isCroppingDrag.value = true
    return
  }

  if (activeToolTab.value !== 'draw') return
  isDrawing = true

  const ctx = canvasRef.value.getContext('2d')
  if (ctx) {
    snapshotData = ctx.getImageData(0, 0, canvasRef.value.width, canvasRef.value.height)
    if (drawTool.value === 'brush') {
      ctx.beginPath()
      ctx.moveTo(coords.x, coords.y)
    } else if (drawTool.value === 'mosaic' && mosaicBlurMode.value === 'brush') {
      applyMosaicBlock(ctx, coords.x, coords.y, strokeWidth.value * 2)
    } else if (drawTool.value === 'blur' && mosaicBlurMode.value === 'brush') {
      applyBlurBrush(ctx, coords.x, coords.y, strokeWidth.value * 2)
    }
  }
}

const handleMouseMove = (e: MouseEvent) => {
  if (!canvasRef.value) return
  const coords = getCanvasCoords(e)

  if (activeToolTab.value === 'crop' && isCroppingDrag.value) {
    let w = coords.x - startX
    let h = coords.y - startY

    // Apply ratio constraints if not free
    if (selectedCropRatio.value === '1:1') {
      const s = Math.min(Math.abs(w), Math.abs(h))
      w = (w >= 0 ? 1 : -1) * s
      h = (h >= 0 ? 1 : -1) * s
    } else if (selectedCropRatio.value === '4:3') {
      const r = 4 / 3
      h = (h >= 0 ? 1 : -1) * (Math.abs(w) / r)
    } else if (selectedCropRatio.value === '16:9') {
      const r = 16 / 9
      h = (h >= 0 ? 1 : -1) * (Math.abs(w) / r)
    } else if (selectedCropRatio.value === '3:2') {
      const r = 3 / 2
      h = (h >= 0 ? 1 : -1) * (Math.abs(w) / r)
    } else if (selectedCropRatio.value === '9:16') {
      const r = 9 / 16
      h = (h >= 0 ? 1 : -1) * (Math.abs(w) / r)
    }

    const x = w >= 0 ? startX : startX + w
    const y = h >= 0 ? startY : startY + h
    cropBox.value = {
      x: Math.round(Math.max(0, x)),
      y: Math.round(Math.max(0, y)),
      w: Math.round(Math.min(canvasRef.value.width - x, Math.abs(w))),
      h: Math.round(Math.min(canvasRef.value.height - y, Math.abs(h)))
    }
    return
  }

  if (!isDrawing || activeToolTab.value !== 'draw') return
  const ctx = canvasRef.value.getContext('2d')
  if (!ctx) return

  if (drawTool.value === 'brush') {
    ctx.strokeStyle = strokeColor.value
    ctx.lineWidth = strokeWidth.value
    ctx.lineCap = 'round'
    ctx.lineJoin = 'round'
    ctx.lineTo(coords.x, coords.y)
    ctx.stroke()
  } else if (drawTool.value === 'mosaic' && mosaicBlurMode.value === 'brush') {
    applyMosaicBlock(ctx, coords.x, coords.y, strokeWidth.value * 2)
  } else if (drawTool.value === 'blur' && mosaicBlurMode.value === 'brush') {
    applyBlurBrush(ctx, coords.x, coords.y, strokeWidth.value * 2)
  } else if (snapshotData) {
    ctx.putImageData(snapshotData, 0, 0)
    ctx.strokeStyle = strokeColor.value
    ctx.lineWidth = strokeWidth.value

    if (drawTool.value === 'rect') {
      ctx.strokeRect(startX, startY, coords.x - startX, coords.y - startY)
    } else if (drawTool.value === 'roundRect') {
      const rx = Math.min(startX, coords.x)
      const ry = Math.min(startY, coords.y)
      const rw = Math.abs(coords.x - startX)
      const rh = Math.abs(coords.y - startY)
      const radius = Math.min(14, rw / 2, rh / 2)
      ctx.beginPath()
      if ((ctx as any).roundRect) {
        (ctx as any).roundRect(rx, ry, rw, rh, radius)
      } else {
        ctx.strokeRect(rx, ry, rw, rh)
      }
      ctx.stroke()
    } else if (drawTool.value === 'circle') {
      const rx = Math.abs(coords.x - startX) / 2
      const ry = Math.abs(coords.y - startY) / 2
      const cx = (startX + coords.x) / 2
      const cy = (startY + coords.y) / 2
      ctx.beginPath()
      ctx.ellipse(cx, cy, rx, ry, 0, 0, Math.PI * 2)
      ctx.stroke()
    } else if (drawTool.value === 'arrow') {
      drawArrow(ctx, startX, startY, coords.x, coords.y, strokeWidth.value)
    } else if (drawTool.value === 'mosaic' || drawTool.value === 'blur') {
      // Box selection outline preview
      ctx.save()
      ctx.setLineDash([6, 6])
      ctx.strokeStyle = '#63e2b7'
      ctx.lineWidth = 2
      ctx.strokeRect(startX, startY, coords.x - startX, coords.y - startY)
      ctx.restore()
    }
  }
}

const handleMouseUp = (e: MouseEvent) => {
  if (isCroppingDrag.value) {
    isCroppingDrag.value = false
    if (!cropBox.value || cropBox.value.w < 20 || cropBox.value.h < 20) {
      setCropRatio(selectedCropRatio.value)
    }
    return
  }

  if (!isDrawing) return
  isDrawing = false

  const ctx = canvasRef.value?.getContext('2d')
  if (!ctx || !canvasRef.value) return

  const coords = getCanvasCoords(e)

  if (drawTool.value === 'text' && annotationText.value) {
    ctx.font = `${Math.max(16, strokeWidth.value * 4)}px sans-serif`
    ctx.fillStyle = strokeColor.value
    ctx.fillText(annotationText.value, coords.x, coords.y)
  } else if ((drawTool.value === 'mosaic' || drawTool.value === 'blur') && mosaicBlurMode.value === 'box') {
    if (snapshotData) {
      ctx.putImageData(snapshotData, 0, 0)
    }
    const bx = Math.min(startX, coords.x)
    const by = Math.min(startY, coords.y)
    const bw = Math.abs(coords.x - startX)
    const bh = Math.abs(coords.y - startY)
    if (bw > 2 && bh > 2) {
      if (drawTool.value === 'mosaic') {
        applyMosaicArea(ctx, bx, by, bw, bh, 12)
      } else {
        applyBlurArea(ctx, bx, by, bw, bh, 8)
      }
    }
  }

  snapshotData = null
  saveHistory()
}

// Mosaic & Blur Renderers
const applyMosaicBlock = (ctx: CanvasRenderingContext2D, cx: number, cy: number, size: number) => {
  const blockSize = 10
  const x0 = Math.max(0, Math.round(cx - size / 2))
  const y0 = Math.max(0, Math.round(cy - size / 2))
  const x1 = Math.min(ctx.canvas.width, x0 + size)
  const y1 = Math.min(ctx.canvas.height, y0 + size)

  applyMosaicArea(ctx, x0, y0, x1 - x0, y1 - y0, blockSize)
}

const applyMosaicArea = (ctx: CanvasRenderingContext2D, x: number, y: number, w: number, h: number, blockSize: number = 10) => {
  if (w <= 0 || h <= 0) return
  const imgData = ctx.getImageData(x, y, w, h)
  for (let py = 0; py < h; py += blockSize) {
    for (let px = 0; px < w; px += blockSize) {
      const sampleX = Math.min(px + Math.floor(blockSize / 2), w - 1)
      const sampleY = Math.min(py + Math.floor(blockSize / 2), h - 1)
      const idx = (sampleY * w + sampleX) * 4
      const r = imgData.data[idx]
      const g = imgData.data[idx + 1]
      const b = imgData.data[idx + 2]

      for (let dy = 0; dy < blockSize && py + dy < h; dy++) {
        for (let dx = 0; dx < blockSize && px + dx < w; dx++) {
          const pIdx = ((py + dy) * w + (px + dx)) * 4
          imgData.data[pIdx] = r
          imgData.data[pIdx + 1] = g
          imgData.data[pIdx + 2] = b
        }
      }
    }
  }
  ctx.putImageData(imgData, x, y)
}

const applyBlurBrush = (ctx: CanvasRenderingContext2D, cx: number, cy: number, size: number) => {
  const radius = Math.max(8, Math.round(size / 2))
  const x0 = Math.max(0, Math.round(cx - radius))
  const y0 = Math.max(0, Math.round(cy - radius))
  const w = Math.min(ctx.canvas.width - x0, radius * 2)
  const h = Math.min(ctx.canvas.height - y0, radius * 2)
  applyBlurArea(ctx, x0, y0, w, h, 6)
}

const applyBlurArea = (ctx: CanvasRenderingContext2D, x: number, y: number, w: number, h: number, radius: number = 8) => {
  if (w <= 0 || h <= 0) return
  const offscreen = document.createElement('canvas')
  offscreen.width = w
  offscreen.height = h
  const offCtx = offscreen.getContext('2d')
  if (!offCtx) return
  offCtx.drawImage(ctx.canvas, x, y, w, h, 0, 0, w, h)
  offCtx.filter = `blur(${radius}px)`
  offCtx.drawImage(offscreen, 0, 0)
  ctx.drawImage(offscreen, 0, 0, w, h, x, y, w, h)
}

// Arrow drawing fix: terminate the shaft line flush at the base of the arrowhead
const drawArrow = (ctx: CanvasRenderingContext2D, fromX: number, fromY: number, toX: number, toY: number, width: number) => {
  const headlen = Math.max(16, width * 3.5)
  const dx = toX - fromX
  const dy = toY - fromY
  const angle = Math.atan2(dy, dx)
  const dist = Math.hypot(dx, dy)

  // Shaft must stop at the base of the arrowhead
  const baseOffset = Math.min(dist * 0.9, headlen * Math.cos(Math.PI / 6))
  const shaftEndX = toX - baseOffset * Math.cos(angle)
  const shaftEndY = toY - baseOffset * Math.sin(angle)

  ctx.beginPath()
  ctx.moveTo(fromX, fromY)
  ctx.lineTo(shaftEndX, shaftEndY)
  ctx.lineWidth = width
  ctx.lineCap = 'butt' // Prevent rounded cap from poking through
  ctx.stroke()

  ctx.beginPath()
  ctx.moveTo(toX, toY)
  ctx.lineTo(toX - headlen * Math.cos(angle - Math.PI / 6), toY - headlen * Math.sin(angle - Math.PI / 6))
  ctx.lineTo(toX - headlen * Math.cos(angle + Math.PI / 6), toY - headlen * Math.sin(angle + Math.PI / 6))
  ctx.closePath()
  ctx.fillStyle = ctx.strokeStyle
  ctx.fill()
}

// Save logic
const handleSave = async (mode: 'copy' | 'overwrite') => {
  if (!canvasRef.value || !props.file) return

  // Automatically apply crop box if currently active
  if (activeToolTab.value === 'crop' && cropBox.value && cropBox.value.w > 20 && cropBox.value.h > 20) {
    applyCrop()
  }

  if (mode === 'copy') {
    savingCopy.value = true
  } else {
    savingOverwrite.value = true
  }

  try {
    const dataUrl = canvasRef.value.toDataURL('image/png', 0.95)
    const res = await api.post(`/files/${props.file.id}/edit`, {
      image_data: dataUrl,
      mode: mode,
      display_name: props.file.display_name
    })

    message.success(mode === 'copy' ? t('editor.saveCopySuccess') : t('editor.overwriteSuccess'))
    visible.value = false
    emit('saved', res.data.file)
  } catch (err: any) {
    message.error(err.response?.data?.error || t('editor.saveFailed'))
  } finally {
    savingCopy.value = false
    savingOverwrite.value = false
  }
}
</script>
