<template>
  <div>
    <!-- Top toolbar & Filters -->
    <div style="display: flex; flex-direction: column; gap: 12px; margin-bottom: 20px;">
      <!-- Row 1: Search, Filter Button, and View Mode -->
      <div class="files-toolbar-row" style="display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px;">
        <div class="files-filter-group" style="display: flex; flex-wrap: wrap; align-items: center; gap: 10px; flex: 1;">
          <n-input
            v-model:value="search"
            :placeholder="$t('common.search')"
            style="width: 200px;"
            clearable
            @update:value="onFilterChange"
          >
            <template #prefix>
              <n-icon :component="SearchOutline" />
            </template>
          </n-input>

          <!-- Advanced Filter Button -->
          <n-button
            size="small"
            :type="activeFilterCount > 0 ? 'primary' : 'default'"
            :secondary="activeFilterCount === 0"
            @click="showFilterModal = true"
          >
            <template #icon>
              <n-icon :component="FilterOutline" />
            </template>
            <span>{{ $t('filter.button') }}</span>
            <n-badge
              v-if="activeFilterCount > 0"
              :value="activeFilterCount"
              type="info"
              style="margin-left: 6px;"
            />
          </n-button>

          <!-- Clear Filters Button -->
          <n-button
            v-if="activeFilterCount > 0"
            size="small"
            quaternary
            @click="clearAllFilters"
          >
            {{ $t('filter.clear') }}
          </n-button>
        </div>

        <!-- View Mode Selector -->
        <div style="display: flex; align-items: center; gap: 6px;">
          <span style="font-size: 13px; color: #888;">{{ $t('files.viewMode') }}:</span>
          <n-select
            v-model:value="viewMode"
            :options="viewModeOptions"
            size="small"
            style="width: 120px;"
          />
        </div>
      </div>

      <!-- Active Filter Tags / Chips -->
      <div v-if="activeFilterChips.length > 0" style="display: flex; flex-wrap: wrap; gap: 6px; align-items: center; background: rgba(255, 255, 255, 0.02); padding: 8px 12px; border-radius: 8px; border: 1px solid rgba(255, 255, 255, 0.05);">
        <span style="font-size: 12px; color: #888;">{{ $t('filter.activeConditions') }}:</span>
        <template v-for="chip in activeFilterChips" :key="chip.index">
          <span v-if="chip.connector" style="font-size: 11px; font-weight: 700; color: #63e2b7; margin: 0 2px;">
            {{ chip.connector }}
          </span>
          <n-tag
            size="small"
            closable
            type="info"
            @close="removeSingleFilter(chip.index)"
          >
            {{ chip.label }}
          </n-tag>
        </template>
      </div>

      <!-- Row 2: Select All Actions & Batch Operations -->
      <div style="display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 12px; background: rgba(255, 255, 255, 0.02); padding: 8px 12px; border-radius: 8px; border: 1px solid rgba(255, 255, 255, 0.05);">
        <!-- Selection buttons -->
        <div style="display: flex; align-items: center; gap: 8px; flex-wrap: wrap;">
          <n-button size="small" secondary @click="handleSelectFiltered">
            {{ $t('files.selectAllFiltered') }}
          </n-button>
          <n-button size="small" secondary @click="handleSelectAll">
            {{ $t('files.selectAllLibrary') }}
          </n-button>
          <n-button size="small" quaternary :disabled="selectedIds.length === 0" @click="handleDeselectAll">
            {{ $t('files.deselectAll') }}
          </n-button>
          <n-tag v-if="selectedIds.length > 0" type="info" size="small" round>
            {{ $t('files.selectedCount', { count: selectedIds.length }) }}
          </n-tag>
        </div>

        <!-- Batch actions -->
        <div v-if="selectedIds.length > 0" style="display: flex; align-items: center; gap: 8px; flex-wrap: wrap;">
          <!-- Batch Watermark -->
          <n-dropdown :options="batchWatermarkMenuOptions" @select="handleBatchWatermark">
            <n-button size="small" type="warning" secondary>
              {{ $t('files.batchWatermark') }}
            </n-button>
          </n-dropdown>

          <!-- Batch Hotlink -->
          <n-dropdown :options="batchHotlinkMenuOptions" @select="handleBatchHotlink">
            <n-button size="small" type="info" secondary>
              {{ $t('files.batchHotlink') }}
            </n-button>
          </n-dropdown>

          <!-- Batch Delete -->
          <n-popconfirm @positive-click="handleBatchDelete">
            <template #trigger>
              <n-button size="small" type="error">
                <span>{{ $t('files.batchDelete') }}</span>
                <span style="margin-left: 6px;">· {{ selectedIds.length }}</span>
              </n-button>
            </template>
            {{ $t('files.confirmBatchDelete', { count: selectedIds.length }) }}
          </n-popconfirm>
        </div>
      </div>
    </div>

    <!-- View 1: Masonry (Default) -->
    <div v-if="viewMode === 'masonry' && files.length > 0" class="masonry-container">
      <div v-for="file in files" :key="file.id" class="masonry-item">
        <n-card
          size="small"
          hoverable
          style="border-radius: 8px; overflow: hidden;"
          :header-style="{ padding: '8px' }"
          :content-style="{ padding: '0' }"
        >
          <div style="background: #0c0d12; position: relative; overflow: hidden; border-radius: 6px 6px 0 0;">
            <img
              v-if="file.is_image"
              :src="getFileUrl(file, 'thumb')"
              loading="lazy"
              :style="file.public_path.toLowerCase().endsWith('.ico') ? 'max-width: 90%; max-height: 160px; width: auto; height: auto; object-fit: contain; margin: 16px auto; display: block; cursor: pointer;' : 'width: 100%; height: auto; display: block; cursor: pointer; transition: transform 0.2s;'"
              @click="openLightbox(file)"
            />
            <div v-else style="font-size: 13px; color: #888; text-align: center; padding: 40px 16px;">
              <div>{{ file.mime_type }}</div>
            </div>

            <div style="position: absolute; top: 8px; left: 8px;">
              <n-checkbox
                :checked="selectedIds.includes(file.id)"
                @update:checked="(val: boolean) => toggleSelect(file.id, val)"
              />
            </div>

            <div style="position: absolute; top: 8px; right: 8px; display: flex; gap: 4px;">
              <n-tag v-if="file.watermark_mode === 'on'" type="warning" size="tiny">
                {{ $t('files.watermarked') }}
              </n-tag>
            </div>
          </div>

          <div style="padding: 10px 12px;">
            <div style="font-weight: 500; font-size: 13px; text-overflow: ellipsis; overflow: hidden; white-space: nowrap; margin-bottom: 4px;">
              {{ file.display_name }}
            </div>
            <div style="font-size: 11px; color: #777; display: flex; justify-content: space-between; margin-bottom: 8px;">
              <span>{{ formatBytes(file.size) }}</span>
              <span v-if="file.width && file.height">{{ file.width }} × {{ file.height }}</span>
              <span>{{ formatDate(file.created_at) }}</span>
            </div>

            <div style="display: flex; flex-wrap: wrap; gap: 6px; justify-content: flex-end; border-top: 1px solid rgba(255, 255, 255, 0.05); padding-top: 8px;">
              <n-button v-if="file.is_image" size="tiny" quaternary type="info" @click="openImageEditor(file)">
                {{ $t('files.editImage') }}
              </n-button>
              <n-button size="tiny" quaternary type="primary" @click="openLinksModal(file)">
                {{ $t('files.showLinks') }}
              </n-button>
              <n-button size="tiny" quaternary @click="openEditPropertiesModal(file)">
                {{ $t('files.editProperties') }}
              </n-button>
              <n-button size="tiny" quaternary @click="copyLink(baseURL + '/' + file.public_path)">
                {{ $t('common.copy') }}
              </n-button>
              <n-popconfirm @positive-click="deleteFile(file.id)">
                <template #trigger>
                  <n-button size="tiny" quaternary type="error">{{ $t('common.delete') }}</n-button>
                </template>
                {{ $t('files.confirmDelete') }}
              </n-popconfirm>
            </div>
          </div>
        </n-card>
      </div>
    </div>

    <!-- View 2: Adaptive Grid -->
    <div v-else-if="viewMode === 'grid' && files.length > 0" class="gallery-adaptive-grid">
      <div v-for="file in files" :key="file.id" class="gallery-grid-card-wrapper">
        <n-card
          size="small"
          hoverable
          style="border-radius: 8px; overflow: hidden; display: flex; flex-direction: column; height: 100%;"
          :header-style="{ padding: '8px' }"
          :content-style="{ padding: '0', flex: 1, display: 'flex', flexDirection: 'column' }"
        >
          <div style="height: 180px; background: #0c0d12; display: flex; align-items: center; justify-content: center; position: relative; overflow: hidden;">
            <img
              v-if="file.is_image"
              :src="getFileUrl(file, 'thumb')"
              loading="lazy"
              style="max-width: 100%; max-height: 100%; object-fit: contain; cursor: pointer; transition: transform 0.2s;"
              @click="openLightbox(file)"
            />
            <div v-else style="font-size: 13px; color: #888; text-align: center; padding: 20px;">
              <div>{{ file.mime_type }}</div>
            </div>

            <div style="position: absolute; top: 8px; left: 8px;">
              <n-checkbox
                :checked="selectedIds.includes(file.id)"
                @update:checked="(val: boolean) => toggleSelect(file.id, val)"
              />
            </div>

            <div style="position: absolute; top: 8px; right: 8px; display: flex; gap: 4px;">
              <n-tag v-if="file.watermark_mode === 'on'" type="warning" size="tiny">
                {{ $t('files.watermarked') }}
              </n-tag>
            </div>
          </div>

          <div style="padding: 12px; flex: 1; display: flex; flex-direction: column; justify-content: space-between;">
            <div>
              <div style="font-weight: 500; font-size: 13px; text-overflow: ellipsis; overflow: hidden; white-space: nowrap; margin-bottom: 4px;">
                {{ file.display_name }}
              </div>
              <div style="font-size: 12px; color: #777; display: flex; justify-content: space-between;">
                <span>{{ formatBytes(file.size) }}</span>
                <span v-if="file.width && file.height">{{ file.width }} × {{ file.height }}</span>
                <span>{{ formatDate(file.created_at) }}</span>
              </div>
            </div>

            <div style="margin-top: 12px; display: flex; flex-wrap: wrap; gap: 6px; justify-content: flex-end;">
              <n-button v-if="file.is_image" size="tiny" quaternary type="info" @click="openImageEditor(file)">
                {{ $t('files.editImage') }}
              </n-button>
              <n-button size="tiny" quaternary type="primary" @click="openLinksModal(file)">
                {{ $t('files.showLinks') }}
              </n-button>
              <n-button size="tiny" quaternary @click="openEditPropertiesModal(file)">
                {{ $t('files.editProperties') }}
              </n-button>
              <n-button size="tiny" quaternary @click="copyLink(baseURL + '/' + file.public_path)">
                {{ $t('common.copy') }}
              </n-button>
              <n-popconfirm @positive-click="deleteFile(file.id)">
                <template #trigger>
                  <n-button size="tiny" quaternary type="error">{{ $t('common.delete') }}</n-button>
                </template>
                {{ $t('files.confirmDelete') }}
              </n-popconfirm>
            </div>
          </div>
        </n-card>
      </div>
    </div>

    <!-- View 3: List -->
    <div v-else-if="viewMode === 'list' && files.length > 0">
      <n-card size="small" style="border-radius: 8px;">
        <div class="table-responsive">
          <n-table :bordered="false" :single-line="false">
          <thead>
            <tr>
              <th style="width: 40px;"></th>
              <th style="width: 60px;">{{ $t('files.preview') }}</th>
              <th>{{ $t('files.fileName') }}</th>
              <th>{{ $t('files.fileSize') }}</th>
              <th>{{ $t('files.dimensions') }}</th>
              <th>{{ $t('files.mimeType') }}</th>
              <th>{{ $t('files.watermarkSetting') }}</th>
              <th>{{ $t('files.callStats') }}</th>
              <th>{{ $t('common.createdAt') }}</th>
              <th style="text-align: right;">{{ $t('common.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="file in files" :key="file.id">
              <td>
                <n-checkbox
                  :checked="selectedIds.includes(file.id)"
                  @update:checked="(val: boolean) => toggleSelect(file.id, val)"
                />
              </td>
              <td>
                <img
                  v-if="file.is_image"
                  :src="getFileUrl(file, 'thumb')"
                  style="width: 44px; height: 44px; object-fit: cover; border-radius: 4px; cursor: pointer;"
                  @click="openLightbox(file)"
                />
                <span v-else style="color: #666; font-size: 11px;">FILE</span>
              </td>
              <td>
                <span style="font-weight: 500;">{{ file.display_name }}</span>
              </td>
              <td>{{ formatBytes(file.size) }}</td>
              <td>{{ file.width && file.height ? `${file.width} × ${file.height}` : '-' }}</td>
              <td><code>{{ file.mime_type }}</code></td>
              <td>
                <n-tag v-if="file.watermark_mode === 'on'" type="warning" size="tiny">{{ $t('files.watermarked') }}</n-tag>
                <n-tag v-else-if="file.watermark_mode === 'off'" size="tiny">{{ $t('common.no') }}</n-tag>
                <n-tag v-else size="tiny" type="info">{{ $t('upload.watermarkInherit') }}</n-tag>
              </td>
              <td>
                <n-tag size="tiny" type="info">
                  {{ file.total_calls || 0 }} PV · {{ file.total_uv || 0 }} UV
                </n-tag>
              </td>
              <td>{{ formatDate(file.created_at) }}</td>
              <td style="text-align: right;">
                <div style="display: flex; gap: 4px; justify-content: flex-end;">
                  <n-button v-if="file.is_image" size="tiny" quaternary type="info" @click="openImageEditor(file)">
                    {{ $t('files.editImage') }}
                  </n-button>
                  <n-button size="tiny" quaternary type="primary" @click="openLinksModal(file)">
                    {{ $t('files.showLinks') }}
                  </n-button>
                  <n-button size="tiny" quaternary @click="openEditPropertiesModal(file)">
                    {{ $t('files.editProperties') }}
                  </n-button>
                  <n-button size="tiny" quaternary @click="copyLink(baseURL + '/' + file.public_path)">
                    {{ $t('common.copy') }}
                  </n-button>
                  <n-popconfirm @positive-click="deleteFile(file.id)">
                    <template #trigger>
                      <n-button size="tiny" quaternary type="error">{{ $t('common.delete') }}</n-button>
                    </template>
                    {{ $t('files.confirmDelete') }}
                  </n-popconfirm>
                </div>
              </td>
            </tr>
          </tbody>
        </n-table>
        </div>
      </n-card>
    </div>

    <!-- Empty state -->
    <div v-if="files.length === 0" style="text-align: center; padding: 60px 0; color: #777;">
      {{ $t('files.empty') }}
    </div>

    <!-- Pagination -->
    <div style="display: flex; justify-content: center; margin-top: 24px;">
      <n-pagination
        v-model:page="page"
        :page-size="pageSize"
        :item-count="total"
        @update:page="loadFiles"
      />
    </div>

    <!-- Image Lightbox Modal -->
    <n-modal
      v-model:show="showLightbox"
      preset="card"
      :title="lightboxTarget?.display_name || $t('files.previewImage')"
      style="width: 85vw; max-width: 1000px; border-radius: 12px;"
    >
      <div style="text-align: center; background: #0c0d12; border-radius: 8px; padding: 16px; min-height: 300px; display: flex; align-items: center; justify-content: center;">
        <img
          v-if="lightboxTarget"
          :src="getFileUrl(lightboxTarget)"
          :style="lightboxTarget.public_path?.toLowerCase().endsWith('.ico') ? 'max-width: 256px; max-height: 256px; object-fit: contain; border-radius: 4px; padding: 16px; background: rgba(255,255,255,0.04);' : 'max-width: 100%; max-height: 70vh; object-fit: contain; border-radius: 4px;'"
        />
      </div>
      <template #footer>
        <div style="display: flex; justify-content: space-between; align-items: center;">
          <div style="font-size: 13px; color: #888;">
            <span v-if="lightboxTarget?.width && lightboxTarget?.height">{{ lightboxTarget.width }} × {{ lightboxTarget.height }} · </span>
            <span>{{ formatBytes(lightboxTarget?.size || 0) }}</span>
          </div>
          <div style="display: flex; gap: 8px;">
            <n-button v-if="lightboxTarget?.is_image" size="small" type="info" secondary @click="openImageEditor(lightboxTarget)">
              {{ $t('files.editImage') }}
            </n-button>
            <n-button size="small" secondary @click="openLinksModal(lightboxTarget)">
              {{ $t('files.showLinks') }}
            </n-button>
            <n-button size="small" secondary @click="openEditPropertiesModal(lightboxTarget)">
              {{ $t('files.editProperties') }}
            </n-button>
            <n-button size="small" type="primary" @click="copyLink(baseURL + '/' + lightboxTarget?.public_path)">
              {{ $t('common.copy') }}
            </n-button>
          </div>
        </div>
      </template>
    </n-modal>

    <!-- Show Links Modal -->
    <n-modal
      v-model:show="showLinksModal"
      preset="card"
      :title="$t('files.showLinks')"
      style="width: 760px; max-width: 95vw; border-radius: 12px;"
    >
      <div v-if="linksTarget" style="margin-top: 6px;">
        <FileLinksPanel :file="linksTarget" :initial-url="baseURL + '/' + linksTarget.public_path" />

        <!-- Call Statistics Section -->
        <div style="margin-top: 24px; border-top: 1px solid rgba(255,255,255,0.08); padding-top: 16px;">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px;">
            <div style="display: flex; align-items: center; gap: 8px;">
              <h4 style="margin: 0; font-size: 14px; font-weight: 600;">{{ $t('files.callStats') }}</h4>
            </div>
            <div style="display: flex; gap: 8px;">
              <n-tag type="info" size="small">
                PV: {{ linksTarget.total_calls || 0 }}
              </n-tag>
              <n-tag type="success" size="small">
                UV: {{ linksTarget.total_uv || 0 }}
              </n-tag>
            </div>
          </div>
          <div v-if="(linksTarget.call_stats || []).length > 0" class="table-responsive">
            <table style="width: 100%; border-collapse: collapse; font-size: 13px; background: rgba(255,255,255,0.02); border-radius: 6px;">
              <thead>
                <tr style="border-bottom: 1px solid rgba(255,255,255,0.08); color: #888; text-align: left;">
                  <th style="padding: 8px 12px;">{{ $t('files.statFormat') }}</th>
                  <th style="padding: 8px 12px;">{{ $t('files.statSize') }}</th>
                  <th style="padding: 8px 12px; text-align: right;">PV</th>
                  <th style="padding: 8px 12px; text-align: right;">UV</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="cs in linksTarget.call_stats" :key="cs.id" style="border-bottom: 1px solid rgba(255,255,255,0.04);">
                  <td style="padding: 8px 12px; font-family: monospace;">
                    <n-tag size="tiny" type="success">{{ cs.format }}</n-tag>
                  </td>
                  <td style="padding: 8px 12px; color: #ccc;">{{ cs.size }}</td>
                  <td style="padding: 8px 12px; text-align: right; font-weight: 600; color: #63e2b7;">{{ cs.count }}</td>
                  <td style="padding: 8px 12px; text-align: right; font-weight: 600; color: #70c0e8;">{{ cs.uv || 0 }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <div v-else style="color: #777; font-size: 13px; padding: 12px 0; text-align: center;">
            {{ $t('files.noCallStats') }}
          </div>
        </div>
      </div>
    </n-modal>

    <!-- Edit Properties Modal -->
    <n-modal
      v-model:show="showPropertiesModal"
      preset="card"
      :title="$t('files.editProperties')"
      style="width: 620px; max-width: 95vw; border-radius: 12px;"
    >
      <n-form label-placement="left" label-width="120" style="margin-top: 12px;">
        <n-form-item :label="$t('files.fileName')">
          <n-input v-model:value="editPropertiesForm.display_name" />
        </n-form-item>

        <n-form-item :label="$t('files.watermarkSetting')">
          <n-select
            v-model:value="editPropertiesForm.watermark_mode"
            :options="watermarkModeOptions"
          />
        </n-form-item>

        <n-form-item v-if="editPropertiesForm.watermark_mode === 'on'" :label="$t('upload.template')">
          <n-select
            v-model:value="editPropertiesForm.watermark_template_id"
            :options="templateOptions"
            clearable
            :placeholder="$t('upload.defaultTemplate')"
          />
        </n-form-item>

        <n-form-item :label="$t('files.hotlinkSetting')">
          <n-select
            v-model:value="editPropertiesForm.hotlink_mode"
            :options="hotlinkModeOptions"
          />
        </n-form-item>

        <n-form-item v-if="editPropertiesForm.hotlink_mode === 'on'" :label="$t('settings.allowedDomains')">
          <n-input
            v-model:value="editPropertiesForm.hotlink_allowed_domains"
            type="textarea"
            :placeholder="$t('settings.allowedDomainsPlaceholder')"
            :rows="2"
          />
        </n-form-item>

        <n-divider style="margin: 12px 0;" />

        <n-descriptions :column="2" label-placement="left" size="small">
          <n-descriptions-item :label="$t('files.fileSize')">
            {{ formatBytes(propertiesTarget?.size || 0) }}
          </n-descriptions-item>
          <n-descriptions-item :label="$t('files.mimeType')">
            {{ propertiesTarget?.mime_type }}
          </n-descriptions-item>
          <n-descriptions-item :label="$t('files.dimensions')">
            {{ propertiesTarget?.width && propertiesTarget?.height ? `${propertiesTarget.width} × ${propertiesTarget.height}` : '-' }}
          </n-descriptions-item>
          <n-descriptions-item :label="$t('common.createdAt')">
            {{ formatDate(propertiesTarget?.created_at) }}
          </n-descriptions-item>
          <n-descriptions-item :label="$t('files.sha256')" :span="2">
            <code style="font-size: 11px; word-break: break-all;">{{ propertiesTarget?.sha256 }}</code>
          </n-descriptions-item>
        </n-descriptions>

        <div style="display: flex; justify-content: flex-end; margin-top: 16px;">
          <n-button type="primary" :loading="savingProperties" @click="savePropertiesEdit">
            {{ $t('common.save') }}
          </n-button>
        </div>
      </n-form>
    </n-modal>

    <!-- Rich Canvas Image Editor Modal -->
    <ImageEditorModal
      v-model:show="showImageEditor"
      :file="imageEditorTarget"
      @saved="handleImageSaved"
    />

    <!-- Advanced Filter Modal -->
    <FileFilterModal
      v-model:show="showFilterModal"
      :policies="policyOptions"
      :initial-payload="filterPayload"
      @apply="onApplyFilter"
      @reset="onResetFilter"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { SearchOutline, FilterOutline } from '@vicons/ionicons5'
import { api } from '../api/client'
import FileLinksPanel from '../components/FileLinksPanel.vue'
import ImageEditorModal from '../components/ImageEditorModal.vue'
import FileFilterModal, { type AdvancedFilterPayload } from '../components/FileFilterModal.vue'
import { formatFilterRuleLabel } from '../utils/filterLabel'

const message = useMessage()
const { t } = useI18n()

const files = ref<any[]>([])
const baseURL = ref(window.location.origin)
const search = ref('')
const showFilterModal = ref(false)
const filterPayload = ref<AdvancedFilterPayload>({ logic: 'and', rules: [] })
const policies = ref<any[]>([])

const page = ref(1)
const pageSize = ref(24)
const total = ref(0)
const selectedIds = ref<number[]>([])

// View mode: masonry (default), grid, list
const viewMode = ref<'masonry' | 'grid' | 'list'>('masonry')
const viewModeOptions = computed(() => [
  { label: t('files.viewMasonry'), value: 'masonry' },
  { label: t('files.viewGrid'), value: 'grid' },
  { label: t('files.viewList'), value: 'list' }
])

const policyOptions = computed(() =>
  policies.value.map((p: any) => ({
    label: `${p.name} (${p.driver})`,
    value: String(p.id)
  }))
)

const activeFilterCount = computed(() => {
  return filterPayload.value.rules ? filterPayload.value.rules.length : 0
})

const activeFilterChips = computed(() => {
  if (!filterPayload.value || !filterPayload.value.rules) return []
  return filterPayload.value.rules.map((r, index) => {
    const pre = r.prefix_parens ? `${r.prefix_parens} ` : ''
    const suf = r.suffix_parens ? ` ${r.suffix_parens}` : ''
    const core = formatFilterRuleLabel(r, policyOptions.value, t)
    return {
      index,
      connector: index > 0 ? (filterPayload.value.rules[index - 1]?.connector || 'AND') : '',
      label: `${pre}${core}${suf}`
    }
  })
})

const onApplyFilter = (payload: AdvancedFilterPayload) => {
  filterPayload.value = payload
  page.value = 1
  loadFiles()
}

const onResetFilter = () => {
  filterPayload.value = { logic: 'and', rules: [] }
  page.value = 1
  loadFiles()
}

const removeSingleFilter = (index: number) => {
  if (filterPayload.value && filterPayload.value.rules) {
    filterPayload.value.rules.splice(index, 1)
    page.value = 1
    loadFiles()
  }
}

const clearAllFilters = () => {
  filterPayload.value = { logic: 'and', rules: [] }
  page.value = 1
  loadFiles()
}

const batchWatermarkMenuOptions = computed(() => [
  { label: t('upload.watermarkOn'), key: 'on' },
  { label: t('upload.watermarkOff'), key: 'off' },
  { label: t('upload.watermarkInherit'), key: 'inherit' }
])

const batchHotlinkMenuOptions = computed(() => [
  { label: t('files.hotlinkOn'), key: 'on' },
  { label: t('files.hotlinkOff'), key: 'off' },
  { label: t('files.hotlinkInherit'), key: 'inherit' }
])

const onFilterChange = () => {
  page.value = 1
  loadFiles()
}

// Lightbox
const showLightbox = ref(false)
const lightboxTarget = ref<any>(null)

// Links Modal
const showLinksModal = ref(false)
const linksTarget = ref<any>(null)

// Properties Modal
const showPropertiesModal = ref(false)
const propertiesTarget = ref<any>(null)
const savingProperties = ref(false)
const templateOptions = ref<any[]>([])

const editPropertiesForm = ref({
  display_name: '',
  watermark_mode: 'inherit',
  watermark_template_id: null as number | null,
  hotlink_mode: 'inherit',
  hotlink_allowed_domains: ''
})

// Image Editor
const showImageEditor = ref(false)
const imageEditorTarget = ref<any>(null)

const watermarkModeOptions = computed(() => [
  { label: t('upload.watermarkInherit'), value: 'inherit' },
  { label: t('upload.watermarkOn'), value: 'on' },
  { label: t('upload.watermarkOff'), value: 'off' }
])

const hotlinkModeOptions = computed(() => [
  { label: t('files.hotlinkInherit'), value: 'inherit' },
  { label: t('files.hotlinkOn'), value: 'on' },
  { label: t('files.hotlinkOff'), value: 'off' }
])

const openLightbox = (file: any) => {
  lightboxTarget.value = file
  showLightbox.value = true
}

const openImageEditor = (file: any) => {
  imageEditorTarget.value = file
  showImageEditor.value = true
}

const getFileUrl = (file: any, size?: string) => {
  if (!file) return ''
  const cleanPath = (file.public_path || '').replace(/^\/+/, '')
  let url = `/${cleanPath}`
  const params: string[] = ['from=console']
  if (size) params.push(`size=${size}`)
  const v = file.sign_version || (file.updated_at ? new Date(file.updated_at).getTime() : '')
  if (v) params.push(`v=${v}`)
  if (params.length > 0) {
    url += '?' + params.join('&')
  }
  return url
}

const handleImageSaved = (savedFile?: any) => {
  if (savedFile && savedFile.id) {
    const idx = files.value.findIndex(f => f.id === savedFile.id)
    if (idx !== -1) {
      files.value[idx] = { ...savedFile, sign_version: Date.now() }
    }
    if (lightboxTarget.value && lightboxTarget.value.id === savedFile.id) {
      lightboxTarget.value = { ...savedFile, sign_version: Date.now() }
    }
  }
  loadFiles()
}

const openLinksModal = async (file: any) => {
  linksTarget.value = { ...file }
  showLinksModal.value = true
  try {
    const res = await api.get(`/files/${file.id}`)
    if (res.data.file) {
      linksTarget.value = {
        ...res.data.file,
        call_stats: res.data.call_stats || [],
        total_calls: res.data.total_calls || 0,
        total_uv: res.data.total_uv || 0
      }
    }
  } catch {}
}

const openEditPropertiesModal = async (file: any) => {
  propertiesTarget.value = file
  editPropertiesForm.value = {
    display_name: file.display_name,
    watermark_mode: file.watermark_mode || 'inherit',
    watermark_template_id: file.watermark_template_id || null,
    hotlink_mode: file.hotlink_mode || 'inherit',
    hotlink_allowed_domains: file.hotlink_allowed_domains || ''
  }

  try {
    const res = await api.get(`/files/${file.id}`)
    propertiesTarget.value = res.data.file
    editPropertiesForm.value = {
      display_name: res.data.file.display_name,
      watermark_mode: res.data.file.watermark_mode || 'inherit',
      watermark_template_id: res.data.file.watermark_template_id || null,
      hotlink_mode: res.data.file.hotlink_mode || 'inherit',
      hotlink_allowed_domains: res.data.file.hotlink_allowed_domains || ''
    }
  } catch {}

  showPropertiesModal.value = true
}

const savePropertiesEdit = async () => {
  if (!propertiesTarget.value) return
  savingProperties.value = true
  try {
    const res = await api.patch(`/files/${propertiesTarget.value.id}`, editPropertiesForm.value)
    message.success(t('files.renameSuccess'))
    propertiesTarget.value = res.data.file
    loadFiles()
    showPropertiesModal.value = false
  } catch (err: any) {
    message.error(err.response?.data?.error || t('files.renameFailed'))
  } finally {
    savingProperties.value = false
  }
}

const toggleSelect = (id: number, checked: boolean) => {
  if (checked) {
    if (!selectedIds.value.includes(id)) {
      selectedIds.value.push(id)
    }
  } else {
    selectedIds.value = selectedIds.value.filter((i) => i !== id)
  }
}

const handleSelectFiltered = async () => {
  try {
    const params: any = {}
    if (search.value) params.search = search.value
    if (filterPayload.value && filterPayload.value.rules && filterPayload.value.rules.length > 0) {
      params.filter_rules = JSON.stringify(filterPayload.value)
    }

    const res = await api.get('/files/ids', { params })
    selectedIds.value = res.data.ids || []
    message.success(t('files.selectedCount', { count: selectedIds.value.length }))
  } catch (err: any) {
    message.error(err.response?.data?.error || t('common.error'))
  }
}

const handleSelectAll = async () => {
  try {
    const res = await api.get('/files/ids')
    selectedIds.value = res.data.ids || []
    message.success(t('files.selectedCount', { count: selectedIds.value.length }))
  } catch (err: any) {
    message.error(err.response?.data?.error || t('common.error'))
  }
}

const handleDeselectAll = () => {
  selectedIds.value = []
}

const handleBatchWatermark = async (mode: string) => {
  if (selectedIds.value.length === 0) return
  try {
    await api.post('/files/batch-update', {
      ids: selectedIds.value,
      watermark_mode: mode
    })
    message.success(t('files.batchUpdateSuccess'))
    loadFiles()
  } catch (err: any) {
    message.error(err.response?.data?.error || t('common.error'))
  }
}

const handleBatchHotlink = async (mode: string) => {
  if (selectedIds.value.length === 0) return
  try {
    await api.post('/files/batch-update', {
      ids: selectedIds.value,
      hotlink_mode: mode
    })
    message.success(t('files.batchUpdateSuccess'))
    loadFiles()
  } catch (err: any) {
    message.error(err.response?.data?.error || t('common.error'))
  }
}

const loadFiles = async () => {
  try {
    const params: any = {
      page: page.value,
      page_size: pageSize.value
    }
    if (search.value) params.search = search.value
    if (filterPayload.value && filterPayload.value.rules && filterPayload.value.rules.length > 0) {
      params.filter_rules = JSON.stringify(filterPayload.value)
    }

    const res = await api.get('/files', { params })
    files.value = res.data.items || []
    total.value = res.data.total || 0
    if (res.data.base_url) {
      baseURL.value = res.data.base_url
    }
  } catch {}
}

const loadPolicies = async () => {
  try {
    const res = await api.get('/me/policies')
    policies.value = res.data || []
  } catch {}
}

const loadTemplates = async () => {
  try {
    const res = await api.get('/watermarks')
    templateOptions.value = (res.data || []).map((tmpl: any) => ({
      label: tmpl.name,
      value: tmpl.id
    }))
  } catch {}
}

const deleteFile = async (id: number) => {
  try {
    await api.delete(`/files/${id}`)
    message.success(t('files.deleteSuccess'))
    window.dispatchEvent(new CustomEvent('refresh-quota'))
    loadFiles()
  } catch (err: any) {
    message.error(err.response?.data?.error || t('files.deleteFailed'))
  }
}

const handleBatchDelete = async () => {
  if (selectedIds.value.length === 0) return
  try {
    await api.post('/files/batch-delete', { ids: selectedIds.value })
    message.success(t('files.deleteSuccess'))
    window.dispatchEvent(new CustomEvent('refresh-quota'))
    selectedIds.value = []
    loadFiles()
  } catch (err: any) {
    message.error(err.response?.data?.error || t('files.deleteFailed'))
  }
}

const copyLink = (text: string) => {
  navigator.clipboard.writeText(text)
  message.success(t('common.copied'))
}

const formatBytes = (bytes: number) => {
  if (!bytes || bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

const formatDate = (isoStr: string) => {
  if (!isoStr) return ''
  const d = new Date(isoStr)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

onMounted(() => {
  loadFiles()
  loadTemplates()
  loadPolicies()
})
</script>

<style scoped>
.gallery-adaptive-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
}

.gallery-grid-card-wrapper {
  display: flex;
  flex-direction: column;
}

.masonry-container {
  column-count: 4;
  column-gap: 16px;
}

@media (max-width: 1400px) {
  .masonry-container {
    column-count: 3;
  }
}

@media (max-width: 900px) {
  .masonry-container {
    column-count: 2;
  }
}

@media (max-width: 550px) {
  .masonry-container {
    column-count: 1;
  }
}

.masonry-item {
  break-inside: avoid;
  margin-bottom: 16px;
}

@media (max-width: 768px) {
  .files-toolbar-row {
    flex-direction: column !important;
    align-items: stretch !important;
  }
  .files-filter-group {
    width: 100% !important;
  }
  .files-filter-group > * {
    flex: 1 1 100% !important;
    width: 100% !important;
  }
}
</style>
