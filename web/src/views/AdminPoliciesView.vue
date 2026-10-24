<template>
  <div>
    <div style="font-size: 18px; font-weight: bold; margin-bottom: 20px;">{{ $t('adminPolicies.title') }}</div>
    <n-card style="border-radius: 12px;">
      <n-table :bordered="false" :single-line="false">
        <thead>
          <tr>
            <th>ID</th>
            <th>{{ $t('adminPolicies.policyName') }}</th>
            <th>{{ $t('adminPolicies.userId') }}</th>
            <th>{{ $t('adminPolicies.rootMapping') }}</th>
            <th>{{ $t('adminPolicies.pathPrefix') }}</th>
            <th>{{ $t('adminPolicies.namingRule') }}</th>
            <th>{{ $t('adminPolicies.isDefault') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="p in policies" :key="p.id">
            <td>{{ p.id }}</td>
            <td><strong>{{ p.name }}</strong></td>
            <td>{{ p.user_id }}</td>
            <td>
              <n-tag :type="p.use_root ? 'warning' : 'default'" size="small">
                {{ p.use_root ? $t('adminPolicies.rootDirect') : $t('common.no') }}
              </n-tag>
            </td>
            <td><code>/{{ p.path_prefix || '' }}</code></td>
            <td><code>{{ p.naming_rule }}</code></td>
            <td>{{ p.is_default ? $t('common.yes') : $t('common.no') }}</td>
          </tr>
        </tbody>
      </n-table>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api } from '../api/client'

const policies = ref<any[]>([])

onMounted(async () => {
  try {
    const res = await api.get('/admin/policies')
    policies.value = res.data
  } catch {}
})
</script>
