import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const inputField = readFileSync(new URL('./Input-field.vue', import.meta.url), 'utf8')
const agentSelector = readFileSync(new URL('./AgentSelector.vue', import.meta.url), 'utf8')
const settingsStore = readFileSync(new URL('../stores/settings.ts', import.meta.url), 'utf8')

test('selecting an agent leaves web search off until the user enables it', () => {
  const selectAgentStart = settingsStore.indexOf('selectAgent(agentId: string')
  const getSelectedAgentStart = settingsStore.indexOf('getSelectedAgentId()', selectAgentStart)
  const selectAgentAction = settingsStore.slice(selectAgentStart, getSelectedAgentStart)

  assert.notEqual(selectAgentStart, -1)
  assert.notEqual(getSelectedAgentStart, -1)
  assert.match(selectAgentAction, /this\.settings\.webSearchEnabled = false/)

  const handleSelectAgentStart = inputField.indexOf('const handleSelectAgent = async')
  const handleSelectAgentEnd = inputField.indexOf('const clearvalue', handleSelectAgentStart)
  const handleSelectAgent = inputField.slice(handleSelectAgentStart, handleSelectAgentEnd)

  assert.notEqual(handleSelectAgentStart, -1)
  assert.notEqual(handleSelectAgentEnd, -1)
  assert.match(handleSelectAgent, /settingsStore\.selectAgent\(agent\.id, sourceTenantId\)/)
  assert.doesNotMatch(handleSelectAgent, /agentWebSearch/)
  assert.doesNotMatch(handleSelectAgent, /settingsStore\.toggleWebSearch/)
})

test('shared-agent web search button waits for source readiness metadata', () => {
  const showWebSearchStart = inputField.indexOf('const showWebSearchButton = computed')
  const showWebSearchEnd = inputField.indexOf('const showImageUploadButton', showWebSearchStart)
  const showWebSearchButton = inputField.slice(showWebSearchStart, showWebSearchEnd)

  assert.notEqual(showWebSearchStart, -1)
  assert.notEqual(showWebSearchEnd, -1)
  assert.match(showWebSearchButton, /isWebSearchReadinessKnown/)
  assert.match(showWebSearchButton, /selectedSharedAgent\.value\?\.web_search_ready/)
})

test('switching agents keeps the user KB selection instead of auto-selecting agent KBs', () => {
  const watchStart = inputField.indexOf('watch([selectedAgentId, agentKnowledgeBases, agentKBSelectionMode]')
  const watchEnd = inputField.indexOf('}, { immediate: true });', watchStart)
  const agentSwitchWatch = inputField.slice(watchStart, watchEnd)

  assert.notEqual(watchStart, -1)
  assert.notEqual(watchEnd, -1)
  // 不再把智能体配置的 KB 列表整体写入选中集合
  assert.doesNotMatch(agentSwitchWatch, /\.\.\.newAgentKbs/)
  // 仅收敛：'selected' 模式保留与允许范围的交集，'none' 清空
  assert.match(agentSwitchWatch, /allowed\.has\(String\(id\)\)/)
})

test('agent picker is a centered dialog instead of an anchored dropdown', () => {
  // 弹窗结构：遮罩居中 + dialog 卡片 + 左右两列（列表 / 详情内联）
  assert.match(agentSelector, /class="agent-selector-dialog"/)
  assert.match(agentSelector, /class="agent-selector-body"/)
  assert.match(agentSelector, /class="agent-detail-panel"/)
  // 不再有锚点定位与悬浮详情浮层的定位逻辑
  assert.doesNotMatch(agentSelector, /anchorEl/)
  assert.doesNotMatch(agentSelector, /updateDropdownPosition/)
  assert.doesNotMatch(agentSelector, /detailPanelStyle/)
  // 输入框侧也不应残留锚点 ref / 定位函数
  assert.doesNotMatch(inputField, /agentModeButtonRef/)
  assert.doesNotMatch(inputField, /updateAgentModeDropdownPosition/)
})

test('mention panel offers MCP/skills when typing @ but not via the KB toolbar button', () => {
  const loadStart = inputField.indexOf('const loadMentionItems')
  const loadEnd = inputField.indexOf('const triggerMention', loadStart)
  const loadMention = inputField.slice(loadStart, loadEnd)

  assert.notEqual(loadStart, -1)
  assert.notEqual(loadEnd, -1)
  // 输入 @ 字符触发：MCP / Skills 正常出现在面板
  assert.match(loadMention, /mcpItems = mcpServices\.value/)
  assert.match(loadMention, /skillItems = editorResources\.skills/)
  // 两种来源的分流开关：按钮触发（isMentionTriggeredByButton=true）不含 MCP/技能
  const gateStart = loadMention.indexOf('if (!isMentionTriggeredByButton.value)')
  assert.notEqual(gateStart, -1)
  const gated = loadMention.slice(gateStart)
  assert.ok(gated.indexOf('mcpItems') > -1)
  assert.ok(gated.indexOf('skillItems') > -1)
  // 工具栏弹窗的勾选入口与 @ 面板共用同一份 store 选择态
  assert.match(inputField, /const toggleMCPService/)
  assert.match(inputField, /const toggleSkill/)
})
