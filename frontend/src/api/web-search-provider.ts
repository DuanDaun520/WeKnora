import { get, post, put, del } from '@/utils/request'

// WebSearchProviderEntity represents a configured web search provider instance
export interface WebSearchProviderEntity {
  id?: string
  tenant_id?: number
  name: string
  provider: 'bing' | 'google' | 'duckduckgo' | 'tavily' | 'ollama' | 'baidu' | 'searxng' | 'keenable' | 'zhipu' | 'metaso' | 'exa' | 'serpbase'
  description?: string
  parameters: {
    // api_key is never returned by the server in this shape; it lives behind
    // the /credentials subresource. Kept on the type so the initial create
    // POST can still include it.
    api_key?: string
    engine_id?: string
    base_url?: string
    proxy_url?: string
    extra_config?: Record<string, string>
  }
  is_default?: boolean
  // Per-field configured? metadata from the main response.
  credentials?: Record<WebSearchCredentialField, { configured: boolean }>
  created_at?: string
  updated_at?: string
}

// Workspace assignment of a platform search service (000095 platform rework):
// a service is configured once at platform level and shared with workspaces
// through these rows; at most one assigned service per workspace is the
// default (flagged here, else the earliest assignment acts as one).
export interface WebSearchProviderTenantAssignment {
  tenant_id: number
  tenant_name: string
  provider_id?: string
  is_default: boolean
  assigned_at?: string
}

// Platform catalog row as returned by the /system/admin endpoints — the
// tenant-scoped entity shape plus the service's workspace assignments.
// is_default is deliberately absent: defaults live on assignments only.
export interface SystemWebSearchProviderEntity extends WebSearchProviderEntity {
  assignments?: WebSearchProviderTenantAssignment[]
}

// WebSearchProviderTypeInfo describes metadata for a provider type
export interface WebSearchProviderTypeInfo {
  id: string
  name: string
  requires_api_key: boolean
  // Keyless-by-default providers that still accept an optional key (e.g. Keenable).
  supports_optional_api_key?: boolean
  requires_engine_id?: boolean
  requires_base_url?: boolean
  supports_proxy?: boolean
  description?: string
  docs_url?: string
  config_fields?: WebSearchProviderConfigField[]
}

export interface WebSearchProviderConfigField {
  key: string
  label: string
  label_key?: string
  type: 'select'
  required?: boolean
  default?: string
  description?: string
  description_key?: string
  options?: Array<{ label: string; label_key?: string; value: string }>
}

// ---- 管理面（000095 平台化）：搜索服务由系统管理员在平台目录配置一次，
// 再按空间分配。全部走 /system/admin 镜像路由，仅系统管理员可用。 ----

// Create a new platform web search provider
export function createWebSearchProvider(data: Partial<SystemWebSearchProviderEntity>) {
  return post('/api/v1/system/admin/web-search-providers', data)
}

// List the whole platform search-service catalog (admin console)
export function listSystemWebSearchProviders(): Promise<SystemWebSearchProviderEntity[]> {
  return get('/api/v1/system/admin/web-search-providers').then((res: any) => {
    if (res.success && Array.isArray(res.data)) {
      return res.data
    }
    return []
  })
}

// Get a single platform web search provider by ID (with assignments)
export function getWebSearchProvider(id: string) {
  return get(`/api/v1/system/admin/web-search-providers/${id}`)
}

// Update an existing platform web search provider
export function updateWebSearchProvider(id: string, data: Partial<SystemWebSearchProviderEntity>) {
  return put(`/api/v1/system/admin/web-search-providers/${id}`, data)
}

// Delete a platform web search provider (its assignments are purged server-side)
export function deleteWebSearchProvider(id: string) {
  return del(`/api/v1/system/admin/web-search-providers/${id}`)
}

// Get available provider types (for dynamic form rendering) — console copy
export function listWebSearchProviderTypes(): Promise<WebSearchProviderTypeInfo[]> {
  return get('/api/v1/system/admin/web-search-providers/types').then((res: any) => {
    if (res.success && res.data) {
      return res.data
    }
    return []
  })
}

// ---- 空间读路径（保持原端点）：聊天开关 / 智能体编辑器解析当前空间的
// 已分配服务列表（is_default 为空间视角的有效默认）。 ----

// List the web search providers assigned to the current workspace
export function listWebSearchProviders() {
  return get('/api/v1/web-search-providers')
}

// ----------------------------------------------------------------------------
// Web search provider credential subresource (platform catalog).
// ----------------------------------------------------------------------------

export type WebSearchCredentialField = 'api_key'

export interface WebSearchCredentialsResponse {
  fields: Record<WebSearchCredentialField, { configured: boolean }>
}

export async function putWebSearchProviderCredentials(
  id: string,
  body: Partial<Record<WebSearchCredentialField, string>>,
): Promise<WebSearchCredentialsResponse> {
  const response: any = await put(`/api/v1/system/admin/web-search-providers/${id}/credentials`, body)
  return (response.data ?? response) as WebSearchCredentialsResponse
}

export async function deleteWebSearchProviderCredentialField(
  id: string,
  field: WebSearchCredentialField,
): Promise<void> {
  await del(`/api/v1/system/admin/web-search-providers/${id}/credentials/${field}`)
}

// Test a web search provider connection.
// If id is provided, tests the existing saved provider.
// If data is provided, tests with raw credentials (no persistence).
export function testWebSearchProvider(id?: string, data?: { provider: string; parameters: any }): Promise<any> {
  if (id) {
    return post(`/api/v1/system/admin/web-search-providers/${id}/test`, {})
  }
  return post('/api/v1/system/admin/web-search-providers/test', data || {})
}

// ----------------------------------------------------------------------------
// Workspace assignments of a platform search service.
// ----------------------------------------------------------------------------

export function listWebSearchProviderTenantAssignments(id: string): Promise<WebSearchProviderTenantAssignment[]> {
  return get(`/api/v1/system/admin/web-search-providers/${id}/tenant-assignments`).then((res: any) => {
    if (res.success && Array.isArray(res.data)) {
      return res.data
    }
    return []
  })
}

// Replace the full assignment list of a service. rows carries the target
// state; a workspace missing from the list is unassigned.
export function updateWebSearchProviderTenantAssignments(
  id: string,
  rows: Array<{ tenant_id: number; is_default: boolean }>,
) {
  return put(`/api/v1/system/admin/web-search-providers/${id}/tenant-assignments`, { assignments: rows })
}
