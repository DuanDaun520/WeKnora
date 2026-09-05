<template>
  <Teleport to="body">
    <t-dialog
      :visible="visible"
      :header="t('memberAudit.title')"
      width="860px"
      :footer="false"
      :close-on-overlay-click="false"
      attach="body"
      @close="handleClose"
    >
      <div class="member-audit">
        <div class="member-audit-toolbar">
          <p class="member-audit-desc">{{ t('memberAudit.description') }}</p>
          <t-button variant="outline" size="small" :loading="loading" @click="reload">
            <template #icon><t-icon name="refresh" /></template>
            {{ t('memberAudit.refresh') }}
          </t-button>
        </div>

        <div v-if="error" class="member-audit-hint">
          <t-alert theme="error" :message="error" />
        </div>
        <div v-else-if="!loading && entries.length === 0" class="member-audit-hint">
          <t-empty :description="t('memberAudit.empty')" />
        </div>
        <div v-else ref="scrollRoot" class="member-audit-scroll">
          <t-table
            row-key="id"
            :data="entries"
            :columns="columns"
            size="medium"
            hover
            stripe
            :loading="loading && entries.length === 0"
          >
            <template #created_at="{ row }">
              <span class="member-audit-time">{{ formatDateTime(row.created_at) }}</span>
            </template>
            <template #actor="{ row }">
              {{ row.actor_user_id ? actorDisplayName(row.actor_user_id) : t('memberAudit.systemActor') }}
            </template>
            <template #action="{ row }">
              <t-tag :theme="actionTheme(row.action)" size="small">
                {{ friendlyAction(row) }}
              </t-tag>
            </template>
            <template #outcome="{ row }">
              <span :class="['member-audit-outcome', `is-${row.outcome}`]">
                {{ outcomeLabel(row.outcome) }}
              </span>
            </template>
          </t-table>
          <div ref="loadSentinel" class="member-audit-sentinel" aria-hidden="true">
            <template v-if="loading">{{ t('memberAudit.loading') }}</template>
            <template v-else-if="!hasMore">{{ entries.length > 0 ? t('memberAudit.end') : '' }}</template>
          </div>
        </div>
      </div>
    </t-dialog>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useUIStore } from '@/stores/ui'
import { useAuthStore } from '@/stores/auth'
import {
  fetchAllTenantMembers,
  type TenantMember,
} from '@/api/tenant/members'
import {
  listAuditLog,
  type AuditAction,
  type AuditLog,
  type AuditOutcome,
} from '@/api/tenant/audit-log'
import { auditActionLabel } from '@/i18n/auditActionLabel'
import { AUDIT_ACTION_I18N_ROOTS } from '@/i18n/auditActionRegistry'

// 000101 成员行为日志：从 Settings 的成员管理里拆出来的独立弹窗
// （成员管理弹窗右上角链接 → uiStore.openMemberAudit）。与旧抽屉的差异按需求
// 收敛为「白话」：只保留 时间 / 操作人 / 事件 / 结果 四列，去掉请求
// 路径与原始 JSON 展开；rbac.access_denied 不再显示「访问被拒」，而是
// 按请求路径细分 —— 登录接口上的拒绝多半是密码输错（疑似登录时密码
// 错误），其余是权限不足或登录过期。
const { t, tm } = useI18n()
const uiStore = useUIStore()
const authStore = useAuthStore()

const visible = computed(() => uiStore.showMemberAuditModal)
const tenantId = computed(() => Number(authStore.currentTenantId ?? 0))

const PAGE_SIZE = 50
const entries = ref<AuditLog[]>([])
const loading = ref(false)
const error = ref('')
const cursor = ref(0)
const hasMore = ref(true)
const loadedOnce = ref(false)

// 操作人 / 目标 user_id → 姓名映射：打开弹窗时尽力拉全量名册（分页
// 循环），失败时退化为显示 user_id，不阻塞审计流。
const memberNameById = ref<Record<string, string>>({})

const scrollRoot = ref<HTMLElement>()
const loadSentinel = ref<HTMLElement>()
let observer: IntersectionObserver | null = null

const columns = computed(() => [
  { colKey: 'created_at', title: t('memberAudit.columns.time'), width: 170 },
  { colKey: 'actor', title: t('memberAudit.columns.actor'), width: 170, ellipsis: true },
  { colKey: 'action', title: t('memberAudit.columns.action'), ellipsis: true },
  { colKey: 'outcome', title: t('memberAudit.columns.outcome'), width: 90, align: 'center' as const },
])

async function loadMemberNames() {
  if (!tenantId.value) return
  try {
    const all: TenantMember[] = await fetchAllTenantMembers(tenantId.value)
    const map: Record<string, string> = {}
    for (const m of all) {
      const name = m.username?.trim() || m.employee_id?.trim()
      if (name) map[m.user_id] = name
    }
    memberNameById.value = map
  } catch {
    memberNameById.value = {}
  }
}

async function load(reset: boolean) {
  if (!tenantId.value || loading.value) return
  if (!reset && !hasMore.value) return
  loading.value = true
  error.value = ''
  try {
    const resp = await listAuditLog(tenantId.value, {
      after_id: reset ? undefined : cursor.value || undefined,
      limit: PAGE_SIZE,
    })
    if (resp.success) {
      const rows = resp.data || []
      entries.value = reset ? rows : [...entries.value, ...rows]
      cursor.value = resp.next_cursor || 0
      hasMore.value = !!resp.next_cursor && rows.length > 0
      loadedOnce.value = true
      // 后端按页带回的操作人姓名（覆盖已移出/停用账号）叠加到名册映射
      // 之上，保证操作人列尽量不退化为裸 user_id。
      if (resp.actor_names && Object.keys(resp.actor_names).length > 0) {
        memberNameById.value = { ...memberNameById.value, ...resp.actor_names }
      }
    } else {
      error.value = resp.message || t('memberAudit.loadFailed')
    }
  } catch (err: any) {
    if (err?.status === 403) {
      error.value = t('memberAudit.forbidden')
    } else {
      error.value = err?.message || t('memberAudit.loadFailed')
    }
  } finally {
    loading.value = false
  }
}

function reload() {
  cursor.value = 0
  hasMore.value = true
  void load(true)
}

function detachObserver() {
  observer?.disconnect()
  observer = null
}

async function attachObserver() {
  detachObserver()
  await nextTick()
  const root = scrollRoot.value
  const sentinel = loadSentinel.value
  if (!root || !sentinel) return
  observer = new IntersectionObserver(
    (list) => {
      const hit = list.some((e) => e.isIntersecting)
      if (!hit || hasMore.value === false || loading.value) return
      void load(false)
    },
    { root, rootMargin: '100px 0px', threshold: 0 },
  )
  observer.observe(sentinel)
}

watch(visible, async (open) => {
  if (open) {
    entries.value = []
    cursor.value = 0
    hasMore.value = true
    void loadMemberNames()
    await load(true)
    void attachObserver()
  } else {
    detachObserver()
  }
})

onUnmounted(detachObserver)

function handleClose() {
  uiStore.closeMemberAudit()
}

// ---------- 白话文案 ----------
// access_denied 按请求路径细分：登录接口 → 疑似登录时密码错误；
// 其它 → 权限不足或登录过期的通俗说法。其余动作走 i18n 词条。
function friendlyAction(row: AuditLog): string {
  if (row.action === 'rbac.access_denied') {
    const path = String(row.request_path || '')
    if (path.includes('auth/login')) return t('memberAudit.loginDenied')
    return t('memberAudit.otherDenied')
  }
  return auditActionLabel({ tm }, AUDIT_ACTION_I18N_ROOTS.tenantMember, row.action)
}

function actionTheme(action: AuditAction): 'success' | 'warning' | 'danger' | 'default' {
  switch (action) {
    case 'rbac.access_denied':
      return 'danger'
    case 'rbac.member_added':
      return 'success'
    case 'rbac.member_removed':
    case 'rbac.member_left':
    case 'rbac.member_role_changed':
    case 'rbac.member_password_reset':
      return 'warning'
    default:
      return 'default'
  }
}

function outcomeLabel(o: AuditOutcome): string {
  if (o === 'success') return t('memberAudit.outcome.success')
  if (o === 'denied') return t('memberAudit.outcome.denied')
  return o
}

function actorDisplayName(userId: string): string {
  return memberNameById.value[userId] || userId
}

// 时间单行展示（2026/09/05 20:54:05），不折行。
function formatDateTime(s: string | undefined): string {
  if (!s) return '-'
  try {
    return new Intl.DateTimeFormat('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    }).format(new Date(s))
  } catch {
    return s
  }
}
</script>

<style lang="less" scoped>
.member-audit {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 200px;
}

.member-audit-toolbar {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;

  .member-audit-desc {
    margin: 0;
    font-size: 12px;
    line-height: 1.6;
    color: var(--td-text-color-secondary);
  }
}

.member-audit-hint {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px 0;
}

.member-audit-scroll {
  max-height: 420px;
  overflow-y: auto;
}

.member-audit-time {
  white-space: nowrap;
}

.member-audit-outcome {
  font-size: 13px;

  &.is-success {
    color: var(--td-success-color);
  }

  &.is-denied {
    color: var(--td-error-color);
  }
}

.member-audit-sentinel {
  padding: 12px 0;
  text-align: center;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}
</style>
