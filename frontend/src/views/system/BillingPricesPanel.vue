<template>
  <div class="billing-prices-panel">
    <div class="section-header billing-header">
      <div>
        <h2>{{ t('usageStats.prices.title') }}</h2>
        <p class="section-description">{{ t('usageStats.prices.desc') }}</p>
      </div>
      <t-button theme="primary" @click="openCreate">
        <template #icon><t-icon name="add" /></template>
        {{ t('usageStats.prices.add') }}
      </t-button>
    </div>

    <div v-if="error" class="billing-branch">
      <t-alert theme="error" :message="error">
        <template #operation>
          <t-button size="small" @click="reload">{{ t('usageStats.filters.refresh') }}</t-button>
        </template>
      </t-alert>
    </div>
    <div v-else-if="!loading && prices.length === 0" class="billing-branch">
      <t-empty :description="t('usageStats.prices.empty')" />
    </div>
    <div v-else class="data-table-shell billing-table-shell">
      <t-table
        row-key="model_id"
        :data="prices"
        :columns="columns"
        :loading="loading"
        size="medium"
        hover
      >
        <template #enabled="{ row }">
          <t-tag size="small" variant="light" :theme="row.enabled ? 'success' : 'default'">
            {{ row.enabled ? t('usageStats.prices.enabled') : t('usageStats.prices.disabled') }}
          </t-tag>
        </template>
        <template #effective_from="{ row }">{{ formatDate(row.effective_from) }}</template>
        <template #actions="{ row }">
          <div class="billing-row-actions">
            <t-button variant="text" size="small" @click="openEdit(row)">
              {{ t('common.edit') }}
            </t-button>
            <t-popconfirm
              :content="t('usageStats.prices.deleteConfirm')"
              @confirm="handleDelete(row)"
            >
              <t-button variant="text" size="small" theme="danger">
                {{ t('common.delete') }}
              </t-button>
            </t-popconfirm>
          </div>
        </template>
      </t-table>
    </div>

    <!-- 新增 / 编辑单价 -->
    <t-dialog
      v-model:visible="dialogVisible"
      :header="editing ? t('usageStats.prices.editTitle', { model: form.model_id }) : t('usageStats.prices.createTitle')"
      :confirm-btn="{ content: t('common.save'), loading: saving }"
      :cancel-btn="t('common.cancel')"
      width="620px"
      :close-on-overlay-click="false"
      @confirm="handleSave"
      @close="dialogVisible = false"
    >
      <div class="billing-form">
        <div class="billing-form-field billing-form-field--wide">
          <span class="billing-form-label">{{ t('usageStats.prices.model') }}</span>
          <t-select
            v-if="!editing"
            v-model="form.model_id"
            :placeholder="t('usageStats.prices.modelPlaceholder')"
            filterable
            creatable
            clearable
          >
            <t-option v-for="m in modelOptions" :key="m.value" :value="m.value" :label="m.label" />
          </t-select>
          <t-input v-else v-model="form.model_id" disabled />
        </div>

        <p class="billing-form-hint">{{ t('usageStats.prices.perMHint') }}</p>

        <div class="billing-form-grid">
          <div v-for="f in PRICE_FIELDS" :key="f.key" class="billing-form-field">
            <span class="billing-form-label">{{ t('usageStats.prices.field.' + f.key) }}</span>
            <t-input-number
              v-model="form[f.key]"
              :min="0"
              :step="0.5"
              :decimal-places="6"
              :placeholder="t('usageStats.prices.fieldHint.' + f.key)"
              theme="column"
            />
          </div>
        </div>

        <div class="billing-form-row">
          <div class="billing-form-field">
            <span class="billing-form-label">{{ t('usageStats.prices.currency') }}</span>
            <t-select v-model="form.currency">
              <t-option value="CNY" label="CNY ¥" />
              <t-option value="USD" label="USD $" />
            </t-select>
          </div>
          <div class="billing-form-field billing-form-field--switch">
            <span class="billing-form-label">{{ t('usageStats.prices.enabled') }}</span>
            <t-switch v-model="form.enabled" />
          </div>
        </div>
      </div>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
// 模型单价配置（docs/Token统计与计费设计.md §5.3）：model_prices 的
// 管理后台 CRUD。计价口径为「当前单价 × 全部历史用量」——修改单价立即
// 反映到所有报表的估算金额，effective_from 仅作记录。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  deleteModelPrice,
  listModelPrices,
  upsertModelPrice,
  type ModelPrice,
} from '@/api/usage'
import { listSystemModels, type ModelConfig } from '@/api/model'

const PRICE_FIELDS = [
  { key: 'price_input_per_m' },
  { key: 'price_output_per_m' },
  { key: 'price_cached_per_m' },
  { key: 'price_per_m' },
  { key: 'price_per_image' },
  { key: 'price_per_audio_min' },
] as const

type PriceFieldKey = (typeof PRICE_FIELDS)[number]['key']

const { t } = useI18n()
const prices = ref<ModelPrice[]>([])
const models = ref<ModelConfig[]>([])
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const dialogVisible = ref(false)
const editing = ref(false)

const defaultForm = (): ModelPrice => ({
  model_id: '',
  price_input_per_m: 0,
  price_output_per_m: 0,
  price_cached_per_m: 0,
  price_per_m: 0,
  price_per_image: 0,
  price_per_audio_min: 0,
  price_per_video_min: 0,
  currency: 'CNY',
  enabled: true,
  effective_from: '',
})
const form = ref<ModelPrice>(defaultForm())

// 模型下拉：展示名 + 类型后缀，值是 model_id（计费表的关联键）。
const modelOptions = computed(() =>
  models.value.map((m) => ({
    value: m.id ?? m.name,
    label: `${m.display_name || m.name}（${m.type}）`,
  })),
)

const columns = computed(() => [
  { colKey: 'model_id', title: t('usageStats.prices.model'), width: 200, ellipsis: true },
  { colKey: 'price_input_per_m', title: t('usageStats.prices.field.price_input_per_m'), align: 'right' },
  { colKey: 'price_output_per_m', title: t('usageStats.prices.field.price_output_per_m'), align: 'right' },
  { colKey: 'price_cached_per_m', title: t('usageStats.prices.field.price_cached_per_m'), align: 'right' },
  { colKey: 'price_per_m', title: t('usageStats.prices.field.price_per_m'), align: 'right' },
  { colKey: 'price_per_image', title: t('usageStats.prices.field.price_per_image'), align: 'right' },
  { colKey: 'price_per_audio_min', title: t('usageStats.prices.field.price_per_audio_min'), align: 'right' },
  { colKey: 'currency', title: t('usageStats.prices.currency'), width: 80 },
  { colKey: 'enabled', title: t('usageStats.prices.statusCol'), width: 90 },
  { colKey: 'effective_from', title: t('usageStats.prices.effectiveFrom'), width: 110 },
  { colKey: 'actions', title: t('usageStats.prices.actionsCol'), width: 130 },
])

const formatDate = (iso: string) => (iso ? new Date(iso).toLocaleDateString() : '—')

async function reload() {
  loading.value = true
  error.value = ''
  try {
    const resp = await listModelPrices()
    prices.value = resp.prices ?? []
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : String(e)
  } finally {
    loading.value = false
  }
}

async function loadModels() {
  try {
    models.value = await listSystemModels()
  } catch {
    // 下拉选项加载失败不阻塞配价——仍可手填 model_id。
    models.value = []
  }
}

function openCreate() {
  editing.value = false
  form.value = defaultForm()
  dialogVisible.value = true
}

function openEdit(row: ModelPrice) {
  editing.value = true
  form.value = { ...row }
  dialogVisible.value = true
}

async function handleSave() {
  const modelId = (form.value.model_id ?? '').trim()
  if (!modelId) {
    void MessagePlugin.warning(t('usageStats.prices.modelRequired'))
    return
  }
  saving.value = true
  try {
    await upsertModelPrice(modelId, { ...form.value, model_id: modelId })
    void MessagePlugin.success(t('usageStats.prices.saved'))
    dialogVisible.value = false
    await reload()
  } catch (e: unknown) {
    void MessagePlugin.error(e instanceof Error ? e.message : String(e))
  } finally {
    saving.value = false
  }
}

async function handleDelete(row: ModelPrice) {
  try {
    await deleteModelPrice(row.model_id)
    void MessagePlugin.success(t('usageStats.prices.deleted'))
    await reload()
  } catch (e: unknown) {
    void MessagePlugin.error(e instanceof Error ? e.message : String(e))
  }
}

onMounted(() => {
  void reload()
  void loadModels()
})
</script>

<style scoped>
.billing-prices-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.billing-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.billing-table-shell {
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--td-radius-medium);
  overflow: hidden;
}

.billing-branch {
  padding: 24px 0;
}

.billing-row-actions {
  display: flex;
  gap: 4px;
}

.billing-form {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.billing-form-hint {
  margin: 0;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.billing-form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.billing-form-row {
  display: flex;
  gap: 24px;
}

.billing-form-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}

.billing-form-field--wide {
  width: 100%;
}

.billing-form-field--switch {
  flex-direction: row;
  align-items: center;
  gap: 10px;
}

.billing-form-label {
  font-size: 13px;
  color: var(--td-text-color-secondary);
}
</style>
