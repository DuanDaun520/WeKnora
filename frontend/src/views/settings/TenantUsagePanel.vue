<template>
  <div class="tenant-usage-panel">
    <div class="section-header">
      <h2>{{ t('usageStats.tenantTitle') }}</h2>
      <p class="section-description">{{ t('usageStats.tenantDesc') }}</p>
    </div>

    <UsageSummarySection
      :fetcher="tenantFetcher"
      :group-by-options="TENANT_GROUP_BYS"
      :show-user-column="true"
      :show-category-filter="false"
      :merge-rows="true"
      :day-chart="true"
      :default-range-days="7"
      :user-names="memberNames"
    />
  </div>
</template>

<script setup lang="ts">
// 空间管理员面板（docs/Token统计与计费设计.md §5.2）：全空间视角的
// AI 用量统计。本人用量走个人设置面板（usage-stats），这里不再重复。
// 空间 ID 取认证上下文的当前空间；查询在后端按 tenant_id 收敛——同一
// 用户在其它空间的用量不会计入本面板。按用户维度展示用户名：成员
// 名册解析不到的 user_id（如已移出的成员）不计入统计。
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import UsageSummarySection from './usage/UsageSummarySection.vue'
import { useAuthStore } from '@/stores/auth'
import { fetchAllTenantMembers } from '@/api/tenant/members'
import {
  getTenantUsageSummary,
  type UsageGroupBy,
  type UsageQueryParams,
  type UsageSummaryResponse,
} from '@/api/usage'

const { t } = useI18n()
const authStore = useAuthStore()

const tenantId = computed(() => Number(authStore.currentTenantId ?? 0))

// 维度：按类型（默认）/ 按天（柱状图）/ 按用户（显示用户名）。
const TENANT_GROUP_BYS: UsageGroupBy[] = ['category', 'day', 'user']

// computed 让空间切换（tenantId 变化）产生新的 fetcher 引用 → 子组件
// watch(fetcher) 自动按新空间重拉。
const tenantFetcher = computed(
  () =>
    (params: UsageQueryParams): Promise<UsageSummaryResponse> =>
      getTenantUsageSummary(tenantId.value, 'tenant', params),
)

// user_id → 用户名：成员名册（Viewer+ 的只读接口，admin 天然可读）。
// 拉取失败时保持空映射——按用户维度将显示为空（无法核名者不计入），
// 类型/天维度不受影响。
const memberNames = ref<Record<string, string>>({})

async function loadMemberNames() {
  if (!tenantId.value) {
    memberNames.value = {}
    return
  }
  try {
    const members = await fetchAllTenantMembers(tenantId.value)
    const map: Record<string, string> = {}
    for (const m of members) {
      if (m.user_id && m.username) map[m.user_id] = m.username
    }
    memberNames.value = map
  } catch {
    memberNames.value = {}
  }
}

onMounted(loadMemberNames)
watch(tenantId, () => void loadMemberNames())
</script>

<style scoped>
.tenant-usage-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
</style>
