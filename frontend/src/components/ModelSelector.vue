<template>
  <div class="model-selector">
    <t-select
      :value="selectedModelId"
      @change="handleModelChange"
      :placeholder="placeholderText"
      :disabled="disabled"
      :loading="loading"
      :status="status"
      :clearable="clearable"
      filterable
      style="width: 100%;"
    >
      <template v-if="selectedModel && showContextWindow" #valueDisplay>
        <span class="selected-model">
          <span class="selected-model__name">{{ modelDisplayName(selectedModel) }}</span>
          <span
            class="model-ctx"
            :class="{ 'model-ctx--default': isDefaultContextWindow(selectedModel.parameters?.context_window) }"
            :title="contextWindowTitle(selectedModel.parameters?.context_window)"
          >{{ formatContextWindow(selectedModel.parameters?.context_window) }}</span>
        </span>
      </template>
      <!-- 已有的模型选项 -->
      <t-option
        v-for="model in optionModels"
        :key="model.id"
        :value="model.id"
        :label="modelDisplayName(model)"
      >
        <div class="model-option">
          <t-icon name="check-circle-filled" class="model-icon" />
          <span class="model-name">{{ modelDisplayName(model) }}</span>
          <span v-if="model.display_name" class="model-raw-name">{{ model.name }}</span>
          <t-tag v-if="model.is_builtin" size="small" theme="primary">{{ $t('model.builtinTag') }}</t-tag>
          <t-tag v-if="model.is_default" size="small" theme="success">{{ $t('model.defaultTag') }}</t-tag>
          <span
            v-if="showContextWindow"
            class="model-ctx"
            :class="{ 'model-ctx--default': isDefaultContextWindow(model.parameters?.context_window) }"
            :title="contextWindowTitle(model.parameters?.context_window)"
          >{{ formatContextWindow(model.parameters?.context_window) }}</span>
        </div>
      </t-option>
      
      <!-- 添加模型选项（在底部）：模型配置已收归系统管理员（000094），
           非系统管理员看不到入口，走空间管理员向平台申请的流程。 -->
      <t-option
        v-if="!disabled && authStore.isSystemAdmin"
        value="__add_model__"
        class="add-model-option"
      >
        <div class="model-option add">
          <t-icon name="add" class="add-icon" />
          <span class="model-name">{{ $t('model.addModelInSettings') }}</span>
        </div>
      </t-option>
    </t-select>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { type ModelConfig } from '@/api/model'
import { useChatResourcesStore } from '@/stores/chatResources'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { filterModelsByType } from './modelSelectorFilter'
import {
  formatContextWindow,
  isDefaultContextWindow,
  effectiveContextWindow,
  modelHasContextWindow,
} from '@/utils/contextWindow'

interface Props {
  modelType: 'KnowledgeQA' | 'Embedding' | 'Rerank' | 'VLLM' | 'ASR'
  selectedModelId?: string
  disabled?: boolean
  placeholder?: string
  status?: 'default' | 'success' | 'warning' | 'error'
  clearable?: boolean
  // 可选：外部传入的所有模型列表，如果提供则不调用API
  allModels?: ModelConfig[]
}

const props = withDefaults(defineProps<Props>(), {
  disabled: false,
  placeholder: '',
  status: 'default',
  clearable: false,
})

const emit = defineEmits<{
  'update:selectedModelId': [value: string]
  'add-model': []
}>()

const models = ref<ModelConfig[]>([])
// 未过滤的模型源（allModels 或自行拉取的全量）——选中的模型被类型过滤
// 剔除时，从这里把名字补回选项（000102）。
const rawModels = ref<ModelConfig[]>([])
const loading = ref(false)
const { t } = useI18n()
const authStore = useAuthStore()
const chatResources = useChatResourcesStore()

const placeholderText = computed(() => {
  return props.placeholder || t('model.selectModelPlaceholder')
})

// 000102：选中模型不在过滤后的选项里时（类型过滤剔除了它 / 列表异步
// 未就绪后又到位），t-select 会把 UUID 当文案显示——普通用户查看智能体
// 看到的就是一串代号。从原始列表补回选中项；原始列表已到齐仍找不到的
// （模型已删除），落占位文案而不是裸 id。
const optionModels = computed<ModelConfig[]>(() => {
  const id = props.selectedModelId
  if (!id || models.value.some(m => m.id === id)) return models.value
  const found = rawModels.value.find(m => m.id === id)
  if (found) return [...models.value, found]
  if (rawModels.value.length > 0) {
    return [...models.value, { id, name: t('model.unknownModel') } as ModelConfig]
  }
  return models.value
})

const modelDisplayName = (model: ModelConfig) => {
  const displayName = model.display_name?.trim()
  return displayName || model.name
}

const showContextWindow = computed(() => modelHasContextWindow(props.modelType))

const contextWindowTitle = (tokens?: number) => {
  if (isDefaultContextWindow(tokens)) {
    return t('model.editor.contextWindowDefaultHint', { value: formatContextWindow(tokens) })
  }
  return t('model.editor.contextWindowTokens', { count: effectiveContextWindow(tokens) })
}

// 外部传入 allModels 时跟着 prop 走；否则用空间级缓存，设置页改完窗口立刻能看见。
// 000102：同步维护 rawModels（未过滤全量源），选中项被类型过滤剔除时从这补回。
watch(
  () => [props.allModels, props.modelType, chatResources.allModels] as const,
  ([newModels]) => {
    const source = Array.isArray(newModels) ? newModels : chatResources.allModels
    rawModels.value = source
    models.value = filterModelsByType(source, props.modelType)
  },
  { immediate: true },
)

const selectedModel = computed(() => {
  if (!props.selectedModelId) return null
  return models.value.find(m => m.id === props.selectedModelId)
})

// 加载模型列表（仅在未提供 allModels 时调用）
const loadModels = async () => {
  if (props.allModels) {
    return
  }

  loading.value = true
  try {
    await chatResources.ensureModels()
    rawModels.value = chatResources.allModels
    models.value = filterModelsByType(chatResources.allModels, props.modelType)
  } catch (error) {
    console.error(t('model.loadFailed'), error)
    MessagePlugin.error(t('model.loadFailed'))
    models.value = []
  } finally {
    loading.value = false
  }
}

// 处理模型选择变化
const handleModelChange = (value?: string) => {
  // 如果选择的是添加模型选项，触发添加事件而不更新选中值
  if (value === '__add_model__') {
    emit('add-model')
    return
  }
  emit('update:selectedModelId', value || '')
}

// 暴露刷新方法给父组件
defineExpose({
  refresh: loadModels
})

onMounted(() => {
  // 只有在没有提供 allModels 时才加载
  if (!props.allModels) {
    loadModels()
  }
})
</script>

<style lang="less" scoped>
.model-selector {
  width: 100%;
}

.model-option {
  display: flex;
  align-items: center;
  gap: 8px;
  
  .model-icon {
    font-size: 14px;
    color: var(--td-brand-color);
  }
  
  .add-icon {
    font-size: 14px;
    color: var(--td-brand-color);
  }
  
  .model-name {
    flex: 0 1 auto;
    min-width: 0;
    font-size: 13px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .model-raw-name {
    flex: 1;
    min-width: 0;
    font-size: 12px;
    color: var(--td-text-color-placeholder);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .model-ctx {
    margin-left: auto;
  }
  
  &.add {
    .model-name {
      color: var(--td-brand-color);
      font-weight: 500;
    }
  }
}

.model-ctx {
  flex-shrink: 0;
  font-size: 11px;
  font-variant-numeric: tabular-nums;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-secondarycontainer);
  padding: 0 6px;
  border-radius: 4px;
  line-height: 18px;

  &--default {
    color: var(--td-text-color-placeholder);
  }
}

.selected-model {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  max-width: 100%;

  &__name {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}
</style>
