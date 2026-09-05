import { get, post, put, del, postUpload, putUpload } from '@/utils/request'
import type { ConfigSkillFileContent, ConfigSkillFileEntry } from '@/api/system'

// ----------------------------------------------------------------------------
// 平台级「技能库」（000098）：系统管理员在控制台注册一次技能，再按空间分
// 配。分配不是引用而是物化 —— 每个空间得到一条普通的 tenant_skill_catalog
// 行（带 source_platform_skill_id 溯源），安装到沙箱仍是空间自己的决定。
// 技能编辑不自动传播：只有显式的「推送更新」才会把新包重注册进各行，且
// 逐空间报告结果；本地删掉物化行的空间由推送「治愈」（重新物化）。
// ----------------------------------------------------------------------------

/** A platform skill as the admin console sees it (bundle metadata, no body). */
export interface PlatformSkill {
  id: string
  name: string
  version?: string
  description?: string
  /** Provenance only — '@owner/slug', a URL, or 'upload'. Never re-fetched. */
  source?: string
  bundle_sha256?: string
  /** Always present (possibly empty) — the assigned workspace list. */
  assignments: PlatformSkillAssignment[]
  created_at?: string
  updated_at?: string
}

/**
 * One workspace's assignment. `drift` means the skill changed after this
 * workspace last received a push — the amber dot on the assignment chip.
 */
export interface PlatformSkillAssignment {
  tenant_id: number
  tenant_name: string
  catalog_id?: string
  catalog_name?: string
  install_count: number
  pushed_at?: string
  drift: boolean
}

export type SkillAssignmentOutcomeStatus = 'assigned' | 'unassigned' | 'blocked' | 'error'

/**
 * Per-workspace result of a replace-all assignment PUT. Partial success is
 * the norm: a `blocked` workspace keeps (or refuses) only its own row while
 * the others land.
 */
export interface SkillAssignmentOutcome {
  tenant_id: number
  tenant_name: string
  status: SkillAssignmentOutcomeStatus
  /** name_conflict | skills_installed */
  code?: string
  message?: string
  catalog_id?: string
  /** Present for code=skills_installed. */
  skill_names?: string[]
}

export type SkillPushOutcomeStatus = 'updated' | 'healed' | 'blocked' | 'error'

/**
 * Per-workspace result of a push. Never a request failure — 200 always.
 * `healed` re-materialized a row the workspace had deleted locally.
 */
export interface SkillPushOutcome {
  tenant_id: number
  tenant_name: string
  status: SkillPushOutcomeStatus
  /** name_conflict */
  code?: string
  message?: string
  catalog_id?: string
  skill_names?: string[]
}

export interface PlatformSkillPushResult {
  results: SkillPushOutcome[]
  /** updated + healed — how many rows are in sync now. */
  updated: number
  healed: number
  blocked: number
}

/** Body of the replace-all assignment PUT. */
export interface PlatformSkillAssignmentUpdateResult {
  assignments: PlatformSkillAssignment[]
  results: SkillAssignmentOutcome[]
}

const BASE = '/api/v1/system/admin/skills'

// List the whole platform skill catalog (with assignments)
export async function listPlatformSkills(): Promise<PlatformSkill[]> {
  const response: any = await get(BASE)
  if (response && Array.isArray(response.data)) {
    return response.data
  }
  return []
}

// Get a single platform skill by ID (with assignments)
export async function getPlatformSkill(id: string): Promise<PlatformSkill> {
  const response: any = await get(`${BASE}/${id}`)
  return response.data
}

// Register a new platform skill from a source reference (ClawHub slug / URL)
export async function createPlatformSkillFromSource(
  source: string,
): Promise<PlatformSkill> {
  const response: any = await post(BASE, { source }, { timeout: 2 * 60 * 1000 })
  return response.data
}

// Register a new platform skill from an uploaded zip
export async function createPlatformSkillFromFile(
  file: File,
  onProgress?: (percent: number) => void,
): Promise<PlatformSkill> {
  const form = new FormData()
  form.append('file', file)
  const response: any = await postUpload(BASE, form, (e: any) => {
    if (e.total) onProgress?.(Math.round((e.loaded * 100) / e.total))
  }, { timeout: 5 * 60 * 1000 })
  return response.data
}

/**
 * Re-register a skill's bundle (same name — renaming is a new skill). Edits
 * do NOT propagate to assigned workspaces: they light drift markers that the
 * explicit push action clears.
 */
export async function updatePlatformSkillFromSource(
  id: string,
  source: string,
): Promise<PlatformSkill> {
  const response: any = await put(`${BASE}/${id}`, { source }, { timeout: 2 * 60 * 1000 })
  return response.data
}

// Re-register a skill's bundle from an uploaded zip
export async function updatePlatformSkillFromFile(
  id: string,
  file: File,
  onProgress?: (percent: number) => void,
): Promise<PlatformSkill> {
  const form = new FormData()
  form.append('file', file)
  const response: any = await putUpload(`${BASE}/${id}`, form, (e: any) => {
    if (e.total) onProgress?.(Math.round((e.loaded * 100) / e.total))
  }, { timeout: 5 * 60 * 1000 })
  return response.data
}

/**
 * Delete a skill. Refused (409 assignments_exist) while any workspace is
 * still assigned — the materialized rows are real catalog rows that may own
 * installs, so unassigning is a guarded per-workspace decision.
 */
export async function deletePlatformSkill(id: string): Promise<void> {
  await del(`${BASE}/${id}`)
}

// List the skill's assigned workspaces (drift markers included)
export async function listPlatformSkillTenantAssignments(
  id: string,
): Promise<PlatformSkillAssignment[]> {
  const response: any = await get(`${BASE}/${id}/tenant-assignments`)
  if (response && Array.isArray(response.data)) {
    return response.data
  }
  return []
}

/**
 * Replace the full assignment list. `rows` carries the target workspace set;
 * a workspace missing from the list is unassigned (guarded: rows with
 * installed skills stay assigned and come back `blocked`).
 */
export async function updatePlatformSkillTenantAssignments(
  id: string,
  rows: Array<{ tenant_id: number }>,
): Promise<PlatformSkillAssignmentUpdateResult> {
  const response: any = await put(`${BASE}/${id}/tenant-assignments`, { assignments: rows })
  return {
    assignments: response?.assignments ?? [],
    results: response?.results ?? [],
  }
}

/**
 * Push the skill's current bundle to every assignment. A workspace whose
 * local row was deleted is healed; one whose name a self-built row took over
 * is blocked — each is a per-row outcome, never a request failure. Push never
 * installs onto any sandbox.
 */
export async function pushPlatformSkill(id: string): Promise<PlatformSkillPushResult> {
  const response: any = await post(`${BASE}/${id}/push`, {})
  return {
    results: response?.results ?? [],
    updated: response?.updated ?? 0,
    healed: response?.healed ?? 0,
    blocked: response?.blocked ?? 0,
  }
}

// Browse the platform skill's stored bundle (the file drawer's platform mode)
export function listPlatformSkillFiles(id: string) {
  return get<{ data: ConfigSkillFileEntry[] }>(`${BASE}/${id}/files`)
}

export function getPlatformSkillFile(id: string, path: string) {
  return get<{ data: ConfigSkillFileContent }>(`${BASE}/${id}/files/content`, {
    params: { path },
  })
}
