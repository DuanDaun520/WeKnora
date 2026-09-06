<template>
  <!-- 分类管理（000105）：分类是独立登记表，先创建再使用 —— 管理员在此新建，
             注册/编辑技能的分类下拉只能选已有分类（不再即输即建）。重命名 = 改
       登记名 + 整组技能换名；删除 = 删登记 + 名下技能置为未分类；两者都点亮
       受影响技能的待推送标记，需逐个「推送更新」后进入空间。 -->
  <t-dialog
    :visible="visible"
    :header="t('skillLibrary.categoryManagerTitle')"
    :confirm-btn="{ content: t('common.close'), theme: 'default' }"
    :cancel-btn="null"
    width="520px"
    attach="body"
    @confirm="close"
    @close="close"
    @opened="load"
  >
    <p class="category-hint">{{ t('skillLibrary.categoryManagerHint') }}</p>

    <div class="category-create">
      <t-input
        v-model="newName"
        :placeholder="t('skillLibrary.categoryManagerNewPlaceholder')"
        :maxlength="255"
        :disabled="creating"
        @enter="commitCreate"
      />
      <t-button
        theme="primary"
        :loading="creating"
        :disabled="!newName.trim() || creating"
        @click="commitCreate"
      >
        {{ t('skillLibrary.categoryManagerCreate') }}
      </t-button>
    </div>

    <t-loading v-if="loading" class="category-loading" :text="t('common.loading')" />

    <div v-else-if="!categories.length" class="category-empty">
      {{ t('skillLibrary.categoryManagerEmpty') }}
    </div>

    <div v-else class="category-rows">
      <div v-for="row in categories" :key="row.name" class="category-row">
        <template v-if="renaming === row.name">
          <t-input
            v-model="renameValue"
            class="category-row__input"
            :placeholder="t('skillLibrary.categoryManagerRenameTo', { name: row.name })"
            :maxlength="255"
            @enter="commitRename(row.name)"
          />
          <t-button
            size="small"
            theme="primary"
            variant="outline"
            :disabled="!renameValue.trim() || renameValue.trim() === row.name"
            :loading="applying"
            @click="commitRename(row.name)"
          >
            {{ t('skillLibrary.categoryManagerRenameSave') }}
          </t-button>
          <t-button size="small" variant="text" :disabled="applying" @click="cancelRename">
            {{ t('skillLibrary.categoryManagerRenameCancel') }}
          </t-button>
        </template>
        <template v-else>
          <div class="category-row__main">
            <span class="category-row__name" :title="row.name">{{ row.name }}</span>
            <span class="category-row__count">
              {{ t('skillLibrary.categoryManagerCount', { count: row.count }) }}
            </span>
          </div>
          <div class="category-row__actions">
            <t-tooltip :content="t('common.edit')">
              <t-button
                variant="text"
                shape="square"
                size="small"
                :disabled="applying"
                @click="startRename(row.name)"
              >
                <t-icon name="edit-1" />
              </t-button>
            </t-tooltip>
            <t-tooltip :content="t('skillLibrary.categoryManagerRemove')">
              <t-button
                variant="text"
                shape="square"
                size="small"
                class="category-row__remove"
                :disabled="applying"
                @click="confirmRemove(row)"
              >
                <t-icon name="delete" />
              </t-button>
            </t-tooltip>
          </div>
        </template>
      </div>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { useConfirmDelete } from '@/components/settings/useConfirmDelete'
import {
  createSkillCategory,
  listSkillCategories,
  removeSkillCategory,
  renameSkillCategory,
  type PlatformSkillCategoryCount,
} from '@/api/skill-library'

defineProps<{ visible: boolean }>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  /** Fired after a create/rename/remove landed — the panel reloads its cards. */
  changed: []
}>()

const { t } = useI18n()
const confirmDelete = useConfirmDelete()

const categories = ref<PlatformSkillCategoryCount[]>([])
const loading = ref(false)
const applying = ref(false)
const renaming = ref('')
const renameValue = ref('')
const newName = ref('')
const creating = ref(false)

const load = async () => {
  loading.value = true
  try {
    categories.value = await listSkillCategories()
  } catch (error: any) {
    MessagePlugin.error(error?.message || t('skillLibrary.toasts.categoryOpFailed'))
  } finally {
    loading.value = false
  }
}

// Create-first (000105): a category is minted HERE, before any skill can select
// it — the register/edit drawer only lists what this dialog has created.
async function commitCreate() {
  const name = newName.value.trim()
  if (!name || creating.value) return
  creating.value = true
  try {
    await createSkillCategory(name)
    MessagePlugin.success(t('skillLibrary.toasts.categoryCreated'))
    newName.value = ''
    await load()
    emit('changed')
  } catch (error: any) {
    MessagePlugin.error(error?.message || t('skillLibrary.categoryManagerCreateFailed'))
  } finally {
    creating.value = false
  }
}

const close = () => {
  cancelRename()
  emit('update:visible', false)
}

const startRename = (name: string) => {
  renaming.value = name
  renameValue.value = name
}

const cancelRename = () => {
  renaming.value = ''
  renameValue.value = ''
}

async function commitRename(from: string) {
  const to = renameValue.value.trim()
  if (!to || to === from || applying.value) return
  applying.value = true
  try {
    const moved = await renameSkillCategory(from, to)
    MessagePlugin.success(t('skillLibrary.toasts.categoryRenamed', { count: moved }))
    cancelRename()
    await load()
    emit('changed')
  } catch (error: any) {
    MessagePlugin.error(error?.message || t('skillLibrary.toasts.categoryOpFailed'))
  } finally {
    applying.value = false
  }
}

function confirmRemove(row: PlatformSkillCategoryCount) {
  confirmDelete({
    body: t('skillLibrary.categoryManagerRemoveConfirmBody', {
      name: row.name,
      count: row.count,
    }),
    onConfirm: async () => {
      try {
        const cleared = await removeSkillCategory(row.name)
        MessagePlugin.success(t('skillLibrary.toasts.categoryRemoved', { count: cleared }))
        await load()
        emit('changed')
      } catch (error: any) {
        MessagePlugin.error(error?.message || t('skillLibrary.toasts.categoryOpFailed'))
      }
    },
  })
}
</script>

<style scoped lang="less">
.category-hint {
  margin: 0 0 12px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--td-text-color-placeholder);
}

.category-create {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;

  .t-input {
    flex: 1;
    min-width: 0;
  }

  .t-button {
    flex-shrink: 0;
  }
}

.category-loading {
  display: block;
  padding: 32px 0;
}

.category-empty {
  padding: 24px 0;
  text-align: center;
  font-size: 13px;
  color: var(--td-text-color-placeholder);
}

.category-rows {
  display: flex;
  flex-direction: column;
  max-height: 360px;
  overflow-y: auto;
}

.category-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 8px 0;

  & + .category-row {
    border-top: 1px solid var(--td-component-stroke);
  }
}

.category-row__main {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 8px;
}

.category-row__name {
  min-width: 0;
  font-size: 13px;
  color: var(--td-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.category-row__count {
  flex-shrink: 0;
  font-size: 11px;
  color: var(--td-text-color-placeholder);
}

.category-row__input {
  flex: 1;
  min-width: 0;
}

.category-row__actions {
  flex-shrink: 0;
  display: flex;
  align-items: center;

  // 删除按钮与编辑按钮同权重（scoped 样式可作用于子组件根节点）
  .category-row__remove {
    color: var(--td-text-color-placeholder);
  }
}
</style>
