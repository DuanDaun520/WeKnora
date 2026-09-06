import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./SkillCatalogSettings.vue', import.meta.url), 'utf8')
const skillApi = readFileSync(new URL('../../api/skill/index.ts', import.meta.url), 'utf8')
const i18n = readFileSync(new URL('../../i18n/locales/zh-CN.ts', import.meta.url), 'utf8')

test('catalog cards surface the category and admins can edit it in place', () => {
  // 分类 pill 只在非空时渲染；编辑入口受 canManage（SystemAdmin）约束，
  // 弹层与安装面板同款受控模式（editingCategoryId + visible-change）。
  assert.match(source, /v-if="item\.category" class="skill-card__category"/)
  assert.match(source, /t-popup v-if="canManage" :visible="editingCategoryId === item\.id"/)
  assert.match(source, /@visible-change="\(visible: boolean\) => setCategoryEditor\(item\.id, visible\)"/)
  assert.match(source, /await updateSkillCatalogMeta\(item\.id, category\)/)
})

test('workspace registration is gone: skills only arrive via platform assignment', () => {
  // 平台单向模型：空间侧不再有登记入口（源/zip 上传向导、POST /skills/catalog
  // 都已移除），目录行只能由控制台「技能库」分配物化而来。
  assert.doesNotMatch(source, /registerSkillCatalog|showAdd|addStep|openAdd/)
  assert.doesNotMatch(source, /skill-card--add/)
  assert.match(source, /emptyDescManaged/)
})

test('the tenant register API surface is gone but install stays', () => {
  assert.doesNotMatch(skillApi, /registerSkillCatalog|SkillCatalogRegisterResult/)
  assert.doesNotMatch(skillApi, /postUpload\('\/api\/v1\/skills\/catalog'/)
  assert.match(skillApi, /\/api\/v1\/skills\/catalog\/\$\{catalogId\}\/install/)
})

test('settings.skills i18n block carries the category strings', () => {
  assert.match(i18n, /editCategory: '修改分类'/)
  assert.match(i18n, /categorySaveFailed: '分类保存失败'/)
})

test('install chip text reflects real state and hides the sandbox name (000107)', () => {
  // 单个就绪安装才写「已安装到本空间沙箱」；安装中/失败/停用不再伪装成已装，
  // 也不再暴露沙箱配置名。
  assert.match(source, /settings\.skills\.installedOnSandbox/)
  assert.match(source, /settings\.skills\.disabledOnSandbox/)
  assert.match(source, /settings\.sandbox\.skillStatusFailed/)
  assert.match(i18n, /installedOnSandbox: '已安装到本空间沙箱'/)
  assert.match(i18n, /disabledOnSandbox: '已停用'/)
})

test('install acceptance opens the manage drawer so progress is visible (000107)', () => {
  // 安装被接受后自动定位到目标沙箱的「管理」抽屉（SandboxSkillsPanel 会订阅
  // 该安装的事件流），而不是只弹 toast 让用户自己找忙碌 chip。
  assert.match(source, /if \(started\) openManage\(item, started\)/)
})

test('the skill name owns its own line above the meta pills', () => {
  // 英文名独占一行：版本/分类/溯源 pill 换行到独立 tags 行，不再挤压标题。
  assert.match(source, /<h3 class="skill-card__title" :title="item\.name">\{\{ item\.name \}\}<\/h3>/)
  assert.match(source, /class="skill-card__tags"/)
})

test('space admins can hide a catalog skill from the workspace (000108)', () => {
  // 管理页 include_hidden 拉全量（隐藏的可改回）；卡片给可见开关与隐藏标；
  // 隐藏技能从 Skills/MCP 浏览、智能体选技能与引用/运行时消失。
  assert.match(source, /listSkillCatalog\(true\)/)
  assert.match(source, /v-if="item\.visible === false" class="skill-card__hidden"/)
  assert.match(source, /item\.visible !== false/)
  assert.match(source, /updateSkillCatalogMeta\(item\.id, item\.category \|\| '', visible\)/)
  assert.match(i18n, /hiddenInSpace: '本空间内隐藏'/)
  assert.match(i18n, /hideFromSpace: '已在本空间隐藏，将不出现在 Skills\/MCP 列表与智能体引用中'/)
})

test('the catalog API surfaces visibility and include_hidden (000108)', () => {
  assert.match(skillApi, /visible\?: boolean/)
  assert.match(skillApi, /include_hidden: '1'/)
  assert.match(skillApi, /listSkillCatalog\(includeHidden = false\)/)
})
