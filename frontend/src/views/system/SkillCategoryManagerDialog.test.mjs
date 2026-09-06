import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./SkillCategoryManagerDialog.vue', import.meta.url), 'utf8')
const i18n = readFileSync(new URL('../../i18n/locales/zh-CN.ts', import.meta.url), 'utf8')

// 分类管理弹窗（000105）：分类是独立登记表，先创建再使用 —— 弹窗顶部是新建
// 入口，列表展示已登记分类（含用量计数），重命名 / 删除管理登记行与引用它的
// 技能。改动点亮 drift，由面板重拉体现。
test('the dialog creates a category first (create-first)', () => {
  assert.match(source, /createSkillCategory/)
  assert.match(source, /categoryManagerCreate/)
  assert.match(source, /categoryManagerNewPlaceholder/)
  assert.match(source, /categoryManagerEmpty/)
  assert.match(source, /skillLibrary\.toasts\.categoryCreated/)
  assert.match(source, /newName\.value = ''/)
  assert.match(source, /@enter="commitCreate"/)
})

test('the dialog lists the registry with counts', () => {
  assert.match(source, /listSkillCategories/)
  assert.match(source, /PlatformSkillCategoryCount\[\]/)
  assert.match(source, /skillLibrary\.categoryManagerCount/)
})

test('rename and remove are bulk operations on registry + skills', () => {
  assert.match(source, /renameSkillCategory\(from, to\)/)
  assert.match(source, /removeSkillCategory\(row\.name\)/)
  // 删除需确认（置空影响整组技能），重命名是行内编辑
  assert.match(source, /confirmDelete\(/)
  assert.match(source, /skillLibrary\.categoryManagerRemoveConfirmBody/)
  assert.match(source, /skillLibrary\.categoryManagerRenameTo/)
  // 操作落地后面板重拉卡片（分类 pill 与 drift 点）
  assert.match(source, /emit\('changed'\)/)
})

test('the hint and empty copy explain create-then-use', () => {
  assert.match(source, /skillLibrary\.categoryManagerHint/)
  assert.match(i18n, /categoryManagerTitle: '技能分类管理'/)
  assert.match(i18n, /categoryManagerEmpty: '暂无分类。输入名称创建第一个分类。'/)
  assert.match(i18n, /categoryCreated: '分类已创建'/)
  assert.match(i18n, /categoryRenamed: '已重命名 \{count\} 个技能的分类'/)
  assert.match(i18n, /categoryRemoved: '已将 \{count\} 个技能移出分类'/)
})
