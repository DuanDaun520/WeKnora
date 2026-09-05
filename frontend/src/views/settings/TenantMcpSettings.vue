<template>
  <div class="tenant-mcp-settings">
    <div class="section-header">
      <h2>{{ t('tenantMcpSettings.title') }}</h2>
      <p class="section-description">{{ t('tenantMcpSettings.description') }}</p>
    </div>

    <div v-if="loading" class="state-hint">{{ t('tenantMcpSettings.loading') }}</div>

    <div v-else-if="loadError" class="state-hint state-hint--error">
      {{ t('tenantMcpSettings.loadFailed') }}
      <a class="retry-link" role="button" tabindex="0" @click="load"
        @keydown.enter.prevent="load">{{ t('tenantMcpSettings.retry') }}</a>
    </div>

    <div v-else-if="services.length === 0" class="state-hint">{{ t('tenantMcpSettings.empty') }}</div>

    <div v-else class="mcp-list">
      <div v-for="svc in services" :key="svc.id" class="mcp-card" :class="{ 'is-disabled': !svc.enabled }">
        <div class="mcp-card__head">
          <span class="mcp-status-dot" :class="svc.enabled ? 'is-on' : 'is-off'"></span>
          <span class="mcp-name" :title="svc.name">{{ svc.name }}</span>
          <span v-if="svc.is_builtin" class="mcp-tag">{{ t('tenantMcpSettings.builtin') }}</span>
          <span class="mcp-tag mcp-tag--transport">{{ transportLabel(svc.transport_type) }}</span>
        </div>
        <p v-if="svc.description" class="mcp-desc">{{ svc.description }}</p>
      </div>
    </div>

    <p class="readonly-hint">{{ t('tenantMcpSettings.readonlyHint') }}</p>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { listMCPServices, type MCPService } from '@/api/mcp-service'

const { t } = useI18n()

// 空间侧只读视图：GET /api/v1/mcp-services（Viewer+）返回对当前空间生效
// 的 MCP 服务 —— 平台目录里分配给本空间的服务 + 内置服务。配置 / 分配 /
// 启停动作都在系统管理控制台（000096 平台化），这里不提供任何写入口。
const services = ref<MCPService[]>([])
const loading = ref(true)
const loadError = ref(false)

const transportLabels: Record<string, string> = {
  sse: 'SSE',
  'http-streamable': 'HTTP Streamable',
  stdio: 'STDIO',
}

const transportLabel = (transport?: string) =>
  (transport && transportLabels[transport]) || (transport ?? '-')

const load = async () => {
  loading.value = true
  loadError.value = false
  try {
    services.value = await listMCPServices()
  } catch (e) {
    console.error('Failed to load tenant MCP services:', e)
    loadError.value = true
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<style lang="less" scoped>
.tenant-mcp-settings {
  display: flex;
  flex-direction: column;
}

.section-header {
  margin-bottom: 18px;

  h2 {
    margin: 0 0 6px;
    color: var(--td-text-color-primary);
    font-size: 18px;
    font-weight: 600;
    line-height: 1.35;
  }
}

.section-description {
  margin: 0;
  color: var(--td-text-color-secondary);
  font-size: 13px;
  line-height: 1.6;
}

.state-hint {
  padding: 24px 12px;
  text-align: center;
  font-size: 13px;
  color: var(--td-text-color-placeholder);

  &--error {
    color: var(--td-error-color);
  }

  .retry-link {
    margin-left: 8px;
    color: var(--td-brand-color);
    cursor: pointer;

    &:hover {
      text-decoration: underline;
    }
  }
}

.mcp-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.mcp-card {
  padding: 12px 14px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  background: var(--td-bg-color-container);

  &.is-disabled {
    opacity: 0.6;
  }

  .mcp-card__head {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }

  .mcp-status-dot {
    flex-shrink: 0;
    width: 8px;
    height: 8px;
    border-radius: 50%;

    &.is-on {
      background: var(--td-success-color);
    }

    &.is-off {
      background: var(--td-text-color-placeholder);
    }
  }

  .mcp-name {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 14px;
    font-weight: 500;
    color: var(--td-text-color-primary);
  }

  .mcp-tag {
    flex-shrink: 0;
    padding: 1px 6px;
    border-radius: 4px;
    font-size: 11px;
    line-height: 1.4;
    background: var(--td-bg-color-secondarycontainer);
    color: var(--td-text-color-secondary);
  }

  .mcp-tag--transport {
    background: transparent;
    border: 1px solid var(--td-component-stroke);
  }

  .mcp-desc {
    margin: 6px 0 0 16px;
    font-size: 12px;
    line-height: 1.5;
    color: var(--td-text-color-secondary);
  }
}

.readonly-hint {
  margin: 16px 0 0;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}
</style>
