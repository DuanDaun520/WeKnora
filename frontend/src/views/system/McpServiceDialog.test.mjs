import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./McpServiceDialog.vue', import.meta.url), 'utf8')

// 必填校验兜底：本抽屉的字段全部走自定义 .form-item div（无 <t-form-item
// name> 注册目标），TDesign 的 rules/validate() 恒通过——曾放行过 name/url
// 全空的 POST，落库后运行时报 "URL is required for SSE transport"、agent
// 零 MCP 工具注册。保存/授权前必须走手动校验。
test('required fields are validated manually before submit', () => {
  assert.match(source, /function validateRequiredFields\(\): boolean/)
  assert.match(source, /if \(!validateRequiredFields\(\)\) return/)
  // 校验覆盖 name 必填与 URL 必填/合法两段
  assert.match(source, /rules\.nameRequired/)
  assert.match(source, /rules\.urlRequired/)
  assert.match(source, /rules\.urlInvalid/)
  assert.match(source, /new URL\(url\)/)
  // 死规则对象与绑定应移除（保留会误导后续维护者以为校验在生效）
  assert.doesNotMatch(source, /const rules/)
  assert.doesNotMatch(source, /:rules="rules"/)
})
