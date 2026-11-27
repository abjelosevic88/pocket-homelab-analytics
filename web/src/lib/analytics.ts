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

/** Money in its currency; without one, the plain number. */
export function fmtMoney(amount: number, currency = ''): string {
  const digits = Math.abs(amount) >= 1000 || Number.isInteger(amount) ? 0 : 2
  try {
    return new Intl.NumberFormat(undefined, currency ? { style: 'currency', currency, minimumFractionDigits: digits, maximumFractionDigits: digits } : { minimumFractionDigits: digits, maximumFractionDigits: digits }).format(amount)
  } catch { return `${amount.toFixed(digits)} ${currency}`.trim() }
}

// ---- how fast: what counts as good, by Google's Core Web Vitals ----

export type SpeedKey = 'lcp' | 'inp' | 'cls' | 'ttfb' | 'fcp'
export type Rating = 'good' | 'ok' | 'slow'
/** Up to the first number is good, up to the second could be better, beyond is slow. */
export const SPEED: Record<SpeedKey, { label: string; short: string; tech: string; good: number; ok: number; help: string }> = {
  lcp: { label: 'The page shows in', short: 'Shows in', tech: 'Largest Contentful Paint', good: 2500, ok: 4000, help: 'Until the main part of the page is on the screen. Good: up to 2.5 seconds.' },
  inp: { label: 'A press answers in', short: 'Answers in', tech: 'Interaction to Next Paint', good: 200, ok: 500, help: 'From a tap, click or key to the page showing a result. Good: up to 200 milliseconds.' },
  cls: { label: 'The layout moves', short: 'Moves', tech: 'Cumulative Layout Shift', good: 0.1, ok: 0.25, help: 'How much things jump around while the page loads (0 = not at all). Good: up to 0.1.' },
  ttfb: { label: 'The server answers in', short: 'Server', tech: 'Time to First Byte', good: 800, ok: 1800, help: 'From asking for the page to its first bytes arriving. Good: up to 0.8 seconds.' },
  fcp: { label: 'Something shows in', short: 'First sign', tech: 'First Contentful Paint', good: 1800, ok: 3000, help: 'Until the first text or picture is on the screen. Good: up to 1.8 seconds.' },
}
export const rate = (k: SpeedKey, v: number): Rating => (v <= SPEED[k].good ? 'good' : v <= SPEED[k].ok ? 'ok' : 'slow')
export const RATING_WORD: Record<Rating, string> = { good: 'Good', ok: 'Could be better', slow: 'Slow' }
export function fmtSpeed(k: SpeedKey, v: number): string {
  if (k === 'cls') return v < 0.005 ? '0' : v.toFixed(2)
  if (v < 1000) return `${Math.round(v)} ms`
  return `${(v / 1000).toFixed(v < 10000 ? 1 : 0)} s`
}

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
  contact: 'Email or phone', form: 'Form', search: 'Searched for', notfound: 'Missing page', prop: 'Detail', from: 'The page with the link',
  network: 'Network', ip: 'IP address', postal: 'Postal code', returning: 'New or returning', screen: 'Screen', timezone: 'Time zone',
}

// Owners of data centres and clouds: a visit from one is a server, a VPN or a robot, rarely a person at home.
const HOSTING = /amazon|aws|google cloud|google llc|microsoft|azure|digitalocean|hetzner|ovh|linode|akamai|vultr|choopa|oracle|alibaba|tencent|contabo|scaleway|leaseweb|m247|datacamp|cloudflare|fastly|hostinger|ionos|netcup|alphavps|hosting|server|data ?cent|cdn77|psychz|quadranet|colocrossing|packet|equinix|zenlayer|g-core|gcore|kamatera|upcloud|nforce|worldstream|serverius|tzulo|ipxo|private internet access|nordvpn|expressvpn|mullvad|proton|surfshark|cyberghost|hide\.me|windscribe/i
/** Whether a network is a data centre, a cloud or a VPN. */
export const isHosting = (network?: string) => !!network && HOSTING.test(network)

/** The lists that count things people did (not visits): their number column says "People". */
export const DID_DIMS = new Set(['event', 'outbound', 'download', 'click', 'contact', 'form', 'search', 'notfound', 'prop', 'from'])

const EVENT_WORDS: Record<string, string> = {
  'Outbound Link: Click': 'Followed a link out', 'File Download': 'Downloaded a file', Click: 'Pressed a link or button', '404': 'Landed on a page that does not exist',
  'Contact Link: Click': 'Pressed an email or phone link', 'Form: Submission': 'Sent a form', Search: 'Searched the site',
}
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

export const FILTERABLE = new Set(['source', 'channel', 'referrer', 'utm_source', 'utm_medium', 'utm_campaign', 'utm_content', 'utm_term', 'country', 'region', 'city', 'device', 'browser', 'os', 'language', 'page', 'entry', 'exit', 'event', 'hostname', 'network', 'ip', 'postal', 'returning', 'screen', 'timezone'])

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

/** A moment as a clock in the site's time zone would show it (a day there starts at its own midnight, not the browser's). */
export function inZone(ts: number, tz?: string): Date {
  const d = new Date(ts * 1000)
  if (!tz) return d
  try {
    const parts = new Intl.DateTimeFormat('en-US', { timeZone: tz, hourCycle: 'h23', year: 'numeric', month: 'numeric', day: 'numeric', hour: 'numeric', minute: 'numeric' }).formatToParts(d)
    const g = (t: string) => Number(parts.find(x => x.type === t)?.value)
    const w = new Date(g('year'), g('month') - 1, g('day'), g('hour'), g('minute'))
    return Number.isNaN(w.getTime()) ? d : w
  } catch { return d }
}

/** The label under a chart point, in the site's time zone. */
export function bucketLabel(ts: number, interval: string, long = false, tz?: string): string {
  const d = inZone(ts, tz)
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
