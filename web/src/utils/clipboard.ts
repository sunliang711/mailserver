import { getCurrentLang, translate } from '../contexts/I18nContext'

export async function copyText(text: string): Promise<void> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      return
    }
  } catch {
    // Fall through to legacy copy path.
  }

  const focused = document.activeElement instanceof HTMLElement ? document.activeElement : null
  const input = document.createElement('textarea')
  input.value = text
  input.readOnly = true
  input.style.cssText = 'position: fixed; left: -9999px; top: 0; opacity: 0; pointer-events: none'
  document.body.appendChild(input)
  try {
    input.select()
    input.setSelectionRange(0, text.length)
    if (!document.execCommand('copy')) {
      throw new Error(translate(getCurrentLang(), 'clipboard.unsupported'))
    }
  } finally {
    input.remove()
    focused?.focus({ preventScroll: true })
  }
}
