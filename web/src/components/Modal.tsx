import React, { useEffect, useRef } from 'react'
import { createPortal } from 'react-dom'
import { X } from 'lucide-react'
import { useI18n } from '../contexts/I18nContext'

interface Props {
  title: string
  onClose: () => void
  busy?: boolean
  dismissible?: boolean
  size?: 'md' | 'lg' | 'xl' | '2xl'
  zIndexClass?: string
  children: React.ReactNode
}

export function Modal({ title, onClose, busy = false, dismissible = true, size = 'lg', zIndexClass = '', children }: Props) {
  const { t } = useI18n()
  const root = useRef<HTMLDivElement>(null)
  const close = useRef(onClose)
  const blocked = useRef(false)
  close.current = onClose
  blocked.current = busy || !dismissible

  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null
    const overflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    const focusable = () => Array.from(root.current?.querySelectorAll<HTMLElement>(
      'button:not(:disabled), a[href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex="0"]'
    ) || []).filter((el) => el.getClientRects().length > 0)
    if (!root.current?.contains(document.activeElement)) {
      const first = root.current?.querySelector<HTMLElement>('input:not(:disabled), select:not(:disabled), textarea:not(:disabled)')
      ;(first || focusable()[0] || root.current)?.focus()
    }
    const onKeyDown = (event: KeyboardEvent) => {
      const dialogs = document.querySelectorAll('[role="dialog"][aria-modal="true"]')
      if (dialogs.length > 0 && dialogs[dialogs.length - 1] !== root.current) return
      if (event.key === 'Escape' && !blocked.current) {
        event.preventDefault()
        event.stopPropagation()
        close.current()
      }
      if (event.key === 'Tab') {
        const elements = focusable()
        const first = elements[0]
        const last = elements[elements.length - 1]
        if (!first) {
          event.preventDefault()
          root.current?.focus()
        } else if (event.shiftKey && (document.activeElement === first || document.activeElement === root.current)) {
          event.preventDefault()
          last.focus()
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault()
          first.focus()
        }
      }
    }
    document.addEventListener('keydown', onKeyDown, true)
    return () => {
      document.body.style.overflow = overflow
      document.removeEventListener('keydown', onKeyDown, true)
      previous?.focus()
    }
  }, [])

  return createPortal(
    <div className={`modal-backdrop ${zIndexClass}`.trim()} onClick={(event) => {
      if (event.target === event.currentTarget && dismissible && !busy) onClose()
    }}>
      <div ref={root} role="dialog" aria-modal="true" aria-label={title} aria-busy={busy} tabIndex={-1} className={`modal-container ${size === 'md' ? 'max-w-md' : size === 'xl' ? 'max-w-3xl' : size === '2xl' ? 'max-w-5xl' : 'max-w-2xl'}`}>
        {dismissible && <button type="button" onClick={onClose} disabled={busy} aria-label={t('common.closeModal')} className="modal-close"><X className="w-4 h-4" /></button>}
        {children}
      </div>
    </div>,
    document.body
  )
}
