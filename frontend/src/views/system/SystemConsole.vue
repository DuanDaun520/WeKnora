<template>
  <main class="system-console">
    <header class="console-header">
      <div class="console-brand">
        <div class="console-mark" aria-hidden="true">
          <t-icon name="setting-1" size="26px" />
        </div>
        <div class="console-brand-text">
          <h1>{{ $t('systemConsole.title') }}</h1>
          <p>{{ $t('systemConsole.subtitle') }}</p>
        </div>
      </div>
      <div class="console-header-actions">
        <span class="console-me">
          <t-icon name="user-circle" size="16px" />
          {{ authStore.user?.employee_id || authStore.user?.username }}
        </span>
        <t-button v-if="authStore.hasValidTenant" variant="outline" @click="enterWorkspace">
          <template #icon><t-icon name="enter" /></template>
          {{ $t('systemConsole.enterWorkspace') }}
        </t-button>
        <t-button theme="default" variant="text" @click="handleLogout">
          <template #icon><t-icon name="logout" /></template>
          {{ $t('auth.logout') }}
        </t-button>
      </div>
    </header>

    <!-- 左侧菜单 + 内容区：系统管理功能以菜单项形式陆续加入。
         三个「按空间代管」面板需先在内容区顶部选择目标空间；
         网络搜索/沙箱连接/技能库为平台目录 + 按空间分配，无需选择空间。 -->
    <div class="console-body">
      <nav class="console-menu" :aria-label="$t('systemConsole.title')">
        <template v-for="group in menuGroups" :key="group.key">
          <div class="console-menu-group">{{ group.label }}</div>
          <div
            v-for="item in group.items"
            :key="item.key"
            :class="['console-menu-item', { active: currentSection === item.key }]"
            :aria-current="currentSection === item.key ? 'page' : undefined"
            @click="handleMenuClick(item.key)"
          >
            <t-icon :name="item.icon" size="16px" class="console-menu-icon" />
            <span>{{ item.label }}</span>
          </div>
        </template>
      </nav>

      <section class="console-content">
        <UsersPanel v-if="currentSection === 'users'" />
        <WorkspacesPanel v-else-if="currentSection === 'tenants'" />
        <ModelsPanel v-else-if="currentSection === 'models'" />
        <OllamaPanel v-else-if="currentSection === 'ollama'" />
        <!-- 网络搜索 000095 平台化：平台级服务目录 + 按空间分配，无空间选择栏 -->
        <WebSearchSettings v-else-if="currentSection === 'websearch'" />
        <!-- MCP 服务 000096 平台化：平台级服务目录 + 按空间分配，无空间选择栏 -->
        <McpSettings v-else-if="currentSection === 'mcp'" />
        <!-- 沙箱连接 000097 平台化：平台级连接目录 + 按空间物化分配，无空间选择栏。
             沙箱配置不再设独立代管面板：连接分配即物化配置，改连接后推送传播。 -->
        <SandboxConnectionsPanel v-else-if="currentSection === 'sandbox-connections'" />
        <!-- 技能库 000098 平台化：平台级技能注册 + 按空间物化分配，无空间选择栏 -->
        <SkillLibraryPanel v-else-if="currentSection === 'skill-library'" />
        <!-- AI用量统计（Token统计与计费设计.md §5.3）：平台总览/按空间/按用户 + 台账明细，
             并入「模型」菜单分组；模型单价配置在面板内以弹窗打开。 -->
        <UsageDashboardPanel v-else-if="currentSection === 'usage'" />
        <!-- 系统设置（平台级开关：SSRF 白名单 / Docker 沙箱等）。该面板原挂在
             /platform 外壳的「系统管理」组，而 bootstrap 系统管理员无空间绑定
             打不开外壳；迁进控制台让无空间管理员也能直达。 -->
        <SystemSettings v-else-if="currentSection === 'system-settings'" />

        <!-- 按空间代管面板：未选目标空间时给引导，选中后渲染管理页 -->
        <template v-else-if="isManagedSection(currentSection)">
          <ManagedWorkspaceBar />
          <template v-if="managedStore.managedTenantId">
            <VectorStoreSettings v-if="currentSection === 'vectorstore'" />
            <ParserEngineSettings v-else-if="currentSection === 'parser'" />
            <StorageBackendSettings v-else-if="currentSection === 'storage'" />
            <PlatformStorageEngines v-else-if="currentSection === 'platform-storage'" />
          </template>
          <div v-else class="managed-empty">
            <t-empty :description="$t('systemConsole.workspace.emptyHint')" />
          </div>
        </template>
      </section>
    </div>
  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import type { LocationQueryRaw } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useManagedWorkspaceStore } from '@/stores/managedWorkspace'
import { useDeploymentCapabilitiesStore } from '@/stores/deploymentCapabilities'
import { CONSOLE_SECTION_CAPABILITY } from '@/config/deploymentCapabilities'
import { logout as logoutApi } from '@/api/auth'
import UsersPanel from './UsersPanel.vue'
import WorkspacesPanel from './WorkspacesPanel.vue'
import ModelsPanel from './ModelsPanel.vue'
import OllamaPanel from './OllamaPanel.vue'
import WebSearchSettings from './WebSearchSettings.vue'
import VectorStoreSettings from './VectorStoreSettings.vue'
import ParserEngineSettings from './ParserEngineSettings.vue'
import StorageBackendSettings from './StorageBackendSettings.vue'
import PlatformStorageEngines from './PlatformStorageEngines.vue'
import McpSettings from './McpSettings.vue'
import SandboxConnectionsPanel from './SandboxConnectionsPanel.vue'
import SkillLibraryPanel from './SkillLibraryPanel.vue'
import UsageDashboardPanel from './UsageDashboardPanel.vue'
import SystemSettings from './SystemSettings.vue'
import ManagedWorkspaceBar from './ManagedWorkspaceBar.vue'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const managedStore = useManagedWorkspaceStore()
const deploymentCapabilities = useDeploymentCapabilitiesStore()

type ConsoleSection =
  | 'users'
  | 'tenants'
  | 'models'
  | 'ollama'
  | 'websearch'
  | 'vectorstore'
  | 'parser'
  | 'storage'
  | 'platform-storage'
  | 'sandbox-connections'
  | 'skill-library'
  | 'mcp'
  | 'usage'
  | 'system-settings'

// 按空间代管的三个面板：数据仍归属各空间，系统管理员选目标空间后管理。
// 网络搜索（000095）与 MCP 服务（000096）平台化后是平台目录 + 分配制，
// 不再是代管面板；沙箱配置并入「沙箱连接」（000097 物化分配制）；技能
// 面板迁回工作空间 Settings 的只读「技能目录」（000099）。
const MANAGED_SECTIONS = new Set<ConsoleSection>([
  'vectorstore',
  'parser',
  'storage',
])

const VALID_SECTIONS: ConsoleSection[] = [
  'users',
  'tenants',
  'models',
  'ollama',
  'websearch',
  'vectorstore',
  'parser',
  'storage',
  'platform-storage',
  'sandbox-connections',
  'skill-library',
  'mcp',
  'usage',
  'system-settings',
]

function isManagedSection(key: ConsoleSection): boolean {
  return MANAGED_SECTIONS.has(key)
}

type MenuItem = { key: ConsoleSection; icon: string; label: string }
type MenuGroup = { key: string; label: string; items: MenuItem[] }

// 能力裁剪沿用工作空间 Settings 的键（CONSOLE_SECTION_CAPABILITY）：
// 部署没有沙箱/MCP/搜索时控制台菜单同样隐藏对应项。
function isSectionSupported(key: ConsoleSection): boolean {
  return deploymentCapabilities.isSupported(CONSOLE_SECTION_CAPABILITY[key])
}

const menuGroups = computed<MenuGroup[]>(() => {
  // 注：ollama 运行时菜单当前被隐藏（保留渲染分支与深链支持）；模型
  // 单价已并入 AI用量统计面板（弹窗），不再占用独立菜单项。
  const all: MenuItem[] = [
    { key: 'users', icon: 'user', label: t('systemConsole.menu.users') },
    { key: 'tenants', icon: 'system-sum', label: t('systemConsole.menu.tenants') },
    { key: 'models', icon: 'ai', label: t('systemConsole.menu.models') },
    { key: 'usage', icon: 'chart-bar', label: t('usageStats.adminMenu') },
    { key: 'skill-library', icon: 'root-list', label: t('settings.skillLibrary') },
    { key: 'mcp', icon: 'tools', label: t('settings.mcpService') },
    { key: 'websearch', icon: 'search', label: t('settings.webSearchConfig') },
    { key: 'sandbox-connections', icon: 'link', label: t('settings.sandboxConnections') },
    { key: 'vectorstore', icon: 'data-base', label: t('settings.vectorStoreEngine') },
    { key: 'parser', icon: 'file-search', label: t('settings.parserEngine') },
    { key: 'storage', icon: 'cloud', label: t('settings.storageEngine') },
    { key: 'platform-storage', icon: 'cloud-upload', label: t('settings.platformStorage.title') },
    { key: 'system-settings', icon: 'server', label: t('settings.system') },
  ]
  const visible = all.filter((item) => isSectionSupported(item.key))
  const pick = (keys: ConsoleSection[]) =>
    keys.map((key) => visible.find((item) => item.key === key)).filter(Boolean) as MenuItem[]
  return [
    {
      key: 'workspace',
      label: t('systemConsole.menuGroups.workspace'),
      items: pick(['users', 'tenants']),
    },
    {
      // 模型分组：模型配置 + AI用量统计（原「模型与运行时」分组，Ollama
      // 运行时已隐藏）。
      key: 'runtime',
      label: t('systemConsole.menuGroups.runtime'),
      items: pick(['models', 'usage']),
    },
    {
      key: 'data_extensions',
      label: t('systemConsole.menuGroups.dataExtensions'),
      items: pick(['skill-library', 'mcp', 'websearch', 'sandbox-connections', 'vectorstore', 'parser', 'storage']),
    },
    {
      // 平台级系统设置（SSRF 白名单 / Docker 沙箱 / 注册模式等）：无空间
      // 管理员进不了 /platform 外壳，这里提供控制台直达。
      key: 'system_admin',
      label: t('systemConsole.menuGroups.systemAdmin'),
      items: pick(['system-settings', 'platform-storage']),
    },
  ].filter((group) => group.items.length > 0)
})

const currentSection = ref<ConsoleSection>('users')

// ?section= 深链：与 Settings.vue 的做法一致，切换菜单时同步 query，
// 刷新 / 分享 URL 后仍停留在对应面板。?tenant= 等代管参数通过展开
// route.query 一并保留。
// 沙箱配置面板并入「沙箱连接」后，旧深链 section=sandbox 改道过去；
// 技能代管面板迁回工作空间（000099）后，旧深链 section=skills 跨路由
// 改道到空间 Settings 的技能目录。
const LEGACY_SECTION_ALIASES: Partial<Record<string, ConsoleSection>> = {
  sandbox: 'sandbox-connections',
}

function normalizeSection(raw: unknown): ConsoleSection {
  if (typeof raw !== 'string') return 'users'
  if ((VALID_SECTIONS as string[]).includes(raw)) return raw as ConsoleSection
  return LEGACY_SECTION_ALIASES[raw] ?? 'users'
}

// 技能面板已离开控制台：旧深链改道到工作空间 Settings 的 skill-catalog。
// ?sandbox= 预选透传；?tenant= 有意丢弃——Settings 展示的是当前所选空间
// 的目录，代管租户参数在那边没有语义。
function redirectLegacySection(section: unknown): boolean {
  if (section !== 'skills') return false
  const query: Record<string, string> = { section: 'skill-catalog' }
  if (typeof route.query.sandbox === 'string') query.sandbox = route.query.sandbox
  void router.replace({ path: '/platform/settings', query })
  return true
}

function handleMenuClick(key: ConsoleSection) {
  currentSection.value = key
  if (route.query.section !== key) {
    // 控制台已无 sandbox 预选消费者（技能面板迁走），切面板时一并丢弃。
    const query: LocationQueryRaw = { ...route.query, section: key }
    delete query.sandbox
    void router.replace({ path: '/system/console', query })
  }
}

watch(
  () => route.query.section,
  (section) => {
    // 仅响应本路由上的 query 变化（路由守卫跳转其它页面时不干预）
    if (route.path !== '/system/console') return
    if (redirectLegacySection(section)) return
    const next = normalizeSection(section)
    if (next !== currentSection.value) {
      currentSection.value = next
    }
  },
)

onMounted(() => {
  if (redirectLegacySection(route.query.section)) return
  currentSection.value = normalizeSection(route.query.section)
  if (route.query.section !== currentSection.value) {
    void router.replace({ path: '/system/console', query: { ...route.query, section: currentSection.value } })
  }
})

// 离开控制台（含组件卸载兜底）：清除按空间代管状态，防止代管租户的
// X-Tenant-ID 泄漏到系统管理员自己的空间请求里。
onUnmounted(() => {
  managedStore.clearManaged()
})

function enterWorkspace() {
  router.push('/platform/knowledge-bases')
}

async function handleLogout() {
  try {
    await logoutApi()
  } finally {
    authStore.logout()
    await router.replace('/login')
  }
}
</script>

<style lang="less" scoped>
.system-console {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  // 工作区统一白色底：去掉品牌渐变与灰底（--td-bg-color-page）
  background: #fff;
}

.console-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 18px 32px;
  border-bottom: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
}

.console-brand {
  display: flex;
  align-items: center;
  gap: 14px;
}

.console-mark {
  width: 48px;
  height: 48px;
  display: grid;
  place-items: center;
  border-radius: 14px;
  color: var(--td-brand-color);
  background: var(--td-brand-color-light);
}

.console-brand-text {
  h1 {
    margin: 0;
    font-size: 19px;
    line-height: 1.35;
    color: var(--td-text-color-primary);
  }

  p {
    margin: 2px 0 0;
    font-size: 13px;
    color: var(--td-text-color-secondary);
  }
}

.console-header-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.console-me {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin-right: 6px;
  font-size: 13px;
  color: var(--td-text-color-secondary);
}

.console-body {
  flex: 1;
  display: flex;
  min-height: 0;
  align-items: stretch;
}

// 左侧菜单：按「空间与用户 / 模型 / 数据与扩展」分组平铺。
.console-menu {
  width: 208px;
  flex: none;
  padding: 16px 12px;
  border-right: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
  display: flex;
  flex-direction: column;
  gap: 4px;
  overflow-y: auto;
}

.console-menu-group {
  padding: 12px 12px 4px;
  font-size: 12px;
  line-height: 1.4;
  color: var(--td-text-color-placeholder);

  &:first-child {
    padding-top: 0;
  }
}

.console-menu-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border-radius: 8px;
  font-size: 14px;
  color: var(--td-text-color-primary);
  cursor: pointer;
  transition: background-color 0.15s ease, color 0.15s ease;

  .console-menu-icon {
    color: var(--td-text-color-secondary);
    transition: color 0.15s ease;
  }

  &:hover {
    background: var(--td-bg-color-secondarycontainer);

    .console-menu-icon {
      color: var(--td-brand-color);
    }
  }

  &.active {
    background: var(--td-brand-color-light);
    color: var(--td-brand-color);
    font-weight: 500;

    .console-menu-icon {
      color: var(--td-brand-color);
    }
  }
}

.console-content {
  flex: 1;
  min-width: 0;
  padding: 0 32px 32px;
  overflow-y: auto;
}

.managed-empty {
  padding: 48px 0;
}

@media (max-width: 720px) {
  .console-header {
    padding-left: 16px;
    padding-right: 16px;
  }

  .console-body {
    flex-direction: column;
  }

  .console-menu {
    width: 100%;
    flex-direction: row;
    flex-wrap: wrap;
    border-right: none;
    border-bottom: 1px solid var(--td-component-stroke);
  }

  .console-menu-group {
    width: 100%;
  }

  .console-content {
    padding: 0 16px 16px;
  }
}
</style>
