import { get, put, del } from '@/utils/request'

// ─────────────────────────────────────────────────────────────────────────────
// Token 统计与计费（docs/Token统计与计费设计.md §5）的前端 API 模块。
// 三级入口共用这一组函数：
//   个人设置面板        → getMyUsageSummary      GET /me/usage/summary
//   空间管理员面板      → getTenantUsageSummary  GET /tenants/:id/usage/summary
//   管理后台看板/配价   → getAdminUsageSummary 等  GET /system/admin/usage/*
// 均为浏览器会话（JWT）接口：API-Key 主体在网关层被默认拒绝。
// ─────────────────────────────────────────────────────────────────────────────

// UsageCategory mirrors internal/types/usage_record.go 的常量。
export type UsageCategory = 'chat' | 'embedding' | 'rerank' | 'vlm' | 'asr'

// UsageSummaryRow mirrors internal/types/interfaces/usage_report.go。
// 维度列（day/model_name/user_id…）按 group_by 选择性出现，其余为 omitempty。
export interface UsageSummaryRow {
  tenant_id?: number
  user_id?: string
  day?: string
  model_id?: string
  model_name?: string
  category: UsageCategory | string
  calls: number
  input_tokens: number
  output_tokens: number
  cached_tokens: number
  images: number
  audio_seconds: number
  amount: number
  approximate: boolean
  has_price_config: boolean
}

export interface UsageSummaryResponse {
  rows: UsageSummaryRow[]
}

// ModelPrice mirrors internal/types/usage_record.go 的 model_prices 表。
// 金额口径：每百万 Token 的单价（_per_m 后缀）、每张图片、每分钟音频。
export interface ModelPrice {
  model_id: string
  price_input_per_m: number
  price_output_per_m: number
  price_cached_per_m: number
  price_per_m: number
  price_per_image: number
  price_per_audio_min: number
  price_per_video_min: number
  currency: string
  enabled: boolean
  effective_from: string
}

// AdminUsageRecords 的明细行，mirror internal/types/usage_record.go 的
// ModelUsageRecord（金额列只在汇总接口折叠，明细不带）。
export interface UsageRecord {
  tenant_id: number
  user_id?: string
  model_id?: string
  model_name: string
  category: UsageCategory | string
  purpose?: string
  status: 'success' | 'failed' | 'interrupted' | string
  input_tokens: number
  output_tokens: number
  cached_tokens: number
  images?: number
  audio_seconds?: number
  occurred_at: string
}

export interface UsageRecordsResponse {
  rows: UsageRecord[]
  total: number
  limit: number
  offset: number
}

export interface ModelPricesResponse {
  prices: ModelPrice[]
}

export type UsageGroupBy = '' | 'model' | 'category' | 'day' | 'user' | 'tenant' | 'tenant_user'

export interface UsageQueryParams {
  // 聚合维度；空串表示后端默认（总量按 category 细分）。
  group_by?: UsageGroupBy
  // 时间范围，YYYY-MM-DD 或 RFC3339；后端把日期补全为当天 00:00:00Z。
  from?: string
  to?: string
  category?: UsageCategory | string
  purpose?: string
  model_id?: string
  // 管理后台看板的下钻层级：platform（总量）/ tenant / user / tenant_user。
  level?: 'platform' | 'tenant' | 'user' | 'tenant_user'
  tenant_id?: number
  user_id?: string
  limit?: number
  offset?: number
}

function buildQuery(params: UsageQueryParams = {}): string {
  const qs = new URLSearchParams()
  if (params.group_by) qs.append('group_by', params.group_by)
  if (params.from) qs.append('from', params.from)
  if (params.to) qs.append('to', params.to)
  if (params.category) qs.append('category', params.category)
  if (params.purpose) qs.append('purpose', params.purpose)
  if (params.model_id) qs.append('model_id', params.model_id)
  if (params.level) qs.append('level', params.level)
  if (params.tenant_id != null) qs.append('tenant_id', String(params.tenant_id))
  if (params.user_id) qs.append('user_id', params.user_id)
  if (params.limit != null) qs.append('limit', String(params.limit))
  if (params.offset != null) qs.append('offset', String(params.offset))
  const tail = qs.toString()
  return tail ? '?' + tail : ''
}

/**
 * 个人 AI 使用统计（设置 → 个人用量）。身份取自认证上下文，
 * 只返回当前空间内本人的用量。Backend: GET /api/v1/me/usage/summary。
 */
export async function getMyUsageSummary(params: UsageQueryParams = {}): Promise<UsageSummaryResponse> {
  return (await get(`/api/v1/me/usage/summary${buildQuery(params)}`)) as unknown as UsageSummaryResponse
}

/**
 * 空间管理员面板。scope='me' 只看本人，scope='tenant' 看全空间。
 * Backend: GET /api/v1/tenants/:id/usage/summary（Admin+，PathTenantMatch）。
 */
export async function getTenantUsageSummary(
  tenantId: number,
  scope: 'me' | 'tenant',
  params: UsageQueryParams = {},
): Promise<UsageSummaryResponse> {
  const qs = buildQuery(params)
  const scopeSuffix = scope === 'me' ? (qs ? '&' : '?') + 'scope=me' : ''
  return (await get(
    `/api/v1/tenants/${tenantId}/usage/summary${qs}${scopeSuffix}`,
  )) as unknown as UsageSummaryResponse
}

/**
 * 管理后台看板。level 决定聚合维度（platform/tenant/user/tenant_user），
 * 可用 tenant_id / user_id 下钻。Backend: GET /api/v1/system/admin/usage/summary。
 */
export async function getAdminUsageSummary(params: UsageQueryParams = {}): Promise<UsageSummaryResponse> {
  return (await get(`/api/v1/system/admin/usage/summary${buildQuery(params)}`)) as unknown as UsageSummaryResponse
}

/**
 * 管理后台用量明细（分页）。Backend: GET /api/v1/system/admin/usage/records。
 */
export async function getAdminUsageRecords(params: UsageQueryParams = {}): Promise<UsageRecordsResponse> {
  return (await get(`/api/v1/system/admin/usage/records${buildQuery(params)}`)) as unknown as UsageRecordsResponse
}

/** 模型单价配置列表。Backend: GET /api/v1/system/admin/model-prices。 */
export async function listModelPrices(): Promise<ModelPricesResponse> {
  return (await get('/api/v1/system/admin/model-prices')) as unknown as ModelPricesResponse
}

/** 新增/覆盖某模型的单价。Backend: PUT /api/v1/system/admin/model-prices/:model_id。 */
export async function upsertModelPrice(modelId: string, price: Partial<ModelPrice>): Promise<{ price: ModelPrice }> {
  return (await put(`/api/v1/system/admin/model-prices/${encodeURIComponent(modelId)}`, price)) as unknown as {
    price: ModelPrice
  }
}

/** 删除某模型的单价（其历史用量按未配价折叠为 0）。Backend: DELETE /api/v1/system/admin/model-prices/:model_id。 */
export async function deleteModelPrice(modelId: string): Promise<{ deleted: string }> {
  return (await del(`/api/v1/system/admin/model-prices/${encodeURIComponent(modelId)}`)) as unknown as {
    deleted: string
  }
}
