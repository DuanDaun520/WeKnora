import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./SystemConsole.vue', import.meta.url), 'utf8')

test('skill-library is a first-class console section, not a managed panel', () => {
  // 平台技能库（000098）与沙箱连接同模式：平台目录 + 按空间分配，
  // 无空间选择栏，因此不进 MANAGED_SECTIONS。
  assert.match(source, /'skill-library'/)
  assert.match(source, /SkillLibraryPanel v-else-if="currentSection === 'skill-library'"/)
  assert.match(source, /key: 'skill-library'/)
  assert.match(source, /t\('settings\.skillLibrary'\)/)
  const managedBlock = source.match(
    /const MANAGED_SECTIONS = new Set<ConsoleSection>\(\[([\s\S]*?)\]\)/,
  )
  assert.ok(managedBlock, 'MANAGED_SECTIONS literal should exist')
  assert.doesNotMatch(managedBlock[1], /skill-library/)
})

test('the sandbox config panel is merged into sandbox-connections', () => {
  // 沙箱配置不再有独立代管面板：菜单、渲染分支、合法 section 列表均无
  // 'sandbox'，旧深链 section=sandbox 经 LEGACY_SECTION_ALIASES 改道到
  // sandbox-connections。
  assert.doesNotMatch(source, /key: 'sandbox',/)
  assert.doesNotMatch(source, /currentSection === 'sandbox'/)
  const validBlock = source.match(/const VALID_SECTIONS: ConsoleSection\[\] = \[([\s\S]*?)\]/)
  assert.ok(validBlock, 'VALID_SECTIONS literal should exist')
  assert.doesNotMatch(validBlock[1], /'sandbox'/)
  assert.match(source, /sandbox: 'sandbox-connections'/)
  assert.doesNotMatch(source, /SandboxSettings/)
})

test('the skill panel moved back to workspace Settings', () => {
  // 000099：技能代管面板移出控制台，技能面收敛为「平台技能库（供给）+
  // 空间技能目录（消费）」。菜单、渲染分支、合法 section 列表均无
  // 'skills'；旧深链 section=skills 跨路由改道到空间 Settings 的
  // skill-catalog，?sandbox= 预选透传。
  assert.doesNotMatch(source, /key: 'skills',/)
  assert.doesNotMatch(source, /currentSection === 'skills'/)
  assert.doesNotMatch(source, /SkillSettings/)
  const managedBlock = source.match(
    /const MANAGED_SECTIONS = new Set<ConsoleSection>\(\[([\s\S]*?)\]\)/,
  )
  assert.ok(managedBlock, 'MANAGED_SECTIONS literal should exist')
  assert.doesNotMatch(managedBlock[1], /'skills'/)
  const validBlock = source.match(/const VALID_SECTIONS: ConsoleSection\[\] = \[([\s\S]*?)\]/)
  assert.ok(validBlock, 'VALID_SECTIONS literal should exist')
  assert.doesNotMatch(validBlock[1], /'skills'/)
  assert.match(source, /section !== 'skills'/)
  assert.match(source, /section: 'skill-catalog'/)
  assert.match(source, /\/platform\/settings/)
})

test('system settings (SSRF whitelist / docker toggle) is reachable from the console', () => {
  // 平台级系统设置原挂在 /platform 外壳的「系统管理」组，bootstrap 系统
  // 管理员无空间绑定打不开外壳；作为独立 section 迁入控制台直达。
  assert.match(source, /'system-settings'/)
  const validBlock = source.match(/const VALID_SECTIONS: ConsoleSection\[\] = \[([\s\S]*?)\]/)
  assert.ok(validBlock, 'VALID_SECTIONS literal should exist')
  assert.match(validBlock[1], /'system-settings'/)
  assert.match(source, /SystemSettings v-else-if="currentSection === 'system-settings'"/)
  assert.match(source, /key: 'system-settings'/)
  assert.match(source, /t\('settings\.system'\)/)
  // 代管面板集合里不应出现 system-settings（它无空间维度）。
  const managedBlock = source.match(
    /const MANAGED_SECTIONS = new Set<ConsoleSection>\(\[([\s\S]*?)\]\)/,
  )
  assert.ok(managedBlock, 'MANAGED_SECTIONS literal should exist')
  assert.doesNotMatch(managedBlock[1], /'system-settings'/)
  assert.match(source, /menuGroups\.systemAdmin/)
})
