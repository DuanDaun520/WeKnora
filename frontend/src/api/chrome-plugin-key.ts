import { get, post, del } from '@/utils/request'

export interface ChromePluginKey {
  id: number
  name: string
  api_key: string
  knowledge_base_ids: string[]
  capabilities: string[]
  created_at: string
}

export interface ChromePluginKeyCreateResponse {
  id: number
  name: string
  api_key: string // Full key only returned on creation
  knowledge_base_ids: string[]
  capabilities: string[]
  created_at: string
}

// Get the current user's Chrome plugin key (if exists)
export async function getChromePluginKey(): Promise<{ success: boolean; data: ChromePluginKey | null }> {
  const response = await get('/api/v1/me/chrome-plugin-key')
  return response as { success: boolean; data: ChromePluginKey | null }
}

// Generate a new Chrome plugin key
export async function generateChromePluginKey(): Promise<{ success: boolean; data: ChromePluginKeyCreateResponse }> {
  const response = await post('/api/v1/me/chrome-plugin-key', {})
  return response as { success: boolean; data: ChromePluginKeyCreateResponse }
}

// Delete the current user's Chrome plugin key
export async function deleteChromePluginKey(): Promise<{ success: boolean }> {
  const response = await del('/api/v1/me/chrome-plugin-key')
  return response as { success: boolean }
}