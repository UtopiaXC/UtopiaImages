<template>
  <div style="display: inline-block;">
    <n-button
      :size="size"
      :disabled="disabled"
      secondary
      @click="openModal"
      style="max-width: 220px; overflow: hidden;"
    >
      <template #icon>
        <n-icon><ServerOutline /></n-icon>
      </template>
      <span style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
        {{ buttonText }}
      </span>
    </n-button>

    <n-modal
      v-model:show="showModal"
      preset="card"
      :title="$t('upload.selectPolicyModalTitle')"
      style="width: 520px; max-width: 95vw; border-radius: 12px;"
    >
      <div style="display: flex; flex-direction: column; gap: 14px;">
        <div v-if="normalizedPolicies.length > 5">
          <n-input
            v-model:value="searchQuery"
            size="small"
            :placeholder="$t('common.search')"
            clearable
          >
            <template #prefix>
              <n-icon><SearchOutline /></n-icon>
            </template>
          </n-input>
        </div>

        <div style="max-height: 360px; overflow-y: auto; display: flex; flex-direction: column; gap: 8px;">
          <div
            v-for="item in filteredPolicies"
            :key="item.id"
            :class="['policy-select-card', { active: tempSelectedIds.includes(item.id) }]"
            @click="togglePolicy(item.id)"
          >
            <div style="display: flex; align-items: center; gap: 10px; width: 100%;">
              <n-checkbox
                :checked="tempSelectedIds.includes(item.id)"
                :disabled="isCheckboxDisabled(item.id)"
                @update:checked="() => togglePolicy(item.id)"
                @click.stop
              />
              <div style="flex: 1; min-width: 0;">
                <div style="display: flex; align-items: center; gap: 8px; flex-wrap: wrap;">
                  <span style="font-weight: 500; font-size: 14px;">{{ item.name }}</span>
                  <n-tag size="tiny" :bordered="false" type="info">{{ item.driver }}</n-tag>
                  <n-tag v-if="item.is_default" size="tiny" type="success" :bordered="false">
                    {{ $t('adminPolicies.isDefault') }}
                  </n-tag>
                </div>
                <div v-if="item.path_prefix" style="font-size: 11px; color: #777; margin-top: 2px;">
                  {{ item.path_prefix }}
                </div>
              </div>
            </div>
          </div>
          <div v-if="filteredPolicies.length === 0" style="text-align: center; color: #888; padding: 24px 0;">
            {{ $t('common.noData') }}
          </div>
        </div>

        <div v-if="min > 0" style="font-size: 12px; color: #888;">
          {{ $t('upload.minPolicyNotice') }}
        </div>
      </div>

      <template #footer>
        <div style="display: flex; justify-content: space-between; align-items: center;">
          <span style="font-size: 13px; color: #aaa;">
            {{ $t('upload.selectedCount', { count: tempSelectedIds.length }) }}
          </span>
          <div style="display: flex; gap: 10px;">
            <n-button size="small" secondary @click="showModal = false">
              {{ $t('common.cancel') }}
            </n-button>
            <n-button
              size="small"
              type="primary"
              :disabled="tempSelectedIds.length < min"
              @click="confirmSelection"
            >
              {{ $t('common.confirm') }}
            </n-button>
          </div>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { ServerOutline, SearchOutline } from '@vicons/ionicons5'

const props = withDefaults(
  defineProps<{
    value?: number[]
    policies?: any[]
    options?: any[]
    min?: number
    size?: 'small' | 'medium'
    disabled?: boolean
    placeholder?: string
  }>(),
  {
    value: () => [],
    policies: () => [],
    options: () => [],
    min: 1,
    size: 'small',
    disabled: false,
    placeholder: '选择存储策略'
  }
)

const emit = defineEmits<{
  (e: 'update:value', val: number[]): void
  (e: 'change', val: number[]): void
}>()

const showModal = ref(false)
const searchQuery = ref('')
const tempSelectedIds = ref<number[]>([])

const normalizedPolicies = computed(() => {
  if (props.policies && props.policies.length > 0) {
    return props.policies.map((p) => ({
      id: Number(p.id ?? p.value),
      name: p.name || p.label?.split(' · ')[0] || `策略 #${p.id ?? p.value}`,
      driver: p.driver || p.label?.split(' · ')[1] || 'storage',
      is_default: Boolean(p.is_default),
      path_prefix: p.path_prefix || ''
    }))
  }
  if (props.options && props.options.length > 0) {
    return props.options.map((o) => ({
      id: Number(o.value),
      name: o.name || o.label?.split(' · ')[0] || `策略 #${o.value}`,
      driver: o.driver || o.label?.split(' · ')[1] || 'storage',
      is_default: Boolean(o.is_default),
      path_prefix: o.path_prefix || ''
    }))
  }
  return []
})

const filteredPolicies = computed(() => {
  if (!searchQuery.value.trim()) return normalizedPolicies.value
  const q = searchQuery.value.toLowerCase()
  return normalizedPolicies.value.filter(
    (p) => p.name.toLowerCase().includes(q) || p.driver.toLowerCase().includes(q)
  )
})

const buttonText = computed(() => {
  const current = props.value || []
  if (current.length === 0) {
    return props.placeholder
  }
  const firstId = current[0]
  const found = normalizedPolicies.value.find((p) => p.id === firstId)
  const firstName = found ? found.name : `策略 #${firstId}`
  if (current.length === 1) {
    return firstName
  }
  return `${firstName} +${current.length - 1}`
})

const isCheckboxDisabled = (policyId: number) => {
  if (props.min > 0 && tempSelectedIds.value.length <= props.min && tempSelectedIds.value.includes(policyId)) {
    return true
  }
  return false
}

const togglePolicy = (policyId: number) => {
  if (tempSelectedIds.value.includes(policyId)) {
    if (props.min > 0 && tempSelectedIds.value.length <= props.min) {
      return
    }
    tempSelectedIds.value = tempSelectedIds.value.filter((id) => id !== policyId)
  } else {
    tempSelectedIds.value.push(policyId)
  }
}

const openModal = () => {
  if (props.disabled) return
  searchQuery.value = ''
  tempSelectedIds.value = [...(props.value || [])]
  showModal.value = true
}

const confirmSelection = () => {
  if (tempSelectedIds.value.length < props.min) return
  emit('update:value', [...tempSelectedIds.value])
  emit('change', [...tempSelectedIds.value])
  showModal.value = false
}
</script>

<style scoped>
.policy-select-card {
  padding: 10px 14px;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(255, 255, 255, 0.02);
  cursor: pointer;
  transition: all 0.2s ease;
}

.policy-select-card:hover {
  border-color: rgba(99, 226, 183, 0.3);
  background: rgba(99, 226, 183, 0.04);
}

.policy-select-card.active {
  border-color: rgba(99, 226, 183, 0.5);
  background: rgba(99, 226, 183, 0.08);
}
</style>
