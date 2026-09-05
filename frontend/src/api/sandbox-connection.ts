import { get, post, put, del } from '@/utils/request'
import type {
  SandboxConfig,
  SandboxCheckResult,
  SandboxInventory,
  SandboxTemplateCatalog,
} from '@/api/system'

// ----------------------------------------------------------------------------
// 平台级「沙箱连接」（000097）：系统管理员在控制台配置一次连接，再按空间
// 分配。分配不是引用而是物化 —— 每个空间得到一条普通的 tenant_sandbox_configs
// 行（带 source_connection_id），所以连接编辑不会自动传播：只有显式的
// 「推送更新」才会把新负载写入各行，且逐空间报告结果。
// ----------------------------------------------------------------------------

/** A platform connection as the admin console sees it (secrets masked). */
export interface SystemSandboxConnection {
  id: string
  name: string
  description?: string
  sandbox_type: string
  config: SandboxConfig
  /** Always present (possibly empty) — the materialized workspace rows. */
  assignments: SandboxConnectionAssignment[]
  created_at?: string
  updated_at?: string
}

/**
 * One workspace's materialized row. `drift` means the connection changed
 * after this row last received a push — the amber dot on the assignment chip.
 */
export interface SandboxConnectionAssignment {
  tenant_id: number
  tenant_name: string
  config_id: string
  config_name: string
  pushed_at?: string
  drift: boolean
  skill_count: number
}

export type AssignmentOutcomeStatus = 'assigned' | 'unassigned' | 'blocked' | 'error'

/**
 * Per-workspace result of a replace-all assignment PUT. Partial success is
 * the norm: `blocked` keeps that workspace's row (and the connection's
 * assignment list) untouched while the others land.
 */
export interface AssignmentOutcome {
  tenant_id: number
  tenant_name: string
  status: AssignmentOutcomeStatus
  /** skills_installed | sandboxes_still_live | sandbox_inventory_unverifiable | skill_snapshot_release_failed */
  code?: string
  message?: string
  config_id?: string
  /** The materialized row's name, which may be auto-suffixed on collision. */
  materialized_name?: string
  /** Present for code=skills_installed. */
  skill_names?: string[]
  /** Present for code=sandboxes_still_live. */
  inventory?: SandboxInventory
}

export type PushOutcomeStatus =
  | 'updated'
  | 'blocked_live_sandboxes'
  | 'blocked_skill_snapshot'
  | 'skipped_cordoned'
  | 'error'

/** Per-workspace result of a push. Never a request failure — 200 always. */
export interface PushOutcome {
  tenant_id: number
  tenant_name: string
  config_id: string
  status: PushOutcomeStatus
  code?: string
  message?: string
  inventory?: SandboxInventory
}

export interface PushResult {
  results: PushOutcome[]
  updated: number
  blocked: number
}

/** Body of the replace-all assignment PUT. */
export interface SandboxConnectionAssignmentUpdateResult {
  assignments: SandboxConnectionAssignment[]
  results: AssignmentOutcome[]
}

const BASE = '/api/v1/system/admin/sandbox-connections'

// List the whole platform connection catalog (with assignments)
export async function listSystemSandboxConnections(): Promise<SystemSandboxConnection[]> {
  const response: any = await get(BASE)
  if (response && Array.isArray(response.data)) {
    return response.data
  }
  return []
}

// Get a single platform connection by ID (with assignments)
export async function getSystemSandboxConnection(id: string): Promise<SystemSandboxConnection> {
  const response: any = await get(`${BASE}/${id}`)
  return response.data
}

// Create a new platform connection (starts unassigned)
export async function createSystemSandboxConnection(data: {
  name: string
  description?: string
  config: SandboxConfig
}): Promise<SystemSandboxConnection> {
  const response: any = await post(BASE, data)
  return response.data
}

/**
 * Update a connection. Edits do NOT propagate to assigned workspaces — they
 * light drift markers that the explicit push action clears.
 */
export async function updateSystemSandboxConnection(
  id: string,
  data: {
    name: string
    description?: string
    config: SandboxConfig
  },
): Promise<SystemSandboxConnection> {
  const response: any = await put(`${BASE}/${id}`, data)
  return response.data
}

/**
 * Delete a connection. Refused (409 assignments_exist) while any workspace
 * still holds a materialized row — unlike MCP there is no purge-on-delete,
 * because these rows are real configs that may own skills and sandboxes.
 */
export async function deleteSystemSandboxConnection(id: string): Promise<void> {
  await del(`${BASE}/${id}`)
}

// List the connection's materialized workspace rows (drift markers included)
export async function listSandboxConnectionTenantAssignments(
  id: string,
): Promise<SandboxConnectionAssignment[]> {
  const response: any = await get(`${BASE}/${id}/tenant-assignments`)
  if (response && Array.isArray(response.data)) {
    return response.data
  }
  return []
}

/**
 * Replace the full assignment list. `rows` carries the target workspace set;
 * a workspace missing from the list is unassigned (guarded: rows with
 * installed skills or live sandboxes stay assigned and come back `blocked`).
 */
export async function updateSandboxConnectionTenantAssignments(
  id: string,
  rows: Array<{ tenant_id: number }>,
): Promise<SandboxConnectionAssignmentUpdateResult> {
  const response: any = await put(`${BASE}/${id}/tenant-assignments`, { assignments: rows })
  return {
    assignments: response?.assignments ?? [],
    results: response?.results ?? [],
  }
}

/**
 * Push the connection's current payload to every materialized row. Rows with
 * live sandboxes are refused (cordon flow), skill-snapshot rows refuse
 * identity changes — each is a per-row outcome, never a request failure.
 */
export async function pushSystemSandboxConnection(id: string): Promise<PushResult> {
  const response: any = await post(`${BASE}/${id}/push`, {})
  return {
    results: response?.results ?? [],
    updated: response?.updated ?? 0,
    blocked: response?.blocked ?? 0,
  }
}

/**
 * Probe a sandbox connection. With `connection_id` the saved payload resolves
 * masked secrets server-side (`/:id/check`), so edits can be tested without
 * retyping the API key; without it the draft config is validated as-is
 * (`/check`). `deep` consumes real sandbox time.
 */
export async function checkSystemSandboxConnection(payload: {
  config?: SandboxConfig
  connection_id?: string
  deep?: boolean
}): Promise<SandboxCheckResult> {
  const { connection_id, ...body } = payload
  const path = connection_id ? `${BASE}/${connection_id}/check` : `${BASE}/check`
  const response: any = await post(path, body)
  return response.data
}

/**
 * Template query for the connection drawer. `connection_id` resolves masked
 * credentials against the saved connection (required for replace_standard —
 * the rebuilt template id is persisted onto the connection).
 */
export async function querySystemSandboxTemplates(payload: {
  config?: SandboxConfig
  connection_id?: string
  ensure_standard?: boolean
  replace_standard?: boolean
}): Promise<SandboxTemplateCatalog> {
  const response: any = await post(`${BASE}/templates/query`, payload)
  return response.data
}
