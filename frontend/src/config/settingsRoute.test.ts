import assert from 'node:assert/strict'
import test from 'node:test'

import {
  buildSettingsRouteQuery,
  integrationSectionKey,
  isIntegrationSection,
  normalizeSettingsSection,
  settingsQueryUnchanged,
} from './settingsRoute'

test('every settings nav item writes only section', () => {
  assert.deepEqual(
    buildSettingsRouteQuery('models', {
      section: 'system-global',
      tab: 'im',
      agentId: 'agt_1',
    }),
    { section: 'models' },
  )
  assert.deepEqual(
    buildSettingsRouteQuery('general', { section: 'system-global' }),
    { section: 'general' },
  )
  assert.deepEqual(
    buildSettingsRouteQuery('runtime-queues', { section: 'system-global' }),
    { section: 'runtime-queues' },
  )
  assert.deepEqual(
    buildSettingsRouteQuery(integrationSectionKey('claw'), {
      section: 'integrations',
      tab: 'im',
    }),
    { section: 'integration-claw' },
  )
  assert.deepEqual(
    buildSettingsRouteQuery(integrationSectionKey('api'), {
      section: 'integrations',
      tab: 'im',
      agentId: 'agt_1',
    }),
    { section: 'integration-api', agentId: 'agt_1' },
  )
})

test('legacy api / integrations / bare-tab query strings normalize to nav keys', () => {
  assert.equal(normalizeSettingsSection('api'), 'integration-api')
  assert.equal(normalizeSettingsSection('claw'), 'integration-claw')
  assert.equal(normalizeSettingsSection('integrations', 'embed'), 'integration-embed')
  assert.equal(normalizeSettingsSection('integrations'), 'integration-im')
  assert.equal(normalizeSettingsSection('integration-chrome'), 'integration-chrome')
  assert.equal(normalizeSettingsSection('system-global'), 'system-global')
  assert.equal(isIntegrationSection('integration-chrome'), true)
  assert.equal(isIntegrationSection('models'), false)
  assert.equal(isIntegrationSection('integration-unknown'), false)
})

test('the pre-000095 skills key aliases to the read-only skill catalog', () => {
  // 000099：技能以只读「技能目录」回到空间 Settings，旧 openSettings/
  // 书签里的 'skills' 归一化到 skill-catalog；?sandbox= 预选只在停留在
  // 技能目录时保留，切走即清理，防止旧预选回流。
  assert.equal(normalizeSettingsSection('skills'), 'skill-catalog')
  assert.deepEqual(
    buildSettingsRouteQuery('general', { section: 'skill-catalog', sandbox: 'sbx_1' }),
    { section: 'general' },
  )
  assert.deepEqual(
    buildSettingsRouteQuery('skill-catalog', { section: 'general', sandbox: 'sbx_1' }),
    { section: 'skill-catalog', sandbox: 'sbx_1' },
  )
})

test('the pre-000101 members key falls back to the general settings page', () => {
  // 000101：成员管理拆出 Settings（独立弹窗，UserMenu → uiStore 直开），
  // 旧深链 ?section=members 归一化到 general，不再渲染已删除的 section。
  assert.equal(normalizeSettingsSection('members'), 'general')
})

test('canonical settings query skips a redundant replace', () => {
  assert.equal(
    settingsQueryUnchanged(
      { section: 'integration-claw' },
      { section: 'integration-claw' },
    ),
    true,
  )
  assert.equal(
    settingsQueryUnchanged(
      { section: 'integrations', tab: 'claw' },
      { section: 'integration-claw' },
    ),
    false,
  )
})
