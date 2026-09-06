import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./AgentEditorModal.vue', import.meta.url), 'utf8')

test('editing an agent closes the editor after a successful save', () => {
  assert.match(
    source,
    /await updateAgent\(formData\.value\.id, formData\.value\);\s*MessagePlugin\.success\(t\('agent\.messages\.updated'\)\);\s*emit\('success'\);\s*handleClose\(\);/
  )
})

test('the first successful create stays open for integration setup', () => {
  const createBranch = source.match(
    /if \(editorMode\.value === 'create'\) \{([\s\S]*?)^\s{4}\} else \{/m
  )?.[1]

  assert.ok(createBranch, 'expected to find the create branch')
  assert.doesNotMatch(createBranch, /handleClose\(\)/)
  assert.match(createBranch, /savedAgent\.value = created;/)
})

test('save button labels distinguish create from save-and-close', () => {
  assert.match(
    source,
    /const saveButtonLabel = computed\(\(\) =>\s*editorMode\.value === 'create'\s*\? t\('agent\.editor\.buttons\.create'\)\s*: t\('agent\.editor\.buttons\.saveAndClose'\)\s*\)/
  )
  assert.match(source, /saveButtonLabel/)
})

test('shows a post-create hint after the first successful save', () => {
  assert.match(source, /const isPostCreateSession = computed\(\(\) => !!savedAgent\.value\)/)
  assert.match(source, /settings-footer-note/)
  assert.match(source, /agent\.editor\.postCreateHint\.title/)
})

// Locate a settings row by the i18n key of its label and return the attributes
// that sit before class="setting-row" — i.e. whatever v-if guards that row.
// Anchoring on the label keeps these assertions stable across reformatting.
function settingRowGuard(labelKey) {
  const key = labelKey.replace(/\./g, '\\.')
  const row = source.match(
    new RegExp(
      `<div([^>]*)class="setting-row">\\s*<div class="setting-info">\\s*<label>\\{\\{ \\$t\\('${key}'\\) \\}\\}`
    )
  )
  assert.ok(row, `expected to find the ${labelKey} row`)
  return row[1]
}

test('conversation settings stay reachable in smart-reasoning mode', () => {
  // The agent path reads history_turns (session_agent_qa.go -> LoadAgentHistory),
  // so the section must not be gated on the running mode.
  assert.match(source, /v-show="currentSection === 'conversation'"/)
  assert.doesNotMatch(source, /currentSection === 'conversation' && !isAgentMode/)

  // The nav entry is pushed unconditionally; a re-introduced guard would indent it.
  const navItems = source.match(/const navItems = computed\(\(\) => \{([\s\S]*?)^\}\);/m)?.[1]
  assert.ok(navItems, 'expected to find the nav items computed')
  assert.match(navItems, /^  items\.push\(\{ key: 'conversation'/m)

  // Switching to agent mode must not evict the user from the section.
  // Tolerates the watch being dropped entirely by a later cleanup.
  const modeWatch = source.match(/watch\(isAgentMode, \(isAgent\) => \{([\s\S]*?)^\}\);/m)?.[1] ?? ''
  assert.doesNotMatch(modeWatch, /conversation/)
})

test('history turns is editable in smart-reasoning mode', () => {
  // A new agent defaults to smart-reasoning with multi_turn_enabled=false in the
  // form, while the server forces multi-turn on. Gating the input on the local
  // switch alone would hide it exactly where it is needed.
  const guard = settingRowGuard('agent.editor.historyTurns')
  assert.match(guard, /formData\.config\.multi_turn_enabled \|\| isAgentMode/)
})

test('multi-turn switch stays hidden in smart-reasoning mode', () => {
  // CustomAgent.EnsureDefaults pins MultiTurnEnabled to true for smart-reasoning,
  // so an editable switch would silently revert after save.
  const guard = settingRowGuard('agent.editor.multiTurn')
  assert.match(guard, /v-if="!isAgentMode"/)
})

test('query rewrite stays hidden in smart-reasoning mode', () => {
  // enable_rewrite is consumed by the KnowledgeQA pipeline only.
  const guard = settingRowGuard('agent.editor.enableRewrite')
  assert.match(guard, /!isAgentMode/)
})

test('the section description matches what the mode actually shows', () => {
  // The default copy mentions query rewriting, which agent mode does not show.
  assert.match(source, /isAgentMode\.value\s*\?\s*t\('agentEditor\.desc\.conversationSectionAgent'\)/)
  assert.match(source, /<p class="section-description">\{\{ conversationSectionDesc \}\}<\/p>/)
})

test('history turns can be raised beyond the old 20 cap', () => {
  // A 5-to-20 range sits two orders of magnitude below the 200k token budget
  // the agent engine already manages, so the turn cap always bites first.
  const input = source.match(/<t-input-number v-model="formData\.config\.history_turns"[^>]*>/)
  assert.ok(input, 'expected to find the history turns input')
  assert.match(input[0], /:max="100"/)
})

test('retrieval retention is offered to agents that actually have a knowledge base', () => {
  // retain_retrieval_history is read only by the agent path
  // (internal/agent/observe.go) and only rewrites KB/Wiki tool results, so it
  // is meaningless without a knowledge base.
  const guard = settingRowGuard('agent.editor.retainRetrievalHistory')
  assert.match(guard, /isAgentMode && hasKnowledgeBase/)
})

test('skills and sandbox share one editor section', () => {
  const navItems = source.match(/const navItems = computed\(\(\) => \{([\s\S]*?)^\}\);/m)?.[1]
  assert.ok(navItems, 'expected to find the nav items computed')
  assert.match(navItems, /key: 'skills'/)
  assert.match(navItems, /icon: SKILL_ICON/)
  assert.doesNotMatch(navItems, /key: 'sandbox'/)

  const capabilityGroup = source.match(/pickItems\(\['knowledge', 'skills', 'mcp', 'websearch', 'multimodal'\]\)/)
  assert.ok(capabilityGroup, 'expected the capability group to list skills without a separate sandbox tab')

  assert.match(source, /v-show="currentSection === 'skills' && isAgentMode"/)
  assert.doesNotMatch(source, /currentSection === 'sandbox' && isAgentMode/)
  assert.match(source, /sandbox: 'skills'/)
  assert.match(source, /formData\.config\.sandbox_config_id/)
  assert.match(source, /:disabled="!canEnableSkills"/)
  assert.match(source, /sandbox-option/)
  assert.doesNotMatch(source, /skill-info-box/)
})

test('nav groups follow the basic / capability / advanced taxonomy', () => {
  // 左侧菜单三类：基础配置（含问题推荐与多轮对话）、能力扩展（知识库/
  // Skills/MCP/搜索/附件）、高级配置（检索策略/工具）。检索策略挂在
  // 知识库上才出现，归高级配置组；发布保持独立末组。
  const groups = source.match(/const navGroups = computed\(\(\) => \{([\s\S]*?)^\}\);/m)?.[1]
  assert.ok(groups, 'expected to find the nav groups computed')
  assert.match(groups, /pickItems\(\['basic', 'prompts', 'model', 'suggestions', 'conversation'\]\)/)
  assert.match(groups, /pickItems\(\['knowledge', 'skills', 'mcp', 'websearch', 'multimodal'\]\)/)
  assert.match(groups, /pickItems\(\['retrieval', 'tools'\]\)/)
  assert.match(groups, /pickItems\(\['share'\]\)/)
  assert.doesNotMatch(groups, /navGroups\.knowledge/)
  assert.match(groups, /navGroups\.advanced/)
})

test('the publish-channel row is hidden from basic info', () => {
  // 000101：基本信息里的「发布渠道」（IM/嵌入渠道计数）不再渲染——
  // 集成入口收敛到集成中心；计数加载函数保留在脚本侧备用。
  assert.doesNotMatch(source, /integrations\.agentEditor\.label/)
  assert.doesNotMatch(source, /integration-inline__stat/)
  assert.match(source, /loadAgentIntegrationCounts/)
})

test('skill install actions follow the space-admin-or-system-admin gate', () => {  // 000107：技能安装后端放宽到 AdminOrSystemAdmin，编辑器内直装按钮与「管理技能」
  // canInstallSkills = 本空间 admin/owner 或系统管理员（hasRole 对系统管理员旁路）；
  // openSkillSettings 直指空间 Settings 的 skill-catalog 并带沙箱预选。
  assert.match(source, /canInstallSkills = computed\(\(\) => authStore\.hasRole\('admin'\)\)/)
  assert.match(source, /openSettings\('skill-catalog'/)
})

test('read-only viewers get a view title and a non-operable form', () => {
  // 000102：普通用户打开他人智能体——标题切「查看智能体」，内容区加
  // is-readonly。禁的是所有后代而非容器本身（容器是滚动容器，挂自己会
  // 连滚轮一起废掉）；prompts 内层滚动容器单独恢复命中。
  assert.match(
    source,
    /props\.readOnly \? \$t\('agent\.editor\.viewTitle'\) : \$t\('agent\.editor\.editTitle'\)/,
  )
  assert.match(source, /'is-readonly': props\.readOnly/)
  assert.match(source, /:deep\(\*\) \{\s*pointer-events: none !important;/)
  assert.match(source, /:deep\(\.prompts-panel__body\) \{\s*pointer-events: auto !important;/)
})

test('editor dependencies load independently instead of failing together', () => {
  // 000102：原先是单个 Promise.all——任何一路 403 都会让模型/KB/搜索
  // 引擎列表全部保持空，查看态下拉只剩 UUID。改 allSettled 互不连坐。
  // （设置弹窗关闭后的 watch 里那个 Promise.all 是局部刷新，已 try/catch，
  // 不在约束范围内。）
  const loadDeps = source.match(/const loadDependencies = async \(\) => \{([\s\S]*?)^\};/m)?.[1]
  assert.ok(loadDeps, 'expected the loadDependencies body')
  assert.match(loadDeps, /await Promise\.allSettled\(/)
  assert.doesNotMatch(loadDeps, /Promise\.all\(/)
})

test('referenced entities fall back to names instead of raw ids', () => {
  // 000102：普通用户要看到名称而不是 UUID——
  //  1) KB：agent 引用但被权限过滤掉的库，经 agent 维度 KB 接口补齐，
  //     单独分组「智能体引用的知识库」，仍缺失的落占位文案；
  //  2) 搜索引擎：选中 provider 不在列表时补占位选项；
  //  3) 模型：ModelSelector 从未过滤源里补回被类型过滤剔除的选中项。
  assert.match(source, /async function hydrateAgentReferencedKbs\(agentId: string\)/)
  assert.match(source, /chatResources\.ensureAgentKnowledgeBases\(agentId/)
  assert.match(source, /unknownKnowledgeBase/)
  assert.match(source, /referencedKnowledgeBases/)
  assert.match(source, /filteredHydratedKbOptions/)
  assert.match(source, /const webSearchProviderOptions = computed/)
  assert.match(source, /v-for="p in webSearchProviderOptions"/)
  assert.match(source, /webSearchProvidersLoaded/)
})

test('sandbox management links split console mgmt from skill installs (000107)', () => {
  // 「管理沙箱」指向平台控制台的沙箱连接，仍仅系统管理员（canManageSandboxConsole）；
  // 「管理技能」指向本空间技能目录，本空间 admin/owner 以上（canInstallSkills）可见；
  // 两者都不满足才显示由管理员统一配置的提示。
  assert.match(source, /const canManageSandboxConsole = computed\(\(\) => authStore\.isSystemAdmin\)/)
  assert.match(source, /v-if="canManageSandboxConsole"[\s\S]*?uiStore\.openSettings\('sandbox'\)/)
  assert.match(source, /v-else class="desc">\{\{ \$t\('agent\.editor\.sandboxManagedHint'\) \}\}/)
  assert.match(source, /@click\.prevent="openSkillSettings"/)
})

test('creating an agent defaults to quick-answer with an empty name', () => {
  // 000100：新建默认「快速问答」，名称/描述不再按 agent_type 预填——
  // 创建分支里不应再有 getPresetDefaultName 的自动填充。
  assert.match(source, /agent_mode: 'quick-answer' as 'quick-answer' \| 'smart-reasoning'/)
  assert.doesNotMatch(source, /if \(!formData\.value\.name\) \{\s*formData\.value\.name = getPresetDefaultName\(preset\);/)
  assert.doesNotMatch(source, /if \(!formData\.value\.description\) \{\s*formData\.value\.description = getPresetDefaultDescription\(preset\);/)
})

test('picking a sandbox defaults skills to selected with every ready skill checked', () => {
  // 000100：下拉 @change 触发默认勾选：模式进「指定」并把该沙箱全部
  // selectable 技能并入 selected_skills；仅用户显式选择触发，编辑态
  // 加载已保存配置不改写「禁用」意图。
  assert.match(source, /@change="onSandboxSelected"/)
  assert.match(source, /let pendingDefaultSkillCheck = false/)
  assert.match(source, /function applyDefaultSkillSelection\(\)/)
  assert.match(source, /skillsSelectionMode\.value = 'selected'/)
  assert.match(source, /Array\.from\(new Set\(\[\.\.\.current, \.\.\.names\]\)\)/)
  // 目录兜底挂在既有的 [visible, catalogSkillRows] watch 里——注册点必须在
  // formData（L2835）之后：单独的 watch(catalogSkillRows) 若写在 setup 前段，
  // 注册时立即求值 computed 会触发 formData 的 TDZ ReferenceError，导致整个
  // AgentList 页面加载即坏、所有弹窗打不开（000100 回归）。
  assert.match(
    source,
    /\[\(\) => props\.visible, catalogSkillRows\],\s*\(\) => \{\s*(?:\/\/[^\n]*\n\s*)?applyDefaultSkillSelection\(\)/,
  )
  assert.doesNotMatch(source, /watch\(catalogSkillRows, \(\) => \{/)
})

test('agent skill picker uses the catalog and only enables ready installs', () => {
  assert.match(source, /function autoBindSoleSandbox\(/)
  assert.match(source, /canEnableSkills/)
  assert.match(source, /catalogSkillRows/)
  assert.match(source, /showCatalogSkillList/)
  assert.match(source, /skillsSelectionMode\.value !== 'none'/)
  assert.match(source, /:disabled="!skill\.selectable"/)
  assert.match(source, /catalogSkillGroups/)
  assert.match(source, /installPartial/)
  assert.match(source, /installCatalogToCurrent/)
  assert.match(source, /agent\.editor\.installToThisSandbox/)
  assert.match(source, /skill-pick-list/)
  assert.match(source, /skill-pick-group/)
  assert.match(source, /skill-pick__badge/)
  assert.match(source, /skillStatusIcon/)
  assert.match(source, /isSkillBusy/)
  assert.match(source, /viewInstallProgress/)
  assert.match(source, /openSkillInstallProgress/)
  assert.doesNotMatch(source, /await openSkillInstallProgress\(skill\)/)
  assert.match(source, /SandboxSkillsPanel/)
  assert.match(source, /focus-skill-id/)
  assert.match(source, /skillsGroupUnavailable/)
  // 000100：技术摘要行（Docker · 镜像 · 描述）与下拉里的 target 行不再渲染。
  assert.doesNotMatch(source, /selectedSandboxSummary/)
  assert.doesNotMatch(source, /sandboxTargetLine/)
  assert.doesNotMatch(source, /sandbox-option__target/)
  assert.match(source, /line-clamp: 2/)
  assert.doesNotMatch(source, /skillsSelectionMode === 'selected' && catalogSkillRows/)
  assert.doesNotMatch(source, /skill-list-summary/)
  assert.doesNotMatch(source, /skill-ready-stat/)
})
