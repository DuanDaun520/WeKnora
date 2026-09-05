import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8')

const listSource = read('./KnowledgeBaseList.vue')
const detailSource = read('./KnowledgeBase.vue')
const editorSource = read('./KnowledgeBaseEditorModal.vue')
const cardSource = read('./components/DocumentCardView.vue')
const listRowSource = read('./components/DocumentListView.vue')
const actionMenuSource = read('./components/DocumentActionMenu.vue')
const dropdownSource = read('./components/KbUploadSourceDropdown.vue')
const localeSource = read('../../i18n/locales/zh-CN.ts')

test('only tenant admins can create knowledge bases (grayed-out button + hint)', () => {
  // 000099 权限收口：创建按钮对普通用户置灰，tooltip 提示仅空间管理员可建。
  assert.match(listSource, /const canCreateKB = computed/)
  assert.match(listSource, /:disabled="!canCreateKB"/)
  assert.match(listSource, /knowledgeList\.createAdminOnly/)
})

test('KB cards show a lock badge for the co-maintain flag', () => {
  // 锁图标：lock-off（开放成员维护）/ lock-on（仅管理员）。
  assert.match(listSource, /lock-off/)
  assert.match(listSource, /lock-on/)
  assert.match(listSource, /kb\.allow_member_contribute/)
  assert.match(listSource, /knowledgeList\.features\.coMaintain/)
  assert.match(listSource, /knowledgeList\.features\.adminOnlyLock/)
})

test('the editor exposes the co-maintain switch to KB creator/admin only', () => {
  // 开关复用 canShareKB 矩阵（创建者或空间管理员），提交时始终携带状态。
  assert.match(editorSource, /allowMemberContribute: false/)
  assert.match(editorSource, /kb as any\)\.allow_member_contribute/)
  assert.match(editorSource, /v-model="formData\.allowMemberContribute"/)
  assert.match(editorSource, /editorMode === 'edit' \? canShareKB : true/)
  assert.match(editorSource, /allow_member_contribute: data\.allow_member_contribute/)
})

test('the KB detail page gates member contribution and per-item mutations', () => {
  // 成员贡献门控：canContributeKnowledge 控制「添加文档」入口；canMutateItem
  // 按条目创建者放行；DocContent 详情对本人条目放开编辑。
  assert.match(detailSource, /const canContributeKnowledge = computed/)
  assert.match(detailSource, /allow_member_contribute && authStore\.hasRole\('contributor'\)/)
  assert.match(detailSource, /const canMutateItem = \(item: any\): boolean =>/)
  assert.match(detailSource, /item\.creator_id === uid/)
  assert.match(detailSource, /v-if="canContributeKnowledge" class="doc-filter-actions"/)
  assert.match(detailSource, /:canEditKB="canEdit \|\| canMutateItem\(details\)"/)
  assert.match(detailSource, /:can-mutate-item="canMutateItem"/)
})

test('card/list views and the action menu honor per-item permission', () => {
  // more 菜单对「有任一条目操作权」的用户可见；菜单内编辑/重建/取消/删除
  // 均需条目级权限，移动/批量仍要求 KB 管理权。
  assert.match(cardSource, /v-if="canEdit \|\| canMutateThisItem\(item\)"/)
  assert.match(cardSource, /:can-mutate-item="canMutateThisItem\(item\)"/)
  assert.match(listRowSource, /v-if="canEdit \|\| canMutateThisItem\(item\)" @click\.stop/)
  assert.match(listRowSource, /:can-mutate-item="canMutateThisItem\(item\)"/)
  assert.match(listRowSource, /const showActionsColumn = computed/)
  assert.match(actionMenuSource, /canMutateItem: true,/)
  assert.match(actionMenuSource, /v-if="canMutateItem && item\.type === 'manual'"/)
  assert.match(actionMenuSource, /v-if="canMutateItem && isParseInFlight"/)
  assert.match(actionMenuSource, /v-else-if="canMutateItem" theme="warning"/)
  assert.match(actionMenuSource, /v-if="canMutateItem" theme="warning"/)
})

test('the add-document trigger renders as icon + label', () => {
  // 需求 5：传入 label 时按钮为「图标+文字」形态。
  assert.match(dropdownSource, /label\?: string/)
  assert.match(dropdownSource, /v-if="label" class="kb-upload-source-label"/)
  assert.match(detailSource, /:label="t\('knowledgeBase\.addDocument'\)"/)
})

test('zh-CN carries the new co-maintain copy', () => {
  assert.match(localeSource, /createAdminOnly: '只有空间管理员才可以新建知识库'/)
  assert.match(localeSource, /coMaintain: '允许空间成员共同维护知识'/)
  assert.match(localeSource, /adminOnlyLock: '仅空间管理员可添加知识'/)
  assert.match(localeSource, /allowMemberContributeLabel: '允许空间成员共同维护知识'/)
  assert.match(localeSource, /allowMemberContributeOn: '成员可添加知识，并维护自己添加的内容'/)
  assert.match(localeSource, /allowMemberContributeOff: '仅空间管理员和创建者可维护'/)
})

test('the uploader name renders under the updated time (gray, left-aligned)', () => {
  // 上传人 = knowledges.creator_id 解析出的展示名。列表视图与卡片视图都在
  // 更新时间下方用灰色小字渲染，并与更新时间左对齐（外层容器保持原对齐）。
  assert.match(listRowSource, /creator_name\?: string/)
  assert.match(listRowSource, /class="cell-time-stack"/)
  assert.match(listRowSource, /v-if="item\.creator_name" class="row-uploader"/)
  assert.match(cardSource, /class="card-time-stack"/)
  assert.match(cardSource, /v-if="item\.creator_name" class="card-uploader"/)
})

test('the backend backfills creator_name on the KB document list', () => {
  const knowledgeHandler = read('../../../../internal/handler/knowledge.go')
  const knowledgeType = read('../../../../internal/types/knowledge.go')
  const kbHandler = read('../../../../internal/handler/knowledgebase.go')
  assert.match(knowledgeType, /CreatorName string `json:"creator_name,omitempty" gorm:"-"`/)
  assert.match(knowledgeHandler, /enrichKnowledgeCreatorNames\(ctx, h\.userService, result\.Data\)/)
  assert.match(kbHandler, /func enrichKnowledgeCreatorNames\(/)
})
