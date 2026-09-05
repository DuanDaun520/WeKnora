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
