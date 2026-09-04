import assert from 'node:assert/strict'
import test from 'node:test'

import {
  SETTINGS_MANAGEMENT_SHORTCUT_MIN_ROLE,
  SETTINGS_SECTION_MIN_ROLE,
  SYSTEM_ADMIN_SETTINGS_SECTIONS,
} from './settingsAccess'

test('management shortcuts are stricter than read-only settings pages', () => {
  assert.equal(SETTINGS_SECTION_MIN_ROLE.members, 'viewer')
  // 两级空间角色后成员名单的快捷入口对空间管理员开放（owner 已随
  // 000092/000093 角色扁平化退出企业版）。
  assert.equal(SETTINGS_MANAGEMENT_SHORTCUT_MIN_ROLE.members, 'admin')
  // 模型 / Ollama / WeKnoraCloud 设置已迁入系统管理控制台（000094
  // 模型平台化），空间 Settings 不再持有这些 section。
  assert.equal(Object.prototype.hasOwnProperty.call(SETTINGS_SECTION_MIN_ROLE, 'models'), false)
  assert.equal(Object.prototype.hasOwnProperty.call(SETTINGS_SECTION_MIN_ROLE, 'ollama'), false)
  assert.equal(Object.prototype.hasOwnProperty.call(SETTINGS_SECTION_MIN_ROLE, 'weknoracloud'), false)
  assert.equal(
    Object.prototype.hasOwnProperty.call(SETTINGS_MANAGEMENT_SHORTCUT_MIN_ROLE, 'models'),
    false,
  )
})

test('the skill catalog is admin-only like the sandbox it installs into', () => {
  assert.equal(SETTINGS_SECTION_MIN_ROLE.skills, 'admin')
  assert.equal(SETTINGS_SECTION_MIN_ROLE.skills, SETTINGS_SECTION_MIN_ROLE.sandbox)
  assert.equal(SETTINGS_MANAGEMENT_SHORTCUT_MIN_ROLE.skills, 'admin')
})

test('personal skill environment variables are visible to every member', () => {
  // 沙箱密钥（envvars）按企业版要求收归空间管理员。
  assert.equal(SETTINGS_SECTION_MIN_ROLE.envvars, 'admin')
  // Workspace-wide skill env values live on the Admin+ skills page; a
  // management shortcut on the avatar menu would only duplicate that entrance.
  assert.equal(
    Object.prototype.hasOwnProperty.call(SETTINGS_MANAGEMENT_SHORTCUT_MIN_ROLE, 'envvars'),
    false,
  )
})

test('system administration settings stay explicitly system-admin-only', () => {
  assert.deepEqual(
    [...SYSTEM_ADMIN_SETTINGS_SECTIONS],
    ['system-global', 'runtime-queues', 'platform-api-keys', 'system-audit-log'],
  )
})
