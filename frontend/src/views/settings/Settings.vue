<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="visible" class="settings-overlay">
        <div class="settings-modal">
          <!-- 关闭按钮 -->
          <button class="close-btn" @click="handleClose" :aria-label="$t('general.close')">
            <svg width="20" height="20" viewBox="0 0 20 20" fill="currentColor">
              <path d="M15 5L5 15M5 5L15 15" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
            </svg>
          </button>

          <div class="settings-container">
            <!-- 左侧导航 -->
            <div class="settings-sidebar">
              <div class="sidebar-header">
                <h2 class="sidebar-title">{{ $t('settings.personalWorkspaceSettings') }}</h2>
              </div>
              <div class="settings-nav">
                <template v-for="group in navGroups" :key="group.key">
                  <div class="nav-group-title">{{ group.label }}</div>
                  <template v-for="item in group.items" :key="item.key">
                    <div :class="['nav-item', {
                      'active': currentSection === item.key,
                      'has-submenu': item.children && item.children.length > 0,
                      'expanded': expandedMenus.includes(item.key)
                    }]" @click="handleNavClick(item)">
                      <span v-if="item.emoji" class="nav-icon nav-icon-emoji">{{ item.emoji }}</span>
                      <t-icon v-else :name="item.icon" class="nav-icon" />
                      <span class="nav-label">{{ item.label }}</span>
                      <t-icon v-if="item.children && item.children.length > 0"
                        :name="expandedMenus.includes(item.key) ? 'chevron-down' : 'chevron-right'"
                        class="expand-icon" />
                    </div>

                    <!-- 子菜单 -->
                    <Transition name="submenu">
                      <div v-if="item.children && expandedMenus.includes(item.key)" class="submenu">
                        <div v-for="(child, childIndex) in item.children" :key="childIndex"
                          :class="['submenu-item', { 'active': currentSubSection === child.key }]"
                          @click.stop="handleSubMenuClick(item.key, child.key)">
                          <span class="submenu-label">{{ child.label }}</span>
                        </div>
                      </div>
                    </Transition>
                  </template>
                </template>
              </div>
            </div>

            <!-- 右侧内容区域 -->
            <div class="settings-content">
              <div class="content-wrapper" :class="{
                'content-wrapper--wide': currentSection === 'members',
                'content-wrapper--full': SYSTEM_ADMIN_SECTIONS.has(currentSection) || isIntegrationSection(currentSection),
              }">
                <!-- 角色不允许访问当前 section（deep-link 进来 / 跨空间切换后角色降级）—— 优先于具体 section 渲染。
                     正常导航走 navItems filter 不会到这里，但 watch(navItems) 的 fallback 会在角色降级
                     的瞬间触发；这一段做兜底兼容旧 URL。 -->
                <div v-if="!canSeeSection(currentSection)" class="section role-denied">
                  <div class="role-denied-icon">
                    <t-icon name="lock-on" size="48px" />
                  </div>
                  <div class="role-denied-title">{{ $t('settings.roleDenied.title') }}</div>
                  <div class="role-denied-desc">{{ $t('settings.roleDenied.desc') }}</div>
                </div>
                <template v-else>
                  <!-- 常规设置 -->
                  <div v-if="currentSection === 'general'" class="section">
                    <GeneralSettings />
                  </div>

                  <!-- 消息管理 -->
                  <div v-if="currentSection === 'chathistory'" class="section">
                    <ChatHistorySettings />
                  </div>

                  <!-- 长期记忆（空间级开关） -->
                  <div v-if="currentSection === 'memory'" class="section">
                    <MemoryWorkspaceSettings />
                  </div>

                  <!-- 我的记忆（个人记忆管理） -->
                  <div v-if="currentSection === 'mymemory'" class="section">
                    <MemorySettings />
                  </div>

                  <!-- 沙箱密钥（成员自己的技能 / 沙箱密钥） -->
                  <div v-if="currentSection === 'envvars'" class="section">
                    <EnvVarSettings />
                  </div>

                  <!-- 技能目录（000099）：空间侧只读视图，写动作仅系统管理员 -->
                  <div v-if="currentSection === 'skill-catalog'" class="section">
                    <SkillCatalogSettings />
                  </div>

                  <!-- 空间 MCP（000096 平台化后的空间只读视图）：列出管理后台
                       分配给本空间生效的 MCP 服务，不提供写入口。 -->
                  <div v-if="currentSection === 'tenant-mcp'" class="section">
                    <TenantMcpSettings />
                  </div>

                  <!-- 系统信息 -->
                  <div v-if="currentSection === 'system'" class="section">
                    <SystemInfo />
                  </div>

                  <!-- 系统管理员可见的全局运行时设置 -->
                  <div v-if="currentSection === 'system-global'" class="section">
                    <SystemSettings />
                  </div>

                  <!-- 系统管理员可见的任务队列运行状态 -->
                  <div v-if="currentSection === 'runtime-queues'" class="section">
                    <RuntimeQueues />
                  </div>

                  <div v-if="currentSection === 'platform-api-keys'" class="section">
                    <PlatformAPIKeys />
                  </div>

                  <div v-if="currentSection === 'system-audit-log'" class="section">
                    <SystemAuditLog />
                  </div>

                  <!-- 用户信息（账户基础信息：ID / 用户名 / 邮箱 / 注册时间）。
                     用户的基本信息不该跟 owner 权限绑定。 -->
                  <div v-if="currentSection === 'userprofile'" class="section">
                    <UserProfile />
                  </div>

                  <!-- 个人 AI 使用统计（Token统计与计费设计.md §5.1）：
                       当前空间内本人的调用量与 Token 消耗。 -->
                  <div v-if="currentSection === 'usage-stats'" class="section">
                    <UsageStatsSettings />
                  </div>

                  <!-- 空间信息 -->
                  <div v-if="currentSection === 'tenant'" class="section">
                    <TenantInfo />
                  </div>

                  <!-- 成员管理 (#1303 PR 3) -->
                  <div v-if="currentSection === 'members'" class="section">
                    <TenantMembers />
                  </div>

                  <!-- 空间用量统计（Token统计与计费设计.md §5.2）：
                       空间管理员的「我的 + 全空间」双页签看板。 -->
                  <div v-if="currentSection === 'tenant-usage'" class="section">
                    <TenantUsagePanel />
                  </div>

                  <!-- 发布集成 -->
                  <div v-if="isIntegrationSection(currentSection)" class="section">
                    <IntegrationSettingsSection :tab="integrationTabFromSection(currentSection)" />
                  </div>
                </template>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { LocationQueryRaw } from 'vue-router'
import { useUIStore } from '@/stores/ui'
import { useAuthStore } from '@/stores/auth'
import { useDeploymentCapabilitiesStore } from '@/stores/deploymentCapabilities'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import SystemInfo from './SystemInfo.vue'
import TenantInfo from './TenantInfo.vue'
import UserProfile from './UserProfile.vue'
import GeneralSettings from './GeneralSettings.vue'
import ChatHistorySettings from './ChatHistorySettings.vue'
import MemorySettings from './MemorySettings.vue'
import EnvVarSettings from './EnvVarSettings.vue'
import SkillCatalogSettings from './SkillCatalogSettings.vue'
import TenantMcpSettings from './TenantMcpSettings.vue'
import MemoryWorkspaceSettings from './MemoryWorkspaceSettings.vue'
import TenantMembers from './TenantMembers.vue'
import UsageStatsSettings from './UsageStatsSettings.vue'
import TenantUsagePanel from './TenantUsagePanel.vue'
import SystemSettings from '@/views/system/SystemSettings.vue'
import RuntimeQueues from '@/views/system/RuntimeQueues.vue'
import PlatformAPIKeys from '@/views/system/PlatformAPIKeys.vue'
import SystemAuditLog from '@/views/system/SystemAuditLog.vue'
import IntegrationSettingsSection from '@/views/integrations/IntegrationSettingsSection.vue'
import {
  INTEGRATION_PREVIEW_ITEMS,
  INTEGRATION_TAB_CAPABILITY,
  INTEGRATION_TAB_MIN_ROLE,
} from '@/config/integrations'
import {
  SETTINGS_SECTION_MIN_ROLE,
  SYSTEM_ADMIN_SETTINGS_SECTIONS,
} from '@/config/settingsAccess'
import { SETTINGS_SECTION_CAPABILITY } from '@/config/deploymentCapabilities'
import {
  buildSettingsRouteQuery,
  integrationSectionKey,
  integrationTabFromSection,
  isIntegrationSection,
  normalizeSettingsSection as normalizeSettingsSectionFromQuery,
  settingsQueryUnchanged,
} from '@/config/settingsRoute'

const route = useRoute()
const router = useRouter()
const uiStore = useUIStore()
const authStore = useAuthStore()
const deploymentCapabilities = useDeploymentCapabilitiesStore()
const { t } = useI18n()

const currentSection = ref<string>('general')
const currentSubSection = ref<string>('')
const expandedMenus = ref<string[]>([])

type NavItem = {
  key: string
  icon: string
  label: string
  emoji?: string
  children?: Array<{ key: string; label: string }>
}

type NavGroup = {
  key: string
  label: string
  items: NavItem[]
}

// 设置二级导航的最低可见角色来自 settingsAccess.ts，和
// internal/router/router.go 的守卫矩阵对齐。
// 以「页面里至少有 1 个有意义的写操作所要求的最低角色」为基准：
// 只读类（general / system info / tenant-info / members 名册）保留 viewer
// 可见；chathistory 配置收到 admin；最高敏感的 reset api key 是
// owner-only。改这张表前请在 router.go 里复核对应路由组。
//
// 特别说明：
// - chathistory 页面唯一的「启用消息索引」开关 PUT /tenants/kv/chat-history-config
//   后端走 g.Admin()。给 viewer/contributor 看到入口、点开开关、保存时
//   403，体验很差，所以入口本身归 admin。
// - models / ollama / weknoracloud 三页已随 000094 模型平台化、
//   websearch / vectorstore / parser / storage / sandbox / mcp 六页已随
//   000095 基础设施收权迁入系统管理控制台，空间 Settings 不再出现
//   （settingsAccess.ts 同步收表，旧深链走 LEGACY_CONSOLE_SECTIONS）。
//   skills 以只读「技能目录」回来（000099，见 settingsAccess.ts）。
const SYSTEM_ADMIN_SECTIONS = SYSTEM_ADMIN_SETTINGS_SECTIONS

const normalizeSettingsSection = (section: string) => {
  return normalizeSettingsSectionFromQuery(section, route.query.tab as string | undefined)
}

// 000094 模型平台化：models / ollama / weknoracloud 三页迁入系统管理控制台。
// 000095 基础设施收权：websearch / vectorstore / parser / storage / sandbox /
// mcp 六页同样迁入控制台。旧深链、openSettings 调用与 settings-nav 事件在
// 这里统一改道：系统管理员直接跳控制台对应面板，其他角色提示权限变化后
// 回到首个可见面板（weknoracloud 的模型侧归模型管理，解析引擎凭证仍在
// parser 页内）。其中 websearch 经 000095 平台化已是平台目录 + 分配制，
// 与 models/ollama 一样不再需要目标空间。skills 不在此列：技能以只读
// 「技能目录」回到空间 Settings（000099），旧键经 settingsRoute 的别名表
// 归一化到 skill-catalog。
const LEGACY_CONSOLE_SECTIONS: Record<string, string> = {
  models: 'models',
  ollama: 'ollama',
  weknoracloud: 'models',
  websearch: 'websearch',
  vectorstore: 'vectorstore',
  parser: 'parser',
  storage: 'storage',
  // 沙箱配置面板已并入「沙箱连接」（000097 分配制），旧入口一并改道。
  sandbox: 'sandbox-connections',
  mcp: 'mcp',
}

// 控制台里仍按空间代管的面板 — 只有这些深链需要 ?tenant= 预选空间。
const MANAGED_CONSOLE_TARGETS = new Set(['vectorstore', 'parser', 'storage'])

const redirectLegacySection = (section: string): boolean => {
  const target = LEGACY_CONSOLE_SECTIONS[section]
  if (!target) return false
  if (authStore.isSystemAdmin) {
    // 代管目标空间默认取系统管理员当前所在空间（没绑空间时留空，由
    // 控制台顶栏自行选择）。
    if (uiStore.showSettingsModal) uiStore.closeSettings()
    const query: Record<string, string> = { section: target }
    if (MANAGED_CONSOLE_TARGETS.has(target)) {
      const currentTenant = localStorage.getItem('weknora_selected_tenant_id')
      if (currentTenant) query.tenant = currentTenant
    }
    void router.push({ path: '/system/console', query })
    return true
  }
  MessagePlugin.warning(t('settings.movedToConsole'))
  const fallback = navItems.value[0]?.key || 'general'
  currentSection.value = fallback
  currentSubSection.value = ''
  syncSettingsRoute(fallback)
  return true
}

const syncSettingsRoute = (sectionKey: string) => {
  if (route.path !== '/platform/settings') return
  const query = buildSettingsRouteQuery(sectionKey, route.query)
  if (settingsQueryUnchanged(route.query, query)) return
  void router.replace({
    path: '/platform/settings',
    query: query as LocationQueryRaw,
  })
}

const isSectionSupported = (key: string): boolean => {
  if (isIntegrationSection(key)) {
    return deploymentCapabilities.isSupported(
      INTEGRATION_TAB_CAPABILITY[integrationTabFromSection(key)],
    )
  }
  return deploymentCapabilities.isSupported(SETTINGS_SECTION_CAPABILITY[key])
}

const canSeeSection = (key: string): boolean => {
  if (isIntegrationSection(key)) {
    const min = INTEGRATION_TAB_MIN_ROLE[integrationTabFromSection(key)]
    if (!min) return true
    if (authStore.canAccessAllTenants) return true
    return authStore.hasRole(min)
  }
  if (SYSTEM_ADMIN_SECTIONS.has(key)) {
    return authStore.isSystemAdmin
  }
  const min = SETTINGS_SECTION_MIN_ROLE[key] ?? 'viewer'
  // canAccessAllTenants（superuser）和路由层一样必须 bypass，否则 cross-tenant
  // 管理员看不到自己有权操作的入口（参考 TenantMembers.vue 的 canManage）。
  if (authStore.canAccessAllTenants) return true
  return authStore.hasRole(min)
}

const navItems = computed(() => {
  // 一律走 SETTINGS_SECTION_MIN_ROLE 表，避免 ad-hoc isAdmin/isOwner 散落在多处。
  // 服务端在每条路由上仍以 g.Viewer/Admin/Owner 为准，这里只决定 UI 是
  // 否露入口；改动入口规则请同步更新 settingsAccess.ts 和对应后端路由。
  // 发布集成暂时收敛：IM 集成 / 网页嵌入 / Claw Skill 入口隐藏（路由保留，
  // 恢复时从这里去掉过滤即可）；Chrome 插件挪到「个人」组展示。
  const hiddenIntegrationTabs = new Set(['im', 'embed', 'claw'])
  const integrationItems: NavItem[] = INTEGRATION_PREVIEW_ITEMS
    .filter((item) => !hiddenIntegrationTabs.has(item.key))
    .map((item) => ({
      key: integrationSectionKey(item.key),
      icon: item.icon.type === 'icon' ? item.icon.name : 'integration',
      emoji: item.icon.type === 'emoji' ? item.icon.value : undefined,
      label: t(`integrations.tabs.${item.key}`),
    }))
  const all: NavItem[] = [
    { key: 'general', icon: 'setting', label: t('general.title') },
    { key: 'chathistory', icon: 'chat', label: t('chatHistorySettings.title') },
    { key: 'memory', icon: 'bulletpoint', label: t('memoryWorkspaceSettings.title') },
    // 沙箱密钥（envvars）/ 版本信息（system）入口暂时隐藏：深链 ?section=
    // 会被 watch(navItems) 兜底回第一个可见项，渲染分支保留不影响。
    { key: 'system-global', icon: 'server', label: t('settings.system') },
    { key: 'runtime-queues', icon: 'queue', label: t('settings.taskQueue') },
    { key: 'platform-api-keys', icon: 'secured', label: t('platformApiKeys.title') },
    { key: 'system-audit-log', icon: 'history', label: t('system.globalSettings.audit.tabLabel') },
    { key: 'userprofile', icon: 'user', label: t('userProfile.title') },
    { key: 'usage-stats', icon: 'chart-bar', label: t('usageStats.title') },
    { key: 'mymemory', icon: 'bookmark', label: t('memorySettings.title') },
    { key: 'skill-catalog', icon: 'rocket', label: t('settings.skills.title') },
    { key: 'tenant-mcp', icon: 'tools', label: t('settings.tenantMcp') },
    { key: 'tenant', icon: 'user-circle', label: t('settings.tenantInfo') },
    { key: 'members', icon: 'usergroup', label: t('tenantMember.title') },
    { key: 'tenant-usage', icon: 'chart-pie', label: t('usageStats.tenantMenu') },
    ...integrationItems,
  ]
  // currentTenantRole 为空表示「membership 还没加载」—— 比起渲染整套
  // viewer 入口然后角色一返回又消失，先卡住不渲染更稳，跟原先 members
  // 入口的策略一致。
  if (!authStore.currentTenantRole && !authStore.canAccessAllTenants) {
    return [] as NavItem[]
  }
  return all.filter((it) => canSeeSection(it.key) && isSectionSupported(it.key))
})

const navGroups = computed<NavGroup[]>(() => {
  const itemMap = new Map(navItems.value.map((item) => [item.key, item]))
  const pickItems = (keys: string[]) => keys.map((key) => itemMap.get(key)).filter(Boolean) as NavItem[]
  // 分组：账户 → 空间 → 发布集成 → 系统管理 → 平台
  // 历史调整：个人偏好(general)和用户信息收进「账户」；空间内功能开关
  // (chathistory)从「平台」挪到「空间」；「数据与扩展」分组随基础设施
  // 收权整体迁入系统管理控制台后删除。
  return [
    {
      key: 'account',
      label: t('settings.navGroups.account'),
      // Chrome 插件与个人相关，挪到「个人」组、放在「我的记忆」下面。
      items: pickItems(['general', 'userprofile', 'usage-stats', 'mymemory', integrationSectionKey('chrome')]),
    },
    {
      key: 'workspace',
      label: t('settings.navGroups.workspace'),
      items: pickItems(['tenant', 'members', 'tenant-usage', 'chathistory', 'memory', 'skill-catalog', 'tenant-mcp']),
    },
    {
      // IM / 网页嵌入 / Claw / Chrome 已隐藏或迁组，发布集成只剩 API。
      key: 'integrations',
      label: t('integrations.title'),
      items: pickItems([
        integrationSectionKey('api'),
      ]),
    },
    {
      key: 'system_administration',
      label: t('settings.navGroups.systemAdministration'),
      items: pickItems(['system-global', 'runtime-queues', 'platform-api-keys', 'system-audit-log']),
    },
    {
      // 版本信息（system）入口暂时隐藏，组内为空自动不渲染。
      key: 'platform',
      label: t('settings.navGroups.platform'),
      items: pickItems(['system']),
    },
  ].filter((group) => group.items.length > 0)
})

// 导航项点击处理
const handleNavClick = (item: any) => {
  if (item.children && item.children.length > 0) {
    // 有子菜单，切换展开状态
    const index = expandedMenus.value.indexOf(item.key)
    if (index > -1) {
      expandedMenus.value.splice(index, 1)
    } else {
      expandedMenus.value.push(item.key)
    }
    currentSubSection.value = item.children[0].key
  } else {
    currentSubSection.value = ''
  }

  // 切换到对应页面，并同步 URL 为 ?section=<navKey>（含 integration-claw）。
  // 否则从其它 section 点进来时 query 不变，路由监听会把内容拉回去。
  currentSection.value = item.key
  syncSettingsRoute(item.key)
}

// 子菜单点击处理
const handleSubMenuClick = (parentKey: string, childKey: string) => {
  currentSection.value = parentKey
  currentSubSection.value = childKey

  // 滚动到对应的模型类型区域
  setTimeout(() => {
    const element = document.querySelector(`[data-model-type="${childKey}"]`)
    if (element) {
      element.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }
  }, 100)
}

// 控制弹窗显示
const visible = computed(() => {
  return route.path === '/platform/settings' || uiStore.showSettingsModal
})

// 关闭弹窗
const handleClose = () => {
  // Blur before unmount so TDesign textarea autosize won't run on a detached node.
  if (document.activeElement instanceof HTMLElement) {
    document.activeElement.blur()
  }
  uiStore.closeSettings()
  // 如果当前路由是设置页，返回上一页
  if (route.path === '/platform/settings') {
    const sec = route.query.section
    if (sec === 'system-global' || sec === 'runtime-queues' || sec === 'platform-api-keys' || sec === 'system-audit-log') {
      router.push('/platform/knowledge-bases')
    } else {
      router.back()
    }
  }
}

// 监听初始导航设置
watch(() => uiStore.settingsInitialSection, (section) => {
  if (section && visible.value) {
    if (redirectLegacySection(section)) return
    const normalizedSection = normalizeSettingsSection(section)
    if (deploymentCapabilities.loaded && !isSectionSupported(normalizedSection)) {
      MessagePlugin.warning(t('settings.capabilityUnavailable'))
      currentSection.value = navItems.value[0]?.key || 'general'
      currentSubSection.value = ''
      return
    }
    currentSection.value = normalizedSection
    syncSettingsRoute(normalizedSection)
    const navItem = (navItems.value as any[]).find((item) => item.key === normalizedSection)
    if (navItem && navItem.children && navItem.children.length > 0) {
      if (!expandedMenus.value.includes(section)) {
        expandedMenus.value.push(section)
      }
      currentSubSection.value = uiStore.settingsInitialSubSection || navItem.children[0].key
      if (uiStore.settingsInitialSubSection) {
        setTimeout(() => {
          const element = document.querySelector(`[data-model-type="${uiStore.settingsInitialSubSection}"]`)
          if (element) {
            element.scrollIntoView({ behavior: 'smooth', block: 'start' })
          }
        }, 300)
      }
    } else {
      currentSubSection.value = ''
    }
  }
}, { immediate: true })

// skill-catalog（000099 回迁）无子菜单：openSettings('skill-catalog', sandboxId)
// 的预选由组件自己读 uiStore.settingsInitialSubSection / ?sandbox=，这里不
// 需要 subsection 监听；旧 openSettings('skills') 经 settingsRoute 别名归一化。

watch(
  () => [visible.value, route.path, route.query.section, deploymentCapabilities.loaded] as const,
  ([isVisible, path, section, capabilitiesLoaded]) => {
    if (!isVisible || path !== '/platform/settings') return
    if (typeof section !== 'string') {
      syncSettingsRoute(currentSection.value || 'general')
      return
    }
    if (redirectLegacySection(section)) return
    const normalizedSection = normalizeSettingsSectionFromQuery(
      section,
      typeof route.query.tab === 'string' ? route.query.tab : undefined,
    )
    if (capabilitiesLoaded && !isSectionSupported(normalizedSection)) {
      MessagePlugin.warning(t('settings.capabilityUnavailable'))
      const fallback = navItems.value[0]?.key || 'general'
      currentSection.value = fallback
      currentSubSection.value = ''
      syncSettingsRoute(fallback)
      return
    }
    currentSection.value = normalizedSection
    currentSubSection.value = ''
    syncSettingsRoute(normalizedSection)
  },
  { immediate: true },
)

// 切换空间后角色可能变化，原本可见的 admin-only 面板可能消失。
// 如果 currentSection 落到了不再显示的 key 上，就回退到第一个可见项。
watch(navItems, (items) => {
  if (!items.some((item) => item.key === currentSection.value)) {
    const fallback = items[0]?.key || 'general'
    currentSection.value = fallback
    currentSubSection.value = ''
    syncSettingsRoute(fallback)
  }
})

// ESC 键关闭
const handleEscape = (e: KeyboardEvent) => {
  if (e.key === 'Escape' && visible.value) {
    handleClose()
  }
}

// 处理快捷导航事件
const handleSettingsNav = (e: CustomEvent) => {
  const { section, subsection } = e.detail
  if (section) {
    if (redirectLegacySection(section)) return
    const normalizedSection = normalizeSettingsSection(section)
    if (deploymentCapabilities.loaded && !isSectionSupported(normalizedSection)) {
      MessagePlugin.warning(t('settings.capabilityUnavailable'))
      currentSection.value = navItems.value[0]?.key || 'general'
      currentSubSection.value = ''
      return
    }
    currentSection.value = normalizedSection
    syncSettingsRoute(normalizedSection)
    // 如果有子菜单，自动展开
    const navItem = (navItems.value as any[]).find((item: any) => item.key === normalizedSection)
    if (navItem && navItem.children && navItem.children.length > 0) {
      if (!expandedMenus.value.includes(section)) {
        expandedMenus.value.push(section)
      }
      // 如果有 subsection，选中对应的子菜单项
      currentSubSection.value = subsection || navItem.children[0].key
    }
  }
}

onMounted(() => {
  window.addEventListener('keydown', handleEscape)
  window.addEventListener('settings-nav', handleSettingsNav as EventListener)
})

watch(currentSection, () => {
  if (document.activeElement instanceof HTMLElement) {
    document.activeElement.blur()
  }
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleEscape)
  window.removeEventListener('settings-nav', handleSettingsNav as EventListener)
})
</script>

<style lang="less" scoped>
/* 遮罩层 */
.settings-overlay {
  position: fixed;
  inset: 0;
  z-index: 1100;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  backdrop-filter: blur(4px);
}

/* 弹窗容器 */
.settings-modal {
  position: relative;
  width: 100%;
  // 1080×780 trades a touch of small-screen real estate for noticeably
  // less cramped tables (member list, system settings rows). Outer
  // padding is 20px so 1080 + 40 = 1120, comfortably within typical
  // laptops (1280+). Below 1100px viewport the `width: 100%` kicks in
  // and the modal shrinks to fit minus the 20px padding.
  max-width: 1080px;
  height: 780px;
  max-height: calc(100vh - 40px);
  background: var(--td-bg-color-container);
  border-radius: 12px;
  box-shadow: 0 6px 28px rgba(15, 23, 42, 0.08);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

/* 关闭按钮 */
.close-btn {
  position: absolute;
  top: 16px;
  right: 16px;
  width: 32px;
  height: 32px;
  border: none;
  background: transparent;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s ease;
  z-index: 10;

  &:hover {
    background: var(--td-bg-color-container-hover);
    color: var(--td-text-color-primary);
  }
}

.settings-container {
  display: flex;
  height: 100%;
  width: 100%;
  overflow: hidden;
}

/* 左侧导航栏：略紧凑于最初版，字号与留白适中 */
.settings-sidebar {
  width: 208px;
  background-color: var(--td-bg-color-settings-modal);
  border-right: 1px solid var(--td-component-stroke);
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.sidebar-header {
  padding: 16px 14px 12px;
  border-bottom: 1px solid var(--td-component-stroke);
  flex-shrink: 0;
}

.sidebar-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  margin: 0;
}

.settings-nav {
  padding: 8px 8px 12px;
  flex: 1;
  overflow-y: auto;
  min-height: 0;
}

.nav-group-title {
  padding: 9px 14px 4px;
  color: var(--td-text-color-placeholder);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.02em;
}

.nav-item {
  display: flex;
  align-items: center;
  padding: 6px 12px;
  margin-bottom: 2px;
  border-radius: 6px;
  cursor: pointer;
  color: var(--td-text-color-primary);
  font-size: 14px;
  transition: all 0.2s ease;
  user-select: none;

  &:hover {
    background-color: var(--td-bg-color-container-hover);
    color: var(--td-text-color-primary);
  }

  &.active {
    background-color: var(--td-bg-color-secondarycontainer);
    color: var(--td-brand-color);
    font-weight: 500;
  }
}

.nav-icon {
  margin-right: 9px;
  font-size: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  color: inherit;
}

.nav-icon-emoji {
  font-size: 14px;
  line-height: 1;
}

.nav-label {
  flex: 1;
}

.expand-icon {
  margin-left: 4px;
  font-size: 14px;
  transition: transform 0.2s ease;
}

/* 子菜单 */
.submenu {
  margin-left: 28px;
  margin-bottom: 3px;
  overflow: hidden;
}

.submenu-item {
  padding: 5px 12px;
  margin-bottom: 2px;
  border-radius: 4px;
  cursor: pointer;
  color: var(--td-text-color-primary);
  font-size: 13px;
  transition: all 0.2s ease;
  user-select: none;

  &:hover {
    background-color: var(--td-bg-color-container-hover);
    color: var(--td-text-color-primary);
  }

  &.active {
    background-color: var(--td-bg-color-secondarycontainer);
    color: var(--td-brand-color);
    font-weight: 500;
  }
}

.submenu-label {
  display: block;
}

/* 子菜单动画 */
.submenu-enter-active,
.submenu-leave-active {
  transition: all 0.2s ease;
}

.submenu-enter-from {
  opacity: 0;
  max-height: 0;
}

.submenu-enter-to {
  opacity: 1;
  max-height: 300px;
}

.submenu-leave-from {
  opacity: 1;
  max-height: 300px;
}

.submenu-leave-to {
  opacity: 0;
  max-height: 0;
}

/* 右侧内容区域 */
.settings-content {
  flex: 1;
  overflow-y: auto;
  background-color: var(--td-bg-color-container);
}

.content-wrapper {
  // Bumped from 600 to 760 when the modal grew from 900→1080 (see
  // .settings-modal). Without this, single-column panes (General,
  // Tenant, API key, …) leave a wide right-hand gutter inside the
  // wider modal. 760 keeps comfortable reading-width on long
  // descriptions without the form fields stretching to the full
  // panel width — which would look stranger than a small gutter.
  max-width: 760px;
  padding: 40px 48px;

  /* 成员 / 审计表格列多，600px 会把操作列挤到贴边；铺满右侧内容列更稳。 */
  &--wide {
    max-width: none;
    width: 100%;
    padding: 32px 36px 40px;
    box-sizing: border-box;
  }

  &--full {
    max-width: none;
    width: 100%;
    padding: 30px 34px 40px;
    box-sizing: border-box;
  }
}

.section {
  animation: fadeIn 0.3s ease;
}

@keyframes fadeIn {
  from {
    opacity: 0;
    transform: translateY(10px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}

/* 弹窗动画 */
.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.2s ease;
}

.modal-enter-active .settings-modal,
.modal-leave-active .settings-modal {
  transition: transform 0.2s ease, opacity 0.2s ease;
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}

.modal-enter-from .settings-modal,
.modal-leave-to .settings-modal {
  transform: scale(0.95);
  opacity: 0;
}

/* 滚动条样式 */
.settings-nav::-webkit-scrollbar,
.settings-content::-webkit-scrollbar {
  width: 6px;
}

.settings-nav::-webkit-scrollbar-track {
  background: var(--td-bg-color-secondarycontainer);
}

.settings-nav::-webkit-scrollbar-thumb {
  background: var(--td-gray-color-5);
  border-radius: 3px;
}

.settings-nav::-webkit-scrollbar-thumb:hover {
  background: var(--td-gray-color-6);
}

.settings-content::-webkit-scrollbar-track {
  background: var(--td-bg-color-container);
}

.settings-content::-webkit-scrollbar-thumb {
  background: var(--td-gray-color-5);
  border-radius: 3px;
}

.settings-content::-webkit-scrollbar-thumb:hover {
  background: var(--td-gray-color-6);
}

.role-denied {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
  padding: 64px 24px;
  gap: 12px;
  min-height: 240px;

  .role-denied-icon {
    color: var(--td-text-color-placeholder);
  }

  .role-denied-title {
    font-size: 16px;
    font-weight: 600;
    color: var(--td-text-color-primary);
  }

  .role-denied-desc {
    font-size: 13px;
    color: var(--td-text-color-secondary);
    max-width: 360px;
    line-height: 1.6;
  }
}
</style>
