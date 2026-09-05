<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';

interface KnowledgeItem {
  id: string;
  file_name?: string;
  title?: string;
  type?: string;
  parse_status?: string;
}

const props = withDefaults(defineProps<{
  item: KnowledgeItem;
  canDownload: boolean;
  canMutateKnowledge: boolean;
  /**
   * 000099 共同维护：该条目是否允许当前用户修改/删除。
   * KB 管理者恒为 true；成员仅在开放维护的 KB 中对自己创建的条目为 true。
   * 缺省 true 以兼容未传该 prop 的调用方。
   */
  canMutateItem?: boolean;
  traceVisible: boolean;
  /** Whether the knowledge base has a folder structure to file documents into. */
  foldersAvailable?: boolean;
}>(), {
  canMutateItem: true,
});

const emit = defineEmits<{
  (e: 'download'): void;
  (e: 'edit'): void;
  (e: 'view-trace'): void;
  (e: 'reparse'): void;
  (e: 'cancel-parse'): void;
  (e: 'move'): void;
  (e: 'move-folder'): void;
  (e: 'batch-manage'): void;
  (e: 'delete'): void;
}>();

const { t } = useI18n();

const CANCELABLE_PARSE_STATUSES = new Set(['pending', 'processing', 'finalizing']);

const isParseInFlight = computed(() =>
  CANCELABLE_PARSE_STATUSES.has(String(props.item.parse_status ?? ''))
);

const fileName = computed(() => props.item.file_name || props.item.title || props.item.id);
</script>

<template>
  <!-- 下载原始文档 -->
  <div
    v-if="canDownload && (item.type === 'file' || item.type === 'manual')"
    class="doc-action-menu-item"
    @click.stop="emit('download')"
  >
    <t-icon class="icon" name="download" />
    <span>{{ $t('common.download') }}</span>
  </div>

  <!-- 编辑文档（000099：仅对该条目有修改权的用户可见） -->
  <div v-if="canMutateItem && item.type === 'manual'" class="doc-action-menu-item" @click.stop="emit('edit')">
    <t-icon class="icon" name="edit" />
    <span>{{ $t('knowledgeBase.editDocument') }}</span>
  </div>

  <!-- 查看处理过程 -->
  <div v-if="traceVisible" class="doc-action-menu-item" @click.stop="emit('view-trace')">
    <t-icon class="icon" name="chart-bar" />
    <span>{{ $t('knowledgeStages.viewTrace') }}</span>
  </div>

  <!-- 重建知识 (in-flight: no popconfirm, just emits)（000099：需条目修改权） -->
  <div v-if="canMutateItem && isParseInFlight" class="doc-action-menu-item" @click.stop="emit('reparse')">
    <t-icon class="icon" name="refresh" />
    <span>{{ $t('knowledgeBase.rebuildDocument') }}</span>
  </div>

  <!-- 重建知识 (normal: with popconfirm) -->
  <t-popconfirm v-else-if="canMutateItem" theme="warning"
    :content="$t('knowledgeBase.rebuildConfirm', { fileName })"
    :confirm-btn="{ content: $t('common.confirm'), theme: 'primary' }"
    :cancel-btn="{ content: $t('common.cancel') }" placement="left"
    @confirm="emit('reparse')">
    <div class="doc-action-menu-item" @click.stop>
      <t-icon class="icon" name="refresh" />
      <span>{{ $t('knowledgeBase.rebuildDocument') }}</span>
    </div>
  </t-popconfirm>

  <!-- 取消解析（000099：需条目修改权） -->
  <t-popconfirm v-if="canMutateItem && isParseInFlight" theme="warning"
    :content="$t('knowledgeBase.cancelParseConfirmBody', { title: fileName })"
    :confirm-btn="{ content: $t('knowledgeBase.cancelParse'), theme: 'danger' }"
    :cancel-btn="{ content: $t('common.cancel') }" placement="left"
    @confirm="emit('cancel-parse')">
    <div class="doc-action-menu-item danger" @click.stop>
      <t-icon class="icon" name="close-circle" />
      <span>{{ $t('knowledgeBase.cancelParse') }}</span>
    </div>
  </t-popconfirm>

  <!-- 移动到目录 -->
  <div v-if="canMutateKnowledge" class="doc-action-menu-item" @click.stop="emit('move-folder')">
    <t-icon class="icon" name="folder" />
    <span>{{ $t('knowledgeBase.moveToFolder.action') }}</span>
  </div>

  <!-- 移动到其他知识库 -->
  <div v-if="canMutateKnowledge" class="doc-action-menu-item" @click.stop="emit('move')">
    <t-icon class="icon" name="swap" />
    <span>{{ $t('knowledgeBase.moveDocument') }}</span>
  </div>

  <!-- 批量管理 -->
  <div v-if="canMutateKnowledge" class="doc-action-menu-item" @click.stop="emit('batch-manage')">
    <t-icon class="icon" name="queue" />
    <span>{{ $t('menu.batchManage') }}</span>
  </div>

  <!-- 删除文档（000099：需条目修改权） -->
  <t-popconfirm v-if="canMutateItem" theme="warning"
    :content="$t('knowledgeBase.confirmDeleteDocument', { fileName })"
    :confirm-btn="{ content: $t('knowledgeBase.confirmDelete'), theme: 'danger' }"
    :cancel-btn="{ content: $t('common.cancel') }" placement="left"
    @confirm="emit('delete')">
    <div class="doc-action-menu-item danger" @click.stop>
      <t-icon class="icon" name="delete" />
      <span>{{ $t('knowledgeBase.deleteDocument') }}</span>
    </div>
  </t-popconfirm>
</template>

<style scoped lang="less">
.doc-action-menu-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  font-size: 14px;
  line-height: 20px;
  color: var(--td-text-color-primary);
  cursor: pointer;
  border-radius: 6px;
  transition: background-color 0.15s cubic-bezier(0.2, 0, 0, 1), transform 0.12s ease;

  &:hover {
    background: var(--td-bg-color-container-hover);
  }

  &:active {
    background: var(--td-bg-color-container-active);
    transform: scale(0.98);
  }

  .icon {
    font-size: 16px;
    color: var(--td-text-color-secondary);
    transition: color 0.15s ease;
  }

  &:hover .icon {
    color: var(--td-text-color-primary);
  }

  &.danger {
    color: var(--td-error-color-6);
    margin-top: 4px;
    position: relative;

    &::before {
      content: '';
      position: absolute;
      top: -3px;
      left: 8px;
      right: 8px;
      height: 1px;
      background: var(--td-component-stroke);
    }

    .icon {
      color: var(--td-error-color-6);
    }

    &:hover {
      background: var(--td-error-color-1);
      color: var(--td-error-color-6);

      .icon {
        color: var(--td-error-color-6);
      }
    }

    &:active {
      background: var(--td-error-color-2);
    }
  }
}
</style>
