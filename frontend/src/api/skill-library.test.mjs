import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./skill-library.ts', import.meta.url), 'utf8')

// 平台技能库 API（000104/000105 增量）：注册只走 zip 上传（从源拉取已随界面
// 下线），分类/作者元数据随注册可带，独立 meta 端点编辑。分类是独立登记表
// （000105）：先创建（createSkillCategory），技能端只能引用已登记的分类。
test('registration is zip-only and carries category/author/zh metadata', () => {
  assert.match(source, /category\?: string/)
  assert.match(source, /author\?: string/)
  assert.match(source, /zh_name\?: string/)
  assert.match(source, /zh_description\?: string/)
  // 空串恒发送：category/author 后端当「回落 SKILL.md frontmatter」，
  // zh_name/zh_description 空串 = 无中文展示（列表回落 SKILL.md 内容）。
  assert.match(source, /form\.append\('category', category \|\| ''\)/)
  assert.match(source, /form\.append\('author', author \|\| ''\)/)
  assert.match(source, /form\.append\('zh_name', zhName \|\| ''\)/)
  assert.match(source, /form\.append\('zh_description', zhDescription \|\| ''\)/)
  // 签名跨行：逐参数断言
  assert.match(source, /category\?: string,\n  author\?: string,\n  zhName\?: string,\n  zhDescription\?: string/)
  // 「从源安装」封装已删除
  assert.doesNotMatch(source, /FromSource/)
})

test('meta and category endpoints are wrapped', () => {
  assert.match(source, /updatePlatformSkillMeta/)
  assert.match(source, /`\$\{BASE\}\/\$\{id\}\/meta`/)
  assert.match(source, /listSkillCategories/)
  assert.match(source, /`\$\{BASE\}\/categories`/)
  assert.match(source, /createSkillCategory/)
  assert.match(source, /post\(`\$\{BASE\}\/categories`, \{ name \}\)/)
  assert.match(source, /renameSkillCategory/)
  assert.match(source, /`\$\{BASE\}\/categories\/rename`/)
  assert.match(source, /removeSkillCategory/)
  assert.match(source, /`\$\{BASE\}\/categories\/remove`/)
  // PlatformSkill 投影带元数据字段（000106：含中文名/描述）
  assert.match(source, /category\?: string/)
  assert.match(source, /author\?: string/)
  assert.match(source, /zh_name\?: string/)
  assert.match(source, /zh_description\?: string/)
})

test('bulk ops report how many skills they touched', () => {
  assert.match(source, /response\?\.renamed \?\? 0/)
  assert.match(source, /response\?\.cleared \?\? 0/)
})
