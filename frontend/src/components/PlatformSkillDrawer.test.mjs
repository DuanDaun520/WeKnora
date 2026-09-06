import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./PlatformSkillDrawer.vue', import.meta.url), 'utf8')
const i18n = readFileSync(new URL('../i18n/locales/zh-CN.ts', import.meta.url), 'utf8')

test('the basic-info section edits category and author (000104/000105)', () => {
  assert.match(source, /skillLibrary\.metaSectionTitle/)
  assert.match(source, /categoryInput/)
  // 分类只从已登记的分类里选（000105）：可筛选、可清空，但不再即输即建
  assert.match(source, /filterable/)
  assert.match(source, /clearable/)
  assert.doesNotMatch(source, /creatable/)
  assert.match(source, /loadSkillCategoriesOnce/)
  // 作者输入框：留空回落 SKILL.md
  assert.match(source, /authorInput/)
  assert.match(source, /:maxlength="255"/)
  // 编辑态显示当前版本（只读）
  assert.match(source, /skillLibrary\.currentVersion/)
})

test('create rides the register POST with the metadata, zip upload only', () => {
  // 空 category/author = 后端回落 SKILL.md frontmatter；zh 字段空串 = 无中文展示
  assert.match(
    source,
    /createPlatformSkillFromFile\(file, onProgress, category, author, zhName, zhDescription\)/,
  )
  // 「从源安装」已下线：界面与 API 封装都不再有 source 注册路径
  assert.doesNotMatch(source, /FromSource|sourceInput/)
  assert.match(source, /hasBundleInput = computed\(\(\) => !!pendingFile\.value\)/)
})

test('edit persists metadata only when a value changed', () => {
  // 差量判断：与打开时的基线相同就不发 meta PUT —— 否则会 bump updated_at
  // 给每个分配伪造 drift。000106 后 diff 覆盖 category/author/zh 四字段。
  assert.match(source, /metaBaseline/)
  assert.match(source, /category === metaBaseline\.value\.category &&/)
  assert.match(source, /author === metaBaseline\.value\.author &&/)
  assert.match(source, /zhName === metaBaseline\.value\.zhName &&/)
  assert.match(source, /zhDescription === metaBaseline\.value\.zhDescription/)
  assert.match(source, /updatePlatformSkillMeta\(props\.skill!\.id, \{/)
  assert.match(source, /zh_name: zhName/)
  assert.match(source, /zh_description: zhDescription/)
  // 保存顺序：bundle → meta → 分配 replace
  const bundleAt = source.indexOf('await persistBundle()')
  const metaAt = source.indexOf('await persistMeta()')
  const assignAt = source.indexOf('await persistAssignments()')
  assert.ok(bundleAt > -1 && metaAt > bundleAt && assignAt > metaAt,
    'save order must be bundle → meta → assignments')
})

test('the drawer edits the Chinese display fields and hints when uninstalled (000106)', () => {
  assert.match(source, /v-model="zhNameInput"/)
  assert.match(source, /v-model="zhDescriptionInput"/)
  assert.match(source, /t-textarea/)
  assert.match(source, /skillLibrary\.zhNameLabel/)
  assert.match(source, /skillLibrary\.zhDescriptionLabel/)
  // 已分配但未装到任何沙箱的行给引导而非静默
  assert.match(source, /skillLibrary\.assignmentNotInstalledHint/)
  assert.match(source, /assignment-row__hint--muted/)
})

test('skillLibrary i18n block carries the metadata strings', () => {
  assert.match(i18n, /metaSectionTitle: '基本信息'/)
  assert.match(i18n, /categoryPlaceholder: '选择分类（未分类留空）'/)
  assert.match(i18n, /authorLabel: '作者'/)
  assert.match(i18n, /authorPlaceholder: '留空自动读取 SKILL\.md'/)
  assert.match(i18n, /currentVersion: '当前版本：\{version\}'/)
  assert.match(i18n, /metaFailed: '保存基本信息失败'/)
  assert.match(i18n, /zhNameLabel: '中文名称'/)
  assert.match(i18n, /zhDescriptionLabel: '描述'/)
})
