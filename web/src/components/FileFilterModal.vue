<template>
  <n-modal
    v-model:show="visible"
    preset="card"
    :title="$t('filter.advancedTitle')"
    style="width: 860px; max-width: 95vw; border-radius: 12px;"
    :segmented="{ content: 'soft', footer: 'soft' }"
  >
    <!-- Header: Quick Presets with Tooltips -->
    <div style="display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin-bottom: 14px; padding-bottom: 12px; border-bottom: 1px solid rgba(255, 255, 255, 0.06);">
      <span style="font-size: 12px; color: #888; margin-right: 4px;">{{ $t('filter.presets') }}:</span>
      <n-tooltip v-for="p in presetButtons" :key="p.key" trigger="hover">
        <template #trigger>
          <n-button
            size="tiny"
            quaternary
            round
            style="background: rgba(255, 255, 255, 0.04); font-size: 11px;"
            @click="applyPreset(p.rule)"
          >
            + {{ p.label }}
          </n-button>
        </template>
        {{ p.tip || p.label }}
      </n-tooltip>
    </div>

    <!-- Rule Rows Container with Drag and Connectors -->
    <div class="filter-rules-scroll-area">
      <div v-if="form.rules.length === 0" style="text-align: center; color: #777; padding: 42px 0; font-size: 13px; background: rgba(255,255,255,0.01); border-radius: 8px; border: 1px dashed rgba(255,255,255,0.08);">
        {{ $t('filter.emptyTip') }}
      </div>

      <template v-for="(rule, index) in form.rules" :key="rule._uid || index">
        <!-- Connector between Row index-1 and Row index -->
        <div v-if="index > 0" class="rule-connector-container">
          <div class="connector-divider-line"></div>
          <n-tooltip trigger="hover">
            <template #trigger>
              <n-select
                v-model:value="form.rules[index - 1].connector"
                :options="connectorOptions"
                size="tiny"
                class="connector-select"
                style="width: 85px;"
              />
            </template>
            {{ getConnectorTip(form.rules[index - 1].connector) }}
          </n-tooltip>
          <div class="connector-divider-line"></div>
        </div>

        <!-- Rule Card Item -->
        <div
          class="rule-row-card"
          :class="{
            'is-dragging': draggedIndex === index,
            'is-dragover': dragOverIndex === index
          }"
          draggable="true"
          @dragstart="onDragStart(index, $event)"
          @dragover.prevent="onDragOver(index)"
          @dragleave="onDragLeave(index)"
          @drop.prevent="onDrop(index)"
          @dragend="onDragEnd"
        >
          <!-- Drag Handle -->
          <div class="rule-drag-handle" :title="$t('filter.dragTip')">
            <n-icon :component="ReorderTwoOutline" size="18" />
          </div>

          <!-- Left Parenthesis Selector -->
          <n-tooltip trigger="hover">
            <template #trigger>
              <n-select
                v-model:value="rule.prefix_parens"
                :options="leftParenOptions"
                size="small"
                style="width: 58px;"
                placeholder="("
              />
            </template>
            {{ $t('filter.parenLeft') }}
          </n-tooltip>

          <!-- Field select -->
          <n-select
            v-model:value="rule.field"
            :options="fieldOptions"
            size="small"
            style="width: 125px;"
            @update:value="() => onFieldChange(rule)"
          />

          <!-- Operator select -->
          <n-select
            v-model:value="rule.op"
            :options="getOpOptions(rule.field)"
            size="small"
            style="width: 110px;"
            @update:value="() => onOpChange(rule)"
          />

          <!-- Dynamic Value Controls -->
          <div style="flex: 1 1 180px; min-width: 150px; display: flex; align-items: center; gap: 6px;">
            <!-- 1. Format Select -->
            <template v-if="rule.field === 'format'">
              <n-select
                v-if="rule.op === 'in' || rule.op === 'not_in'"
                v-model:value="rule.value"
                multiple
                :options="formatValueOptions"
                size="small"
                style="width: 100%;"
              />
              <n-select
                v-else
                v-model:value="rule.value"
                :options="formatValueOptions"
                size="small"
                style="width: 100%;"
              />
            </template>

            <!-- 2. Watermark Select -->
            <template v-else-if="rule.field === 'watermark_mode'">
              <n-select
                v-model:value="rule.value"
                :options="watermarkValueOptions"
                size="small"
                style="width: 100%;"
              />
            </template>

            <!-- 3. Hotlink Select -->
            <template v-else-if="rule.field === 'hotlink_mode'">
              <n-select
                v-model:value="rule.value"
                :options="hotlinkValueOptions"
                size="small"
                style="width: 100%;"
              />
            </template>

            <!-- 4. Policy Select -->
            <template v-else-if="rule.field === 'policy_id'">
              <n-select
                v-if="rule.op === 'in' || rule.op === 'not_in'"
                v-model:value="rule.value"
                multiple
                :options="policyOptions"
                size="small"
                style="width: 100%;"
              />
              <n-select
                v-else
                v-model:value="rule.value"
                :options="policyOptions"
                size="small"
                style="width: 100%;"
              />
            </template>

            <!-- 5. Size Inputs -->
            <template v-else-if="rule.field === 'size'">
              <template v-if="rule.op === 'between'">
                <n-input-number v-model:value="rule.value" :min="0" size="small" style="flex: 1;" />
                <span style="color: #888;">-</span>
                <n-input-number v-model:value="rule.value_to" :min="0" size="small" style="flex: 1;" />
              </template>
              <template v-else>
                <n-input-number v-model:value="rule.value" :min="0" size="small" style="flex: 1;" />
              </template>
              <n-select v-model:value="rule.unit" :options="sizeUnitOptions" size="small" style="width: 68px;" />
            </template>

            <!-- 6. Dimensions -->
            <template v-else-if="rule.field === 'dimensions'">
              <template v-if="['landscape', 'portrait', 'square'].includes(rule.op)">
                <span style="font-size: 12px; color: #aaa;">{{ getAspectLabel(rule.op) }}</span>
              </template>
              <template v-else-if="rule.op.endsWith('between')">
                <n-input-number v-model:value="rule.value" :min="0" size="small" style="flex: 1;" placeholder="Min" />
                <span style="color: #888;">-</span>
                <n-input-number v-model:value="rule.value_to" :min="0" size="small" style="flex: 1;" placeholder="Max" />
                <span style="font-size: 12px; color: #888;">px</span>
              </template>
              <template v-else>
                <n-input-number v-model:value="rule.value" :min="0" size="small" style="flex: 1;" placeholder="px" />
                <span style="font-size: 12px; color: #888;">px</span>
              </template>
            </template>

            <!-- 7. Is Image -->
            <template v-else-if="rule.field === 'is_image'">
              <span style="font-size: 12px; color: #aaa;">
                {{ rule.op === 'image' ? $t('filter.presetImages') : $t('filter.presetNonImages') }}
              </span>
            </template>

            <!-- 8. Created At -->
            <template v-else-if="rule.field === 'created_at'">
              <template v-if="rule.op === 'between'">
                <n-date-picker
                  v-model:value="rule.date_range"
                  type="daterange"
                  size="small"
                  style="width: 100%;"
                  clearable
                  @update:value="(val: any) => handleDateRangeUpdate(rule, val)"
                />
              </template>
              <template v-else-if="rule.op === 'after' || rule.op === 'before'">
                <n-date-picker
                  v-model:value="rule.value"
                  type="date"
                  size="small"
                  style="width: 100%;"
                  clearable
                />
              </template>
              <template v-else>
                <span style="font-size: 12px; color: #aaa;">{{ getTimePresetLabel(rule.op) }}</span>
              </template>
            </template>

            <!-- 9. Temp Links -->
            <template v-else-if="rule.field === 'temp_links'">
              <span style="font-size: 12px; color: #aaa;">{{ getTempLinkLabel(rule.op) }}</span>
            </template>

            <!-- 10. Calls / UV -->
            <template v-else-if="rule.field === 'calls' || rule.field === 'uv'">
              <template v-if="rule.op === 'is_zero' || rule.op === 'gt_zero'">
                <span style="font-size: 12px; color: #aaa;">
                  {{ rule.op === 'gt_zero' ? $t('filter.opGtZero') : $t('filter.opIsZero') }}
                </span>
              </template>
              <template v-else-if="rule.op === 'between'">
                <n-input-number v-model:value="rule.value" :min="0" size="small" style="flex: 1;" />
                <span style="color: #888;">-</span>
                <n-input-number v-model:value="rule.value_to" :min="0" size="small" style="flex: 1;" />
              </template>
              <template v-else>
                <n-input-number v-model:value="rule.value" :min="0" size="small" style="flex: 1;" />
              </template>
            </template>

            <!-- Default Text Inputs -->
            <template v-else>
              <n-input
                v-model:value="rule.value"
                size="small"
                clearable
                style="width: 100%;"
              />
            </template>
          </div>

          <!-- Right Parenthesis Selector -->
          <n-tooltip trigger="hover">
            <template #trigger>
              <n-select
                v-model:value="rule.suffix_parens"
                :options="rightParenOptions"
                size="small"
                style="width: 58px;"
                placeholder=")"
              />
            </template>
            {{ $t('filter.parenRight') }}
          </n-tooltip>

          <!-- Delete Action -->
          <n-button
            circle
            size="tiny"
            quaternary
            type="error"
            @click="removeRule(index)"
          >
            <template #icon>
              <n-icon :component="TrashOutline" />
            </template>
          </n-button>
        </div>
      </template>
    </div>

    <!-- Add Condition Button -->
    <n-button dashed block size="small" style="margin: 12px 0 8px;" @click="addRule">
      <template #icon>
        <n-icon :component="AddOutline" />
      </template>
      {{ $t('filter.addCondition') }}
    </n-button>

    <!-- Expression Live Preview & Parentheses Validation Alert -->
    <div v-if="form.rules.length > 0" style="margin-top: 10px;">
      <div v-if="!parenCounts.isBalanced" style="margin-bottom: 8px;">
        <n-alert type="warning" size="small" :bordered="false">
          {{ $t('filter.parenMismatch', { left: parenCounts.left, right: parenCounts.right }) }}
        </n-alert>
      </div>

      <div class="expression-preview-card">
        <span class="preview-label">{{ $t('filter.expressionPreview') }}:</span>
        <code class="preview-code">{{ expressionPreviewText }}</code>
      </div>
    </div>

    <!-- Modal Footer -->
    <template #footer>
      <div style="display: flex; align-items: center; justify-content: space-between;">
        <n-button size="small" quaternary type="warning" @click="handleReset">
          {{ $t('filter.reset') }}
        </n-button>
        <div style="display: flex; gap: 8px;">
          <n-button size="small" @click="visible = false">
            {{ $t('common.cancel') }}
          </n-button>
          <n-button
            size="small"
            type="primary"
            :disabled="!canApply"
            @click="handleApply"
          >
            {{ $t('filter.apply') }} ({{ validRulesCount }})
          </n-button>
        </div>
      </div>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { AddOutline, TrashOutline, ReorderTwoOutline } from '@vicons/ionicons5'
import { formatFilterRuleLabel } from '../utils/filterLabel'

export interface FilterRule {
  _uid?: string
  prefix_parens?: string
  field: string
  op: string
  value?: any
  value_to?: any
  unit?: string
  sub_type?: string
  suffix_parens?: string
  connector?: string // 'AND' | 'OR' | 'NOT'
  negate?: boolean
  date_range?: [number, number] | null
}

export interface AdvancedFilterPayload {
  logic?: 'and' | 'or'
  rules: FilterRule[]
}

const props = defineProps<{
  show: boolean
  isAdmin?: boolean
  policies?: Array<{ label: string; value: string | number }>
  initialPayload?: AdvancedFilterPayload
}>()

const emit = defineEmits<{
  (e: 'update:show', val: boolean): void
  (e: 'apply', payload: AdvancedFilterPayload): void
  (e: 'reset'): void
}>()

const { t } = useI18n()

const visible = computed({
  get: () => props.show,
  set: (val: boolean) => emit('update:show', val)
})

const form = ref<AdvancedFilterPayload>({
  logic: 'and',
  rules: []
})

const createUid = () => 'r_' + Date.now() + '_' + Math.random().toString(36).slice(2, 7)

// Synchronize initial payload
watch(
  () => props.show,
  (open) => {
    if (open) {
      if (props.initialPayload && props.initialPayload.rules) {
        const copy: AdvancedFilterPayload = JSON.parse(JSON.stringify(props.initialPayload))
        copy.rules = (copy.rules || []).map(r => ({
          ...r,
          _uid: r._uid || createUid(),
          prefix_parens: r.prefix_parens || '',
          suffix_parens: r.suffix_parens || '',
          connector: r.connector || 'AND'
        }))
        form.value = copy
      } else {
        form.value = { logic: 'and', rules: [] }
      }
    }
  },
  { immediate: true }
)

// Drag and drop state
const draggedIndex = ref<number | null>(null)
const dragOverIndex = ref<number | null>(null)

const onDragStart = (idx: number, e: DragEvent) => {
  draggedIndex.value = idx
  if (e.dataTransfer) {
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', String(idx))
  }
}

const onDragOver = (idx: number) => {
  if (draggedIndex.value !== null && draggedIndex.value !== idx) {
    dragOverIndex.value = idx
  }
}

const onDragLeave = (idx: number) => {
  if (dragOverIndex.value === idx) {
    dragOverIndex.value = null
  }
}

const onDrop = (idx: number) => {
  if (draggedIndex.value !== null && draggedIndex.value !== idx) {
    const moved = form.value.rules.splice(draggedIndex.value, 1)[0]
    form.value.rules.splice(idx, 0, moved)
  }
  draggedIndex.value = null
  dragOverIndex.value = null
}

const onDragEnd = () => {
  draggedIndex.value = null
  dragOverIndex.value = null
}

// Connectors & Parentheses options
const connectorOptions = computed(() => [
  { label: 'AND', value: 'AND' },
  { label: 'OR', value: 'OR' },
  { label: 'NOT', value: 'NOT' }
])

const getConnectorTip = (conn?: string) => {
  switch ((conn || 'AND').toUpperCase()) {
  case 'OR': return t('filter.connOrTip')
  case 'NOT': return t('filter.connNotTip')
  default: return t('filter.connAndTip')
  }
}

const leftParenOptions = computed(() => [
  { label: t('filter.parenNone'), value: '' },
  { label: '(', value: '(' },
  { label: '((', value: '((' },
  { label: '(((', value: '(((' }
])

const rightParenOptions = computed(() => [
  { label: t('filter.parenNone'), value: '' },
  { label: ')', value: ')' },
  { label: '))', value: '))' },
  { label: ')))', value: ')))' }
])

// Parentheses balance check
const parenCounts = computed(() => {
  let left = 0
  let right = 0
  for (const r of form.value.rules) {
    if (r.prefix_parens) {
      left += (r.prefix_parens.match(/\(/g) || []).length
    }
    if (r.suffix_parens) {
      right += (r.suffix_parens.match(/\)/g) || []).length
    }
  }
  return { left, right, isBalanced: left === right }
})

const fieldOptions = computed(() => {
  const list = [
    { label: t('filter.fieldFormat'), value: 'format' },
    { label: t('filter.fieldWatermark'), value: 'watermark_mode' },
    { label: t('filter.fieldHotlink'), value: 'hotlink_mode' },
    { label: t('filter.fieldPolicy'), value: 'policy_id' },
    { label: t('filter.fieldSize'), value: 'size' },
    { label: t('filter.fieldDimensions'), value: 'dimensions' },
    { label: t('filter.fieldType'), value: 'is_image' },
    { label: t('filter.fieldTime'), value: 'created_at' },
    { label: t('filter.fieldTempLinks'), value: 'temp_links' },
    { label: t('filter.fieldCalls'), value: 'calls' },
    { label: t('filter.fieldUV'), value: 'uv' },
    { label: t('filter.fieldName'), value: 'display_name' },
    { label: t('filter.fieldPath'), value: 'public_path' },
    { label: t('filter.fieldSha256'), value: 'sha256' },
  ]
  if (props.isAdmin) {
    list.push({ label: t('filter.fieldUploader'), value: 'uploader' })
  }
  return list
})

const formatValueOptions = computed(() => [
  { label: 'JPEG / JPG', value: 'jpg' },
  { label: 'PNG', value: 'png' },
  { label: 'WebP', value: 'webp' },
  { label: 'GIF', value: 'gif' },
  { label: 'SVG', value: 'svg' },
  { label: 'ICO', value: 'ico' },
  { label: 'HEIF / HEIC', value: 'heif' },
  { label: t('files.formatOther'), value: 'other' }
])

const watermarkValueOptions = computed(() => [
  { label: t('upload.watermarkInherit'), value: 'inherit' },
  { label: t('upload.watermarkOn'), value: 'on' },
  { label: t('upload.watermarkOff'), value: 'off' }
])

const hotlinkValueOptions = computed(() => [
  { label: t('files.hotlinkInherit'), value: 'inherit' },
  { label: t('files.hotlinkOn'), value: 'on' },
  { label: t('files.hotlinkOff'), value: 'off' }
])

const policyOptions = computed(() => {
  if (props.policies && props.policies.length > 0) {
    return props.policies.filter(p => p.value !== 'all')
  }
  return []
})

const sizeUnitOptions = [
  { label: 'MB', value: 'MB' },
  { label: 'KB', value: 'KB' },
  { label: 'GB', value: 'GB' },
  { label: 'B', value: 'B' }
]

const getOpOptions = (field: string) => {
  switch (field) {
  case 'format':
  case 'policy_id':
    return [
      { label: t('filter.opIs'), value: 'eq' },
      { label: t('filter.opIsNot'), value: 'neq' },
      { label: t('filter.opIn'), value: 'in' },
      { label: t('filter.opNotIn'), value: 'not_in' }
    ]
  case 'watermark_mode':
  case 'hotlink_mode':
    return [
      { label: t('filter.opIs'), value: 'eq' },
      { label: t('filter.opIsNot'), value: 'neq' }
    ]
  case 'size':
    return [
      { label: t('filter.opGt'), value: 'gt' },
      { label: t('filter.opGte'), value: 'gte' },
      { label: t('filter.opLt'), value: 'lt' },
      { label: t('filter.opLte'), value: 'lte' },
      { label: t('filter.opBetween'), value: 'between' }
    ]
  case 'dimensions':
    return [
      { label: t('filter.opLandscape'), value: 'landscape' },
      { label: t('filter.opPortrait'), value: 'portrait' },
      { label: t('filter.opSquare'), value: 'square' },
      { label: t('filter.opWidth') + ' >', value: 'width_gt' },
      { label: t('filter.opWidth') + ' <', value: 'width_lt' },
      { label: t('filter.opWidth') + ' ' + t('filter.opBetween'), value: 'width_between' },
      { label: t('filter.opHeight') + ' >', value: 'height_gt' },
      { label: t('filter.opHeight') + ' <', value: 'height_lt' },
      { label: t('filter.opHeight') + ' ' + t('filter.opBetween'), value: 'height_between' }
    ]
  case 'is_image':
    return [
      { label: t('filter.presetImages'), value: 'image' },
      { label: t('filter.presetNonImages'), value: 'non_image' }
    ]
  case 'created_at':
    return [
      { label: t('filter.timeToday'), value: 'today' },
      { label: t('filter.timeYesterday'), value: 'yesterday' },
      { label: t('filter.time7Days'), value: '7d' },
      { label: t('filter.time30Days'), value: '30d' },
      { label: t('filter.timeThisMonth'), value: 'this_month' },
      { label: t('filter.opCustomTime'), value: 'between' },
      { label: t('filter.opGte'), value: 'after' },
      { label: t('filter.opLte'), value: 'before' }
    ]
  case 'temp_links':
    return [
      { label: t('filter.opHasActive'), value: 'has_active' },
      { label: t('filter.opHasAny'), value: 'has_any' },
      { label: t('filter.opNoTemp'), value: 'none' }
    ]
  case 'calls':
  case 'uv':
    return [
      { label: t('filter.opGtZero'), value: 'gt_zero' },
      { label: t('filter.opIsZero'), value: 'is_zero' },
      { label: t('filter.opGt'), value: 'gt' },
      { label: t('filter.opGte'), value: 'gte' },
      { label: t('filter.opLt'), value: 'lt' },
      { label: t('filter.opLte'), value: 'lte' },
      { label: t('filter.opBetween'), value: 'between' }
    ]
  case 'display_name':
  case 'public_path':
    return [
      { label: t('filter.opContains'), value: 'contains' },
      { label: t('filter.opNotContains'), value: 'not_contains' },
      { label: t('filter.opStartsWith'), value: 'starts_with' },
      { label: t('filter.opEndsWith'), value: 'ends_with' },
      { label: t('filter.opIs'), value: 'eq' }
    ]
  case 'sha256':
    return [
      { label: t('filter.opIs'), value: 'eq' },
      { label: t('filter.opStartsWith'), value: 'starts_with' }
    ]
  case 'uploader':
    return [
      { label: t('filter.opContains'), value: 'contains' },
      { label: t('filter.opNotContains'), value: 'not_contains' },
      { label: t('filter.opIs') + ' ID', value: 'eq' }
    ]
  default:
    return [{ label: t('filter.opIs'), value: 'eq' }]
  }
}

const onFieldChange = (rule: FilterRule) => {
  const ops = getOpOptions(rule.field)
  rule.op = ops[0]?.value || 'eq'
  rule.value = ''
  rule.value_to = ''
  rule.unit = 'MB'
  rule.sub_type = ''
  rule.date_range = null

  if (rule.field === 'format') {
    rule.value = 'jpg'
  } else if (rule.field === 'watermark_mode') {
    rule.value = 'on'
  } else if (rule.field === 'hotlink_mode') {
    rule.value = 'on'
  } else if (rule.field === 'policy_id' && policyOptions.value.length > 0) {
    rule.value = policyOptions.value[0].value
  } else if (rule.field === 'size') {
    rule.value = 5
    rule.unit = 'MB'
  } else if (rule.field === 'dimensions') {
    rule.sub_type = 'aspect_ratio'
    rule.op = 'landscape'
  } else if (rule.field === 'calls' || rule.field === 'uv') {
    rule.op = 'gt_zero'
  } else if (rule.field === 'temp_links') {
    rule.op = 'has_active'
  } else if (rule.field === 'is_image') {
    rule.op = 'image'
  } else if (rule.field === 'created_at') {
    rule.op = 'today'
  }
}

const onOpChange = (rule: FilterRule) => {
  if (rule.field === 'dimensions') {
    if (['landscape', 'portrait', 'square'].includes(rule.op)) {
      rule.sub_type = 'aspect_ratio'
    } else if (rule.op.startsWith('width_')) {
      rule.sub_type = 'width'
      rule.op = rule.op.replace('width_', '')
    } else if (rule.op.startsWith('height_')) {
      rule.sub_type = 'height'
      rule.op = rule.op.replace('height_', '')
    }
  }
}

const handleDateRangeUpdate = (rule: FilterRule, val: [number, number] | null) => {
  if (val && val.length === 2) {
    rule.value = new Date(val[0]).toISOString()
    rule.value_to = new Date(val[1]).toISOString()
  } else {
    rule.value = ''
    rule.value_to = ''
  }
}

const addRule = () => {
  if (form.value.rules.length > 0) {
    const lastRule = form.value.rules[form.value.rules.length - 1]
    if (!lastRule.connector) {
      lastRule.connector = 'AND'
    }
  }
  const newRule: FilterRule = {
    _uid: createUid(),
    prefix_parens: '',
    field: 'format',
    op: 'eq',
    value: 'jpg',
    suffix_parens: '',
    connector: 'AND'
  }
  form.value.rules.push(newRule)
}

const removeRule = (idx: number) => {
  form.value.rules.splice(idx, 1)
}

const presetButtons = computed(() => [
  { key: 'today', label: t('filter.presetToday'), tip: t('filter.presetToday'), rule: { field: 'created_at', op: 'today' } },
  { key: 'large', label: t('filter.presetLarge'), tip: t('filter.presetLargeTip'), rule: { field: 'size', op: 'gt', value: 10, unit: 'MB' } },
  { key: 'wm', label: t('filter.presetWatermarked'), tip: t('filter.presetWatermarked'), rule: { field: 'watermark_mode', op: 'eq', value: 'on' } },
  { key: 'hl', label: t('filter.presetHotlink'), tip: t('filter.presetHotlink'), rule: { field: 'hotlink_mode', op: 'eq', value: 'on' } },
  { key: 'calls', label: t('filter.presetHasCalls'), tip: t('filter.presetHasCalls'), rule: { field: 'calls', op: 'gt_zero' } },
  { key: 'temp', label: t('filter.presetTempLink'), tip: t('filter.presetTempLink'), rule: { field: 'temp_links', op: 'has_active' } },
  { key: 'images', label: t('filter.presetImages'), tip: t('filter.fieldTypeTip'), rule: { field: 'is_image', op: 'image' } },
  { key: 'non_images', label: t('filter.presetNonImages'), tip: t('filter.fieldTypeTip'), rule: { field: 'is_image', op: 'non_image' } },
  { key: 'portrait', label: t('filter.presetPortrait'), tip: t('filter.presetPortraitTip'), rule: { field: 'dimensions', op: 'portrait', sub_type: 'aspect_ratio' } },
  { key: 'landscape', label: t('filter.presetLandscape'), tip: t('filter.presetLandscapeTip'), rule: { field: 'dimensions', op: 'landscape', sub_type: 'aspect_ratio' } },
])

const applyPreset = (presetRule: any) => {
  if (form.value.rules.length > 0) {
    const lastRule = form.value.rules[form.value.rules.length - 1]
    if (!lastRule.connector) {
      lastRule.connector = 'AND'
    }
  }
  form.value.rules.push({
    _uid: createUid(),
    prefix_parens: '',
    suffix_parens: '',
    connector: 'AND',
    ...presetRule
  })
}

const getAspectLabel = (op: string) => {
  switch (op) {
  case 'landscape': return t('filter.opLandscape')
  case 'portrait': return t('filter.opPortrait')
  case 'square': return t('filter.opSquare')
  default: return op
  }
}

const getTimePresetLabel = (op: string) => {
  switch (op) {
  case 'today': return t('filter.timeToday')
  case 'yesterday': return t('filter.timeYesterday')
  case '7d': return t('filter.time7Days')
  case '30d': return t('filter.time30Days')
  case 'this_month': return t('filter.timeThisMonth')
  default: return op
  }
}

const getTempLinkLabel = (op: string) => {
  switch (op) {
  case 'has_active': return t('filter.opHasActive')
  case 'has_any': return t('filter.opHasAny')
  case 'none': return t('filter.opNoTemp')
  default: return op
  }
}

const validRulesCount = computed(() => {
  return form.value.rules.filter(r => {
    if (!r.field) return false
    if (['is_image', 'temp_links'].includes(r.field)) return true
    if (r.field === 'created_at' && ['today', 'yesterday', '7d', '30d', 'this_month'].includes(r.op)) return true
    if (r.field === 'dimensions' && ['landscape', 'portrait', 'square'].includes(r.op)) return true
    if (['calls', 'uv'].includes(r.field) && ['is_zero', 'gt_zero'].includes(r.op)) return true
    return r.value !== undefined && r.value !== '' && r.value !== null
  }).length
})

const expressionPreviewText = computed(() => {
  if (form.value.rules.length === 0) return ''
  const parts: string[] = []
  form.value.rules.forEach((rule, idx) => {
    if (idx > 0) {
      const conn = form.value.rules[idx - 1].connector || 'AND'
      parts.push(conn)
    }
    const pre = rule.prefix_parens ? `${rule.prefix_parens} ` : ''
    const suf = rule.suffix_parens ? ` ${rule.suffix_parens}` : ''
    const label = formatFilterRuleLabel(rule, policyOptions.value, t)
    parts.push(`${pre}${label}${suf}`)
  })
  return parts.join(' ')
})

const canApply = computed(() => {
  return validRulesCount.value > 0 && parenCounts.value.isBalanced
})

const handleReset = () => {
  form.value.rules = []
  form.value.logic = 'and'
  emit('reset')
  visible.value = false
}

const handleApply = () => {
  const cleanedRules = form.value.rules.map(r => {
    const copy = { ...r }
    delete copy.date_range
    return copy
  })
  emit('apply', {
    logic: form.value.logic || 'and',
    rules: cleanedRules
  })
  visible.value = false
}
</script>

<style scoped>
.filter-rules-scroll-area {
  display: flex;
  flex-direction: column;
  max-height: 52vh;
  overflow-y: auto;
  padding-right: 4px;
}

.rule-connector-container {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 4px 0;
  padding: 0 16px;
}

.connector-divider-line {
  flex: 1;
  height: 1px;
  background: rgba(255, 255, 255, 0.08);
}

.rule-row-card {
  display: flex;
  align-items: center;
  gap: 8px;
  background: rgba(255, 255, 255, 0.025);
  border: 1px solid rgba(255, 255, 255, 0.07);
  padding: 8px 12px;
  border-radius: 8px;
  transition: all 0.2s ease;
}

.rule-row-card:hover {
  background: rgba(255, 255, 255, 0.04);
  border-color: rgba(255, 255, 255, 0.12);
}

.rule-row-card.is-dragging {
  opacity: 0.4;
  border-style: dashed;
}

.rule-row-card.is-dragover {
  border-color: #63e2b7;
  background: rgba(99, 226, 183, 0.06);
}

.rule-drag-handle {
  display: flex;
  align-items: center;
  cursor: grab;
  color: #777;
  padding: 2px 4px;
  border-radius: 4px;
  transition: color 0.15s ease;
}

.rule-drag-handle:hover {
  color: #ddd;
  background: rgba(255, 255, 255, 0.06);
}

.rule-drag-handle:active {
  cursor: grabbing;
}

.expression-preview-card {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  background: rgba(255, 255, 255, 0.02);
  border: 1px solid rgba(255, 255, 255, 0.06);
  padding: 8px 12px;
  border-radius: 6px;
  font-size: 12px;
}

.preview-label {
  color: #888;
  white-space: nowrap;
  font-weight: 500;
}

.preview-code {
  color: #63e2b7;
  font-family: monospace;
  word-break: break-all;
  line-height: 1.5;
}
</style>
