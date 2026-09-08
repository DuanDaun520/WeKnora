import { get, post, put, del } from '@/utils/request'

export interface PlatformStorageEngine {
  id: string
  name: string
  provider: string
  config: {
    mode?: string
    endpoint?: string
    region?: string
    access_key_id?: string
    secret_access_key?: string
    bucket_name?: string
    path_prefix?: string
    app_id?: string
    use_ssl?: boolean
    force_path_style?: boolean
    use_temp_bucket?: boolean
    temp_bucket_name?: string
    temp_region?: string
  }
  status: 'active' | 'disabled'
  created_at?: string
  updated_at?: string
}

export interface PlatformStorageEngineListResponse {
  success: boolean
  data: PlatformStorageEngine[]
}

export const listPlatformStorageEngines = (): Promise<PlatformStorageEngineListResponse> =>
  get('/api/v1/platform/storage-engines')

export const getPlatformStorageEngine = (id: string): Promise<{ success: boolean; data: PlatformStorageEngine; tenant_count?: number }> =>
  get(`/api/v1/platform/storage-engines/${id}`)

export const createPlatformStorageEngine = (data: Partial<PlatformStorageEngine>): Promise<{ success: boolean; data: PlatformStorageEngine }> =>
  post('/api/v1/platform/storage-engines', data)

export const updatePlatformStorageEngine = (id: string, data: Partial<PlatformStorageEngine>): Promise<{ success: boolean; data: PlatformStorageEngine }> =>
  put(`/api/v1/platform/storage-engines/${id}`, data)

export const deletePlatformStorageEngine = (id: string): Promise<{ success: boolean }> =>
  del(`/api/v1/platform/storage-engines/${id}`)

export const testPlatformStorageEngine = (data: Partial<PlatformStorageEngine>): Promise<{ success: boolean; error?: string }> =>
  post('/api/v1/platform/storage-engines/test', data)

export const testPlatformStorageEngineByID = (id: string): Promise<{ success: boolean; error?: string }> =>
  post(`/api/v1/platform/storage-engines/${id}/test`, {})

export const listPlatformStorageEngineTypes = (): Promise<{ success: boolean; data: string[] }> =>
  get('/api/v1/platform/storage-engines/types')

// Tenant storage engine assignment APIs
export const assignStorageEngineToTenant = (tenantId: number, engineId: string): Promise<{ success: boolean }> =>
  put(`/api/v1/tenants/${tenantId}/storage-engine`, { platform_storage_engine_id: engineId })

export const unassignStorageEngineFromTenant = (tenantId: number): Promise<{ success: boolean }> =>
  del(`/api/v1/tenants/${tenantId}/storage-engine`)