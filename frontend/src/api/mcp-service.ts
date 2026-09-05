import { get, post, put, del } from '@/utils/request'

export interface MCPService {
  id: string
  tenant_id?: number
  name: string
  description: string
  enabled: boolean
  transport_type: 'sse' | 'http-streamable' | 'stdio'
  url?: string // Optional: required for SSE/HTTP Streamable
  headers?: Record<string, string>
  auth_config?: {
    // Authentication strategy. Empty/absent means none. "oauth" enables the
    // per-user OAuth2 authorization-code flow (zero-config: discovery +
    // dynamic client registration).
    auth_type?: '' | 'api_key' | 'bearer' | 'oauth'
    // Secret fields (api_key, token) are NEVER returned by the server in
    // this shape — they live behind the /credentials subresource. The
    // optional-property typing remains so create-mode payloads can still
    // carry them in the initial POST body.
    api_key?: string
    // Header name carrying api_key when auth_type is "api_key". Non-secret;
    // empty defaults to "X-API-Key". Lets services expecting the key in a
    // different header (e.g. raw token in "Authorization") work.
    api_key_header?: string
    token?: string
    custom_headers?: Record<string, string>
    // OAuth-only, non-secret configuration.
    scopes?: string[]
    auth_server_metadata_url?: string
  }
  advanced_config?: {
    timeout?: number
    retry_count?: number
    retry_delay?: number
  }
  stdio_config?: {
    command: 'uvx' | 'npx' // Command: uvx or npx
    args: string[] // Command arguments array
  }
  env_vars?: Record<string, string> // Environment variables for stdio transport
  is_builtin?: boolean // Whether this is a builtin MCP service
  // Per-field "configured?" map embedded on the main response (server-side
  // dto.MCPServiceResponse.Credentials). Drives the CredentialResource card
  // without a follow-up GET. Absent for builtin services.
  credentials?: Record<McpCredentialField, CredentialFieldMetadata>
  created_at?: string
  updated_at?: string
}

export interface MCPTool {
  name: string
  description: string
  inputSchema: Record<string, any>
  require_approval?: boolean
}

export interface MCPToolApprovalRow {
  id: string
  tenant_id?: number
  service_id: string
  tool_name: string
  require_approval: boolean
}

export interface MCPResource {
  uri: string
  name: string
  description?: string
  mimeType?: string
}

export interface MCPTestResult {
  success: boolean
  message?: string
  description?: string
  // Set when the server requires OAuth (RFC 9728) but the service was not
  // configured for it — the UI guides the user to switch to OAuth 2.0.
  oauth_required?: boolean
  tools?: MCPTool[]
  resources?: MCPResource[]
}

// List all MCP services
export async function listMCPServices(): Promise<MCPService[]> {
  const response: any = await get('/api/v1/mcp-services')
  return response.data || []
}

// ----------------------------------------------------------------------------
// 管理面（000096 平台化）：MCP 服务由系统管理员在平台目录配置一次，再按
// 空间分配。全部走 /system/admin 镜像路由，仅系统管理员可用。
//
// 空间读路径（聊天/智能体解析已分配 + builtin 服务）保持上面的
// /api/v1/mcp-services 端点不动。
// ----------------------------------------------------------------------------

// Workspace assignment of a platform MCP service (000096 platform rework).
// Unlike web search there is no default flag — MCP has no per-workspace
// default semantics.
export interface MCPServiceTenantAssignment {
  tenant_id: number
  tenant_name: string
  service_id?: string
  assigned_at?: string
}

// Platform catalog row as returned by the /system/admin endpoints — the
// tenant-scoped entity shape plus the service's workspace assignments.
// Builtin services carry no assignments: they are visible to every workspace.
export interface SystemMCPService extends MCPService {
  assignments?: MCPServiceTenantAssignment[]
}

// List the whole platform MCP catalog (admin console)
export async function listSystemMCPServices(): Promise<SystemMCPService[]> {
  const response: any = await get('/api/v1/system/admin/mcp-services')
  if (response && Array.isArray(response.data)) {
    return response.data
  }
  return []
}

// Get a single platform MCP service by ID (with assignments)
export async function getSystemMCPService(id: string): Promise<SystemMCPService> {
  const response: any = await get(`/api/v1/system/admin/mcp-services/${id}`)
  return response.data
}

// Create a new platform MCP service (starts unassigned)
export async function createSystemMCPService(data: Partial<MCPService>): Promise<SystemMCPService> {
  const response: any = await post('/api/v1/system/admin/mcp-services', data)
  return response.data
}

// Update an existing platform MCP service
export async function updateSystemMCPService(id: string, data: Partial<MCPService>): Promise<SystemMCPService> {
  const response: any = await put(`/api/v1/system/admin/mcp-services/${id}`, data)
  return response.data
}

// Delete a platform MCP service (its assignments are purged server-side)
export async function deleteSystemMCPService(id: string): Promise<void> {
  await del(`/api/v1/system/admin/mcp-services/${id}`)
}

// Test a saved platform MCP service connection (OAuth services will surface an
// "authorization required" result — per-user OAuth needs a workspace context)
export async function testSystemMCPService(id: string): Promise<MCPTestResult> {
  const response: any = await post(`/api/v1/system/admin/mcp-services/${id}/test`, {})
  if (response && response.data) {
    return response.data
  }
  return response
}

// Get a single MCP service by ID
export async function getMCPService(id: string): Promise<MCPService> {
  const response: any = await get(`/api/v1/mcp-services/${id}`)
  return response.data
}

// Create a new MCP service
export async function createMCPService(data: Partial<MCPService>): Promise<MCPService> {
  const response: any = await post('/api/v1/mcp-services', data)
  return response.data
}

// Update an existing MCP service
export async function updateMCPService(id: string, data: Partial<MCPService>): Promise<MCPService> {
  const response: any = await put(`/api/v1/mcp-services/${id}`, data)
  return response.data
}

// Delete an MCP service
export async function deleteMCPService(id: string): Promise<void> {
  await del(`/api/v1/mcp-services/${id}`)
}

// Test MCP service connection
export async function testMCPService(id: string): Promise<MCPTestResult> {
  const response: any = await post(`/api/v1/mcp-services/${id}/test`, {})
  // 后端返回格式: { success: true, data: MCPTestResult }
  // response interceptor 已经返回了 data，所以 response 就是 { success: true, data: {...} }
  if (response && response.data) {
    return response.data
  }
  // 如果格式不对，尝试直接返回 response（可能是直接返回的数据）
  return response
}

// Get tools from an MCP service
export async function getMCPServiceTools(id: string): Promise<MCPTool[]> {
  const response: any = await get(`/api/v1/mcp-services/${id}/tools`)
  return response.data || []
}

// Get resources from an MCP service
export async function getMCPServiceResources(id: string): Promise<MCPResource[]> {
  const response: any = await get(`/api/v1/mcp-services/${id}/resources`)
  return response.data || []
}

/** Persisted per-tool human-approval flags (issue #1173) */
export async function getMCPToolApprovals(serviceId: string): Promise<MCPToolApprovalRow[]> {
  const response: any = await get(`/api/v1/mcp-services/${serviceId}/tool-approvals`)
  return response.data || []
}

export async function setMCPToolApproval(serviceId: string, toolName: string, requireApproval: boolean): Promise<void> {
  await put(`/api/v1/mcp-services/${serviceId}/tool-approvals/${encodeURIComponent(toolName)}`, {
    require_approval: requireApproval
  })
}

// ----------------------------------------------------------------------------
// Credential subresource (issue #988 follow-up).
//
// Secrets travel through a dedicated /credentials endpoint instead of the
// main MCP PUT body. "Is this configured?" metadata is embedded on the main
// MCPService response (MCPService.credentials), so there is no GET on this
// endpoint — only PUT (write) and DELETE (clear). Both trigger an MCP
// client reconnect server-side.
// ----------------------------------------------------------------------------

export type McpCredentialField = 'api_key' | 'token'

export interface CredentialFieldMetadata {
  configured: boolean
}

export interface McpCredentialsResponse {
  fields: Record<McpCredentialField, CredentialFieldMetadata>
}

export async function putMCPCredentials(
  serviceId: string,
  body: Partial<Record<McpCredentialField, string>>
): Promise<McpCredentialsResponse> {
  const response: any = await put(`/api/v1/mcp-services/${serviceId}/credentials`, body)
  return (response.data ?? response) as McpCredentialsResponse
}

export async function deleteMCPCredentialField(
  serviceId: string,
  field: McpCredentialField
): Promise<void> {
  await del(`/api/v1/mcp-services/${serviceId}/credentials/${field}`)
}

// ---- Credential subresource, platform-catalog copy (admin console). Same
// contract; the handler treats the missing workspace context as platform
// scope. ----

export async function putSystemMCPCredentials(
  serviceId: string,
  body: Partial<Record<McpCredentialField, string>>
): Promise<McpCredentialsResponse> {
  const response: any = await put(`/api/v1/system/admin/mcp-services/${serviceId}/credentials`, body)
  return (response.data ?? response) as McpCredentialsResponse
}

export async function deleteSystemMCPCredentialField(
  serviceId: string,
  field: McpCredentialField
): Promise<void> {
  await del(`/api/v1/system/admin/mcp-services/${serviceId}/credentials/${field}`)
}

// ---- Platform-default tool-approval policy (admin console). Writes the
// tenant_id=0 rows; runtime resolution prefers a workspace override and falls
// back to these. ----

export async function getSystemMCPToolApprovals(serviceId: string): Promise<MCPToolApprovalRow[]> {
  const response: any = await get(`/api/v1/system/admin/mcp-services/${serviceId}/tool-approvals`)
  return response.data || []
}

export async function setSystemMCPToolApproval(serviceId: string, toolName: string, requireApproval: boolean): Promise<void> {
  await put(`/api/v1/system/admin/mcp-services/${serviceId}/tool-approvals/${encodeURIComponent(toolName)}`, {
    require_approval: requireApproval
  })
}

// ----------------------------------------------------------------------------
// Workspace assignments of a platform MCP service (admin console drawer).
// ----------------------------------------------------------------------------

export function listMCPTenantAssignments(id: string): Promise<MCPServiceTenantAssignment[]> {
  return get(`/api/v1/system/admin/mcp-services/${id}/tenant-assignments`).then((res: any) => {
    if (res.success && Array.isArray(res.data)) {
      return res.data
    }
    return []
  })
}

// Replace the full assignment list of a service. rows carries the target
// state; a workspace missing from the list is unassigned. Builtin services
// reject assignment management (visible to every workspace by design).
export function updateMCPTenantAssignments(
  id: string,
  rows: Array<{ tenant_id: number }>,
) {
  return put(`/api/v1/system/admin/mcp-services/${id}/tenant-assignments`, { assignments: rows })
}

// ----------------------------------------------------------------------------
// Per-user OAuth2 authorization-code flow.
//
// The user authorizes a service once; the backend stores their access/refresh
// token (per tenant + user + service) and refreshes it transparently. The
// callback is a public backend route that the third-party authorization
// server redirects to.
// ----------------------------------------------------------------------------

// Path of the public backend OAuth callback (registered outside /mcp-services
// to avoid a route conflict, and allow-listed for no-auth in the backend).
export const MCP_OAUTH_CALLBACK_PATH = '/api/v1/mcp-oauth/callback'

export interface MCPOAuthAuthorization {
  authorizationUrl: string
  authorizationAttempt: string
}

export type MCPOAuthTokenState = 'authorized' | 'refreshable' | 'reauth_required' | 'pending'

export interface MCPOAuthStatus {
  authorized: boolean
  state: MCPOAuthTokenState
  refresh_available: boolean
  expires_at?: string
}

// Begin authorization for the current user. The attempt id binds polling to
// this popup, so an older stored token cannot be mistaken for fresh consent.
export async function getMCPOAuthAuthorizeURL(
  serviceId: string,
  body: { redirect_uri: string; frontend_redirect?: string }
): Promise<MCPOAuthAuthorization> {
  const response: any = await post(`/api/v1/mcp-services/${serviceId}/oauth/authorize-url`, body)
  const data = response.data ?? response
  return {
    authorizationUrl: data?.authorization_url ?? '',
    authorizationAttempt: data?.authorization_attempt ?? '',
  }
}

// Whether the current user has authorized this service.
export async function getMCPOAuthStatus(
  serviceId: string,
  authorizationAttempt?: string,
): Promise<boolean> {
  const query = authorizationAttempt
    ? `?authorization_attempt=${encodeURIComponent(authorizationAttempt)}`
    : ''
  const response: any = await get(`/api/v1/mcp-services/${serviceId}/oauth/status${query}`)
  return Boolean((response.data ?? response)?.authorized)
}

// Full lifecycle status for management surfaces. Expired access tokens with a
// refresh token are "refreshable", not falsely presented as already usable.
export async function getMCPOAuthAuthorizationStatus(serviceId: string): Promise<MCPOAuthStatus> {
  const response: any = await get(`/api/v1/mcp-services/${serviceId}/oauth/status`)
  const data = response.data ?? response
  return {
    authorized: Boolean(data?.authorized),
    state: data?.state ?? 'reauth_required',
    refresh_available: Boolean(data?.refresh_available),
    expires_at: data?.expires_at,
  }
}

// Revoke the current user's token (forces re-authorization).
export async function revokeMCPOAuthToken(serviceId: string): Promise<void> {
  await del(`/api/v1/mcp-services/${serviceId}/oauth/token`)
}

export async function resolveToolApproval(
  pendingId: string,
  body: { decision: 'approve' | 'reject'; modified_args?: Record<string, unknown>; reason?: string }
): Promise<void> {
  await post(`/api/v1/agent/tool-approvals/${encodeURIComponent(pendingId)}`, body)
}

// Resume an agent run that paused on an in-conversation MCP OAuth prompt.
// Call after the per-user authorization popup completes; the backend verifies
// the token exists before unblocking the paused tool call.
export async function resolveMCPOAuth(
  pendingId: string,
  body: { service_id: string; decision?: 'authorize' | 'cancel' }
): Promise<void> {
  await post(`/api/v1/agent/mcp-oauth-resolutions/${encodeURIComponent(pendingId)}`, body)
}

export async function cancelMCPOAuth(pendingId: string): Promise<void> {
  await post(`/api/v1/agent/mcp-oauth-resolutions/${encodeURIComponent(pendingId)}/cancel`, {})
}
