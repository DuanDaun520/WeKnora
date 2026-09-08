<template>
  <div class="platform-storage-engines">
    <div class="section-header">
      <div class="section-header__top">
        <div>
          <h2>{{ t('settings.platformStorage.title') }}</h2>
          <p class="section-description">{{ t('settings.platformStorage.description') }}</p>
        </div>
      </div>
    </div>

    <t-loading :loading="loading" size="small" class="engine-list-loading">
      <div v-if="!loading" class="engine-grid">
        <div
          v-for="engine in engines"
          :key="engine.id"
          class="engine-card"
          :class="[`engine-card--${engine.provider}`]"
          role="button"
          tabindex="0"
          @click="openEdit(engine)"
          @keydown.enter="openEdit(engine)"
        >
          <div class="engine-card__badge" :class="badgeClass(engine.provider)" :style="badgeStyle(engine.provider)">
            <span>{{ providerInitial(engine.provider) }}</span>
          </div>
          <div class="engine-card__body">
            <div class="engine-card__header">
              <h3 class="engine-card__title">{{ engine.name }}</h3>
              <t-tag v-if="engine.status === 'active'" theme="success" variant="light" size="small">{{ t('settings.platformStorage.active') }}</t-tag>
              <t-tag v-else theme="danger" variant="light" size="small">{{ t('settings.platformStorage.disabled') }}</t-tag>
            </div>
            <p class="engine-card__subtitle">
              <span>{{ engine.provider.toUpperCase() }}</span>
              <span class="engine-card__sep">·</span>
              <span class="engine-card__meta">{{ engineMeta(engine) }}</span>
            </p>
          </div>
        </div>

        <button type="button" class="engine-card engine-card--add" @click="openCreate">
          <span class="engine-card--add__icon" aria-hidden="true">
            <add-icon />
          </span>
          <span class="engine-card--add__label">{{ t('settings.platformStorage.add') }}</span>
        </button>
      </div>
    </t-loading>

    <SettingDrawer
      v-model:visible="visible"
      :title="editing ? t('settings.platformStorage.editTitle') : t('settings.platformStorage.createTitle')"
      :confirm-loading="saving"
      @confirm="save"
      @cancel="visible = false"
    >
      <t-form :data="form" layout="vertical">
        <section class="setting-drawer__section">
          <h4 class="setting-drawer__section-title">{{ t('settings.platformStorage.basicSection') }}</h4>
          <div class="form-item">
            <label class="form-label required">{{ t('settings.platformStorage.nameLabel') }}</label>
            <t-input v-model="form.name" :placeholder="t('settings.platformStorage.namePlaceholder')" clearable />
          </div>
          <div class="form-item">
            <label class="form-label required">{{ t('settings.platformStorage.providerLabel') }}</label>
            <t-select v-model="form.provider" :disabled="!!editing" @change="resetConfig">
              <t-option v-for="provider in providers" :key="provider" :value="provider" :label="provider.toUpperCase()" />
            </t-select>
          </div>
        </section>

        <section class="setting-drawer__section">
          <h4 class="setting-drawer__section-title">{{ t('settings.platformStorage.connectionSection') }}</h4>
          <div v-if="needsEndpoint" class="form-item">
            <label class="form-label required">Endpoint</label>
            <t-input v-model="form.config.endpoint" :disabled="!!editing" clearable />
          </div>
          <div v-if="needsRegion" class="form-item">
            <label class="form-label required">Region</label>
            <t-input v-model="form.config.region" :disabled="!!editing" clearable />
          </div>
          <template v-if="needsCredentials">
            <div class="form-item">
              <label class="form-label required">Access Key / Secret ID</label>
              <t-input v-model="form.config.access_key_id" placeholder="***" clearable />
            </div>
            <div class="form-item">
              <label class="form-label required">Secret Key</label>
              <t-input v-model="form.config.secret_access_key" type="password" placeholder="***" clearable />
            </div>
          </template>
          <div v-if="form.provider !== 'local'" class="form-item">
            <label class="form-label required">Bucket</label>
            <t-input v-model="form.config.bucket_name" :disabled="!!editing" clearable />
          </div>
        </section>

        <section class="setting-drawer__section">
          <h4 class="setting-drawer__section-title">{{ t('settings.platformStorage.advancedSection') }}</h4>
          <div class="form-item">
            <label class="form-label">{{ t('settings.platformStorage.pathPrefixLabel') }}</label>
            <t-input v-model="form.config.path_prefix" :disabled="!!editing" placeholder="weknora/" clearable />
          </div>
        </section>
      </t-form>

      <template #footer-left>
        <t-button variant="outline" :loading="testing" @click="testRaw">
          <template #icon>
            <t-icon v-if="!testing && testResult === 'ok'" name="check-circle-filled" class="status-icon available" />
            <t-icon v-else-if="!testing && testResult === 'error'" name="close-circle-filled" class="status-icon unavailable" />
          </template>
          {{ t('settings.platformStorage.testConnection') }}
        </t-button>
      </template>
    </SettingDrawer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { DialogPlugin, MessagePlugin } from 'tdesign-vue-next'
import { AddIcon } from 'tdesign-icons-vue-next'
import { useI18n } from 'vue-i18n'
import SettingDrawer from '@/components/settings/SettingDrawer.vue'
import {
  createPlatformStorageEngine,
  deletePlatformStorageEngine,
  listPlatformStorageEngines,
  listPlatformStorageEngineTypes,
  testPlatformStorageEngine,
  testPlatformStorageEngineByID,
  updatePlatformStorageEngine,
  type PlatformStorageEngine,
} from '@/api/platform-storage-engine'

const { t } = useI18n()
const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const visible = ref(false)
const engines = ref<PlatformStorageEngine[]>([])
const providers = ref<string[]>([])
const editing = ref<PlatformStorageEngine | null>(null)
const testResult = ref<'ok' | 'error' | null>(null)

const blankConfig = () => ({
  mode: 'remote',
  endpoint: '',
  region: '',
  access_key_id: '',
  secret_access_key: '',
  bucket_name: '',
  path_prefix: '',
  use_ssl: true,
})

const form = reactive<{ name: string; provider: string; config: ReturnType<typeof blankConfig> }>({
  name: '',
  provider: 'local',
  config: blankConfig(),
})

const needsEndpoint = computed(() => !['local', 'cos'].includes(form.provider))
const needsRegion = computed(() => !['local', 'minio'].includes(form.provider))
const needsCredentials = computed(() => form.provider !== 'local')

const providerInitial = (provider: string) => (provider || '?').trim().charAt(0).toUpperCase() || '?'

const badgeClass = (provider: string) => ({
  'engine-card__badge--logo': false,
  'engine-card__badge--color': true,
})

const badgeStyle = (provider: string): Record<string, string> => {
  const colors: Record<string, string> = {
    local: 'rgba(70, 70, 70, 0.1)',
    minio: 'rgba(225, 38, 38, 0.12)',
    cos: 'rgba(0, 82, 217, 0.1)',
    tos: 'rgba(0, 137, 255, 0.12)',
    s3: 'rgba(255, 153, 0, 0.12)',
    oss: 'rgba(255, 90, 0, 0.12)',
    ks3: 'rgba(7, 192, 95, 0.12)',
    obs: 'rgba(206, 17, 38, 0.1)',
  }
  return { background: colors[provider] || 'rgba(0, 82, 217, 0.1)' }
}

function engineMeta(engine: PlatformStorageEngine): string {
  return engine.config.endpoint || engine.config.bucket_name || engine.config.path_prefix || t('settings.platformStorage.localStorage')
}

async function load() {
  loading.value = true
  try {
    const [list, types] = await Promise.all([listPlatformStorageEngines(), listPlatformStorageEngineTypes()])
    engines.value = list.data || []
    providers.value = types.data || []
  } finally {
    loading.value = false
  }
}

function resetConfig() {
  form.config = blankConfig()
  testResult.value = null
}

function openCreate() {
  editing.value = null
  form.name = ''
  form.provider = providers.value[0] || 'local'
  form.config = blankConfig()
  testResult.value = null
  visible.value = true
}

function openEdit(engine: PlatformStorageEngine) {
  editing.value = engine
  form.name = engine.name
  form.provider = engine.provider
  form.config = { ...blankConfig(), ...engine.config }
  testResult.value = null
  visible.value = true
}

async function testRaw() {
  testing.value = true
  testResult.value = null
  try {
    const r: any = editing.value
      ? await testPlatformStorageEngineByID(editing.value.id)
      : await testPlatformStorageEngine(form)
    if (r.success) {
      testResult.value = 'ok'
      MessagePlugin.success(t('settings.platformStorage.testSuccess'))
    } else {
      testResult.value = 'error'
      MessagePlugin.error(r.error || t('settings.platformStorage.testFailed'))
    }
  } finally {
    testing.value = false
  }
}

async function save() {
  if (!form.name.trim()) {
    MessagePlugin.warning(t('settings.platformStorage.nameRequired'))
    return
  }
  saving.value = true
  try {
    const payload = { name: form.name.trim(), provider: form.provider, config: { ...form.config } }
    if (editing.value) {
      await updatePlatformStorageEngine(editing.value.id, payload)
    } else {
      await createPlatformStorageEngine(payload)
    }
    MessagePlugin.success(t('settings.platformStorage.saveSuccess'))
    visible.value = false
    await load()
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('settings.platformStorage.saveFailed'))
  } finally {
    saving.value = false
  }
}

function remove(engine: PlatformStorageEngine) {
  const dialog = DialogPlugin.confirm({
    header: t('settings.platformStorage.deleteTitle'),
    body: t('settings.platformStorage.deleteConfirm', { name: engine.name }),
    onConfirm: async () => {
      dialog.destroy()
      try {
        await deletePlatformStorageEngine(engine.id)
        await load()
        MessagePlugin.success(t('settings.platformStorage.deleted'))
      } catch (e: any) {
        MessagePlugin.error(e?.message || t('settings.platformStorage.deleteFailed'))
      }
    },
    onCancel: () => dialog.destroy(),
  })
}

onMounted(load)
</script>

<style scoped lang="less">
.platform-storage-engines {
  width: 100%;
}

.section-header {
  margin-bottom: 28px;

  h2 {
    font-size: 20px;
    font-weight: 600;
    color: var(--td-text-color-primary);
    margin: 0 0 8px 0;
  }

  .section-description {
    font-size: 14px;
    color: var(--td-text-color-secondary);
    margin: 0;
    line-height: 1.6;
  }
}

.engine-list-loading {
  min-height: 120px;
}

.engine-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 12px;

  .engine-card--add {
    width: 100%;
    height: 100%;
  }
}

.engine-card {
  position: relative;
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 14px 16px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 10px;
  background: var(--td-bg-color-container);
  transition: border-color 0.18s ease, box-shadow 0.18s ease;
  min-width: 0;
  cursor: pointer;

  &:hover {
    border-color: var(--td-brand-color-3, var(--td-brand-color));
    box-shadow: 0 4px 14px rgba(15, 23, 42, 0.06);
  }

  &--add {
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    min-height: 68px;
    border-style: dashed;
    background: transparent;
    color: var(--td-text-color-placeholder);
    font: inherit;
    text-align: center;

    &:hover,
    &:focus-visible {
      color: var(--td-brand-color);
      border-color: var(--td-brand-color);
      background: color-mix(in srgb, var(--td-brand-color) 6%, transparent);
      box-shadow: none;
    }

    &__icon {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 32px;
      height: 32px;
      border-radius: 8px;
      background: color-mix(in srgb, var(--td-brand-color) 10%, transparent);
      color: var(--td-brand-color);
      font-size: 18px;
    }

    &__label {
      font-size: 13px;
      font-weight: 500;
      line-height: 1.4;
    }
  }
}

.engine-card__badge {
  flex-shrink: 0;
  width: 36px;
  height: 36px;
  border-radius: 9px;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-top: 1px;
  font-size: 15px;
  font-weight: 600;
  letter-spacing: 0.02em;
  color: #0052d9;
}

.engine-card__body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 2px;
}

.engine-card__header {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.engine-card__title {
  flex: 1;
  min-width: 0;
  margin: 0;
  font-size: 14px;
  font-weight: 600;
  line-height: 1.4;
  color: var(--td-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.engine-card__subtitle {
  margin: 2px 0 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--td-text-color-secondary);
  display: flex;
  align-items: center;
  min-width: 0;
}

.engine-card__sep {
  margin: 0 6px;
  color: var(--td-text-color-placeholder);
  flex-shrink: 0;
}

.engine-card__meta {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}

.form-item {
  margin-bottom: 0;
}

.form-label {
  display: block;
  margin-bottom: 6px;
  font-size: 13px;
  font-weight: 500;
  color: var(--td-text-color-primary);
  line-height: 1.4;

  &.required::before {
    content: '*';
    color: var(--td-error-color);
    margin-right: 4px;
    font-weight: 500;
    line-height: 1;
  }
}

:deep(.t-input),
:deep(.t-select) {
  width: 100%;
  font-size: 13px;
}

.status-icon {
  font-size: 16px;
  flex-shrink: 0;

  &.available {
    color: var(--td-brand-color);
  }

  &.unavailable {
    color: var(--td-error-color);
  }
}
</style>