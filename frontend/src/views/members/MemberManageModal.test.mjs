import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./MemberManageModal.vue', import.meta.url), 'utf8')
const uiStoreSource = readFileSync(new URL('../../stores/ui.ts', import.meta.url), 'utf8')
const userMenuSource = readFileSync(new URL('../../components/UserMenu.vue', import.meta.url), 'utf8')

test('member manage modal is a standalone dialog driven by the ui store', () => {
  assert.match(source, /showMemberManageModal/)
  assert.match(source, /uiStore\.closeMemberManage\(\)/)
  assert.match(uiStoreSource, /openMemberManage\(\)/)
  // UserMenu 直开弹窗，不再借 Settings 路由深链（?section=members 已成历史）。
  assert.match(userMenuSource, /openMemberManage/)
  assert.doesNotMatch(userMenuSource, /handleQuickNav\('members'\)/)
})

test('roster table shows 工号/姓名/角色/添加时间/最后登录 with never-logged dash', () => {
  assert.match(source, /colKey: 'employee_id'/)
  assert.match(source, /colKey: 'username'/)
  assert.match(source, /colKey: 'role'/)
  assert.match(source, /colKey: 'joined_at'/)
  assert.match(source, /colKey: 'last_login_at'/)
  // 从未登录显示 -（i18n memberManage.lastLoginNever）。
  assert.match(source, /memberManage\.lastLoginNever/)
})

test('row operations call the 000101 tenant member APIs', () => {
  assert.match(source, /addTenantMember/)
  assert.match(source, /resetMemberPassword/)
  assert.match(source, /getMemberStats/)
  assert.match(source, /removeMember/)
  // 重置结果一次性展示 8 位数字新密码。
  assert.match(source, /new_password/)
  assert.match(source, /memberManage\.passwordResultHint/)
})

test('invite copies site + employee id + name + default password for never-logged members', () => {
  assert.match(source, /window\.location\.origin/)
  assert.match(source, /memberManage\.invite\.textEmployeeId/)
  assert.match(source, /memberManage\.invite\.textPassword/)
  // 仅从未登录的成员可点邀请（有 last_login_at 即禁用）。
  assert.match(source, /:disabled="!!row\.last_login_at"/)
})

test('admins and self are protected from removal — prompt only, no API call', () => {
  // isProtected 判定在 onRemove 里先行返回，不会走到 removeMember。
  assert.match(source, /function isProtected\(row: TenantMember\): boolean/)
  assert.match(source, /row\.user_id === selfUserId\.value \|\| isManagerRole\(row\.role\)/)
  assert.match(source, /memberManage\.remove\.adminProtected/)
  const onRemove = source.slice(source.indexOf('function onRemove('))
  const promptReturn = onRemove.indexOf('adminProtected')
  const apiCall = onRemove.indexOf('removeMember(')
  assert.ok(promptReturn > -1 && apiCall > -1 && promptReturn < apiCall)
  // 不能重置自己的密码：提示后直接返回。
  assert.match(source, /memberManage\.reset\.selfForbidden/)
})
