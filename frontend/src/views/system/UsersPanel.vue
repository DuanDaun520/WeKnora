<template>
  <div class="panel-root">
    <div class="panel-header">
      <div>
        <h2>{{ $t('systemConsole.users.title') }}</h2>
        <p class="panel-header-desc">{{ $t('systemConsole.users.description') }}</p>
      </div>
      <div class="panel-header-actions">
        <t-button variant="outline" :loading="usersLoading" @click="reloadUsers">
          <template #icon><t-icon name="refresh" /></template>
          {{ $t('systemConsole.users.refresh') }}
        </t-button>
        <t-button theme="primary" @click="openCreateDialog">
          <template #icon><t-icon name="add" /></template>
          {{ $t('systemConsole.users.create') }}
        </t-button>
      </div>
    </div>

    <div class="console-toolbar">
      <t-input
        v-model="userFilters.query"
        class="toolbar-search"
        clearable
        :placeholder="$t('systemConsole.users.searchPlaceholder')"
        @enter="reloadUsers"
        @clear="reloadUsers"
      >
        <template #prefix-icon><t-icon name="search" /></template>
      </t-input>
      <t-select
        v-model="userFilters.status"
        class="toolbar-select"
        :placeholder="$t('systemConsole.users.statusAll')"
        clearable
        @change="reloadUsers"
      >
        <t-option value="active" :label="$t('systemConsole.users.statusActive')" />
        <t-option value="disabled" :label="$t('systemConsole.users.statusDisabled')" />
      </t-select>
      <t-select
        v-model="userFilters.tenantId"
        class="toolbar-select"
        :placeholder="$t('systemConsole.users.tenantAll')"
        clearable
        @change="reloadUsers"
      >
        <t-option v-for="tn in tenants" :key="tn.id" :value="tn.id" :label="tn.name" />
      </t-select>
    </div>

    <div v-if="usersError" class="console-inline-error">
      <t-alert theme="error" :message="usersError">
        <template #operation>
          <t-button size="small" @click="reloadUsers">{{ $t('systemConsole.retry') }}</t-button>
        </template>
      </t-alert>
    </div>

    <div class="data-table-shell console-table-shell">
      <t-table
        row-key="id"
        :data="users"
        :columns="userColumns"
        :loading="usersLoading"
        hover
        stripe
        size="medium"
      >
        <template #username="{ row }">
          <div class="cell-person">
            <span class="cell-person-name">{{ row.username }}</span>
            <t-tag v-if="row.is_system_admin" size="small" theme="warning" variant="light">
              {{ $t('systemConsole.users.sysAdmin') }}
            </t-tag>
          </div>
        </template>
        <template #email="{ row }">
          <span class="cell-muted">{{ row.email || '—' }}</span>
        </template>
        <template #state="{ row }">
          <t-tag :theme="row.is_active ? 'success' : 'danger'" size="small" variant="light">
            {{ row.is_active ? $t('systemConsole.users.statusActive') : $t('systemConsole.users.statusDisabled') }}
          </t-tag>
        </template>
        <template #bindings="{ row }">
          <div class="cell-bindings">
            <t-tag
              v-for="b in row.bindings"
              :key="b.tenant_id"
              size="small"
              :theme="isWorkspaceAdminRole(b.role) ? 'warning' : 'primary'"
              variant="light"
            >
              {{ b.tenant_name || `#${b.tenant_id}` }}
              <span v-if="isWorkspaceAdminRole(b.role)" class="binding-role-suffix">
                · {{ $t('systemConsole.members.roleAdmin') }}
              </span>
            </t-tag>
            <span v-if="!row.bindings.length" class="cell-muted">—</span>
          </div>
        </template>
        <template #created_at="{ row }">{{ formatDate(row.created_at) }}</template>
        <template #actions="{ row }">
          <div class="cell-actions">
            <t-button variant="text" size="small" @click="openBindingsDialog(row)">
              {{ $t('systemConsole.users.bindings') }}
            </t-button>
            <t-button variant="text" size="small" @click="openEditDialog(row)">
              {{ $t('systemConsole.users.edit') }}
            </t-button>
            <t-button
              variant="text"
              size="small"
              :disabled="row.id === authStore.currentUserId"
              @click="openResetDialog(row)"
            >
              {{ $t('systemConsole.users.resetPassword') }}
            </t-button>
            <t-dropdown
              :options="rowActionOptions(row)"
              trigger="click"
              @click="(item: any) => onRowAction(String(item.value), row)"
            >
              <t-button variant="text" size="small" shape="square">
                <template #icon><t-icon name="more" /></template>
              </t-button>
            </t-dropdown>
          </div>
        </template>
      </t-table>
      <div v-if="usersTotal > 0" class="data-table-shell__pager">
        <t-pagination
          v-model="usersPage"
          v-model:page-size="usersPageSize"
          :total="usersTotal"
          size="small"
          show-jumper
          show-page-number
          show-page-size
          @change="loadUsers"
        />
      </div>
    </div>

    <!-- 开户弹层 -->
    <t-dialog
      v-model:visible="createVisible"
      :header="$t('systemConsole.create.title')"
      :confirm-btn="{ content: $t('systemConsole.create.submit'), theme: 'primary', loading: createLoading }"
      :cancel-btn="$t('common.cancel')"
      :close-on-overlay-click="false"
      width="560px"
      @confirm="submitCreate"
      @close="resetCreateForm"
    >
      <div v-if="createResult" class="create-result">
        <t-alert theme="success" :title="$t('systemConsole.create.successTitle')" />
        <div v-if="createResult.generated_password" class="password-reveal">
          <div class="password-reveal-label">
            <t-icon name="lock-on" size="15px" />
            {{ $t('systemConsole.create.passwordOnce') }}
          </div>
          <div class="password-reveal-row">
            <code class="password-reveal-value">{{ createResult.generated_password }}</code>
            <t-button size="small" variant="outline" @click="copyText(createResult!.generated_password!)">
              {{ $t('systemConsole.create.copy') }}
            </t-button>
          </div>
        </div>
        <div v-if="createResult.bindings?.length" class="bind-results">
          <div
            v-for="b in createResult.bindings"
            :key="b.tenant_id"
            :class="['bind-result-row', b.ok ? 'ok' : 'fail']"
          >
            <t-icon :name="b.ok ? 'check-circle' : 'error-circle'" size="15px" />
            <span>{{ tenantNameOf(b.tenant_id) }}</span>
            <span v-if="!b.ok" class="bind-result-error">{{ $t('systemConsole.create.bindFailed') }}</span>
          </div>
        </div>
      </div>
      <t-form v-else :data="createForm" label-align="top" @submit.prevent>
        <t-form-item :label="$t('systemConsole.create.employeeId')" :help="$t('systemConsole.create.employeeIdHelp')">
          <t-input v-model="createForm.employee_id" :placeholder="$t('systemConsole.create.employeeIdPlaceholder')" />
        </t-form-item>
        <t-form-item :label="$t('systemConsole.create.username')">
          <t-input v-model="createForm.username" :placeholder="$t('systemConsole.create.usernamePlaceholder')" />
        </t-form-item>
        <t-form-item :label="$t('systemConsole.create.email')">
          <t-input v-model="createForm.email" :placeholder="$t('systemConsole.create.emailPlaceholder')" />
        </t-form-item>
        <t-form-item :label="$t('systemConsole.create.password')">
          <div class="password-field">
            <t-input
              v-model="createForm.password"
              type="password"
              :placeholder="$t('systemConsole.create.passwordPlaceholder')"
            />
            <t-button variant="outline" @click="createForm.password = generatePassword()">
              {{ $t('systemConsole.create.generate') }}
            </t-button>
          </div>
        </t-form-item>
        <t-form-item :label="$t('systemConsole.create.tenants')">
          <t-select
            v-model="createForm.tenant_ids"
            multiple
            clearable
            :placeholder="$t('systemConsole.create.tenantsPlaceholder')"
          >
            <t-option v-for="tn in tenants" :key="tn.id" :value="tn.id" :label="tn.name" />
          </t-select>
        </t-form-item>
      </t-form>
    </t-dialog>

    <!-- 编辑资料 -->
    <t-dialog
      v-model:visible="editVisible"
      :header="$t('systemConsole.edit.title', { name: editForm.username })"
      :confirm-btn="{ content: t('common.confirm'), theme: 'primary', loading: editLoading }"
      :cancel-btn="$t('common.cancel')"
      width="480px"
      @confirm="submitEdit"
    >
      <t-form :data="editForm" label-align="top" @submit.prevent>
        <t-form-item :label="$t('systemConsole.edit.employeeId')">
          <t-input :value="editForm.employee_id" disabled />
        </t-form-item>
        <t-form-item :label="$t('systemConsole.create.username')">
          <t-input v-model="editForm.username" />
        </t-form-item>
        <t-form-item :label="$t('systemConsole.create.email')">
          <t-input v-model="editForm.email" :placeholder="$t('systemConsole.create.emailPlaceholder')" />
        </t-form-item>
      </t-form>
    </t-dialog>

    <!-- 重置密码 -->
    <t-dialog
      v-model:visible="resetVisible"
      :header="$t('systemConsole.reset.title', { name: resetTarget?.username || resetTarget?.employee_id })"
      :confirm-btn="{ content: t('systemConsole.reset.submit'), theme: 'primary', loading: resetLoading }"
      :cancel-btn="$t('common.cancel')"
      width="480px"
      @confirm="submitReset"
      @close="resetPasswordValue = ''"
    >
      <p class="dialog-hint">{{ $t('systemConsole.reset.hint') }}</p>
      <t-form-item :label="$t('systemConsole.reset.newPassword')">
        <div class="password-field">
          <t-input v-model="resetPasswordValue" type="password" />
          <t-button variant="outline" @click="resetPasswordValue = generatePassword()">
            {{ $t('systemConsole.create.generate') }}
          </t-button>
        </div>
      </t-form-item>
    </t-dialog>

    <!-- 绑定空间 -->
    <t-dialog
      v-model:visible="bindingsVisible"
      :header="$t('systemConsole.bindings.title', { name: bindingsTarget?.username || bindingsTarget?.employee_id })"
      :footer="false"
      width="560px"
      @close="bindingsTarget = null"
    >
      <template v-if="bindingsTarget">
        <div v-if="bindingsList.length === 0" class="bindings-empty">
          <t-empty :description="$t('systemConsole.bindings.empty')" />
        </div>
        <div v-else class="bindings-list">
          <div v-for="b in bindingsList" :key="b.tenant_id" class="binding-row">
            <t-icon name="system-sum" size="16px" />
            <span class="binding-name">{{ b.tenant_name || `#${b.tenant_id}` }}</span>
            <t-tag
              size="small"
              :theme="isWorkspaceAdminRole(b.role) ? 'warning' : 'default'"
              variant="light"
            >
              {{ workspaceRoleLabel(b.role) }}
            </t-tag>
            <t-button
              variant="text"
              size="small"
              theme="danger"
              :loading="bindingBusy === b.tenant_id"
              @click="removeBinding(b)"
            >
              {{ $t('systemConsole.bindings.remove') }}
            </t-button>
          </div>
        </div>
        <div class="bindings-add">
          <t-select
            v-model="bindingCandidate"
            class="bindings-add-select"
            :placeholder="$t('systemConsole.bindings.addPlaceholder')"
          >
            <t-option
              v-for="tn in bindableTenants"
              :key="tn.id"
              :value="tn.id"
              :label="tn.name"
            />
          </t-select>
          <t-select
            v-model="bindingRole"
            class="bindings-role-select"
            :placeholder="$t('systemConsole.bindings.rolePlaceholder')"
          >
            <t-option value="contributor" :label="$t('systemConsole.members.roleMember')" />
            <t-option value="admin" :label="$t('systemConsole.members.roleAdmin')" />
          </t-select>
          <t-button
            theme="primary"
            variant="outline"
            :disabled="!bindingCandidate"
            :loading="bindingBusy === 'add'"
            @click="addBinding"
          >
            {{ $t('systemConsole.bindings.add') }}
          </t-button>
        </div>
      </template>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
// 用户管理面板：从原 SystemConsole 的「用户管理」页签整体迁出
//（000094 控制台左侧菜单化改造）。开户 / 编辑 / 重置密码 / 绑定空间
// 四个弹层随之归入本面板；空间列表仅用于筛选与绑定候选，独立加载。
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import type { PrimaryTableCol } from 'tdesign-vue-next'
import { useAuthStore } from '@/stores/auth'
import {
  bindUserToTenant,
  createEnterpriseUser,
  listEnterpriseUsers,
  listPlatformTenants,
  listUserBindings,
  promoteUserToSystemAdmin,
  resetEnterpriseUserPassword,
  revokeSystemAdmin,
  unbindUserFromTenant,
  updateEnterpriseUser,
  type CreateEnterpriseUserResponse,
  type EnterpriseBinding,
  type EnterpriseUserRow,
  type EnterpriseWorkspaceRole,
  type PlatformTenant,
} from '@/api/system'

const { t } = useI18n()
const authStore = useAuthStore()

const USERS_PAGE_SIZE = 20

// ---- 用户列表 ----
const users = ref<EnterpriseUserRow[]>([])
const usersTotal = ref(0)
const usersPage = ref(1)
const usersPageSize = ref(USERS_PAGE_SIZE)
const usersLoading = ref(false)
const usersError = ref('')
const userFilters = reactive({ query: '', status: '' as '' | 'active' | 'disabled', tenantId: null as number | null })

const userColumns = computed<PrimaryTableCol<EnterpriseUserRow>[]>(() => [
  { colKey: 'employee_id', title: t('systemConsole.users.colEmployeeId'), width: 130, ellipsis: true },
  { colKey: 'username', title: t('systemConsole.users.colUsername'), width: 170 },
  { colKey: 'email', title: t('systemConsole.users.colEmail'), minWidth: 180, ellipsis: true, cell: 'email' } as PrimaryTableCol<EnterpriseUserRow>,
  { colKey: 'state', title: t('systemConsole.users.colState'), width: 90, cell: 'state' } as PrimaryTableCol<EnterpriseUserRow>,
  { colKey: 'bindings', title: t('systemConsole.users.colBindings'), cell: 'bindings' } as PrimaryTableCol<EnterpriseUserRow>,
  { colKey: 'created_at', title: t('systemConsole.users.colCreatedAt'), width: 120, cell: 'created_at' } as PrimaryTableCol<EnterpriseUserRow>,
  { colKey: 'actions', title: t('systemConsole.users.colActions'), width: 300, align: 'right', cell: 'actions' } as PrimaryTableCol<EnterpriseUserRow>,
])

async function loadUsers() {
  usersLoading.value = true
  usersError.value = ''
  try {
    const resp = await listEnterpriseUsers({
      query: userFilters.query.trim() || undefined,
      tenant_id: userFilters.tenantId ?? undefined,
      is_active: userFilters.status === '' ? undefined : userFilters.status === 'active',
      offset: (usersPage.value - 1) * usersPageSize.value,
      limit: usersPageSize.value,
    })
    users.value = resp.users || []
    usersTotal.value = resp.total || 0
  } catch (e: any) {
    usersError.value = e?.message || t('systemConsole.messages.loadFailed')
  } finally {
    usersLoading.value = false
  }
}

function reloadUsers() {
  usersPage.value = 1
  void loadUsers()
}

// ---- 空间列表（筛选 / 绑定候选） ----
const tenants = ref<PlatformTenant[]>([])

async function loadTenants() {
  try {
    const resp = await listPlatformTenants()
    tenants.value = resp.tenants || []
  } catch {
    // 筛选下拉退化为空即可，不打断用户列表
  }
}

function tenantNameOf(id: number) {
  return tenants.value.find((tn) => tn.id === id)?.name || `#${id}`
}

// ---- 两级空间角色辅助 ----
// 'owner' 仅存在于历史数据（000092 之前），展示时视同空间管理员。
function isWorkspaceAdminRole(role: string) {
  return role === 'admin' || role === 'owner'
}

function workspaceRoleLabel(role: string) {
  return isWorkspaceAdminRole(role)
    ? t('systemConsole.members.roleAdmin')
    : t('systemConsole.members.roleMember')
}

// ---- 开户 ----
const createVisible = ref(false)
const createLoading = ref(false)
const createResult = ref<CreateEnterpriseUserResponse | null>(null)
const createForm = reactive({
  employee_id: '',
  username: '',
  email: '',
  password: '',
  tenant_ids: [] as number[],
})

function openCreateDialog() {
  resetCreateForm()
  createVisible.value = true
}

function resetCreateForm() {
  createResult.value = null
  createForm.employee_id = ''
  createForm.username = ''
  createForm.email = ''
  createForm.password = ''
  createForm.tenant_ids = []
}

async function submitCreate() {
  createForm.employee_id = createForm.employee_id.trim()
  createForm.username = createForm.username.trim()
  createForm.email = createForm.email.trim()
  if (!createForm.employee_id || !createForm.username) {
    MessagePlugin.warning(t('systemConsole.create.missingRequired'))
    return
  }
  createLoading.value = true
  try {
    const resp = await createEnterpriseUser({
      employee_id: createForm.employee_id,
      username: createForm.username,
      email: createForm.email || undefined,
      password: createForm.password || undefined,
      tenant_ids: createForm.tenant_ids,
    })
    createResult.value = resp
    void loadUsers()
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('systemConsole.messages.opFailed'))
  } finally {
    createLoading.value = false
  }
}

// ---- 编辑资料 ----
const editVisible = ref(false)
const editLoading = ref(false)
const editTargetId = ref('')
const editForm = reactive({ employee_id: '', username: '', email: '' })

function openEditDialog(row: EnterpriseUserRow) {
  editTargetId.value = row.id
  editForm.employee_id = row.employee_id
  editForm.username = row.username
  editForm.email = row.email || ''
  editVisible.value = true
}

async function submitEdit() {
  editForm.username = editForm.username.trim()
  editForm.email = editForm.email.trim()
  if (!editForm.username) {
    MessagePlugin.warning(t('systemConsole.create.missingRequired'))
    return
  }
  editLoading.value = true
  try {
    await updateEnterpriseUser(editTargetId.value, {
      username: editForm.username,
      email: editForm.email,
    })
    MessagePlugin.success(t('systemConsole.messages.opSuccess'))
    editVisible.value = false
    void loadUsers()
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('systemConsole.messages.opFailed'))
  } finally {
    editLoading.value = false
  }
}

// ---- 重置密码 ----
const resetVisible = ref(false)
const resetLoading = ref(false)
const resetTarget = ref<EnterpriseUserRow | null>(null)
const resetPasswordValue = ref('')

function openResetDialog(row: EnterpriseUserRow) {
  resetTarget.value = row
  resetPasswordValue.value = ''
  resetVisible.value = true
}

async function submitReset() {
  if (!resetTarget.value) return
  if (!resetPasswordValue.value) {
    MessagePlugin.warning(t('systemConsole.reset.required'))
    return
  }
  resetLoading.value = true
  try {
    await resetEnterpriseUserPassword(resetTarget.value.id, resetPasswordValue.value)
    MessagePlugin.success(t('systemConsole.reset.success'))
    resetVisible.value = false
    resetPasswordValue.value = ''
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('systemConsole.messages.opFailed'))
  } finally {
    resetLoading.value = false
  }
}

// ---- 行操作：启停 / 系统管理员 ----
function rowActionOptions(row: EnterpriseUserRow) {
  const options: Array<{ value: string; content: string; theme?: 'default' | 'error' }> = []
  options.push(
    row.is_active
      ? { value: 'disable', content: t('systemConsole.users.disable'), theme: 'error' }
      : { value: 'enable', content: t('systemConsole.users.enable') },
  )
  if (row.is_system_admin) {
    options.push({ value: 'revoke', content: t('systemConsole.users.revoke'), theme: 'error' })
  } else {
    options.push({ value: 'promote', content: t('systemConsole.users.promote') })
  }
  return options
}

async function onRowAction(action: string, row: EnterpriseUserRow) {
  const self = row.id === authStore.currentUserId
  if (action === 'disable') {
    if (self) return
    const ok = await dialogConfirm(t('systemConsole.users.disableConfirm', { name: row.username }))
    if (!ok) return
    await mutateUser(() => updateEnterpriseUser(row.id, { is_active: false }))
  } else if (action === 'enable') {
    await mutateUser(() => updateEnterpriseUser(row.id, { is_active: true }))
  } else if (action === 'promote') {
    const ok = await dialogConfirm(t('systemConsole.users.promoteConfirm', { name: row.username }))
    if (!ok) return
    await mutateUser(() => promoteUserToSystemAdmin({ user_id: row.id }))
  } else if (action === 'revoke') {
    const ok = await dialogConfirm(t('systemConsole.users.revokeConfirm', { name: row.username }))
    if (!ok) return
    await mutateUser(() => revokeSystemAdmin(row.id))
  }
}

async function mutateUser(fn: () => Promise<unknown>) {
  try {
    await fn()
    MessagePlugin.success(t('systemConsole.messages.opSuccess'))
    void loadUsers()
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('systemConsole.messages.opFailed'))
  }
}

// dialogConfirm 走原生确认框：操作频率低，不值得为它引入全局抽屉组件。
function dialogConfirm(message: string): Promise<boolean> {
  return new Promise((resolve) => {
    resolve(window.confirm(message))
  })
}

// ---- 绑定空间 ----
const bindingsVisible = ref(false)
const bindingsTarget = ref<EnterpriseUserRow | null>(null)
const bindingsList = ref<EnterpriseBinding[]>([])
const bindingCandidate = ref<number | null>(null)
const bindingRole = ref<EnterpriseWorkspaceRole>('contributor')
const bindingBusy = ref<number | 'add' | null>(null)

const bindableTenants = computed(() =>
  tenants.value.filter(
    (tn) => !bindingsList.value.some((b) => b.tenant_id === tn.id),
  ),
)

function openBindingsDialog(row: EnterpriseUserRow) {
  bindingsTarget.value = row
  bindingsList.value = row.bindings ? [...row.bindings] : []
  bindingCandidate.value = null
  bindingRole.value = 'contributor'
  bindingsVisible.value = true
}

async function refreshBindings() {
  if (!bindingsTarget.value) return
  try {
    const resp = await listUserBindings(bindingsTarget.value.id)
    bindingsList.value = resp.bindings || []
  } catch {
    // best-effort；下一次打开会重新拉取
  }
}

async function addBinding() {
  if (!bindingsTarget.value || !bindingCandidate.value) return
  bindingBusy.value = 'add'
  try {
    await bindUserToTenant(bindingsTarget.value.id, bindingCandidate.value, bindingRole.value)
    MessagePlugin.success(t('systemConsole.messages.opSuccess'))
    bindingCandidate.value = null
    bindingRole.value = 'contributor'
    await refreshBindings()
    void loadUsers()
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('systemConsole.messages.opFailed'))
  } finally {
    bindingBusy.value = null
  }
}

async function removeBinding(b: EnterpriseBinding) {
  if (!bindingsTarget.value) return
  const ok = await dialogConfirm(
    t('systemConsole.bindings.removeConfirm', { name: bindingsTarget.value.username, tenant: b.tenant_name || `#${b.tenant_id}` }),
  )
  if (!ok) return
  bindingBusy.value = b.tenant_id
  try {
    await unbindUserFromTenant(bindingsTarget.value.id, b.tenant_id)
    MessagePlugin.success(t('systemConsole.messages.opSuccess'))
    await refreshBindings()
    void loadUsers()
  } catch (e: any) {
    MessagePlugin.error(lastAdminAwareError(e))
  } finally {
    bindingBusy.value = null
  }
}

// 后端对"最后一名空间管理员"的移除返回 409，本地化后再提示。
function lastAdminAwareError(e: any): string {
  if (e?.status === 409 || /last workspace admin/i.test(String(e?.message || ''))) {
    return t('systemConsole.members.lastAdminError')
  }
  return e?.message || t('systemConsole.messages.opFailed')
}

// ---- 工具 ----
function generatePassword(): string {
  // 覆盖复杂密码策略的四类字符；crypto.getRandomValues 避免可预测的
  // Math.random 序列（密码虽然通常由管理员现场转达，仍不弱化熵）。
  const classes = [
    'ABCDEFGHJKLMNPQRSTUVWXYZ',
    'abcdefghijkmnopqrstuvwxyz',
    '23456789',
    '!@#$%^&*_-+=',
  ]
  const all = classes.join('')
  const bytes = new Uint32Array(16)
  crypto.getRandomValues(bytes)
  const chars = classes.map((set, i) => set[bytes[i] % set.length])
  for (let i = classes.length; i < bytes.length; i++) {
    chars.push(all[bytes[i] % all.length])
  }
  // 简单洗牌，避免前四位固定为四类各一的可预测模式。
  for (let i = chars.length - 1; i > 0; i--) {
    const j = bytes[i] % (i + 1)
    ;[chars[i], chars[j]] = [chars[j], chars[i]]
  }
  return chars.join('')
}

async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    MessagePlugin.success(t('systemConsole.create.copied'))
  } catch {
    MessagePlugin.warning(text)
  }
}

function formatDate(value: string): string {
  if (!value) return '—'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  return d.toLocaleDateString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit' })
}

onMounted(() => {
  void loadUsers()
  void loadTenants()
})
</script>

<style lang="less" scoped>
@import './consolePanel.less';

.cell-person {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.cell-person-name {
  font-weight: 500;
  color: var(--td-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cell-muted {
  color: var(--td-text-color-placeholder);
}

.cell-bindings {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
}

.cell-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 2px;
}

.console-inline-error {
  padding: 12px 0;
}

// 开户结果面板
.create-result {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.password-reveal-label {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--td-text-color-secondary);
  margin-bottom: 8px;
}

.password-reveal-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.password-reveal-value {
  flex: 1;
  padding: 10px 14px;
  border-radius: 8px;
  border: 1px dashed var(--td-brand-color);
  background: var(--td-brand-color-light);
  font-size: 15px;
  letter-spacing: 0.5px;
  word-break: break-all;
  user-select: all;
}

.bind-results {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.bind-result-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;

  &.ok {
    color: var(--td-success-color);
  }

  &.fail {
    color: var(--td-error-color);
  }
}

.bind-result-error {
  color: var(--td-error-color);
}

.password-field {
  display: flex;
  gap: 8px;
  width: 100%;

  .t-input {
    flex: 1;
  }
}

.dialog-hint {
  margin: 0 0 14px;
  font-size: 13px;
  line-height: 1.6;
  color: var(--td-text-color-secondary);
}

// 绑定空间
.bindings-list {
  display: flex;
  flex-direction: column;
  margin-bottom: 16px;
}

.binding-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 4px;
  border-bottom: 1px solid var(--td-component-stroke);

  .binding-name {
    flex: 1;
    font-weight: 500;
    color: var(--td-text-color-primary);
  }
}

.bindings-add {
  display: flex;
  gap: 10px;

  .bindings-add-select {
    flex: 1;
  }

  .bindings-role-select {
    width: 140px;
    flex: none;
  }
}

.binding-role-suffix {
  margin-left: 2px;
  opacity: 0.85;
}
</style>
