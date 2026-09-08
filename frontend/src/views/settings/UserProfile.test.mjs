import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./UserProfile.vue', import.meta.url), 'utf8')
const userMenu = readFileSync(new URL('../../components/UserMenu.vue', import.meta.url), 'utf8')
const authApi = readFileSync(new URL('../../api/auth/index.ts', import.meta.url), 'utf8')
const useMyAvatar = readFileSync(new URL('../../composables/useMyAvatar.ts', import.meta.url), 'utf8')
const cropDialog = readFileSync(new URL('../../components/AvatarCropDialog.vue', import.meta.url), 'utf8')
const i18n = readFileSync(new URL('../../i18n/locales/zh-CN.ts', import.meta.url), 'utf8')

test('the settings page rides the shared avatar components and upload flow', () => {
  // 头像行：UserAvatar 展示（blob URL 来自 useMyAvatar）+ 隐藏 input 选图，
  // 选完先裁剪（AvatarCropDialog）再上传。
  assert.match(source, /<UserAvatar :src="avatarUrl" :name="userInfo\?\.username" size="lg" \/>/)
  assert.match(source, /import UserAvatar from '@\/components\/UserAvatar\.vue'/)
  assert.match(source, /import AvatarCropDialog from '@\/components\/AvatarCropDialog\.vue'/)
  assert.match(source, /import \{ useMyAvatar \} from '@\/composables\/useMyAvatar'/)
  assert.match(source, /ref="avatarInputRef"[\s\S]*?accept="image\/png,image\/jpeg,image\/webp"/)
  assert.match(source, /pendingAvatarFile\.value = file/)
  assert.match(source, /cropDialogVisible\.value = true/)
  assert.match(source, /const resp = await uploadMyAvatar\(file\)/)
})

test('upload success refreshes the store so every consumer updates live', () => {
  // users.avatar 是存储 ref；refreshFromAuthMe 更新 authStore.user.avatar，
  // useMyAvatar 的 watch 重新拉字节 —— 侧边栏菜单不用刷新页面就换图。
  assert.match(source, /await authStore\.refreshFromAuthMe\(\)/)
  assert.match(useMyAvatar, /watch\(/)
  assert.match(useMyAvatar, /\(\) => authStore\.user\?\.avatar \|\| ''/)
  assert.match(useMyAvatar, /fetchMyAvatarBlob\(trimmed\)/)
  // 版本参数击穿 max-age：URL 随 ref 变化，换头像立即生效。
  assert.match(authApi, /\/auth\/me\/avatar\?v=\$\{encodeURIComponent\(version\)\}/)
})

test('workspace-less users get a gated button instead of a backend error', () => {
  // 资源目录要求归属租户：无空间用户禁用上传并给提示文案。
  assert.match(source, /canUseAvatar = computed\(/)
  assert.match(source, /v-if="!canUseAvatar"/)
  assert.match(source, /userProfile\.avatar\.noWorkspace/)
})

test('the cropper is hand-rolled: pan + zoom + circular viewport, no new deps', () => {
  // 零依赖：Pointer Events 平移、滚轮/滑杆缩放、canvas 512×512 JPEG 导出；
  // createImageBitmap 烘焙 EXIF 方向，否则手机竖拍照片会裁歪。
  assert.match(cropDialog, /imageOrientation: 'from-image'/)
  assert.match(cropDialog, /setPointerCapture/)
  assert.match(cropDialog, /@wheel\.prevent="onWheel"/)
  assert.match(cropDialog, /border-radius: 50%/)
  assert.match(cropDialog, /toBlob\(resolve, 'image\/jpeg', 0\.92\)/)
  assert.match(cropDialog, /new File\(\[blob\], 'avatar\.jpg'/)
})

test('the auth API carries the identity-scoped avatar endpoints', () => {
  assert.match(authApi, /uploadMyAvatar/)
  assert.match(authApi, /postUpload\('\/api\/v1\/auth\/me\/avatar'/)
  assert.match(authApi, /deleteMyAvatar/)
  assert.match(authApi, /del\('\/api\/v1\/auth\/me\/avatar'\)/)
  assert.match(authApi, /fetchMyAvatarBlob/)
})

test('UserMenu renders the shared component and drops the gradient initial', () => {
  // 旧回退是品牌色渐变 + 首字母；现在统一走 UserAvatar 的灰色默认剪影。
  // （租户切换子菜单的首字母块是另一套，不在本断言范围。）
  assert.match(userMenu, /<UserAvatar :src="avatarUrl" :name="userName" size="sm" \/>/)
  assert.doesNotMatch(userMenu, /avatar-placeholder/)
  assert.doesNotMatch(userMenu, /userInitial/)
  assert.doesNotMatch(userMenu, /\.user-avatar[\s\S]{0,400}?linear-gradient\(135deg, var\(--td-brand-color\)/)
})

test('userProfile i18n block carries the avatar strings', () => {
  assert.match(i18n, /avatar: \{/)
  assert.match(i18n, /cropTitle: '裁剪头像'/)
  assert.match(i18n, /cropHint: '拖动调整位置，滚轮或滑杆缩放'/)
  assert.match(i18n, /noWorkspace: '请先加入工作空间后再设置头像'/)
  assert.match(i18n, /tooLarge: '图片不能超过 2MB'/)
})
