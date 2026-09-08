import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./SkillsMcpList.vue', import.meta.url), 'utf8')
const i18n = readFileSync(new URL('../../i18n/locales/zh-CN.ts', import.meta.url), 'utf8')

test('sidebar hides the workspace/org sections and counts both tabs together', () => {
  // 技能与 MCP 都是空间级资源，没有「本空间 / 协作空间」维度；
  // hide-workspace 是 P7 给 ListSpaceSidebar 加的开关。
  assert.match(source, /<ListSpaceSidebar[^>]*hide-workspace/)
  assert.match(source, /:count-all="totalCount"/)
  assert.match(source, /:count-favorites="favoritesCount"/)
  assert.match(source, /:count-recents="recentsCount"/)
  assert.match(source, /e\.type === 'skill' \|\| e\.type === 'mcp'/)
})

test('scope + favorites/recents hydration follows the AgentList pattern', () => {
  assert.match(source, /useListUrlState\(\{ defaultScope: 'all' \}\)/)
  assert.match(source, /useResourcePins\(\)/)
  // 收藏/最近用 pins 的 id 对内存索引水合
  assert.match(source, /pins\.favorites\.value\s*\n?\s*\.filter\(\(e\) => e\.type === type\)/)
  assert.match(source, /pins\.recents\.value\s*\n?\s*\.filter\(\(e\) => e\.type === type\)/)
  assert.match(source, /pins\.touchRecent\('skill', id\)/)
  assert.match(source, /pins\.touchRecent\('mcp', id\)/)
  assert.match(source, /pins\.toggleFavorite\(type, id\)/)
})

test('both lists load in parallel and refresh on tenant switch', () => {
  assert.match(source, /listSkillCatalog\(\)/)
  assert.match(source, /listMCPServices\(\)/)
  assert.match(source, /Promise\.all\(/)
  assert.match(source, /watch\(\(\) => authStore\.currentTenantId/)
  assert.match(source, /chatResources\.ensureAgents\(\)/)
})

test('referencing-agent reverse join uses the documented config rules', () => {
  // 仅 smart-reasoning；MCP 按 id + mcp_selection_mode；Skill 按名称 +
  // skills_selection_mode；'none'/'' 均视为未引用。
  assert.match(source, /cfg\.agent_mode !== 'smart-reasoning'/)
  assert.match(source, /cfg\.mcp_selection_mode \|\| ''/)
  assert.match(source, /mode === 'all'\) return true/)
  assert.match(source, /if \(mode !== 'selected'\) return false/)
  assert.match(source, /\(cfg\.mcp_services \|\| \[\]\)\.includes\(key\)/)
  assert.match(source, /cfg\.skills_selection_mode \|\| ''/)
  assert.match(source, /\(cfg\.selected_skills \|\| \[\]\)\.includes\(key\)/)
  // 详情弹窗渲染引用智能体 chip，点击跳智能体页
  assert.match(source, /detailReferencingAgents/)
  assert.match(source, /router\.push\('\/platform\/agents'\)/)
})

test('category chips derive from live data and both tabs keep separate filters', () => {
  assert.match(source, /const skillCategory = ref\(CATEGORY_ALL\)/)
  assert.match(source, /const mcpCategory = ref\(CATEGORY_ALL\)/)
  assert.match(source, /deriveCategories\(/)
  // 空分类归入「未分类」chip
  assert.match(source, /if \(list\.includes\(''\)\) out\.push\(''\)/)
  assert.match(source, /cat === '' \? \$t\('skillsMcp\.uncategorized'\)/)
})

test('detail dialog: info rows, usage hint, lazy MCP tools, admin category edit', () => {
  assert.match(source, /getMCPServiceTools\(id\)/)
  assert.match(source, /isSkillInstalled\(/)
  assert.match(source, /i\.status === 'ready' && i\.enabled/)
  // SystemAdmin 才能改分类；MCP builtin 行后端拒绝更新 → 隐藏入口
  assert.match(source, /authStore\.isSystemAdmin/)
  assert.match(source, /!detailMcp\.value\.is_builtin/)
  assert.match(source, /updateSkillCatalogMeta\(detailSkill\.value\.id, cat\)/)
  assert.match(source, /updateMCPService\(detailMcp\.value\.id, \{ category: cat \} as Partial<MCPService>\)/)
})

test('synthetic catalog rows (no bundle) hide the pin star', () => {
  // 源 catalog 已删、由安装行派生的合成行 id 不稳定，收藏会永远水合
  // 不出来——不给点星兜底。
  assert.match(source, /function canPinSkill\(item: SkillCatalogItem\): boolean/)
  assert.match(source, /!!item\.bundle_sha256 \|\| !!item\.source_platform_skill_id/)
  assert.match(source, /v-if="canPinSkill\(item\)"/)
})

test('skill detail dialog shows the author row when present (000104)', () => {
  // 作者来自 SKILL.md frontmatter 或平台推送；空值不占行。
  assert.match(source, /v-if="detailSkill\.author"/)
  assert.match(source, /\$t\('skillsMcp\.author'\)/)
})

test('skill cards show the author, not the creator (000110)', () => {
  // 推送场景下创建者恒为"系统"，没有信息量；技能卡片 footer 只显示作者，
  // 空值不占位。MCP 卡片保留创建者。
  const skillTab = source.match(/<template v-if="activeTab === 'skills'">([\s\S]*?)<template v-else>/)?.[1]
  assert.ok(skillTab, 'expected the skills tab block')
  assert.match(skillTab, /v-if="item\.author" class="creator"\>\{\{ item\.author \}\}/)
  assert.doesNotMatch(skillTab, /creator_name/)
})

test('detail dialog: install status rides the title, meta rows move to the tail (000110)', () => {
  // 安装状态徽标紧跟技能英文名，旁边是复制名称按钮；已安装沙箱/分类/
  // 创建者/创建时间后置到「更多信息」区（基本信息只剩版本/作者/帮助网址）。
  assert.match(source, /<template #header>/)
  assert.match(source, /class="detail-title-name"/)
  assert.match(source, /:class="isSkillInstalled\(detailSkill\) \? 'ok' : 'off'"/)
  assert.match(source, /@click="copyDetailName"/)
  assert.match(source, /navigator\.clipboard\.writeText\(name\)/)
  assert.match(source, /\$t\('skillsMcp\.otherInfo'\)/)
  // 尾部元信息区包含已安装沙箱/分类/创建者/创建时间
  const tail = source.match(/\$t\('skillsMcp\.otherInfo'\)[\s\S]*?<\/div>\s*<\/div>\s*<\/div>\s*<\/t-dialog>/)?.[0]
  assert.ok(tail, 'expected the trailing meta section')
  for (const key of ['installedOn', 'category', 'creator', 'createdAt']) {
    assert.match(tail, new RegExp(`skillsMcp\\.${key}`), `tail section missing ${key}`)
  }
})

test('detail dialog renders the help URL as an external link (000109)', () => {
  assert.match(source, /v-if="detailSkill\.help_url"/)
  assert.match(source, /:href="detailSkill\.help_url"/)
  assert.match(source, /target="_blank"/)
  assert.match(source, /rel="noopener noreferrer"/)
  assert.match(source, /helpUrlHost/)
})

test('i18n carries the full skillsMcp block used by the page', () => {
  assert.match(i18n, /skillsMcp: \{/)
  for (const key of [
    'title', 'subtitle', 'searchPlaceholder', 'categoryAll', 'uncategorized',
    'countLabel', 'installed', 'enabled', 'builtin', 'referencingAgents',
    'noReferencingAgents', 'editCategory', 'usageHint', 'basicInfo', 'usage',
    'author',
  ]) {
    assert.match(i18n, new RegExp(` ${key}: `), `missing skillsMcp.${key}`)
  }
  assert.match(i18n, /tabs: \{\s*skills: 'Skills',\s*mcp: 'MCP'\s*\}/)
})
