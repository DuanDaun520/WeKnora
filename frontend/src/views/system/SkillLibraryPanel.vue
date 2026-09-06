<template>
  <div class="panel-root skill-library">
    <!-- 面板头与 SandboxConnectionsPanel 同款：标题 + 描述居左，刷新 / 新建按钮居右 -->
    <div class="panel-header">
      <div>
        <h2>{{ t('skillLibrary.title') }}</h2>
        <p class="panel-header-desc">{{ t('skillLibrary.description') }}</p>
      </div>
      <div class="panel-header-actions">
        <t-button variant="outline" :loading="loading" @click="loadSkills">
          <template #icon><t-icon name="refresh" /></template>
          {{ t('skillLibrary.refresh') }}
        </t-button>
        <t-button variant="outline" @click="categoryManagerVisible = true">
          <template #icon><t-icon name="folder" /></template>
          {{ t('skillLibrary.categoryManager') }}
        </t-button>
        <t-button theme="primary" @click="openCreate">
          <template #icon><t-icon name="add" /></template>
          {{ t('skillLibrary.addSkill') }}
        </t-button>
      </div>
    </div>

    <div class="console-toolbar">
      <t-input
        v-model="searchQuery"
        class="toolbar-search"
        clearable
        :placeholder="t('skillLibrary.searchPlaceholder')"
      >
        <template #prefix-icon><t-icon name="search" /></template>
      </t-input>
    </div>

    <!-- 分类快速过滤（000106）：chips 从当前列表派生，与用户侧 SkillsMcpList
         同款交互 —— 全部分类 + 在用的分类 + （若有）未分类。 -->
    <div v-if="skills.length" class="category-row">
      <button
        type="button"
        class="category-chip"
        :class="{ active: activeCategory === CATEGORY_ALL }"
        @click="activeCategory = CATEGORY_ALL"
      >
        {{ t('skillLibrary.categoryFilterAll') }}
      </button>
      <button
        v-for="cat in skillCategories"
        :key="cat"
        type="button"
        class="category-chip"
        :class="{ active: activeCategory === cat }"
        @click="activeCategory = cat"
      >
        {{ cat === '' ? t('skillLibrary.uncategorized') : cat }}
      </button>
      <span class="count-label">{{ t('skillLibrary.countLabel', { count: filteredSkills.length }) }}</span>
    </div>

    <div v-if="loading" class="loading-container">
      <t-loading :text="t('common.loading')" />
    </div>

    <div v-else-if="filteredSkills.length === 0" class="empty-state">
      <t-empty
        :description="hasActiveFilter
          ? t('skillLibrary.noMatchHint')
          : t('skillLibrary.emptyHint')"
      />
    </div>

    <!-- 与 sandbox-connections 同形的卡片：名称 + 版本 / 描述 / 更新时间 +
         分配 chips（带琥珀点 = 该空间的物化行落后于技能定义，未推送）。 -->
    <div v-else class="skill-grid">
      <div
        v-for="skill in filteredSkills"
        :key="skill.id"
        class="skill-card"
        role="button"
        tabindex="0"
        @click="onCardClick($event, skill)"
        @keydown.enter="onCardClick($event, skill)"
      >
        <div class="skill-card__body">
          <div class="skill-card__header">
            <h3 class="skill-card__title" :title="cardTitle(skill)">{{ cardTitle(skill) }}</h3>
            <span v-if="skill.version" class="skill-card__version" :title="skill.version">
              {{ skill.version }}
            </span>
            <!-- 分类 pill：与空间技能目录卡同款（点击卡片进编辑抽屉可改） -->
            <span
              v-if="skill.category"
              class="skill-card__category"
              :title="skill.category"
            >
              {{ skill.category }}
            </span>
            <div class="skill-card__actions" @click.stop>
              <t-tooltip :content="t('skillLibrary.browseFiles')">
                <t-button
                  variant="text"
                  shape="square"
                  size="small"
                  class="skill-card__icon-btn"
                  @click="openFiles(skill)"
                >
                  <t-icon name="folder-open" />
                </t-button>
              </t-tooltip>
              <t-dropdown
                :options="cardOptions(skill)"
                placement="bottom-right"
                attach="body"
                trigger="click"
                @click="(data: any) => handleMenuAction({ value: data.value }, skill)"
              >
                <t-button variant="text" shape="square" size="small" class="skill-card__more">
                  <t-icon name="ellipsis" />
                </t-button>
              </t-dropdown>
            </div>
          </div>
          <div v-if="cardTitle(skill) !== skill.name" class="skill-card__subname" :title="skill.name">
            {{ skill.name }}
          </div>
          <!-- 描述展示：中文描述(000106)优先，空则回落 SKILL.md description；卡片只
               显示前 40 字，完整文本在 title tooltip 里。 -->
          <div v-if="cardDescExcerpt(skill)" class="skill-card__desc" :title="cardDescFull(skill)">
            {{ cardDescExcerpt(skill) }}
          </div>
          <div v-if="skill.updated_at || skill.author" class="skill-card__time">
            <template v-if="skill.updated_at">
              {{ t('skillLibrary.updatedAt') }}：{{ formatTime(skill.updated_at) }}
            </template>
            <template v-if="skill.updated_at && skill.author"> · </template>
            <span v-if="skill.author" class="skill-card__author" :title="skill.author">
              {{ skill.author }}
            </span>
          </div>
          <div class="skill-card__assignments">
            <template v-if="skill.assignments?.length">
              <t-tooltip
                v-for="assignment in skill.assignments"
                :key="assignment.tenant_id"
                :content="assignment.drift
                  ? t('skillLibrary.assignmentDriftHint')
                  : t('skillLibrary.assignmentInSync')"
              >
                <t-tag
                  size="small"
                  variant="outline"
                  :class="{ 'skill-card__tenant-chip--drift': assignment.drift }"
                >
                  <span v-if="assignment.drift" class="skill-card__drift-dot" />
                  {{ assignment.tenant_name }}
                </t-tag>
              </t-tooltip>
            </template>
            <span v-else class="skill-card__assignments-empty">
              {{ t('skillLibrary.unassigned') }}
            </span>
          </div>
        </div>
      </div>
    </div>

    <!-- 注册 / 编辑抽屉（编辑态含空间分配区） -->
    <PlatformSkillDrawer
      v-model:visible="drawerVisible"
      :skill="editing"
      @saved="handleSaved"
    />

    <!-- 平台 zip 文件浏览器 -->
    <SkillFilesDrawer
      v-model:visible="filesVisible"
      platform
      :catalog-id="filesSkillId"
      :skill-name="filesSkillName"
    />

    <!-- 分类管理：派生目录 + 批量重命名 / 删除 -->
    <SkillCategoryManagerDialog
      v-model:visible="categoryManagerVisible"
      @changed="loadSkills"
    />

    <!-- 推送结果：逐空间状态，被阻止的行带原因。底部「关闭」是 confirm 按钮，
         须显式把它关掉（tdesign 的 confirm 不自动 emit update:visible）。 -->
    <t-dialog
      v-model:visible="pushResultVisible"
      :header="t('skillLibrary.pushResultTitle')"
      :confirm-btn="{ content: t('common.close'), theme: 'default' }"
      :cancel-btn="null"
      width="560px"
      attach="body"
      @confirm="pushResultVisible = false"
    >
      <div class="push-summary">
        <t-tag theme="success" variant="light">
          {{ t('skillLibrary.pushStatus.updated') }}：{{ pushResultSummary.updated }}
        </t-tag>
        <t-tag v-if="pushResultSummary.healed" theme="success" variant="light">
          {{ t('skillLibrary.pushStatus.healed') }}：{{ pushResultSummary.healed }}
        </t-tag>
        <t-tag v-if="pushResultSummary.blocked" theme="warning" variant="light">
          {{ t('skillLibrary.pushBlockedCount') }}：{{ pushResultSummary.blocked }}
        </t-tag>
      </div>
      <!-- 000106 引导：推送只同步定义，到用户环境可选用还需空间侧安装到沙箱。 -->
      <p v-if="pushResultSummary.updated + pushResultSummary.healed > 0" class="push-note">
        {{ t('skillLibrary.pushNoteInstall') }}
      </p>
      <div class="push-rows">
        <div v-for="row in pushResultRows" :key="row.tenant_id" class="push-row">
          <span class="push-row__name" :title="row.tenant_name || `#${row.tenant_id}`">
            {{ row.tenant_name || `#${row.tenant_id}` }}
          </span>
          <t-tag :theme="pushStatusTheme(row.status)" variant="light" size="small">
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
import PlatformSkillDrawer from '@/components/PlatformSkillDrawer.vue'
import SkillFilesDrawer from '@/components/SkillFilesDrawer.vue'
import SkillCategoryManagerDialog from '@/views/system/SkillCategoryManagerDialog.vue'
import { useConfirmDelete } from '@/components/settings/useConfirmDelete'
import {
  deletePlatformSkill,
  listPlatformSkills,
  pushPlatformSkill,
  type PlatformSkill,
  type SkillPushOutcome,
} from '@/api/skill-library'

const { t } = useI18n()
const confirmDelete = useConfirmDelete()

const skills = ref<PlatformSkill[]>([])
const loading = ref(false)

// ---- 工具栏筛选：关键词搜索 + 分类快速过滤（000106）----
const searchQuery = ref('')
const CATEGORY_ALL = '__all__'
const activeCategory = ref<string>(CATEGORY_ALL)

const hasActiveFilter = computed(
  () => !!searchQuery.value.trim() || activeCategory.value !== CATEGORY_ALL,
)

// 分类 chips 从当前列表派生（与用户侧 SkillsMcpList 同款）：只出现有技能在用
// 的分类，'' 代表未分类。登记表里 0 用量的分类不在这里刷存在感。
const skillCategories = computed(() => {
  const names = new Set<string>()
  let hasUncategorized = false
  for (const skill of skills.value) {
    const cat = skill.category || ''
    if (cat === '') hasUncategorized = true
    else names.add(cat)
  }
  const sorted = [...names].sort((a, b) => a.localeCompare(b, 'zh-Hans-CN'))
  if (hasUncategorized) sorted.push('')
  return sorted
})

const filteredSkills = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  return skills.value.filter((skill) => {
    if (
      activeCategory.value !== CATEGORY_ALL &&
      (skill.category || '') !== activeCategory.value
    ) {
      return false
    }
    if (!q) return true
    return [
      skill.name,
      skill.zh_name,
      skill.description,
      skill.zh_description,
      skill.version,
      skill.category,
      skill.author,
    ].some((field) => (field || '').toLowerCase().includes(q))
  })
})

// 卡片标题/描述：中文展示元数据（000106）为空时回落 SKILL.md 的 name/
// description；描述在卡片只显示前 40 字，完整文本留在 title tooltip。
const cardTitle = (skill: PlatformSkill) => skill.zh_name || skill.name
const cardDescFull = (skill: PlatformSkill) => skill.zh_description || skill.description || ''
const cardDescExcerpt = (skill: PlatformSkill) => {
  const full = cardDescFull(skill).trim()
  return full.length > 40 ? `${full.slice(0, 40)}…` : full
}

// ---- 抽屉 ----
const drawerVisible = ref(false)
const editing = ref<PlatformSkill | null>(null)

// ---- 文件浏览器 ----
const filesVisible = ref(false)
const filesSkillId = ref('')
const filesSkillName = ref('')

// ---- 分类管理弹窗：重命名/删除批量改技能行并点亮 drift，changed 后重拉 ----
const categoryManagerVisible = ref(false)

// ---- 推送结果对话框 ----
const pushResultVisible = ref(false)
const pushResultRows = ref<SkillPushOutcome[]>([])
const pushResultSummary = computed(() => ({
  updated: pushResultRows.value.filter((row) => row.status === 'updated').length,
  healed: pushResultRows.value.filter((row) => row.status === 'healed').length,
  blocked: pushResultRows.value.filter((row) => row.status === 'blocked').length,
}))

const loadSkills = async () => {
  loading.value = true
  try {
    skills.value = await listPlatformSkills()
  } catch (error) {
    MessagePlugin.error(t('skillLibrary.toasts.loadFailed'))
    console.error('Failed to load platform skills:', error)
  } finally {
    loading.value = false
  }
}

const openCreate = () => {
  editing.value = null
  drawerVisible.value = true
}

const openEdit = (skill: PlatformSkill) => {
  editing.value = { ...skill }
  drawerVisible.value = true
}

// The drawer emits saved after the skill (and, in edit mode, the assignment
// replace) landed — partially or fully. Reload in both cases so drift markers
// and assignment chips reflect reality.
const handleSaved = () => {
  void loadSkills()
}

const openFiles = (skill: PlatformSkill) => {
  filesSkillId.value = skill.id
  filesSkillName.value = skill.name
  filesVisible.value = true
}

const onCardClick = (event: Event, skill: PlatformSkill) => {
  if (event.type === 'keydown') {
    const ke = event as KeyboardEvent
    if (ke.key !== 'Enter' && ke.key !== ' ') return
    ke.preventDefault()
  }
  const target = event.target as HTMLElement | null
  if (target?.closest('.skill-card__actions')) return
  openEdit(skill)
}

// ---- 推送：编辑不自动传播，唯一的扩散入口在这里 ----
const runPush = async (skill: PlatformSkill) => {
  try {
    const result = await pushPlatformSkill(skill.id)
    pushResultRows.value = result.results || []
    pushResultVisible.value = true
    // Blocked outcomes leave drift markers on; refresh so the chips match.
    await loadSkills()
  } catch (error: any) {
    MessagePlugin.error(error?.message || t('skillLibrary.toasts.pushFailed'))
  }
}

const pushStatusLabel = (row: SkillPushOutcome): string => {
  switch (row.status) {
    case 'updated':
      return t('skillLibrary.pushStatus.updated')
    case 'healed':
      return t('skillLibrary.pushStatus.healed')
    case 'blocked':
      return row.code === 'name_conflict'
        ? t('skillLibrary.pushStatus.blockedNameConflict')
        : t('skillLibrary.pushStatus.error')
    default:
      return row.message || t('skillLibrary.pushStatus.error')
  }
}

const pushStatusTheme = (status: string): 'success' | 'warning' | 'danger' => {
  switch (status) {
    case 'updated':
    case 'healed':
      return 'success'
    case 'blocked':
      return 'warning'
    default:
      return 'danger'
  }
}

// ---- 删除：有活分配时后端 409 assignments_exist（物化行是真实的空间目录
//      行，可拥有安装，不做平台级级联清除）----
const runDelete = (skill: PlatformSkill) => {
  confirmDelete({
    body: t('skillLibrary.deleteConfirmBody', { name: skill.name }),
    onConfirm: async () => {
      try {
        await deletePlatformSkill(skill.id)
        MessagePlugin.success(t('skillLibrary.toasts.deleted'))
        await loadSkills()
      } catch (error: any) {
        if (error?.error?.code === 'assignments_exist') {
          MessagePlugin.warning(t('skillLibrary.blockedAssignmentsExist'))
        } else {
          MessagePlugin.error(error?.message || t('skillLibrary.toasts.deleteFailed'))
        }
      }
    },
  })
}

const cardOptions = (skill: PlatformSkill) => {
  const options: Array<{ content: string; value: string; theme?: 'error' }> = [
    { content: t('common.edit'), value: 'edit' },
  ]
  // Push only means something while workspaces hold materialized rows.
  if (skill.assignments?.length) {
    options.push({ content: t('skillLibrary.pushAction'), value: 'push' })
  }
  options.push({ content: t('common.delete'), value: 'delete', theme: 'error' })
  return options
}

const handleMenuAction = (data: { value: string }, skill: PlatformSkill) => {
  switch (data.value) {
    case 'edit':
      openEdit(skill)
      break
    case 'push':
      runPush(skill)
      break
    case 'delete':
      runDelete(skill)
      break
  }
}

const formatTime = (value: string): string => {
  const ms = Date.parse(value)
  return Number.isNaN(ms) ? value : new Date(ms).toLocaleString()
}

onMounted(() => {
  loadSkills()
})
</script>

<style scoped lang="less">
@import './consolePanel.less';

.skill-library {
  width: 100%;
}

.loading-container {
  padding: 40px 0;
  text-align: center;
}

// 空态：无技能或筛选无结果时占位（新建入口统一在 panel-header 右上角）
.empty-state {
  padding: 48px 0;
}

.skill-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 12px;
}

// 与 sandbox-connections 的 connection-card 同形的卡片
.skill-card {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 14px;
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

.skill-card__body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.skill-card__header {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.skill-card__title {
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

.skill-card__version {
  flex-shrink: 0;
  max-width: 120px;
  font-size: 11px;
  line-height: 1.4;
  color: var(--td-text-color-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

// 分类 pill：与空间技能目录卡（SkillCatalogSettings）同款
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

.skill-card__actions {
  flex-shrink: 0;
  display: flex;
  align-items: center;
}

.skill-card__icon-btn,
.skill-card__more {
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

.skill-card:hover .skill-card__icon-btn,
.skill-card:hover .skill-card__more,
.skill-card:focus-within .skill-card__icon-btn,
.skill-card:focus-within .skill-card__more,
.skill-card__actions:focus-within .skill-card__icon-btn,
.skill-card__actions:focus-within .skill-card__more {
  opacity: 1;
}

// 中文名存在时，英文 SKILL.md 名作为副信息小字（000106）
.skill-card__subname {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}

// 描述摘要：最多两行（前 40 字），超出折叠；完整文本在 title tooltip
.skill-card__desc {
  font-size: 12px;
  line-height: 1.4;
  color: var(--td-text-color-secondary);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  min-width: 0;
  word-break: break-all;
}

.skill-card__time {
  font-size: 11px;
  line-height: 1.4;
  color: var(--td-text-color-placeholder);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.skill-card__author {
  color: var(--td-text-color-secondary);
}

// 已分配空间 chips：drift 的行带琥珀点（技能编辑过、还没推送）
.skill-card__assignments {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 2px;

  :deep(.t-tag) {
    max-width: 100%;
  }
}

.skill-card__assignments-empty {
  font-size: 11px;
  line-height: 1.4;
  color: var(--td-text-color-placeholder);
}

.skill-card__tenant-chip--drift {
  color: var(--td-warning-color-7, #B85C00);
  border-color: var(--td-warning-color-5, #E37318);
}

.skill-card__drift-dot {
  display: inline-block;
  width: 6px;
  height: 6px;
  margin-right: 4px;
  border-radius: 50%;
  background: var(--td-warning-color, #ED7B2F);
  vertical-align: middle;
}

// ---- 分类快速过滤 chips（000106，同用户侧 SkillsMcpList 交互）----
.category-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  padding: 2px 0 10px;
}

.category-chip {
  padding: 2px 10px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 12px;
  background: var(--td-bg-color-container);
  font-size: 12px;
  line-height: 20px;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  transition: border-color 0.15s ease, background 0.15s ease, color 0.15s ease;

  &:hover {
    border-color: var(--td-brand-color-5, var(--td-brand-color));
  }

  &.active {
    background: var(--td-brand-color);
    border-color: var(--td-brand-color);
    color: #fff;
  }
}

.count-label {
  margin-left: 4px;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

// ---- 推送结果对话框 ----
.push-summary {
  display: flex;
  gap: 8px;
  margin-bottom: 8px;
}

.push-note {
  margin: 0 0 10px;
  padding: 8px 10px;
  border-radius: 6px;
  background: var(--td-brand-color-light, rgba(0, 95, 255, 0.08));
  font-size: 12px;
  line-height: 1.5;
  color: var(--td-text-color-secondary);
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
