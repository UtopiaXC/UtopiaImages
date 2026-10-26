<template>
  <n-config-provider :theme="darkTheme" :locale="naiveLocale" :date-locale="naiveDateLocale">
    <n-loading-bar-provider>
      <n-dialog-provider>
        <n-notification-provider>
          <n-message-provider>
            <router-view />
          </n-message-provider>
        </n-notification-provider>
      </n-dialog-provider>
    </n-loading-bar-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { darkTheme, zhCN, dateZhCN, enUS, dateEnUS, jaJP, dateJaJP } from 'naive-ui'

const { locale } = useI18n()

const naiveLocale = computed(() => {
  if (locale.value === 'zh-CN') return zhCN
  if (locale.value === 'ja-JP') return jaJP
  return enUS
})

const naiveDateLocale = computed(() => {
  if (locale.value === 'zh-CN') return dateZhCN
  if (locale.value === 'ja-JP') return dateJaJP
  return dateEnUS
})
</script>

<style>
* {
  box-sizing: border-box;
}

body {
  margin: 0;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
  -webkit-font-smoothing: antialiased;
  background-color: #101014;
  color: #fff;
  overflow-x: hidden;
}

/* Global Responsive Table Utilities */
.table-responsive {
  width: 100%;
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
  margin-bottom: 12px;
}

@media (max-width: 768px) {
  .table-responsive table {
    min-width: 520px;
  }

  /* Automatically adapt left-labelled forms on mobile to vertical */
  .n-form-item.n-form-item--left-labelled {
    display: flex !important;
    flex-direction: column !important;
    align-items: stretch !important;
  }
  .n-form-item.n-form-item--left-labelled .n-form-item-label {
    text-align: left !important;
    padding-right: 0 !important;
    padding-bottom: 6px !important;
    width: auto !important;
    min-width: 0 !important;
  }

  /* Modal responsiveness on mobile screens */
  .n-modal.n-card {
    max-width: 95vw !important;
    margin: 10px auto !important;
  }

  /* Card paddings on mobile */
  .n-card {
    border-radius: 8px !important;
  }

  /* Reduce input max-widths so they stretch cleanly */
  .n-input, .n-select, .n-input-number {
    max-width: 100% !important;
  }
}
</style>
