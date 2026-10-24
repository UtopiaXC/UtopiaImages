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

          <div style="font-size: 13px; color: #aaa; margin-bottom: 12px; line-height: 1.6;">
            <div><strong>{{ $t('watermarks.layout') }}：</strong>{{ parseLayers(t.layers).layout === 'tile' ? $t('watermarks.layoutTile') : $t('watermarks.layoutPosition') }}</div>
            <div><strong>{{ $t('watermarks.content') }}：</strong><code style="background: rgba(255,255,255,0.08); padding: 2px 4px; border-radius: 4px;">{{ parseLayers(t.layers).text }}</code></div>
            <div><strong>{{ $t('watermarks.opacity') }}：</strong>{{ Math.round((parseLayers(t.layers).opacity || 0.2) * 100) }}%</div>
          </div>

          <template #action>
            <div style="display: flex; justify-content: flex-end; gap: 8px;">
              <n-button size="tiny" quaternary @click="previewExisting(t)">{{ $t('watermarks.preview') }}</n-button>
              <n-button size="tiny" quaternary type="error" @click="deleteTemplate(t.id)">{{ $t('common.delete') }}</n-button>
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
      :title="$t('watermarks.new')"
      style="width: 720px; max-width: 95vw; border-radius: 12px;"
    >
      <n-form label-placement="left" label-width="100">
        <n-form-item :label="$t('watermarks.name')">
          <n-input v-model:value="editForm.name" :placeholder="$t('watermarks.namePlaceholder')" />
        </n-form-item>

        <n-form-item v-if="isAdmin" :label="$t('watermarks.global')">
          <n-switch v-model:value="editForm.is_global" />
          <span style="margin-left: 10px; font-size: 12px; color: #888;">{{ $t('watermarks.globalTip') }}</span>
        </n-form-item>

        <n-form-item :label="$t('watermarks.content')">
          <div style="width: 100%;">
            <n-input v-model:value="editForm.text" :placeholder="$t('watermarks.contentPlaceholder')" @input="debouncedPreview" />
            <div style="margin-top: 8px; display: flex; flex-wrap: wrap; gap: 6px;">
              <n-button size="tiny" quaternary @click="insertTag('{now}')">{{ $t('watermarks.tagTime') }}</n-button>
              <n-button size="tiny" quaternary @click="insertTag('{ip}')">{{ $t('watermarks.tagIp') }}</n-button>
              <n-button size="tiny" quaternary @click="insertTag('{owner}')">{{ $t('watermarks.tagOwner') }}</n-button>
              <n-button size="tiny" quaternary @click="insertTag('{filename}')">{{ $t('watermarks.tagFilename') }}</n-button>
            </div>
          </div>
        </n-form-item>

        <n-form-item :label="$t('watermarks.layout')">
          <n-radio-group v-model:value="editForm.layout" @update:value="debouncedPreview">
            <n-radio-button value="tile">{{ $t('watermarks.layoutTile') }}</n-radio-button>
            <n-radio-button value="position">{{ $t('watermarks.layoutPosition') }}</n-radio-button>
          </n-radio-group>
        </n-form-item>

        <n-form-item v-if="editForm.layout === 'position'" :label="$t('watermarks.position')">
          <n-select
            v-model:value="editForm.position"
            :options="positionOptions"
            @update:value="debouncedPreview"
          />
        </n-form-item>

        <n-form-item :label="$t('watermarks.opacity')">
          <div style="width: 100%; display: flex; align-items: center; gap: 16px;">
            <n-slider v-model:value="editForm.opacityPercent" :min="5" :max="100" style="flex: 1;" @update:value="debouncedPreview" />
            <span style="width: 45px; text-align: right;">{{ editForm.opacityPercent }}%</span>
          </div>
        </n-form-item>

        <!-- Live Preview Box -->
        <n-divider dashed title-placement="left" style="font-size: 13px;">{{ $t('watermarks.preview') }}</n-divider>
        <div style="text-align: center; background: #0c0d12; padding: 12px; border-radius: 8px;">
          <img
            v-if="previewUrl"
            :src="previewUrl"
            style="max-width: 100%; height: auto; border-radius: 4px; box-shadow: 0 4px 16px rgba(0,0,0,0.4);"
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
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'

const message = useMessage()
const { t } = useI18n()
const templates = ref<any[]>([])
const showModal = ref(false)
const saving = ref(false)
const isAdmin = ref(false)
const previewUrl = ref('')

const positionOptions = computed(() => [
  { label: t('watermarks.posBottomRight'), value: 'bottom-right' },
  { label: t('watermarks.posBottomLeft'), value: 'bottom-left' },
  { label: t('watermarks.posTopRight'), value: 'top-right' },
  { label: t('watermarks.posTopLeft'), value: 'top-left' },
  { label: t('watermarks.posCenter'), value: 'center' },
])

const editForm = ref({
  name: t('watermarks.defaultName'),
  is_global: false,
  text: 'Utopia Images {now} {ip}',
  layout: 'tile',
  position: 'bottom-right',
  opacityPercent: 20
})

const parseLayers = (jsonStr: string) => {
  try {
    return JSON.parse(jsonStr)
  } catch {
    return { text: 'Utopia Images', layout: 'tile', opacity: 0.2 }
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
  editForm.value = {
    name: t('watermarks.tileName'),
    is_global: isAdmin.value,
    text: 'Utopia Images {now} {ip}',
    layout: 'tile',
    position: 'bottom-right',
    opacityPercent: 20
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
      opacity: editForm.value.opacityPercent / 100
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
      opacity: editForm.value.opacityPercent / 100
    })

    await api.post('/watermarks', {
      name: editForm.value.name,
      layers: layers,
      is_global: editForm.value.is_global
    })

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
  const l = parseLayers(tItem.layers)
  editForm.value = {
    name: tItem.name,
    is_global: !tItem.owner_id,
    text: l.text || 'Utopia Images',
    layout: l.layout || 'tile',
    position: l.position || 'bottom-right',
    opacityPercent: Math.round((l.opacity || 0.2) * 100)
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
