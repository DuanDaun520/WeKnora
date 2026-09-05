<template>
  <div class="panel-root sandbox-connections">
    <!-- 面板头与 UsersPanel / McpSettings 同款：标题 + 描述居左，刷新 / 新建按钮居右 -->
    <div class="panel-header">
      <div>
        <h2>{{ t('sandboxConnections.title') }}</h2>
        <p class="panel-header-desc">{{ t('sandboxConnections.description') }}</p>
      </div>
      <div class="panel-header-actions">
        <t-button variant="outline" :loading="loading" @click="loadConnections">
          <template #icon><t-icon name="refresh" /></template>
          {{ t('sandboxConnections.refresh') }}
        </t-button>
        <t-button theme="primary" @click="openCreate">
          <template #icon><t-icon name="add" /></template>
          {{ t('sandboxConnections.addConnection') }}
        </t-button>
      </div>
    </div>

    <div class="console-toolbar">
      <t-input
        v-model="searchQuery"
        class="toolbar-search"
        clearable
        :placeholder="t('sandboxConnections.searchPlaceholder')"
      >
        <template #prefix-icon><t-icon name="search" /></template>
      </t-input>
      <t-select
        v-model="typeFilter"
        class="toolbar-select"
        :placeholder="t('sandboxConnections.filterTypeAll')"
        clearable
      >
        <t-option
          v-for="type in NAMED_SANDBOX_BACKEND_TYPES"
          :key="type"
          :value="type"
          :label="t(`settings.sandbox.backends.${type}`)"
        />
      </t-select>
    </div>

    <div v-if="loading" class="loading-container">
      <t-loading :text="t('common.loading')" />
    </div>

    <div v-else-if="filteredConnections.length === 0" class="empty-state">
      <t-empty
        :description="hasActiveFilter
          ? t('sandboxConnections.noMatchHint')
          : t('sandboxConnections.emptyHint')"
      />
    </div>

    <!-- 与 McpSettings 同形的卡片：左侧类型徽章 + 名称 / 副标题行 + 分配 chips。
         分配 chip 带琥珀点 = 该空间的物化配置落后于连接（连接被编辑过未推送）。 -->
    <div v-else class="connection-grid">
      <div
        v-for="connection in filteredConnections"
        :key="connection.id"
        class="connection-card"
        role="button"
        tabindex="0"
        @click="onCardClick($event, connection)"
        @keydown.enter="onCardClick($event, connection)"
      >
        <div class="connection-card__badge">
          <SandboxBackendBadge :type="connection.sandbox_type" />
        </div>
        <div class="connection-card__body">
          <div class="connection-card__header">
            <h3 class="connection-card__title" :title="connection.name">{{ connection.name }}</h3>
            <div class="connection-card__actions" @click.stop>
              <t-dropdown
                :options="cardOptions(connection)"
                placement="bottom-right"
                attach="body"
                trigger="click"
                @click="(data: any) => handleMenuAction({ value: data.value }, connection)"
              >
                <t-button variant="text" shape="square" size="small" class="connection-card__more">
                  <t-icon name="ellipsis" />
                </t-button>
              </t-dropdown>
            </div>
          </div>
          <div class="connection-card__subtitle">
            <span class="connection-card__type">
              {{ t(`settings.sandbox.backends.${connection.sandbox_type}`) }}
            </span>
            <template v-if="connection.description">
              <span class="connection-card__sep">·</span>
              <span class="connection-card__desc" :title="connection.description">
                {{ connection.description }}
              </span>
            </template>
          </div>
          <div v-if="connection.updated_at" class="connection-card__time">
            {{ t('sandboxConnections.updatedAt') }}：{{ formatTime(connection.updated_at) }}
          </div>
          <div class="connection-card__assignments">
            <template v-if="connection.assignments?.length">
              <t-tooltip
                v-for="assignment in connection.assignments"
                :key="assignment.tenant_id"
                :content="assignment.drift
                  ? t('sandboxConnections.assignmentDriftHint')
                  : t('sandboxConnections.assignmentInSync')"
              >
                <t-tag
                  size="small"
                  variant="outline"
                  :class="{ 'connection-card__tenant-chip--drift': assignment.drift }"
                >
                  <span v-if="assignment.drift" class="connection-card__drift-dot" />
                  {{ assignment.tenant_name }}
                </t-tag>
              </t-tooltip>
            </template>
            <span v-else class="connection-card__assignments-empty">
              {{ t('sandboxConnections.unassigned') }}
            </span>
          </div>
        </div>
      </div>
    </div>

    <!-- 编辑 / 新建：同一个三步向导抽屉的 systemMode 变体 -->
    <SandboxConfigEditorDrawer
      v-model:visible="drawerVisible"
      :record="null"
      system-mode
      :source-connection="editing"
      @saved="handleSaved"
    />

    <!-- 推送结果：逐空间状态，被阻止的行带原因 -->
    <t-dialog
      v-model:visible="pushResultVisible"
      :header="t('sandboxConnections.pushResultTitle')"
      :confirm-btn="{ content: t('common.close'), theme: 'default' }"
      :cancel-btn="null"
      width="560px"
      attach="body"
    >
      <div class="push-summary">
        <t-tag theme="success" variant="light">
          {{ t('sandboxConnections.pushStatus.updated') }}：{{ pushResultSummary.updated }}
        </t-tag>
        <t-tag v-if="pushResultSummary.blocked" theme="warning" variant="light">
          {{ t('sandboxConnections.pushBlockedCount') }}：{{ pushResultSummary.blocked }}
        </t-tag>
      </div>
      <div class="push-rows">
        <div v-for="row in pushResultRows" :key="`${row.tenant_id}:${row.config_id}`" class="push-row">
          <span class="push-row__name" :title="row.tenant_name || `#${row.tenant_id}`">
            {{ row.tenant_name || `#${row.tenant_id}` }}
          </span>
          <t-tag
            :theme="pushStatusTheme(row.status)"
            variant="light"
            size="small"
          >
            {{ pushStatusLabel(row) }}
          </t-tag>
        </div>
      </div>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import SandboxBackendBadge from '@/components/settings/SandboxBackendBadge.vue'
import SandboxConfigEditorDrawer from '@/components/SandboxConfigEditorDrawer.vue'
import { useConfirmDelete } from '@/components/settings/useConfirmDelete'
import {
  deleteSystemSandboxConnection,
  listSystemSandboxConnections,
  pushSystemSandboxConnection,
  type PushOutcome,
  type PushResult,
  type SystemSandboxConnection,
} from '@/api/sandbox-connection'
import { NAMED_SANDBOX_BACKEND_TYPES } from '@/api/system'

const { t } = useI18n()
const confirmDelete = useConfirmDelete()

const connections = ref<SystemSandboxConnection[]>([])
const loading = ref(false)

// ---- 工具栏筛选（console-toolbar 同款：搜索 + 类型下拉）----
const searchQuery = ref('')
const typeFilter = ref('')

const hasActiveFilter = computed(() => !!searchQuery.value.trim() || !!typeFilter.value)

const filteredConnections = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  return connections.value.filter((connection) => {
    if (typeFilter.value && connection.sandbox_type !== typeFilter.value) return false
    if (!q) return true
    return [connection.name, connection.description].some(
      (field) => (field || '').toLowerCase().includes(q),
    )
  })
})

// ---- 抽屉 ----
const drawerVisible = ref(false)
const editing = ref<SystemSandboxConnection | null>(null)

// ---- 推送结果对话框 ----
const pushResultVisible = ref(false)
const pushResultRows = ref<PushOutcome[]>([])
const pushResultSummary = computed(() => ({
  updated: pushResultRows.value.filter((row) => row.status === 'updated').length,
  blocked: pushResultRows.value.filter(
    (row) => row.status === 'blocked_live_sandboxes' || row.status === 'blocked_skill_snapshot',
  ).length,
}))

const loadConnections = async () => {
  loading.value = true
  try {
    connections.value = await listSystemSandboxConnections()
  } catch (error) {
    MessagePlugin.error(t('sandboxConnections.toasts.loadFailed'))
    console.error('Failed to load sandbox connections:', error)
  } finally {
    loading.value = false
  }
}

const openCreate = () => {
  editing.value = null
  drawerVisible.value = true
}

const openEdit = (connection: SystemSandboxConnection) => {
  editing.value = { ...connection }
  drawerVisible.value = true
}

// The drawer emits saved after the connection (and, in edit mode, the
// assignment replace) landed — partially or fully. Reload in both cases so
// drift markers and assignment chips reflect reality.
const handleSaved = () => {
  void loadConnections()
}

const onCardClick = (event: Event, connection: SystemSandboxConnection) => {
  if (event.type === 'keydown') {
    const ke = event as KeyboardEvent
    if (ke.key !== 'Enter' && ke.key !== ' ') return
    ke.preventDefault()
  }
  const target = event.target as HTMLElement | null
  if (target?.closest('.connection-card__actions')) return
  openEdit(connection)
}

// ---- 推送：编辑不自动传播，唯一的扩散入口在这里 ----
const runPush = async (connection: SystemSandboxConnection) => {
  try {
    const result: PushResult = await pushSystemSandboxConnection(connection.id)
    pushResultRows.value = result.results || []
    pushResultVisible.value = true
    // Blocked outcomes leave drift markers on; refresh so the chips match.
    await loadConnections()
  } catch (error: any) {
    MessagePlugin.error(error?.message || t('sandboxConnections.toasts.pushFailed'))
  }
}

const pushStatusLabel = (row: PushOutcome): string => {
  switch (row.status) {
    case 'updated':
      return t('sandboxConnections.pushStatus.updated')
    case 'blocked_live_sandboxes':
      return t('sandboxConnections.pushStatus.blockedLive')
    case 'blocked_skill_snapshot':
      return t('sandboxConnections.pushStatus.blockedSnapshot')
    case 'skipped_cordoned':
      return t('sandboxConnections.pushStatus.skippedCordoned')
    default:
      return row.message || t('sandboxConnections.pushStatus.error')
  }
}

const pushStatusTheme = (status: string): 'success' | 'warning' | 'danger' | 'default' => {
  switch (status) {
    case 'updated':
      return 'success'
    case 'blocked_live_sandboxes':
    case 'blocked_skill_snapshot':
    case 'skipped_cordoned':
      return 'warning'
    default:
      return 'danger'
  }
}

// ---- 删除：有活分配时后端 409 assignments_exist（物化行是真实的工作空间
//      配置，可拥有技能与沙箱，不做平台级级联清除）----
const runDelete = (connection: SystemSandboxConnection) => {
  confirmDelete({
    body: t('sandboxConnections.deleteConfirmBody', { name: connection.name }),
    onConfirm: async () => {
      try {
        await deleteSystemSandboxConnection(connection.id)
        MessagePlugin.success(t('sandboxConnections.toasts.deleted'))
        await loadConnections()
      } catch (error: any) {
        if (error?.error?.code === 'assignments_exist') {
          MessagePlugin.warning(t('sandboxConnections.blockedAssignmentsExist'))
        } else {
          MessagePlugin.error(error?.message || t('sandboxConnections.toasts.deleteFailed'))
        }
      }
    },
  })
}

const cardOptions = (connection: SystemSandboxConnection) => {
  const options: Array<{ content: string; value: string; theme?: 'error' }> = [
    { content: t('common.edit'), value: 'edit' },
  ]
  // Push only means something while workspaces hold materialized rows.
  if (connection.assignments?.length) {
    options.push({ content: t('sandboxConnections.pushAction'), value: 'push' })
  }
  options.push({ content: t('common.delete'), value: 'delete', theme: 'error' })
  return options
}

const handleMenuAction = (data: { value: string }, connection: SystemSandboxConnection) => {
  switch (data.value) {
    case 'edit':
      openEdit(connection)
      break
    case 'push':
      runPush(connection)
      break
    case 'delete':
      runDelete(connection)
      break
  }
}

const formatTime = (value: string): string => {
  const ms = Date.parse(value)
  return Number.isNaN(ms) ? value : new Date(ms).toLocaleString()
}

onMounted(() => {
  loadConnections()
})
</script>

<style scoped lang="less">
@import './consolePanel.less';

.sandbox-connections {
  width: 100%;
}

.loading-container {
  padding: 40px 0;
  text-align: center;
}

// 空态：无连接或筛选无结果时占位（新建入口统一在 panel-header 右上角）
.empty-state {
  padding: 48px 0;
}

.connection-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 12px;
}

// 与 McpSettings 的 service-card 同形的卡片
.connection-card {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 14px 14px 14px 12px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 10px;
  background: var(--td-bg-color-container);
  transition: border-color 0.18s ease, box-shadow 0.18s ease;
  min-width: 0;
  cursor: pointer;

  &:hover {
    border-color: var(--td-brand-color-3, var(--td-brand-color));
    box-shadow: 0 4px 14px rgba(15, 23, 42, 0.06);
  }

  &:focus-visible {
    outline: 2px solid var(--td-brand-color);
    outline-offset: 2px;
  }
}

.connection-card__actions {
  flex-shrink: 0;
}

.connection-card__badge {
  flex-shrink: 0;
  margin-top: 1px;
}

.connection-card__body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.connection-card__header {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.connection-card__title {
  flex: 1;
  min-width: 0;
  margin: 0;
  font-size: 14px;
  font-weight: 600;
  line-height: 1.4;
  color: var(--td-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.connection-card__more {
  flex-shrink: 0;
  color: var(--td-text-color-placeholder);
  padding: 2px;
  opacity: 0;
  transition: opacity 0.15s ease;

  &:hover,
  &:focus-visible {
    background: var(--td-bg-color-secondarycontainer);
    color: var(--td-text-color-primary);
  }
}

.connection-card:hover .connection-card__more,
.connection-card:focus-within .connection-card__more,
.connection-card__actions:focus-within .connection-card__more {
  opacity: 1;
}

.connection-card__subtitle {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px;
  font-size: 12px;
  line-height: 1.4;
  color: var(--td-text-color-secondary);
  min-width: 0;
}

.connection-card__type {
  font-weight: 500;
}

.connection-card__sep {
  color: var(--td-text-color-placeholder);
}

.connection-card__desc {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}

.connection-card__time {
  font-size: 11px;
  line-height: 1.4;
  color: var(--td-text-color-placeholder);
}

// 已分配空间 chips：drift 的行带琥珀点（连接编辑过、还没推送）
.connection-card__assignments {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 2px;

  :deep(.t-tag) {
    max-width: 100%;
  }
}

.connection-card__assignments-empty {
  font-size: 11px;
  line-height: 1.4;
  color: var(--td-text-color-placeholder);
}

.connection-card__tenant-chip--drift {
  color: var(--td-warning-color-7, #B85C00);
  border-color: var(--td-warning-color-5, #E37318);
}

.connection-card__drift-dot {
  display: inline-block;
  width: 6px;
  height: 6px;
  margin-right: 4px;
  border-radius: 50%;
  background: var(--td-warning-color, #ED7B2F);
  vertical-align: middle;
}

// ---- 推送结果对话框 ----
.push-summary {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
}

.push-rows {
  display: flex;
  flex-direction: column;
  max-height: 320px;
  overflow-y: auto;
}

.push-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 7px 0;

  & + .push-row {
    border-top: 1px solid var(--td-component-stroke);
  }
}

.push-row__name {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  color: var(--td-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
