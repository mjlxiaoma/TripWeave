import { ApiError } from '../../services/api'
import i18n from '../../i18n'

export function authErrorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    const key = `errors.${err.code}`
    const translated = i18n.t(key)
    if (translated !== key) return translated
  }
  return i18n.t('errors.UNKNOWN')
}
