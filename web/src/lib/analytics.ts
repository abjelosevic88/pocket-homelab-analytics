// Words, numbers and the range/filter state shared by the pages.
import { useCallback, useMemo } from 'react'
import { useSearchParams } from 'react-router-dom'
import type { Filter } from './types'
import { fmtTime } from './format'

export const fmtNum = (n: number | undefined | null): string => {
  const v = n || 0
  if (Math.abs(v) >= 1e6) return (v / 1e6).toFixed(v >= 1e7 ? 0 : 1).replace(/\.0$/, '') + 'M'
  if (Math.abs(v) >= 1e4) return (v / 1e3).toFixed(v >= 1e5 ? 0 : 1).replace(/\.0$/, '') + 'k'
  return Math.round(v).toLocaleString()
}

export const fmtDur = (sec: number | undefined | null): string => {
  const s = Math.round(sec || 0)
  if (s < 60) return `${s}s`
  if (s < 3600) return `${Math.floor(s / 60)}m ${String(s % 60).padStart(2, '0')}s`
  return `${Math.floor(s / 3600)}h ${String(Math.floor((s % 3600) / 60)).padStart(2, '0')}m`
}

export const fmtPct = (n: number | undefined | null) => `${Math.round(n || 0)}%`

/** The change from before to now, and whether it is good news. */
export function delta(now: number, before: number, lowerIsBetter = false): { text: string; dir: 'up' | 'down' | 'flat'; good: boolean } | null {
  if (!before) return now ? null : null
  const p = Math.round(((now - before) * 100) / before)
  if (p === 0) return { text: '0%', dir: 'flat', good: true }
  const up = p > 0
  return { text: `${up ? '+' : '−'}${Math.abs(p)}%`, dir: up ? 'up' : 'down', good: lowerIsBetter ? !up : up }
}

let regionNames: Intl.DisplayNames | null = null
try { regionNames = new Intl.DisplayNames(['en'], { type: 'region' }) } catch { regionNames = null }

export function countryName(code: string): string {
  if (!code) return 'Unknown'
  try { return regionNames?.of(code.toUpperCase()) || code } catch { return code }
}

export function flag(code: string): string {
  if (!code || code.length !== 2) return '🌐'
  return String.fromCodePoint(...code.toUpperCase().split('').map(c => 0x1f1a5 + c.charCodeAt(0)))
}

let langNames: Intl.DisplayNames | null = null
try { langNames = new Intl.DisplayNames(['en'], { type: 'language' }) } catch { langNames = null }
export function languageName(code: string): string {
  if (!code) return 'Unknown'
  try { return langNames?.of(code) || code } catch { return code }
}

export const DIM_LABEL: Record<string, string> = {
  source: 'Source', channel: 'Channel', referrer: 'Referrer', utm_source: 'Campaign source', utm_medium: 'Campaign medium', utm_campaign: 'Campaign',
  utm_content: 'Campaign content', utm_term: 'Campaign term', country: 'Country', region: 'Region', city: 'City', device: 'Device', browser: 'Browser',
  browser_version: 'Browser version', os: 'System', os_version: 'System version', language: 'Language', hostname: 'Host', page: 'Page', entry: 'Entry page',
  exit: 'Exit page', event: 'Event', goal: 'Goal', outbound: 'Link', download: 'File', click: 'Clicked',
}

const EVENT_WORDS: Record<string, string> = { 'Outbound Link: Click': 'Followed a link out', 'File Download': 'Downloaded a file', Click: 'Pressed a link or button', '404': 'Page not found' }
/** Built-in event names in plain words. */
export const eventLabel = (n: string) => EVENT_WORDS[n] || n

/** What a value of a dimension looks like for people. */
export function valueLabel(dim: string, v: string): string {
  if (dim === 'event') return eventLabel(v)
  if (dim === 'country') return countryName(v)
  if (dim === 'language') return languageName(v)
  if (v === '' || v === '(none)') return dim === 'utm_campaign' || dim.startsWith('utm_') ? '(no campaign)' : '(none)'
  return v
}

export const FILTERABLE = new Set(['source', 'channel', 'referrer', 'utm_source', 'utm_medium', 'utm_campaign', 'utm_content', 'utm_term', 'country', 'region', 'city', 'device', 'browser', 'os', 'language', 'page', 'entry', 'exit', 'event', 'hostname'])

export const RANGES: { key: string; label: string; hint?: string }[] = [
  { key: 'today', label: 'Today', hint: 'D' }, { key: 'yesterday', label: 'Yesterday', hint: 'E' }, { key: '7d', label: 'Last 7 days', hint: 'W' },
  { key: '30d', label: 'Last 30 days', hint: 'T' }, { key: 'month', label: 'This month', hint: 'M' }, { key: 'lastmonth', label: 'Last month' },
  { key: '12mo', label: 'Last 12 months', hint: 'Y' }, { key: 'year', label: 'This year' }, { key: 'all', label: 'All time', hint: 'A' },
]

export const deviceIcon = (d: string) => (d === 'Phone' ? 'smartphone' : d === 'Tablet' ? 'smartphone' : 'monitor')

/** The range and filters live in the address, so a view can be shared and reloads keep it. */
export function useView() {
  const [sp, setSp] = useSearchParams()
  const range = sp.get('range') || '30d'
  const from = sp.get('from') || ''
  const to = sp.get('to') || ''
  const interval = sp.get('interval') || ''
  const fRaw = sp.getAll('f')
  const filters: Filter[] = useMemo(() => fRaw.map(f => {
    const not = f.startsWith('!')
    const body = not ? f.slice(1) : f
    const i = body.indexOf(':')
    return { dim: body.slice(0, i), value: body.slice(i + 1), not }
  }).filter(f => f.dim), [fRaw.join('\n')]) // eslint-disable-line react-hooks/exhaustive-deps
  const query = useMemo(() => {
    const q = new URLSearchParams()
    q.set('range', range)
    if (range === 'custom') { q.set('from', from); q.set('to', to) }
    if (interval) q.set('interval', interval)
    fRaw.forEach(f => q.append('f', f))
    return q.toString()
  }, [range, from, to, interval, fRaw.join('\n')]) // eslint-disable-line react-hooks/exhaustive-deps
  const update = useCallback((fn: (q: URLSearchParams) => void) => {
    setSp(prev => { const q = new URLSearchParams(prev); fn(q); return q }, { replace: true })
  }, [setSp])
  const setRange = useCallback((key: string, a = '', b = '') => update(q => {
    q.set('range', key); q.delete('interval')
    if (key === 'custom') { q.set('from', a); q.set('to', b) } else { q.delete('from'); q.delete('to') }
  }), [update])
  const setInterval = useCallback((i: string) => update(q => { if (i) q.set('interval', i); else q.delete('interval') }), [update])
  const addFilter = useCallback((dim: string, value: string, not = false) => update(q => {
    const keep = q.getAll('f').filter(f => !(f.replace(/^!/, '').startsWith(dim + ':')))
    q.delete('f')
    keep.forEach(f => q.append('f', f))
    q.append('f', `${not ? '!' : ''}${dim}:${value}`)
  }), [update])
  const removeFilter = useCallback((dim: string) => update(q => {
    const keep = q.getAll('f').filter(f => !(f.replace(/^!/, '').startsWith(dim + ':')))
    q.delete('f')
    keep.forEach(f => q.append('f', f))
  }), [update])
  const clearFilters = useCallback(() => update(q => q.delete('f')), [update])
  return { range, from, to, interval, filters, query, setRange, setInterval, addFilter, removeFilter, clearFilters }
}

/** The label under a chart point. */
export function bucketLabel(ts: number, interval: string, long = false): string {
  const d = new Date(ts * 1000)
  if (interval === 'hour') return fmtTime(d)
  if (interval === 'month') return d.toLocaleDateString(undefined, { month: 'short', year: long ? 'numeric' : '2-digit' })
  const s = d.toLocaleDateString(undefined, { day: 'numeric', month: 'short', ...(long ? { year: 'numeric' } : {}) })
  return interval === 'week' && long ? `Week of ${s}` : s
}

export function ago(ts: number): string {
  const s = Math.max(0, Math.round(Date.now() / 1000 - ts))
  if (s < 60) return 'just now'
  if (s < 3600) return `${Math.floor(s / 60)} min ago`
  if (s < 86400) return `${Math.floor(s / 3600)} h ago`
  const d = Math.floor(s / 86400)
  return d === 1 ? 'yesterday' : `${d} days ago`
}

export function zones(): string[] {
  try { return (Intl as unknown as { supportedValuesOf: (k: string) => string[] }).supportedValuesOf('timeZone') } catch { return ['UTC', 'Europe/Sarajevo', 'Europe/Berlin', 'Europe/London', 'America/New_York'] }
}
