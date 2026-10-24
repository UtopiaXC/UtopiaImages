<template>
  <div>
    <div style="font-size: 18px; font-weight: bold; margin-bottom: 20px;">存储策略与路由挂载</div>
    <n-card style="border-radius: 12px;">
      <n-table :bordered="false" :single-line="false">
        <thead>
          <tr>
            <th>ID</th>
            <th>策略名称</th>
            <th>归属用户ID</th>
            <th>根路径映射 (UseRoot)</th>
            <th>URL 路径前缀</th>
            <th>文件命名规则</th>
            <th>默认策略</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="p in policies" :key="p.id">
            <td>{{ p.id }}</td>
            <td><strong>{{ p.name }}</strong></td>
            <td>{{ p.user_id }}</td>
            <td>
              <n-tag :type="p.use_root ? 'warning' : 'default'" size="small">
                {{ p.use_root ? '是 (根路径 direct)' : '否 (/{username}/)' }}
              </n-tag>
            </td>
            <td><code>/{{ p.path_prefix || '' }}</code></td>
            <td><code>{{ p.naming_rule }}</code></td>
            <td>{{ p.is_default ? '✅ 是' : '-' }}</td>
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
