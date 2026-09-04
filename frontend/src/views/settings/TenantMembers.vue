<template>
  <div class="tenant-members">
    <!-- Section header -->
    <div class="section-header">
      <div class="section-header-row">
        <div class="section-header-titlewrap">
          <h2>{{ $t('tenantMember.title') }}</h2>
          <t-popup placement="bottom-start" trigger="hover" overlay-class-name="permissions-popup-overlay"
            :overlay-inner-style="permissionsPopupInnerStyle">
            <button type="button" class="permissions-trigger-btn" :aria-label="$t('tenantMember.permissions.title')"
              :title="$t('tenantMember.permissions.iconHint')">
              <t-icon name="info-circle" size="16px" />
            </button>
            <template #content>
              <div class="permissions-compact permissions-compact--popover">
                <div class="permissions-compact-header">
                  <span class="permissions-compact-title">{{ $t('tenantMember.permissions.title') }}</span>
                  <span class="permissions-compact-desc">{{ $t('tenantMember.permissions.desc') }}</span>
                </div>
                <div class="permissions-compact-grid">
                  <div v-for="r in roleMatrixOrder" :key="r"
                    :class="['perm-role-block', r, { 'is-me': currentRole === r }]">
                    <div class="perm-role-tag">
                      <t-icon :name="roleMatrixIcon(r)" size="12px" />
                      <span>{{ $t('tenantMember.role.' + r) }}</span>
                      <span v-if="currentRole === r" class="me-badge">{{ $t('common.me') }}</span>
                    </div>
                    <div class="perm-items">
                      <span v-for="(perm, i) in roleMatrix[r]" :key="i" :class="['perm-item', perm.has ? 'has' : 'no']">
                        <t-icon :name="perm.has ? 'check' : 'close'" size="12px" />
                        {{ $t('tenantMember.permissions.' + perm.key) }}
                      </span>
                    </div>
                  </div>
                </div>
              </div>
            </template>
          </t-popup>
          <!-- Audit log entry -->
          <t-button v-if="canViewAudit" variant="text" size="small" class="header-audit-btn" @click="openAuditDrawer">
            <template #icon><t-icon name="history" /></template>
            {{ $t('tenantMember.audit.tabLabel') }}
          </t-button>
        </div>
      </div>
      <p class="section-description">
        {{ $t('tenantMember.sectionDescriptionEnterprise') }}
      </p>
    </div>

    <div class="members-tab-layout">
      <!-- Member list -->
      <div class="members-list-wrap">
        <div class="members-list-header">
          <div class="members-list-titlewrap">
            <span class="members-list-title">{{ $t('tenantMember.listTitle') }}</span>
            <span class="members-list-count-badge">{{ membersTotal }}</span>
          </div>
          <div class="members-list-actions">
            <div class="members-list-search">
              <t-input v-model="searchQuery" size="small" :placeholder="$t('tenantMember.searchPlaceholder')" clearable>
                <template #prefix-icon><t-icon name="search" /></template>
              </t-input>
            </div>
          </div>
        </div>
        <div v-if="loading && members.length === 0" class="loading-inline">
          <t-loading size="small" />
          <span>{{ $t('tenantMember.loading') }}</span>
        </div>
        <div v-else-if="error" class="error-inline">
          <t-alert theme="error" :message="error">
            <template #operation>
              <t-button size="small" @click="loadMembers">{{ $t('tenantMember.retry') }}</t-button>
            </template>
          </t-alert>
        </div>
        <div v-else-if="membersTotal === 0" class="empty-state">
          <t-empty :description="searchQuery.trim()
            ? $t('tenantMember.emptySearch', { q: searchQuery })
            : $t('tenantMember.empty')
            " />
        </div>
        <div v-else class="data-table-shell">
          <div class="data-table-shell__scroll">
            <t-table row-key="user_id" :data="members" :columns="columns" size="medium" hover stripe :loading="loading">
              <template #member="{ row }">
                <div class="member-cell">
                  <span class="member-name">{{ memberPrimary(row) }}</span>
                  <span v-if="memberSecondary(row)" class="member-email">{{ memberSecondary(row) }}</span>
                </div>
              </template>
              <template #role="{ row }">
                <t-tag :theme="roleTagTheme(row.role)" size="small">
                  {{ $t('tenantMember.role.' + row.role) }}
                </t-tag>
              </template>
              <template #joined_at="{ row }">{{ formatDate(row.joined_at) }}</template>
            </t-table>
          </div>
          <div v-if="membersTotal > 0" class="data-table-shell__pager">
            <t-pagination v-model="membersPage" v-model:page-size="membersPageSize" :total="membersTotal" size="small"
              show-jumper show-page-number show-page-size :page-size-options="MEMBERS_PAGE_SIZE_OPTIONS"
              @change="onMembersPageChange" />
          </div>
        </div>
      </div>
    </div>

    <!-- Audit log drawer -->
    <t-drawer v-if="canViewAudit" v-model:visible="auditDrawerVisible" :header="$t('tenantMember.audit.tabLabel')"
      drawer-class-name="tenant-members-audit-drawer" size="880px" :footer="false" placement="right" destroy-on-close>
      <div class="audit-drawer-inner audit-panel audit-panel--drawer">
        <div class="audit-header">
          <span class="audit-desc">{{ $t('tenantMember.audit.description') }}</span>
          <t-button variant="text" size="small" class="audit-refresh-btn"
            :loading="auditLoading" :disabled="auditLoading" @click="reloadAuditLog">
            <template #icon><t-icon name="refresh" /></template>
            {{ $t('tenantMember.audit.refresh') }}
          </t-button>
        </div>

        <div class="audit-drawer-fill">
          <div v-if="auditError" class="audit-drawer-branch audit-drawer-branch--error">
            <div class="error-inline">
              <t-alert theme="error" :message="auditError">
                <template #operation>
                  <t-button size="small" @click="reloadAuditLog">
                    {{ $t('tenantMember.retry') }}
                  </t-button>
                </template>
              </t-alert>
            </div>
          </div>

          <div v-else-if="!auditLoading && auditEntries.length === 0"
            class="audit-drawer-branch audit-drawer-branch--empty empty-state empty-state--audit">
            <t-empty :description="$t('tenantMember.audit.empty')" />
          </div>

          <div v-else class="audit-scroll-area narrow-scrollbar audit-drawer-branch" ref="auditScrollRoot">
            <div class="data-table-shell audit-table-shell">
              <t-table
                row-key="id"
                :data="auditEntries"
                :columns="auditColumns"
                size="medium"
                hover
                expand-on-row-click
                :expanded-row-keys="auditExpandedRowKeys"
                @expand-change="onAuditExpandChange"
              >
                <template #created_at="{ row }">
                  <div class="audit-time">
                    <span class="audit-time-date">{{ formatAuditDatePart(row.created_at) }}</span>
                    <span class="audit-time-clock">{{ formatAuditTimePart(row.created_at) }}</span>
                  </div>
                </template>
                <template #actor="{ row }">
                  <div class="audit-actor">
                    <span class="audit-actor-name">
                      {{ row.actor_user_id ? actorDisplayName(row.actor_user_id) :
                        $t('tenantMember.audit.systemActor') }}
                    </span>
                    <span v-if="row.actor_role" class="audit-actor-role">
                      {{ $t('tenantMember.role.' + row.actor_role) }}
                    </span>
                  </div>
                </template>
                <template #action="{ row }">
                  <t-tag :theme="auditActionTheme(row.action)" size="small" variant="light-outline">
                    {{ formatAuditAction(row.action) }}
                  </t-tag>
                </template>
                <template #target="{ row }">
                  <div class="audit-target">
                    <span v-if="auditTargetSubject(row)" class="audit-target-key">{{ auditTargetSubject(row) }}</span>
                    <span v-if="auditTargetDiff(row)" class="audit-target-diff">{{ auditTargetDiff(row) }}</span>
                    <span v-else-if="!auditTargetSubject(row)" class="audit-target-empty">—</span>
                  </div>
                </template>
                <template #request_path="{ row }">
                  <span v-if="row.request_path" class="audit-path">
                    <span v-if="row.request_method" class="audit-method">{{ row.request_method }}</span>
                    {{ row.request_path }}
                  </span>
                  <span v-else class="audit-target-empty">—</span>
                </template>
                <template #outcome="{ row }">
                  <t-tag :theme="auditOutcomeTheme(row.outcome)" size="small" variant="light">
                    {{ $t('tenantMember.audit.outcome.' + row.outcome) }}
                  </t-tag>
                </template>
                <template #expandedRow="{ row }">
                  <div class="audit-expanded">
                    <div class="audit-expanded-grid">
                      <div class="audit-expanded-cell">
                        <span class="audit-expanded-label">{{ $t('tenantMember.audit.expanded.actorId') }}</span>
                        <span class="audit-expanded-value mono">{{ row.actor_user_id || '—' }}</span>
                      </div>
                      <div v-if="row.target_user_id" class="audit-expanded-cell">
                        <span class="audit-expanded-label">{{ $t('tenantMember.audit.expanded.targetUserId') }}</span>
                        <span class="audit-expanded-value mono">{{ row.target_user_id }}</span>
                      </div>
                      <div v-if="row.target_type" class="audit-expanded-cell">
                        <span class="audit-expanded-label">{{ $t('tenantMember.audit.expanded.targetType') }}</span>
                        <span class="audit-expanded-value mono">{{ row.target_type }}</span>
                      </div>
                      <div v-if="row.target_id" class="audit-expanded-cell">
                        <span class="audit-expanded-label">{{ $t('tenantMember.audit.expanded.targetId') }}</span>
                        <span class="audit-expanded-value mono">{{ row.target_id }}</span>
                      </div>
                    </div>
                    <div class="audit-expanded-details">
                      <span class="audit-expanded-label">{{ $t('tenantMember.audit.expanded.details') }}</span>
                      <pre class="audit-expanded-json mono">{{ auditDetailsJSON(row) }}</pre>
                    </div>
                  </div>
                </template>
              </t-table>
            </div>

            <div ref="auditLoadSentinelEl" class="audit-load-sentinel" aria-hidden="true" />

            <div v-if="auditLoading && auditEntries.length > 0" class="audit-loading-more">
              <t-loading size="small" />
              <span>{{ $t('tenantMember.loading') }}</span>
            </div>

            <p v-if="!auditHasMore && auditEntries.length > 0 && !auditLoading" class="audit-end-hint">
              {{ $t('tenantMember.audit.end') }}
            </p>
          </div>
        </div>
      </div>
    </t-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { AUDIT_ACTION_I18N_ROOTS } from '@/i18n/auditActionRegistry'
import { auditActionLabel } from '@/i18n/auditActionLabel'
import {
  listMembers,
  type TenantMember,
  type TenantRole,
} from '@/api/tenant/members'
import {
  listAuditLog,
  type AuditLog,
  type AuditAction,
  type AuditOutcome,
} from '@/api/tenant/audit-log'

const { t, tm } = useI18n()
const authStore = useAuthStore()

const permissionsPopupInnerStyle = {
  boxSizing: 'border-box' as const,
  padding: '0',
  width: 'min(520px, calc(100vw - 24px))',
  maxWidth: 'min(520px, calc(100vw - 24px))',
  maxHeight: 'min(400px, 65vh)',
  overflow: 'hidden',
}

// State
const members = ref<TenantMember[]>([])
const loading = ref(false)
const error = ref('')
const searchQuery = ref('')
const memberSearchQ = ref('')
let memberSearchDebounceTimer: number | undefined

const membersTotal = ref(0)
const membersPage = ref(1)
const membersPageSize = ref(20)

const memberDisplayByUserId = reactive<Record<string, { username?: string; email?: string }>>({})

const MEMBERS_PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

const auditDrawerVisible = ref(false)
const auditEntries = ref<AuditLog[]>([])
const auditLoading = ref(false)
const auditError = ref('')
const auditCursor = ref<number>(0)
const auditHasMore = ref(true)
const auditLoadedOnce = ref(false)
const AUDIT_PAGE_SIZE = 50

const auditScrollRoot = ref<HTMLElement | null>(null)
const auditLoadSentinelEl = ref<HTMLElement | null>(null)
let auditScrollObserver: IntersectionObserver | null = null

const currentRole = computed<TenantRole | ''>(() => (authStore.currentTenantRole || '') as TenantRole | '')
const canViewAudit = computed(
  () =>
    currentRole.value === 'owner' ||
    currentRole.value === 'admin' ||
    authStore.canAccessAllTenants === true,
)
const activeTenantId = computed(() => Number(authStore.currentTenantId ?? 0))

// Role matrix — 企业版两级角色：空间管理员 / 普通用户。owner/viewer 仅存于
// 历史数据（000092 前），不再出现在矩阵里，但保留映射供历史审计行展示。
type RolePerm = { key: string; has: boolean }
const roleMatrixOrder: TenantRole[] = ['admin', 'contributor']
const roleMatrix: Record<TenantRole, RolePerm[]> = {
  owner: [
    { key: 'manageMembers', has: true },
    { key: 'manageTenantConfig', has: true },
    { key: 'manageInfra', has: true },
    { key: 'createOwnKB', has: true },
    { key: 'readAll', has: true },
  ],
  admin: [
    // 成员与空间本身的增删由系统管理员在控制台操作
    { key: 'manageMembers', has: false },
    { key: 'manageTenantConfig', has: true },
    { key: 'manageInfra', has: true },
    { key: 'createOwnKB', has: true },
    { key: 'readAll', has: true },
  ],
  contributor: [
    { key: 'manageMembers', has: false },
    { key: 'manageTenantConfig', has: false },
    { key: 'manageInfra', has: false },
    { key: 'createOwnKB', has: true },
    { key: 'readAll', has: true },
  ],
  viewer: [
    { key: 'manageMembers', has: false },
    { key: 'manageTenantConfig', has: false },
    { key: 'manageInfra', has: false },
    { key: 'createOwnKB', has: false },
    { key: 'readAll', has: true },
  ],
}

function roleMatrixIcon(role: TenantRole): string {
  switch (role) {
    case 'owner':
      return 'user-vip-filled'
    case 'admin':
      return 'user-safety'
    case 'contributor':
      return 'edit'
    default:
      return 'browse'
  }
}

const columns = computed(() => [
  { colKey: 'member', title: t('tenantMember.columns.member'), ellipsis: true, minWidth: 132 },
  { colKey: 'role', title: t('tenantMember.columns.role'), width: 128 },
  { colKey: 'joined_at', title: t('tenantMember.columns.joinedAt'), width: 154 },
])

function memberPrimary(row: { username?: string; email?: string; employee_id?: string }) {
  return row.username?.trim() || row.employee_id?.trim() || row.email?.trim() || '—'
}

function memberSecondary(row: { username?: string; email?: string; employee_id?: string }) {
  const name = row.username?.trim()
  const empId = row.employee_id?.trim()
  const mail = row.email?.trim()
  if (name && empId && name !== empId) return empId
  if (name && mail && name !== mail) return mail
  if (empId && mail && empId !== mail) return mail
  return ''
}

function roleTagTheme(role: TenantRole): 'primary' | 'warning' | 'success' | 'default' {
  switch (role) {
    case 'owner':
      return 'primary'
    case 'admin':
      return 'warning'
    case 'contributor':
      return 'success'
    default:
      return 'default'
  }
}

function formatDate(s: string | undefined): string {
  if (!s) return '-'
  try {
    const d = new Date(s)
    return new Intl.DateTimeFormat('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    }).format(d)
  } catch {
    return s
  }
}

function rememberMembersForAudit(rows: TenantMember[]) {
  for (const m of rows) {
    memberDisplayByUserId[m.user_id] = { username: m.username, email: m.email }
  }
}

async function loadMembers() {
  if (!activeTenantId.value) {
    return
  }
  loading.value = true
  error.value = ''
  try {
    const resp = await listMembers(activeTenantId.value, {
      page: membersPage.value,
      page_size: membersPageSize.value,
      q: memberSearchQ.value || undefined,
    })
    if (resp.success && resp.data) {
      const total = resp.data.total ?? 0
      const ps = resp.data.page_size ?? membersPageSize.value
      const safePs = Math.max(1, ps)
      const maxPage = Math.max(1, Math.ceil(total / safePs))
      if (membersPage.value > maxPage) {
        membersPage.value = maxPage
        loading.value = false
        await loadMembers()
        return
      }
      members.value = resp.data.members ?? []
      membersTotal.value = total
      if (typeof resp.data.page === 'number' && resp.data.page > 0) {
        membersPage.value = resp.data.page
      }
      if (typeof resp.data.page_size === 'number' && resp.data.page_size > 0) {
        membersPageSize.value = resp.data.page_size
      }
      rememberMembersForAudit(members.value)
    } else {
      error.value = resp.message || t('tenantMember.errors.generic')
    }
  } catch (err: any) {
    error.value = err?.message || t('tenantMember.errors.generic')
  } finally {
    loading.value = false
  }
}

function onMembersPageChange() {
  void loadMembers()
}

watch(searchQuery, () => {
  if (!activeTenantId.value) return
  window.clearTimeout(memberSearchDebounceTimer)
  memberSearchDebounceTimer = window.setTimeout(() => {
    memberSearchQ.value = searchQuery.value.trim()
    membersPage.value = 1
    loadMembers()
  }, 320)
})

// Audit log helpers
const auditColumns = computed(() => [
  { colKey: 'created_at', title: t('tenantMember.audit.columns.time'), width: 120 },
  { colKey: 'actor', title: t('tenantMember.audit.columns.actor'), width: 180 },
  { colKey: 'action', title: t('tenantMember.audit.columns.action'), width: 130 },
  {
    colKey: 'target',
    title: t('tenantMember.audit.columns.target'),
    minWidth: 200,
  },
  {
    colKey: 'request_path',
    title: t('tenantMember.audit.columns.path'),
    minWidth: 160,
  },
  { colKey: 'outcome', title: t('tenantMember.audit.columns.outcome'), width: 80, align: 'center' as const },
])

function formatAuditDatePart(s: string | undefined): string {
  if (!s) return '-'
  try {
    return new Intl.DateTimeFormat('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    }).format(new Date(s))
  } catch {
    return s
  }
}

function formatAuditTimePart(s: string | undefined): string {
  if (!s) return ''
  try {
    return new Intl.DateTimeFormat('zh-CN', {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    }).format(new Date(s))
  } catch {
    return ''
  }
}

function auditActionTheme(
  action: AuditAction,
): 'success' | 'warning' | 'danger' | 'primary' | 'default' {
  switch (action) {
    case 'rbac.access_denied':
      return 'danger'
    case 'rbac.member_added':
      return 'success'
    case 'rbac.member_removed':
    case 'rbac.member_left':
    case 'rbac.member_role_changed':
      return 'warning'
    default:
      return 'default'
  }
}

function auditOutcomeTheme(o: AuditOutcome): 'success' | 'danger' | 'default' {
  if (o === 'denied') return 'danger'
  if (o === 'success') return 'success'
  return 'default'
}

function formatAuditAction(action: AuditAction): string {
  return auditActionLabel({ tm }, AUDIT_ACTION_I18N_ROOTS.tenantMember, action)
}

function actorDisplayName(userId: string): string {
  const cur = members.value.find((x) => x.user_id === userId)
  if (cur?.username?.trim()) return cur.username.trim()
  if (cur?.email?.trim()) return cur.email.trim()
  const memo = memberDisplayByUserId[userId]
  if (memo?.username?.trim()) return memo.username!.trim()
  if (memo?.email?.trim()) return memo.email!.trim()
  return userId
}

function auditDetailsObject(row: AuditLog): Record<string, unknown> | null {
  if (row.details && typeof row.details === 'object') {
    return row.details as Record<string, unknown>
  }
  return null
}

function auditTargetSubject(row: AuditLog): string {
  if (row.target_user_id) return actorDisplayName(row.target_user_id)
  if (row.target_id) {
    return row.target_type ? `${row.target_type}:${row.target_id}` : row.target_id
  }
  return ''
}

function auditTargetDiff(row: AuditLog): string {
  const d = auditDetailsObject(row)
  if (!d) return ''
  if (row.action === 'rbac.member_role_changed') {
    if (d.old_role && d.new_role) return `${d.old_role} → ${d.new_role}`
  }
  if (row.action === 'rbac.access_denied') {
    if (typeof d.required_role === 'string') {
      return t('tenantMember.audit.requiredRole', { role: d.required_role })
    }
  }
  if (row.action === 'rbac.invitation_sent' || row.action === 'rbac.invitation_revoked') {
    if (typeof d.role === 'string') return String(d.role)
  }
  return ''
}

const auditExpandedRowKeys = ref<number[]>([])

function onAuditExpandChange(value: (string | number)[]) {
  auditExpandedRowKeys.value = value
    .map((v) => (typeof v === 'number' ? v : Number(v)))
    .filter((v) => Number.isFinite(v))
}

function auditDetailsJSON(row: AuditLog): string {
  if (row.details === null || row.details === undefined) return '{}'
  if (typeof row.details === 'string') return row.details
  try {
    return JSON.stringify(row.details, null, 2)
  } catch {
    return String(row.details)
  }
}

async function loadAuditLog(reset: boolean) {
  if (!activeTenantId.value || !canViewAudit.value) return
  if (auditLoading.value) return
  if (!reset && !auditHasMore.value) return

  auditLoading.value = true
  auditError.value = ''
  try {
    const resp = await listAuditLog(activeTenantId.value, {
      after_id: reset ? undefined : auditCursor.value || undefined,
      limit: AUDIT_PAGE_SIZE,
    })
    if (resp.success) {
      const rows = resp.data || []
      if (reset) {
        auditEntries.value = rows
      } else {
        auditEntries.value = [...auditEntries.value, ...rows]
      }
      auditCursor.value = resp.next_cursor || 0
      auditHasMore.value = !!resp.next_cursor && rows.length > 0
      auditLoadedOnce.value = true
    } else {
      auditError.value = resp.message || t('tenantMember.errors.generic')
    }
  } catch (err: any) {
    const status = err?.status
    if (status === 403) {
      auditError.value = t('tenantMember.audit.forbidden')
    } else {
      auditError.value = err?.message || t('tenantMember.errors.generic')
    }
  } finally {
    auditLoading.value = false
  }
}

function detachAuditInfiniteScroll() {
  auditScrollObserver?.disconnect()
  auditScrollObserver = null
}

function attachAuditInfiniteScroll() {
  detachAuditInfiniteScroll()
  const root = auditScrollRoot.value
  const sentinel = auditLoadSentinelEl.value
  if (!root || !sentinel) return

  auditScrollObserver = new IntersectionObserver(
    (entries) => {
      const hitBottom = entries.some((e) => e.isIntersecting)
      if (!hitBottom || !auditHasMore.value || auditLoading.value) return
      void loadAuditLog(false)
    },
    { root, rootMargin: '100px 0px', threshold: 0 },
  )
  auditScrollObserver.observe(sentinel)
}

function reloadAuditLog() {
  auditCursor.value = 0
  auditHasMore.value = true
  loadAuditLog(true)
}

function openAuditDrawer() {
  auditDrawerVisible.value = true
  if (!auditLoadedOnce.value) {
    loadAuditLog(true)
  }
}

watch(
  auditDrawerVisible,
  async (open) => {
    if (!open) {
      detachAuditInfiniteScroll()
      return
    }
    await nextTick()
    attachAuditInfiniteScroll()
  },
  { flush: 'post' },
)

watch(
  () => auditError.value,
  async () => {
    if (!auditDrawerVisible.value) return
    await nextTick()
    if (!auditError.value) {
      attachAuditInfiniteScroll()
      return
    }
    detachAuditInfiniteScroll()
  },
  { flush: 'post' },
)

onUnmounted(() => detachAuditInfiniteScroll())

watch(
  activeTenantId,
  (id) => {
    if (id) {
      searchQuery.value = ''
      memberSearchQ.value = ''
      window.clearTimeout(memberSearchDebounceTimer)
      membersPage.value = 1
      membersTotal.value = 0
      loadMembers()
    }
  },
  { immediate: true },
)
</script>

<style lang="less" scoped>
.tenant-members {
  width: 100%;
}

.member-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  padding: 2px 0;

  .member-name {
    font-weight: 500;
    font-size: 14px;
    color: var(--td-text-color-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .member-email {
    font-size: 12px;
    line-height: 1.35;
    color: var(--td-text-color-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.section-header {
  margin-bottom: 20px;

  h2 {
    font-size: 20px;
    font-weight: 600;
    color: var(--td-text-color-primary);
    margin: 0;
    letter-spacing: -0.02em;
  }

  .section-description {
    color: var(--td-text-color-secondary);
    font-size: 13px;
    line-height: 1.55;
    margin: 8px 0 0;
    max-width: 52rem;
  }
}

.section-header-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.section-header-titlewrap {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  min-width: 0;

  h2 {
    margin: 0;
    line-height: 1.25;
  }
}

.header-audit-btn {
  flex-shrink: 0;
}

.members-tab-layout {
  display: flex;
  flex-direction: column;
}

.data-table-shell {
  overflow-x: auto;
  border-radius: 10px;
  border: 1px solid var(--td-component-stroke);
  background-color: var(--td-bg-color-container);

  &:deep(thead th) {
    font-weight: 600;
    font-size: 13px;
  }

  &:deep(.t-table td),
  &:deep(.t-table th) {
    padding-top: 12px;
    padding-bottom: 12px;
  }
}

.permissions-trigger-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  width: 22px;
  height: 22px;
  margin: 0;
  padding: 0;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  line-height: 0;
  transition: background-color 0.2s ease, color 0.2s ease;

  :deep(.t-icon) {
    display: block;
  }

  &:hover {
    background-color: var(--td-bg-color-secondarycontainer);
    color: var(--td-brand-color);
  }

  &:focus-visible {
    outline: 2px solid var(--td-brand-color-focus);
    outline-offset: 1px;
  }
}

.members-list-wrap {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.members-list-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 0 2px;
  flex-wrap: wrap;
}

.members-list-titlewrap {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.members-list-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.members-list-count-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 22px;
  height: 20px;
  padding: 0 7px;
  border-radius: 10px;
  background-color: var(--td-bg-color-secondarycontainer);
  color: var(--td-text-color-primary);
  font-size: 12px;
  font-weight: 600;
  line-height: 1;
}

.members-list-actions {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  flex: 0 1 auto;
  min-width: 0;
}

.members-list-search {
  flex: 0 0 14rem;
  width: 14rem;
  min-width: 0;

  :deep(.t-input) {
    width: 100%;
  }
}

@media (max-width: 560px) {
  .members-list-actions {
    width: 100%;
    justify-content: flex-start;
  }

  .members-list-search {
    flex: 1 1 auto;
    width: auto;
    max-width: none;
  }
}

.permissions-compact {
  padding: 8px;

  .permissions-compact-header {
    display: flex;
    flex-direction: column;
    gap: 4px;
    margin-bottom: 16px;

    .permissions-compact-title {
      font-size: 14px;
      font-weight: 600;
      color: var(--td-text-color-primary);
    }

    .permissions-compact-desc {
      font-size: 13px;
      color: var(--td-text-color-secondary);
    }
  }

  .permissions-compact-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
    gap: 12px;
  }

  .perm-role-block {
    border: 1px solid var(--td-component-stroke);
    border-radius: 8px;
    padding: 14px 16px;
    background: var(--td-bg-color-container);
    transition: all 0.2s ease;

    &.is-me {
      border-color: var(--td-brand-color);
      background: var(--td-brand-color-light);
    }

    .perm-role-tag {
      display: flex;
      align-items: center;
      gap: 6px;
      font-size: 14px;
      font-weight: 600;
      color: var(--td-text-color-primary);
      margin-bottom: 12px;

      .me-badge {
        margin-left: auto;
        font-size: 12px;
        font-weight: 500;
        color: var(--td-brand-color);
        padding: 2px 8px;
        background: var(--td-brand-color-light);
        border-radius: 4px;
      }
    }

    .perm-items {
      display: flex;
      flex-direction: column;
      gap: 6px;

      .perm-item {
        display: flex;
        align-items: flex-start;
        gap: 6px;
        font-size: 13px;
        line-height: 1.5;

        .t-icon {
          margin-top: 2px;
          flex-shrink: 0;
        }

        &.has {
          color: var(--td-text-color-secondary);

          .t-icon {
            color: var(--td-brand-color);
          }
        }

        &.no {
          color: var(--td-text-color-disabled);

          .t-icon {
            color: var(--td-text-color-disabled);
          }
        }
      }
    }
  }

  &.permissions-compact--popover {
    padding: 10px 12px;
    margin: 0;
    max-height: min(392px, calc(65vh - 8px));
    overflow-x: hidden;
    overflow-y: auto;

    .permissions-compact-header {
      gap: 2px;
      margin-bottom: 10px;

      .permissions-compact-title {
        font-size: 13px;
      }

      .permissions-compact-desc {
        font-size: 11px;
        line-height: 1.4;
      }
    }

    .permissions-compact-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 8px;
    }

    .perm-role-block {
      padding: 8px 10px;
      border-radius: 6px;

      .perm-role-tag {
        font-size: 12px;
        margin-bottom: 6px;
        gap: 4px;

        .me-badge {
          font-size: 10px;
          padding: 1px 5px;
        }
      }

      .perm-items {
        gap: 3px;

        .perm-item {
          font-size: 11px;
          line-height: 1.35;
          gap: 4px;

          .t-icon {
            margin-top: 1px;
            flex-shrink: 0;
          }
        }
      }
    }
  }

  @media (max-width: 480px) {
    &.permissions-compact--popover .permissions-compact-grid {
      grid-template-columns: 1fr;
    }
  }
}

.loading-inline,
.error-inline {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 20px 0 8px;
}

.empty-state {
  padding: 40px 0 16px;
  display: flex;
  justify-content: center;
}

.audit-panel {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding-top: 8px;
}

.audit-panel--drawer {
  padding-top: 0;
}

.audit-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: var(--td-bg-color-secondarycontainer);
  padding: 12px 16px;
  border-radius: 8px;
  gap: 12px;

  .audit-desc {
    flex: 1;
    min-width: 0;
    font-size: 13px;
    color: var(--td-text-color-secondary);
  }

  .audit-refresh-btn {
    flex-shrink: 0;
  }
}

.audit-drawer-inner {
  display: flex;
  flex-direction: column;
  flex: 1 1 auto;
  gap: 14px;
  min-height: 0;
  width: 100%;
  box-sizing: border-box;
}

.audit-drawer-fill {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.audit-drawer-branch {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.audit-drawer-branch--error {
  justify-content: center;

  .error-inline {
    width: 100%;
  }
}

.audit-drawer-branch--empty.empty-state--audit {
  flex: 1 1 auto;
  justify-content: center;
  align-items: center;
  padding: 24px 12px;
  min-height: 0;
}

.audit-scroll-area {
  flex: 1 1 auto;
  min-height: 0;
  overflow-x: hidden;
  overflow-y: auto;
}

.audit-load-sentinel {
  height: 1px;
  width: 100%;
  pointer-events: none;
}

.audit-loading-more {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 12px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.audit-end-hint {
  text-align: center;
  font-size: 12px;
  color: var(--td-text-color-disabled);
  padding: 8px 0 14px;
  margin: 0;
}

.audit-time {
  display: flex;
  flex-direction: column;
  gap: 2px;
  line-height: 1.3;

  .audit-time-date {
    font-size: 12px;
    color: var(--td-text-color-secondary);
  }

  .audit-time-clock {
    font-size: 13px;
    font-weight: 500;
    color: var(--td-text-color-primary);
    font-variant-numeric: tabular-nums;
  }
}

.audit-actor {
  display: flex;
  flex-direction: column;
  gap: 2px;
  line-height: 1.3;
  min-width: 0;

  .audit-actor-name {
    font-size: 13px;
    font-weight: 500;
    color: var(--td-text-color-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .audit-actor-role {
    font-size: 12px;
    color: var(--td-text-color-secondary);
  }
}

.audit-target {
  display: flex;
  flex-direction: column;
  gap: 4px;
  line-height: 1.35;
  min-width: 0;
  padding: 2px 0;

  .audit-target-key {
    font-size: 13px;
    color: var(--td-text-color-primary);
    word-break: break-all;
  }

  .audit-target-diff {
    font-size: 12px;
    color: var(--td-text-color-secondary);
    font-family: var(--td-font-family-mono, monospace);
    word-break: break-all;
    line-height: 1.4;
  }

  .audit-target-empty {
    color: var(--td-text-color-placeholder);
  }
}

.audit-path {
  font-family: var(--td-font-family-mono, monospace);
  font-size: 12px;
  color: var(--td-text-color-secondary);
  word-break: break-all;

  .audit-method {
    display: inline-block;
    font-weight: 600;
    color: var(--td-text-color-primary);
    margin-right: 4px;
  }
}

.audit-expanded {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 12px 16px;
  background: var(--td-bg-color-container-hover);
}

.audit-expanded-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 10px 18px;
}

.audit-expanded-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.audit-expanded-label {
  font-size: 11px;
  font-weight: 600;
  color: var(--td-text-color-secondary);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.audit-expanded-value {
  font-size: 12px;
  color: var(--td-text-color-primary);
  word-break: break-all;
}

.audit-expanded-details {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.audit-expanded-json {
  margin: 0;
  padding: 10px 12px;
  font-size: 12px;
  line-height: 1.55;
  color: var(--td-text-color-primary);
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 280px;
  overflow: auto;
}

.mono {
  font-family: var(--td-font-family-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace);
}
</style>

<style lang="less">
.permissions-popup-overlay {
  z-index: 3050 !important;

  .t-popup__content {
    padding: 0 !important;
    border-radius: 12px !important;
    background: var(--td-bg-color-container) !important;
    border: 0.5px solid var(--td-component-stroke) !important;
    box-shadow:
      0 0 0 0.5px rgba(0, 0, 0, 0.03),
      0 2px 4px rgba(0, 0, 0, 0.04),
      0 8px 24px rgba(0, 0, 0, 0.1) !important;
    backdrop-filter: blur(20px) saturate(180%) !important;
    -webkit-backdrop-filter: blur(20px) saturate(180%) !important;
    overflow: hidden;
  }
}

:root[theme-mode='dark'] .permissions-popup-overlay .t-popup__content {
  background: rgba(36, 36, 36, 0.92) !important;
  border-color: rgba(255, 255, 255, 0.08) !important;
  box-shadow:
    0 0 0 0.5px rgba(255, 255, 255, 0.05),
    0 2px 4px rgba(0, 0, 0, 0.12),
    0 8px 32px rgba(0, 0, 0, 0.28) !important;
}

.t-drawer.tenant-members-audit-drawer.t-drawer--right .t-drawer__content-wrapper--right {
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  max-height: 100vh;
  height: 100%;
}

.t-drawer.tenant-members-audit-drawer .t-drawer__body {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  flex-direction: column;
  box-sizing: border-box;
  overflow: hidden !important;
}
</style>