import type { DeploymentCapabilityKey } from './deploymentCapabilities'

export const CHROME_EXTENSION_URL =
  'https://chromewebstore.google.com/detail/jpemjbopikggjlmikmclgbmkhhopjdgd?utm_source=item-share-cb'

export const CLAWHUB_SKILL_URL = 'https://clawhub.ai/lyingbug/weknora'

export type IntegrationTab = 'im' | 'embed' | 'api' | 'chrome' | 'claw'

export const INTEGRATION_TABS: IntegrationTab[] = ['im', 'embed', 'api', 'chrome', 'claw']

/** Aligns with routes_auth_tenant.go g.AdminOrSystemAdmin() on /tenants/:id/api-keys & api-principal-config. */
export type IntegrationTabRole = 'viewer' | 'contributor' | 'admin' | 'owner'

export const INTEGRATION_TAB_MIN_ROLE: Partial<Record<IntegrationTab, IntegrationTabRole>> = {
  // API Key / API 主端点信息对齐后端（routes_auth_tenant.go 对
  // /tenants/:id/api-keys 与 api-principal-config 均为 AdminOrSystemAdmin），
  // 空间管理员即可见；此前 owner 收得过紧，成员管理员会看不到自己的入口。
  api: 'admin',
  // 企业版两级角色：IM 集成 / 网页嵌入 / Claw Skill 仅空间管理员可见；
  // Chrome 插件与所有用户相关，保持全员可见。
  im: 'admin',
  embed: 'admin',
  claw: 'admin',
}

export const INTEGRATION_TAB_CAPABILITY: Partial<Record<IntegrationTab, DeploymentCapabilityKey>> = {
  im: 'integrations.im',
  embed: 'integrations.embed',
  api: 'integrations.api',
}

export type IntegrationPreviewIcon =
  | { type: 'icon'; name: string }
  | { type: 'emoji'; value: string }

/** Sidebar hover preview + Integrations modal nav — add new entries here. */
export const INTEGRATION_PREVIEW_ITEMS: Array<{
  key: IntegrationTab
  icon: IntegrationPreviewIcon
}> = [
  { key: 'im', icon: { type: 'icon', name: 'chat-message' } },
  { key: 'embed', icon: { type: 'icon', name: 'code' } },
  { key: 'api', icon: { type: 'icon', name: 'secured' } },
  { key: 'chrome', icon: { type: 'icon', name: 'extension' } },
  { key: 'claw', icon: { type: 'emoji', value: '🦞' } },
]
