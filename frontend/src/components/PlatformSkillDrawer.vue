<template>
  <SettingDrawer
    class="platform-skill-drawer"
    :visible="visible"
    :title="isEdit ? t('skillLibrary.editTitle') : t('skillLibrary.registerTitle')"
    :description="isEdit ? t('skillLibrary.editDescription') : t('skillLibrary.registerDescription')"
    icon="root-list"
    width="640px"
    :min-width="520"
    :max-width="860"
    storage-key="setting-drawer:width:platform-skill"
    :confirm-loading="saving"
    :confirm-disabled="primaryDisabled"
    :confirm-text="t('common.save')"
    @confirm="handleSave"
    @cancel="close"
    @update:visible="(v: boolean) => emit('update:visible', v)"
  >
    <!-- 名称不可改（改名=新技能）：服务端 400 name_immutable 时同样落到这里 -->
    <t-alert
      v-if="nameImmutableAlert"
      theme="warning"
      :message="t('skillLibrary.nameImmutableHint')"
      class="blocked-top"
    />

    <template v-if="isEdit">
      <section class="setting-drawer__section">
        <h4 class="setting-drawer__section-title">{{ t('skillLibrary.nameSectionTitle') }}</h4>
        <div class="skill-name-readonly" :title="skill?.name">{{ skill?.name }}</div>
        <p class="section-help section-help--under-title">{{ t('skillLibrary.nameImmutableHint') }}</p>
      </section>
    </template>

    <!-- 注册只走 zip 上传（从源拉取的公网依赖不适合内网环境，界面已去掉；
         上传上限与空间「技能管理」注册步一致） -->
    <section class="setting-drawer__section">
      <h4 class="setting-drawer__section-title">{{ t('settings.sandbox.skillUploadSection') }}</h4>
      <p class="section-help section-help--under-title">
        {{ t('settings.sandbox.skillUploadSectionHint', { size: maxSkillBundleMB }) }}
      </p>
      <input
        ref="fileInputRef"
        type="file"
        accept=".zip,application/zip"
        class="file-input-hidden"
        @change="onFileInputChange"
      />
      <div
        class="skill-upload-area"
        :class="{ 'has-file': !!pendingFile, 'is-disabled': busy }"
        @click="!busy && fileInputRef?.click()"
        @dragover.prevent
        @dragenter.prevent
        @drop.prevent="onFileDrop"
      >
        <t-icon name="cloud-upload" size="28px" class="skill-upload-area__icon" />
        <div class="skill-upload-area__text">
          <span v-if="pendingFile" class="skill-upload-area__name" :title="pendingFile.name">
            {{ t('settings.skills.addFileSelected', { name: pendingFile.name }) }}
          </span>
          <template v-else>
            <span>{{ t('settings.sandbox.skillUploadClick') }}</span>
            <span class="skill-upload-area__secondary">
              {{ t('settings.sandbox.skillUploadDrag') }}
            </span>
          </template>
        </div>
        <t-progress v-if="uploading" :percentage="uploadPercent" size="small" class="skill-upload-area__progress" />
      </div>
      <t-button
        v-if="pendingFile"
        variant="text"
        size="small"
        :disabled="busy"
        @click="clearFile"
      >
        {{ t('settings.skills.addClearFile') }}
      </t-button>
    </section>

    <!-- 基本信息（000104/000105）：分类只能从已登记的分类中选择 —— 新分类在
         技能库「分类管理」里先创建（000105），本下拉不再即输即建。分类留空或
         前端未匹配登记表时回落到未分类；作者留空时注册自动读取 SKILL.md。
         编辑时与 bundle 重注册分开保存 —— 仅改动的字段会点亮 drift，需推送
         后才进入各空间。 -->
    <section class="setting-drawer__section">
      <h4 class="setting-drawer__section-title">{{ t('skillLibrary.metaSectionTitle') }}</h4>
      <p class="section-help section-help--under-title">{{ t('skillLibrary.metaSectionHint') }}</p>
      <div class="meta-grid">
        <div class="meta-field">
          <span class="meta-field__label">{{ t('skillLibrary.categoryLabel') }}</span>
          <t-select
            v-model="categoryInput"
            :placeholder="t('skillLibrary.categoryPlaceholder')"
            :loading="categoriesLoading"
            filterable
            clearable
            :disabled="busy"
          >
            <t-option v-for="c in categoryOptions" :key="c" :value="c" :label="c" />
          </t-select>
        </div>
        <div class="meta-field">
          <span class="meta-field__label">{{ t('skillLibrary.authorLabel') }}</span>
          <t-input
            v-model="authorInput"
            :placeholder="t('skillLibrary.authorPlaceholder')"
            :maxlength="255"
            :disabled="busy"
          />
        </div>
      </div>
      <!-- 000106：中文展示元数据。独立于 SKILL.md 的 name/description，空值时
           列表回落 SKILL.md 内容；描述只影响展示，不写入技能包。 -->
      <div class="meta-field meta-field--span2">
        <span class="meta-field__label">{{ t('skillLibrary.zhNameLabel') }}</span>
        <t-input
          v-model="zhNameInput"
          :placeholder="t('skillLibrary.zhNamePlaceholder')"
          :maxlength="255"
          :disabled="busy"
        />
      </div>
      <div class="meta-field meta-field--span2">
        <span class="meta-field__label">{{ t('skillLibrary.zhDescriptionLabel') }}</span>
        <t-textarea
          v-model="zhDescriptionInput"
          :placeholder="t('skillLibrary.zhDescriptionPlaceholder')"
          :maxlength="2000"
          :disabled="busy"
          :autosize="{ minRows: 2, maxRows: 5 }"
        />
      </div>
      <p v-if="isEdit && skill?.version" class="meta-version">
        {{ t('skillLibrary.currentVersion', { version: skill.version }) }}
      </p>
    </section>

    <!-- 空间分配（仅编辑已保存技能时出现）：保存是两个请求 —— 技能更新 PUT 先
         落地，分配 replace-all 跟上；被阻止的是逐空间结果，抽屉保留并把原因
         列在下方。 -->
    <template v-if="isEdit">
      <section class="setting-drawer__section">
        <h4 class="setting-drawer__section-title">{{ t('skillLibrary.assignmentsSection') }}</h4>
        <p class="section-help section-help--under-title">
          {{ t('skillLibrary.assignmentsSectionDesc') }}
        </p>
        <div v-if="!assignmentRows.length" class="assignment-empty">
          {{ t('skillLibrary.noPlatformTenants') }}
        </div>
        <template v-else>
          <div v-for="row in assignmentRows" :key="row.tenantId" class="assignment-row">
            <div class="assignment-row__main">
              <span class="assignment-row__name" :title="row.tenantName">{{ row.tenantName }}</span>
              <span v-if="row.assigned && row.drift" class="assignment-row__hint">
                {{ t('skillLibrary.assignmentDriftHint') }}
              </span>
              <span v-else-if="row.assigned && row.installCount > 0" class="assignment-row__hint">
                {{ t('skillLibrary.assignmentSkillsHint', { count: row.installCount }) }}
              </span>
              <!-- 已分配但空间尚未安装到任何沙箱：引导文案而非静默（000106） -->
              <span v-else-if="row.assigned" class="assignment-row__hint--muted">
                {{ t('skillLibrary.assignmentNotInstalledHint') }}
              </span>
            </div>
            <t-switch
              v-model="row.assigned"
              size="small"
              :aria-label="t('skillLibrary.assignSwitchLabel')"
            />
          </div>
        </template>
        <div v-if="blockedOutcomes.length" class="assignment-outcomes">
          <p class="section-help">{{ t('skillLibrary.assignmentBlockedTitle') }}</p>
          <div
            v-for="outcome in blockedOutcomes"
            :key="outcome.tenant_id"
            class="assignment-outcome"
          >
            <t-tag theme="warning" variant="light" size="small">
              {{ outcome.tenant_name || `#${outcome.tenant_id}` }}
            </t-tag>
            <span class="assignment-outcome__reason">{{ outcomeLabel(outcome) }}</span>
          </div>
        </div>
      </section>
    </template>
  </SettingDrawer>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import SettingDrawer from '@/components/settings/SettingDrawer.vue'
import { listPlatformTenants, type PlatformTenant } from '@/api/system'
import {
  createPlatformSkillFromFile,
  listSkillCategories,
  updatePlatformSkillFromFile,
  updatePlatformSkillMeta,
  updatePlatformSkillTenantAssignments,
  type PlatformSkill,
  type PlatformSkillAssignment,
  type SkillAssignmentOutcome,
} from '@/api/skill-library'
import { MAX_SKILL_BUNDLE_SIZE_BYTES, MAX_SKILL_BUNDLE_SIZE_MB } from '@/utils'

const props = defineProps<{
  visible: boolean
  /** Null opens the drawer in register (create) mode. */
  skill: PlatformSkill | null
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  /** Fired after the skill (and, in edit mode, the assignment replace) landed — partially or fully. */
  saved: []
}>()

const { t } = useI18n()

const isEdit = computed(() => !!props.skill)

// ---- register inputs ----
const pendingFile = ref<File | null>(null)
const uploading = ref(false)
const uploadPercent = ref(0)
const fileInputRef = ref<HTMLInputElement | null>(null)
// A name_immutable refusal (zip carries a different name) keeps the drawer
// open with the alert pinned on top.
const nameImmutableAlert = ref(false)

const busy = computed(() => uploading.value)
const hasBundleInput = computed(() => !!pendingFile.value)
const saving = computed(() => busy.value || savingAssignments.value || savingMeta.value)
const primaryDisabled = computed(() => !isEdit.value && !hasBundleInput.value)

// ---- definition metadata: category / author (000104) ----
// Create mode rides the register POST (empty = fall back to SKILL.md); edit
// mode persists through the separate meta PUT only when a value changed — a
// same-value PUT would bump updated_at and fake a drift marker per assignment.
const categoryInput = ref('')
const authorInput = ref('')
const zhNameInput = ref('')
const zhDescriptionInput = ref('')
const categoryOptions = ref<string[]>([])
const categoriesLoading = ref(false)
const savingMeta = ref(false)
const metaBaseline = ref({ category: '', author: '', zhName: '', zhDescription: '' })

async function loadSkillCategoriesOnce() {
  if (categoryOptions.value.length) return
  categoriesLoading.value = true
  try {
    const rows = await listSkillCategories()
    categoryOptions.value = rows.map((row) => row.name)
    // A skill may already carry a category; keep it selectable even if the
    // registry list is somehow stale (a category can only ever be set when it
    // was registered, but the edit view stays open over slow refreshes).
    const baseline = metaBaseline.value.category
    if (baseline && !categoryOptions.value.includes(baseline)) {
      categoryOptions.value = [baseline, ...categoryOptions.value]
    }
  } catch {
    // A failed directory load only costs the dropdown suggestions.
  } finally {
    categoriesLoading.value = false
  }
}

// ---- workspace assignment section (edit mode only) ----
const platformTenants = ref<PlatformTenant[]>([])
const assignmentRows = ref<Array<{
  tenantId: number
  tenantName: string
  assigned: boolean
  drift: boolean
  installCount: number
}>>([])
const assignmentOutcomes = ref<SkillAssignmentOutcome[]>([])
const savingAssignments = ref(false)
const blockedOutcomes = computed(() =>
  assignmentOutcomes.value.filter((r) => r.status === 'blocked' || r.status === 'error'))

function outcomeLabel(outcome: SkillAssignmentOutcome): string {
  switch (outcome.code) {
    case 'skills_installed':
      return t('skillLibrary.outcomeSkillsInstalled', {
        skills: (outcome.skill_names || []).join('、'),
      })
    case 'name_conflict':
      return t('skillLibrary.outcomeNameConflict')
    default:
      return outcome.message || t('skillLibrary.outcomeFailed')
  }
}

async function loadPlatformTenantsOnce() {
  if (platformTenants.value.length) return
  try {
    const res = await listPlatformTenants()
    platformTenants.value = res.tenants || []
  } catch {
    // The section renders its own empty hint; there is nothing to retry into.
  }
}

// Seeds the switches from the skill's assignment rows: assigned flags plus
// the drift marker and install count the hints are built from.
function seedAssignmentRows(assignments: PlatformSkillAssignment[]) {
  const byTenant = new Map(assignments.map((a) => [a.tenant_id, a]))
  assignmentRows.value = platformTenants.value.map((tenant) => {
    const assignment = byTenant.get(tenant.id)
    return {
      tenantId: tenant.id,
      tenantName: tenant.name,
      assigned: !!assignment,
      drift: !!assignment?.drift,
      installCount: assignment?.install_count ?? 0,
    }
  })
}

// Re-applies what the server reports after a replace-all: a blocked unassign
// keeps that workspace assigned, so its switch has to snap back on.
function reseedAssignmentRows(assignments: PlatformSkillAssignment[]) {
  const byTenant = new Map(assignments.map((a) => [a.tenant_id, a]))
  assignmentRows.value = assignmentRows.value.map((row) => {
    const assignment = byTenant.get(row.tenantId)
    return {
      ...row,
      assigned: !!assignment,
      drift: !!assignment?.drift,
      installCount: assignment?.install_count ?? row.installCount,
    }
  })
}

watch(
  () => props.visible,
  async (open) => {
    if (!open) return
    clearFile()
    nameImmutableAlert.value = false
    assignmentOutcomes.value = []
    metaBaseline.value = {
      category: props.skill?.category || '',
      author: props.skill?.author || '',
      zhName: props.skill?.zh_name || '',
      zhDescription: props.skill?.zh_description || '',
    }
    categoryInput.value = metaBaseline.value.category
    authorInput.value = metaBaseline.value.author
    zhNameInput.value = metaBaseline.value.zhName
    zhDescriptionInput.value = metaBaseline.value.zhDescription
    void loadSkillCategoriesOnce()
    if (props.skill) {
      await loadPlatformTenantsOnce()
      seedAssignmentRows(props.skill.assignments || [])
    }
  },
)

// ---- file inputs ----

function isZipFile(file: File): boolean {
  return file.name.toLowerCase().endsWith('.zip') || file.type === 'application/zip'
}

function acceptPendingFile(file: File) {
  if (busy.value) return
  if (!isZipFile(file)) {
    MessagePlugin.error(t('settings.sandbox.skillUploadFailed'))
    return
  }
  if (file.size > MAX_SKILL_BUNDLE_SIZE_BYTES) {
    MessagePlugin.error(t('settings.sandbox.skillBundleTooLarge', { size: maxSkillBundleMB }))
    return
  }
  pendingFile.value = file
}

function onFileInputChange(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (file) acceptPendingFile(file)
  input.value = ''
}

function onFileDrop(e: DragEvent) {
  const file = e.dataTransfer?.files?.[0]
  if (file) acceptPendingFile(file)
}

function clearFile() {
  pendingFile.value = null
  uploadPercent.value = 0
  if (fileInputRef.value) fileInputRef.value.value = ''
}

const maxSkillBundleMB = MAX_SKILL_BUNDLE_SIZE_MB

// Mirrors the workspace register step's error mapping so platform-side
// refusals read the same as the tenant-side ones.
function registerErrorMessage(err: any): string {
  const raw = String(err?.message || '')
  if (/cannot exceed \d+\s*MB/i.test(raw)) {
    return t('settings.sandbox.skillBundleTooLarge', { size: maxSkillBundleMB })
  }
  const tooManyFiles = raw.match(/skill directory holds more than (\d+) files/i)
  if (tooManyFiles) {
    return t('settings.sandbox.skillBundleTooManyFiles', { count: tooManyFiles[1] })
  }
  const tooManyEntries = raw.match(/archive has more than (\d+) zip entries/i)
  if (tooManyEntries) {
    return t('settings.sandbox.skillBundleTooManyZipEntries', { count: tooManyEntries[1] })
  }
  if (raw) return raw
  return t('settings.sandbox.skillUploadFailed')
}

// ---- save: create registers; edit re-registers (optional new bundle) then
//      replaces the assignment set. The bundle PUT lands first so a refused
//      rename stops the save before any workspace is touched. ----
async function persistBundle(): Promise<boolean> {
  if (!hasBundleInput.value) return true
  nameImmutableAlert.value = false
  // Empty metadata on CREATE means "read it from the SKILL.md frontmatter".
  const category = categoryInput.value.trim()
  const author = authorInput.value.trim()
  const file = pendingFile.value
  if (!file) return true
  try {
    uploading.value = true
    uploadPercent.value = 0
    const onProgress = (percent: number) => { uploadPercent.value = percent }
    if (isEdit.value) {
      await updatePlatformSkillFromFile(props.skill!.id, file, onProgress)
    } else {
      const zhName = zhNameInput.value.trim()
      const zhDescription = zhDescriptionInput.value.trim()
      await createPlatformSkillFromFile(file, onProgress, category, author, zhName, zhDescription)
    }
    return true
  } catch (e: any) {
    if (e?.error?.code === 'name_immutable') {
      nameImmutableAlert.value = true
      MessagePlugin.warning(t('skillLibrary.nameImmutableHint'))
    } else if (e?.error?.code === 'name_conflict') {
      MessagePlugin.warning(e?.message || t('skillLibrary.toasts.registerFailed'))
    } else {
      MessagePlugin.error(registerErrorMessage(e))
    }
    return false
  } finally {
    uploading.value = false
    uploadPercent.value = 0
  }
}

async function persistAssignments(): Promise<boolean> {
  const skillId = props.skill!.id
  const rows = assignmentRows.value
    .filter((row) => row.assigned)
    .map((row) => ({ tenant_id: row.tenantId }))
  savingAssignments.value = true
  try {
    const res = await updatePlatformSkillTenantAssignments(skillId, rows)
    reseedAssignmentRows(res.assignments || [])
    assignmentOutcomes.value = res.results || []
    const blocked = res.results?.some(
      (r: SkillAssignmentOutcome) => r.status === 'blocked' || r.status === 'error')
    return !blocked
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('skillLibrary.toasts.updated'))
    return false
  } finally {
    savingAssignments.value = false
  }
}

// Edit-mode metadata save. Skipped when nothing changed so an untouched
// drawer never lights drift; an explicit empty string clears the field.
async function persistMeta(): Promise<boolean> {
  if (!isEdit.value) return true
  const category = categoryInput.value.trim()
  const author = authorInput.value.trim()
  const zhName = zhNameInput.value.trim()
  const zhDescription = zhDescriptionInput.value.trim()
  const same =
    category === metaBaseline.value.category &&
    author === metaBaseline.value.author &&
    zhName === metaBaseline.value.zhName &&
    zhDescription === metaBaseline.value.zhDescription
  if (same) return true
  savingMeta.value = true
  try {
    await updatePlatformSkillMeta(props.skill!.id, {
      category,
      author,
      zh_name: zhName,
      zh_description: zhDescription,
    })
    metaBaseline.value = { category, author, zhName, zhDescription }
    return true
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('skillLibrary.toasts.metaFailed'))
    return false
  } finally {
    savingMeta.value = false
  }
}

async function handleSave() {
  if (saving.value || primaryDisabled.value) return
  if (!(await persistBundle())) return
  if (!isEdit.value) {
    MessagePlugin.success(t('skillLibrary.toasts.registered'))
    emit('saved')
    close()
    return
  }
  if (!(await persistMeta())) return
  const clean = await persistAssignments()
  emit('saved')
  if (clean) {
    MessagePlugin.success(t('skillLibrary.toasts.updated'))
    close()
  }
  // Blocked outcomes keep the drawer open with the reasons listed below.
}

function close() {
  emit('update:visible', false)
}
</script>

<style scoped lang="less">
.blocked-top {
  margin-bottom: 12px;
}

.skill-name-readonly {
  padding: 8px 12px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  background: var(--td-bg-color-secondarycontainer);
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-input-hidden {
  display: none;
}

.meta-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;

  @media (max-width: 640px) {
    grid-template-columns: 1fr;
  }
}

.meta-field {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.meta-field__label {
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.meta-field--span2 {
  grid-column: 1 / -1;
}

.meta-version {
  margin: 8px 0 0;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.skill-upload-area {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 16px;
  border: 1px dashed var(--td-component-stroke);
  border-radius: 8px;
  background: var(--td-bg-color-container);
  cursor: pointer;
  transition: border-color 0.15s ease, background 0.15s ease;

  &:hover:not(.is-disabled),
  &.has-file {
    border-color: var(--td-brand-color-5, var(--td-brand-color));
  }

  &.is-disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
}

.skill-upload-area__icon {
  flex-shrink: 0;
  color: var(--td-text-color-placeholder);
}

.skill-upload-area__text {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 13px;
  color: var(--td-text-color-secondary);
}

.skill-upload-area__name {
  color: var(--td-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.skill-upload-area__secondary {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.skill-upload-area__progress {
  flex-shrink: 0;
  min-width: 120px;
}

.assignment-empty {
  padding: 12px 0;
  font-size: 13px;
  color: var(--td-text-color-placeholder);
}

.assignment-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 7px 0;

  & + .assignment-row {
    border-top: 1px solid var(--td-component-stroke);
  }
}

.assignment-row__main {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 8px;
}

.assignment-row__name {
  min-width: 0;
  font-size: 13px;
  color: var(--td-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.assignment-row__hint {
  flex-shrink: 0;
  font-size: 11px;
  color: var(--td-warning-color-7, #B85C00);
}

.assignment-row__hint--muted {
  flex-shrink: 0;
  font-size: 11px;
  color: var(--td-text-color-placeholder);
}

.assignment-outcomes {
  margin-top: 10px;
  padding-top: 8px;
  border-top: 1px dashed var(--td-component-stroke);
}

.assignment-outcome {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 0;
}

.assignment-outcome__reason {
  min-width: 0;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}
</style>
