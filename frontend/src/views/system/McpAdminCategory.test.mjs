import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const dialog = readFileSync(new URL('./McpServiceDialog.vue', import.meta.url), 'utf8')
const list = readFileSync(new URL('./McpSettings.vue', import.meta.url), 'utf8')
const i18n = readFileSync(new URL('../../i18n/locales/zh-CN.ts', import.meta.url), 'utf8')

test('the MCP editor dialog owns a category field that is read-only for builtin rows', () => {
  // category 随主表单提交（buildPayload），builtin 行后端拒绝更新 → 输入禁用。
  assert.match(dialog, /v-model="formData\.category"/)
  assert.match(dialog, /:disabled="!!props\.service\?\.is_builtin"/)
  assert.match(dialog, /category: formData\.value\.category\.trim\(\)/)
  // 编辑回填 + 重置 + 初始值三处都要有，否则编辑一次后脏值漂到新增
  assert.match(dialog, /category: service\.category \|\| ''/)
  assert.match(dialog, /category: '',/)
})

test('the console list cards show the category next to the description', () => {
  assert.match(list, /v-if="service\.category"/)
  assert.match(list, /:title="service\.category">\{\{ service\.category \}\}/)
})

test('mcpServiceDialog i18n block carries the category strings', () => {
  assert.match(i18n, /category: '分类'/)
  assert.match(i18n, /categoryPlaceholder: 'Skills\/MCP 浏览页的分组，留空为未分类'/)
})
