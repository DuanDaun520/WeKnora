export type SettingsRoleKey = 'viewer' | 'contributor' | 'admin' | 'owner'

/**
 * Workspace-scoped settings access policy.
 *
 * Keep this as the single frontend source of truth for both the complete
 * Settings navigation and any shortcuts that lead into it. Backend route
 * guards remain authoritative.
 */
export const SETTINGS_SECTION_MIN_ROLE: Record<string, SettingsRoleKey> = {
  general: 'viewer',
  // 模型 / Ollama / WeKnoraCloud 设置已随 000094 模型平台化、
  // websearch / vectorstore / parser / storage / sandbox / mcp 六项基础
  // 设施配置已随 000095 收权迁入系统管理控制台（/system/console）。
  // 技能以只读「技能目录」形态回到空间 Settings（000099）：目录与文件
  // 浏览对空间管理员开放，写动作仅系统管理员可见（见 SkillCatalogSettings）。
  chathistory: 'admin',
  // 版本信息仅空间管理员可见。
  system: 'admin',
  userprofile: 'viewer',
  tenant: 'viewer',
  members: 'viewer',
  mymemory: 'viewer',
  memory: 'admin',
  // 沙箱密钥（个人环境变量）对普通用户隐藏，仅空间管理员可见。
  envvars: 'admin',
  // 技能目录（000099）：空间管理员只读可见；系统管理员经 auth store 的
  // currentTenantRole='admin' 旁路天然可见并可操作。
  'skill-catalog': 'admin',
}

/**
 * A management-labelled avatar shortcut has a stricter threshold than the
 * corresponding read-only Settings page. Skills moved to the console
 * (000095): the shortcut is now system-admin-only, see UserMenu.vue.
 */
export const SETTINGS_MANAGEMENT_SHORTCUT_MIN_ROLE = {
  members: 'admin',
} as const satisfies Record<string, SettingsRoleKey>

export const SYSTEM_ADMIN_SETTINGS_SECTIONS = new Set([
  'system-global',
  'runtime-queues',
  'platform-api-keys',
  'system-audit-log',
])
