// The big chart on a site's page: this period as a filled line, the period
// before as a dashed one, with a readout under the pointer. Drawn at the
// box's real width, so text and dots keep their shape.
import { useCallback, useMemo, useRef, useState } from 'react'
import { bucketLabel } from '../lib/analytics'

function useWidth(fallback: number) {
  const [w, setW] = useState(fallback)
  const ro = useRef<ResizeObserver | null>(null)
  const ref = useCallback((el: Element | null) => {
    ro.current?.disconnect(); ro.current = null
    if (!el) return
    const measure = () => { const cw = Math.round(el.getBoundingClientRect().width); if (cw > 0) setW(cw) }
    measure(); ro.current = new ResizeObserver(measure); ro.current.observe(el)
  }, [])
  return [ref, w] as const
}

function niceTicks(max: number): number[] {
  if (max <= 0) return [0, 1]
  const raw = max / 4
  const mag = Math.pow(10, Math.floor(Math.log10(raw)))
  const norm = raw / mag
  const step = (norm <= 1 ? 1 : norm <= 2 ? 2 : norm <= 2.5 ? 2.5 : norm <= 5 ? 5 : 10) * mag
  const out: number[] = []
  for (let t = 0; t <= max + step * 0.999 && out.length < 7; t += step) out.push(+t.toFixed(6))
  return out
}

export function MainChart({ starts, values, previous, interval, format, height = 240, now, label, notes, tz }: {
  starts: number[]; values: number[]; previous?: number[]; interval: string; format: (v: number) => string; height?: number; now: number; label: string
  /** What happened on a day; ts = when that day starts. */
  notes?: { id: number; ts?: number; text: string }[]
  /** The site's time zone: days and hours are labelled as they are there. */
  tz?: string
}) {
  const [ref, W] = useWidth(700)
  const [hover, setHover] = useState<number | null>(null)
  const H = height, L = 44, R = 12, T = 12, B = 26
  const n = values.length
  // buckets that have not started yet are not drawn
  const lastReal = useMemo(() => { let i = n - 1; while (i > 0 && starts[i] > now) i--; return i }, [starts, n, now])
  const max = Math.max(1, ...values.slice(0, lastReal + 1), ...(previous || []).slice(0, n))
  const ticks = niceTicks(max)
  const top = ticks[ticks.length - 1] || 1
  const x = (i: number) => L + (n > 1 ? (i * (W - L - R)) / (n - 1) : (W - L - R) / 2)
  const y = (v: number) => T + (1 - v / top) * (H - T - B)
  const line = useMemo(() => values.slice(0, lastReal + 1).map((v, i) => `${i ? 'L' : 'M'}${x(i).toFixed(1)},${y(v).toFixed(1)}`).join(' '), [values, lastReal, W, top]) // eslint-disable-line react-hooks/exhaustive-deps
  const prevLine = useMemo(() => (previous || []).slice(0, n).map((v, i) => `${i ? 'L' : 'M'}${x(i).toFixed(1)},${y(v).toFixed(1)}`).join(' '), [previous, n, W, top]) // eslint-disable-line react-hooks/exhaustive-deps
  const every = Math.max(1, Math.ceil(n / Math.max(2, Math.floor((W - L - R) / 84))))
  const onMove = (e: React.MouseEvent | React.TouchEvent) => {
    if (!n) return
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
    const cx = 'touches' in e ? e.touches[0].clientX : e.clientX
    const i = Math.round(((cx - rect.left - L) / (W - L - R)) * (n - 1))
    setHover(Math.max(0, Math.min(lastReal, i)))
  }
  // each note sits on the bucket its day falls in
  const marks = useMemo(() => {
    const by = new Map<number, string[]>()
    for (const nt of notes || []) {
      if (nt.ts === undefined || !n) continue
      let i = -1
      for (let k = 0; k < n; k++) if (starts[k] <= nt.ts) i = k
      // a day note on an hourly chart belongs to that day's first hour; on weeks and months, to the bucket holding it
      if (i < 0) i = 0
      by.set(i, [...(by.get(i) || []), nt.text])
    }
    return by
  }, [notes, starts, n])
  const hv = hover !== null ? values[hover] : null
  const hp = hover !== null && previous ? previous[hover] : undefined
  const tipLeft = hover !== null ? Math.min(Math.max(x(hover), 90), W - 90) : 0
  return (
    <div ref={ref} className="main-chart" style={{ height: H }} onMouseMove={onMove} onMouseLeave={() => setHover(null)} onTouchStart={onMove} onTouchMove={onMove} onTouchEnd={() => setHover(null)}>
      <svg viewBox={`0 0 ${W} ${H}`} width={W} height={H} role="img" aria-label={label}>
        <defs>
          <linearGradient id="mc-fill" x1="0" x2="0" y1="0" y2="1">
            <stop offset="0%" stopColor="var(--accent)" stopOpacity="0.22" />
            <stop offset="100%" stopColor="var(--accent)" stopOpacity="0.02" />
          </linearGradient>
        </defs>
        {ticks.map(t => <g key={t}><line className="mc-grid" x1={L} x2={W - R} y1={y(t)} y2={y(t)} /><text className="mc-tick" x={L - 8} y={y(t) + 4} textAnchor="end">{format(t)}</text></g>)}
        {prevLine && <path className="mc-prev" d={prevLine} />}
        {line && n > 1 && <path d={`${line} L${x(lastReal).toFixed(1)},${y(0)} L${x(0)},${y(0)} Z`} fill="url(#mc-fill)" />}
        {line && <path className="mc-line" d={line} />}
        {n <= 45 && values.slice(0, lastReal + 1).map((v, i) => <circle key={i} className="mc-dot" cx={x(i)} cy={y(v)} r={2.5} />)}
        {starts.map((s, i) => (i % every === 0 && i + every / 2 < n) || i === n - 1 ? <text key={i} className="mc-tick" x={x(i)} y={H - 7} textAnchor={i === 0 ? 'start' : i === n - 1 ? 'end' : 'middle'}>{bucketLabel(s, interval, false, tz)}</text> : null)}
        {[...marks.keys()].map(i => <g key={`m${i}`} className="mc-mark">
          <line x1={x(i)} x2={x(i)} y1={T + 7} y2={H - B} />
          <circle cx={x(i)} cy={T + 3} r={4} />
        </g>)}
        {hover !== null && hv !== null && <g>
          <line className="mc-cursor" x1={x(hover)} x2={x(hover)} y1={T} y2={H - B} />
          {hp !== undefined && <circle cx={x(hover)} cy={y(hp)} r={3.5} className="mc-dot-prev" />}
          <circle cx={x(hover)} cy={y(hv)} r={4.5} className="mc-dot-hi" />
        </g>}
      </svg>
      {hover !== null && hv !== null && <div className="mc-tip" style={{ left: tipLeft }}>
        <span className="mc-tip-when">{bucketLabel(starts[hover], interval, true, tz)}</span>
        <b>{format(hv)} {label.toLowerCase()}</b>
        {hp !== undefined && <span className="mc-tip-prev">Before: {format(hp)}</span>}
        {(marks.get(hover) || []).map((t, k) => <span key={k} className="mc-tip-note">{t}</span>)}
      </div>}
    </div>
  )
}
