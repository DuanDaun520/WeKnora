<template>
  <Teleport to="body">
    <t-dialog
      :visible="visible"
      width="960px"
      :footer="false"
      :close-on-overlay-click="false"
      attach="body"
      @close="handleClose"
    >
      <!-- 标题行右侧挂「成员行为日志」入口（000101 第二轮：从头像菜单
           移到这里，菜单里不再单独占一项）。 -->
      <template #header>
        <div class="member-manage-header">
          <span>{{ t('memberManage.title') }}</span>
          <t-button variant="text" size="small" class="member-manage-header-audit" @click="openMemberAudit">
            {{ t('memberAudit.title') }}
          </t-button>
        </div>
      </template>
      <div class="member-manage">
        <p class="member-manage-desc">{{ t('memberManage.description') }}</p>
        <!-- 顶部：搜索 + 添加成员 -->
        <div class="member-manage-toolbar">
          <t-input
            v-model="searchQuery"
            class="member-manage-search"
            clearable
            :placeholder="t('memberManage.searchPlaceholder')"
            @change="onSearchChange"
          >
            <template #prefix-icon><t-icon name="search" /></template>
          </t-input>
          <t-button theme="primary" @click="openAddDialog">
            <template #icon><t-icon name="add" /></template>
            {{ t('memberManage.add.button') }}
          </t-button>
        </div>

        <!-- 名册表格：工号 / 姓名 / 角色 / 添加时间 / 最后登录 / 操作。
             弹窗高度固定：行数少时表格下方留白，行数多时在内容区内滚动。 -->
        <div v-if="loading && members.length === 0" class="member-manage-hint">
          <t-loading size="small" />
          <span>{{ t('memberManage.loading') }}</span>
        </div>
        <div v-else-if="error" class="member-manage-hint">
          <t-alert theme="error" :message="error">
            <template #operation>
              <t-button size="small" @click="reload">{{ t('memberManage.retry') }}</t-button>
            </template>
          </t-alert>
        </div>
        <div v-else-if="members.length === 0" class="member-manage-hint">
          <t-empty :description="searchQuery.trim()
            ? t('memberManage.emptySearch', { q: searchQuery })
            : t('memberManage.empty')" />
        </div>
        <div v-else class="member-manage-body">
          <t-table
            row-key="user_id"
            :data="members"
            :columns="columns"
            size="medium"
            hover
            stripe
            :loading="loading"
          >
            <template #role="{ row }">
              <t-tag :theme="isManagerRole(row.role) ? 'primary' : 'default'" size="small">
                {{ roleLabel(row.role) }}
              </t-tag>
            </template>
            <template #joined_at="{ row }">{{ formatDateTime(row.joined_at) }}</template>
            <template #last_login_at="{ row }">
              <span :class="{ 'member-manage-never-login': !row.last_login_at }">
                {{ row.last_login_at ? formatDateTime(row.last_login_at) : t('memberManage.lastLoginNever') }}
              </span>
            </template>
            <template #operations="{ row }">
              <div class="member-manage-ops">
                <t-button variant="text" size="small" @click="onResetPassword(row)">
                  {{ t('memberManage.ops.resetPassword') }}
                </t-button>
                <!-- 邀请只服务从未登录的成员：登录过的成员已有已知密码可登录。 -->
                <t-button
                  variant="text"
                  size="small"
                  :disabled="!!row.last_login_at"
                  @click="onInvite(row)"
                >
                  {{ t('memberManage.ops.invite') }}
                </t-button>
                <t-button variant="text" size="small" @click="onStats(row)">
                  {{ t('memberManage.ops.stats') }}
                </t-button>
                <t-button
                  variant="text"
                  size="small"
                  :class="{ 'member-manage-remove-btn': !isProtected(row) }"
                  @click="onRemove(row)"
                >
                  {{ t('memberManage.ops.remove') }}
                </t-button>
              </div>
            </template>
          </t-table>
          <div v-if="total > pageSize" class="member-manage-pager">
            <t-pagination
              v-model="page"
              :total="total"
              :page-size="pageSize"
              size="small"
              show-jumper
              @change="onPageChange"
            />
          </div>
        </div>
      </div>

      <!-- 添加成员（工号 + 姓名，默认密码 abc1234#） -->
      <t-dialog
        :visible="addVisible"
        :header="t('memberManage.add.title')"
        width="480px"
        :confirm-btn="{
          content: addSubmitting ? t('memberManage.add.submitting') : t('memberManage.add.submit'),
          loading: addSubmitting,
          disabled: addSubmitting,
        }"
        :close-on-overlay-click="false"
        :on-confirm="submitAdd"
        @close="closeAddDialog"
      >
        <t-form label-align="top">
          <t-form-item :label="t('memberManage.add.employeeIdLabel')" name="employee_id">
            <t-input
              v-model="addForm.employee_id"
              :placeholder="t('memberManage.add.employeeIdPlaceholder')"
              :maxlength="64"
              :disabled="addSubmitting"
            />
          </t-form-item>
          <t-form-item :label="t('memberManage.add.usernameLabel')" name="username">
            <t-input
              v-model="addForm.username"
              :placeholder="t('memberManage.add.usernamePlaceholder')"
              :maxlength="50"
              :disabled="addSubmitting"
            />
          </t-form-item>
        </t-form>
        <t-alert theme="info" :message="t('memberManage.add.passwordHint')" />
      </t-dialog>

      <!-- 成员已添加：主按钮复制邀请信息（含项目介绍 + 登录信息）。
           密码不再单独展示/复制 —— 初始密码只出现在邀请文本和提示条里。 -->
      <t-dialog
        :visible="addResultVisible"
        :header="t('memberManage.add.resultTitle')"
        width="480px"
        :confirm-btn="{ content: t('memberManage.add.copyInvite'), theme: 'primary' }"
        :cancel-btn="t('common.close')"
        :on-confirm="copyAddInvite"
        @close="addResultVisible = false"
      >
        <p class="member-manage-add-result-line">
          {{ t('memberManage.add.resultBody', {
            name: addResultMember?.username || '',
            employeeId: addResultMember?.employee_id || '',
          }) }}
        </p>
        <t-alert theme="info" :message="t('memberManage.add.passwordHint')" />
      </t-dialog>

      <!-- 重置密码结果：8 位数字密码仅本次返回，一次性展示。 -->
      <t-dialog
        :visible="passwordResultVisible"
        :header="passwordResultTitle"
        width="480px"
        :confirm-btn="t('common.confirm')"
        :cancel-btn="null"
        @confirm="passwordResultVisible = false"
        @close="passwordResultVisible = false"
      >
        <div class="member-manage-password-result">
          <div class="member-manage-password-value">{{ passwordResult }}</div>
          <t-button variant="outline" size="small" @click="copyPasswordResult">
            {{ t('common.copy') }}
          </t-button>
        </div>
        <p class="member-manage-password-hint">{{ t('memberManage.passwordResultHint') }}</p>
      </t-dialog>

      <!-- 成员统计 -->
      <t-dialog
        :visible="statsVisible"
        :header="t('memberManage.stats.title')"
        width="420px"
        :confirm-btn="t('common.confirm')"
        :cancel-btn="null"
        @confirm="statsVisible = false"
        @close="statsVisible = false"
      >
        <div class="member-manage-stats">
          <div class="member-manage-stats-name">
            {{ statsRow?.username }}<span v-if="statsRow?.employee_id" class="member-manage-stats-id">
              （{{ statsRow.employee_id }}）
            </span>
          </div>
          <div v-if="statsLoading" class="member-manage-hint">
            <t-loading size="small" />
          </div>
          <div v-else-if="statsData" class="member-manage-stats-grid">
            <div class="member-manage-stats-item">
              <div class="member-manage-stats-num">{{ statsData.knowledge_count }}</div>
              <div class="member-manage-stats-label">{{ t('memberManage.stats.knowledge') }}</div>
            </div>
            <div class="member-manage-stats-item">
              <div class="member-manage-stats-num">{{ statsData.session_count }}</div>
              <div class="member-manage-stats-label">{{ t('memberManage.stats.sessions') }}</div>
            </div>
          </div>
          <t-alert v-else theme="error" :message="t('memberManage.stats.loadFailed')" />
        </div>
      </t-dialog>
    </t-dialog>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { DialogPlugin, MessagePlugin } from 'tdesign-vue-next'
import { useUIStore } from '@/stores/ui'
import { useAuthStore } from '@/stores/auth'
import {
  addTenantMember,
  getMemberStats,
  listMembers,
  removeMember,
  resetMemberPassword,
  type MemberStats,
  type TenantMember,
  type TenantRole,
} from '@/api/tenant/members'
import { copyToClipboard } from '@/utils/clipboard'

// 000101 空间管理员成员管理弹窗：独立于 Settings 的全局模态框
// （UserMenu 入口 → uiStore.openMemberManage）。表格列出工号 / 姓名 /
// 角色 / 添加时间 / 最后登录（从未登录显示 -），行操作为重置密码 /
// 邀请 / 统计 / 移出空间。管理员与本人不可被空间管理员移出 —— 与后端
// RemoveTenantMember 的守卫一致，前端只提示、不调接口。
const { t } = useI18n()
const uiStore = useUIStore()
const authStore = useAuthStore()

const visible = computed(() => uiStore.showMemberManageModal)
const tenantId = computed(() => Number(authStore.currentTenantId ?? 0))
const selfUserId = computed(() => authStore.user?.id || '')

// ---------- 名册 ----------
const PAGE_SIZE = 20
const pageSize = PAGE_SIZE
const members = ref<TenantMember[]>([])
const loading = ref(false)
const error = ref('')
const total = ref(0)
const page = ref(1)
const searchQuery = ref('')
let searchTimer: ReturnType<typeof setTimeout> | null = null

// 列宽合计 ~850px < 960px 弹窗的内容区宽度（~900px），保证不出横向
// 滚动条；工号/姓名超长走省略号。
const columns = computed(() => [
  { colKey: 'employee_id', title: t('memberManage.columns.employeeId'), width: 110, ellipsis: true },
  { colKey: 'username', title: t('memberManage.columns.name'), width: 110, ellipsis: true },
  { colKey: 'role', title: t('memberManage.columns.role'), width: 90 },
  { colKey: 'joined_at', title: t('memberManage.columns.joinedAt'), width: 150 },
  { colKey: 'last_login_at', title: t('memberManage.columns.lastLoginAt'), width: 150 },
  { colKey: 'operations', title: t('memberManage.columns.operations'), width: 240 },
])

async function loadMembers() {
  if (!tenantId.value) return
  loading.value = true
  error.value = ''
  try {
    const resp = await listMembers(tenantId.value, {
      page: page.value,
      page_size: pageSize,
      q: searchQuery.value.trim() || undefined,
    })
    if (resp.success && resp.data) {
      members.value = resp.data.members || []
      total.value = resp.data.total || 0
    } else {
      error.value = resp.message || t('memberManage.errors.generic')
    }
  } catch (err: any) {
    error.value = err?.message || t('memberManage.errors.generic')
  } finally {
    loading.value = false
  }
}

function reload() {
  void loadMembers()
}

function onPageChange(next: number) {
  page.value = next
  void loadMembers()
}

function onSearchChange() {
  if (searchTimer) clearTimeout(searchTimer)
  // 300ms 防抖后回到第一页再查询，避免每个按键都打一次后端。
  searchTimer = setTimeout(() => {
    page.value = 1
    void loadMembers()
  }, 300)
}

watch(visible, (open) => {
  if (open) {
    page.value = 1
    searchQuery.value = ''
    void loadMembers()
  }
})

onUnmounted(() => {
  if (searchTimer) clearTimeout(searchTimer)
})

function handleClose() {
  uiStore.closeMemberManage()
}

// 头部右上角「成员行为日志」链接：在成员管理弹窗之上叠开行为日志
// 弹窗（两个都是全局 uiStore 模态，后开的层级更高）。
function openMemberAudit() {
  uiStore.openMemberAudit()
}

// ---------- 展示辅助 ----------
function isManagerRole(role: TenantRole | string): boolean {
  return role === 'admin' || role === 'owner'
}

function roleLabel(role: TenantRole | string): string {
  if (isManagerRole(role)) return t('memberManage.roleManager')
  return t('memberManage.roleMember')
}

function formatDateTime(s: string | null | undefined): string {
  if (!s) return '-'
  try {
    return new Intl.DateTimeFormat('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
    }).format(new Date(s))
  } catch {
    return s
  }
}

// 管理员与本人是移出保护的两种形态（后端 403 同款规则）。
function isProtected(row: TenantMember): boolean {
  return row.user_id === selfUserId.value || isManagerRole(row.role)
}

// ---------- 添加成员 ----------
const addVisible = ref(false)
const addSubmitting = ref(false)
const addForm = ref({ employee_id: '', username: '' })

// 添加成功（新建账号）：弹「成员已添加」，主按钮复制邀请信息；密码只在
// 邀请文本与提示条里出现，弹窗本身不再展示/复制密码。
const addResultVisible = ref(false)
const addResultMember = ref<TenantMember | null>(null)

// 重置密码成功后的一次性 8 位密码展示（仅此场景使用）。
const passwordResultVisible = ref(false)
const passwordResult = ref('')
const passwordResultTitle = ref('')

function openAddDialog() {
  addForm.value = { employee_id: '', username: '' }
  addVisible.value = true
}

function closeAddDialog() {
  addVisible.value = false
}

async function submitAdd() {
  const employeeId = addForm.value.employee_id.trim()
  const username = addForm.value.username.trim()
  if (!employeeId) {
    MessagePlugin.warning(t('memberManage.add.employeeIdRequired'))
    return
  }
  if (!username) {
    MessagePlugin.warning(t('memberManage.add.usernameRequired'))
    return
  }
  if (!tenantId.value) return
  addSubmitting.value = true
  try {
    const resp = await addTenantMember(tenantId.value, { employee_id: employeeId, username })
    if (!resp.success || !resp.data) {
      MessagePlugin.error(resp.message || t('memberManage.add.failed'))
      return
    }
    addVisible.value = false
    // 新建账号：弹「成员已添加」，可一键复制邀请信息（含项目介绍）；
    // 已有平台账号：只提示已直接加入本空间，无密码相关展示。
    if (resp.data.created) {
      addResultMember.value = resp.data.member
      addResultVisible.value = true
    } else {
      MessagePlugin.success(t('memberManage.add.successBound'))
    }
    page.value = 1
    searchQuery.value = ''
    void loadMembers()
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('memberManage.add.failed'))
  } finally {
    addSubmitting.value = false
  }
}

function copyPasswordResult() {
  void copyToClipboard(passwordResult.value).then((ok) => {
    if (ok) MessagePlugin.success(t('common.copySuccess'))
    else MessagePlugin.error(t('common.copyFailed'))
  })
}

// ---------- 重置密码 ----------
function onResetPassword(row: TenantMember) {
  if (row.user_id === selfUserId.value) {
    MessagePlugin.warning(t('memberManage.reset.selfForbidden'))
    return
  }
  const dialog = DialogPlugin.confirm({
    header: t('memberManage.reset.confirmTitle'),
    body: t('memberManage.reset.confirmBody', {
      name: row.username || row.employee_id,
      employeeId: row.employee_id,
    }),
    confirmBtn: { content: t('memberManage.reset.confirmTitle'), theme: 'danger' },
    cancelBtn: t('common.cancel'),
    onConfirm: async () => {
      try {
        const resp = await resetMemberPassword(tenantId.value, row.user_id)
        if (!resp.success || !resp.data) {
          MessagePlugin.error(resp.message || t('memberManage.reset.failed'))
          return
        }
        passwordResultTitle.value = t('memberManage.reset.resultTitle')
        passwordResult.value = resp.data.new_password
        passwordResultVisible.value = true
      } catch (err: any) {
        MessagePlugin.error(err?.message || t('memberManage.reset.failed'))
      } finally {
        dialog.destroy()
      }
    },
    onClose: () => dialog.destroy(),
  })
}

// ---------- 邀请（从未登录的成员） ----------
// 行内「邀请」与「成员已添加」弹窗的复制按钮共用同一段邀请文本：
// 项目名 + 一句话介绍 + 登录网址/工号/姓名/初始密码 + 修改密码提醒。
function buildInviteText(row: TenantMember): string {
  return [
    t('memberManage.invite.textTitle', { tenant: authStore.tenant?.name || '' }),
    t('memberManage.invite.textIntro'),
    t('memberManage.invite.textSite', { url: window.location.origin }),
    t('memberManage.invite.textEmployeeId', { employeeId: row.employee_id || row.user_id }),
    t('memberManage.invite.textName', { name: row.username || row.employee_id }),
    t('memberManage.invite.textPassword'),
    t('memberManage.invite.textFooter'),
  ].join('\n')
}

async function copyInviteText(row: TenantMember) {
  const ok = await copyToClipboard(buildInviteText(row))
  if (ok) MessagePlugin.success(t('memberManage.invite.copied'))
  else MessagePlugin.error(t('memberManage.invite.copyFailed'))
}

async function onInvite(row: TenantMember) {
  await copyInviteText(row)
}

// 「成员已添加」弹窗主按钮：复制新成员的邀请信息。
function copyAddInvite() {
  if (addResultMember.value) void copyInviteText(addResultMember.value)
}

// ---------- 统计 ----------
const statsVisible = ref(false)
const statsLoading = ref(false)
const statsRow = ref<TenantMember | null>(null)
const statsData = ref<MemberStats | null>(null)

async function onStats(row: TenantMember) {
  statsRow.value = row
  statsData.value = null
  statsVisible.value = true
  statsLoading.value = true
  try {
    const resp = await getMemberStats(tenantId.value, row.user_id)
    if (resp.success && resp.data) {
      statsData.value = resp.data
    } else {
      statsData.value = null
    }
  } catch {
    statsData.value = null
  } finally {
    statsLoading.value = false
  }
}

// ---------- 移出空间 ----------
function onRemove(row: TenantMember) {
  // 管理员 / 本人：只提示，不调接口（与后端 403 文案一致）。
  if (isProtected(row)) {
    MessagePlugin.warning(t('memberManage.remove.adminProtected'))
    return
  }
  const dialog = DialogPlugin.confirm({
    header: t('memberManage.remove.confirmTitle'),
    body: t('memberManage.remove.confirmBody', {
      name: row.username || row.employee_id,
      employeeId: row.employee_id,
    }),
    confirmBtn: { content: t('memberManage.remove.confirmTitle'), theme: 'danger' },
    cancelBtn: t('common.cancel'),
    onConfirm: async () => {
      try {
        const resp = await removeMember(tenantId.value, row.user_id)
        if (!resp.success) {
          MessagePlugin.error(resp.message || t('memberManage.remove.failed'))
          return
        }
        MessagePlugin.success(t('memberManage.remove.success'))
        void loadMembers()
      } catch (err: any) {
        MessagePlugin.error(err?.message || t('memberManage.remove.failed'))
      } finally {
        dialog.destroy()
      }
    },
    onClose: () => dialog.destroy(),
  })
}
</script>

<style lang="less" scoped>
// 弹窗高度固定（含描述/工具栏约 480px 内容区）：行数少时表格下方留白，
// 行数多时表格在 .member-manage-body 内部滚动，弹窗整体不伸缩。
.member-manage {
  display: flex;
  flex-direction: column;
  gap: 12px;
  height: 480px;
}

// 标题行：左标题 + 右侧「成员行为日志」链接（避开右上角关闭按钮）。
.member-manage-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-right: 16px;
}

.member-manage-desc {
  margin: 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--td-text-color-secondary);
}

.member-manage-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;

  .member-manage-search {
    flex: 1;
    max-width: 320px;
  }
}

// 加载/错误/空态：撑满剩余高度并垂直居中，与固定弹窗高度配合。
.member-manage-hint {
  display: flex;
  flex: 1;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 24px 0;
  color: var(--td-text-color-secondary);
}

// 名册区：吃掉剩余高度，超高时只在这里出纵向滚动。
.member-manage-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

.member-manage-never-login {
  color: var(--td-text-color-placeholder);
}

.member-manage-ops {
  display: flex;
  align-items: center;
  gap: 2px;
}

.member-manage-remove-btn {
  color: var(--td-error-color);

  &:hover {
    color: var(--td-error-color);
  }
}

.member-manage-pager {
  display: flex;
  justify-content: flex-end;
  padding-top: 4px;
}

// 「成员已添加」弹窗正文：成功一行 + 初始密码提示条。
.member-manage-add-result-line {
  margin: 0 0 12px;
  font-size: 14px;
  line-height: 1.6;
  color: var(--td-text-color-primary);
}

.member-manage-password-result {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;

  .member-manage-password-value {
    font-family: monospace;
    font-size: 20px;
    font-weight: 600;
    letter-spacing: 2px;
    color: var(--td-text-color-primary);
  }
}

.member-manage-password-hint {
  margin: 12px 0 0;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  text-align: center;
}

.member-manage-stats {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 100px;
}

.member-manage-stats-name {
  text-align: center;
  font-size: 14px;
  color: var(--td-text-color-primary);

  .member-manage-stats-id {
    color: var(--td-text-color-secondary);
  }
}

.member-manage-stats-grid {
  display: flex;
  gap: 12px;
}

.member-manage-stats-item {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 16px 0;
  border-radius: 8px;
  background: var(--td-bg-color-secondarycontainer);

  .member-manage-stats-num {
    font-size: 24px;
    font-weight: 600;
    color: var(--td-brand-color);
  }

  .member-manage-stats-label {
    font-size: 12px;
    color: var(--td-text-color-secondary);
  }
}
</style>
