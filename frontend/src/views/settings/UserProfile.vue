<template>
  <div class="user-profile">
    <div class="section-header">
      <h2>{{ $t('userProfile.title') }}</h2>
      <p class="section-description">{{ $t('userProfile.description') }}</p>
    </div>

    <!-- Loading -->
    <div v-if="loading" class="loading-inline">
      <t-loading size="small" />
      <span>{{ $t('tenant.loadingInfo') }}</span>
    </div>

    <!-- Error -->
    <div v-else-if="error" class="error-inline">
      <t-alert theme="error" :message="error">
        <template #operation>
          <t-button size="small" @click="loadInfo">{{ $t('tenant.retry') }}</t-button>
        </template>
      </t-alert>
    </div>

    <!-- Content -->
    <div v-else class="settings-group">
      <!-- 头像：灰色默认可上传裁剪替换；ref 在 authStore，字节经 useMyAvatar 拉成 blob -->
      <div class="setting-row">
        <div class="setting-info">
          <label>{{ $t('userProfile.avatar.label') }}</label>
          <p class="desc">{{ $t('userProfile.avatar.description') }}</p>
        </div>
        <div class="setting-control setting-control--avatar">
          <UserAvatar :src="avatarUrl" :name="userInfo?.username" size="lg" />
          <div class="avatar-actions">
            <t-tooltip v-if="!canUseAvatar" :content="$t('userProfile.avatar.noWorkspace')">
              <t-button size="small" variant="outline" disabled>
                {{ hasAvatar ? $t('userProfile.avatar.change') : $t('userProfile.avatar.upload') }}
              </t-button>
            </t-tooltip>
            <t-button
              v-else
              size="small"
              variant="outline"
              :loading="uploading"
              @click="pickAvatarFile"
            >
              {{ hasAvatar ? $t('userProfile.avatar.change') : $t('userProfile.avatar.upload') }}
            </t-button>
            <t-button
              v-if="hasAvatar"
              size="small"
              variant="text"
              theme="danger"
              :disabled="uploading"
              @click="removeAvatar"
            >
              {{ $t('userProfile.avatar.remove') }}
            </t-button>
          </div>
          <input
            ref="avatarInputRef"
            type="file"
            accept="image/png,image/jpeg,image/webp"
            class="avatar-input"
            @change="onAvatarFilePicked"
          >
        </div>
      </div>

      <!-- 工号 -->
      <div class="setting-row">
        <div class="setting-info">
          <label>{{ $t('userProfile.employeeIdLabel') }}</label>
          <p class="desc">{{ $t('userProfile.employeeIdDescription') }}</p>
        </div>
        <div class="setting-control">
          <span class="info-value">{{ userInfo?.employee_id || '-' }}</span>
        </div>
      </div>

      <!-- 姓名 -->
      <div class="setting-row">
        <div class="setting-info">
          <label>{{ $t('userProfile.nameLabel') }}</label>
          <p class="desc">{{ $t('userProfile.nameDescription') }}</p>
        </div>
        <div class="setting-control">
          <span class="info-value">{{ userInfo?.username || '-' }}</span>
        </div>
      </div>

      <!-- 注册时间 -->
      <div class="setting-row">
        <div class="setting-info">
          <label>{{ $t('tenant.api.createdAtLabel') }}</label>
          <p class="desc">{{ $t('tenant.api.createdAtDescription') }}</p>
        </div>
        <div class="setting-control">
          <span class="info-value">{{ formatDate(userInfo?.created_at) }}</span>
        </div>
      </div>

      <!-- 用户 ID（企业版置底，日常以工号为主标识） -->
      <div class="setting-row">
        <div class="setting-info">
          <label>{{ $t('tenant.api.userIdLabel') }}</label>
          <p class="desc">{{ $t('tenant.api.userIdDescription') }}</p>
        </div>
        <div class="setting-control">
          <span class="info-value">{{ userInfo?.id || '-' }}</span>
        </div>
      </div>

      <!-- 修改密码：与其它 setting-row 同款只读行 + 编辑入口，表单进原地 popup -->
      <div class="setting-row">
        <div class="setting-info">
          <label>{{ $t('userProfile.changePassword.label') }}</label>
          <p class="desc">
            {{ oidcOnlyLogin
              ? $t('userProfile.changePassword.oidcOnlyDescription')
              : $t('userProfile.changePassword.description') }}
          </p>
        </div>
        <div class="setting-control">
          <template v-if="oidcOnlyLogin">
            <span class="info-value info-value--muted">—</span>
          </template>
          <template v-else>
            <span class="info-value password-mask" aria-hidden="true">••••••••</span>
            <t-popup
              v-model="passwordPopupVisible"
              trigger="click"
              placement="bottom-end"
              destroy-on-close
              overlay-class-name="user-profile-password-popup-overlay"
            >
              <t-button
                theme="default"
                variant="text"
                shape="square"
                size="small"
                class="edit-btn"
                :title="$t('userProfile.changePassword.label')"
                :aria-label="$t('userProfile.changePassword.label')"
              >
                <template #icon>
                  <t-icon name="edit" />
                </template>
              </t-button>
              <template #content>
                <div class="password-popup-inner" @click.stop>
                  <div class="password-popup-title">{{ $t('userProfile.changePassword.label') }}</div>
                  <p class="password-popup-hint">{{ $t('userProfile.changePassword.description') }}</p>
                  <t-form
                    ref="passwordFormRef"
                    :data="passwordForm"
                    :rules="passwordRules"
                    label-align="top"
                    class="password-popup-form"
                    @submit.prevent
                  >
                    <t-form-item :label="$t('userProfile.changePassword.currentLabel')" name="oldPassword">
                      <t-input
                        v-model="passwordForm.oldPassword"
                        type="password"
                        autocomplete="current-password"
                        :disabled="passwordSubmitting"
                        :placeholder="$t('userProfile.changePassword.currentPlaceholder')"
                      />
                    </t-form-item>
                    <t-form-item :label="$t('userProfile.changePassword.newLabel')" name="newPassword">
                      <t-input
                        v-model="passwordForm.newPassword"
                        type="password"
                        autocomplete="new-password"
                        :disabled="passwordSubmitting"
                        :placeholder="$t('userProfile.changePassword.newPlaceholder')"
                      />
                    </t-form-item>
                    <t-form-item :label="$t('userProfile.changePassword.confirmLabel')" name="confirmPassword">
                      <t-input
                        v-model="passwordForm.confirmPassword"
                        type="password"
                        autocomplete="new-password"
                        :disabled="passwordSubmitting"
                        :placeholder="$t('userProfile.changePassword.confirmPlaceholder')"
                        @enter="submitPasswordChange"
                      />
                    </t-form-item>
                  </t-form>
                  <div class="password-popup-footer">
                    <t-button
                      variant="outline"
                      :disabled="passwordSubmitting"
                      @click="closePasswordPopup"
                    >
                      {{ $t('common.cancel') }}
                    </t-button>
                    <t-button
                      theme="primary"
                      :loading="passwordSubmitting"
                      @click="submitPasswordChange"
                    >
                      {{ $t('userProfile.changePassword.submit') }}
                    </t-button>
                  </div>
                </div>
              </template>
            </t-popup>
          </template>
        </div>
      </div>
    </div>

    <!-- 裁剪弹窗：确认后才真正上传 -->
    <AvatarCropDialog
      v-model:visible="cropDialogVisible"
      :file="pendingAvatarFile"
      @confirm="onAvatarCropped"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import type { FormInstanceFunctions, FormRule } from 'tdesign-vue-next'
import {
  getCurrentUser,
  changePassword,
  logout as logoutApi,
  uploadMyAvatar,
  deleteMyAvatar,
  type UserInfo,
} from '@/api/auth'
import UserAvatar from '@/components/UserAvatar.vue'
import AvatarCropDialog from '@/components/AvatarCropDialog.vue'
import { useMyAvatar } from '@/composables/useMyAvatar'
import { useAuthStore } from '@/stores/auth'
import { useI18n } from 'vue-i18n'
import { newPasswordRules } from '@/utils/passwordPolicy'

const { t, locale } = useI18n()
const router = useRouter()
const authStore = useAuthStore()
const { avatarUrl } = useMyAvatar()

const userInfo = ref<UserInfo | null>(null)
const loading = ref(true)
const error = ref('')

// --- 头像 ---
const avatarInputRef = ref<HTMLInputElement | null>(null)
const cropDialogVisible = ref(false)
const pendingAvatarFile = ref<File | null>(null)
const uploading = ref(false)

const MAX_AVATAR_UPLOAD_BYTES = 2 * 1024 * 1024
const AVATAR_MIME_TYPES = ['image/png', 'image/jpeg', 'image/webp']

const hasAvatar = computed(() => !!authStore.user?.avatar)
// 无空间的 tenantless 用户无法存头像（资源目录要求归属租户），给禁用态
// 与提示而不是让后端报错。
const canUseAvatar = computed(
  () => !!authStore.user?.tenant_id || authStore.memberships.length > 0,
)

function pickAvatarFile(): void {
  avatarInputRef.value?.click()
}

function onAvatarFilePicked(e: Event): void {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = '' // 复位以允许再次选择同一文件
  if (!file) return
  if (!AVATAR_MIME_TYPES.includes(file.type)) {
    MessagePlugin.error(t('userProfile.avatar.invalidType'))
    return
  }
  if (file.size > MAX_AVATAR_UPLOAD_BYTES) {
    MessagePlugin.error(t('userProfile.avatar.tooLarge'))
    return
  }
  pendingAvatarFile.value = file
  cropDialogVisible.value = true
}

// 后端的 ErrAvatarNoWorkspace 是固定英文串；映射成可读的本地提示。
function avatarErrorMessage(message?: string): string {
  if (message && message.includes('workspace')) {
    return t('userProfile.avatar.noWorkspace')
  }
  return message || t('userProfile.avatar.uploadFailed')
}

async function onAvatarCropped(file: File): Promise<void> {
  if (uploading.value) return
  uploading.value = true
  try {
    const resp = await uploadMyAvatar(file)
    if (!resp.success || !resp.data?.user) {
      MessagePlugin.error(avatarErrorMessage(resp.message))
      return
    }
    userInfo.value = resp.data.user
    // user.avatar 变化 → useMyAvatar 自动重拉字节，侧边栏菜单同步更新。
    await authStore.refreshFromAuthMe()
    MessagePlugin.success(t('userProfile.avatar.updated'))
  } catch (err: any) {
    MessagePlugin.error(avatarErrorMessage(err?.message))
  } finally {
    uploading.value = false
  }
}

async function removeAvatar(): Promise<void> {
  if (uploading.value) return
  uploading.value = true
  try {
    const resp = await deleteMyAvatar()
    if (!resp.success) {
      MessagePlugin.error(resp.message || t('userProfile.avatar.removeFailed'))
      return
    }
    await authStore.refreshFromAuthMe()
    if (userInfo.value) userInfo.value = { ...userInfo.value, avatar: '' }
    MessagePlugin.success(t('userProfile.avatar.removed'))
  } finally {
    uploading.value = false
  }
}

const passwordPopupVisible = ref(false)
const passwordFormRef = ref<FormInstanceFunctions | null>(null)
const passwordSubmitting = ref(false)
const passwordForm = reactive({
  oldPassword: '',
  newPassword: '',
  confirmPassword: '',
})

const oidcOnlyLogin = computed(
  () => userInfo.value?.preferences?.oidc_only_login === true,
)

watch(passwordPopupVisible, (open) => {
  if (!open) {
    resetPasswordForm()
    return
  }
  resetPasswordForm()
})

const passwordRules = computed<Record<string, FormRule[]>>(() => ({
  oldPassword: [
    { required: true, message: t('userProfile.changePassword.currentRequired'), type: 'error' },
  ],
  newPassword: newPasswordRules(t, [
    {
      validator: (val: string) => val !== passwordForm.oldPassword,
      message: t('userProfile.changePassword.sameAsCurrent'),
      type: 'error',
    },
  ]),
  confirmPassword: [
    { required: true, message: t('auth.confirmPasswordRequired'), type: 'error' },
    {
      validator: (val: string) => val === passwordForm.newPassword,
      message: t('auth.passwordMismatch'),
      type: 'error',
      trigger: 'blur',
    },
  ],
}))

const loadInfo = async () => {
  try {
    loading.value = true
    error.value = ''
    const resp = await getCurrentUser()
    if ((resp as any).success && resp.data) {
      userInfo.value = resp.data.user
    } else {
      error.value = resp.message || t('tenant.messages.fetchFailed')
    }
  } catch (err: any) {
    error.value = err?.message || t('tenant.messages.networkError')
  } finally {
    loading.value = false
  }
}

const formatDate = (dateStr: string | undefined) => {
  if (!dateStr) return t('tenant.unknown')
  try {
    const d = new Date(dateStr)
    const fmt = new Intl.DateTimeFormat(locale.value || 'zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    })
    return fmt.format(d)
  } catch {
    return t('tenant.formatError')
  }
}

const resetPasswordForm = () => {
  passwordForm.oldPassword = ''
  passwordForm.newPassword = ''
  passwordForm.confirmPassword = ''
  passwordFormRef.value?.clearValidate?.()
}

const closePasswordPopup = () => {
  if (passwordSubmitting.value) return
  passwordPopupVisible.value = false
  resetPasswordForm()
}

const submitPasswordChange = async () => {
  if (passwordSubmitting.value) return
  const result = await passwordFormRef.value?.validate?.()
  if (result !== true) return

  passwordSubmitting.value = true
  try {
    const resp = await changePassword({
      old_password: passwordForm.oldPassword,
      new_password: passwordForm.newPassword,
    })
    if (!resp.success) {
      MessagePlugin.error(resp.message || t('userProfile.changePassword.failed'))
      return
    }

    passwordPopupVisible.value = false
    MessagePlugin.success(t('userProfile.changePassword.success'))
    resetPasswordForm()

    // Backend revokes all sessions on success; mirror that locally and
    // force a fresh login with the new credential.
    try {
      await logoutApi()
    } catch {
      /* ignore — local cleanup still proceeds */
    }
    authStore.logout()
    router.push('/login')
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('userProfile.changePassword.failed'))
  } finally {
    passwordSubmitting.value = false
  }
}

onMounted(loadInfo)
</script>

<style lang="less" scoped>
.user-profile {
  width: 100%;
}

.section-header {
  margin-bottom: 32px;

  h2 {
    font-size: 20px;
    font-weight: 600;
    color: var(--td-text-color-primary);
    margin: 0 0 8px 0;
  }

  .section-description {
    font-size: 14px;
    color: var(--td-text-color-secondary);
    margin: 0;
    line-height: 1.5;
  }
}

.loading-inline {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 40px 0;
  justify-content: center;
  color: var(--td-text-color-secondary);
  font-size: 14px;
}

.error-inline {
  padding: 20px 0;
}

.settings-group {
  display: flex;
  flex-direction: column;
  gap: 0;
}

.setting-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding: 20px 0;
  border-bottom: 1px solid var(--td-component-stroke);

  &:last-child {
    border-bottom: none;
  }
}

.setting-info {
  flex: 1;
  max-width: 65%;
  padding-right: 24px;

  label {
    font-size: 15px;
    font-weight: 500;
    color: var(--td-text-color-primary);
    display: block;
    margin-bottom: 4px;
  }

  .desc {
    font-size: 13px;
    color: var(--td-text-color-secondary);
    margin: 0;
    line-height: 1.5;
  }
}

.setting-control {
  flex-shrink: 0;
  min-width: 280px;
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 8px;

  .info-value {
    font-size: 14px;
    color: var(--td-text-color-primary);
    text-align: right;
    word-break: break-word;
  }

  .info-value--muted {
    color: var(--td-text-color-placeholder);
  }

  .edit-btn {
    flex-shrink: 0;
  }
}

.setting-control--avatar {
  align-items: center;

  .avatar-actions {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 4px;
  }
}

.avatar-input {
  display: none;
}

.password-mask {
  letter-spacing: 0.12em;
  color: var(--td-text-color-secondary);
}

.password-popup-inner {
  max-width: 100%;
}

.password-popup-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  margin: 0 0 8px;
  line-height: 1.35;
}

.password-popup-hint {
  margin: 0 0 12px;
  font-size: 13px;
  line-height: 1.55;
  color: var(--td-text-color-secondary);
}

.password-popup-form {
  :deep(.t-form__item) {
    margin-bottom: 14px;

    &:last-child {
      margin-bottom: 4px;
    }
  }
}

.password-popup-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 16px;
}
</style>

<style lang="less">
/* t-popup 挂到 body，需全局样式；z-index 需高于设置全屏遮罩（2000）。 */
.user-profile-password-popup-overlay {
  z-index: 3050 !important;

  .t-popup__content {
    padding: 14px 16px !important;
    min-width: 300px;
    max-width: min(392px, calc(100vw - 24px));
    border-radius: 12px !important;
    background: var(--td-bg-color-container) !important;
    border: 0.5px solid var(--td-component-stroke) !important;
    box-shadow:
      0 0 0 0.5px rgba(0, 0, 0, 0.03),
      0 2px 4px rgba(0, 0, 0, 0.04),
      0 8px 24px rgba(0, 0, 0, 0.1) !important;
    backdrop-filter: blur(20px) saturate(180%) !important;
    -webkit-backdrop-filter: blur(20px) saturate(180%) !important;
  }
}

:root[theme-mode='dark'] .user-profile-password-popup-overlay .t-popup__content {
  background: rgba(36, 36, 36, 0.92) !important;
  border-color: rgba(255, 255, 255, 0.08) !important;
  box-shadow:
    0 0 0 0.5px rgba(255, 255, 255, 0.05),
    0 2px 4px rgba(0, 0, 0, 0.12),
    0 8px 32px rgba(0, 0, 0, 0.28) !important;
}
</style>
