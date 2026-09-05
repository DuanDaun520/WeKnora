<template>
  <div class="usage-summary-section">
    <!-- 筛选栏：维度 / 类型 / 时间范围 -->
    <div class="usage-filter-bar">
      <div class="usage-filter-field">
        <span class="usage-filter-label">{{ t('usageStats.filters.dimension') }}</span>
        <t-select v-model="groupBy" class="usage-filter-control" @change="reload">
          <!-- label 必须显式传：只给默认插槽时折叠态会回退显示 value（英文键）。 -->
          <t-option v-for="opt in groupByOptions" :key="opt" :value="opt" :label="groupByLabel(opt)" />
        </t-select>
      </div>
      <div v-if="showCategoryFilter" class="usage-filter-field">
        <span class="usage-filter-label">{{ t('usageStats.filters.category') }}</span>
        <t-select v-model="categoryFilter" class="usage-filter-control" clearable
          :placeholder="t('usageStats.filters.allCategories')" @change="reload">
          <t-option v-for="cat in USAGE_CATEGORIES" :key="cat" :value="cat"
            :label="t('usageStats.category.' + cat)" />
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
    <div v-else-if="!loading && displayRows.length === 0" class="usage-branch">
      <t-empty :description="t('usageStats.empty')" />
    </div>

    <!-- 按天维度（个人面板形态）：柱状图，柱高 = 输入+输出+缓存命中，
         悬停展示当日各值。明细表格仍走下面的 v-else 分支。 -->
    <div v-else-if="showDayChart" class="usage-chart-card">
      <div class="usage-chart-legend">
        <span class="usage-chart-legend-item"><i class="usage-seg usage-seg--input"></i>{{ t('usageStats.chart.input') }}</span>
        <span class="usage-chart-legend-item"><i class="usage-seg usage-seg--output"></i>{{ t('usageStats.chart.output') }}</span>
        <span class="usage-chart-legend-item"><i class="usage-seg usage-seg--cached"></i>{{ t('usageStats.chart.cached') }}</span>
      </div>
      <div class="usage-chart">
        <div v-for="d in chartData" :key="d.row.day ?? d.row.category" class="usage-chart-col">
          <t-tooltip placement="top">
            <div class="usage-chart-track">
              <div class="usage-chart-bar" :style="{ height: (d.max > 0 ? (d.total / d.max) * 100 : 0) + '%' }">
                <i class="usage-seg usage-seg--input"
                  :style="{ height: (d.total > 0 ? (d.row.input_tokens / d.total) * 100 : 0) + '%' }"></i>
                <i class="usage-seg usage-seg--output"
                  :style="{ height: (d.total > 0 ? (d.row.output_tokens / d.total) * 100 : 0) + '%' }"></i>
                <i class="usage-seg usage-seg--cached"
                  :style="{ height: (d.total > 0 ? (d.row.cached_tokens / d.total) * 100 : 0) + '%' }"></i>
              </div>
            </div>
            <template #content>
              <div class="usage-chart-pop">
                <div class="usage-chart-pop-day">{{ d.row.day }}</div>
                <div>{{ t('usageStats.table.calls') }}：{{ formatCount(d.row.calls) }}</div>
                <div>{{ t('usageStats.chart.input') }}：{{ formatCount(d.row.input_tokens) }}</div>
                <div>{{ t('usageStats.chart.output') }}：{{ formatCount(d.row.output_tokens) }}</div>
                <div>{{ t('usageStats.chart.cached') }}：{{ formatCount(d.row.cached_tokens) }}</div>
                <div>{{ t('usageStats.chart.totalTokens') }}：{{ formatCount(d.total) }}</div>
                <div>{{ t('usageStats.table.amount') }}：{{ d.row.has_price_config ? formatAmount(d.row.amount) : '—' }}</div>
              </div>
            </template>
          </t-tooltip>
          <span class="usage-chart-xlabel">{{ d.row.day ? d.row.day.slice(5) : '' }}</span>
        </div>
      </div>
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
        <template #user_id="{ row }">
          <span class="usage-cell-user">{{ userDisplayName(row.user_id) }}</span>
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
  // —— 个人面板（§5.1）的交互定制；其余两级面板不传即保持表格形态 ——
  // 隐藏「类型」筛选下拉（类型不参与过滤）。
  showCategoryFilter?: boolean
  // 后端为计价始终按模型细分返回，前端按展示维度（类型/日期）把模型
  // 细分合并回去：各行金额已折叠完，直接求和不丢计价。
  mergeRows?: boolean
  // 按天维度用柱状图替代明细表（柱高 = 输入+输出+缓存命中）。
  dayChart?: boolean
  // 初始时间范围：今天向前推 N 天（含今天）；0 表示不设初始范围。
  defaultRangeDays?: number
  // user_id → 用户名映射（按用户维度用）。提供时：用户列显示用户名，
  // 且解析不到用户名的行不计入统计（如已移出空间的成员）。
  userNames?: Record<string, string>
}>()

const DEFAULT_FETCHER = (params: UsageQueryParams) => getMyUsageSummary(params)
const fetcher = computed(() => props.fetcher ?? DEFAULT_FETCHER)

const USAGE_CATEGORIES = ['chat', 'embedding', 'rerank', 'vlm', 'asr'] as const

const { t } = useI18n()
const groupBy = ref<UsageGroupBy>(props.groupByOptions?.[0] ?? 'category')
const categoryFilter = ref<string>('')

const showCategoryFilter = computed(() => props.showCategoryFilter !== false)

const fmtDay = (d: Date) =>
  `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`

// 时间范围的绑定格式容错：TDesign 的 valueType 为空时自动推断，
// 可能回填 'YYYY-MM-DD' 字符串或 Date 对象，统一归一成日期串。
const asDayString = (v: unknown): string | null => {
  if (v instanceof Date) return fmtDay(v)
  if (typeof v === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(v)) return v
  return null
}

const initialRange = (): string[] => {
  const n = props.defaultRangeDays ?? 0
  if (n <= 0) return []
  const today = new Date()
  const from = new Date(today)
  from.setDate(from.getDate() - n)
  return [fmtDay(from), fmtDay(today)]
}
const dateRange = ref<string[]>(initialRange())
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
  displayRows.value.reduce(
    (acc, r) => ({
      calls: acc.calls + r.calls,
      input: acc.input + r.input_tokens,
      output: acc.output + r.output_tokens,
      amount: acc.amount + r.amount,
    }),
    { calls: 0, input: 0, output: 0, amount: 0 },
  ),
)

const hasApproxRows = computed(() => displayRows.value.some((r) => r.approximate))
const hasImages = computed(() => displayRows.value.some((r) => (r.images ?? 0) > 0))
const hasAudio = computed(() => displayRows.value.some((r) => (r.audio_seconds ?? 0) > 0))

// 展示行：mergeRows 时按当前维度（类型/日期/用户）合并后端的模型细分，
// 数值列求和、估算标记与配价标记取「任一行为真」。按用户维度且提供
// userNames 时，解析不到用户名的行（如已移出空间的成员）不计入。
const displayRows = computed<UsageSummaryRow[]>(() => {
  if (!props.mergeRows) return rows.value
  const keyOf = (r: UsageSummaryRow): string => {
    switch (groupBy.value) {
      case 'day':
        return r.day ?? ''
      case 'user':
        return r.user_id ?? ''
      case 'tenant':
        return String(r.tenant_id ?? '')
      case 'tenant_user':
        return `${r.tenant_id ?? ''}|${r.user_id ?? ''}`
      default:
        return r.category
    }
  }
  const merged = new Map<string, UsageSummaryRow>()
  for (const r of rows.value) {
    const key = keyOf(r)
    const acc = merged.get(key)
    if (!acc) {
      merged.set(key, { ...r, model_id: '', model_name: '' })
      continue
    }
    acc.calls += r.calls
    acc.input_tokens += r.input_tokens
    acc.output_tokens += r.output_tokens
    acc.cached_tokens += r.cached_tokens
    acc.images = (acc.images ?? 0) + (r.images ?? 0)
    acc.audio_seconds = (acc.audio_seconds ?? 0) + (r.audio_seconds ?? 0)
    acc.amount += r.amount
    acc.approximate = acc.approximate || r.approximate
    acc.has_price_config = acc.has_price_config || r.has_price_config
  }
  const list = [...merged.values()]
  const byUser = groupBy.value === 'user' || groupBy.value === 'tenant_user'
  if (props.userNames && byUser) {
    return list.filter((r) => !!r.user_id && !!props.userNames?.[r.user_id])
  }
  return list
})

const userDisplayName = (id?: string) => (id ? props.userNames?.[id] || id : '—')

const showDayChart = computed(() => props.dayChart === true && groupBy.value === 'day')

// 柱状图数据：每日一根柱，柱高 = 输入+输出+缓存命中（total / max 归一）。
// 选定时间范围内没有用量的日子补零根柱，时间轴保持连续（默认 8 天里
// 往往只有零星几天有调用）。超过 92 天的范围不再补零，只画有数据的天。
const chartData = computed(() => {
  const rowsByDay = new Map(displayRows.value.map((r) => [r.day ?? '', r]))
  const from = asDayString(dateRange.value?.[0])
  const to = asDayString(dateRange.value?.[1])
  let days: string[] | null = null
  if (from && to) {
    const list: string[] = []
    const cur = new Date(from + 'T00:00:00')
    const end = new Date(to + 'T00:00:00')
    while (cur <= end && list.length <= 92) {
      list.push(fmtDay(cur))
      cur.setDate(cur.getDate() + 1)
    }
    if (cur > end) days = list
  }
  const source = (days ?? [...rowsByDay.keys()]).map(
    (day) =>
      rowsByDay.get(day) ?? {
        day,
        category: '',
        calls: 0,
        input_tokens: 0,
        output_tokens: 0,
        cached_tokens: 0,
        images: 0,
        audio_seconds: 0,
        amount: 0,
        approximate: false,
        has_price_config: false,
      },
  )
  const pts = source.map((row) => ({
    row,
    total: row.input_tokens + row.output_tokens + row.cached_tokens,
  }))
  const max = pts.reduce((m, p) => Math.max(m, p.total), 0)
  return pts.map((p) => ({ ...p, max }))
})

// 行合并：同一维度下 category 列始终存在（后端 summaryDims 规则），
// 直接给每行编个稳定序号作 row-key 即可。
const tableRows = computed(() => displayRows.value.map((r, i) => ({ ...r, __key: i })))

const columns = computed(() => {
  type Col = { colKey: string; title: string; align?: string; width?: string | number; ellipsis?: boolean }
  const cols: Col[] = []
  if (groupBy.value === 'day' || displayRows.value.some((r) => r.day)) {
    cols.push({ colKey: 'day', title: t('usageStats.table.day'), width: 110 })
  }
  if (props.showTenantColumn && displayRows.value.some((r) => r.tenant_id != null)) {
    cols.push({ colKey: 'tenant', title: t('usageStats.table.tenant'), width: 90 })
  }
  if (props.showUserColumn && displayRows.value.some((r) => r.user_id)) {
    cols.push({ colKey: 'user_id', title: t('usageStats.table.user'), width: 140, ellipsis: true })
  }
  if (displayRows.value.some((r) => r.model_name || r.model_id)) {
    cols.push({ colKey: 'model', title: t('usageStats.table.model'), ellipsis: true })
  }
  // 简化形态（mergeRows）下的按用户视图不展示类型列：跨类型合并后
  // 单一类型标签没有意义（管理后台完整表格不受影响）。
  const byUser = groupBy.value === 'user' || groupBy.value === 'tenant_user'
  if (!(props.mergeRows && byUser)) {
    cols.push({ colKey: 'category', title: t('usageStats.table.category'), width: 110 })
  }
  cols.push({ colKey: 'calls', title: t('usageStats.table.calls'), align: 'right', width: 100 })
  cols.push({ colKey: 'input_tokens', title: t('usageStats.table.inputTokens'), align: 'right' })
  cols.push({ colKey: 'output_tokens', title: t('usageStats.table.outputTokens'), align: 'right' })
  if (displayRows.value.some((r) => (r.cached_tokens ?? 0) > 0)) {
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

// YYYY-MM-DD → 带本地时区偏移的当日零点 RFC3339。后端把纯日期补成
// UTC 零点，东八区直接传日期串会把当天 0–8 点漏在范围外；to 是排他
// 上界，结束日 +1 天才能把选中当天完整计入。
const localDayStart = (day: unknown, addDays = 0): string | undefined => {
  const dayStr = asDayString(day)
  if (!dayStr) return undefined
  const d = new Date(dayStr + 'T00:00:00')
  d.setDate(d.getDate() + addDays)
  const off = -d.getTimezoneOffset()
  const sign = off >= 0 ? '+' : '-'
  const abs = Math.abs(off)
  const pad = (v: number) => String(v).padStart(2, '0')
  return `${fmtDay(d)}T00:00:00${sign}${pad(Math.floor(abs / 60))}:${pad(abs % 60)}`
}

async function reload() {
  loading.value = true
  error.value = ''
  try {
    const params: UsageQueryParams = { group_by: groupBy.value }
    if (categoryFilter.value) params.category = categoryFilter.value
    if (dateRange.value && dateRange.value.length === 2) {
      if (dateRange.value[0]) params.from = localDayStart(dateRange.value[0])
      if (dateRange.value[1]) params.to = localDayStart(dateRange.value[1], 1)
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
.usage-cell-tenant,
.usage-cell-user {
  font-variant-numeric: tabular-nums;
}

/* ── 按天维度的柱状图（个人面板形态，无图表库依赖，纯 CSS） ── */
.usage-chart-card {
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--td-radius-medium);
  padding: 16px;
  background: var(--td-bg-color-container);
}

.usage-chart-legend {
  display: flex;
  justify-content: flex-end;
  gap: 16px;
  margin-bottom: 10px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.usage-chart-legend-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.usage-chart-legend-item .usage-seg {
  width: 10px;
  height: 10px;
  border-radius: 2px;
}

.usage-chart {
  display: flex;
  align-items: stretch;
  gap: 8px;
  height: 200px;
}

.usage-chart-col {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
}

.usage-chart-track {
  width: 100%;
  max-width: 44px;
  flex: 1;
  display: flex;
  align-items: flex-end;
  justify-content: center;
  cursor: default;
}

.usage-chart-bar {
  width: 100%;
  display: flex;
  flex-direction: column;
  border-radius: 3px 3px 0 0;
  overflow: hidden;
  /* 零用量的日子也留一个 2px 的底，保持横轴连续感 */
  min-height: 2px;
}

.usage-seg {
  display: block;
  width: 100%;
}

.usage-seg--input {
  background: var(--td-brand-color);
}

.usage-seg--output {
  background: var(--td-success-color);
}

.usage-seg--cached {
  background: var(--td-warning-color);
}

.usage-chart-xlabel {
  margin-top: 6px;
  font-size: 11px;
  color: var(--td-text-color-secondary);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.usage-chart-pop {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 12px;
  line-height: 1.5;
}

.usage-chart-pop-day {
  font-weight: 600;
  margin-bottom: 2px;
}
</style>
