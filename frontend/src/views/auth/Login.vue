<template>
  <div class="login-layout">
    <div class="animated-bg">
      <div class="knowledge-node node-1">
        <svg class="node-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20" />
          <path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z" />
        </svg>
      </div>
      <div class="knowledge-node node-2">
        <svg class="node-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />
        </svg>
      </div>
      <div class="knowledge-node node-3">
        <svg class="node-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M12 2L2 7l10 5 10-5-10-5z" />
          <path d="M2 17l10 5 10-5" />
          <path d="M2 12l10 5 10-5" />
        </svg>
      </div>
      <div class="knowledge-node node-4">
        <svg class="node-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <ellipse cx="12" cy="5" rx="9" ry="3" />
          <path d="M21 12c0 1.66-4 3-9 3s-9-1.34-9-3" />
          <path d="M3 5v14c0 1.66 4 3 9 3s9-1.66 9-3V5" />
        </svg>
      </div>
      <div class="knowledge-node node-5">
        <svg class="node-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="11" cy="11" r="8" />
          <path d="m21 21-4.35-4.35" />
        </svg>
      </div>
      <div class="knowledge-node node-6">
        <svg class="node-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path
            d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z" />
          <polyline points="3.27 6.96 12 12.01 20.73 6.96" />
          <line x1="12" y1="22.08" x2="12" y2="12" />
        </svg>
      </div>
      <div class="knowledge-node node-7">
        <svg class="node-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
          <polyline points="14 2 14 8 20 8" />
        </svg>
      </div>
      <div class="knowledge-node node-8">
        <svg class="node-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
          <circle cx="9" cy="7" r="4" />
          <path d="M23 21v-2a4 4 0 0 0-3-3.87" />
          <path d="M16 3.13a4 4 0 0 1 0 7.75" />
        </svg>
      </div>
      <div class="knowledge-node node-9">
        <svg class="node-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
        </svg>
      </div>
      <div class="knowledge-node node-10">
        <svg class="node-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="3" />
          <path
            d="M12 1v6m0 6v6M5.64 5.64l4.24 4.24m4.24 4.24l4.24 4.24M1 12h6m6 0h6M5.64 18.36l4.24-4.24m4.24-4.24l4.24-4.24" />
        </svg>
      </div>
      <div class="knowledge-node node-11">
        <svg class="node-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M9 11l3 3L22 4" />
          <path d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11" />
        </svg>
      </div>
      <div class="knowledge-node node-12">
        <svg class="node-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <polygon
            points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />
        </svg>
      </div>

      <svg class="knowledge-lines" viewBox="0 0 100 100" preserveAspectRatio="none">
        <line class="connection-line line-1" x1="20" y1="15" x2="35" y2="25" />
        <line class="connection-line line-2" x1="35" y1="25" x2="55" y2="20" />
        <line class="connection-line line-3" x1="55" y1="20" x2="85" y2="12" />
        <line class="connection-line line-4" x1="8" y1="35" x2="25" y2="45" />
        <line class="connection-line line-5" x1="25" y1="45" x2="65" y2="48" />
        <line class="connection-line line-6" x1="20" y1="60" x2="60" y2="75" />
        <line class="connection-line line-7" x1="20" y1="15" x2="20" y2="60" />
        <line class="connection-line line-8" x1="55" y1="20" x2="45" y2="50" />
        <line class="connection-line line-9" x1="65" y1="48" x2="90" y2="38" />
        <line class="connection-line line-10" x1="40" y1="70" x2="75" y2="80" />
        <line class="connection-line line-11" x1="35" y1="25" x2="25" y2="45" />
        <line class="connection-line line-12" x1="75" y1="30" x2="65" y2="48" />
      </svg>
    </div>

    <!-- Centered Form Section -->
    <div class="form-section">
      <div class="form-panel">
        <!-- 强制改密卡片：管理员开户/重置的密码（MustChangePassword=true）首登必须轮换 -->
        <div class="form-card" v-if="mustChangeMode">
          <div class="form-header">
            <h2 class="form-title">{{ $t('auth.mustChangePassword.title') }}</h2>
            <p class="form-subtitle">{{ $t('auth.mustChangePassword.description') }}</p>
          </div>

          <div class="form-content">
            <t-form ref="mustChangeFormRef" :data="mustChangeForm" :rules="mustChangeRules"
              @submit="handleMustChangeSubmit" layout="vertical" label-align="top">
              <t-form-item :label="$t('userProfile.changePassword.currentLabel')" name="oldPassword">
                <t-input v-model="mustChangeForm.oldPassword" :placeholder="$t('userProfile.changePassword.currentPlaceholder')"
                  type="password" autocomplete="new-password" size="large" :disabled="mustChangeLoading" />
              </t-form-item>

              <t-form-item :label="$t('userProfile.changePassword.newLabel')" name="newPassword">
                <t-input v-model="mustChangeForm.newPassword" :placeholder="$t('userProfile.changePassword.newPlaceholder')"
                  type="password" autocomplete="new-password" size="large" :disabled="mustChangeLoading" />
              </t-form-item>

              <t-form-item :label="$t('userProfile.changePassword.confirmLabel')" name="confirmPassword">
                <t-input v-model="mustChangeForm.confirmPassword" :placeholder="$t('userProfile.changePassword.confirmPlaceholder')"
                  type="password" autocomplete="new-password" size="large" :disabled="mustChangeLoading"
                  @enter="handleMustChangeSubmit" />
              </t-form-item>

              <t-button type="submit" theme="primary" size="large" block :loading="mustChangeLoading" class="submit-button">
                {{ $t('auth.mustChangePassword.submit') }}
              </t-button>
            </t-form>

            <div class="form-footer">
              <a href="#" @click.prevent="exitMustChangeMode" class="link-button">
                {{ $t('auth.mustChangePassword.backToLogin') }}
              </a>
            </div>
          </div>
        </div>

        <!-- Login Card -->
        <div class="form-card" v-if="!mustChangeMode">
          <div class="form-header">
            <img class="brand-logo" :src="logoUrl" alt="">
            <h2 class="form-title">{{ $t('platform.brandName') }}</h2>
            <p class="form-welcome">{{ $t('platform.subtitle') }}</p>
          </div>

          <div class="form-content">
            <t-form ref="formRef" :data="formData" :rules="formRules" @submit="handleLogin" layout="vertical"
              label-align="top">
              <t-form-item :label="$t('auth.employeeId')" name="email">
                <t-auto-complete v-model="formData.email" :options="loginIdOptions"
                  :placeholder="$t('auth.employeeIdPlaceholder')" filterable clearable size="large" :disabled="loading"
                  autocomplete="off" name="zhishu-login-id" />
              </t-form-item>

              <t-form-item :label="$t('auth.password')" name="password">
                <t-input v-model="formData.password" :placeholder="$t('auth.passwordPlaceholder')" type="password"
                  autocomplete="new-password" size="large" :disabled="loading" @enter="handleLogin" />
              </t-form-item>

              <t-button type="submit" theme="primary" size="large" block :loading="loading" class="submit-button">
                {{ loading ? $t('auth.loggingIn') : $t('auth.login') }}
              </t-button>
            </t-form>

            <p class="test-mode-note">{{ $t('auth.testModeNote') }}</p>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, nextTick, onMounted, computed } from 'vue'
import logoUrl from '@/assets/img/logo.png'
import { useRoute, useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { useRoleLabel } from '@/composables/useRoleLabel'
import { notifyLoginSuccess } from '@/utils/loginNotify'
import { newPasswordRules } from '@/utils/passwordPolicy'
import {
  login,
  changePassword,
  userInfoFromApi,
} from '@/api/auth'
import { useAuthStore } from '@/stores/auth'
import { useI18n } from 'vue-i18n'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()
const { t, tm } = useI18n()
const { formatRole, roleIcon } = useRoleLabel()

// Form references
const formRef = ref()

// State management
const loading = ref(false)
const complexPasswordEnabled = ref(false)

// 本系统专属的登录历史：只记录登录成功的工号（localStorage，最多 5 个），
// 用 t-auto-complete 下拉展示，替代浏览器按域名共享的原生自动填充——
// 同域名下跑多个系统时原生建议会混入其他账号。
const LOGIN_HISTORY_KEY = 'zhishu_login_ids'
const savedLoginIds = ref<string[]>([])
const loginIdOptions = computed(() => savedLoginIds.value.map(id => ({ label: id, value: id })))

const loadLoginHistory = () => {
  try {
    const raw = localStorage.getItem(LOGIN_HISTORY_KEY)
    const list = raw ? JSON.parse(raw) : []
    savedLoginIds.value = Array.isArray(list)
      ? list.filter((x: unknown): x is string => typeof x === 'string').slice(0, 5)
      : []
  } catch {
    savedLoginIds.value = []
  }
}

const rememberLoginId = (id: string) => {
  const v = id.trim()
  if (!v) return
  savedLoginIds.value = [v, ...savedLoginIds.value.filter(x => x !== v)].slice(0, 5)
  localStorage.setItem(LOGIN_HISTORY_KEY, JSON.stringify(savedLoginIds.value))
}

// 后端登录失败固定返回英文模糊文案（防止探测区分账号不存在/密码错误），
// 展示层映射为中文；未知文案原样透出。
const localizeAuthError = (msg?: string) => {
  if (!msg) return t('auth.loginErrorRetry')
  if (/invalid employee id/i.test(msg)) return t('auth.loginErrorEmployeeId')
  if (/account is disabled/i.test(msg)) return t('auth.accountDisabled')
  return msg
}

// 强制改密状态。登录响应带 must_change_password=true（管理员开户/重置
// 密码）或存量会话被 403 PASSWORD_CHANGE_REQUIRED 拦回登录页时切入该
// 卡片：改密成功后后端吊销全部 token，本地会话整体清空并回到登录卡片
// 用新密码重新进入。
const mustChangeMode = ref(false)
const mustChangeFormRef = ref()
const mustChangeLoading = ref(false)
const mustChangeForm = reactive({
  employeeId: '',
  oldPassword: '',
  newPassword: '',
  confirmPassword: '',
})

// Login form data
const formData = reactive<{ [key: string]: any }>({
  email: '',
  password: '',
})

// Login form validation rules（登录标识是工号，只做必填校验，不做邮箱格式校验）
const formRules = computed(() => ({
  email: [
    { required: true, message: t('auth.employeeIdRequired'), type: 'error' }
  ],
  password: [
    { required: true, message: t('auth.passwordRequired'), type: 'error' },
    { min: 8, message: t('auth.passwordMinLength'), type: 'error' },
    { max: 32, message: t('auth.passwordMaxLength'), type: 'error' }
  ],
}))

// 强制改密表单校验：新密码复用注册处的密码策略（含复杂密码开关），
// 文案沿用个人资料页改密弹层的 key，避免两套说法。
const mustChangeRules = computed(() => ({
  oldPassword: [
    { required: true, message: t('userProfile.changePassword.currentRequired'), type: 'error' }
  ],
  newPassword: newPasswordRules(t, complexPasswordEnabled.value),
  confirmPassword: [
    { required: true, message: t('auth.confirmPasswordRequired'), type: 'error' },
    {
      validator: (val: string) => val === mustChangeForm.newPassword,
      message: t('auth.passwordMismatch'),
      type: 'error'
    }
  ]
}))

// 登录成功且 must_change_password=true 时切入强制改密卡片。先持久化会话
// （change-password 在后端白名单内，需要携带有效 Authorization），旧密码
// 用刚输入的值代填。改密成功后这套会话会被整体吊销清空。
const enterMustChangeMode = async (response: any) => {
  await persistLoginResponse(response, true)
  mustChangeForm.employeeId = response.user?.employee_id || formData.email.trim()
  mustChangeForm.oldPassword = formData.password
  mustChangeForm.newPassword = ''
  mustChangeForm.confirmPassword = ''
  mustChangeMode.value = true
  await nextTick()
}

// 放弃改密回到登录卡片。token 除白名单外全部 403，留着只会触发更多
// 拦截，直接清空本地会话。
const exitMustChangeMode = () => {
  authStore.logout()
  mustChangeMode.value = false
  formData.password = ''
  mustChangeForm.oldPassword = ''
  mustChangeForm.newPassword = ''
  mustChangeForm.confirmPassword = ''
}

const handleMustChangeSubmit = async () => {
  try {
    const valid = await mustChangeFormRef.value?.validate()
    if (valid !== true) return

    mustChangeLoading.value = true
    const result = await changePassword({
      old_password: mustChangeForm.oldPassword,
      new_password: mustChangeForm.newPassword,
    })
    if (!result.success) {
      // changePassword 内部已把后端错误映射成本地化文案
      MessagePlugin.error(result.message || t('userProfile.changePassword.failed'))
      return
    }

    // 改密成功：后端已吊销全部 token，清空本地会话，回登录卡片用新
    // 密码重新进入（工号代填，密码留空）。
    const employeeId = mustChangeForm.employeeId
    authStore.logout()
    mustChangeMode.value = false
    formData.email = employeeId
    formData.password = ''
    mustChangeForm.oldPassword = ''
    mustChangeForm.newPassword = ''
    mustChangeForm.confirmPassword = ''
    MessagePlugin.success(t('auth.mustChangePassword.success'))
  } catch (error: any) {
    console.error('强制改密错误:', error)
    MessagePlugin.error(error.message || t('userProfile.changePassword.failed'))
  } finally {
    mustChangeLoading.value = false
  }
}

const persistLoginResponse = async (response: any, skipRedirect = false) => {
  const activeTenant = response.active_tenant || response.tenant
  if (response.user && response.token) {
    const homeTenantIdRaw = response.user.tenant_id ?? activeTenant?.id ?? ''
    authStore.setUser(userInfoFromApi(response.user, homeTenantIdRaw))
    authStore.setToken(response.token)
    if (response.refresh_token) {
      authStore.setRefreshToken(response.refresh_token)
    }
    if (activeTenant) {
      authStore.setTenant({
        id: String(activeTenant.id) || '',
        name: activeTenant.name || '',
        owner_id: response.user.id || '',
        created_at: activeTenant.created_at || new Date().toISOString(),
        updated_at: activeTenant.updated_at || new Date().toISOString()
      })
    } else {
      authStore.setTenant(null)
    }
    if (Array.isArray(response.memberships)) {
      authStore.setMemberships(response.memberships)
    }
    const activeIdNum = Number(activeTenant?.id)
    const homeIdNum = Number(homeTenantIdRaw)
    if (Number.isFinite(activeIdNum) && Number.isFinite(homeIdNum) && activeIdNum !== homeIdNum) {
      authStore.setSelectedTenant(activeIdNum, activeTenant?.name || null)
    } else {
      authStore.setSelectedTenant(null, null)
    }
  }

  await authStore.refreshFromAuthMe()
  await nextTick()
  if (skipRedirect) return
  // 无空间时的落脚点：系统管理员（bootstrap 账号）进管理控制台，
  // 普通用户进等待分配页 —— 与路由守卫 resolveWorkspaceEntry 保持一致。
  router.replace(
    authStore.hasValidTenant
      ? '/platform/knowledge-bases'
      : authStore.isSystemAdmin
        ? '/system/console'
        : '/onboarding/workspace',
  )
}

// Handle login
const handleLogin = async () => {
  try {
    const valid = await formRef.value?.validate()
    if (valid !== true) return

    loading.value = true

    const response = await login({
      employee_id: formData.email.trim(),
      password: formData.password,
    })

    if (response.success) {
      // 认证已通过才记入本系统登录历史（含待强制改密的首登）
      rememberLoginId(response.user?.employee_id || formData.email.trim())
      // 管理员开户/重置密码的首登：优先于一切后续流程，
      // 先完成改密再重新登录。
      if (response.must_change_password || response.user?.must_change_password) {
        await enterMustChangeMode(response)
        return
      }
      await persistLoginResponse(response)
      notifyLoginSuccess(response, t, tm, formatRole, roleIcon)
    } else {
      MessagePlugin.error(localizeAuthError(response.message))
    }
  } catch (error: any) {
    console.error('登录错误:', error)
    MessagePlugin.error(error.message || t('auth.loginErrorRetry'))
  } finally {
    loading.value = false
  }
}

// Check if already logged in
onMounted(async () => {
  loadLoginHistory()
  if (authStore.isLoggedIn) {
    // 管理员重置密码后，存量会话的后续请求被 403 PASSWORD_CHANGE_REQUIRED
    // 拦回登录页。此时本地会话仍有效，若照常跳平台页会形成
    // /login → 平台页 → 403 → /login 的循环；检测到强制改密标记则直接
    // 进入改密卡片（旧密码需用户自行输入——存量会话没有明文密码）。
    if (authStore.user?.must_change_password) {
      mustChangeForm.employeeId = authStore.user.employee_id || ''
      mustChangeForm.oldPassword = ''
      mustChangeMode.value = true
      return
    }
    router.replace(
      authStore.hasValidTenant
        ? '/platform/knowledge-bases'
        : authStore.isSystemAdmin
          ? '/system/console'
          : '/onboarding/workspace',
    )
    return
  }
})
</script>

<style lang="less" scoped>
.login-layout {
  display: flex;
  width: 100%;
  min-height: 100%;
  overflow: hidden;
  position: relative;
  background: linear-gradient(225deg, #022c22 0%, #064e3b 15%, #065f46 25%, #047857 38%, #059669 50%, #07C05F 65%, #10B981 78%, #34D399 90%, #6EE7B7 100%);

  &::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background: radial-gradient(circle at 20% 50%, rgba(255, 255, 255, 0.06) 0%, transparent 50%),
      radial-gradient(circle at 80% 50%, rgba(255, 255, 255, 0.04) 0%, transparent 50%);
    pointer-events: none;
  }
}

.animated-bg {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  pointer-events: none;
  z-index: 1;
  overflow: hidden;
  contain: strict;
}

.knowledge-node {
  position: absolute;
  width: 40px;
  height: 40px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.15);
  border: 2px solid rgba(255, 255, 255, 0.3);
  box-shadow:
    0 0 15px rgba(255, 255, 255, 0.35),
    0 0 30px rgba(16, 185, 129, 0.2),
    inset 0 0 8px rgba(255, 255, 255, 0.1);
  display: flex;
  align-items: center;
  justify-content: center;
  animation: nodePulse 5s infinite ease-in-out;
  will-change: transform, opacity;
}

.node-icon {
  width: 20px;
  height: 20px;
  color: rgba(255, 255, 255, 0.9);
}

.node-1 {
  top: 15%;
  left: 20%;
  animation-delay: 0s;
}

.node-2 {
  top: 25%;
  left: 35%;
  animation-delay: 0.5s;
}

.node-3 {
  top: 20%;
  left: 55%;
  animation-delay: 1s;
}

.node-4 {
  top: 30%;
  left: 75%;
  animation-delay: 1.5s;
}

.node-5 {
  top: 45%;
  left: 25%;
  animation-delay: 2s;
}

.node-6 {
  top: 50%;
  left: 45%;
  animation-delay: 2.5s;
}

.node-7 {
  top: 48%;
  left: 65%;
  animation-delay: 3s;
}

.node-8 {
  top: 60%;
  left: 20%;
  animation-delay: 0.3s;
}

.node-9 {
  top: 12%;
  right: 15%;
  animation-delay: 1.8s;
}

.node-10 {
  top: 38%;
  right: 10%;
  animation-delay: 2.3s;
}

.node-11 {
  top: 70%;
  left: 40%;
  animation-delay: 0.8s;
}

.node-12 {
  top: 65%;
  left: 80%;
  animation-delay: 1.3s;
}

@keyframes nodePulse {

  0%,
  100% {
    transform: scale(1);
    opacity: 0.65;
  }

  50% {
    transform: scale(1.08);
    opacity: 0.9;
  }
}

.knowledge-lines {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  opacity: 0.35;
}

.connection-line {
  stroke: rgba(255, 255, 255, 0.5);
  stroke-width: 1.5;
  stroke-dasharray: 6, 3;
  stroke-linecap: round;
  animation: lineFlow 10s infinite linear;
  will-change: stroke-dashoffset;
}

.line-1 {
  animation-delay: 0s;
}

.line-2 {
  animation-delay: 0.5s;
}

.line-3 {
  animation-delay: 1s;
}

.line-4 {
  animation-delay: 0.3s;
}

.line-5 {
  animation-delay: 0.8s;
}

.line-6 {
  animation-delay: 1.3s;
}

.line-7 {
  animation-delay: 1.8s;
}

.line-8 {
  animation-delay: 2.3s;
}

.line-9 {
  animation-delay: 0.2s;
}

.line-10 {
  animation-delay: 0.7s;
}

.line-11 {
  animation-delay: 0.9s;
}

.line-12 {
  animation-delay: 1.5s;
}

@keyframes lineFlow {
  0% {
    stroke-dashoffset: 0;
  }

  100% {
    stroke-dashoffset: 18;
  }
}

@media (prefers-reduced-motion: reduce) {
  .knowledge-node {
    animation: none;
    opacity: 0.65;
  }

  .connection-line {
    animation: none;
  }
}

/* Centered Form Section */
.form-section {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 40px 24px;
  box-sizing: border-box;
  position: relative;
  z-index: 2;
}

.form-panel {
  width: 100%;
  max-width: 400px;
  position: relative;
}

.form-card {
  background: rgba(255, 255, 255, 0.97);
  border-radius: 16px;
  padding: 40px;
  box-shadow: 0 10px 40px rgba(0, 0, 0, 0.15);
  box-sizing: border-box;
  border: none;
  width: 100%;
}

.form-header {
  text-align: center;
  margin-bottom: 32px;
}

.brand-logo {
  width: 56px;
  height: 56px;
  display: block;
  margin: 0 auto 16px;
}

.test-mode-note {
  margin: 20px 0 0;
  text-align: center;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
  font-family: var(--app-font-family);
  user-select: none;
}

.form-title {
  font-size: 26px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  margin: 0 0 6px 0;
  font-family: var(--app-font-family);
}

.form-welcome {
  font-size: 13px;
  color: var(--td-text-color-secondary);
  margin: 0;
  font-family: var(--app-font-family);
}

.form-subtitle {
  font-size: 13px;
  color: var(--td-text-color-secondary);
  margin: 0;
  font-family: var(--app-font-family);
}

.form-content {
  :deep(.t-form-item__label) {
    font-size: 14px;
    color: var(--td-text-color-primary);
    font-weight: 500;
    margin-bottom: 8px;
    font-family: var(--app-font-family);
    display: block;
    text-align: left;
  }

  :deep(.t-input) {
    border: 1px solid var(--td-component-stroke);
    border-radius: 8px;
    background: var(--td-bg-color-container);
    transition: all 0.2s;

    &:focus-within {
      border-color: var(--td-brand-color);
      box-shadow: 0 0 0 3px rgba(7, 192, 95, 0.1);
    }

    &:hover {
      border-color: var(--td-brand-color);
    }

    .t-input__inner {
      border: none !important;
      box-shadow: none !important;
      outline: none !important;
      background: transparent;
      font-size: 15px;
      font-family: var(--app-font-family);

      &:focus {
        border: none !important;
        box-shadow: none !important;
        outline: none !important;
      }
    }

    .t-input__wrap {
      border: none !important;
      box-shadow: none !important;
    }
  }

  :deep(.t-form-item) {
    margin-bottom: 18px;

    &:last-child {
      margin-bottom: 0;
    }
  }

  :deep(.t-form-item__control) {
    width: 100%;
  }
}

.submit-button {
  height: 46px;
  border-radius: 8px;
  font-size: 16px;
  font-weight: 500;
  font-family: var(--app-font-family);
  margin: 20px 0 0 0;
}

.form-footer {
  text-align: center;
  font-size: 14px;
  color: var(--td-text-color-secondary);
  font-family: var(--app-font-family);
  margin-top: 16px;
  padding-bottom: 16px;
  border-bottom: 1px solid var(--td-component-stroke);

  .link-button {
    color: var(--td-brand-color);
    text-decoration: none;
    margin-left: 4px;
    font-weight: 500;
    transition: all 0.2s;

    &:hover {
      color: var(--td-brand-color);
      text-decoration: underline;
    }
  }
}

/* Responsive Design */
@media (max-width: 768px) {
  .knowledge-node:nth-of-type(n + 9) {
    display: none;
  }

  .connection-line:nth-of-type(n + 9) {
    display: none;
  }

  .form-section {
    padding: 24px;
  }

  .form-card {
    padding: 32px 24px;
  }

  .form-title {
    font-size: 22px;
  }
}

@media (max-width: 480px) {
  .animated-bg {
    display: none;
  }

  .form-section {
    padding: 20px;
  }

  .form-card {
    padding: 28px 20px;
  }

  .form-header {
    margin-bottom: 24px;
  }
}

@media (prefers-reduced-motion: reduce) {

  .knowledge-node,
  .connection-line {
    animation: none !important;
    transition: none !important;
  }

  .animated-bg {
    display: none;
  }
}
</style>

<style lang="less">
html[theme-mode="dark"] {
  .login-layout {
    background: linear-gradient(225deg, #011a14 0%, #032e22 15%, #043a2c 25%, #05503d 38%, #046647 50%, #038a56 65%, #049b60 78%, #06a06a 90%, #07b074 100%);
  }

  .knowledge-node {
    background: rgba(255, 255, 255, 0.1);
    border-color: rgba(255, 255, 255, 0.2);
    box-shadow: 0 0 8px rgba(255, 255, 255, 0.15);
  }

  .connection-line {
    stroke: rgba(255, 255, 255, 0.25);
  }

  .form-card {
    background: rgba(36, 36, 36, 0.97) !important;
    box-shadow: 0 10px 40px rgba(0, 0, 0, 0.4) !important;
  }

  .form-content .t-input {
    background: var(--td-bg-color-page) !important;
    border-color: rgba(255, 255, 255, 0.1) !important;

    &:hover {
      border-color: var(--td-brand-color) !important;
    }

    &:focus-within {
      border-color: var(--td-brand-color) !important;
    }
  }
}
</style>
