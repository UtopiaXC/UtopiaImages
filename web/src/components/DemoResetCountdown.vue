<template>
  <div class="demo-countdown-floating" :class="{ 'is-urgent': isUrgent, 'is-resetting': isResetting }">
    <div class="demo-countdown-dot" />
    <span class="demo-countdown-label">
      <template v-if="isResetting">
        {{ $t('demo.resetting') }}
      </template>
      <template v-else>
        {{ $t('demo.countdownLabel', { time: formattedRemaining }) }}
      </template>
    </span>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'
import { clearCurrentUser } from '../utils/auth'

const message = useMessage()
const { t } = useI18n()

const knownEpoch = ref<number | null>(null)
const nextResetAtMs = ref<number>(Date.now() + 30 * 60 * 1000)
const serverOffsetMs = ref<number>(0)
const remainingMs = ref<number>(30 * 60 * 1000)
const serverResetting = ref<boolean>(false)
const reloading = ref<boolean>(false)

let eventSource: EventSource | null = null
let tickTimer: ReturnType<typeof setInterval> | null = null
let pollTimer: ReturnType<typeof setInterval> | null = null

const isResetting = computed(() => serverResetting.value || remainingMs.value <= 0)
const isUrgent = computed(() => !isResetting.value && remainingMs.value <= 60 * 1000)

const formattedRemaining = computed(() => {
  const totalSeconds = Math.max(0, Math.ceil(remainingMs.value / 1000))
  const mins = Math.floor(totalSeconds / 60)
  const secs = totalSeconds % 60
  return `${String(mins).padStart(2, '0')}:${String(secs).padStart(2, '0')}`
})

const triggerReloadAfterReset = () => {
  if (reloading.value) return
  reloading.value = true
  clearCurrentUser()
  message.info(t('demo.resetCompletedReloading'), { duration: 2000 })
  setTimeout(() => {
    window.location.reload()
  }, 600)
}

const applyStatus = (data: any, isResetEvent = false) => {
  if (!data) return
  const epoch = Number(data.reset_epoch || 0)
  const serverTime = Number(data.server_time || Date.now())
  serverOffsetMs.value = serverTime - Date.now()
  if (data.next_reset_at) {
    nextResetAtMs.value = Number(data.next_reset_at)
  }
  serverResetting.value = Boolean(data.is_resetting)

  if (knownEpoch.value === null) {
    knownEpoch.value = epoch
  } else if (epoch > knownEpoch.value || isResetEvent) {
    knownEpoch.value = epoch
    triggerReloadAfterReset()
    return
  }

  updateRemaining()
}

const fetchStatus = async () => {
  try {
    const res = await api.get('/demo/status')
    applyStatus(res.data, false)
  } catch {
    // Server may briefly return 503 during reset; keep polling
  }
}

const startFastPoll = () => {
  if (pollTimer) return
  pollTimer = setInterval(() => {
    fetchStatus()
  }, 1500)
}

const stopFastPoll = () => {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

const updateRemaining = () => {
  const nowServer = Date.now() + serverOffsetMs.value
  const diff = Math.max(0, nextResetAtMs.value - nowServer)
  remainingMs.value = diff
  if (diff <= 0) {
    startFastPoll()
  } else if (!serverResetting.value) {
    stopFastPoll()
  }
}

const connectSSE = () => {
  if (typeof EventSource === 'undefined') {
    startFastPoll()
    return
  }
  if (eventSource) {
    eventSource.close()
  }
  eventSource = new EventSource('/api/v1/demo/events')
  eventSource.addEventListener('status', (ev: MessageEvent) => {
    try {
      applyStatus(JSON.parse(ev.data), false)
    } catch {}
  })
  eventSource.addEventListener('reset', (ev: MessageEvent) => {
    try {
      applyStatus(JSON.parse(ev.data), true)
    } catch {
      triggerReloadAfterReset()
    }
  })
  eventSource.onerror = () => {
    // Rely on periodic status check if SSE disconnects
    if (remainingMs.value <= 5000) {
      startFastPoll()
    }
  }
}

onMounted(() => {
  fetchStatus()
  connectSSE()
  tickTimer = setInterval(updateRemaining, 250)
})

onBeforeUnmount(() => {
  if (eventSource) {
    eventSource.close()
    eventSource = null
  }
  if (tickTimer) {
    clearInterval(tickTimer)
    tickTimer = null
  }
  stopFastPoll()
})
</script>

<style scoped>
.demo-countdown-floating {
  position: fixed;
  right: 20px;
  bottom: 20px;
  z-index: 2147483647;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 8px 14px;
  border-radius: 999px;
  background: rgba(24, 24, 28, 0.92);
  backdrop-filter: blur(12px);
  border: 1px solid rgba(99, 226, 183, 0.35);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.55);
  color: #e6f7f0;
  font-size: 13px;
  font-weight: 500;
  font-variant-numeric: tabular-nums;
  user-select: none;
  pointer-events: auto;
  transition: border-color 0.25s ease, color 0.25s ease;
}

.demo-countdown-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #63e2b7;
  box-shadow: 0 0 8px rgba(99, 226, 183, 0.8);
  flex-shrink: 0;
}

.demo-countdown-floating.is-urgent {
  border-color: rgba(242, 201, 125, 0.65);
  color: #f2c97d;
}

.demo-countdown-floating.is-urgent .demo-countdown-dot {
  background: #f2c97d;
  box-shadow: 0 0 8px rgba(242, 201, 125, 0.9);
}

.demo-countdown-floating.is-resetting {
  border-color: rgba(232, 128, 128, 0.7);
  color: #e88080;
}

.demo-countdown-floating.is-resetting .demo-countdown-dot {
  background: #e88080;
  box-shadow: 0 0 8px rgba(232, 128, 128, 0.9);
}

@media (max-width: 768px) {
  .demo-countdown-floating {
    right: 12px;
    bottom: 12px;
    padding: 6px 12px;
    font-size: 12px;
  }
}
</style>
