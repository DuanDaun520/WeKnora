<template>
  <div class="tenant-usage-panel">
    <div class="section-header">
      <h2>{{ t('usageStats.tenantTitle') }}</h2>
      <p class="section-description">{{ t('usageStats.tenantDesc') }}</p>
    </div>

    <t-tabs v-model="scope">
      <t-tab-panel value="me" :label="t('usageStats.scopeMe')" />
      <t-tab-panel value="tenant" :label="t('usageStats.scopeTenant')" />
    </t-tabs>

    <!-- key 强制切页签时重建：fetcher 变化本身会触发重拉，
         key 让滚动位置/表格状态同步复位，行为与独立面板一致。 -->
    <UsageSummarySection
      :key="scope"
      :fetcher="scopeFetcher"
      :group-by-options="groupByOptions"
      :show-user-column="scope === 'tenant'"
    />
  </div>
</template>

<script setup lang="ts">
// 空间管理员面板（docs/Token统计与计费设计.md §5.2）：本人用量 + 全空间
// 用量两个页签。空间 ID 取认证上下文的当前空间；scope=me 在后端把查询
// 收敛到管理员本人，scope=tenant 汇总全空间（含各成员与后台解析任务）。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import UsageSummarySection from './usage/UsageSummarySection.vue'
import { useAuthStore } from '@/stores/auth'
import {
  getTenantUsageSummary,
  type UsageGroupBy,
  type UsageQueryParams,
  type UsageSummaryResponse,
} from '@/api/usage'

type UsageScope = 'me' | 'tenant'

const { t } = useI18n()
const authStore = useAuthStore()
const scope = ref<UsageScope>('me')

const tenantId = computed(() => Number(authStore.currentTenantId ?? 0))

// 全空间视角多一个“按用户”维度（后端附带模型细分）；
// “我的用量”维度集合与个人设置面板一致。
const ME_GROUP_BYS: UsageGroupBy[] = ['category', 'model', 'day']
const TENANT_GROUP_BYS: UsageGroupBy[] = ['category', 'model', 'day', 'user']
const groupByOptions = computed(() => (scope.value === 'tenant' ? TENANT_GROUP_BYS : ME_GROUP_BYS))

// computed 保证 scope 切换时 fetcher 引用变化 → 子组件自动重拉。
const scopeFetcher = computed(
  () =>
    (params: UsageQueryParams): Promise<UsageSummaryResponse> =>
      getTenantUsageSummary(tenantId.value, scope.value, params),
)
</script>

<style scoped>
.tenant-usage-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
</style>
