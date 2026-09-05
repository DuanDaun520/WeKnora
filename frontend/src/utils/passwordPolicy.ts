import type { FormRule } from 'tdesign-vue-next'

type Translate = (key: string, params?: Record<string, unknown>) => string

// 密码策略：仅要求至少 6 位，不做字符类别 / 长度上限等其他要求，
// 与后端 ValidatePasswordPolicy 保持一致。
export function newPasswordRules(
  t: Translate,
  extra: FormRule[] = [],
): FormRule[] {
  return [
    { required: true, message: t('auth.passwordRequired'), type: 'error' },
    { min: 6, message: t('auth.passwordMinLength'), type: 'error' },
    ...extra,
  ]
}
