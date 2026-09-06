import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./SkillLibraryPanel.vue', import.meta.url), 'utf8')

test('the panel reads the platform skill library API only', () => {
  assert.match(source, /listPlatformSkills/)
  assert.match(source, /pushPlatformSkill/)
  assert.match(source, /deletePlatformSkill/)
  // 平台面板不碰租户目录 API：物化后空间侧行由各空间自己的面板管理。
  assert.doesNotMatch(source, /listSkillCatalog/)
  assert.doesNotMatch(source, /installSkillCatalog/)
})

test('assignment chips carry the drift marker that push clears', () => {
  assert.match(source, /skill-card__tenant-chip--drift/)
  assert.match(source, /skill-card__drift-dot/)
  assert.match(source, /skillLibrary\.assignmentDriftHint/)
  assert.match(source, /skillLibrary\.assignmentInSync/)
  assert.match(source, /skillLibrary\.unassigned/)
})

test('push is gated on live assignments and reports per-workspace status', () => {
  // 无分配的技能没有可推送对象：菜单项只在 assignments 存在时出现。
  assert.match(source, /skill\.assignments\?\.length/)
  assert.match(source, /pushResultRows/)
  assert.match(source, /pushStatusTheme/)
  assert.match(source, /pushStatusLabel/)
  assert.match(source, /blockedNameConflict/)
  assert.match(source, /skillLibrary\.pushAction/)
  assert.match(source, /skillLibrary\.pushResultTitle/)
})

test('delete surfaces the assignments-exist refusal instead of cascading', () => {
  assert.match(source, /assignments_exist/)
  assert.match(source, /skillLibrary\.blockedAssignmentsExist/)
  assert.match(source, /skillLibrary\.deleteConfirmBody/)
})

test('register/edit drawer and platform file browser are wired in', () => {
  assert.match(source, /PlatformSkillDrawer/)
  assert.match(source, /handleSaved/)
  // 平台 zip 文件浏览器走 platform 分支（catalog-id 传平台技能 id）。
  assert.match(source, /SkillFilesDrawer/)
  assert.match(source, /platform/)
  assert.match(source, /:catalog-id="filesSkillId"/)
  assert.match(source, /skillLibrary\.browseFiles/)
})

test('panel chrome reuses the console conventions', () => {
  assert.match(source, /console-toolbar/)
  assert.match(source, /panel-header/)
  assert.match(source, /skillLibrary\.title/)
  assert.match(source, /skillLibrary\.description/)
  assert.match(source, /skillLibrary\.searchPlaceholder/)
  assert.match(source, /skillLibrary\.emptyHint/)
  assert.match(source, /skillLibrary\.noMatchHint/)
  assert.match(source, /skillLibrary\.addSkill/)
})

test('cards surface the definition metadata (000104)', () => {
  // 分类 pill 与空间技能目录卡同款；作者与更新时间同行。
  assert.match(source, /skill-card__category/)
  assert.match(source, /skill\.category/)
  assert.match(source, /skill-card__author/)
  assert.match(source, /skill\.author/)
})

test('search spans the metadata fields too (000106 adds zh copy)', () => {
  assert.match(source, /skill\.name,/)
  assert.match(source, /skill\.zh_name,/)
  assert.match(source, /skill\.description,/)
  assert.match(source, /skill\.zh_description,/)
  assert.match(source, /skill\.version,/)
  assert.match(source, /skill\.category,/)
  assert.match(source, /skill\.author,/)
})

test('category chips quick-filter the list (000106)', () => {
  assert.match(source, /CATEGORY_ALL = '__all__'/)
  assert.match(source, /const skillCategories = computed/)
  assert.match(source, /class="category-chip"/)
  assert.match(source, /skillLibrary\.categoryFilterAll/)
  assert.match(source, /skillLibrary\.uncategorized/)
  assert.match(source, /skillLibrary\.countLabel/)
})

test('cards prefer the Chinese display metadata and clamp the description (000106)', () => {
  assert.match(source, /const cardTitle = \(skill: PlatformSkill\) => skill\.zh_name \|\| skill\.name/)
  assert.match(source, /cardDescExcerpt/)
  assert.match(source, /full\.slice\(0, 40\)/)
  assert.match(source, /skill-card__subname/)
})

test('push results dialog closes via its confirm button and explains installs', () => {
  // 000106 修复：tdesign 的 confirm 按钮点击只发 @confirm，不自动关弹窗。
  assert.match(source, /@confirm="pushResultVisible = false"/)
  assert.match(source, /skillLibrary\.pushNoteInstall/)
})

test('the category manager dialog is wired in (000104)', () => {
  assert.match(source, /SkillCategoryManagerDialog/)
  assert.match(source, /skillLibrary\.categoryManager/)
  assert.match(source, /@changed="loadSkills"/)
})
