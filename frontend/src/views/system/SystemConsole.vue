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

    <!-- 左侧菜单 + 内容区：后续系统管理功能以菜单项形式陆续加入 -->
    <div class="console-body">
      <nav class="console-menu" :aria-label="$t('systemConsole.title')">
        <div
          v-for="item in menuItems"
          :key="item.key"
          :class="['console-menu-item', { active: currentSection === item.key }]"
          :aria-current="currentSection === item.key ? 'page' : undefined"
          @click="handleMenuClick(item.key)"
        >
          <t-icon :name="item.icon" size="16px" class="console-menu-icon" />
          <span>{{ item.label }}</span>
        </div>
      </nav>

      <section class="console-content">
        <UsersPanel v-if="currentSection === 'users'" />
        <WorkspacesPanel v-else-if="currentSection === 'tenants'" />
        <ModelsPanel v-else-if="currentSection === 'models'" />
        <OllamaPanel v-else-if="currentSection === 'ollama'" />
      </section>
    </div>
  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { logout as logoutApi } from '@/api/auth'
import UsersPanel from './UsersPanel.vue'
import WorkspacesPanel from './WorkspacesPanel.vue'
import ModelsPanel from './ModelsPanel.vue'
import OllamaPanel from './OllamaPanel.vue'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()

type ConsoleSection = 'users' | 'tenants' | 'models' | 'ollama'

const menuItems = computed(() => [
  { key: 'users' as const, icon: 'user', label: t('systemConsole.menu.users') },
  { key: 'tenants' as const, icon: 'system-sum', label: t('systemConsole.menu.tenants') },
  { key: 'models' as const, icon: 'ai', label: t('systemConsole.menu.models') },
  { key: 'ollama' as const, icon: 'cloud', label: t('systemConsole.menu.ollama') },
])

const VALID_SECTIONS: ConsoleSection[] = ['users', 'tenants', 'models', 'ollama']

const currentSection = ref<ConsoleSection>('users')

// ?section= 深链：与 Settings.vue 的做法一致，切换菜单时同步 query，
// 刷新 / 分享 URL 后仍停留在对应面板。
function normalizeSection(raw: unknown): ConsoleSection {
  return typeof raw === 'string' && (VALID_SECTIONS as string[]).includes(raw)
    ? (raw as ConsoleSection)
    : 'users'
}

function handleMenuClick(key: ConsoleSection) {
  currentSection.value = key
  if (route.query.section !== key) {
    void router.replace({ path: '/system/console', query: { section: key } })
  }
}

watch(
  () => route.query.section,
  (section) => {
    // 仅响应本路由上的 query 变化（路由守卫跳转其它页面时不干预）
    if (route.path !== '/system/console') return
    const next = normalizeSection(section)
    if (next !== currentSection.value) {
      currentSection.value = next
    }
  },
)

onMounted(() => {
  currentSection.value = normalizeSection(route.query.section)
  if (route.query.section !== currentSection.value) {
    void router.replace({ path: '/system/console', query: { section: currentSection.value } })
  }
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

// 左侧菜单：后续系统管理功能陆续加入，这里只做单级平铺。
.console-menu {
  width: 208px;
  flex: none;
  padding: 16px 12px;
  border-right: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
  display: flex;
  flex-direction: column;
  gap: 4px;
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

  .console-content {
    padding: 0 16px 16px;
  }
}
</style>
