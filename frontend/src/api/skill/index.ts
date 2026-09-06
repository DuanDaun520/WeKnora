import { del, get, post, put } from "../../utils/request";
import type { ConfigSkillFileContent, ConfigSkillFileEntry } from "../system";

// Skill信息
export interface SkillInfo {
  name: string;
  description: string;
}

export interface SkillCatalogInstall {
  skill_id: string;
  sandbox_config_id: string;
  sandbox_config_name?: string;
  sandbox_type?: string;
  status: string;
  enabled: boolean;
  error?: string;
  bundle_sha256?: string;
  updated_at: string;
}

export interface SkillCatalogItem {
  id: string;
  name: string;
  version?: string;
  description?: string;
  bundle_sha256?: string;
  /** Present when the row was materialized from a platform skill (000098). */
  source_platform_skill_id?: string;
  /** Grouping category for the Skills/MCP browser (empty = uncategorized). */
  category?: string;
  /** Author metadata — SKILL.md frontmatter or platform push (empty = unknown). */
  author?: string;
  /** Space-level visibility (000108). false = hidden from the Skills/MCP browser,
   * the agent picker and @mention/runtime in this workspace. */
  visible?: boolean;
  /** Creator user id + read-time enriched display name. */
  created_by?: string;
  creator_name?: string;
  created_at: string;
  updated_at: string;
  installations: SkillCatalogInstall[];
}

// 获取当前沙箱配置上可执行的 Skills；未传 sandboxConfigId 或
// skills_available 为 false 时，前端应隐藏/禁用 Skills 配置
export function listSkills(sandboxConfigId?: string) {
  return get<{ data: SkillInfo[]; skills_available?: boolean }>('/api/v1/skills', {
    params: sandboxConfigId ? { sandbox_config_id: sandboxConfigId } : {},
  });
}

// includeHidden keeps 000108-hidden skills in the listing — only the 技能目录
// management page passes it so a hidden skill stays manageable; every browsing
// surface (Skills/MCP, agent editor, @mention) reads the default and never
// sees hidden rows.
export function listSkillCatalog(includeHidden = false) {
  return get<{ data: SkillCatalogItem[] }>('/api/v1/skills/catalog', {
    params: includeHidden ? { include_hidden: '1' } : {},
  });
}

// Registration has no tenant-side surface anymore: catalog rows materialize
// from the platform skill library's workspace assignments. POST
// /api/v1/skills/catalog (source / zip upload) was removed with it.

export function installSkillCatalog(catalogId: string, sandboxConfigIds: string[]) {
  return post<{ data: { installs: Record<string, string>; errors?: Record<string, string> } }>(
    `/api/v1/skills/catalog/${catalogId}/install`,
    { sandbox_config_ids: sandboxConfigIds },
  );
}

export function deleteSkillCatalog(catalogId: string) {
  return del(`/api/v1/skills/catalog/${catalogId}`);
}

// Edit the grouping category and/or the 000108 space visibility of a catalog
// row (space admin/owner or system admin). `visible` is optional: omitted keeps
// the current switch. Category-only callers (Skills/MCP saveCategory) pass no
// visible and never touch visibility.
export function updateSkillCatalogMeta(catalogId: string, category: string, visible?: boolean) {
  return put<{ data: { id: string; category: string; visible: boolean } }>(
    `/api/v1/skills/catalog/${catalogId}`,
    visible === undefined ? { category } : { category, visible },
  );
}

export function listCatalogSkillFiles(catalogId: string) {
  return get<{ data: ConfigSkillFileEntry[] }>(`/api/v1/skills/catalog/${catalogId}/files`);
}

export function getCatalogSkillFile(catalogId: string, path: string) {
  return get<{ data: ConfigSkillFileContent }>(`/api/v1/skills/catalog/${catalogId}/files/content`, {
    params: { path },
  });
}
