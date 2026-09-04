<template>
  <main class="workspace-onboarding">
    <section class="workspace-card">
      <div class="workspace-mark" aria-hidden="true">
        <t-icon name="system-sum" size="30px" />
      </div>
      <h1>{{ $t('auth.workspaceOnboarding.enterpriseTitle') }}</h1>
      <p class="workspace-description">
        {{ $t('auth.workspaceOnboarding.enterpriseDescription') }}
      </p>

      <div class="workspace-actions workspace-actions--single">
        <t-button theme="primary" size="large" @click="handleLogout">
          <template #icon><t-icon name="logout" /></template>
          {{ $t('auth.logout') }}
        </t-button>
      </div>

      <p class="workspace-help">
        {{ $t('auth.workspaceOnboarding.enterpriseHelp') }}
      </p>
    </section>
  </main>
</template>

<script setup lang="ts">
import { onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { logout as logoutApi } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const authStore = useAuthStore()

onMounted(async () => {
  await authStore.refreshFromAuthMe()
  // 系统管理员不该看到"等待分配"——控制台里可以自己绑空间/建空间。
  // 正常情况下路由守卫已把他们送到 /system/console，这里兜底处理
  // 直链/刷新落在本页的管理员会话。
  if (authStore.isSystemAdmin) {
    router.replace('/system/console')
  }
})

watch(
  () => authStore.hasValidTenant,
  (ready) => {
    if (ready) router.replace('/platform/knowledge-bases')
  },
)

async function handleLogout() {
  await logoutApi()
  authStore.logout()
  await router.replace('/login')
}
</script>

<style scoped lang="less">
.workspace-onboarding {
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: 32px 20px;
  background:
    radial-gradient(circle at 20% 10%, color-mix(in srgb, var(--td-brand-color) 12%, transparent), transparent 38%),
    var(--td-bg-color-page);
}

.workspace-card {
  width: min(520px, 100%);
  padding: 44px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 20px;
  background: var(--td-bg-color-container);
  box-shadow: var(--td-shadow-2);
  text-align: center;
}

.workspace-mark {
  width: 64px;
  height: 64px;
  margin: 0 auto 22px;
  display: grid;
  place-items: center;
  border-radius: 18px;
  color: var(--td-brand-color);
  background: var(--td-brand-color-light);
}

h1 {
  margin: 0;
  color: var(--td-text-color-primary);
  font-size: 26px;
  line-height: 1.3;
}

.workspace-description,
.workspace-help {
  color: var(--td-text-color-secondary);
  line-height: 1.7;
}

.workspace-description {
  margin: 14px 0 28px;
}

.workspace-actions {
  display: grid;
  grid-template-columns: minmax(220px, 1fr);
  gap: 12px;
}

.workspace-actions--single {
  grid-template-columns: minmax(220px, 1fr);
}

.workspace-help {
  margin: 24px 0 0;
  font-size: 13px;
}

@media (max-width: 560px) {
  .workspace-card {
    padding: 32px 22px;
  }

  .workspace-actions {
    grid-template-columns: 1fr;
  }
}
</style>