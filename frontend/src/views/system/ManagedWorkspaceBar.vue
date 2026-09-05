<template>
  <div class="managed-workspace-bar">
    <span class="managed-workspace-label">
      <t-icon name="user-circle" size="16px" />
      {{ $t('systemConsole.workspace.label') }}
    </span>
    <t-select
      :value="managedStore.managedTenantId"
      :options="tenantOptions"
      :placeholder="$t('systemConsole.workspace.placeholder')"
      :loading="loading"
      clearable
      filterable
      class="managed-workspace-select"
      @change="handleChange"
      @clear="handleChange(null)"
    />
    <span v-if="managedStore.managedTenantName" class="managed-workspace-hint">
      {{ $t('systemConsole.workspace.managingHint', { name: managedStore.managedTenantName }) }}
    </span>
  </div>
</template>

<script setup lang="ts">
// 「按空间代管」目标空间选择条：控制台七个代管面板（向量库/解析引擎/
// 存储/沙箱/技能/网络搜索/MCP）共用的顶部工具栏。选中后由
// resolveRequestTenantId() 让本页所有请求（含 SSE）的 X-Tenant-ID 指向
// 该空间；选择随 ?tenant= query 持久化，离开控制台由路由守卫清除。
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { useManagedWorkspaceStore } from '@/stores/managedWorkspace'
import { listPlatformTenants, type PlatformTenant } from '@/api/system'

const route = useRoute()
const router = useRouter()
const managedStore = useManagedWorkspaceStore()

const loading = ref(false)
const tenants = ref<PlatformTenant[]>([])

const tenantOptions = computed(() =>
  tenants.value.map((item) => ({
    value: String(item.id),
    label: item.name,
  })),
)

function findTenant(id: string): PlatformTenant | undefined {
  return tenants.value.find((item) => String(item.id) === id)
}

// 选择变化 → 写 store + 同步 ?tenant=（清空选择时把 tenant 从 query
// 里去掉；skills 面板迁走后控制台不再有 sandbox 预选参数）。
function handleChange(value: unknown) {
  const id = typeof value === 'string' && value ? value : null
  if (id) {
    managedStore.setManaged(id, findTenant(id)?.name)
  } else {
    managedStore.clearManaged()
  }
  const query: Record<string, string> = { section: String(route.query.section ?? 'users') }
  if (id) query.tenant = id
  void router.replace({ path: '/system/console', query })
}

onMounted(async () => {
  loading.value = true
  try {
    const { tenants: rows } = await listPlatformTenants()
    tenants.value = rows ?? []
    // 初始化顺序：已代管（面板间切换重挂载）→ ?tenant= 深链 → 都没有则留空。
    const queryTenant = typeof route.query.tenant === 'string' ? route.query.tenant : ''
    if (managedStore.managedTenantId) {
      if (queryTenant !== managedStore.managedTenantId) {
        void router.replace({
          path: '/system/console',
          query: { ...route.query, tenant: managedStore.managedTenantId } as Record<string, string>,
        })
      }
    } else if (queryTenant) {
      const match = findTenant(queryTenant)
      if (match) {
        managedStore.setManaged(queryTenant, match.name)
      } else {
        // query 指向不存在的空间：清掉，避免静默代管错误目标。
        const { tenant: _drop, ...rest } = route.query
        void router.replace({ path: '/system/console', query: rest as Record<string, string> })
      }
    }
  } catch (err) {
    console.error('加载平台空间列表失败:', err)
    MessagePlugin.error(String((err as Error)?.message || '加载平台空间列表失败'))
  } finally {
    loading.value = false
  }
})
</script>

<style lang="less" scoped>
.managed-workspace-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 14px 0 4px;
}

.managed-workspace-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--td-text-color-secondary);
  flex-shrink: 0;
}

.managed-workspace-select {
  width: 260px;
}

.managed-workspace-hint {
  font-size: 13px;
  color: var(--td-brand-color);
}
</style>
