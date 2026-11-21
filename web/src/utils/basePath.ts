export function getBasePath(): string {
  const raw = (window as any).__MAILSERVER_BASE__
  if (typeof raw !== 'string') return ''
  const trimmed = raw.trim().replace(/\/+$/, '')
  if (!trimmed || trimmed === '/') return ''
  return trimmed.startsWith('/') ? trimmed : `/${trimmed}`
}

export function withBasePath(path: string): string {
  const base = getBasePath()
  const normalized = path.startsWith('/') ? path : `/${path}`
  return `${base}${normalized}` || '/'
}
