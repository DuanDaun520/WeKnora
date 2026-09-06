import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./useResourcePins.ts', import.meta.url), 'utf8')
const favoritesApi = readFileSync(new URL('../api/user-favorites.ts', import.meta.url), 'utf8')

test('the pinnable resource types cover kb/agent/skill/mcp via a single list', () => {
  // Skills/MCP 页面把技能与 MCP 服务接进同一套收藏/最近机制。全部类型
  // 由 ALL_RESOURCE_TYPES 单点定义，per-type 状态（favoritesByType/
  // loaded/inFlight）从这里派生，杜绝「某处硬编码漏加新类型」这类漂移。
  assert.match(source, /const ALL_RESOURCE_TYPES = \['kb', 'agent', 'skill', 'mcp'\] as const/)
  assert.match(source, /ALL_RESOURCE_TYPES\.map\(\(t\) => \[t, ref<PinEntry\[\]>\(\[\]\)\]\)/)
  assert.match(source, /ALL_RESOURCE_TYPES\.flatMap\(\(t\) => favoritesByType\[t\]\.value\)/)
  // 租户切换重置、惰性首拉、refresh 三处也必须走同一个列表
  const resetLoops = (source.match(/for \(const t of ALL_RESOURCE_TYPES\)/g) || []).length
  assert.ok(resetLoops >= 4, `expected ALL_RESOURCE_TYPES loops in watcher/lazy-load/refresh, got ${resetLoops}`)
})

test('localStorage recents validation accepts every type from the list', () => {
  // readRecents 旧的 'kb'||'agent' 双字面量过滤已被列表 contains 取代，
  // 否则 skill/mcp 的最近记录会被静默丢弃。
  assert.match(source, /\(ALL_RESOURCE_TYPES as readonly string\[\]\)\.includes\(\(e as PinEntry\)\.type\)/)
  assert.doesNotMatch(source, /=== 'kb' \|\| \(e as PinEntry\)\.type === 'agent'/)
})

test('the favorites API type union includes skill and mcp', () => {
  // 与后端 types.IsValidFavoriteResourceType 白名单保持同步。
  assert.match(favoritesApi, /export type FavoriteResourceType = 'kb' \| 'agent' \| 'skill' \| 'mcp'/)
})
