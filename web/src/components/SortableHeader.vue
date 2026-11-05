<template>
  <div
    class="sortable-header"
    :style="{ justifyContent: align === 'right' ? 'flex-end' : align === 'center' ? 'center' : 'flex-start' }"
    @click="toggleSort"
  >
    <span class="sortable-title">{{ title }}</span>
    <span class="sort-icon-box">
      <n-icon v-if="isActive && currentOrder === 'asc'" size="14" class="active-icon">
        <ArrowUpOutline />
      </n-icon>
      <n-icon v-else-if="isActive && currentOrder === 'desc'" size="14" class="active-icon">
        <ArrowDownOutline />
      </n-icon>
      <n-icon v-else size="14" class="inactive-icon">
        <SwapVerticalOutline />
      </n-icon>
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { ArrowUpOutline, ArrowDownOutline, SwapVerticalOutline } from '@vicons/ionicons5'

const props = withDefaults(
  defineProps<{
    title: string
    column: string
    currentSortBy?: string
    currentOrder?: 'asc' | 'desc' | null | string
    align?: 'left' | 'right' | 'center'
  }>(),
  {
    currentSortBy: '',
    currentOrder: '',
    align: 'left'
  }
)

const emit = defineEmits<{
  (e: 'sort', column: string, order: 'asc' | 'desc' | null): void
}>()

const isActive = computed(() => props.currentSortBy === props.column && Boolean(props.currentOrder))

const toggleSort = () => {
  if (props.currentSortBy !== props.column) {
    emit('sort', props.column, 'desc')
    return
  }
  if (props.currentOrder === 'desc') {
    emit('sort', props.column, 'asc')
  } else if (props.currentOrder === 'asc') {
    emit('sort', props.column, null)
  } else {
    emit('sort', props.column, 'desc')
  }
}
</script>

<style scoped>
.sortable-header {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  cursor: pointer;
  user-select: none;
  transition: color 0.2s ease;
}

.sortable-header:hover .sortable-title {
  color: #fff;
}

.sortable-header:hover .inactive-icon {
  color: #aaa;
}

.sort-icon-box {
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.active-icon {
  color: #63e2b7;
}

.inactive-icon {
  color: #666;
  transition: color 0.2s ease;
}
</style>
