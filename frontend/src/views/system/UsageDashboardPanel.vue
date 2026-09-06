<template>
  <div class="usage-dashboard-panel">
    <div class="section-header usage-header">
      <div>
        <h2>{{ t('usageStats.adminTitle') }}</h2>
        <p class="section-description">{{ t('usageStats.adminDesc') }}</p>
      </div>
      <t-button variant="outline" @click="pricesVisible = true">
        <template #icon><t-icon name="money-circle" /></template>
        {{ t('usageStats.prices.menuLabel') }}
      </t-button>
    </div>

    <t-tabs v-model="view">
      <t-tab-panel value="dashboard" :label="t('usageStats.tabs.dashboard')" />
      <t-tab-panel value="records" :label="t('usageStats.tabs.records')" />
    </t-tabs>

    <!-- 看板：平台总览 / 按空间 / 按用户 + 下钻过滤 -->
    <template v-if="view === 'dashboard'">
      <div class="usage-admin-filters">
        <div class="usage-filter-field">
          <span class="usage-filter-label">{{ t('usageStats.filters.level') }}</span>
          <t-radio-group variant="default-filled" :value="level" @change="handleLevelChange">
            <t-radio-button value="platform">{{ t('usageStats.level.platform') }}</t-radio-button>
            <t-radio-button value="tenant">{{ t('usageStats.level.tenant') }}</t-radio-button>
            <t-radio-button value="user">{{ t('usageStats.level.user') }}</t-radio-button>
          </t-radio-group>
        </div>
        <div class="usage-filter-field">
          <span class="usage-filter-label">{{ t('usageStats.filters.tenantId') }}</span>
          <t-input
            v-model="drillTenantId"
            class="usage-admin-input"
            :placeholder="t('usageStats.filters.tenantIdPlaceholder')"
            clearable
          />
        </div>
        <div class="usage-filter-field">
          <span class="usage-filter-label">{{ t('usageStats.filters.userId') }}</span>
          <t-input
            v-model="drillUserId"
            class="usage-admin-input usage-admin-input--user"
            :placeholder="t('usageStats.filters.userIdPlaceholder')"
            clearable
          />
        </div>
        <t-button theme="primary" variant="outline" @click="applyDrill">
          {{ t('usageStats.filters.apply') }}
        </t-button>
      </div>

      <UsageSummarySection
        :key="`${level}-${appliedTenantId}-${appliedUserId}`"
        :fetcher="dashboardFetcher"
        :group-by-options="groupByOptions"
        :show-tenant-column="level !== 'platform'"
        :show-user-column="level === 'user'"
      />
    </template>

    <!-- 明细：原始台账分页 -->
    <div v-else class="usage-records">
      <div class="usage-filter-bar">
        <div class="usage-filter-field">
          <span class="usage-filter-label">{{ t('usageStats.filters.category') }}</span>
          <t-select v-model="recordsCategory" class="usage-filter-control" clearable
            :placeholder="t('usageStats.filters.allCategories')">
            <t-option v-for="cat in USAGE_CATEGORIES" :key="cat" :value="cat">
              {{ t('usageStats.category.' + cat) }}
            </t-option>
          </t-select>
        </div>
        <div class="usage-filter-field usage-filter-field--wide">
          <span class="usage-filter-label">{{ t('usageStats.filters.range') }}</span>
          <t-date-range-picker
            v-model="recordsRange"
            class="usage-filter-control usage-filter-control--wide"
            :placeholder="[t('usageStats.filters.from'), t('usageStats.filters.to')]"
            clearable
            allow-input
          />
        </div>
        <t-button variant="outline" @click="reloadRecords">
          <template #icon><t-icon :name="recordsLoading ? 'loading' : 'refresh'" /></template>
          {{ t('usageStats.filters.refresh') }}
        </t-button>
      </div>

      <div v-if="recordsError" class="usage-branch">
        <t-alert theme="error" :message="recordsError" />
      </div>
      <div v-else-if="!recordsLoading && records.length === 0" class="usage-branch">
        <t-empty :description="t('usageStats.empty')" />
      </div>
      <div v-else class="data-table-shell usage-table-shell">
        <t-table
          row-key="occurred_at_idx"
          :data="recordRows"
          :columns="recordColumns"
          :loading="recordsLoading"
          size="medium"
          hover
        >
          <template #occurred_at="{ row }">{{ formatDateTime(row.occurred_at) }}</template>
          <template #category="{ row }">
            <t-tag size="small" variant="light">{{ categoryLabel(row.category) }}</t-tag>
          </template>
          <template #status="{ row }">
            <t-tag size="small" variant="light-outline" :theme="statusTheme(row.status)">
              {{ t('usageStats.status.' + row.status) }}
            </t-tag>
          </template>
        </t-table>
      </div>

      <div class="usage-records-pagination">
        <t-pagination
          v-model="recordsPage"
          :total="recordsTotal"
          :page-size="recordsPageSize"
          :show-jumper="true"
          @current-change="reloadRecords"
        />
      </div>
    </div>

    <!-- 模型单价：从「模型」菜单收敛进看板内，以弹窗弹出配置（不再单独占菜单）。 -->
    <t-dialog
      v-model:visible="pricesVisible"
      :header="t('usageStats.prices.title')"
      :footer="false"
      width="960px"
      destroy-on-close
      @close="pricesVisible = false"
    >
      <BillingPricesPanel embedded />
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
// 管理后台用量看板（docs/Token统计与计费设计.md §5.3）：平台总览 /
// 按空间 / 按用户三级聚合 + 空间/用户下钻，外加原始台账明细页。
// 挂在 SystemConsole 的「用量看板」菜单（g.SystemAdmin 路由）。
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import UsageSummarySection from '@/views/settings/usage/UsageSummarySection.vue'
import BillingPricesPanel from './BillingPricesPanel.vue'
import {
  getAdminUsageRecords,
  getAdminUsageSummary,
  type UsageGroupBy,
  type UsageQueryParams,
  type UsageRecord,
  type UsageSummaryResponse,
} from '@/api/usage'

type AdminLevel = 'platform' | 'tenant' | 'user'

const USAGE_CATEGORIES = ['chat', 'embedding', 'rerank', 'vlm', 'asr'] as const

const { t } = useI18n()
const view = ref<'dashboard' | 'records'>('dashboard')
const level = ref<AdminLevel>('platform')
// 模型单价配置弹窗（已从控制台独立菜单收敛进看板内）。
const pricesVisible = ref(false)

// 下钻过滤：输入与已应用分离，点「应用」才生效（避免每敲一个字符重拉）。
const drillTenantId = ref('')
const drillUserId = ref('')
const appliedTenantId = ref('')
const appliedUserId = ref('')

// 平台层默认维度与空间层一致（类型/模型/天）；用户层给用户维度。
const PLATFORM_GROUP_BYS: UsageGroupBy[] = ['category', 'model', 'day']
const USER_GROUP_BYS: UsageGroupBy[] = ['category', 'model', 'day', 'user']
const groupByOptions = computed(() => (level.value === 'user' ? USER_GROUP_BYS : PLATFORM_GROUP_BYS))

const dashboardFetcher = computed(
  () =>
    (params: UsageQueryParams): Promise<UsageSummaryResponse> =>
      getAdminUsageSummary({
        ...params,
        level: level.value,
        tenant_id: appliedTenantId.value ? Number(appliedTenantId.value) : undefined,
        user_id: appliedUserId.value || undefined,
      }),
)

function handleLevelChange(next: unknown) {
  level.value = next as AdminLevel
}

function applyDrill() {
  appliedTenantId.value = drillTenantId.value.trim()
  appliedUserId.value = drillUserId.value.trim()
}

// ── 明细页 ────────────────────────────────────────────────────────────────
const recordsCategory = ref('')
const recordsRange = ref<string[]>([])
const records = ref<UsageRecord[]>([])
const recordsTotal = ref(0)
const recordsPage = ref(1)
const recordsPageSize = 50
const recordsLoading = ref(false)
const recordsError = ref('')

const recordRows = computed(() => records.value.map((r, i) => ({ ...r, occurred_at_idx: i })))

const recordColumns = computed(() => [
  { colKey: 'occurred_at', title: t('usageStats.table.occurredAt'), width: 170 },
  { colKey: 'tenant_id', title: t('usageStats.table.tenant'), width: 80, align: 'right' },
  { colKey: 'user_id', title: t('usageStats.table.user'), width: 130, ellipsis: true },
  { colKey: 'model_name', title: t('usageStats.table.model'), ellipsis: true },
  { colKey: 'category', title: t('usageStats.table.category'), width: 100 },
  { colKey: 'purpose', title: t('usageStats.table.purpose'), width: 130, ellipsis: true },
  { colKey: 'status', title: t('usageStats.table.status'), width: 90 },
  { colKey: 'input_tokens', title: t('usageStats.table.inputTokens'), align: 'right' },
  { colKey: 'output_tokens', title: t('usageStats.table.outputTokens'), align: 'right' },
])

const categoryLabel = (cat: string) =>
  USAGE_CATEGORIES.includes(cat as (typeof USAGE_CATEGORIES)[number])
    ? t('usageStats.category.' + cat)
    : cat

const statusTheme = (status: string): string =>
  ({ success: 'success', failed: 'danger', interrupted: 'warning' })[status] ?? 'default'

const formatDateTime = (iso: string) => {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

async function reloadRecords() {
  recordsLoading.value = true
  recordsError.value = ''
  try {
    const params: UsageQueryParams = {
      limit: recordsPageSize,
      offset: (recordsPage.value - 1) * recordsPageSize,
    }
    if (recordsCategory.value) params.category = recordsCategory.value
    if (recordsRange.value && recordsRange.value.length === 2) {
      if (recordsRange.value[0]) params.from = recordsRange.value[0]
      if (recordsRange.value[1]) params.to = recordsRange.value[1]
    }
    const resp = await getAdminUsageRecords(params)
    records.value = resp.rows ?? []
    recordsTotal.value = resp.total ?? 0
  } catch (e: unknown) {
    recordsError.value = e instanceof Error ? e.message : String(e)
    records.value = []
  } finally {
    recordsLoading.value = false
  }
}

watch(view, (v) => {
  if (v === 'records' && records.value.length === 0 && !recordsError.value) void reloadRecords()
})
</script>

<style scoped>
.usage-dashboard-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.usage-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.usage-admin-filters {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
}

.usage-filter-field {
  display: flex;
  align-items: center;
  gap: 8px;
}

.usage-filter-label {
  font-size: 13px;
  color: var(--td-text-color-secondary);
  white-space: nowrap;
}

.usage-admin-input {
  width: 130px;
}

.usage-admin-input--user {
  width: 200px;
}

.usage-filter-control {
  width: 150px;
}

.usage-filter-control--wide {
  width: 260px;
}

.usage-table-shell {
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--td-radius-medium);
  overflow: hidden;
}

.usage-branch {
  padding: 24px 0;
}

.usage-records-pagination {
  display: flex;
  justify-content: flex-end;
}
</style>
