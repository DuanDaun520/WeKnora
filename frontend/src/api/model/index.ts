import { get, post, postUpload, put, del } from '../../utils/request';
import i18n from '@/i18n'

const t = (key: string) => i18n.global.t(key)

// 模型类型定义
export interface ModelConfig {
  id?: string;
  tenant_id?: number;
  name: string;
  display_name?: string;
  type: 'KnowledgeQA' | 'Embedding' | 'Rerank' | 'VLLM' | 'ASR';
  source: 'local' | 'remote';
  description?: string;
  parameters: {
    base_url?: string;
    api_key?: string;
    provider?: string; // Provider identifier: openai, aliyun, zhipu, generic
    embedding_parameters?: {
      dimension?: number;
      truncate_prompt_tokens?: number;
      supports_dimension_override?: boolean;
    };
    interface_type?: 'ollama' | 'openai'; // VLLM专用
    parameter_size?: string; // Ollama模型参数大小 (e.g., "7B", "13B", "70B")
    extra_config?: Record<string, string>; // Provider-specific configuration
    // 自定义 HTTP 请求头（类似 Python OpenAI SDK 的 extra_headers），
    // 会在调用远程模型 API 时附加到每个请求上。Authorization、Content-Type 等保留头会被忽略。
    custom_headers?: Record<string, string>;
    supports_vision?: boolean; // Whether the model accepts image/multimodal input
    // 后台任务（入库/富化）对该模型的并发上限，按模型 ID 全副本共享。
    // 0 或不填表示沿用全局默认（model.max_concurrency）；仅对 chat/embedding/vllm 生效。
    max_concurrency?: number;
    app_id?: string;
    // Secret fields (api_key, app_secret) are never returned by the server in
    // this shape — they live behind the /credentials subresource. They are
    // kept on the type so create-mode payloads can still carry them in the
    // initial POST body.
    app_secret?: string;
  };
  is_default?: boolean;
  is_builtin?: boolean;
  status?: string;
  // Per-field configured? metadata from the main response. For builtin
  // models it is returned only to system administrators.
  credentials?: Record<ModelCredentialField, { configured: boolean }>;
  created_at?: string;
  updated_at?: string;
  deleted_at?: string | null;
}

// ---- 模型目录管理（000094 收权）：写操作全部走 /system/admin 镜像路由，
// 仅系统管理员可用（无空间绑定的系统管理员同样可调）。读接口
// listModels 仍走空间级 /api/v1/models，供各处模型选择器使用。 ----

// 创建模型（平台目录）
export function createModel(data: ModelConfig): Promise<ModelConfig> {
  return new Promise((resolve, reject) => {
    post('/api/v1/system/admin/models', data)
      .then((response: any) => {
        if (response && response.model) {
          resolve(response.model);
        } else {
          reject(new Error(response.message || t('error.model.createFailed')));
        }
      })
      .catch((error: any) => {
        console.error('Failed to create model:', error);
        reject(error);
      });
  });
}

// 平台模型目录全量列表（含内置模型），供系统管理控制台使用
export async function listSystemModels(params?: { type?: string; q?: string }): Promise<ModelConfig[]> {
  const qs = new URLSearchParams()
  if (params?.type) qs.set('type', params.type)
  if (params?.q) qs.set('q', params.q)
  const suffix = qs.toString() ? `?${qs.toString()}` : ''
  const response: any = await get(`/api/v1/system/admin/models${suffix}`)
  return (response && response.models) || []
}

// 获取模型列表
export function listModels(type?: string): Promise<ModelConfig[]> {
  return new Promise((resolve, reject) => {
    const url = `/api/v1/models`;
    get(url)
      .then((response: any) => {
        if (response.success && response.data) {
          if (type) {
            response.data = response.data.filter((item: ModelConfig) => item.type === type);
          }
          resolve(response.data);
        } else {
          resolve([]);
        }
      })
      .catch((error: any) => {
        console.error('Failed to list models:', error);
        // 抛出而非吞掉：调用方（含缓存层）才能区分「真失败」与「成功但无模型」，
        // 避免把一次瞬时失败的空结果缓存下来。各 UI 调用点均已 try/catch 兜底。
        reject(error);
      });
  });
}

// 获取单个模型（平台目录）
export function getModel(id: string): Promise<ModelConfig> {
  return new Promise((resolve, reject) => {
    get(`/api/v1/system/admin/models/${id}`)
      .then((response: any) => {
        if (response && response.model) {
          resolve(response.model);
        } else {
          reject(new Error(response.message || t('error.model.getFailed')));
        }
      })
      .catch((error: any) => {
        console.error('Failed to get model:', error);
        reject(error);
      });
  });
}

// 更新模型（平台目录）
export function updateModel(id: string, data: Partial<ModelConfig>): Promise<ModelConfig> {
  return new Promise((resolve, reject) => {
    put(`/api/v1/system/admin/models/${id}`, data)
      .then((response: any) => {
        if (response && response.model) {
          resolve(response.model);
        } else {
          reject(new Error(response.message || t('error.model.updateFailed')));
        }
      })
      .catch((error: any) => {
        console.error('Failed to update model:', error);
        reject(error);
      });
  });
}

// 删除模型（平台目录）
export function deleteModel(id: string): Promise<void> {
  return new Promise((resolve, reject) => {
    del(`/api/v1/system/admin/models/${id}`)
      .then((response: any) => {
        if (response.success) {
          resolve();
        } else {
          reject(new Error(response.message || t('error.model.deleteFailed')));
        }
      })
      .catch((error: any) => {
        console.error('Failed to delete model:', error);
        reject(error);
      });
  });
}

export interface ModelDebugOptions {
  system_prompt?: string
  temperature?: number
  top_p?: number
  max_tokens?: number
  thinking?: boolean
}

export interface ModelDebugResult {
  ok: boolean
  elapsed_ms: number
  request: Record<string, unknown>
  raw_response: unknown
  observations: Record<string, unknown>
  error?: string
}

export async function debugModel(
  id: string,
  data: {
    input?: string
    documents?: string[]
    options?: ModelDebugOptions
    file?: File | null
  },
): Promise<ModelDebugResult> {
  const form = new FormData()
  form.append('input', data.input || '')
  form.append('documents', JSON.stringify(data.documents || []))
  form.append('options', JSON.stringify(data.options || {}))
  if (data.file) form.append('file', data.file)
  const response: any = await postUpload(
    `/api/v1/system/admin/models/${id}/debug`,
    form,
    undefined,
    { timeout: 300000 },
  )
  if (response?.success && response?.data) return response.data
  throw new Error(response?.message || t('error.model.getFailed'))
}

// ----------------------------------------------------------------------------
// Model credential subresource. See mcp-service.ts for the matching MCP API
// shape and the design notes in internal/handler/dto/mcp.go.
// ----------------------------------------------------------------------------

export type ModelCredentialField = 'api_key' | 'app_secret'

export interface ModelCredentialsResponse {
  fields: Record<ModelCredentialField, { configured: boolean }>
}

export async function putModelCredentials(
  id: string,
  body: Partial<Record<ModelCredentialField, string>>,
): Promise<ModelCredentialsResponse> {
  const response: any = await put(`/api/v1/system/admin/models/${id}/credentials`, body)
  return (response.data ?? response) as ModelCredentialsResponse
}

export async function deleteModelCredentialField(
  id: string,
  field: ModelCredentialField,
): Promise<void> {
  await del(`/api/v1/system/admin/models/${id}/credentials/${field}`)
}

// ---- WeKnoraCloud 空间级凭证 ----
// 模型凭证已随 000094 平台化改为每模型 app_id / app_secret（见上方
// credentials 子资源）；这对接口操作的是空间级 tenants.credentials，
// 目前唯一的消费方是文档解析引擎（ParserEngineSettings）。写路由仅
// 系统管理员可调。
export interface WeKnoraCloudTenantCredentials {
  app_id: string;
  app_secret: string;
}

export interface WeKnoraCloudStatus {
  has_models: boolean;
  needs_reinit: boolean;
  reason?: string;
}

export async function saveWeKnoraCloudCredentials(
  data: WeKnoraCloudTenantCredentials,
): Promise<void> {
  await post('/api/v1/weknoracloud/credentials', data)
}

export async function getWeKnoraCloudStatus(): Promise<WeKnoraCloudStatus> {
  const response: any = await get('/api/v1/models/weknoracloud/status')
  return (response?.data ?? response) as WeKnoraCloudStatus
}
