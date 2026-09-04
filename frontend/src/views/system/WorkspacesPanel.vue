<template>
  <div class="panel-root">
    <div class="panel-header">
      <div>
        <h2>{{ $t('systemConsole.tenants.title') }}</h2>
        <p class="panel-header-desc">{{ $t('systemConsole.tenants.description') }}</p>
      </div>
      <div class="panel-header-actions">
        <t-button variant="outline" :loading="tenantsLoading" @click="loadTenants">
          <template #icon><t-icon name="refresh" /></template>
          {{ $t('systemConsole.users.refresh') }}
        </t-button>
        <t-button theme="primary" @click="tenantCreateVisible = true">
          <template #icon><t-icon name="add" /></template>
          {{ $t('systemConsole.tenants.create') }}
        </t-button>
      </div>
    </div>

    <div class="console-toolbar">
      <span class="toolbar-hint">{{ $t('systemConsole.tenants.hint') }}</span>
    </div>

    <div class="data-table-shell console-table-shell">
      <t-table
        row-key="id"
        :data="tenants"
        :columns="tenantColumns"
        :loading="tenantsLoading"
        hover
        stripe
        size="medium"
      >
        <template #name="{ row }">
          <div class="cell-person">
            <span class="cell-person-name">{{ row.name }}</span>
            <t-tag v-if="isHomeTenant(row)" size="small" theme="success" variant="light">
              {{ $t('systemConsole.tenants.homeBadge') }}
            </t-tag>
          </div>
        </template>
        <template #description="{ row }">
          <span class="cell-muted">{{ row.description || '—' }}</span>
        </template>
        <template #quota="{ row }">{{ formatQuota(row) }}</template>
        <template #created_at="{ row }">{{ formatDate(row.created_at) }}</template>
        <template #tenantActions="{ row }">
          <div class="cell-actions">
            <t-button variant="text" size="small" @click="openMembersDialog(row)">
              <template #icon><t-icon name="usergroup" /></template>
              {{ $t('systemConsole.members.open') }}
            </t-button>
            <t-button variant="text" size="small" @click="openAssignmentsDialog(row)">
              <template #icon><t-icon name="components" /></template>
              {{ $t('systemConsole.assignments.open') }}
            </t-button>
          </div>
        </template>
      </t-table>
    </div>

    <!-- 空间成员管理：指定空间管理员 -->
    <t-dialog
      v-model:visible="membersVisible"
      :header="$t('systemConsole.members.title', { name: membersTenant?.name || '' })"
      :footer="false"
      width="680px"
      @close="membersTenant = null"
    >
      <template v-if="membersTenant">
        <p class="dialog-hint">{{ $t('systemConsole.members.hint') }}</p>
        <div class="members-toolbar">
          <t-input
            v-model="membersQuery"
            class="members-search"
            clearable
            :placeholder="$t('systemConsole.members.searchPlaceholder')"
            @enter="reloadMembers"
            @clear="reloadMembers"
          >
            <template #prefix-icon><t-icon name="search" /></template>
          </t-input>
          <t-button variant="outline" :loading="membersLoading" @click="reloadMembers">
            <template #icon><t-icon name="refresh" /></template>
            {{ $t('systemConsole.users.refresh') }}
          </t-button>
        </div>
        <div class="members-table">
          <t-table
            row-key="user_id"
            :data="membersList"
            :columns="memberColumns"
            :loading="membersLoading"
            hover
            size="small"
          >
            <template #member="{ row }">
              <div class="cell-person">
                <span class="cell-person-name">{{ row.username || row.user_id }}</span>
                <span class="cell-muted">{{ row.employee_id }}</span>
              </div>
            </template>
            <template #role="{ row }">
              <t-tag
                size="small"
                :theme="isWorkspaceAdminRole(row.role) ? 'warning' : 'default'"
                variant="light"
              >
                {{ workspaceRoleLabel(row.role) }}
              </t-tag>
            </template>
            <template #joined_at="{ row }">{{ formatDate(row.joined_at) }}</template>
            <template #memberActions="{ row }">
              <div class="cell-actions">
                <t-button
                  v-if="!isWorkspaceAdminRole(row.role)"
                  variant="text"
                  size="small"
                  :loading="memberBusy === row.user_id"
                  @click="setMemberRole(row, 'admin')"
                >
                  {{ $t('systemConsole.members.setAdmin') }}
                </t-button>
                <t-button
                  v-else
                  variant="text"
                  size="small"
                  :loading="memberBusy === row.user_id"
                  @click="setMemberRole(row, 'contributor')"
                >
                  {{ $t('systemConsole.members.setMember') }}
                </t-button>
                <t-button
                  variant="text"
                  size="small"
                  theme="danger"
                  :loading="memberBusy === row.user_id"
                  @click="removeMember(row)"
                >
                  {{ $t('systemConsole.members.remove') }}
                </t-button>
              </div>
            </template>
          </t-table>
          <div v-if="membersTotal > membersPageSize" class="data-table-shell__pager">
            <t-pagination
              v-model="membersPage"
              v-model:page-size="membersPageSize"
              :total="membersTotal"
              size="small"
              layout="prev, pager, next"
              @change="loadMembers"
            />
          </div>
        </div>
      </template>
    </t-dialog>

    <!-- 模型分配：把平台目录中的模型分配给该空间 -->
    <t-dialog
      v-model:visible="assignmentsVisible"
      :header="$t('systemConsole.assignments.title', { name: assignmentsTenant?.name || '' })"
      :confirm-btn="{ content: t('common.confirm'), theme: 'primary', loading: assignmentsSaving }"
      :cancel-btn="$t('common.cancel')"
      width="640px"
      @confirm="submitAssignments"
      @close="assignmentsTenant = null"
    >
      <template v-if="assignmentsTenant">
        <p class="dialog-hint">{{ $t('systemConsole.assignments.hint') }}</p>
        <div v-if="assignmentsLoading" class="assignments-loading">
          <t-loading />
        </div>
        <template v-else>
          <div v-for="group in assignmentGroups" :key="group.key" class="assignment-group">
            <div class="assignment-group-head">
              <span class="assignment-group-title">
                <t-icon :name="group.icon" size="15px" />
                {{ group.label }}
              </span>
              <t-checkbox
                :checked="groupSelected(group)"
                :indeterminate="groupIndeterminate(group)"
                :disabled="group.models.length === 0"
                @change="(v: any) => toggleGroup(group, !!v)"
              >
                {{ $t('systemConsole.assignments.selectAll') }}
              </t-checkbox>
            </div>
            <div v-if="group.models.length === 0" class="assignment-empty">
              {{ $t('systemConsole.assignments.noModelsOfType') }}
            </div>
            <div v-else class="assignment-list">
              <t-checkbox
                v-for="m in group.models"
                :key="m.id"
                :checked="selectedModelIds.includes(m.id)"
                @change="() => toggleModel(m.id)"
              >
                <span class="assignment-model-name">{{ m.display_name || m.name }}</span>
                <span class="assignment-model-sub">{{ m.name }}</span>
              </t-checkbox>
            </div>
          </div>
        </template>
      </template>
    </t-dialog>

    <!-- 新建空间 -->
    <t-dialog
      v-model:visible="tenantCreateVisible"
      :header="$t('systemConsole.tenants.create')"
      :confirm-btn="{ content: t('systemConsole.tenants.create'), theme: 'primary', loading: tenantCreateLoading }"
      :cancel-btn="$t('common.cancel')"
      width="520px"
      @confirm="submitTenantCreate"
      @close="resetTenantCreateForm"
    >
      <t-form :data="tenantCreateForm" label-align="top" @submit.prevent>
        <t-form-item :label="$t('systemConsole.tenants.name')">
          <t-input v-model="tenantCreateForm.name" :placeholder="$t('systemConsole.tenants.namePlaceholder')" />
        </t-form-item>
        <t-form-item :label="$t('systemConsole.tenants.description')">
          <t-textarea
            v-model="tenantCreateForm.description"
            :placeholder="$t('systemConsole.tenants.descriptionPlaceholder')"
            :autosize="{ minRows: 2, maxRows: 4 }"
          />
        </t-form-item>
        <t-form-item :label="$t('systemConsole.tenants.quota')">
          <t-input-number
            v-model="tenantCreateForm.storage_quota_gb"
            :min="1"
            :placeholder="$t('systemConsole.tenants.quotaPlaceholder')"
            style="width: 100%"
          />
        </t-form-item>
      </t-form>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
// 工作空间面板：从原 SystemConsole 的「工作空间」页签整体迁出
//（000094 控制台左侧菜单化改造），并新增「模型分配」弹层 —— 系统
// 管理员把平台模型目录中的模型按类型勾选分配给空间，空间成员的
// 模型选择器随后只能看到已分配 + 内置模型。
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import type { PrimaryTableCol } from 'tdesign-vue-next'
import { useAuthStore } from '@/stores/auth'
import {
  createPlatformTenant,
  listPlatformTenants,
  listTenantModelAssignments,
  listWorkspaceMembers,
  unbindUserFromTenant,
  updateTenantModelAssignments,
  updateWorkspaceMemberRole,
  type EnterpriseWorkspaceMember,
  type EnterpriseWorkspaceRole,
  type PlatformTenant,
} from '@/api/system'
import { listSystemModels, type ModelConfig } from '@/api/model'

const { t } = useI18n()
const authStore = useAuthStore()

// ---- 空间列表 ----
const tenants = ref<PlatformTenant[]>([])
const tenantsLoading = ref(false)

const tenantColumns = computed<PrimaryTableCol<PlatformTenant>[]>(() => [
  { colKey: 'name', title: t('systemConsole.tenants.colName'), width: 220 },
  { colKey: 'description', title: t('systemConsole.tenants.colDescription'), cell: 'description' } as PrimaryTableCol<PlatformTenant>,
  { colKey: 'quota', title: t('systemConsole.tenants.colQuota'), width: 150, cell: 'quota' } as PrimaryTableCol<PlatformTenant>,
  { colKey: 'created_at', title: t('systemConsole.users.colCreatedAt'), width: 130, cell: 'created_at' } as PrimaryTableCol<PlatformTenant>,
  { colKey: 'actions', title: t('systemConsole.users.colActions'), width: 230, align: 'right', cell: 'tenantActions' } as PrimaryTableCol<PlatformTenant>,
])

async function loadTenants() {
  tenantsLoading.value = true
  try {
    const resp = await listPlatformTenants()
    tenants.value = resp.tenants || []
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('systemConsole.messages.loadFailed'))
  } finally {
    tenantsLoading.value = false
  }
}

function isHomeTenant(row: PlatformTenant) {
  return authStore.user != null && Number(authStore.user.tenant_id) === row.id
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

// dialogConfirm 走原生确认框：操作频率低，不值得为它引入全局抽屉组件。
function dialogConfirm(message: string): Promise<boolean> {
  return new Promise((resolve) => {
    resolve(window.confirm(message))
  })
}

// ---- 空间成员管理（指定空间管理员） ----
const MEMBERS_PAGE_SIZE = 20
const membersVisible = ref(false)
const membersTenant = ref<PlatformTenant | null>(null)
const membersList = ref<EnterpriseWorkspaceMember[]>([])
const membersTotal = ref(0)
const membersPage = ref(1)
const membersPageSize = ref(MEMBERS_PAGE_SIZE)
const membersLoading = ref(false)
const membersQuery = ref('')
const memberBusy = ref<string | null>(null)

const memberColumns = computed<PrimaryTableCol<EnterpriseWorkspaceMember>[]>(() => [
  { colKey: 'member', title: t('systemConsole.members.colMember'), cell: 'member' } as PrimaryTableCol<EnterpriseWorkspaceMember>,
  { colKey: 'role', title: t('systemConsole.members.colRole'), width: 110, cell: 'role' } as PrimaryTableCol<EnterpriseWorkspaceMember>,
  { colKey: 'joined_at', title: t('systemConsole.members.colJoinedAt'), width: 110, cell: 'joined_at' } as PrimaryTableCol<EnterpriseWorkspaceMember>,
  { colKey: 'actions', title: t('systemConsole.users.colActions'), width: 210, align: 'right', cell: 'memberActions' } as PrimaryTableCol<EnterpriseWorkspaceMember>,
])

function openMembersDialog(row: PlatformTenant) {
  membersTenant.value = row
  membersList.value = []
  membersTotal.value = 0
  membersPage.value = 1
  membersQuery.value = ''
  membersVisible.value = true
  void loadMembers()
}

async function loadMembers() {
  if (!membersTenant.value) return
  membersLoading.value = true
  try {
    const resp = await listWorkspaceMembers(membersTenant.value.id, {
      q: membersQuery.value.trim() || undefined,
      page: membersPage.value,
      page_size: membersPageSize.value,
    })
    membersList.value = resp.members || []
    membersTotal.value = resp.total || 0
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('systemConsole.messages.loadFailed'))
  } finally {
    membersLoading.value = false
  }
}

function reloadMembers() {
  membersPage.value = 1
  void loadMembers()
}

// 后端对"最后一名空间管理员"的降级/移除返回 409，本地化后再提示。
function lastAdminAwareError(e: any): string {
  if (e?.status === 409 || /last workspace admin/i.test(String(e?.message || ''))) {
    return t('systemConsole.members.lastAdminError')
  }
  return e?.message || t('systemConsole.messages.opFailed')
}

async function setMemberRole(row: EnterpriseWorkspaceMember, role: EnterpriseWorkspaceRole) {
  if (!membersTenant.value) return
  memberBusy.value = row.user_id
  try {
    await updateWorkspaceMemberRole(membersTenant.value.id, row.user_id, role)
    MessagePlugin.success(t('systemConsole.messages.opSuccess'))
    await loadMembers()
  } catch (e: any) {
    MessagePlugin.error(lastAdminAwareError(e))
  } finally {
    memberBusy.value = null
  }
}

async function removeMember(row: EnterpriseWorkspaceMember) {
  if (!membersTenant.value) return
  const ok = await dialogConfirm(
    t('systemConsole.members.removeConfirm', {
      name: row.username || row.employee_id || row.user_id,
      tenant: membersTenant.value.name,
    }),
  )
  if (!ok) return
  memberBusy.value = row.user_id
  try {
    await unbindUserFromTenant(row.user_id, membersTenant.value.id)
    MessagePlugin.success(t('systemConsole.messages.opSuccess'))
    await loadMembers()
  } catch (e: any) {
    MessagePlugin.error(lastAdminAwareError(e))
  } finally {
    memberBusy.value = null
  }
}

// ---- 模型分配 ----
// 平台目录中可分配的是非内置模型；builtin 对所有空间可见、无需分配。
const assignmentsVisible = ref(false)
const assignmentsTenant = ref<PlatformTenant | null>(null)
const assignmentsLoading = ref(false)
const assignmentsSaving = ref(false)
const catalogModels = ref<ModelConfig[]>([])
const selectedModelIds = ref<string[]>([])

const ASSIGNMENT_TYPE_ORDER = [
  { key: 'KnowledgeQA', icon: 'chat' },
  { key: 'Embedding', icon: 'chart-bubble' },
  { key: 'Rerank', icon: 'filter-sort' },
  { key: 'VLLM', icon: 'image' },
  { key: 'ASR', icon: 'sound' },
]

const assignmentGroups = computed(() =>
  ASSIGNMENT_TYPE_ORDER.map(({ key, icon }) => ({
    key,
    icon,
    label: t(`modelSettings.typeShort.${backendTypeToModelType[key] ?? 'chat'}`),
    models: catalogModels.value.filter((m) => m.type === key && !m.is_builtin),
  })),
)

// 后端 ModelType → 前端模型类型短标签的映射（与 ModelSettings 的
// backendTypeToModelType 保持一致）。
const backendTypeToModelType: Record<string, string> = {
  KnowledgeQA: 'chat',
  Embedding: 'embedding',
  Rerank: 'rerank',
  VLLM: 'vllm',
  ASR: 'asr',
}

function groupSelected(group: { models: ModelConfig[] }) {
  return group.models.length > 0 && group.models.every((m) => selectedModelIds.value.includes(m.id))
}

function groupIndeterminate(group: { models: ModelConfig[] }) {
  const hit = group.models.filter((m) => selectedModelIds.value.includes(m.id)).length
  return hit > 0 && hit < group.models.length
}

function toggleGroup(group: { models: ModelConfig[] }, on: boolean) {
  const ids = group.models.map((m) => m.id)
  if (on) {
    selectedModelIds.value = Array.from(new Set([...selectedModelIds.value, ...ids]))
  } else {
    selectedModelIds.value = selectedModelIds.value.filter((id) => !ids.includes(id))
  }
}

function toggleModel(id: string) {
  const idx = selectedModelIds.value.indexOf(id)
  if (idx >= 0) {
    selectedModelIds.value.splice(idx, 1)
  } else {
    selectedModelIds.value.push(id)
  }
}

async function openAssignmentsDialog(row: PlatformTenant) {
  assignmentsTenant.value = row
  assignmentsVisible.value = true
  assignmentsLoading.value = true
  selectedModelIds.value = []
  try {
    const [catalog, current] = await Promise.all([
      listSystemModels(),
      listTenantModelAssignments(row.id),
    ])
    catalogModels.value = catalog
    selectedModelIds.value = (current.models || []).map((m) => m.id)
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('systemConsole.messages.loadFailed'))
  } finally {
    assignmentsLoading.value = false
  }
}

async function submitAssignments() {
  if (!assignmentsTenant.value) return
  assignmentsSaving.value = true
  try {
    await updateTenantModelAssignments(assignmentsTenant.value.id, [...selectedModelIds.value])
    MessagePlugin.success(t('systemConsole.assignments.saved'))
    assignmentsVisible.value = false
    assignmentsTenant.value = null
  } catch (e: any) {
    // 409 = 移除了仍被该空间 KB / 智能体 / 记忆配置引用的模型，
    // 后端 message 已带具体用量，直接展示。
    MessagePlugin.error(e?.message || t('systemConsole.messages.opFailed'))
  } finally {
    assignmentsSaving.value = false
  }
}

// ---- 新建空间 ----
const tenantCreateVisible = ref(false)
const tenantCreateLoading = ref(false)
const tenantCreateForm = reactive({ name: '', description: '', storage_quota_gb: null as number | null })

function resetTenantCreateForm() {
  tenantCreateForm.name = ''
  tenantCreateForm.description = ''
  tenantCreateForm.storage_quota_gb = null
}

async function submitTenantCreate() {
  tenantCreateForm.name = tenantCreateForm.name.trim()
  if (!tenantCreateForm.name) {
    MessagePlugin.warning(t('systemConsole.tenants.nameRequired'))
    return
  }
  tenantCreateLoading.value = true
  try {
    await createPlatformTenant({
      name: tenantCreateForm.name,
      description: tenantCreateForm.description.trim() || undefined,
      storage_quota_gb: tenantCreateForm.storage_quota_gb ?? undefined,
    })
    MessagePlugin.success(t('systemConsole.messages.opSuccess'))
    tenantCreateVisible.value = false
    resetTenantCreateForm()
    void loadTenants()
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('systemConsole.messages.opFailed'))
  } finally {
    tenantCreateLoading.value = false
  }
}

// ---- 工具 ----
function formatDate(value: string): string {
  if (!value) return '—'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  return d.toLocaleDateString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit' })
}

function formatQuota(row: PlatformTenant): string {
  const usedGB = (row.storage_used || 0) / 1024 / 1024 / 1024
  const quotaGB = (row.storage_quota || 0) / 1024 / 1024 / 1024
  if (!quotaGB) return `${usedGB.toFixed(1)} GB / —`
  return `${usedGB.toFixed(1)} / ${quotaGB.toFixed(0)} GB`
}

onMounted(() => {
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

.cell-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 2px;
}

.dialog-hint {
  margin: 0 0 14px;
  font-size: 13px;
  line-height: 1.6;
  color: var(--td-text-color-secondary);
}

// 空间成员管理
.members-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;

  .members-search {
    flex: 1;
  }
}

.members-table {
  :deep(.t-table) {
    border-radius: 8px;
  }

  .data-table-shell__pager {
    padding: 10px 0 2px;
    display: flex;
    justify-content: flex-end;
  }
}

// 模型分配
.assignments-loading {
  display: grid;
  place-items: center;
  padding: 48px 0;
}

.assignment-group {
  margin-bottom: 18px;
}

.assignment-group-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 0;
  border-bottom: 1px solid var(--td-component-stroke);
}

.assignment-group-title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-weight: 500;
  color: var(--td-text-color-primary);
}

.assignment-empty {
  padding: 8px 0 4px;
  font-size: 13px;
  color: var(--td-text-color-placeholder);
}

.assignment-list {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 4px 16px;
  padding: 6px 0;

  .assignment-model-name {
    margin-right: 6px;
    color: var(--td-text-color-primary);
  }

  .assignment-model-sub {
    font-size: 12px;
    color: var(--td-text-color-placeholder);
    word-break: break-all;
  }
}
</style>
