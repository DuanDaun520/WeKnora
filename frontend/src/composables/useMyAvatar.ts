/**
 * 当前登录用户的头像 objectURL（单例缓存）。
 *
 * users.avatar 存的是存储 ref（resource://...），<img> 不能直接渲染；
 * 专用端点 /auth/me/avatar 按 Bearer 鉴权流式返回字节，所以这里用
 * blob 拉取 + createObjectURL。缓存挂在 window 上（与
 * protectedFileAccess 同款）：Vite 热更新会替换模块但不会重建文档，
 * 模块级变量会丢掉已生成的 objectURL。
 */
import { ref, watch } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { fetchMyAvatarBlob } from '@/api/auth'

interface MyAvatarState {
  /** 当前已解析的 avatar ref；'' = 无头像。 */
  ref: string
  /** ref 对应的 objectURL；null = 无头像或加载失败（回落灰色默认）。 */
  url: string | null
}

const freshState = (): MyAvatarState => ({ ref: '', url: null })

const state: MyAvatarState = (() => {
  if (typeof window === 'undefined') return freshState()
  const scope = window as typeof window & { __weknoraMyAvatarV1__?: MyAvatarState }
  scope.__weknoraMyAvatarV1__ ||= freshState()
  return scope.__weknoraMyAvatarV1__
})()

// 响应式镜像：state 本体挂在 window（防 HMR 丢缓存），变化经 ref 广播
// 给已挂载的 UserAvatar 消费方。
const avatarUrl = ref<string | null>(state.url)
const avatarRef = ref<string>(state.ref)
const loading = ref(false)

// 串行化令牌：等待期间又换头像（上传两连发）时，旧请求的结果按
// 「完成时 state.ref 是否已被新请求写掉」判断丢弃。
let requestSeq = 0

async function applyRef(nextRef: string): Promise<void> {
  const trimmed = nextRef.trim()
  if (!trimmed) {
    if (state.url) URL.revokeObjectURL(state.url)
    state.ref = ''
    state.url = null
    avatarRef.value = ''
    avatarUrl.value = null
    return
  }
  if (trimmed === state.ref) {
    // 同一 ref 复用缓存；失败的加载在换 ref 前不重试（404 也走这里，
    // 避免每次组件挂载都打一次注定失败的请求）。
    return
  }
  const seq = ++requestSeq
  loading.value = true
  try {
    const blob = await fetchMyAvatarBlob(trimmed)
    if (seq !== requestSeq) return // 期间 ref 又变了，丢弃过期结果
    const nextUrl = URL.createObjectURL(blob)
    if (state.url) URL.revokeObjectURL(state.url)
    state.ref = trimmed
    state.url = nextUrl
    avatarRef.value = trimmed
    avatarUrl.value = nextUrl
  } catch {
    // 404（无头像）/网络失败 → 灰色默认；不打扰用户。
    if (seq !== requestSeq) return
    avatarRef.value = trimmed
    avatarUrl.value = null
  } finally {
    if (seq === requestSeq) loading.value = false
  }
}

export function useMyAvatar() {
  const authStore = useAuthStore()

  // 上传/移除后 authStore.refreshFromAuthMe() 更新 user.avatar，
  // watch 在这里触发重新拉取 —— UserMenu 与设置页随之响应式更新。
  watch(
    () => authStore.user?.avatar || '',
    (next) => { void applyRef(next) },
    { immediate: true },
  )

  return {
    avatarUrl,
    avatarRef,
    loading,
    /** 显式刷新钩子（正常流程由 watch 覆盖，一般无需调用）。 */
    refresh: () => applyRef(authStore.user?.avatar || ''),
  }
}

/** 测试专用：清空单例缓存。 */
export function resetMyAvatarCacheForTest(): void {
  if (state.url) URL.revokeObjectURL(state.url)
  Object.assign(state, freshState())
  avatarUrl.value = null
  avatarRef.value = ''
  loading.value = false
  requestSeq++
}
