import { defineStore } from 'pinia'

// 系统管理后台「按空间代管」状态：系统管理员在控制台的 7 个代管面板
// （向量库/解析引擎/存储/沙箱/技能/网络搜索/MCP）中选中的目标工作空间。
// 选中期间所有请求的 X-Tenant-ID 都指向该空间（见 resolveRequestTenantId），
// 离开 /system/console 时由路由守卫 + SystemConsole onUnmounted 清除，
// 防止代管租户头泄漏到系统管理员自己的空间请求里。
export const useManagedWorkspaceStore = defineStore('managedWorkspace', {
  state: () => ({
    managedTenantId: null as string | null,
    managedTenantName: null as string | null,
  }),
  getters: {
    isManaging: (state) => state.managedTenantId != null,
  },
  actions: {
    setManaged(tenantId: string | number, tenantName?: string) {
      this.managedTenantId = String(tenantId)
      this.managedTenantName = tenantName ?? null
    },
    clearManaged() {
      this.managedTenantId = null
      this.managedTenantName = null
    },
  },
})

// 请求最终生效的空间 ID：控制台代管值优先，其次用户自己选择的空间。
// 供 axios 拦截器与三处 SSE（fetchEventSource）消费方共用，保证两条
// 请求路径的租户解析永远一致。pinia 未激活（单测环境）时退回后者。
export function resolveRequestTenantId(): string | null {
  try {
    const managed = useManagedWorkspaceStore().managedTenantId
    if (managed) return managed
  } catch {
    // pinia 未安装（单元测试直接调拦截器）——退回当前用户选择的空间。
  }
  return localStorage.getItem('weknora_selected_tenant_id')
}
