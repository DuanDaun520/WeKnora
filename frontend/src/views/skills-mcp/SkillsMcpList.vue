<template>
  <div class="skills-mcp-container">
    <!-- 左侧栏：全部 / 收藏 / 最近。技能与 MCP 都是空间级资源（无"本空间/
         协作空间"维度），所以 hide-workspace 收掉后两组入口。计数取两个
         Tab 的并集。 -->
    <ListSpaceSidebar v-if="!authStore.isLiteMode" v-model="scope" hide-workspace :count-all="totalCount"
      :count-favorites="favoritesCount" :count-recents="recentsCount" collapsed-key="sidebar-collapsed-skills-mcp" />
    <div class="skills-mcp-content">
      <div class="header" style="--wails-draggable: drag">
        <div class="header-title" style="--wails-draggable: drag">
          <div class="title-row" style="--wails-draggable: drag">
            <h2 style="--wails-draggable: drag">{{ $t('skillsMcp.title') }}</h2>
          </div>
          <p class="header-subtitle" style="--wails-draggable: drag">{{ $t('skillsMcp.subtitle') }}</p>
        </div>
      </div>

      <div class="skills-mcp-main">
        <t-tabs v-model="activeTab" class="skills-mcp-tabs">
          <t-tab-panel value="skills" :label="$t('skillsMcp.tabs.skills')" />
          <t-tab-panel value="mcp" :label="$t('skillsMcp.tabs.mcp')" />
        </t-tabs>

        <!-- 搜索 + 分类目录 + 计数（两个 Tab 各自一套过滤状态） -->
        <div class="toolbar">
          <t-input v-model="activeQuery" class="search-input" clearable
            :placeholder="$t('skillsMcp.searchPlaceholder')">
            <template #prefix-icon>
              <t-icon name="search" size="16px" />
            </template>
          </t-input>
          <div class="category-row">
            <button type="button" class="category-chip" :class="{ active: activeCategory === CATEGORY_ALL }"
              @click="setCategory(CATEGORY_ALL)">
              {{ $t('skillsMcp.categoryAll') }}
            </button>
            <button v-for="cat in activeCategories" :key="cat" type="button" class="category-chip"
              :class="{ active: activeCategory === cat }" @click="setCategory(cat)">
              {{ cat === '' ? $t('skillsMcp.uncategorized') : cat }}
            </button>
          </div>
          <span class="count-label">{{ $t('skillsMcp.countLabel', { count: activeFiltered.length }) }}</span>
        </div>

        <!-- 骨架屏 -->
        <div v-if="loading && totalCount === 0" class="card-wrap">
          <div v-for="n in 6" :key="'skel-' + n" class="item-card skeleton-card">
            <t-skeleton animation="gradient"
              :row-col="[[{ width: '40%', height: '18px' }], [{ width: '100%', height: '14px' }, { width: '70%', height: '14px' }]]" />
          </div>
        </div>

        <!-- 卡片网格 -->
        <div v-else-if="activeFiltered.length > 0" class="card-wrap">
          <template v-if="activeTab === 'skills'">
            <div v-for="item in skillFiltered" :key="item.id" class="item-card"
              @click="openDetail('skill', item.id)">
              <button v-if="canPinSkill(item)" type="button" class="favorite-star"
                :class="{ 'is-favorited': pins.isFavorite('skill', item.id) }"
                @click.stop="toggleFavorite('skill', item.id)">
                <t-icon :name="pins.isFavorite('skill', item.id) ? 'star-filled' : 'star'" size="14px" />
              </button>
              <div class="card-title-row">
                <span class="card-title" :title="item.name">{{ item.name }}</span>
                <span v-if="item.version" class="version-badge">v{{ item.version }}</span>
                <span v-if="isSkillInstalled(item)" class="status-badge ok">{{ $t('skillsMcp.installed') }}</span>
              </div>
              <p class="card-desc" :title="item.description">{{ item.description || '—' }}</p>
              <div class="card-footer">
                <span v-if="item.category" class="category-tag">{{ item.category }}</span>
                <!-- 000109：技能卡片显示作者（SKILL.md/平台元数据），不再显示
                     创建者（推送场景下创建者恒为"系统"，没有信息量）。 -->
                <span v-if="item.author" class="creator">{{ item.author }}</span>
              </div>
            </div>
          </template>
          <template v-else>
            <div v-for="item in mcpFiltered" :key="item.id" class="item-card" @click="openDetail('mcp', item.id)">
              <button type="button" class="favorite-star"
                :class="{ 'is-favorited': pins.isFavorite('mcp', item.id) }"
                @click.stop="toggleFavorite('mcp', item.id)">
                <t-icon :name="pins.isFavorite('mcp', item.id) ? 'star-filled' : 'star'" size="14px" />
              </button>
              <div class="card-title-row">
                <span class="card-title" :title="item.name">{{ item.name }}</span>
                <span v-if="item.is_builtin" class="version-badge builtin">{{ $t('skillsMcp.builtin') }}</span>
                <span class="status-badge" :class="item.enabled ? 'ok' : 'off'">
                  {{ item.enabled ? $t('skillsMcp.enabled') : $t('skillsMcp.disabled') }}
                </span>
              </div>
              <p class="card-desc" :title="item.description">{{ item.description || '—' }}</p>
              <div class="card-footer">
                <span v-if="item.category" class="category-tag">{{ item.category }}</span>
                <span class="creator">{{ item.creator_name || $t('skillsMcp.systemCreator') }}</span>
              </div>
            </div>
          </template>
        </div>

        <!-- 空态：按 scope / 搜索分别给文案 -->
        <div v-else class="empty-state">
          <img class="empty-img" src="@/assets/img/upload.svg" alt="">
          <span class="empty-txt">{{ emptyTitle }}</span>
          <span class="empty-desc">{{ emptyDesc }}</span>
        </div>
      </div>
    </div>

    <!-- 详情弹窗：基本信息 + 使用说明 + 引用智能体（+ 管理员改分类 + 尾部元信息） -->
    <t-dialog v-model:visible="detailVisible" footer-placement="right" :footer="false"
      width="560px" class="skills-mcp-detail-dialog">
      <!-- 000110：标题行 = 技能英文名 + 安装状态徽标（紧跟名后）+ 复制名称按钮 -->
      <template #header>
        <div class="detail-title-row">
          <span class="detail-title-name" :title="detailName">{{ detailName }}</span>
          <span
            v-if="detailType === 'skill' && detailSkill"
            class="status-badge"
            :class="isSkillInstalled(detailSkill) ? 'ok' : 'off'"
          >
            {{ isSkillInstalled(detailSkill) ? $t('skillsMcp.installed') : $t('skillsMcp.notInstalled') }}
          </span>
          <button
            type="button"
            class="detail-title-copy"
            :title="$t('skillsMcp.copyName')"
            :aria-label="$t('skillsMcp.copyName')"
            @click="copyDetailName"
          >
            <t-icon name="file-copy" size="14px" />
          </button>
        </div>
      </template>
      <div v-if="detailVisible" class="detail-body">
        <!-- 基本信息 -->
        <div class="detail-section">
          <div class="section-title">{{ $t('skillsMcp.basicInfo') }}</div>
          <div class="info-grid">
            <template v-if="detailType === 'skill' && detailSkill">
              <div v-if="detailSkill.version" class="info-item">
                <span class="info-label">{{ $t('skillsMcp.version') }}</span>
                <span class="info-value">v{{ detailSkill.version }}</span>
              </div>
              <!-- 作者：SKILL.md frontmatter 或平台推送带来的元数据（000104）。
                   安装状态已上移到标题行（000110）。 -->
              <div v-if="detailSkill.author" class="info-item">
                <span class="info-label">{{ $t('skillsMcp.author') }}</span>
                <span class="info-value">{{ detailSkill.author }}</span>
              </div>
              <!-- 000109：介绍与帮助网址，新窗口打开；未配置不占行。 -->
              <div v-if="detailSkill.help_url" class="info-item wide">
                <span class="info-label">{{ $t('skillsMcp.helpUrl') }}</span>
                <a
                  class="info-value info-link"
                  :href="detailSkill.help_url"
                  target="_blank"
                  rel="noopener noreferrer"
                >{{ helpUrlHost }}<t-icon name="link" size="12px" /></a>
              </div>
            </template>
            <template v-else-if="detailMcp">
              <div class="info-item">
                <span class="info-label">{{ $t('skillsMcp.transportLabel') }}</span>
                <span class="info-value">{{ transportLabel(detailMcp) }}</span>
              </div>
              <div class="info-item">
                <span class="info-label">{{ $t('skillsMcp.builtin') }}</span>
                <span class="info-value">{{ detailMcp.is_builtin ? $t('common.yes') : $t('common.no') }}</span>
              </div>
              <div class="info-item">
                <span class="info-label">{{ $t('skillsMcp.toolsCount') }}</span>
                <span class="info-value">{{ mcpToolsText }}</span>
              </div>
            </template>
          </div>
        </div>

        <!-- 使用说明 -->
        <div class="detail-section">
          <div class="section-title">{{ $t('skillsMcp.usage') }}</div>
          <p class="usage-text">{{ detailDescription || '—' }}</p>
          <p class="usage-hint">{{ $t('skillsMcp.usageHint') }}</p>
        </div>

        <!-- 引用智能体：前端联表 GET /api/v1/agents 的 config -->
        <div class="detail-section">
          <div class="section-title">
            {{ $t('skillsMcp.referencingAgents') }}
            <span class="section-count">{{ detailReferencingAgents.length }}</span>
          </div>
          <div v-if="detailReferencingAgents.length" class="agent-chips">
            <t-tag v-for="a in detailReferencingAgents" :key="a.id" variant="light" theme="primary"
              class="agent-chip" @click="goAgents">
              {{ a.name }}
            </t-tag>
          </div>
          <p v-else class="usage-hint">{{ $t('skillsMcp.noReferencingAgents') }}</p>
        </div>

        <!-- 管理员：编辑分类（MCP builtin 行后端拒绝更新，隐藏入口） -->
        <div v-if="canEditCategory" class="detail-section">
          <div class="section-title">{{ $t('skillsMcp.editCategory') }}</div>
          <div class="category-edit-row">
            <t-input v-model="editingCategory" class="category-edit-input" maxlength="255"
              :placeholder="$t('skillsMcp.categoryPlaceholder')" />
            <t-button size="small" :loading="categorySaving" @click="saveCategory">{{ $t('common.save') }}</t-button>
          </div>
        </div>

        <!-- 000110：元信息后置——已安装沙箱/分类/创建者/创建时间从基本信息挪到
             弹窗末尾，保持顶部聚焦在"这个技能是什么、怎么用"。 -->
        <div class="detail-section">
          <div class="section-title">{{ $t('skillsMcp.otherInfo') }}</div>
          <div class="info-grid">
            <div v-if="detailType === 'skill' && detailSkill && installedSandboxes(detailSkill).length"
              class="info-item wide">
              <span class="info-label">{{ $t('skillsMcp.installedOn') }}</span>
              <span class="info-value">{{ installedSandboxes(detailSkill).join('、') }}</span>
            </div>
            <div class="info-item">
              <span class="info-label">{{ $t('skillsMcp.category') }}</span>
              <span class="info-value">{{ detailCategory || $t('skillsMcp.uncategorized') }}</span>
            </div>
            <div class="info-item">
              <span class="info-label">{{ $t('skillsMcp.creator') }}</span>
              <span class="info-value">{{ detailCreatorName }}</span>
            </div>
            <div class="info-item wide">
              <span class="info-label">{{ $t('skillsMcp.createdAt') }}</span>
              <span class="info-value">{{ detailCreatedAt }}</span>
            </div>
          </div>
        </div>
      </div>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import ListSpaceSidebar from '@/components/ListSpaceSidebar.vue'
import { useListUrlState } from '@/composables/useListUrlState'
import { useResourcePins } from '@/composables/useResourcePins'
import { useAuthStore } from '@/stores/auth'
import { useChatResourcesStore } from '@/stores/chatResources'
import { formatStringDate } from '@/utils/index'
import {
  listSkillCatalog,
  updateSkillCatalogMeta,
  type SkillCatalogItem,
} from '@/api/skill'
import {
  listMCPServices,
  updateMCPService,
  getMCPServiceTools,
  type MCPService,
} from '@/api/mcp-service'
import type { CustomAgent } from '@/api/agent'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const chatResources = useChatResourcesStore()
const pins = useResourcePins()

// 与 AgentList/KnowledgeBaseList 同款：scope 同步到 ?scope=。技能与 MCP
// 没有 mine/org 维度，侧栏只剩 all/favorites/recents 三个保留值。
const { scope } = useListUrlState({ defaultScope: 'all' })

// ---------- 数据 ----------
const skills = ref<SkillCatalogItem[]>([])
const mcpServices = ref<MCPService[]>([])
const loading = ref(false)

async function loadAll() {
  loading.value = true
  try {
    const [skillRes, mcpRes] = await Promise.all([
      listSkillCatalog() as unknown as Promise<{ data: SkillCatalogItem[] }>,
      listMCPServices(),
    ])
    skills.value = skillRes?.data || []
    mcpServices.value = Array.isArray(mcpRes) ? mcpRes : []
  } catch (err) {
    console.warn('[SkillsMcpList] failed to load catalog/services:', err)
    MessagePlugin.warning(t('skillsMcp.loadFailed'))
  } finally {
    loading.value = false
  }
}

// 租户切换后两份列表都要重拉（服务端按空间可见性过滤）。
watch(() => authStore.currentTenantId, () => { void loadAll() })
onMounted(() => {
  void loadAll()
  void chatResources.ensureAgents()
})

const totalCount = computed(() => skills.value.length + mcpServices.value.length)
const favoritesCount = computed(
  () => pins.favorites.value.filter((e) => e.type === 'skill' || e.type === 'mcp').length
)
const recentsCount = computed(
  () => pins.recents.value.filter((e) => e.type === 'skill' || e.type === 'mcp').length
)

// ---------- Tab 与过滤 ----------
const activeTab = ref<'skills' | 'mcp'>('skills')
const skillQuery = ref('')
const mcpQuery = ref('')
const CATEGORY_ALL = '__all__'
const skillCategory = ref(CATEGORY_ALL)
const mcpCategory = ref(CATEGORY_ALL)

// 打开页面时支持 ?tab=mcp 深链。
if (route.query.tab === 'mcp') activeTab.value = 'mcp'

// v-model 需要成员表达式：两个 Tab 的搜索框共用一个可读写的代理。
const activeQuery = computed({
  get: () => (activeTab.value === 'skills' ? skillQuery.value : mcpQuery.value),
  set: (v: string) => {
    if (activeTab.value === 'skills') skillQuery.value = v
    else mcpQuery.value = v
  },
})
const activeCategory = computed(() => (activeTab.value === 'skills' ? skillCategory.value : mcpCategory.value))
function setCategory(cat: string) {
  if (activeTab.value === 'skills') skillCategory.value = cat
  else mcpCategory.value = cat
}

// 分类目录：从当前 Tab 的全量数据派生（收藏/最近视角下也保留完整目录，
// 否则切 scope 后 chip 会闪没）。空分类统一归入「未分类」chip。
const skillCategories = computed(() => deriveCategories(skills.value.map((s) => s.category || '')))
const mcpCategories = computed(() => deriveCategories(mcpServices.value.map((s) => s.category || '')))
const activeCategories = computed(() => (activeTab.value === 'skills' ? skillCategories.value : mcpCategories.value))

function deriveCategories(list: string[]): string[] {
  const set = new Set(list.filter((c) => c !== ''))
  const out = [...set].sort((a, b) => a.localeCompare(b, 'zh-Hans-CN'))
  if (list.includes('')) out.push('')
  return out
}

// scope → 候选集。favorites/recents 用 pins 的 id 对内存索引水合；mine/org
// 不会出现（侧栏已 hide-workspace），兜底按 all 处理。
const skillIndex = computed(() => new Map(skills.value.map((s) => [s.id, s])))
const mcpIndex = computed(() => new Map(mcpServices.value.map((s) => [s.id, s])))

function scopeCandidates<T extends { id: string }>(type: 'skill' | 'mcp', index: Map<string, T>): T[] {
  if (scope.value === 'favorites') {
    return pins.favorites.value
      .filter((e) => e.type === type)
      .map((e) => index.get(e.id))
      .filter((x): x is T => !!x)
  }
  if (scope.value === 'recents') {
    return pins.recents.value
      .filter((e) => e.type === type)
      .map((e) => index.get(e.id))
      .filter((x): x is T => !!x)
  }
  return [...index.values()]
}

function applyFilters<T extends { id: string; name: string; description?: string; category?: string }>(
  list: T[], query: string, category: string
): T[] {
  const q = query.trim().toLowerCase()
  return list.filter((item) => {
    if (category !== CATEGORY_ALL && (item.category || '') !== category) return false
    if (!q) return true
    return (
      item.name.toLowerCase().includes(q) ||
      (item.description || '').toLowerCase().includes(q)
    )
  })
}

const skillFiltered = computed(() =>
  applyFilters(scopeCandidates('skill', skillIndex.value), skillQuery.value, skillCategory.value)
)
const mcpFiltered = computed(() =>
  applyFilters(scopeCandidates('mcp', mcpIndex.value), mcpQuery.value, mcpCategory.value)
)
const activeFiltered = computed(() =>
  activeTab.value === 'skills' ? skillFiltered.value : mcpFiltered.value
)

const emptyTitle = computed(() => {
  if (activeQuery.value.trim()) return t('skillsMcp.empty.search')
  if (scope.value === 'favorites') return t('skillsMcp.empty.favorites')
  if (scope.value === 'recents') return t('skillsMcp.empty.recents')
  return activeTab.value === 'skills' ? t('skillsMcp.empty.skills') : t('skillsMcp.empty.mcp')
})
const emptyDesc = computed(() => {
  if (activeQuery.value.trim()) return t('skillsMcp.empty.searchDesc')
  if (scope.value === 'favorites') return t('skillsMcp.empty.favoritesDesc')
  if (scope.value === 'recents') return t('skillsMcp.empty.recentsDesc')
  return t('skillsMcp.empty.allDesc')
})

// ---------- 收藏 ----------
function toggleFavorite(type: 'skill' | 'mcp', id: string) {
  void pins.toggleFavorite(type, id)
}

// 合成 catalog 行（源 catalog 已删、由安装行派生，无 bundle）id 不稳定，
// 收藏后可能永远水合不出来——这类行直接不给点星。
function canPinSkill(item: SkillCatalogItem): boolean {
  return !!item.bundle_sha256 || !!item.source_platform_skill_id
}

// ---------- 卡片徽标 ----------
function isSkillInstalled(item: SkillCatalogItem): boolean {
  return (item.installations || []).some((i) => i.status === 'ready' && i.enabled)
}
function installedSandboxes(item: SkillCatalogItem): string[] {
  return (item.installations || [])
    .filter((i) => i.status === 'ready' && i.enabled)
    .map((i) => i.sandbox_config_name || i.sandbox_config_id)
}
function transportLabel(item: MCPService): string {
  return t(`skillsMcp.transport.${item.transport_type}`) || item.transport_type
}

// ---------- 引用智能体（前端联表） ----------
// 仅 smart-reasoning 智能体可挂 MCP/Skills。MCP 按 id 匹配 mcp_services，
// 'all' 视为引用全部；Skill 按【名称】匹配 selected_skills，'all' 同理。
// selection_mode 为 'none' 或 ''（旧数据缺省）都视为未引用。
function referencingAgents(tab: 'skills' | 'mcp', key: string, agents: CustomAgent[]): CustomAgent[] {
  return agents.filter((a) => {
    const cfg = a.config || {}
    if (cfg.agent_mode !== 'smart-reasoning') return false
    if (tab === 'mcp') {
      const mode = cfg.mcp_selection_mode || ''
      if (mode === 'all') return true
      if (mode !== 'selected') return false
      return (cfg.mcp_services || []).includes(key)
    }
    const mode = cfg.skills_selection_mode || ''
    if (mode === 'all') return true
    if (mode !== 'selected') return false
    return (cfg.selected_skills || []).includes(key)
  })
}

// ---------- 详情弹窗 ----------
const detailVisible = ref(false)
const detailType = ref<'skill' | 'mcp'>('skill')
const detailSkill = ref<SkillCatalogItem | null>(null)
const detailMcp = ref<MCPService | null>(null)
const mcpToolsCount = ref<number | null>(null)
const mcpToolsLoading = ref(false)

const detailName = computed(() =>
  detailType.value === 'skill' ? detailSkill.value?.name || '' : detailMcp.value?.name || ''
)
const detailCategory = computed(() =>
  (detailType.value === 'skill' ? detailSkill.value?.category : detailMcp.value?.category) || ''
)
const detailCreatorName = computed(() =>
  (detailType.value === 'skill' ? detailSkill.value?.creator_name : detailMcp.value?.creator_name)
  || t('skillsMcp.systemCreator')
)
const detailCreatedAt = computed(() => {
  const raw = detailType.value === 'skill' ? detailSkill.value?.created_at : detailMcp.value?.created_at
  return raw ? formatStringDate(raw) : '—'
})
// 帮助网址（000109）：链接文字显示主机名（完整 URL 在 hover/复制时可见）
const helpUrlHost = computed(() => {
  const raw = detailSkill.value?.help_url || ''
  if (!raw) return ''
  try {
    return new URL(raw).host || raw
  } catch {
    return raw
  }
})

async function copyDetailName() {
  const name = detailName.value
  if (!name) return
  try {
    await navigator.clipboard.writeText(name)
    MessagePlugin.success(t('skillsMcp.nameCopied'))
  } catch {
    MessagePlugin.warning(t('skillsMcp.copyFailed'))
  }
}

const detailDescription = computed(() =>
  detailType.value === 'skill' ? detailSkill.value?.description : detailMcp.value?.description
)
const detailReferencingAgents = computed(() => {
  if (!detailVisible.value) return []
  if (detailType.value === 'skill' && detailSkill.value) {
    return referencingAgents('skill', detailSkill.value.name, chatResources.agents || [])
  }
  if (detailMcp.value) return referencingAgents('mcp', detailMcp.value.id, chatResources.agents || [])
  return []
})
const mcpToolsText = computed(() => {
  if (mcpToolsLoading.value) return t('skillsMcp.toolsLoading')
  return mcpToolsCount.value === null ? '—' : String(mcpToolsCount.value)
})

function openDetail(type: 'skill' | 'mcp', id: string) {
  mcpToolsCount.value = null
  mcpToolsLoading.value = false
  if (type === 'skill') {
    detailSkill.value = skillIndex.value.get(id) || null
    detailMcp.value = null
    if (detailSkill.value) pins.touchRecent('skill', id)
  } else {
    detailMcp.value = mcpIndex.value.get(id) || null
    detailSkill.value = null
    if (detailMcp.value) {
      pins.touchRecent('mcp', id)
      // 工具数惰性加载：只在打开 MCP 详情时才握手远端。
      mcpToolsLoading.value = true
      getMCPServiceTools(id)
        .then((tools) => { mcpToolsCount.value = Array.isArray(tools) ? tools.length : 0 })
        .catch(() => { mcpToolsCount.value = 0 })
        .finally(() => { mcpToolsLoading.value = false })
    }
  }
  detailType.value = type
  editingCategory.value = detailCategory.value
  detailVisible.value = true
}

function goAgents() {
  router.push('/platform/agents')
}

// ---------- 管理员改分类 ----------
// 000107：技能目录写操作放宽到本空间 admin/owner 或系统管理员（后端 PUT
// /skills/catalog/:id = AdminOrSystemAdmin）；MCP 行平台全局仍仅系统管理员，
// builtin 行后端拒绝更新 → 不显示编辑入口。
const canEditCategory = computed(() => {
  if (detailType.value === 'skill') {
    return !!detailSkill.value && (authStore.isSystemAdmin || authStore.hasRole('admin'))
  }
  return !!detailMcp.value && !detailMcp.value.is_builtin && authStore.isSystemAdmin
})
const editingCategory = ref('')
const categorySaving = ref(false)

async function saveCategory() {
  const cat = editingCategory.value.trim()
  try {
    categorySaving.value = true
    if (detailType.value === 'skill' && detailSkill.value) {
      await updateSkillCatalogMeta(detailSkill.value.id, cat)
      detailSkill.value.category = cat
      const row = skills.value.find((s) => s.id === detailSkill.value!.id)
      if (row) row.category = cat
    } else if (detailMcp.value) {
      await updateMCPService(detailMcp.value.id, { category: cat } as Partial<MCPService>)
      detailMcp.value.category = cat
      const row = mcpServices.value.find((s) => s.id === detailMcp.value!.id)
      if (row) row.category = cat
    }
    MessagePlugin.success(t('skillsMcp.categorySaved'))
  } catch (err) {
    console.warn('[SkillsMcpList] failed to save category:', err)
    MessagePlugin.error(t('skillsMcp.categorySaveFailed'))
  } finally {
    categorySaving.value = false
  }
}
</script>

<style scoped lang="less">
.skills-mcp-container {
  display: flex;
  width: 100%;
  height: 100%;
  overflow: hidden;
}

.skills-mcp-content {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.header {
  padding: 16px 24px 0;

  .title-row {
    display: flex;
    align-items: center;
    gap: 12px;

    h2 {
      margin: 0;
      font-size: 20px;
      font-weight: 600;
      color: var(--td-text-color-primary);
    }
  }

  .header-subtitle {
    margin: 4px 0 0;
    font-size: 13px;
    color: var(--td-text-color-secondary);
  }
}

.skills-mcp-main {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 8px 24px 24px;
}

.skills-mcp-tabs {
  margin-bottom: 4px;

  :deep(.t-tabs__header) {
    border-bottom: none;
  }
}

.toolbar {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 8px 0 12px;

  .search-input {
    max-width: 360px;
  }

  .category-row {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }

  .category-chip {
    padding: 3px 12px;
    border-radius: 12px;
    border: 1px solid var(--td-component-stroke);
    background: var(--td-bg-color-container);
    color: var(--td-text-color-secondary);
    font-size: 12px;
    cursor: pointer;
    transition: all 0.15s ease;

    &:hover {
      color: var(--td-text-color-primary);
      border-color: var(--td-brand-color);
    }

    &.active {
      background: var(--td-brand-color);
      border-color: var(--td-brand-color);
      color: #fff;
    }
  }

  .count-label {
    font-size: 12px;
    color: var(--td-text-color-secondary);
  }
}

.card-wrap {
  display: grid;
  gap: 12px;
  grid-template-columns: 1fr;
  animation: contentFadeIn 0.32s ease-out;
}

@keyframes contentFadeIn {
  from {
    opacity: 0;
    transform: translateY(6px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.item-card {
  position: relative;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
  background: var(--td-bg-color-container);
  cursor: pointer;
  transition: all 0.25s ease;
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  height: 118px;
  min-height: 118px;
  box-sizing: border-box;

  &:hover {
    border-color: var(--td-brand-color);
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
    transform: translateY(-2px);
  }

  .card-title-row {
    display: flex;
    align-items: center;
    gap: 6px;
    padding-right: 22px;

    .card-title {
      font-size: 14px;
      font-weight: 600;
      color: var(--td-text-color-primary);
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .version-badge {
      flex-shrink: 0;
      padding: 1px 6px;
      border-radius: 4px;
      background: var(--td-bg-color-secondarycontainer);
      color: var(--td-text-color-secondary);
      font-size: 11px;

      &.builtin {
        background: var(--td-brand-color-light);
        color: var(--td-brand-color);
      }
    }

    .status-badge {
      flex-shrink: 0;
      display: inline-flex;
      align-items: center;
      gap: 4px;
      padding: 1px 6px;
      border-radius: 4px;
      font-size: 11px;

      &.ok {
        color: var(--td-success-color);
        background: var(--td-success-color-1);
      }

      &.off {
        color: var(--td-text-color-placeholder);
        background: var(--td-bg-color-secondarycontainer);
      }
    }
  }

  .card-desc {
    margin: 6px 0 0;
    font-size: 12px;
    line-height: 1.5;
    color: var(--td-text-color-secondary);
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .card-footer {
    margin-top: auto;
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;

    .category-tag {
      flex-shrink: 0;
      padding: 1px 8px;
      border-radius: 8px;
      background: var(--td-bg-color-secondarycontainer);
      color: var(--td-text-color-secondary);
      font-size: 11px;
    }

    .creator {
      font-size: 11px;
      color: var(--td-text-color-placeholder);
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  }

  .favorite-star {
    position: absolute;
    top: 8px;
    right: 8px;
    width: 24px;
    height: 24px;
    display: flex;
    align-items: center;
    justify-content: center;
    border: none;
    background: transparent;
    color: var(--td-text-color-placeholder);
    cursor: pointer;
    border-radius: 4px;
    transition: all 0.15s ease;

    &:hover {
      color: var(--td-warning-color);
    }

    &.is-favorited {
      color: var(--td-warning-color);
    }
  }
}

.skeleton-card {
  cursor: default;

  &:hover {
    border-color: var(--td-component-stroke);
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
    transform: none;
  }
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 0;

  .empty-img {
    width: 72px;
    height: 72px;
    opacity: 0.5;
  }

  .empty-txt {
    margin-top: 12px;
    font-size: 14px;
    font-weight: 500;
    color: var(--td-text-color-primary);
  }

  .empty-desc {
    margin-top: 6px;
    font-size: 12px;
    color: var(--td-text-color-secondary);
  }
}

// ---------- 详情弹窗 ----------
.detail-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.detail-section {
  .section-title {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 13px;
    font-weight: 600;
    color: var(--td-text-color-primary);
    margin-bottom: 8px;

    .section-count {
      padding: 0 8px;
      border-radius: 8px;
      background: var(--td-bg-color-secondarycontainer);
      color: var(--td-text-color-secondary);
      font-size: 11px;
      font-weight: 500;
    }
  }
}

.info-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px 16px;

  .info-item {
    display: flex;
    align-items: baseline;
    gap: 8px;
    font-size: 12px;
    min-width: 0;

    &.wide {
      grid-column: 1 / -1;
    }

    .info-label {
      flex-shrink: 0;
      color: var(--td-text-color-secondary);
    }

    .info-value {
      color: var(--td-text-color-primary);
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    // 帮助网址（000109）：主机名 + 外链小图标，新窗口打开
    .info-link {
      display: inline-flex;
      align-items: center;
      gap: 3px;
      color: var(--td-brand-color);
      text-decoration: none;

      &:hover {
        text-decoration: underline;
      }
    }
  }
}

// 000110：详情弹窗标题行——技能名 + 安装状态徽标 + 复制按钮
.detail-title-row {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  padding-right: 24px; // 给右上角关闭按钮让位

  .detail-title-name {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 16px;
    font-weight: 600;
    color: var(--td-text-color-primary);
  }

  .status-badge {
    flex-shrink: 0;
  }

  .detail-title-copy {
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
    color: var(--td-text-color-placeholder);
    cursor: pointer;

    &:hover {
      color: var(--td-brand-color);
      background: var(--td-bg-color-container-hover);
    }
  }
}

.usage-text {
  margin: 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--td-text-color-primary);
  white-space: pre-wrap;
  word-break: break-word;
}

.usage-hint {
  margin: 6px 0 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--td-text-color-placeholder);
}

.agent-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;

  .agent-chip {
    cursor: pointer;
  }
}

.category-edit-row {
  display: flex;
  gap: 8px;

  .category-edit-input {
    flex: 1;
  }
}

// 响应式布局：与 AgentList 卡片网格同一套断点
@media (min-width: 900px) {
  .card-wrap {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (min-width: 1250px) {
  .card-wrap {
    grid-template-columns: repeat(3, 1fr);
  }
}

@media (min-width: 1600px) {
  .card-wrap {
    grid-template-columns: repeat(4, 1fr);
  }
}

@media (min-width: 1900px) {
  .card-wrap {
    grid-template-columns: repeat(5, 1fr);
  }
}

@media (min-width: 2200px) {
  .card-wrap {
    grid-template-columns: repeat(6, 1fr);
  }
}
</style>
