import React, { createContext, useCallback, useContext, useState } from 'react'
import { CheckCircle2, Info, X, XCircle } from 'lucide-react'
import { useI18n } from '../contexts/I18nContext'

type ToastKind = 'success' | 'error' | 'info'

interface ToastItem {
  id: number
  kind: ToastKind
  text: string
}

interface ToastContextValue {
  success: (text: string) => void
  error: (text: string) => void
  info: (text: string) => void
}

const ToastContext = createContext<ToastContextValue | null>(null)

let nextToastId = 1

export const ToastProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const { t } = useI18n()
  const [items, setItems] = useState<ToastItem[]>([])

  const dismiss = useCallback((id: number) => {
    setItems((prev) => prev.filter((item) => item.id !== id))
  }, [])

  const push = useCallback(
    (kind: ToastKind, text: string) => {
      const id = nextToastId++
      setItems((prev) => [...prev.slice(-4), { id, kind, text }])
      const duration = kind === 'error' ? 6000 : 3500
      setTimeout(() => {
        dismiss(id)
      }, duration)
    },
    [dismiss]
  )

  const value: ToastContextValue = {
    success: useCallback((text: string) => push('success', text), [push]),
    error: useCallback((text: string) => push('error', text), [push]),
    info: useCallback((text: string) => push('info', text), [push]),
  }

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div aria-live="polite" aria-relevant="additions" className="fixed top-4 right-4 z-[100] flex flex-col gap-2 max-w-md w-[calc(100vw-2rem)] pointer-events-none">
        {items.map((item) => {
          const style =
            item.kind === 'success'
              ? 'bg-emerald-50 border-emerald-200 text-emerald-900 dark:bg-emerald-950/95 dark:border-emerald-800 dark:text-emerald-100'
              : item.kind === 'error'
              ? 'bg-rose-50 border-rose-200 text-rose-900 dark:bg-rose-950/95 dark:border-rose-800 dark:text-rose-100'
              : 'bg-indigo-50 border-indigo-200 text-indigo-900 dark:bg-indigo-950/95 dark:border-indigo-800 dark:text-indigo-100'
          const icon =
            item.kind === 'success' ? (
              <CheckCircle2 className="w-4 h-4 text-emerald-600 dark:text-emerald-400 shrink-0 mt-0.5" />
            ) : item.kind === 'error' ? (
              <XCircle className="w-4 h-4 text-rose-600 dark:text-rose-400 shrink-0 mt-0.5" />
            ) : (
              <Info className="w-4 h-4 text-indigo-600 dark:text-indigo-400 shrink-0 mt-0.5" />
            )
          return (
            <div
              key={item.id}
              role={item.kind === 'error' ? 'alert' : 'status'}
              className={`pointer-events-auto flex items-start gap-2.5 px-3.5 py-2.5 rounded-xl border shadow-lg text-sm ${style}`}
            >
              {icon}
              <div className="flex-1 min-w-0 break-words">{item.text}</div>
              <button
                type="button"
                onClick={() => dismiss(item.id)}
                className="p-0.5 rounded opacity-60 hover:opacity-100 shrink-0"
                aria-label={t('common.closeToast')}
              >
                <X className="w-3.5 h-3.5" />
              </button>
            </div>
          )
        })}
      </div>
    </ToastContext.Provider>
  )
}

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext)
  if (!ctx) throw new Error('useToast must be used within ToastProvider')
  return ctx
}
