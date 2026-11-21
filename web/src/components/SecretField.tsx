import React, { useEffect, useState } from 'react'
import { Check, Copy, Eye, EyeOff, Loader2 } from 'lucide-react'
import { useToast } from './Toast'
import { useI18n } from '../contexts/I18nContext'
import { copyText } from '../utils/clipboard'

interface SecretFieldProps {
  id?: string
  value: string
  onChange: (next: string) => void
  hasSaved?: boolean
  onReveal?: () => Promise<string>
  disabled?: boolean
  readOnly?: boolean
  required?: boolean
  placeholder?: string
  autoComplete?: string
}

export const SecretField: React.FC<SecretFieldProps> = ({
  id,
  value,
  onChange,
  hasSaved = false,
  onReveal,
  disabled = false,
  readOnly = false,
  required = false,
  placeholder,
  autoComplete = 'off',
}) => {
  const toast = useToast()
  const { t } = useI18n()
  const [editing, setEditing] = useState(false)
  const [showPlain, setShowPlain] = useState(false)
  const [revealedValue, setRevealedValue] = useState<string | null>(null)
  const [revealing, setRevealing] = useState(false)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    setEditing(false)
    setShowPlain(false)
    setRevealedValue(null)
  }, [hasSaved, disabled])

  const showingSavedMask = !disabled && hasSaved && !editing
  const displayValue = disabled
    ? ''
    : showingSavedMask
    ? showPlain && revealedValue !== null
      ? revealedValue
      : '*****'
    : value

  const fetchSecret = async (): Promise<string | null> => {
    if (revealedValue !== null) return revealedValue
    if (!onReveal) return null
    setRevealing(true)
    try {
      const secret = await onReveal()
      setRevealedValue(secret)
      return secret
    } catch (e: any) {
      toast.error(e?.message || t('secret.fetchError'))
      return null
    } finally {
      setRevealing(false)
    }
  }

  const handleToggleVisibility = async () => {
    if (disabled || revealing) return
    if (showPlain) {
      setShowPlain(false)
      return
    }
    if (showingSavedMask) {
      const secret = await fetchSecret()
      if (secret === null) return
      if (secret === '') {
        toast.info(t('secret.emptySaved'))
        return
      }
    }
    setShowPlain(true)
  }

  const handleCopy = async () => {
    if (disabled || revealing) return
    let textToCopy = ''
    if (showingSavedMask) {
      const secret = await fetchSecret()
      if (!secret) {
        toast.info(t('secret.nothingToCopy'))
        return
      }
      textToCopy = secret
    } else {
      if (!value) return
      textToCopy = value
    }
    try {
      await copyText(textToCopy)
      setCopied(true)
      toast.success(t('secret.copied'))
      setTimeout(() => setCopied(false), 1500)
    } catch (e: any) {
      toast.error(e?.message || t('secret.copyFailedManual'))
    }
  }

  const canRevealOrCopy = !disabled && (hasSaved || value !== '')

  return (
    <div className="flex items-center gap-1.5">
      <div className="relative flex-1 min-w-0">
        <input
          id={id}
          autoComplete={autoComplete}
          required={required}
          disabled={disabled}
          readOnly={readOnly}
          value={displayValue}
          type={showingSavedMask ? 'text' : showPlain ? 'text' : 'password'}
          placeholder={
            placeholder ??
            (hasSaved
              ? readOnly
                ? t('secret.placeholderReadOnly')
                : t('secret.placeholderSaved')
              : t('secret.placeholderEmpty'))
          }
          onFocus={() => {
            if (!disabled && !readOnly && hasSaved && !editing) {
              setEditing(true)
              onChange('')
            }
          }}
          onBlur={() => {
            if (editing && value === '' && hasSaved) {
              setEditing(false)
            }
          }}
          onChange={(e) => {
            if (readOnly) return
            if (!editing) setEditing(true)
            onChange(e.target.value)
          }}
          className="w-full px-3 py-1.5 pr-8 border rounded-lg font-mono text-xs disabled:bg-slate-100 dark:disabled:bg-slate-800"
        />
        <button
          type="button"
          disabled={!canRevealOrCopy || revealing}
          onClick={handleToggleVisibility}
          title={showPlain ? t('secret.hide') : t('secret.show')}
          aria-label={showPlain ? t('secret.hide') : t('secret.show')}
          className="absolute right-2 top-1/2 -translate-y-1/2 text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 disabled:opacity-30"
        >
          {revealing ? (
            <Loader2 className="w-3.5 h-3.5 animate-spin" />
          ) : showPlain ? (
            <EyeOff className="w-3.5 h-3.5" />
          ) : (
            <Eye className="w-3.5 h-3.5" />
          )}
        </button>
      </div>
      <button
        type="button"
        disabled={!canRevealOrCopy || revealing}
        onClick={handleCopy}
        title={t('common.copy')}
        aria-label={t('common.copy')}
        className="inline-flex items-center gap-1 px-2.5 py-1.5 border border-slate-300 dark:border-slate-700 hover:bg-slate-50 dark:hover:bg-slate-800 disabled:opacity-40 disabled:cursor-not-allowed rounded-lg text-xs text-slate-700 dark:text-slate-200 shrink-0"
      >
        {copied ? (
          <>
            <Check className="w-3.5 h-3.5 text-emerald-600 dark:text-emerald-400" />
            <span className="text-emerald-700 dark:text-emerald-300">{t('common.copied')}</span>
          </>
        ) : (
          <>
            <Copy className="w-3.5 h-3.5" />
            <span>{t('common.copy')}</span>
          </>
        )}
      </button>
    </div>
  )
}
