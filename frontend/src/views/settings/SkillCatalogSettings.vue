<template>
  <div class="skill-settings">
    <div class="section-header">
      <div class="section-header__title-row">
        <h2>{{ $t('settings.skills.title') }}</h2>
        <t-tooltip :content="$t('settings.skills.helpTooltip')" placement="right"
          overlay-class-name="skill-settings__help-tooltip">
          <t-icon name="help-circle" class="section-header__help" :aria-label="$t('settings.skills.helpTooltip')" />
        </t-tooltip>
      </div>
      <p class="section-description">{{ $t('settings.skills.description') }}</p>
      <!-- 000099 空间侧只读：目录由系统管理员维护，空间管理员只看不动 -->
      <p v-if="!canManage" class="installer-model-hint">{{ $t('settings.skills.readonlyHint') }}</p>
    </div>

    <div v-if="loading" class="loading-container">
      <t-loading :text="$t('common.loading')" />
    </div>

    <template v-else>
      <div v-if="catalog.length === 0" class="empty-state">
        <t-empty :description="canManage ? $t('settings.skills.emptyDescManaged') : $t('settings.skills.emptyDescReadonly')" />
        <p v-if="canManage && skillConfigs.length === 0" class="empty-hint">
          {{ $t('settings.skills.emptyNoSandboxHint') }}
        </p>
        <div v-if="canManage && skillConfigs.length === 0" class="empty-actions">
          <t-button theme="default" variant="outline" @click="goSandboxSection">
            {{ $t('settings.skills.goSandboxSettings') }}
          </t-button>
        </div>
      </div>

      <div v-else class="skill-list">
        <article v-for="item in catalog" :key="item.id" class="skill-card" :class="{
          'skill-card--installed': liveInstalls(item).length > 0,
          'skill-card--idle': liveInstalls(item).length === 0,
        }">
          <div class="skill-card__main">
            <div class="skill-card__badge" aria-hidden="true">
              <t-icon :name="SKILL_ICON" size="16px" />
            </div>
            <div class="skill-card__body">
              <div class="skill-card__header">
                <div class="skill-card__heading">
                  <!-- 技能英文名独占一行，不再被版本/分类/溯源 pill 挤压 -->
                  <h3 class="skill-card__title" :title="item.name">{{ item.name }}</h3>
                  <div v-if="item.source_platform_skill_id || item.version || item.category || item.visible === false" class="skill-card__tags">
                    <!-- 000108：本空间已隐藏（管理页 include_hidden 才会列出） -->
                    <span v-if="item.visible === false" class="skill-card__hidden">
                      {{ $t('settings.skills.hiddenInSpace') }}
                    </span>
                    <!-- 000098 溯源：该目录行由平台「技能库」分配物化而来 -->
                    <span v-if="item.source_platform_skill_id" class="skill-card__from-library">
                      {{ $t('settings.skills.fromLibrary') }}
                    </span>
                    <span v-if="item.version" class="skill-card__type">{{ item.version }}</span>
                    <!-- 分类：Skills/MCP 浏览页分组；空间管理员/系统管理员可就地改 -->
                    <span v-if="item.category" class="skill-card__category" :title="item.category">
                      {{ item.category }}
                    </span>
                  </div>
                </div>
                <div class="skill-card__actions">
                  <!-- 000108：空间可见开关（隐藏后从 Skills/MCP 浏览、智能体选技能
                       与引用/运行时消失；列表本身用 include_hidden 保留可管理） -->
                  <t-switch
                    v-if="canManage"
                    size="small"
                    class="skill-card__visible-switch"
                    :value="item.visible !== false"
                    :disabled="savingVisibleId === item.id"
                    :aria-label="$t('settings.skills.spaceVisibleAria')"
                    @change="(v: boolean) => setSpaceVisible(item, v)"
                  />
                  <!-- 修改分类：t-popup 内联编辑（与安装面板同款受控弹层） -->
                  <t-popup v-if="canManage" :visible="editingCategoryId === item.id" trigger="click"
                    placement="bottom-right" attach="body" destroy-on-close
                    overlay-class-name="skill-category-editor-overlay"
                    :overlay-inner-style="{ padding: 0 }"
                    @visible-change="(visible: boolean) => setCategoryEditor(item.id, visible)">
                    <button type="button" class="skill-card__icon-btn"
                      :title="$t('settings.skills.editCategory')" :aria-label="$t('settings.skills.editCategory')">
                      <t-icon name="edit-1" size="14px" />
                    </button>
                    <template #content>
                      <div class="skill-category-editor">
                        <t-input v-model="editingCategoryValue" maxlength="255"
                          :placeholder="$t('settings.skills.categoryPlaceholder')" :disabled="savingCategoryId === item.id"
                          @enter="saveCategory(item)" />
                        <t-button size="small" theme="primary" :loading="savingCategoryId === item.id"
                          @click="saveCategory(item)">
                          {{ $t('common.save') }}
                        </t-button>
                      </div>
                    </template>
                  </t-popup>
                  <button type="button" class="skill-card__icon-btn" :title="$t('settings.sandbox.skillFiles')"
                    :aria-label="$t('settings.sandbox.skillFiles')" @click="openCatalogFiles(item)">
                    <folder-icon size="14px" />
                  </button>
                  <button v-if="canManage && canDelete(item)" type="button" class="skill-card__icon-btn skill-card__icon-btn--danger"
                    :disabled="deletingId === item.id" :title="$t('settings.skills.deleteCatalog')"
                    :aria-label="$t('settings.skills.deleteCatalog')" @click="askDelete(item)">
                    <delete-icon size="14px" />
                  </button>
                </div>
              </div>
              <p v-if="item.description" class="skill-card__desc" :title="item.description">
                {{ compactText(item.description) }}
              </p>
              <div v-for="view in [installsView(item)]" :key="'installs'" class="skill-card__installs">
                <span v-if="view.installs.length === 0 && (!view.canAdd || !canManage)" class="skill-card__installs-label">
                  {{ $t('settings.skills.noInstalls') }}
                </span>
                <!-- 只读：渲染同款 chip 但不可点（无 chevron / 弹层 / 安装入口） -->
                <span v-else-if="!canManage" class="skill-card__chip" :class="chipClass(item, view)"
                  :title="installSummaryTooltip(item, view)" :aria-label="installSummary(item, view)">
                  <span v-if="view.installs.some(isInstallBusy)" class="skill-card__entry-dot" aria-hidden="true" />
                  <span class="skill-card__chip-text">{{ installSummary(item, view) }}</span>
                </span>
                <button v-else-if="!view.needsPanel" type="button" class="skill-card__chip"
                  :class="chipClass(item, view)"
                  :disabled="Boolean(view.installs[0] && !recordFor(view.installs[0].sandbox_config_id))"
                  :title="installSummaryTooltip(item, view)" :aria-label="installSummary(item, view)"
                  @click="onInstallChipClick(item, view)">
                  <span v-if="view.installs.some(isInstallBusy)" class="skill-card__entry-dot" aria-hidden="true" />
                  <span class="skill-card__chip-text">{{ installSummary(item, view) }}</span>
                  <t-icon name="chevron-right" size="14px" class="skill-card__chip-go" />
                </button>
                <t-popup v-else :visible="openPanelId === item.id" trigger="click" placement="bottom-left" attach="body"
                  destroy-on-close overlay-class-name="skill-install-panel-overlay"
                  :overlay-inner-style="{ padding: 0 }"
                  @visible-change="(visible: boolean) => setInstallPanel(item.id, visible)">
                  <button type="button" class="skill-card__chip" :class="chipClass(item, view)"
                    :title="installSummaryTooltip(item, view)" :aria-label="installSummary(item, view)"
                    :aria-expanded="openPanelId === item.id">
                    <span v-if="view.installs.some(isInstallBusy)" class="skill-card__entry-dot" aria-hidden="true" />
                    <span class="skill-card__chip-text">{{ installSummary(item, view) }}</span>
                    <t-icon name="chevron-down" size="14px" class="skill-card__chip-go" />
                  </button>
                  <template #content>
                    <div class="skill-install-panel">
                      <template v-if="view.installs.length > 0">
                        <p class="skill-install-panel__group">{{ $t('settings.skills.installPanelGroup') }}</p>
                        <button v-for="inst in view.installs" :key="inst.skill_id" type="button"
                          class="skill-install-panel__item" :class="installEntryClass(item, inst)"
                          :disabled="!recordFor(inst.sandbox_config_id)" :title="installTooltip(item, inst)"
                          @click="openManageFromPanel(item, inst)">
                          <SandboxBackendBadge v-if="inst.sandbox_type" :type="inst.sandbox_type" size="xs" />
                          <span class="skill-install-panel__name">{{ installName(inst) }}</span>
                          <span v-if="isInstallBusy(inst)" class="skill-card__entry-dot" aria-hidden="true" />
                          <t-icon v-else-if="installChipStatusIcon(item, inst)"
                            :name="installChipStatusIcon(item, inst)" size="14px" class="skill-card__entry-status" />
                        </button>
                      </template>
                      <template v-if="view.available.length > 0">
                        <div v-if="view.installs.length > 0" class="skill-install-panel__split" role="separator" />
                        <p class="skill-install-panel__group">{{ $t('settings.skills.installPanelAvailable') }}</p>
                        <button v-for="cfg in view.available" :key="cfg.id" type="button"
                          class="skill-install-panel__item skill-install-panel__item--available"
                          :title="sandboxMetaLine(cfg)" @click="openInstallTo(item, cfg)">
                          <SandboxBackendBadge :type="cfg.sandbox_type" size="xs" />
                          <span class="skill-install-panel__name">{{ cfg.name }}</span>
                          <t-icon name="add" size="14px" class="skill-install-panel__add" />
                        </button>
                      </template>
                    </div>
                  </template>
                </t-popup>
              </div>
            </div>
          </div>
        </article>
      </div>
    </template>

    <SettingDrawer v-model:visible="showInstall" :title="$t('settings.skills.installToSandbox')"
      :description="installDrawerDesc" :icon="SKILL_ICON" width="560px" :min-width="480" :max-width="760"
      storage-key="setting-drawer:width:skill-catalog-install" :confirm-loading="installing"
      :confirm-disabled="installConfirmDisabled" :confirm-text="installConfirmText" @confirm="onInstallDrawerConfirm">
      <p class="installer-model-hint">{{ $t('settings.skills.installToSandboxDesc') }}</p>
      <section v-if="installPickRows.length > 0" class="setting-drawer__section">
        <div class="sandbox-pick-list">
          <div v-for="row in installPickRows" :key="row.cfg.id" class="sandbox-pick-row"
            :class="{ 'is-busy': row.busy, 'is-ready': row.ready }">
            <t-checkbox v-if="row.selectable" :checked="installTargetIds.includes(row.cfg.id)" :disabled="installing"
              class="sandbox-pick" @change="(checked: boolean) => setInstallPick(row.cfg.id, checked)">
              <span class="sandbox-pick__main">
                <SandboxBackendBadge :type="row.cfg.sandbox_type" size="sm" />
                <span class="sandbox-pick__text">
                  <span class="sandbox-pick__name">{{ row.cfg.name }}</span>
                  <span class="sandbox-pick__meta">{{ sandboxMetaLine(row.cfg) }}</span>
                </span>
              </span>
            </t-checkbox>
            <div v-else class="sandbox-pick sandbox-pick--status">
              <span class="sandbox-pick__main">
                <SandboxBackendBadge :type="row.cfg.sandbox_type" size="sm" />
                <span class="sandbox-pick__text">
                  <span class="sandbox-pick__name">{{ row.cfg.name }}</span>
                  <span class="sandbox-pick__meta">{{ sandboxPickStatus(row) }}</span>
                </span>
              </span>
              <div v-if="row.busy" class="sandbox-pick__progress">
                <t-progress theme="circle" :percentage="sandboxPickPercent(row) ?? 0" :size="18" :stroke-width="2"
                  :label="false" />
                <span v-if="sandboxPickPercent(row) != null">{{ sandboxPickPercent(row) }}%</span>
              </div>
              <t-button v-if="row.busy && row.install" size="small" variant="text" theme="primary"
                @click="openManageFromPick(installCatalog?.id, row.install)">
                {{ $t('settings.skills.viewInstallProgress') }}
              </t-button>
            </div>
          </div>
        </div>
      </section>
      <p v-else class="installer-model-hint">{{ $t('settings.skills.noSandboxToInstall') }}</p>
      <section v-if="installTargetIds.length > 0" class="setting-drawer__section">
        <h4 class="setting-drawer__section-title">{{ $t('settings.sandbox.skillInstallerModel') }}</h4>
        <p class="installer-model-hint">{{ $t('settings.sandbox.skillInstallerModelHint') }}</p>
        <ModelSelector model-type="KnowledgeQA" :selected-model-id="installerModelId"
          :disabled="savingInstallerModel || installing" @update:selected-model-id="onInstallerModelChange" />
      </section>
    </SettingDrawer>

    <SettingDrawer v-model:visible="showManage" :title="manageTitle" :description="manageDesc" :icon="SKILL_ICON"
      width="680px" :min-width="560" :max-width="920" storage-key="setting-drawer:width:skill-catalog-manage"
      :hide-footer="true" :z-index="2600">
      <SandboxSkillsPanel v-if="showManage && manageRecord && manageSkillId" :record="manageRecord" mode="list" hide-add
        :focus-skill-id="manageSkillId" @updated="onPanelUpdated" @skills-changed="loadCatalog" />
    </SettingDrawer>

    <SkillFilesDrawer v-model:visible="filesDrawerVisible" :catalog-id="filesCatalogId"
      :skill-name="filesCatalogName" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { DeleteIcon, FolderIcon } from 'tdesign-icons-vue-next'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import SandboxSkillsPanel from '@/components/SandboxSkillsPanel.vue'
import SkillFilesDrawer from '@/components/SkillFilesDrawer.vue'
import SandboxBackendBadge from '@/components/settings/SandboxBackendBadge.vue'
import SettingDrawer from '@/components/settings/SettingDrawer.vue'
import ModelSelector from '@/components/ModelSelector.vue'
import { useConfirmDelete } from '@/components/settings/useConfirmDelete'
import { useConfigSkillInstallProgress } from '@/composables/useConfigSkillInstallProgress'
import { SKILL_ICON } from '@/types/mention'
import {
  deleteSkillCatalog,
  installSkillCatalog,
  listSkillCatalog,
  updateSkillCatalogMeta,
  type SkillCatalogInstall,
  type SkillCatalogItem,
} from '@/api/skill'
import {
  getAgentById,
  updateAgent,
  type CustomAgent,
} from '@/api/agent'
import {
  isNamedSandboxBackend,
  listSandboxConfigs,
  type SandboxConfigRecord,
} from '@/api/system'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const confirmDelete = useConfirmDelete()

// 000107 空间侧技能目录：安装/启停/删除后端放宽到 AdminOrSystemAdmin —— 本
// 空间 admin/owner 可对**自己空间**的沙箱做技能镜像管理，平台系统管理员照旧
// （hasRole 对系统管理员按 admin 旁路）。hasRole 只是 UI 对齐，后端守卫才是
// 权威；登记入口仍移除（平台单向分配），这里只消费平台物化出的目录行。
const canManage = computed(() => authStore.hasRole('admin'))

// 空态「前往沙箱配置」：沙箱配置已并入控制台「沙箱连接」，从空间 Settings
// 跨路由跳过去；保留 ?tenant=（无害：连接面板不读它）。
function goSandboxSection() {
  void router.push({
    path: '/system/console',
    query: { ...route.query, section: 'sandbox-connections' },
  })
}

const loading = ref(false)
const records = ref<SandboxConfigRecord[]>([])
const catalog = ref<SkillCatalogItem[]>([])
const deletingId = ref('')
const showInstall = ref(false)
const showManage = ref(false)
const installTargetIds = ref<string[]>([])
const installSessionIds = ref<string[]>([])
const installCatalog = ref<SkillCatalogItem | null>(null)
const manageRecord = ref<SandboxConfigRecord | null>(null)
const manageSkillId = ref('')
const manageTitle = ref('')
const filesDrawerVisible = ref(false)
const filesCatalogId = ref('')
const filesCatalogName = ref('')
const openPanelId = ref('')
// 分类编辑（SystemAdmin）：卡片内 t-popup 受控状态
const editingCategoryId = ref('')
const editingCategoryValue = ref('')
const savingCategoryId = ref('')
// 000108：空间可见开关正在保存的技能
const savingVisibleId = ref('')
const installing = ref(false)
const installerAgent = ref<CustomAgent | null>(null)
const installerModelId = ref('')
const savingInstallerModel = ref(false)

const INSTALLER_AGENT_ID = 'builtin-skill-installer'
const LAST_CHAT_MODEL_KEY = 'weknora_last_chat_model_id'

const {
  percentOf: installEventPercent,
  sync: syncInstallProgress,
  stopAll: stopInstallProgress,
} = useConfigSkillInstallProgress({
  onDone() {
    void loadCatalog(true)
  },
})

let pollTimer: number | null = null

const skillConfigs = computed(() =>
  records.value.filter((record) => isNamedSandboxBackend(record.sandbox_type)),
)

const installConfirmText = computed(() =>
  installTargetIds.value.length > 0
    ? t('settings.skills.installToSandbox')
    : t('settings.skills.addFinish'),
)

const installConfirmDisabled = computed(() =>
  installing.value || (installTargetIds.value.length > 0 && !installerModelId.value),
)

const installPickRows = computed(() =>
  sandboxPickRows(catalogItemById(installCatalog.value?.id), installSessionIds.value),
)

const installDrawerDesc = computed(() => {
  const item = installCatalog.value
  if (!item) return t('settings.skills.installToSandboxDesc')
  return t('settings.skills.installDrawerDesc', { name: item.name })
})

const manageDesc = computed(() => {
  const record = manageRecord.value
  if (!record) return ''
  return t('settings.skills.manageDrawerDesc', { name: record.name })
})

function liveInstalls(item: SkillCatalogItem): SkillCatalogInstall[] {
  return (item.installations || []).filter((inst) => inst.status && inst.status !== 'removed')
}

function canDelete(item: SkillCatalogItem): boolean {
  return liveInstalls(item).length === 0
}

function targetsFor(item: SkillCatalogItem): SandboxConfigRecord[] {
  const taken = new Set(
    liveInstalls(item)
      .filter((inst) => inst.status === 'installing' || inst.status === 'ready' || inst.status === 'removing')
      .map((inst) => inst.sandbox_config_id),
  )
  return skillConfigs.value.filter((cfg) => !taken.has(cfg.id))
}

function recordFor(id: string): SandboxConfigRecord | undefined {
  return records.value.find((record) => record.id === id)
}

function backendLabel(type: string): string {
  return t(`settings.sandbox.backends.${type}`)
}

function sandboxTargetLine(record: SandboxConfigRecord): string {
  if (record.sandbox_type === 'docker') {
    return record.config?.docker?.image?.trim() || ''
  }
  const remote = record.config?.e2b || record.config?.cube
  const raw = remote?.api_url?.trim() || ''
  if (!raw) return ''
  try {
    return new URL(raw).host
  } catch {
    return raw
  }
}

function sandboxMetaLine(record: SandboxConfigRecord): string {
  const type = backendLabel(record.sandbox_type)
  const target = sandboxTargetLine(record)
  return target ? `${type} · ${target}` : type
}

type SandboxPickRow = {
  cfg: SandboxConfigRecord
  install?: SkillCatalogInstall
  selectable: boolean
  busy: boolean
  ready: boolean
}

function catalogItemById(id: string | undefined | null): SkillCatalogItem | null {
  const key = (id || '').trim()
  if (!key) return null
  return catalog.value.find((row) => row.id === key) || null
}

function sandboxPickRows(
  item: SkillCatalogItem | null,
  sessionIds: string[],
): SandboxPickRow[] {
  const byId = new Map(
    (item ? liveInstalls(item) : []).map((inst) => [inst.sandbox_config_id, inst]),
  )
  const session = new Set(sessionIds)
  return skillConfigs.value
    .filter((cfg) => {
      const inst = byId.get(cfg.id)
      if (session.has(cfg.id)) return true
      if (inst && isInstallBusy(inst)) return true
      if (!inst || inst.status === 'failed') return true
      return false
    })
    .map((cfg) => {
      const install = byId.get(cfg.id)
      const busy = Boolean(install && isInstallBusy(install))
      const ready = install?.status === 'ready'
      return {
        cfg,
        install,
        selectable: !busy && !ready,
        busy,
        ready,
      }
    })
}

function sandboxPickPercent(row: SandboxPickRow): number | null {
  if (!row.busy || !row.install) return null
  return installEventPercent(row.cfg.id, row.install.skill_id)
}

function sandboxPickStatus(row: SandboxPickRow): string {
  if (row.install && isInstallBusy(row.install)) return installStatusText(row.install)
  if (row.ready) return t('settings.sandbox.skillStatusReady')
  return sandboxMetaLine(row.cfg)
}

function setInstallPick(id: string, checked: boolean) {
  setPickId(installTargetIds, id, checked)
}

function setPickId(ids: { value: string[] }, id: string, checked: boolean) {
  const on = Boolean(checked)
  if (on) {
    if (!ids.value.includes(id)) ids.value = [...ids.value, id]
    return
  }
  ids.value = ids.value.filter((item) => item !== id)
}

function prunePicks(ids: { value: string[] }, rows: SandboxPickRow[]) {
  const allowed = new Set(rows.filter((row) => row.selectable).map((row) => row.cfg.id))
  ids.value = ids.value.filter((id) => allowed.has(id))
}

function rememberSession(session: { value: string[] }, ids: string[]) {
  session.value = [...new Set([...session.value, ...ids])]
}

function compactText(value: string): string {
  return value.replace(/\s+/g, ' ').trim()
}

function installOutdated(item: SkillCatalogItem, inst: SkillCatalogInstall): boolean {
  return Boolean(
    item.bundle_sha256
    && inst.bundle_sha256
    && item.bundle_sha256 !== inst.bundle_sha256,
  )
}

function installStatusText(inst: SkillCatalogInstall): string {
  if (inst.status === 'installing') return t('settings.sandbox.skillStatusInstalling')
  if (inst.status === 'removing') return t('settings.sandbox.skillStatusRemoving')
  if (inst.status === 'failed') return t('settings.sandbox.skillStatusFailed')
  if (inst.status === 'ready' && !inst.enabled) return t('common.off')
  if (inst.status === 'ready') return t('settings.sandbox.skillStatusReady')
  return inst.status
}

function installChipStatus(item: SkillCatalogItem, inst: SkillCatalogInstall): string {
  if (inst.status === 'ready' && inst.enabled && installOutdated(item, inst)) {
    return t('settings.skills.installOutdated')
  }
  if (inst.status === 'ready' && inst.enabled) return ''
  return installStatusText(inst)
}

function installChipStatusIcon(item: SkillCatalogItem, inst: SkillCatalogInstall): string {
  if (inst.status === 'failed') return 'close-circle'
  if (inst.status === 'ready' && inst.enabled && installOutdated(item, inst)) return 'error-circle'
  if (inst.status === 'ready' && inst.enabled) return 'check-circle-filled'
  return ''
}

function isInstallBusy(inst: SkillCatalogInstall): boolean {
  return inst.status === 'installing' || inst.status === 'removing'
}

function installPriority(item: SkillCatalogItem, inst: SkillCatalogInstall): number {
  if (inst.status === 'failed') return 0
  if (isInstallBusy(inst)) return 1
  if (installOutdated(item, inst)) return 2
  if (inst.status === 'ready' && !inst.enabled) return 3
  return 4
}

function unusedTargets(item: SkillCatalogItem): SandboxConfigRecord[] {
  const live = new Set(liveInstalls(item).map((inst) => inst.sandbox_config_id))
  return skillConfigs.value
    .filter((cfg) => !live.has(cfg.id))
    .sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }))
}

function installsView(item: SkillCatalogItem) {
  const installs = [...liveInstalls(item)].sort((a, b) => {
    const diff = installPriority(item, a) - installPriority(item, b)
    if (diff !== 0) return diff
    return installName(a).localeCompare(installName(b), undefined, { sensitivity: 'base' })
  })
  const available = unusedTargets(item)
  return {
    installs,
    available,
    canAdd: available.length > 0,
    needsPanel: installs.length + available.length > 1,
  }
}

function installSummary(item: SkillCatalogItem, view: ReturnType<typeof installsView>): string {
  if (view.installs.length === 0) {
    // 可装（有可选沙箱）时给动作文案；纯只读空态给「尚未装到任何沙箱」。
    return view.canAdd
      ? t('settings.skills.installToSandbox')
      : t('settings.skills.noInstalls')
  }
  if (view.installs.length === 1) {
    // 单个安装：chip 文字反映真实状态，而不是一律写「已安装到沙箱名」——
    // 安装中/失败/停用的技能不该伪装成已就绪（颜色只做辅助，文字才是语义）。
    const inst = view.installs[0]
    if (isInstallBusy(inst)) return installStatusText(inst) // 安装中 / 删除中
    if (inst.status === 'failed') return t('settings.sandbox.skillStatusFailed')
    if (inst.status === 'ready' && !inst.enabled) return t('settings.skills.disabledOnSandbox')
    // ready && enabled：真正可用。文案不再暴露沙箱名（空间自建/平台推送通用）。
    return t('settings.skills.installedOnSandbox')
  }
  return t('settings.skills.installedCount', { count: view.installs.length })
}

function chipClass(item: SkillCatalogItem, view: ReturnType<typeof installsView>): (string | undefined)[] {
  return [
    view.installs.length === 0 ? 'skill-card__chip--idle' : 'skill-card__chip--installed',
    view.installs[0] ? installEntryClass(item, view.installs[0]) : undefined,
  ]
}

function installSummaryTooltip(item: SkillCatalogItem, view: ReturnType<typeof installsView>): string {
  const lines = view.installs.map((inst) => installTooltip(item, inst))
  for (const cfg of view.available) {
    const meta = sandboxMetaLine(cfg)
    lines.push(
      meta
        ? `${cfg.name} · ${t('settings.skills.installPanelAvailable')} · ${meta}`
        : `${cfg.name} · ${t('settings.skills.installPanelAvailable')}`,
    )
  }
  if (lines.length === 0) return t('settings.skills.installToSandbox')
  return lines.join('\n')
}

function onInstallChipClick(item: SkillCatalogItem, view: ReturnType<typeof installsView>) {
  if (view.installs[0]) {
    openManage(item, view.installs[0])
    return
  }
  if (view.canAdd) openInstall(item)
}

function setInstallPanel(id: string, visible: boolean) {
  if (visible) {
    openPanelId.value = id
    return
  }
  if (openPanelId.value === id) openPanelId.value = ''
}

// 分类编辑弹层：打开时用当前值播种，关闭时清空（与安装面板同一受控模式）。
function setCategoryEditor(id: string, visible: boolean) {
  if (visible) {
    editingCategoryValue.value = catalog.value.find((row) => row.id === id)?.category || ''
    editingCategoryId.value = id
    return
  }
  if (editingCategoryId.value === id) editingCategoryId.value = ''
}

async function saveCategory(item: SkillCatalogItem) {
  const category = editingCategoryValue.value.trim()
  if (category.length > 255) {
    MessagePlugin.warning(t('settings.skills.categorySaveFailed'))
    return
  }
  if (category === (item.category || '')) {
    editingCategoryId.value = ''
    return
  }
  savingCategoryId.value = item.id
  try {
    await updateSkillCatalogMeta(item.id, category)
    MessagePlugin.success(t('settings.skills.categorySaved'))
    editingCategoryId.value = ''
    await loadCatalog(true)
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('settings.skills.categorySaveFailed'))
  } finally {
    savingCategoryId.value = ''
  }
}

// 000108：空间内显示/隐藏。切换不影响分类，故把当前分类一并带上（后端 PUT
// 会写 category，空字符串会把分类清空）。
async function setSpaceVisible(item: SkillCatalogItem, visible: boolean) {
  if (savingVisibleId.value) return
  savingVisibleId.value = item.id
  try {
    await updateSkillCatalogMeta(item.id, item.category || '', visible)
    MessagePlugin.success(
      visible ? t('settings.skills.showToSpace') : t('settings.skills.hideFromSpace'),
    )
    await loadCatalog(true)
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('settings.skills.categorySaveFailed'))
  } finally {
    savingVisibleId.value = ''
  }
}

function openManageFromPanel(item: SkillCatalogItem, inst: SkillCatalogInstall) {
  openPanelId.value = ''
  openManage(item, inst)
}

function openInstallTo(item: SkillCatalogItem, cfg: SandboxConfigRecord) {
  openPanelId.value = ''
  installCatalog.value = item
  installSessionIds.value = []
  installTargetIds.value = [cfg.id]
  void loadInstallerModel()
  showInstall.value = true
}

function openManageFromPick(catalogId: string | undefined, inst: SkillCatalogInstall) {
  const item = catalogItemById(catalogId)
  if (!item) return
  openManage(item, inst)
}

function installEntryClass(item: SkillCatalogItem, inst: SkillCatalogInstall): string {
  if (inst.status === 'failed') return 'skill-card__entry--failed'
  if (isInstallBusy(inst)) return 'skill-card__entry--busy'
  if (inst.status === 'ready' && !inst.enabled) return 'skill-card__entry--off'
  if (installOutdated(item, inst)) return 'skill-card__entry--stale'
  return 'skill-card__entry--ready'
}

function installName(inst: SkillCatalogInstall): string {
  return inst.sandbox_config_name || inst.sandbox_config_id
}

function installTooltip(item: SkillCatalogItem, inst: SkillCatalogInstall): string {
  const parts = [
    inst.sandbox_config_name || inst.sandbox_config_id,
    inst.sandbox_type ? backendLabel(inst.sandbox_type) : '',
    installChipStatus(item, inst) || installStatusText(inst),
  ].filter(Boolean)
  return parts.join(' · ')
}

function openCatalogFiles(item: SkillCatalogItem) {
  filesCatalogId.value = item.id
  filesCatalogName.value = item.name
  filesDrawerVisible.value = true
}

function askDelete(item: SkillCatalogItem) {
  if (!canDelete(item)) {
    MessagePlugin.warning(t('settings.skills.deleteCatalogBlocked'))
    return
  }
  confirmDelete({
    body: t('settings.skills.deleteCatalogConfirm', { name: item.name }),
    onConfirm: () => removeCatalog(item),
  })
}

function openInstall(item: SkillCatalogItem) {
  if (!canManage.value) return
  installCatalog.value = item
  installSessionIds.value = []
  const remaining = targetsFor(item)
  installTargetIds.value = remaining.length === 1 ? [remaining[0].id] : []
  void loadInstallerModel()
  showInstall.value = true
}

function openManage(item: SkillCatalogItem, inst: SkillCatalogInstall) {
  if (!canManage.value) return
  const record = recordFor(inst.sandbox_config_id)
  if (!record) return
  manageRecord.value = record
  manageSkillId.value = inst.skill_id
  manageTitle.value = item.name
  showManage.value = true
}

function onPanelUpdated(record: SandboxConfigRecord) {
  records.value = records.value.map((item) => (item.id === record.id ? { ...item, ...record } : item))
}

function readLastChatModelID(): string {
  try {
    return localStorage.getItem(LAST_CHAT_MODEL_KEY) || ''
  } catch {
    return ''
  }
}

async function loadInstallerModel() {
  try {
    const res = await getAgentById(INSTALLER_AGENT_ID)
    installerAgent.value = res?.data || null
    const configured = installerAgent.value?.config?.model_id?.trim() || ''
    installerModelId.value = configured || readLastChatModelID()
  } catch {
    installerAgent.value = null
    installerModelId.value = readLastChatModelID()
  }
}

async function persistInstallerModel(modelId: string) {
  const id = modelId.trim()
  if (!id) {
    throw new Error(t('settings.sandbox.skillInstallerModelRequired'))
  }
  const current = installerAgent.value
  const config = { ...(current?.config || {}), model_id: id }
  const res = await updateAgent(INSTALLER_AGENT_ID, {
    name: current?.name || '',
    description: current?.description || '',
    avatar: current?.avatar || '',
    config,
  })
  installerAgent.value = res?.data || { ...(current as CustomAgent), config }
  installerModelId.value = id
}

async function onInstallerModelChange(modelId: string) {
  if (!modelId || modelId === '__add_model__') return
  installerModelId.value = modelId
  savingInstallerModel.value = true
  try {
    await persistInstallerModel(modelId)
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('settings.sandbox.skillInstallerModelSaveFailed'))
  } finally {
    savingInstallerModel.value = false
  }
}

async function ensureInstallerModelIfNeeded(configIds: string[]) {
  if (configIds.length === 0) return
  if (!installerModelId.value) {
    throw new Error(t('settings.sandbox.skillInstallerModelRequired'))
  }
  await persistInstallerModel(installerModelId.value)
}

function catalogInstallFailedCount(res: { data?: { errors?: Record<string, string> } } | null | undefined): number {
  return Object.keys(res?.data?.errors || {}).length
}

function onInstallDrawerConfirm() {
  if (installTargetIds.value.length === 0) {
    showInstall.value = false
    return
  }
  void confirmInstall()
}

async function confirmInstall() {
  const item = installCatalog.value
  const targets = [...installTargetIds.value]
  if (!item || targets.length === 0) return
  installing.value = true
  try {
    await ensureInstallerModelIfNeeded(targets)
    const res = await installSkillCatalog(item.id, targets)
    const failed = catalogInstallFailedCount(res)
    if (failed > 0) {
      MessagePlugin.warning(t('settings.skills.installPartial', { failed }))
    } else {
      MessagePlugin.success(t('settings.skills.installAccepted'))
    }
    rememberSession(installSessionIds, targets)
    await loadCatalog()
    prunePicks(installTargetIds, installPickRows.value)
    // 安装已接受：直接打开该技能在目标沙箱的「管理」抽屉，让安装过程（进度/
    // 阶段/转录）立即可见，而不是只弹一个 toast 后让用户去找那个忙碌 chip。
    const refreshed = catalog.value.find((row) => row.id === item.id)
    const targetSet = new Set(targets)
    const started = (refreshed?.installations || []).find(
      (inst) => targetSet.has(inst.sandbox_config_id) && isInstallBusy(inst),
    )
    if (started) openManage(item, started)
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('settings.sandbox.skillUploadFailed'))
  } finally {
    installing.value = false
  }
}

async function removeCatalog(item: SkillCatalogItem) {
  if (!canDelete(item)) {
    MessagePlugin.warning(t('settings.skills.deleteCatalogBlocked'))
    return
  }
  deletingId.value = item.id
  try {
    await deleteSkillCatalog(item.id)
    MessagePlugin.success(t('settings.skills.deleteSuccess'))
    await loadCatalog()
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('common.deleteFailed'))
  } finally {
    deletingId.value = ''
  }
}

function catalogBusy(): boolean {
  return catalog.value.some((item) =>
    liveInstalls(item).some((inst) => inst.status === 'installing' || inst.status === 'removing'),
  )
}

function stopPoll() {
  if (pollTimer != null) {
    window.clearInterval(pollTimer)
    pollTimer = null
  }
}

function ensurePoll() {
  if (!catalogBusy()) {
    stopPoll()
    return
  }
  if (pollTimer != null) return
  pollTimer = window.setInterval(() => {
    void loadCatalog(true)
  }, 2500)
}

async function loadCatalog(silent = false) {
  try {
    // 000108：管理页 include_hidden，隐藏的技能也列出以便改回可见
    const res = await listSkillCatalog(true)
    catalog.value = res?.data || []
  } catch (e: any) {
    if (!silent) MessagePlugin.error(e?.message || t('settings.skills.loadFailed'))
  } finally {
    ensurePoll()
  }
}

async function load() {
  loading.value = true
  try {
    const [configRes] = await Promise.all([listSandboxConfigs(), loadCatalog()])
    records.value = configRes?.data || []
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('settings.skills.loadFailed'))
  } finally {
    loading.value = false
  }
}

watch(showManage, (open) => {
  if (!open) {
    void loadCatalog()
    manageSkillId.value = ''
    manageRecord.value = null
  } else {
    openPanelId.value = ''
  }
})

watch(showInstall, (open) => {
  if (open) {
    openPanelId.value = ''
    return
  }
  installSessionIds.value = []
})

const busyPickTargets = computed(() => {
  const targets: { configId: string; skillId: string }[] = []
  if (!showInstall.value) return targets
  for (const row of installPickRows.value) {
    if (!row.busy || !row.install?.skill_id) continue
    targets.push({ configId: row.cfg.id, skillId: row.install.skill_id })
  }
  return targets
})

watch(busyPickTargets, (targets) => {
  syncInstallProgress(targets)
}, { immediate: true })

onMounted(load)
onUnmounted(() => {
  stopPoll()
  stopInstallProgress()
})
</script>

<style lang="less" scoped>
.skill-settings {
  width: 100%;
}

.section-header {
  margin-bottom: 28px;

  &__title-row {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;
  }

  h2 {
    font-size: 20px;
    font-weight: 600;
    color: var(--td-text-color-primary);
    margin: 0;
  }

  &__help {
    color: var(--td-text-color-placeholder);
    font-size: 16px;
    cursor: help;
    transition: color 0.15s ease;

    &:hover {
      color: var(--td-text-color-secondary);
    }
  }

  .section-description {
    font-size: 14px;
    color: var(--td-text-color-secondary);
    margin: 0;
    line-height: 1.6;
  }
}

:global(.skill-settings__help-tooltip .t-popup__content) {
  max-width: 340px;
  line-height: 1.55;
}

:global(.skill-install-panel-overlay) {
  z-index: 3050 !important;
}

:global(.skill-category-editor-overlay) {
  z-index: 3050 !important;
}

// 分类编辑弹层：输入 + 保存一行（复用安装面板的 overlay 样式骨架）
.skill-category-editor {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 260px;
  max-width: calc(100vw - 32px);
  padding: 8px;
}

:global(.skill-install-panel-overlay .t-popup__content) {
  padding: 0 !important;
  border-radius: 6px !important;
  border: 1px solid var(--td-component-stroke) !important;
  background: var(--td-bg-color-container) !important;
  box-shadow: var(--td-shadow-2, 0 3px 14px 2px rgba(0, 0, 0, 0.05)) !important;
}

.loading-container {
  padding: 40px 0;
  text-align: center;
}

.empty-state {
  padding: 80px 0;
  text-align: center;

  :deep(.t-empty__description) {
    font-size: 14px;
    color: var(--td-text-color-placeholder);
    margin-bottom: 16px;
  }
}

.empty-hint {
  margin: 0 0 16px;
  font-size: 13px;
  color: var(--td-text-color-placeholder);
}

.empty-actions {
  display: flex;
  justify-content: center;
  gap: 8px;
}

.skill-list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 10px;
  align-items: stretch;
}

.skill-card {
  position: relative;
  display: flex;
  flex-direction: column;
  padding: 0;
  overflow: hidden;
  border: 1px solid var(--td-component-stroke);
  border-radius: 10px;
  background: var(--td-bg-color-container);
  transition: border-color 0.18s ease, box-shadow 0.18s ease;
  min-width: 0;
  height: 100%;

  &--focused {
    border-color: var(--td-brand-color);
    box-shadow: 0 0 0 2px var(--td-brand-color-focus, rgba(0, 168, 112, 0.18));
  }

  &--installed .skill-card__badge {
    background: color-mix(in srgb, var(--td-brand-color) 12%, transparent);
    color: var(--td-brand-color);
  }
}

.skill-card__main {
  display: flex;
  align-items: stretch;
  gap: 10px;
  padding: 10px 12px;
  min-width: 0;
  flex: 1;
}

.skill-card__badge {
  flex-shrink: 0;
  align-self: flex-start;
  width: 28px;
  height: 28px;
  border-radius: 7px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--td-bg-color-secondarycontainer);
  color: var(--td-text-color-secondary);
}

.skill-card__body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.skill-card__header {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  min-width: 0;
}

.skill-card__heading {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}

// 版本 / 分类 / 溯源 pill 排在英文名之下的独立一行，可换行不挤压标题
.skill-card__tags {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.skill-card__title {
  flex: 1 1 auto;
  min-width: 0;
  margin: 0;
  font-size: 14px;
  font-weight: 600;
  line-height: 1.35;
  color: var(--td-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.skill-card__actions {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 2px;
}

.skill-card__icon-btn {
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  margin: 0;
  padding: 0;
  border: 0;
  border-radius: 6px;
  background: none;
  color: var(--td-text-color-secondary);
  cursor: pointer;

  :deep(svg) {
    display: block;
    overflow: visible;
  }

  &:hover:not(:disabled) {
    color: var(--td-text-color-primary);
    background: var(--td-bg-color-container-hover);
  }

  &--danger:hover:not(:disabled) {
    color: var(--td-error-color);
    background: color-mix(in srgb, var(--td-error-color) 8%, transparent);
  }

  &:disabled {
    cursor: not-allowed;
    opacity: 0.4;
  }
}

.skill-card__type {
  flex-shrink: 0;
  font-size: 12px;
  font-weight: 500;
  line-height: 1.35;
  color: var(--td-text-color-placeholder);
}

// 000108：本空间已隐藏的标识（管理页 include_hidden 列出的隐藏技能）
.skill-card__hidden {
  flex-shrink: 0;
  padding: 0 6px;
  border: 1px solid var(--td-warning-color-5, #E37318);
  border-radius: 3px;
  font-size: 11px;
  line-height: 18px;
  color: var(--td-warning-color-7, #B85C00);
  background: color-mix(in srgb, var(--td-warning-color) 10%, transparent);
  white-space: nowrap;
}

.skill-card__visible-switch {
  flex-shrink: 0;
  margin-right: 2px;
}

// 溯源 pill：平台技能库分配物化的目录行（自建行不渲染）
.skill-card__from-library {
  flex-shrink: 0;
  padding: 0 6px;
  border: 1px solid var(--td-brand-color-5, var(--td-brand-color));
  border-radius: 3px;
  font-size: 11px;
  line-height: 18px;
  color: var(--td-brand-color);
  background: var(--td-brand-color-light, transparent);
  white-space: nowrap;
}

// 分类 pill：中性配色与溯源 pill 区分（分类可改，溯源是事实）
.skill-card__category {
  flex-shrink: 0;
  max-width: 120px;
  padding: 0 6px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 3px;
  font-size: 11px;
  line-height: 18px;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-secondarycontainer);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.skill-card__desc {
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  margin: 0;
  overflow: hidden;
  font-size: 12px;
  line-height: 1.45;
  color: var(--td-text-color-secondary);
  word-break: break-word;
}

.skill-card__installs {
  display: flex;
  align-items: center;
  min-width: 0;
  margin-top: auto;
}

.skill-card__installs-label {
  font-size: 12px;
  line-height: 20px;
  color: var(--td-text-color-placeholder);
}

.skill-card__chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
  max-width: 100%;
  margin: 0;
  padding: 3px 8px 3px 10px;
  border: 0;
  border-radius: 8px;
  background: var(--td-bg-color-secondarycontainer);
  color: var(--td-text-color-secondary);
  cursor: pointer;
  font: inherit;
  font-size: 12px;
  line-height: 20px;
  text-align: left;

  &:hover:not(:disabled) {
    color: var(--td-text-color-primary);
    background: var(--td-bg-color-container-hover);
  }

  &:disabled {
    cursor: default;
    opacity: 0.6;
  }

  &--idle {
    color: var(--td-brand-color);
    background: color-mix(in srgb, var(--td-brand-color) 10%, transparent);

    &:hover:not(:disabled) {
      color: var(--td-brand-color);
      background: color-mix(in srgb, var(--td-brand-color) 16%, transparent);
    }

    .skill-card__chip-go {
      color: var(--td-brand-color);
    }
  }

  &.skill-card__entry--off {
    color: var(--td-text-color-placeholder);
  }

  &--installed .skill-card__entry-status {
    color: var(--td-success-color, var(--td-brand-color));
  }

  &.skill-card__entry--stale {
    background: color-mix(in srgb, var(--td-warning-color) 10%, transparent);

    .skill-card__entry-status {
      color: var(--td-warning-color);
    }
  }

  &.skill-card__entry--failed {
    background: color-mix(in srgb, var(--td-error-color) 10%, transparent);

    .skill-card__entry-status {
      color: var(--td-error-color);
    }
  }

  &.skill-card__entry--busy .skill-card__entry-dot {
    background: var(--td-warning-color);
  }
}

.skill-card__chip-text {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.skill-card__chip-go,
.skill-card__entry-status {
  flex-shrink: 0;
}

.skill-card__entry--ready .skill-card__entry-status {
  color: var(--td-success-color, var(--td-brand-color));
}

.skill-card__chip-go {
  color: var(--td-text-color-placeholder);
}

.skill-card__chip:hover:not(:disabled) .skill-card__chip-go {
  color: currentColor;
}

.skill-card__entry-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
  animation: skill-chip-dot 1.2s ease-in-out infinite;
}

.skill-install-panel {
  display: flex;
  flex-direction: column;
  width: 240px;
  max-width: calc(100vw - 32px);
  max-height: min(360px, 70vh);
  overflow-y: auto;
  padding: 4px 0;
}

.skill-install-panel__group {
  margin: 0;
  padding: 6px 12px 4px;
  font-size: 12px;
  line-height: 20px;
  color: var(--td-text-color-placeholder);
}

.skill-install-panel__item {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  height: 32px;
  margin: 0;
  padding: 0 12px;
  border: 0;
  border-radius: 0;
  background: none;
  color: var(--td-text-color-primary);
  cursor: pointer;
  font: inherit;
  font-size: 13px;
  line-height: 22px;
  text-align: left;

  &:hover:not(:disabled) {
    background: var(--td-bg-color-container-hover);
  }

  &:disabled {
    cursor: default;
    opacity: 0.5;
  }

  &.skill-card__entry--stale .skill-card__entry-status {
    color: var(--td-warning-color);
  }

  &.skill-card__entry--failed .skill-card__entry-status {
    color: var(--td-error-color);
  }

  &.skill-card__entry--busy .skill-card__entry-dot {
    background: var(--td-warning-color);
  }
}

.skill-install-panel__item--available {
  color: var(--td-text-color-secondary);
}

.skill-install-panel__name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.skill-install-panel__add {
  flex-shrink: 0;
  color: var(--td-text-color-placeholder);
}

.skill-install-panel__item--available:hover .skill-install-panel__add {
  color: var(--td-brand-color);
}

.skill-install-panel__split {
  height: 1px;
  margin: 4px 0;
  background: var(--td-component-stroke);
}

@keyframes skill-chip-dot {

  0%,
  100% {
    opacity: 1;
  }

  50% {
    opacity: 0.35;
  }
}

.installer-model-hint {
  margin: 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--td-text-color-secondary);
}

.sandbox-pick-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
  min-width: 0;
  max-width: 100%;
  box-sizing: border-box;

  :deep(.t-checkbox) {
    width: 100%;
    max-width: 100%;
    margin: 0;
    align-items: center;
    padding: 10px 12px;
    border: 1px solid var(--td-component-stroke);
    border-radius: 10px;
    background: var(--td-bg-color-container);
    cursor: pointer;
    box-sizing: border-box;

    &:hover:not(.t-is-disabled) {
      border-color: color-mix(in srgb, var(--td-brand-color) 40%, transparent);
      background: color-mix(in srgb, var(--td-brand-color) 4%, transparent);
    }

    &.t-is-checked {
      border-color: color-mix(in srgb, var(--td-brand-color) 40%, transparent);
      background: color-mix(in srgb, var(--td-brand-color) 5%, transparent);
    }
  }

  :deep(.t-checkbox__label) {
    width: 100%;
    min-width: 0;
    margin: 0;
    padding-left: 8px;
    white-space: normal;
  }

  :deep(.t-checkbox__former),
  :deep(.t-checkbox__input) {
    flex-shrink: 0;
    margin-top: 0;
    align-self: center;
  }
}

.sandbox-pick-row {
  min-width: 0;
  max-width: 100%;
}

.sandbox-pick--status {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  max-width: 100%;
  min-width: 0;
  box-sizing: border-box;
  padding: 10px 12px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 10px;
  background: var(--td-bg-color-container);

  .sandbox-pick__main {
    flex: 1;
    min-width: 0;
  }

  .sandbox-pick__progress,
  :deep(.t-button) {
    flex-shrink: 0;
  }

  .sandbox-pick-row.is-busy & {
    border-color: color-mix(in srgb, var(--td-warning-color) 35%, transparent);
  }
}

.sandbox-pick__main {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.sandbox-pick__text {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
}

.sandbox-pick__name {
  font-size: 13px;
  font-weight: 500;
  color: var(--td-text-color-primary);
  line-height: 1.3;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sandbox-pick__meta {
  font-size: 12px;
  color: var(--td-text-color-secondary);
  line-height: 1.3;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sandbox-pick__progress {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 500;
  line-height: 1;
  color: var(--td-brand-color);

  :deep(.t-progress--circle svg) {
    display: block;
  }
}
</style>
