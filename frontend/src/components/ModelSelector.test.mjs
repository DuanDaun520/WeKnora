import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./ModelSelector.vue', import.meta.url), 'utf8')

test('a selected model that is filtered out still renders by name', () => {
  // 000102：类型过滤或列表加载时序会把选中 id 剔除出选项，t-select 随即
  // 把 UUID 当文案显示。从 rawModels（未过滤源）把选中项补回选项尾部；
  // 源已到齐仍找不到（模型已删除）时落「未知模型」占位，绝不裸显 id。
  assert.match(source, /const rawModels = ref<ModelConfig\[\]>\(\[\]\)/)
  assert.match(source, /const optionModels = computed<ModelConfig\[\]>/)
  assert.match(source, /v-for="model in optionModels"/)
  assert.match(source, /const found = rawModels\.value\.find\(m => m\.id === id\)/)
  assert.match(source, /if \(found\) return \[\.\.\.models\.value, found\]/)
  assert.match(source, /if \(rawModels\.value\.length > 0\)/)
  assert.match(source, /unknownModel/)
})
