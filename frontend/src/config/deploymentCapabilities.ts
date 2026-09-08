export const DEPLOYMENT_CAPABILITY_KEYS = [
  'organizations',
  'agents',
  'integrations.im',
  'integrations.embed',
  'integrations.api',
  'settings.mcp',
  'settings.websearch',
  'settings.vectorstore',
  'settings.storage',
  'settings.sandbox',
  'settings.sandbox.docker',
] as const

export type DeploymentCapabilityKey = typeof DEPLOYMENT_CAPABILITY_KEYS[number]

export interface DeploymentCapability {
  supported: boolean
  reason?: string
}

export type DeploymentCapabilityMap = Partial<Record<DeploymentCapabilityKey, DeploymentCapability>>

/**
 * 能力接口失败或旧版后端没有返回某个键时保持可见，避免一次探测失败把整个菜单清空。
 * 只有后端明确返回 supported: false 时才隐藏入口。
 */
export function isDeploymentCapabilitySupported(
  capabilities: DeploymentCapabilityMap,
  key?: DeploymentCapabilityKey,
  options?: { liteMode?: boolean; edition?: string },
): boolean {
  if (!key) return true
  if (key === 'organizations') {
    const isLite =
      options?.liteMode === true ||
      options?.edition?.trim().toLowerCase() === 'lite'
    if (isLite) return false
  }
  // Docker talks to a local Engine API (often docker.sock = host root), so
  // missing or failed capability probes must not leave the picker visible.
  if (key === 'settings.sandbox.docker') {
    return capabilities[key]?.supported === true
  }
  return capabilities[key]?.supported !== false
}

export const SETTINGS_SECTION_CAPABILITY: Partial<Record<string, DeploymentCapabilityKey>> = {
  // Skill credentials exist only because sandboxes do: the values are injected
  // into a skill script's process. A deployment without sandbox support has
  // nowhere to inject them, so the page would only ever show its empty state.
  envvars: 'settings.sandbox',
  // Read-only skill catalog (000099) rides the same gate: skills are baked
  // into sandbox images, so a deployment without sandbox support has no
  // catalog content to show either.
  'skill-catalog': 'settings.sandbox',
}

// 系统管理后台「按空间代管」面板的能力裁剪：向量库/解析引擎/存储/网络
// 搜索/MCP 等面板从工作空间 Settings 迁入控制台，能力键沿用原
// SETTINGS_SECTION_CAPABILITY 的值，部署不支持时控制台菜单同样隐藏。
// 沙箱配置面板并入「沙箱连接」（000097 分配制）、技能面板迁回工作空间
// Settings 只读目录（000099）后，各自的独立键随之移除。
export const CONSOLE_SECTION_CAPABILITY: Partial<Record<string, DeploymentCapabilityKey>> = {
  websearch: 'settings.websearch',
  vectorstore: 'settings.vectorstore',
  storage: 'settings.storage',
  // Platform storage engines share the same capability gate as workspace storage.
  // A deployment with storage support can configure platform-level engines.
  'platform-storage': 'settings.storage',
  // Platform sandbox connections (000097) share the sandbox capability gate:
  // a deployment without sandbox support has nothing to configure here.
  'sandbox-connections': 'settings.sandbox',
  // Platform skill library (000098) shares the same gate: skills only mean
  // something where sandboxes can install them.
  'skill-library': 'settings.sandbox',
  mcp: 'settings.mcp',
}
