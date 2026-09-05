import { get, post, put, del } from '@/utils/request'

// TenantRole mirrors internal/types/tenant_member.go's four-role enum.
// Keep the string values aligned with the Go constants.
export type TenantRole = 'owner' | 'admin' | 'contributor' | 'viewer'

export type TenantMemberStatus = 'active' | 'invited' | 'suspended'

// TenantMember is the API projection of a (user, tenant) membership row,
// already joined with the user's email/username/avatar by the backend.
export interface TenantMember {
  user_id: string
  employee_id: string
  email: string
  username: string
  avatar?: string
  role: TenantRole
  status: TenantMemberStatus
  invited_by?: string | null
  joined_at: string
  /** 最后登录时间；null = 从未登录（历史账号在下次登录前保持 null）。 */
  last_login_at?: string | null
}

export interface ListMembersResponse {
  success: boolean
  data?: {
    members: TenantMember[]
    total: number
    page?: number
    page_size?: number
  }
  message?: string
}

export interface ListMembersParams {
  page?: number
  page_size?: number
  /** 按邮箱/用户名筛选（服务端模糊匹配） */
  q?: string
}

function buildMembersQuery(params: ListMembersParams | undefined): string {
  if (!params) return ''
  const u = new URLSearchParams()
  if (params.page != null && params.page > 0) u.set('page', String(params.page))
  if (params.page_size != null && params.page_size > 0) u.set('page_size', String(params.page_size))
  const q = params.q?.trim()
  if (q) u.set('q', q)
  const qs = u.toString()
  return qs ? `?${qs}` : ''
}

export interface SimpleResponse {
  success: boolean
  message?: string
}

// 000101 空间管理员添加成员：工号 + 姓名（+可选密码）。已有平台账号的
// 工号直接绑定为普通成员（created=false），否则创建新账号（默认密码
// abc1234# 由后端补齐并在响应里原样返回一次，供弹窗一次性展示）。
export interface AddTenantMemberRequest {
  employee_id: string
  username: string
  password?: string
}

export interface AddTenantMemberResponse {
  success: boolean
  data?: {
    member: TenantMember
    created: boolean
    password: string
  }
  message?: string
}

export interface ResetMemberPasswordResponse {
  success: boolean
  data?: {
    /** 随机 8 位数字新密码，仅本次返回；成员下次登录须改密。 */
    new_password: string
  }
  message?: string
}

export interface MemberStats {
  /** 该成员在本空间上传的知识数量（未删除）。 */
  knowledge_count: number
  /** 该成员在本空间参与过的会话次数（未删除）。 */
  session_count: number
}

export interface MemberStatsResponse {
  success: boolean
  data?: MemberStats
  message?: string
}

/**
 * 分页列出空间成员。
 * Backend: GET /api/v1/tenants/:id/members (Viewer+)。查询参数：`q`、`page`、`page_size`。
 */
export async function listMembers(
  tenantId: number,
  params: ListMembersParams = {},
): Promise<ListMembersResponse> {
  const qs = buildMembersQuery(params)
  return (await get(
    `/api/v1/tenants/${tenantId}/members${qs}`,
  )) as unknown as ListMembersResponse
}

/**
 * 遍历分页拉取空间的全部成员（每页最大 100，最多 500 页兜底）。
 * 用于「退出空间」等对全量成员的轻量校验；普通表格请直接使用 {@link listMembers} 分页接口。
 */
export async function fetchAllTenantMembers(tenantId: number): Promise<TenantMember[]> {
  const pageSize = 100
  let page = 1
  const out: TenantMember[] = []
  let total = Number.POSITIVE_INFINITY
  for (let guard = 0; guard < 500 && out.length < total; guard++) {
    const resp = await listMembers(tenantId, { page, page_size: pageSize })
    if (!resp.success || !resp.data) break
    total = resp.data.total
    const batch = resp.data.members || []
    if (batch.length === 0 && page >= 2) break
    out.push(...batch)
    if (batch.length < pageSize) break
    page++
  }
  return out
}

/**
 * 添加成员（000101 空间管理员弹窗）。
 * Backend: POST /api/v1/tenants/:id/members (Admin+)。
 *
 * 工号已存在平台账号 → 直接绑定为普通成员（created=false）；否则创建
 * 新账号，默认密码 abc1234#（可经 body.password 覆盖），首次登录须改密。
 * 400：工号已存在但请求带了不同姓名 / 该用户已是本空间成员 / 账号已停用。
 */
export async function addTenantMember(
  tenantId: number,
  body: AddTenantMemberRequest,
): Promise<AddTenantMemberResponse> {
  return (await post(`/api/v1/tenants/${tenantId}/members`, body)) as unknown as AddTenantMemberResponse
}

/**
 * 将成员密码重置为随机 8 位数字并吊销其会话。
 * Backend: POST /api/v1/tenants/:id/members/:user_id/reset-password (Admin+)。
 * 新密码仅本次响应返回；成员下次登录会被要求改成符合策略的密码。
 */
export async function resetMemberPassword(
  tenantId: number,
  userId: string,
): Promise<ResetMemberPasswordResponse> {
  return (await post(
    `/api/v1/tenants/${tenantId}/members/${userId}/reset-password`,
  )) as unknown as ResetMemberPasswordResponse
}

/**
 * 成员在本空间的用量统计（上传知识数 / 参与会话数）。
 * Backend: GET /api/v1/tenants/:id/members/:user_id/stats (Admin+)。
 */
export async function getMemberStats(
  tenantId: number,
  userId: string,
): Promise<MemberStatsResponse> {
  return (await get(
    `/api/v1/tenants/${tenantId}/members/${userId}/stats`,
  )) as unknown as MemberStatsResponse
}

/**
 * Change an existing member's role.
 * Backend: PUT /api/v1/tenants/:id/members/:user_id (Owner+).
 *
 * Returns 409 when this would demote the last active Owner of the tenant.
 */
export async function updateMemberRole(
  tenantId: number,
  userId: string,
  role: TenantRole,
): Promise<SimpleResponse> {
  return (await put(`/api/v1/tenants/${tenantId}/members/${userId}`, { role })) as unknown as SimpleResponse
}

/**
 * Remove a member from the tenant (移出空间).
 * Backend: DELETE /api/v1/tenants/:id/members/:user_id (Admin+)。
 *
 * 403 when the target is the caller themselves or another workspace
 * admin — those need the system admin (unbind via /system/admin).
 */
export async function removeMember(
  tenantId: number,
  userId: string,
): Promise<SimpleResponse> {
  return (await del(`/api/v1/tenants/${tenantId}/members/${userId}`)) as unknown as SimpleResponse
}

/**
 * Quit the tenant on your own. Same last-Owner invariant as
 * removeMember, but does NOT require Owner+ — any active member can
 * call it.
 * Backend: POST /api/v1/tenants/:id/leave (Viewer+).
 */
export async function leaveTenant(tenantId: number): Promise<SimpleResponse> {
  return (await post(`/api/v1/tenants/${tenantId}/leave`)) as unknown as SimpleResponse
}
