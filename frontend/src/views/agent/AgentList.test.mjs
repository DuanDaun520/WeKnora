import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./AgentList.vue', import.meta.url), 'utf8')

test('the header create button carries a text label, not just the icon', () => {
  // 000101：创建智能体按钮原先只有火花图标 + tooltip，现补上文字；
  // 样式从 28px 方块改为自适应宽度，图标与文字并排完整显示。
  const headerBtn = source.match(/data-guide="agent-list-create"[\s\S]*?<\/t-button>/)
  assert.ok(headerBtn, 'expected the header create button')
  assert.match(headerBtn[0], /\{\{ \$t\('agent\.createAgent'\) \}\}/)

  const btnStyle = source.match(/\.header-action-btn \{([\s\S]*?)\n\}/)
  assert.ok(btnStyle, 'expected the header-action-btn style block')
  assert.match(btnStyle[1], /width: auto !important/)
  assert.doesNotMatch(btnStyle[1], /width: 28px/)
  assert.match(btnStyle[1], /white-space: nowrap/)
})

test('agent cards show a skills feature badge when skills are in use', () => {
  // 与 网络搜索/知识库/MCP/多轮 同一徽章行：勾选了技能或选「全部」时显示，
  // 图标与编辑器/提及选择器共用 SKILL_ICON。三处卡片（主列表、收藏/最近、
  // 共享智能体）都要有。
  const badgeCount = (source.match(/feature-badge skills/g) || []).length
  assert.ok(badgeCount >= 3, `expected skills badges on all card rows, got ${badgeCount}`)
  assert.match(source, /selected_skills\?\.length \|\| agent\.config\?\.skills_selection_mode === 'all'/)
  assert.match(source, /shared\.agent\?\.config\?\.selected_skills\?\.length \|\| shared\.agent\?\.config\?\.skills_selection_mode === 'all'/)
  assert.match(source, /:name="SKILL_ICON"/)
  assert.match(source, /import \{ SKILL_ICON \} from '@\/types\/mention'/)
  assert.match(source, /features\.skills/)
})

test('the create button is greyed out (not hidden) for non-admins, with an admin-names tooltip', () => {
  // 000102：普通用户（=角色体系里的 contributor，UI 文案「普通用户」）与
  // viewer 都不能创建——按钮保持可见但置灰，悬停提示联系空间管理员并列出
  // 名单（listMembers 是 Viewer+，可拉到 admin/owner 用户名）。门槛与后端
  // POST /agents 的 Admin+ 门对齐。tooltip 挂外层 span：原生 disabled 按钮
  // 在部分浏览器不派发 hover。
  const headerBtnTag = source.match(/<t-button[^>]*class="header-action-btn"[^>]*>/)
  assert.ok(headerBtnTag, 'expected the header create button tag')
  assert.match(headerBtnTag[0], /:disabled="!canCreateAgent"/)
  assert.match(headerBtnTag[0], /data-guide="agent-list-create"/)
  assert.doesNotMatch(headerBtnTag[0], /v-if=/)

  assert.match(source, /const canCreateAgent = computed\(\(\) => authStore\.hasRole\('admin'\)\)/)
  assert.doesNotMatch(source, /canCreateAgent = computed\(\(\) => authStore\.hasRole\('contributor'\)\)/)
  assert.match(source, /:content="canCreateAgent \? \$t\('agent\.createAgent'\) : createDeniedTooltip"/)
  assert.match(source, /createDeniedHint/)
  assert.match(source, /m\.role === 'admin' \|\| m\.role === 'owner'/)
  assert.match(source, /if \(!canCreateAgent\.value\) void fetchTenantAdminNames\(\)/)
  // listMembers 的 page_size 上限是 100：传 200 会 400 且被 catch 吞掉，
  // tooltip 误报「暂无」。分页拉全量 + 租户 ID 晚就绪时补拉。
  assert.doesNotMatch(source, /page_size: 200/)
  assert.match(source, /const pageSize = 100/)
  assert.match(source, /listMembers\(tenantId, \{ page, page_size: pageSize \}\)/)
  assert.match(source, /watch\(\(\) => authStore\.currentTenantId, \(id\) => \{/)
  assert.match(source, /\(m\.status \?\? 'active'\) === 'active'/)
  // 外部入口（菜单 openAgentEditor 事件 / defineExpose）同样过门
  assert.match(source, /const openCreateModal = \(\) => \{\s*\/\/ 000102[^\n]*\n\s*if \(!canCreateAgent\.value\) return/)
  assert.match(source, /class="create-btn-wrap"/)
  // 置灰态压掉 hover 主题色（样式被 !important 钉死，必须显式覆盖）
  assert.match(source, /&:not\(\.t-is-disabled\):hover/)
  assert.match(source, /\.t-is-disabled \{/)
})

test('viewer clicks on agents they cannot manage open the editor read-only', () => {
  // 000102：canManageAgent 之外的成员点卡片进「查看智能体」只读弹窗。
  assert.match(
    source,
    /:readOnly="editorMode === 'edit' && editingAgent != null && !canManageAgent\(editingAgent as AgentWithUI\)"/,
  )
})

test('the copy action follows the admin-only creation gate', () => {
  // 000102：复制也是创建（POST /agents/:id/copy 已同步收紧为 Admin+），
  // 菜单项与「更多」弹层的可见性都不再放 contributor 进来。
  const copyItems = source.match(/v-if="canCreateAgent" class="popup-menu-item" @click="handleCopy\(agent\)"/g) || []
  assert.ok(copyItems.length >= 2, `expected both copy menu items gated by canCreateAgent, got ${copyItems.length}`)
  assert.doesNotMatch(source, /v-if="authStore\.hasRole\('contributor'\)" class="popup-menu-item" @click="handleCopy/)
  // 空态引导只对能创建的管理员展示
  assert.match(source, /if \(!authStore\.hasRole\('admin'\)\) return false/)
})
