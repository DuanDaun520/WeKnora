import assert from 'node:assert/strict'
import { test } from 'node:test'

import { BUILT_IN_DEFAULT, SUPPORTED_LOCALES, resolveDefaultLocale } from './resolveDefaultLocale.ts'

// 单语言部署（企业版只保留简体中文）：历史多语言标签一律回落到
// zh-CN，而不是继续把 en-US 等当作合法运行时覆盖。
test('resolveDefaultLocale accepts the only supported locale', () => {
  assert.equal(resolveDefaultLocale('zh-CN', BUILT_IN_DEFAULT), 'zh-CN')
})

test('resolveDefaultLocale falls back to build-time then built-in default', () => {
  assert.equal(resolveDefaultLocale('', 'zh-CN'), 'zh-CN')
  assert.equal(resolveDefaultLocale(undefined, undefined), BUILT_IN_DEFAULT)
  assert.deepEqual([...SUPPORTED_LOCALES], ['zh-CN'])
})

test('resolveDefaultLocale trims whitespace around supported tags', () => {
  assert.equal(resolveDefaultLocale(' zh-CN '), 'zh-CN')
})

test('resolveDefaultLocale rejects unknown and legacy multi-locale values', () => {
  assert.equal(resolveDefaultLocale('fr-FR'), BUILT_IN_DEFAULT)
  assert.equal(resolveDefaultLocale('en-US'), BUILT_IN_DEFAULT)
  assert.equal(resolveDefaultLocale('ko-KR'), BUILT_IN_DEFAULT)
  assert.equal(resolveDefaultLocale('ru-RU'), BUILT_IN_DEFAULT)
  assert.equal(resolveDefaultLocale('zh-CN"};alert(1);//'), BUILT_IN_DEFAULT)
  assert.equal(resolveDefaultLocale('   '), BUILT_IN_DEFAULT)
})
