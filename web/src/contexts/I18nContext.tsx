import React, { createContext, useCallback, useContext, useEffect, useState } from 'react'
import { zh, type TranslationKey } from '../i18n/zh'
import { en } from '../i18n/en'

export type Lang = 'zh' | 'en'

const LANG_STORAGE_KEY = 'mailserver.lang'
const dictionaries: Record<Lang, Record<TranslationKey, string>> = { zh, en }

let currentLangCache: Lang = 'zh'

function getInitialLang(): Lang {
  if (typeof window === 'undefined') return 'zh'
  try {
    const saved = window.localStorage.getItem(LANG_STORAGE_KEY)
    if (saved === 'zh' || saved === 'en') {
      currentLangCache = saved
      return saved
    }
  } catch {
    // ignore storage access errors
  }
  currentLangCache = 'zh'
  return 'zh'
}

export function getCurrentLang(): Lang {
  return currentLangCache
}

export function translate(lang: Lang, key: TranslationKey, vars?: Record<string, string | number>): string {
  const dict = dictionaries[lang] || zh
  let text = dict[key] ?? zh[key] ?? key
  if (vars) {
    for (const [k, v] of Object.entries(vars)) {
      text = text.replace(new RegExp(`\\{${k}\\}`, 'g'), String(v))
    }
  }
  return text
}

interface I18nContextValue {
  lang: Lang
  setLang: (next: Lang) => void
  toggleLang: () => void
  t: (key: TranslationKey, vars?: Record<string, string | number>) => string
}

const I18nContext = createContext<I18nContextValue | null>(null)

export const I18nProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [lang, setLangState] = useState<Lang>(getInitialLang)

  useEffect(() => {
    currentLangCache = lang
    document.documentElement.lang = lang === 'en' ? 'en' : 'zh-CN'
    document.title = translate(lang, 'app.title')
    try {
      window.localStorage.setItem(LANG_STORAGE_KEY, lang)
    } catch {
      // ignore storage errors
    }
  }, [lang])

  const setLang = useCallback((next: Lang) => {
    currentLangCache = next
    setLangState(next)
  }, [])

  const toggleLang = useCallback(() => {
    setLangState((prev) => {
      const next: Lang = prev === 'zh' ? 'en' : 'zh'
      currentLangCache = next
      return next
    })
  }, [])

  const t = useCallback(
    (key: TranslationKey, vars?: Record<string, string | number>) => translate(lang, key, vars),
    [lang]
  )

  return <I18nContext.Provider value={{ lang, setLang, toggleLang, t }}>{children}</I18nContext.Provider>
}

export function useI18n(): I18nContextValue {
  const ctx = useContext(I18nContext)
  if (!ctx) throw new Error('useI18n must be used within I18nProvider')
  return ctx
}
