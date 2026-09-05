import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./MemberAuditLogModal.vue', import.meta.url), 'utf8')
const localeSource = readFileSync(new URL('../../i18n/locales/zh-CN.ts', import.meta.url), 'utf8')
const registrySource = readFileSync(new URL('../../i18n/auditActionRegistry.ts', import.meta.url), 'utf8')

test('member audit log is a standalone dialog driven by the ui store', () => {
  assert.match(source, /showMemberAuditModal/)
  assert.match(source, /uiStore\.closeMemberAudit\(\)/)
  assert.match(source, /listAuditLog/)
  // 游标分页 + 触底加载。
  assert.match(source, /after_id/)
  assert.match(source, /IntersectionObserver/)
})

test('access denied is translated into plain language by request path', () => {
  // 登录接口上的拒绝 → 疑似登录时密码错误；其余 → 权限/登录过期的白话。
  assert.match(source, /auth\/login/)
  assert.match(source, /memberAudit\.loginDenied/)
  assert.match(source, /memberAudit\.otherDenied/)
  assert.match(localeSource, /疑似登录时密码错误/)
  assert.match(localeSource, /尝试使用未开通的功能，或登录状态已过期/)
})

test('friendly action labels replace jargon in the shared tenantMember bag', () => {
  assert.match(localeSource, /'rbac\.member_added': '添加了成员'/)
  assert.match(localeSource, /'rbac\.member_removed': '将成员移出了空间'/)
  assert.match(localeSource, /'rbac\.member_password_reset': '重置了成员密码'/)
  assert.match(registrySource, /'rbac\.member_password_reset'/)
  assert.match(localeSource, /denied: '未通过'/)
  // 不再出现「访问被拒」这类术语化文案。
  assert.doesNotMatch(localeSource, /访问被拒/)
})

test('table keeps only plain columns — no request path, no raw JSON expand', () => {
  assert.match(source, /memberAudit\.columns\.time/)
  assert.match(source, /memberAudit\.columns\.actor/)
  assert.match(source, /memberAudit\.columns\.action/)
  assert.match(source, /memberAudit\.columns\.outcome/)
  assert.doesNotMatch(source, /columns\.path/)
  assert.doesNotMatch(source, /JSON\.stringify/)
})
