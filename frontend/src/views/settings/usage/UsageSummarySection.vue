<template>
  <div class="usage-summary-section">
    <!-- 筛选栏：维度 / 类型 / 时间范围 -->
    <div class="usage-filter-bar">
      <div class="usage-filter-field">
        <span class="usage-filter-label">{{ t('usageStats.filters.dimension') }}</span>
        <t-select v-model="groupBy" class="usage-filter-control" @change="reload">
          <t-option v-for="opt in groupByOptions" :key="opt" :value="opt">
            {{ groupByLabel(opt) }}
          </t-option>
        </t-select>
      </div>
      <div class="usage-filter-field">
        <span class="usage-filter-label">{{ t('usageStats.filters.category') }}</span>
        <t-select v-model="categoryFilter" class="usage-filter-control" clearable
          :placeholder="t('usageStats.filters.allCategories')" @change="reload">
          <t-option v-for="cat in USAGE_CATEGORIES" :key="cat" :value="cat">
            {{ t('usageStats.category.' + cat) }}
          </t-option>
        </t-select>
      </div>
      <div class="usage-filter-field usage-filter-field--wide">
        <span class="usage-filter-label">{{ t('usageStats.filters.range') }}</span>
        <t-date-range-picker
          v-model="dateRange"
          class="usage-filter-control"
          :placeholder="[t('usageStats.filters.from'), t('usageStats.filters.to')]"
          :disable-date="disableFutureDate"
          clearable
          allow-input
          @change="reload"
        />
      </div>
      <t-button variant="outline" :disabled="loading" class="usage-refresh-btn" @click="reload">
        <template #icon><t-icon :name="loading ? 'loading' : 'refresh'" /></template>
        {{ t('usageStats.filters.refresh') }}
      </t-button>
    </div>

    <!-- 汇总卡片 -->
    <div class="usage-total-cards">
      <div class="usage-total-card">
        <span class="usage-total-label">{{ t('usageStats.totals.calls') }}</span>
        <span class="usage-total-value">{{ formatCount(totals.calls) }}</span>
      </div>
      <div class="usage-total-card">
        <span class="usage-total-label">{{ t('usageStats.totals.input') }}</span>
        <span class="usage-total-value">{{ formatCount(totals.input) }}</span>
      </div>
      <div class="usage-total-card">
        <span class="usage-total-label">{{ t('usageStats.totals.output') }}</span>
        <span class="usage-total-value">{{ formatCount(totals.output) }}</span>
      </div>
      <div class="usage-total-card">
        <span class="usage-total-label">{{ t('usageStats.totals.amount') }}</span>
        <span class="usage-total-value" :class="{ 'is-zero': totals.amount <= 0 }">
          {{ formatAmount(totals.amount) }}
        </span>
      </div>
    </div>

    <!-- 明细表 -->
    <div v-if="error" class="usage-branch usage-branch--error">
      <t-alert theme="error" :message="error">
        <template #operation>
          <t-button size="small" @click="reload">{{ t('usageStats.filters.refresh') }}</t-button>
        </template>
      </t-alert>
    </div>
    <div v-else-if="!loading && rows.length === 0" class="usage-branch">
      <t-empty :description="t('usageStats.empty')" />
    </div>
    <div v-else class="data-table-shell usage-table-shell">
      <t-table
        row-key="__key"
        :data="tableRows"
        :columns="columns"
        :loading="loading"
        size="medium"
        hover
      >
        <template #day="{ row }">
          <span class="usage-cell-day">{{ row.day }}</span>
        </template>
        <template #tenant="{ row }">
          <span class="usage-cell-tenant">{{ tenantLabel(row.tenant_id) }}</span>
        </template>
        <template #model="{ row }">
          <span class="usage-cell-model">{{ row.model_name || row.model_id || '—' }}</span>
        </template>
        <template #category="{ row }">
          <t-tag size="small" variant="light" :theme="categoryTheme(row.category)">
            {{ categoryLabel(row.category) }}
          </t-tag>
        </template>
        <template #calls="{ row }">
          <span>{{ formatCount(row.calls) }}<span v-if="row.approximate" class="usage-approx-mark">*</span></span>
        </template>
        <template #amount="{ row }">
          <span v-if="row.has_price_config">{{ formatAmount(row.amount) }}</span>
          <t-tooltip v-else :content="t('usageStats.table.unpricedHint')">
            <span class="usage-unpriced">—</span>
          </t-tooltip>
        </template>
        <template #audio_seconds="{ row }">{{ formatAudioMinutes(row.audio_seconds) }}</template>
      </t-table>
    </div>

    <p v-if="hasApproxRows" class="usage-approx-note">{{ t('usageStats.approxNote') }}</p>
  </div>
</template>

<script setup lang="ts">
// 用量汇总的共用展示组件（docs/Token统计与计费设计.md §5 三级入口共用）：
// 筛选（维度/类型/时间范围）+ 汇总卡片 + 明细表。数据获取通过 fetcher
// 注入——个人面板、空间面板、管理后台看板各自闭包不同的 API 函数，
// 组件本身不感知权限层级。
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  getMyUsageSummary,
  type UsageGroupBy,
  type UsageQueryParams,
  type UsageSummaryResponse,
  type UsageSummaryRow,
} from '@/api/usage'

const props = defineProps<{
  // 数据获取函数：参数已带上组件内选择的维度/类型/时间过滤。
  fetcher?: (params: UsageQueryParams) => Promise<UsageSummaryResponse>
  // 可选的聚合维度；默认按类型汇总。
  groupByOptions?: UsageGroupBy[]
  // 行合并键（多行同维度时 t-table 需要稳定 row-key）。
  showTenantColumn?: boolean
  showUserColumn?: boolean
}>()

const DEFAULT_FETCHER = (params: UsageQueryParams) => getMyUsageSummary(params)
const fetcher = computed(() => props.fetcher ?? DEFAULT_FETCHER)

const USAGE_CATEGORIES = ['chat', 'embedding', 'rerank', 'vlm', 'asr'] as const

const { t } = useI18n()
const groupBy = ref<UsageGroupBy>(props.groupByOptions?.[0] ?? 'category')
const categoryFilter = ref<string>('')
const dateRange = ref<string[]>([])
const rows = ref<UsageSummaryRow[]>([])
const loading = ref(false)
const error = ref('')

const groupByLabel = (key: UsageGroupBy | string) =>
  key === '' || key === 'category'
    ? t('usageStats.groupBy.category')
    : t(`usageStats.groupBy.${key === 'tenant_user' ? 'tenantUser' : key}`)

const categoryLabel = (cat: string) =>
  USAGE_CATEGORIES.includes(cat as (typeof USAGE_CATEGORIES)[number])
    ? t('usageStats.category.' + cat)
    : cat

const categoryTheme = (cat: string): string =>
  ({ chat: 'primary', embedding: 'success', rerank: 'warning', vlm: 'default', asr: 'default' })[cat] ?? 'default'

const tenantLabel = (tenantId?: number) =>
  tenantId == null ? '—' : `#${tenantId}`

const disableFutureDate = (d: Date) => d.getTime() > Date.now()

const totals = computed(() =>
  rows.value.reduce(
    (acc, r) => ({
      calls: acc.calls + r.calls,
      input: acc.input + r.input_tokens,
      output: acc.output + r.output_tokens,
      amount: acc.amount + r.amount,
    }),
    { calls: 0, input: 0, output: 0, amount: 0 },
  ),
)

const hasApproxRows = computed(() => rows.value.some((r) => r.approximate))
const hasImages = computed(() => rows.value.some((r) => (r.images ?? 0) > 0))
const hasAudio = computed(() => rows.value.some((r) => (r.audio_seconds ?? 0) > 0))

// 行合并：同一维度下 category 列始终存在（后端 summaryDims 规则），
// 直接给每行编个稳定序号作 row-key 即可。
const tableRows = computed(() => rows.value.map((r, i) => ({ ...r, __key: i })))

const columns = computed(() => {
  type Col = { colKey: string; title: string; align?: string; width?: string | number; ellipsis?: boolean }
  const cols: Col[] = []
  if (groupBy.value === 'day' || rows.value.some((r) => r.day)) {
    cols.push({ colKey: 'day', title: t('usageStats.table.day'), width: 110 })
  }
  if (props.showTenantColumn && rows.value.some((r) => r.tenant_id != null)) {
    cols.push({ colKey: 'tenant', title: t('usageStats.table.tenant'), width: 90 })
  }
  if (props.showUserColumn && rows.value.some((r) => r.user_id)) {
    cols.push({ colKey: 'user_id', title: t('usageStats.table.user'), width: 140, ellipsis: true })
  }
  if (rows.value.some((r) => r.model_name || r.model_id)) {
    cols.push({ colKey: 'model', title: t('usageStats.table.model'), ellipsis: true })
  }
  cols.push({ colKey: 'category', title: t('usageStats.table.category'), width: 110 })
  cols.push({ colKey: 'calls', title: t('usageStats.table.calls'), align: 'right', width: 100 })
  cols.push({ colKey: 'input_tokens', title: t('usageStats.table.inputTokens'), align: 'right' })
  cols.push({ colKey: 'output_tokens', title: t('usageStats.table.outputTokens'), align: 'right' })
  if (rows.value.some((r) => (r.cached_tokens ?? 0) > 0)) {
    cols.push({ colKey: 'cached_tokens', title: t('usageStats.table.cachedTokens'), align: 'right' })
  }
  if (hasImages.value) cols.push({ colKey: 'images', title: t('usageStats.table.images'), align: 'right', width: 90 })
  if (hasAudio.value) {
    cols.push({ colKey: 'audio_seconds', title: t('usageStats.table.audioMinutes'), align: 'right' })
  }
  cols.push({ colKey: 'amount', title: t('usageStats.table.amount'), align: 'right', width: 130 })
  return cols
})

const formatCount = (n: number) => (n ?? 0).toLocaleString()
const formatAmount = (n: number) => {
  const v = n ?? 0
  if (v === 0) return '¥0'
  return '¥' + v.toFixed(4).replace(/\.?0+$/, '')
}
const formatAudioMinutes = (seconds: number) => ((seconds ?? 0) / 60).toFixed(1)

async function reload() {
  loading.value = true
  error.value = ''
  try {
    const params: UsageQueryParams = { group_by: groupBy.value }
    if (categoryFilter.value) params.category = categoryFilter.value
    if (dateRange.value && dateRange.value.length === 2) {
      if (dateRange.value[0]) params.from = dateRange.value[0]
      if (dateRange.value[1]) params.to = dateRange.value[1]
    }
    const resp = await fetcher.value(params)
    rows.value = resp.rows ?? []
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : String(e)
    rows.value = []
  } finally {
    loading.value = false
  }
}

// 面板外层切换 fetcher（例如空间面板切“我的/全空间”页签）时重新拉取。
watch(fetcher, () => reload())

// 维度选项随层级变化（全空间比“我的”多“按用户”）时收敛非法选择。
watch(
  () => props.groupByOptions,
  (opts) => {
    if (opts && !opts.includes(groupBy.value)) {
      groupBy.value = opts[0] ?? 'category'
      void reload()
    }
  },
)

onMounted(reload)
</script>

<style scoped>
.usage-summary-section {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.usage-filter-bar {
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

.usage-filter-field--wide .usage-filter-control {
  width: 260px;
}

.usage-filter-label {
  font-size: 13px;
  color: var(--td-text-color-secondary);
  white-space: nowrap;
}

.usage-filter-control {
  width: 150px;
}

.usage-total-cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 12px;
}

.usage-total-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 14px 16px;
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--td-radius-medium);
  background: var(--td-bg-color-container);
}

.usage-total-label {
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.usage-total-value {
  font-size: 20px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

.usage-total-value.is-zero {
  color: var(--td-text-color-placeholder);
}

.usage-table-shell {
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--td-radius-medium);
  overflow: hidden;
}

.usage-branch {
  padding: 24px 0;
}

.usage-approx-mark {
  margin-left: 2px;
  color: var(--td-warning-color);
}

.usage-approx-note {
  margin: 0;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.usage-unpriced {
  color: var(--td-text-color-placeholder);
  cursor: help;
}

.usage-cell-day,
.usage-cell-tenant {
  font-variant-numeric: tabular-nums;
}
</style>
