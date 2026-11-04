<template>
  <div>
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px;">
      <div>
        <div style="font-size: 18px; font-weight: bold;">{{ $t('adminPolicies.title') }}</div>
        <div style="font-size: 13px; color: #888; margin-top: 4px;">{{ $t('adminPolicies.subtitle') }}</div>
      </div>
      <n-button type="primary" @click="openCreateModal">{{ $t('adminPolicies.new') }}</n-button>
    </div>

    <n-card style="border-radius: 12px;">
      <div class="table-responsive">
        <n-table :bordered="false" :single-line="false">
          <thead>
            <tr>
              <th style="width: 80px;"><SortableHeader title="ID" column="id" :current-sort-by="sortBy" :current-order="sortOrder" @sort="handleSort" /></th>
              <th><SortableHeader :title="$t('adminPolicies.policyName')" column="name" :current-sort-by="sortBy" :current-order="sortOrder" @sort="handleSort" /></th>
              <th><SortableHeader :title="$t('adminPolicies.driver')" column="driver" :current-sort-by="sortBy" :current-order="sortOrder" @sort="handleSort" /></th>
              <th><SortableHeader :title="$t('adminPolicies.maxCapacity')" column="max_capacity_bytes" :current-sort-by="sortBy" :current-order="sortOrder" @sort="handleSort" /></th>
              <th><SortableHeader :title="$t('adminPolicies.maxSingleFileSize')" column="max_single_file_size_mb" :current-sort-by="sortBy" :current-order="sortOrder" @sort="handleSort" /></th>
              <th><SortableHeader :title="$t('adminPolicies.usedSpace')" column="used_bytes" :current-sort-by="sortBy" :current-order="sortOrder" @sort="handleSort" /></th>
              <th>{{ $t('adminPolicies.pathPrefix') }}</th>
              <th>{{ $t('adminPolicies.namingRule') }}</th>
              <th>{{ $t('adminPolicies.permittedRoles') }}</th>
              <th><SortableHeader :title="$t('adminPolicies.isDefault')" column="is_default" :current-sort-by="sortBy" :current-order="sortOrder" @sort="handleSort" /></th>
              <th style="text-align: right;">{{ $t('common.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="p in policies" :key="p.id">
              <td>{{ p.id }}</td>
              <td><strong>{{ p.name }}</strong></td>
              <td>
                <n-tag size="tiny" type="info">{{ getDriverLabel(p.driver) }}</n-tag>
              </td>
              <td>{{ p.max_capacity_bytes === 0 ? $t('common.unlimited') : formatBytes(p.max_capacity_bytes) }}</td>
              <td>
                <span v-if="p.max_single_file_size_mb > 0">
                  {{ p.max_single_file_size_mb }} MB
                </span>
                <span v-else style="color: #888; font-size: 12px;">
                  {{ $t('adminPolicies.systemDefault') }}
                </span>
              </td>
              <td>{{ formatBytes(p.used_bytes) }}</td>
              <td><code>{{ formatPathPrefix(p.path_prefix) }}</code></td>
              <td><code>{{ p.naming_rule }}</code></td>
              <td>
                <div style="display: flex; flex-wrap: wrap; gap: 4px;">
                  <n-tag v-for="r in getRoleTags(p.role_ids)" :key="r.id" size="tiny" type="success">
                    {{ r.display_name }}
                  </n-tag>
                  <span v-if="!p.role_ids || p.role_ids.length === 0" style="color: #666; font-size: 12px;">-</span>
                </div>
              </td>
              <td>
                <n-tag :type="p.is_default ? 'success' : 'default'" size="small">
                  {{ p.is_default ? $t('common.yes') : $t('common.no') }}
                </n-tag>
              </td>
              <td style="text-align: right;">
                <div style="display: flex; gap: 8px; justify-content: flex-end;">
                  <n-button size="tiny" quaternary type="primary" @click="openEditModal(p)">
                    {{ $t('common.edit') }}
                  </n-button>
                  <n-popconfirm @positive-click="handleDeletePolicy(p.id)">
                    <template #trigger>
                      <n-button size="tiny" quaternary type="error" :disabled="p.is_default && policies.length === 1">
                        {{ $t('common.delete') }}
                      </n-button>
                    </template>
                    {{ $t('adminPolicies.confirmDelete') }}
                  </n-popconfirm>
                </div>
              </td>
            </tr>
          </tbody>
        </n-table>
      </div>
    </n-card>

    <!-- Create / Edit Policy Modal -->
    <n-modal
      v-model:show="showModal"
      preset="card"
      :title="isEditing ? $t('adminPolicies.editTitle') : $t('adminPolicies.new')"
      style="width: 660px; max-width: 95vw; border-radius: 12px;"
    >
      <n-form :model="policyForm" label-placement="left" label-width="130">
        <n-form-item :label="$t('adminPolicies.policyName')">
          <n-input v-model:value="policyForm.name" :placeholder="$t('adminPolicies.namePlaceholder')" />
        </n-form-item>

        <n-form-item :label="$t('adminPolicies.driver')">
          <n-select
            v-model:value="policyForm.driver"
            :options="driverOptions"
            :disabled="isEditing"
          />
        </n-form-item>

        <!-- Dynamic Driver Settings -->
        <n-card size="small" embedded style="margin-bottom: 18px; border-radius: 8px;">
          <template #header>
            <div style="display: flex; justify-content: space-between; align-items: center;">
              <span style="font-size: 13px; font-weight: bold;">{{ $t('adminPolicies.driverConfig') }}</span>
              <n-button size="tiny" secondary :loading="testingConnection" @click="handleTestConnection">
                {{ $t('adminPolicies.testConnection') }}
              </n-button>
            </div>
          </template>

          <!-- Local -->
          <div v-if="policyForm.driver === 'local'" style="font-size: 13px; color: #888; line-height: 1.6;">
            {{ $t('adminPolicies.driverLocalDesc') }}
          </div>

          <!-- Database -->
          <div v-else-if="policyForm.driver === 'database'">
            <div style="font-size: 13px; color: #888; line-height: 1.6;">
              {{ $t('adminPolicies.driverDatabaseDesc') }}
            </div>
          </div>

          <!-- WebDAV -->
          <div v-else-if="policyForm.driver === 'webdav'" style="display: flex; flex-direction: column; gap: 12px;">
            <n-form-item :label="$t('adminPolicies.serviceEndpoint')" :show-feedback="false">
              <n-input v-model:value="driverConfig.webdav_endpoint" placeholder="https://dav.example.com" />
            </n-form-item>
            <n-form-item :label="$t('auth.username')" :show-feedback="false">
              <n-input v-model:value="driverConfig.webdav_username" />
            </n-form-item>
            <n-form-item :label="$t('auth.password')" :show-feedback="false">
              <n-input v-model:value="driverConfig.webdav_password" type="password" show-password-on="click" />
            </n-form-item>
            <n-form-item :label="$t('adminPolicies.directUrl')" :show-feedback="false">
              <n-input v-model:value="driverConfig.webdav_direct_url" placeholder="可选，直链访问域名" />
            </n-form-item>
          </div>

          <!-- S3 Compatible / S3 / MinIO -->
          <div v-else-if="policyForm.driver === 's3' || policyForm.driver === 's3_compatible' || policyForm.driver === 'minio'" style="display: flex; flex-direction: column; gap: 12px;">
            <n-form-item v-if="policyForm.driver !== 's3'" label="Endpoint" :show-feedback="false">
              <n-input v-model:value="driverConfig.s3_endpoint" placeholder="https://s3.example.com" />
            </n-form-item>
            <n-form-item label="Bucket" :show-feedback="false">
              <n-input v-model:value="driverConfig.s3_bucket" />
            </n-form-item>
            <n-form-item label="Region" :show-feedback="false">
              <n-input v-model:value="driverConfig.s3_region" placeholder="us-east-1" />
            </n-form-item>
            <n-form-item label="AccessKey" :show-feedback="false">
              <n-input v-model:value="driverConfig.s3_access_key_id" />
            </n-form-item>
            <n-form-item label="SecretKey" :show-feedback="false">
              <n-input v-model:value="driverConfig.s3_secret_access_key" type="password" show-password-on="click" />
            </n-form-item>
            <n-form-item :label="$t('adminPolicies.customDomain')" :show-feedback="false">
              <n-input v-model:value="driverConfig.s3_custom_domain" placeholder="可选，如 https://cdn.example.com" />
            </n-form-item>
            <n-form-item v-if="policyForm.driver !== 's3'" :label="$t('adminPolicies.forcePathStyle')" :show-feedback="false">
              <n-switch v-model:value="driverConfig.s3_use_path_style" />
            </n-form-item>
          </div>

          <!-- Aliyun OSS -->
          <div v-else-if="policyForm.driver === 'oss'" style="display: flex; flex-direction: column; gap: 12px;">
            <n-form-item label="Endpoint" :show-feedback="false">
              <n-input v-model:value="driverConfig.oss_endpoint" placeholder="如 oss-cn-hangzhou.aliyuncs.com 或 oss-ap-northeast-1.aliyuncs.com" />
            </n-form-item>
            <n-form-item label="Bucket" :show-feedback="false">
              <n-input v-model:value="driverConfig.oss_bucket" />
            </n-form-item>
            <n-form-item label="AccessKey ID" :show-feedback="false">
              <n-input v-model:value="driverConfig.oss_access_key_id" />
            </n-form-item>
            <n-form-item label="AccessKey Secret" :show-feedback="false">
              <n-input v-model:value="driverConfig.oss_secret_access_key" type="password" show-password-on="click" />
            </n-form-item>
            <n-form-item :label="$t('adminPolicies.customDomain')" :show-feedback="false">
              <n-input v-model:value="driverConfig.oss_custom_domain" placeholder="可选，如 https://cdn.example.com" />
            </n-form-item>
          </div>

          <!-- Tencent Cloud COS -->
          <div v-else-if="policyForm.driver === 'cos'" style="display: flex; flex-direction: column; gap: 12px;">
            <n-form-item label="Bucket" :show-feedback="false">
              <n-input v-model:value="driverConfig.cos_bucket" placeholder="example-1250000000" />
            </n-form-item>
            <n-form-item label="Region" :show-feedback="false">
              <n-input v-model:value="driverConfig.cos_region" placeholder="ap-guangzhou" />
            </n-form-item>
            <n-form-item label="SecretId" :show-feedback="false">
              <n-input v-model:value="driverConfig.cos_secret_id" />
            </n-form-item>
            <n-form-item label="SecretKey" :show-feedback="false">
              <n-input v-model:value="driverConfig.cos_secret_key" type="password" show-password-on="click" />
            </n-form-item>
            <n-form-item :label="$t('adminPolicies.customDomain')" :show-feedback="false">
              <n-input v-model:value="driverConfig.cos_custom_domain" placeholder="可选" />
            </n-form-item>
          </div>

          <!-- Huawei Cloud OBS -->
          <div v-else-if="policyForm.driver === 'obs'" style="display: flex; flex-direction: column; gap: 12px;">
            <n-form-item label="Endpoint" :show-feedback="false">
              <n-input v-model:value="driverConfig.obs_endpoint" placeholder="obs.cn-north-4.myhuaweicloud.com" />
            </n-form-item>
            <n-form-item label="Bucket" :show-feedback="false">
              <n-input v-model:value="driverConfig.obs_bucket" />
            </n-form-item>
            <n-form-item label="AccessKey" :show-feedback="false">
              <n-input v-model:value="driverConfig.obs_access_key_id" />
            </n-form-item>
            <n-form-item label="SecretKey" :show-feedback="false">
              <n-input v-model:value="driverConfig.obs_secret_access_key" type="password" show-password-on="click" />
            </n-form-item>
            <n-form-item :label="$t('adminPolicies.customDomain')" :show-feedback="false">
              <n-input v-model:value="driverConfig.obs_custom_domain" placeholder="可选" />
            </n-form-item>
          </div>

          <!-- Google Cloud Storage -->
          <div v-else-if="policyForm.driver === 'gcs'" style="display: flex; flex-direction: column; gap: 12px;">
            <n-form-item label="Bucket" :show-feedback="false">
              <n-input v-model:value="driverConfig.gcs_bucket" />
            </n-form-item>
            <n-form-item :label="$t('adminPolicies.serviceAccountKey')" :show-feedback="false">
              <n-input
                v-model:value="driverConfig.gcs_credentials_json"
                type="textarea"
                :rows="3"
                placeholder="Service Account 密钥 JSON 文本"
              />
            </n-form-item>
            <n-form-item :label="$t('adminPolicies.customDomain')" :show-feedback="false">
              <n-input v-model:value="driverConfig.gcs_custom_domain" placeholder="可选" />
            </n-form-item>
          </div>

          <!-- Azure Blob -->
          <div v-else-if="policyForm.driver === 'azure_blob'" style="display: flex; flex-direction: column; gap: 12px;">
            <n-form-item label="Container" :show-feedback="false">
              <n-input v-model:value="driverConfig.azure_container_name" />
            </n-form-item>
            <n-form-item :label="$t('adminPolicies.connectionString')" :show-feedback="false">
              <n-input v-model:value="driverConfig.azure_connection_string" placeholder="完整连接字符串，或在下方分别填写账号与密钥" />
            </n-form-item>
            <n-form-item label="Account Name" :show-feedback="false">
              <n-input v-model:value="driverConfig.azure_account_name" />
            </n-form-item>
            <n-form-item label="Account Key" :show-feedback="false">
              <n-input v-model:value="driverConfig.azure_account_key" type="password" show-password-on="click" />
            </n-form-item>
            <n-form-item :label="$t('adminPolicies.customDomain')" :show-feedback="false">
              <n-input v-model:value="driverConfig.azure_custom_domain" placeholder="可选" />
            </n-form-item>
          </div>

          <!-- GitHub Repository -->
          <div v-else-if="policyForm.driver === 'github'" style="display: flex; flex-direction: column; gap: 12px;">
            <n-form-item label="Owner" :show-feedback="false">
              <n-input v-model:value="driverConfig.github_owner" placeholder="用户名或组织名" />
            </n-form-item>
            <n-form-item label="Repository" :show-feedback="false">
              <n-input v-model:value="driverConfig.github_repo" placeholder="仓库名称" />
            </n-form-item>
            <n-form-item label="Branch" :show-feedback="false">
              <n-input v-model:value="driverConfig.github_branch" placeholder="main" />
            </n-form-item>
            <n-form-item label="Token" :show-feedback="false">
              <n-input v-model:value="driverConfig.github_token" type="password" show-password-on="click" placeholder="Personal Access Token" />
            </n-form-item>
            <n-form-item :label="$t('adminPolicies.githubCdn')" :show-feedback="false">
              <n-select
                v-model:value="driverConfig.github_cdn_type"
                :options="[
                  { label: 'jsDelivr', value: 'jsdelivr' },
                  { label: 'GitHub Raw', value: 'raw' },
                  { label: $t('adminPolicies.customCdn'), value: 'custom' }
                ]"
              />
            </n-form-item>
            <n-form-item v-if="driverConfig.github_cdn_type === 'custom'" :label="$t('adminPolicies.customCdnUrl')" :show-feedback="false">
              <n-input v-model:value="driverConfig.github_cdn_custom_url" placeholder="https://cdn.example.com" />
            </n-form-item>
          </div>

          <!-- OneDrive -->
          <div v-else-if="policyForm.driver === 'onedrive'" style="display: flex; flex-direction: column; gap: 12px;">
            <div style="background: rgba(255, 255, 255, 0.03); border: 1px solid rgba(255, 255, 255, 0.08); border-radius: 8px; padding: 12px;">
              <div style="display: flex; justify-content: space-between; align-items: center;">
                <div>
                  <div style="font-weight: 500; font-size: 13px;">{{ $t('adminPolicies.onedriveQuickAuth') }}</div>
                  <div style="font-size: 12px; color: #888; margin-top: 2px;">{{ $t('adminPolicies.onedriveQuickAuthDesc') }}</div>
                </div>
                <n-button type="primary" secondary size="small" :loading="startingOneDriveAuth" @click="handleStartOneDriveAuth">
                  {{ $t('adminPolicies.onedriveAuthBtn') }}
                </n-button>
              </div>
              <div v-if="driverConfig.onedrive_refresh_token" style="font-size: 12px; color: #63e2b7; display: flex; align-items: center; gap: 4px; margin-top: 8px;">
                <n-icon size="14"><CheckmarkCircleOutline /></n-icon>
                <span>{{ $t('adminPolicies.onedriveAuthorized') }}</span>
              </div>
            </div>

            <n-form-item :label="$t('adminPolicies.accountType')" :show-feedback="false">
              <n-select
                v-model:value="driverConfig.onedrive_account_type"
                :options="[
                  { label: $t('adminPolicies.accountPersonal'), value: 'personal' },
                  { label: $t('adminPolicies.accountBusiness'), value: 'business' },
                  { label: $t('adminPolicies.accountChina'), value: 'china' }
                ]"
              />
            </n-form-item>
            <n-form-item label="Client ID" :show-feedback="false">
              <n-input v-model:value="driverConfig.onedrive_client_id" placeholder="使用默认内置公共客户端或自定义 Azure AD App" />
            </n-form-item>
            <n-form-item label="Client Secret" :show-feedback="false">
              <n-input v-model:value="driverConfig.onedrive_client_secret" type="password" show-password-on="click" placeholder="可选，公共客户端留空即可" />
            </n-form-item>
            <n-form-item label="Redirect URI" :show-feedback="false">
              <n-input v-model:value="driverConfig.onedrive_redirect_uri" placeholder="可选" />
            </n-form-item>
            <n-form-item label="Refresh Token" :show-feedback="false">
              <n-input v-model:value="driverConfig.onedrive_refresh_token" type="password" show-password-on="click" placeholder="点击上方一键授权自动填充，或手动填入" />
            </n-form-item>
          </div>
        </n-card>

        <!-- Capacity Limit -->
        <n-form-item :label="$t('adminPolicies.maxCapacity')">
          <div style="display: flex; align-items: center; gap: 10px; width: 100%;">
            <n-input-number
              v-model:value="capacityValue"
              :min="0"
              :step="1"
              :placeholder="$t('adminPolicies.capacityPlaceholder')"
              style="flex: 1;"
            />
            <n-select
              v-model:value="capacityUnit"
              :options="capacityUnitOptions"
              style="width: 90px;"
            />
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-icon size="16" style="color: #888; cursor: pointer;">
                  <InformationCircleOutline />
                </n-icon>
              </template>
              {{ $t('adminPolicies.capacityUnlimitedTip') }}
            </n-tooltip>
          </div>
        </n-form-item>

        <!-- Max Single File Size -->
        <n-form-item :label="$t('adminPolicies.maxSingleFileSize')">
          <div style="display: flex; align-items: center; gap: 10px; width: 100%;">
            <n-input-number
              v-model:value="policyForm.max_single_file_size_mb"
              :min="0"
              :step="1"
              placeholder="0"
              style="flex: 1;"
            />
            <span style="color: #aaa; font-size: 13px;">MB</span>
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-icon size="16" style="color: #888; cursor: pointer;">
                  <InformationCircleOutline />
                </n-icon>
              </template>
              {{ $t('adminPolicies.maxSingleFileTip') }}
            </n-tooltip>
          </div>
        </n-form-item>

        <n-form-item>
          <template #label>
            <div style="display: flex; align-items: center; gap: 4px;">
              <span>{{ $t('adminPolicies.pathPrefix') }}</span>
              <n-tooltip trigger="hover">
                <template #trigger>
                  <n-icon size="16" style="color: #888; cursor: pointer;">
                    <InformationCircleOutline />
                  </n-icon>
                </template>
                {{ $t('adminPolicies.pathPrefixTip') }}
              </n-tooltip>
            </div>
          </template>
          <div style="width: 100%;">
            <n-input v-model:value="policyForm.path_prefix" placeholder="留空为根路径，或输入前缀如 uploads" clearable />
            <div style="display: flex; flex-wrap: wrap; gap: 6px; margin-top: 8px; align-items: center;">
              <span style="font-size: 12px; color: #888;">{{ $t('adminPolicies.insertToken') }}:</span>
              <n-button size="tiny" secondary type="warning" @click="policyForm.path_prefix = ''">
                {{ $t('adminPolicies.rootDirect') }}
              </n-button>
              <n-button size="tiny" secondary type="info" @click="policyForm.path_prefix = 'uploads'">
                uploads
              </n-button>
              <n-button size="tiny" secondary type="info" @click="insertPathPrefixToken('{username}')">
                {username} · {{ $t('adminPolicies.tokenUsername') }}
              </n-button>
              <n-button size="tiny" secondary type="info" @click="insertPathPrefixToken('{uid}')">
                {uid} · {{ $t('adminPolicies.tokenUid') }}
              </n-button>
              <n-button size="tiny" secondary type="info" @click="insertPathPrefixToken('{role}')">
                {role} · {{ $t('adminPolicies.tokenRole') }}
              </n-button>
              <n-button size="tiny" secondary type="info" @click="insertPathPrefixToken('{year}')">
                {year} · {{ $t('adminPolicies.tokenYear') }}
              </n-button>
              <n-button size="tiny" secondary type="info" @click="insertPathPrefixToken('{month}')">
                {month} · {{ $t('adminPolicies.tokenMonth') }}
              </n-button>
              <n-button size="tiny" secondary type="info" @click="insertPathPrefixToken('{day}')">
                {day} · {{ $t('adminPolicies.tokenDay') }}
              </n-button>
              <n-button size="tiny" secondary type="info" @click="insertPathPrefixToken('{hour}')">
                {hour} · {{ $t('adminPolicies.tokenHour') }}
              </n-button>
            </div>
          </div>
        </n-form-item>

        <n-form-item :label="$t('adminPolicies.namingRule')">
          <div style="width: 100%;">
            <n-input v-model:value="policyForm.naming_rule" placeholder="{Y}/{m}/{d}/{random:16}.{ext}" />
            <div style="display: flex; flex-wrap: wrap; gap: 6px; margin-top: 8px; align-items: center;">
              <span style="font-size: 12px; color: #888;">{{ $t('adminPolicies.insertToken') }}:</span>
              <n-button size="tiny" secondary type="success" @click="insertNamingRuleToken('{Y}')">
                {Y} · {{ $t('adminPolicies.tokenYear') }}
              </n-button>
              <n-button size="tiny" secondary type="success" @click="insertNamingRuleToken('{m}')">
                {m} · {{ $t('adminPolicies.tokenMonth') }}
              </n-button>
              <n-button size="tiny" secondary type="success" @click="insertNamingRuleToken('{d}')">
                {d} · {{ $t('adminPolicies.tokenDay') }}
              </n-button>
              <n-button size="tiny" secondary type="success" @click="insertNamingRuleToken('{H}')">
                {H} · {{ $t('adminPolicies.tokenHour') }}
              </n-button>
              <n-button size="tiny" secondary type="success" @click="insertNamingRuleToken('{i}')">
                {i} · {{ $t('adminPolicies.tokenMinute') }}
              </n-button>
              <n-button size="tiny" secondary type="success" @click="insertNamingRuleToken('{s}')">
                {s} · {{ $t('adminPolicies.tokenSecond') }}
              </n-button>
              <n-button size="tiny" secondary type="success" @click="insertNamingRuleToken('{random:16}')">
                {random:16} · {{ $t('adminPolicies.tokenRandom') }}
              </n-button>
              <n-button size="tiny" secondary type="success" @click="insertNamingRuleToken('{filename}')">
                {filename} · {{ $t('adminPolicies.tokenFilename') }}
              </n-button>
              <n-button size="tiny" secondary type="success" @click="insertNamingRuleToken('{ext}')">
                {ext} · {{ $t('adminPolicies.tokenExt') }}
              </n-button>
              <n-button size="tiny" secondary type="success" @click="insertNamingRuleToken('{timestamp}')">
                {timestamp} · {{ $t('adminPolicies.tokenTimestamp') }}
              </n-button>
              <n-button size="tiny" secondary type="success" @click="insertNamingRuleToken('{sha256}')">
                {sha256} · {{ $t('adminPolicies.tokenSha256') }}
              </n-button>
            </div>
          </div>
        </n-form-item>

        <n-form-item :label="$t('adminPolicies.permittedRoles')">
          <div style="width: 100%;">
            <n-checkbox-group v-model:value="policyForm.role_ids">
              <div class="role-checkbox-container">
                <n-checkbox
                  v-for="r in roles"
                  :key="r.id"
                  :value="r.id"
                  class="role-checkbox-card"
                >
                  <div class="role-checkbox-text">
                    <span class="role-checkbox-title">{{ r.display_name }}</span>
                    <span class="role-checkbox-sub">· {{ r.name }}</span>
                  </div>
                </n-checkbox>
              </div>
            </n-checkbox-group>
            <div style="font-size: 12px; color: #888; margin-top: 6px;">
              {{ $t('adminPolicies.guestSinglePolicyTip') }}
            </div>
          </div>
        </n-form-item>

        <n-form-item :label="$t('adminPolicies.isDefault')">
          <n-switch v-model:value="policyForm.is_default" />
        </n-form-item>
      </n-form>

      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 12px;">
          <n-button @click="showModal = false">{{ $t('common.cancel') }}</n-button>
          <n-button type="primary" :loading="saving" @click="handleSavePolicy">{{ $t('common.save') }}</n-button>
        </div>
      </template>
    </n-modal>

    <!-- OneDrive Device Code Auth Modal -->
    <n-modal
      v-model:show="showOneDriveModal"
      preset="card"
      :title="$t('adminPolicies.onedriveAuthTitle')"
      style="width: 520px; max-width: 95vw; border-radius: 12px;"
      :mask-closable="false"
      :closable="!oneDrivePolling"
    >
      <div v-if="oneDriveAuthData" style="display: flex; flex-direction: column; gap: 16px;">
        <div style="font-size: 13px; color: #aaa; line-height: 1.6;">
          {{ $t('adminPolicies.onedriveAuthStep1') }}
        </div>

        <div style="background: rgba(255, 255, 255, 0.04); border: 1px solid rgba(255, 255, 255, 0.1); border-radius: 8px; padding: 16px; text-align: center;">
          <div style="font-size: 12px; color: #888; margin-bottom: 6px;">{{ $t('adminPolicies.onedriveDeviceCode') }}</div>
          <div style="font-size: 26px; font-weight: bold; letter-spacing: 4px; color: #63e2b7; user-select: all;">
            {{ oneDriveAuthData.user_code }}
          </div>
        </div>

        <div style="display: flex; gap: 12px; justify-content: center;">
          <n-button type="primary" @click="handleOpenOneDriveLogin">
            <template #icon>
              <n-icon><OpenOutline /></n-icon>
            </template>
            {{ $t('adminPolicies.onedriveOpenAndCopy') }}
          </n-button>
        </div>

        <div style="display: flex; align-items: center; justify-content: center; gap: 8px; margin-top: 8px; font-size: 13px; color: #888;">
          <n-spin size="small" />
          <span>{{ $t('adminPolicies.onedriveWaiting') }}</span>
        </div>
      </div>

      <template #footer>
        <div style="display: flex; justify-content: flex-end;">
          <n-button @click="handleCancelOneDriveAuth">{{ $t('common.cancel') }}</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useMessage } from 'naive-ui'
import { InformationCircleOutline, CheckmarkCircleOutline, OpenOutline } from '@vicons/ionicons5'
import { useI18n } from 'vue-i18n'
import { api } from '../api/client'
import SortableHeader from '../components/SortableHeader.vue'

const message = useMessage()
const { t } = useI18n()
const policies = ref<any[]>([])
const roles = ref<any[]>([])

const sortBy = ref<string>('id')
const sortOrder = ref<'asc' | 'desc' | null>('asc')

const handleSort = (key: string, order: 'asc' | 'desc' | null) => {
  sortBy.value = key
  sortOrder.value = order
  loadData()
}

const showModal = ref(false)
const isEditing = ref(false)
const editTargetId = ref<number | null>(null)
const saving = ref(false)
const testingConnection = ref(false)
const capacityValue = ref<number | null>(0)
const capacityUnit = ref<'KB' | 'MB' | 'GB' | 'TB'>('GB')
const capacityUnitOptions = [
  { label: 'KB', value: 'KB' },
  { label: 'MB', value: 'MB' },
  { label: 'GB', value: 'GB' },
  { label: 'TB', value: 'TB' }
]

const unitMultiplier: Record<string, number> = {
  KB: 1024,
  MB: 1024 * 1024,
  GB: 1024 * 1024 * 1024,
  TB: 1024 * 1024 * 1024 * 1024
}

// OneDrive Device Code Auth State
const startingOneDriveAuth = ref(false)
const showOneDriveModal = ref(false)
const oneDrivePolling = ref(false)
const oneDriveAuthData = ref<{ user_code: string; verification_uri: string; device_code: string; expires_in: number; interval?: number } | null>(null)
let oneDrivePollTimer: any = null

const policyForm = ref({
  name: '',
  driver: 'local',
  path_prefix: 'uploads',
  naming_rule: '{Y}/{m}/{d}/{random:16}.{ext}',
  max_single_file_size_mb: 0,
  is_default: false,
  role_ids: [] as number[]
})

const driverConfig = ref<any>({
  base_path: 'data/storage',
  local_sub_path: 'storage',
  webdav_endpoint: '',
  webdav_username: '',
  webdav_password: '',
  webdav_root_path: '',
  webdav_direct_url: '',
  s3_endpoint: '',
  s3_region: '',
  s3_bucket: '',
  s3_access_key_id: '',
  s3_secret_access_key: '',
  s3_custom_domain: '',
  s3_root_dir: '',
  s3_use_path_style: false,
  s3_use_ssl: true,
  oss_endpoint: '',
  oss_bucket: '',
  oss_access_key_id: '',
  oss_secret_access_key: '',
  oss_custom_domain: '',
  oss_root_dir: '',
  cos_bucket: '',
  cos_region: '',
  cos_secret_id: '',
  cos_secret_key: '',
  cos_custom_domain: '',
  cos_root_dir: '',
  obs_endpoint: '',
  obs_bucket: '',
  obs_access_key_id: '',
  obs_secret_access_key: '',
  obs_custom_domain: '',
  obs_root_dir: '',
  gcs_bucket: '',
  gcs_credentials_json: '',
  gcs_custom_domain: '',
  gcs_root_dir: '',
  azure_account_name: '',
  azure_account_key: '',
  azure_container_name: '',
  azure_connection_string: '',
  azure_custom_domain: '',
  azure_root_dir: '',
  github_owner: '',
  github_repo: '',
  github_branch: 'main',
  github_token: '',
  github_path_prefix: '',
  github_cdn_type: 'jsdelivr',
  github_cdn_custom_url: '',
  onedrive_account_type: 'personal',
  onedrive_client_id: '',
  onedrive_client_secret: '',
  onedrive_redirect_uri: '',
  onedrive_refresh_token: '',
  onedrive_base_path: ''
})

const driverOptions = computed(() => [
  { label: `${t('adminPolicies.driverLocal')} · local`, value: 'local' },
  { label: `${t('adminPolicies.driverDatabase')} · database`, value: 'database' },
  { label: `${t('adminPolicies.driverWebdav')} · webdav`, value: 'webdav' },
  { label: `${t('adminPolicies.driverS3')} · s3_compatible`, value: 's3_compatible' },
  { label: `${t('adminPolicies.s3Standard')} · s3`, value: 's3' },
  { label: `${t('adminPolicies.minio')} · minio`, value: 'minio' },
  { label: `${t('adminPolicies.driverOss')} · oss`, value: 'oss' },
  { label: `${t('adminPolicies.driverCos')} · cos`, value: 'cos' },
  { label: `${t('adminPolicies.driverObs')} · obs`, value: 'obs' },
  { label: `${t('adminPolicies.driverGcs')} · gcs`, value: 'gcs' },
  { label: `${t('adminPolicies.driverAzure')} · azure_blob`, value: 'azure_blob' },
  { label: `${t('adminPolicies.driverGithub')} · github`, value: 'github' },
  { label: `${t('adminPolicies.driverOnedrive')} · onedrive`, value: 'onedrive' }
])

const getDriverLabel = (driver: string) => {
  const match = driverOptions.value.find(d => d.value === driver)
  return match ? match.label.split(' · ')[0] : driver
}

const roleOptions = computed(() => {
  return roles.value.map(r => ({
    label: `${r.display_name} - ${r.name}`,
    value: r.id
  }))
})

const insertPathPrefixToken = (token: string) => {
  if (!policyForm.value.path_prefix) {
    policyForm.value.path_prefix = token
  } else if (policyForm.value.path_prefix.endsWith('/')) {
    policyForm.value.path_prefix += token
  } else {
    policyForm.value.path_prefix += '/' + token
  }
}

const insertNamingRuleToken = (token: string) => {
  if (!policyForm.value.naming_rule) {
    policyForm.value.naming_rule = token
  } else {
    policyForm.value.naming_rule += token
  }
}

const getRoleTags = (roleIds: number[]) => {
  if (!roleIds) return []
  return roles.value.filter(r => roleIds.includes(r.id))
}

const loadData = async () => {
  try {
    const params: any = {}
    if (sortBy.value && sortOrder.value) {
      params.sort_by = sortBy.value
      params.order = sortOrder.value
    }
    const [pRes, rRes] = await Promise.all([
      api.get('/admin/policies', { params }),
      api.get('/admin/roles')
    ])
    policies.value = pRes.data || []
    roles.value = rRes.data || []
  } catch {}
}

const formatPathPrefix = (prefix: string | undefined | null) => {
  if (prefix === undefined || prefix === null || prefix.trim() === '' || prefix.trim() === '/') {
    return '/'
  }
  const clean = prefix.replace(/^\/+|\/+$/g, '')
  return '/' + clean
}

const buildConfigJSON = (driver: string): string => {
  const c = driverConfig.value
  switch (driver) {
    case 'local':
      return JSON.stringify({ base_path: 'data/storage' })
    case 'database':
      return '{}'
    case 'webdav':
      return JSON.stringify({
        endpoint: c.webdav_endpoint,
        username: c.webdav_username,
        password: c.webdav_password,
        root_path: c.webdav_root_path,
        direct_url: c.webdav_direct_url
      })
    case 's3_compatible':
    case 'minio':
      return JSON.stringify({
        endpoint: c.s3_endpoint,
        region: c.s3_region,
        bucket: c.s3_bucket,
        access_key_id: c.s3_access_key_id,
        secret_access_key: c.s3_secret_access_key,
        custom_domain: c.s3_custom_domain,
        root_dir: c.s3_root_dir,
        use_path_style: c.s3_use_path_style,
        use_ssl: c.s3_use_ssl
      })
    case 's3':
      return JSON.stringify({
        region: c.s3_region,
        bucket: c.s3_bucket,
        access_key_id: c.s3_access_key_id,
        secret_access_key: c.s3_secret_access_key,
        custom_domain: c.s3_custom_domain,
        root_dir: c.s3_root_dir,
      })
    case 'oss':
      return JSON.stringify({
        endpoint: c.oss_endpoint,
        bucket: c.oss_bucket,
        access_key_id: c.oss_access_key_id,
        secret_access_key: c.oss_secret_access_key,
        custom_domain: c.oss_custom_domain,
        root_dir: c.oss_root_dir,
      })
    case 'cos':
      return JSON.stringify({
        bucket: c.cos_bucket,
        region: c.cos_region,
        secret_id: c.cos_secret_id,
        secret_key: c.cos_secret_key,
        custom_domain: c.cos_custom_domain,
        root_dir: c.cos_root_dir,
      })
    case 'obs':
      return JSON.stringify({
        endpoint: c.obs_endpoint,
        bucket: c.obs_bucket,
        access_key_id: c.obs_access_key_id,
        secret_access_key: c.obs_secret_access_key,
        custom_domain: c.obs_custom_domain,
        root_dir: c.obs_root_dir,
      })
    case 'gcs':
      return JSON.stringify({
        bucket: c.gcs_bucket,
        credentials_json: c.gcs_credentials_json,
        custom_domain: c.gcs_custom_domain,
        root_dir: c.gcs_root_dir,
      })
    case 'azure_blob':
      return JSON.stringify({
        account_name: c.azure_account_name,
        account_key: c.azure_account_key,
        container_name: c.azure_container_name,
        connection_string: c.azure_connection_string,
        custom_domain: c.azure_custom_domain,
        root_dir: c.azure_root_dir,
      })
    case 'github':
      return JSON.stringify({
        owner: c.github_owner,
        repo: c.github_repo,
        branch: c.github_branch || 'main',
        token: c.github_token,
        path_prefix: c.github_path_prefix,
        cdn_type: c.github_cdn_type || 'jsdelivr',
        cdn_custom_url: c.github_cdn_custom_url
      })
    case 'onedrive':
      return JSON.stringify({
        account_type: c.onedrive_account_type || 'personal',
        client_id: c.onedrive_client_id,
        client_secret: c.onedrive_client_secret,
        redirect_uri: c.onedrive_redirect_uri,
        refresh_token: c.onedrive_refresh_token,
        base_path: c.onedrive_base_path,
      })
    default:
      return '{}'
  }
}

const parseConfigJSON = (driver: string, raw: string) => {
  if (!raw) return
  try {
    const data = JSON.parse(raw)
    const c = driverConfig.value
    switch (driver) {
      case 'local': {
        const full = data.base_path || 'data/storage'
        c.base_path = full
        c.local_sub_path = full.startsWith('data/') ? full.substring(5) : (full === 'data' ? '' : full)
        break
      }
      case 'webdav':
        c.webdav_endpoint = data.endpoint || data.url || ''
        c.webdav_username = data.username || ''
        c.webdav_password = data.password || ''
        c.webdav_root_path = data.root_path || data.base_path || ''
        c.webdav_direct_url = data.direct_url || data.public_url_prefix || data.custom_domain || ''
        break
      case 's3':
      case 's3_compatible':
      case 'minio':
        c.s3_endpoint = data.endpoint || ''
        c.s3_region = data.region || ''
        c.s3_bucket = data.bucket || ''
        c.s3_access_key_id = data.access_key_id || ''
        c.s3_secret_access_key = data.secret_access_key || data.secret_key || ''
        c.s3_custom_domain = data.custom_domain || data.public_url_prefix || ''
        c.s3_root_dir = data.root_dir || data.base_path || data.root_path || ''
        c.s3_use_path_style = Boolean(data.use_path_style || data.force_path_style)
        c.s3_use_ssl = data.use_ssl !== false
        break
      case 'oss':
        c.oss_endpoint = data.endpoint || ''
        c.oss_bucket = data.bucket || ''
        c.oss_access_key_id = data.access_key_id || ''
        c.oss_secret_access_key = data.secret_access_key || data.access_key_secret || ''
        c.oss_custom_domain = data.custom_domain || data.public_url_prefix || ''
        c.oss_root_dir = data.root_dir || data.base_path || data.root_path || ''
        break
      case 'cos':
        c.cos_bucket = data.bucket || ''
        c.cos_region = data.region || ''
        c.cos_secret_id = data.secret_id || data.access_key_id || ''
        c.cos_secret_key = data.secret_key || data.secret_access_key || ''
        c.cos_custom_domain = data.custom_domain || data.public_url_prefix || ''
        c.cos_root_dir = data.root_dir || data.base_path || data.root_path || ''
        break
      case 'obs':
        c.obs_endpoint = data.endpoint || ''
        c.obs_bucket = data.bucket || ''
        c.obs_access_key_id = data.access_key_id || data.access_key || ''
        c.obs_secret_access_key = data.secret_access_key || data.security_key || data.secret_key || ''
        c.obs_custom_domain = data.custom_domain || data.public_url_prefix || ''
        c.obs_root_dir = data.root_dir || data.base_path || data.root_path || ''
        break
      case 'gcs':
        c.gcs_bucket = data.bucket || ''
        c.gcs_credentials_json = data.credentials_json || ''
        c.gcs_custom_domain = data.custom_domain || data.public_url_prefix || ''
        c.gcs_root_dir = data.root_dir || data.base_path || data.root_path || ''
        break
      case 'azure_blob':
        c.azure_account_name = data.account_name || ''
        c.azure_account_key = data.account_key || ''
        c.azure_container_name = data.container_name || ''
        c.azure_connection_string = data.connection_string || ''
        c.azure_custom_domain = data.custom_domain || data.public_url_prefix || ''
        c.azure_root_dir = data.root_dir || data.base_path || data.root_path || ''
        break
      case 'github':
        c.github_owner = data.owner || ''
        c.github_repo = data.repo || ''
        c.github_branch = data.branch || 'main'
        c.github_token = data.token || ''
        c.github_path_prefix = data.path_prefix || ''
        c.github_cdn_type = data.cdn_type || 'jsdelivr'
        c.github_cdn_custom_url = data.cdn_custom_url || data.custom_domain || ''
        break
      case 'onedrive':
        c.onedrive_account_type = data.account_type || 'personal'
        c.onedrive_client_id = data.client_id || ''
        c.onedrive_client_secret = data.client_secret || ''
        c.onedrive_redirect_uri = data.redirect_uri || ''
        c.onedrive_refresh_token = data.refresh_token || ''
        c.onedrive_base_path = data.base_path || ''
        break
    }
  } catch {}
}

const handleStartOneDriveAuth = async () => {
  startingOneDriveAuth.value = true
  try {
    const res = await api.post('/admin/policies/onedrive/device-code', {
      client_id: driverConfig.value.onedrive_client_id,
      account_type: driverConfig.value.onedrive_account_type || 'personal'
    })
    oneDriveAuthData.value = res.data
    showOneDriveModal.value = true
    oneDrivePolling.value = true

    const interval = Math.max(res.data.interval || 5, 3) * 1000
    const expiresAt = Date.now() + (res.data.expires_in || 900) * 1000

    if (oneDrivePollTimer) clearInterval(oneDrivePollTimer)
    oneDrivePollTimer = setInterval(async () => {
      if (Date.now() > expiresAt) {
        clearInterval(oneDrivePollTimer)
        oneDrivePollTimer = null
        oneDrivePolling.value = false
        message.error(t('adminPolicies.onedriveTimeout'))
        return
      }

      try {
        const tokenRes = await api.post('/admin/policies/onedrive/device-token', {
          client_id: driverConfig.value.onedrive_client_id,
          account_type: driverConfig.value.onedrive_account_type || 'personal',
          device_code: oneDriveAuthData.value?.device_code
        })
        if (tokenRes.data && tokenRes.data.refresh_token) {
          driverConfig.value.onedrive_refresh_token = tokenRes.data.refresh_token
          clearInterval(oneDrivePollTimer)
          oneDrivePollTimer = null
          oneDrivePolling.value = false
          showOneDriveModal.value = false
          message.success(t('adminPolicies.onedriveSuccess'))
        }
      } catch (pollErr: any) {
        const errType = pollErr.response?.data?.error
        if (errType === 'authorization_pending') {
          return
        }
        clearInterval(oneDrivePollTimer)
        oneDrivePollTimer = null
        oneDrivePolling.value = false
        message.error(pollErr.response?.data?.error || t('common.failed'))
      }
    }, interval)
  } catch (err: any) {
    message.error(err.response?.data?.error || t('common.failed'))
  } finally {
    startingOneDriveAuth.value = false
  }
}

const handleOpenOneDriveLogin = () => {
  if (!oneDriveAuthData.value) return
  navigator.clipboard.writeText(oneDriveAuthData.value.user_code)
  message.success(t('common.copied'))
  window.open(oneDriveAuthData.value.verification_uri, '_blank')
}

const handleCancelOneDriveAuth = () => {
  if (oneDrivePollTimer) {
    clearInterval(oneDrivePollTimer)
    oneDrivePollTimer = null
  }
  oneDrivePolling.value = false
  showOneDriveModal.value = false
}

const handleTestConnection = async () => {
  testingConnection.value = true
  try {
    const configStr = buildConfigJSON(policyForm.value.driver)
    const res = await api.post('/admin/policies/test', {
      driver: policyForm.value.driver,
      config: configStr,
      id: isEditing.value ? editTargetId.value : undefined
    })
    message.success(res.data?.message || t('adminPolicies.testSuccess'))
  } catch (err: any) {
    message.error(err.response?.data?.error || t('adminPolicies.testFailed'))
  } finally {
    testingConnection.value = false
  }
}

const openCreateModal = () => {
  isEditing.value = false
  editTargetId.value = null
  capacityValue.value = 0
  capacityUnit.value = 'GB'
  policyForm.value = {
    name: '',
    driver: 'local',
    path_prefix: '',
    naming_rule: '{Y}/{m}/{d}/{random:16}.{ext}',
    max_single_file_size_mb: 0,
    is_default: false,
    role_ids: roles.value.map(r => r.id)
  }
  driverConfig.value.base_path = 'data/storage'
  showModal.value = true
}

const openEditModal = (p: any) => {
  isEditing.value = true
  editTargetId.value = p.id
  if (!p.max_capacity_bytes || p.max_capacity_bytes <= 0) {
    capacityValue.value = 0
    capacityUnit.value = 'GB'
  } else {
    const b = p.max_capacity_bytes
    const tb = 1024 * 1024 * 1024 * 1024
    const gb = 1024 * 1024 * 1024
    const mb = 1024 * 1024
    const kb = 1024
    if (b % tb === 0) {
      capacityValue.value = b / tb
      capacityUnit.value = 'TB'
    } else if (b % gb === 0) {
      capacityValue.value = b / gb
      capacityUnit.value = 'GB'
    } else if (b % mb === 0) {
      capacityValue.value = b / mb
      capacityUnit.value = 'MB'
    } else if (b % kb === 0) {
      capacityValue.value = b / kb
      capacityUnit.value = 'KB'
    } else {
      capacityValue.value = Math.round((b / mb) * 100) / 100
      capacityUnit.value = 'MB'
    }
  }
  policyForm.value = {
    name: p.name,
    driver: p.driver || 'local',
    path_prefix: p.path_prefix !== undefined && p.path_prefix !== null ? p.path_prefix : '',
    naming_rule: p.naming_rule || '{Y}/{m}/{d}/{random:16}.{ext}',
    max_single_file_size_mb: p.max_single_file_size_mb || 0,
    is_default: p.is_default,
    role_ids: p.role_ids || []
  }
  parseConfigJSON(p.driver, p.config)
  showModal.value = true
}

const handleSavePolicy = async () => {
  if (!policyForm.value.name.trim()) {
    message.warning(t('adminPolicies.nameRequired'))
    return
  }

  saving.value = true
  try {
    const mult = unitMultiplier[capacityUnit.value] || (1024 * 1024 * 1024)
    const maxCapacityBytes = (capacityValue.value || 0) * mult
    const cleanPrefix = (policyForm.value.path_prefix || '').trim().replace(/^\/+|\/+$/g, '')
    const configStr = buildConfigJSON(policyForm.value.driver)

    const payload = {
      ...policyForm.value,
      path_prefix: cleanPrefix,
      max_capacity_bytes: maxCapacityBytes,
      max_single_file_size_mb: Number(policyForm.value.max_single_file_size_mb) || 0,
      config: configStr
    }

    let res: any
    if (isEditing.value && editTargetId.value) {
      res = await api.patch(`/admin/policies/${editTargetId.value}`, payload)
    } else {
      res = await api.post('/admin/policies', payload)
    }
    if (res.data?.warning) {
      message.warning(res.data.warning, { duration: 6000 })
    }
    message.success(t('adminPolicies.saveSuccess'))
    showModal.value = false
    loadData()
  } catch (err: any) {
    message.error(err.response?.data?.error || t('common.save'))
  } finally {
    saving.value = false
  }
}

const handleDeletePolicy = async (id: number) => {
  try {
    await api.delete(`/admin/policies/${id}`)
    message.success(t('adminPolicies.deleteSuccess'))
    loadData()
  } catch (err: any) {
    message.error(err.response?.data?.error || t('common.delete'))
  }
}

const formatBytes = (bytes: number) => {
  if (!bytes || bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

onMounted(loadData)
</script>

<style scoped>
.role-checkbox-container {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.role-checkbox-card {
  background: rgba(255, 255, 255, 0.03);
  border: 1px solid rgba(255, 255, 255, 0.08);
  padding: 6px 12px;
  border-radius: 6px;
  transition: all 0.2s ease;
  cursor: pointer;
}

.role-checkbox-card:hover {
  background: rgba(255, 255, 255, 0.06);
  border-color: rgba(99, 226, 183, 0.4);
}

.role-checkbox-text {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.role-checkbox-title {
  font-weight: 500;
  font-size: 13px;
}

.role-checkbox-sub {
  font-size: 12px;
  color: #888;
}
</style>
