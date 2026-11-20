// More of what a site's page shows: how fast the pages are, when people
// come, the way through a page, and notes about what happened on a day.
import React, { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import type { Note, Paths, Site, Speed as SpeedT, SpeedRow, When as WhenT } from '../lib/types'
import { RATING_WORD, SPEED, type SpeedKey, countryName, deviceIcon, flag, fmtNum, fmtSpeed, rate, useView } from '../lib/analytics'
import { fmtDate, fmtTime, todayISO } from '../lib/format'
import { Confirm, Field, Loading, Modal, errMsg, useToast } from './ui'
import { Icon } from './icons'

/** Loads one of a site's lists for the range and filters in the address. */
function useSiteData<T>(siteID: number, what: string, extra = ''): T | null {
  const view = useView()
  const [data, setData] = useState<T | null>(null)
  useEffect(() => {
    let live = true
    api.get<T>(`${V1}/sites/${siteID}/${what}?${view.query}${extra}`).then(d => { if (live) setData(d) }).catch(() => { if (live) setData(null) })
    return () => { live = false }
  }, [siteID, what, view.query, extra])
  return data
}

// ---- how fast ----

const SHOWN: SpeedKey[] = ['lcp', 'inp', 'cls', 'ttfb']
const SPEED_TABS = [{ dim: 'page', label: 'Pages' }, { dim: 'device', label: 'Devices' }, { dim: 'browser', label: 'Browsers' }, { dim: 'country', label: 'Countries' }]

function SpeedCell({ k, row }: { k: SpeedKey; row: SpeedRow }) {
  const m = row[k]
  if (!m) return <span className="num sp-cell muted">—</span>
  const r = rate(k, m.p75)
  return <span className={`num sp-cell sp-${r}`} title={`${RATING_WORD[r]} · ${m.n.toLocaleString()} ${m.n === 1 ? 'load' : 'loads'}`}><i className="sp-dot" aria-hidden="true" />{fmtSpeed(k, m.p75)}</span>
}

export function Speed({ site }: { site: Site }) {
  const view = useView()
  const [dim, setDim] = useState('page')
  const data = useSiteData<SpeedT>(site.id, 'speed', `&dim=${dim}&limit=8`)
  const label = (r: SpeedRow) => dim === 'country' ? <><span className="flag" aria-hidden="true">{flag(r.value)}</span>{countryName(r.value)}</>
    : dim === 'device' ? <><Icon name={deviceIcon(r.value)} size={14} className="muted" />{r.value}</> : r.value || '(unknown)'
  return (
    <section className="card panel panel-wide">
      <div className="panel-head">
        <h3><Icon name="gauge" size={15} />How fast the pages are</h3>
        <div className="ptabs" role="tablist">
          {SPEED_TABS.map(t => <button key={t.dim} type="button" role="tab" aria-selected={t.dim === dim} className={`ptab ${t.dim === dim ? 'on' : ''}`} onClick={() => setDim(t.dim)}>{t.label}</button>)}
        </div>
      </div>
      {!data ? <div className="rows rows-loading short"><Loading /></div>
        : !data.all.loads ? <div className="rows rows-empty short muted small">{data.on
          ? 'No page loads were measured in this time. Speed is measured in each visitor\'s browser while the page loads, so the next visits fill this in.'
          : 'Speed is switched off for this site (Site settings → What is counted).'}</div>
        : <>
          <div className="sp-tiles">
            {SHOWN.map(k => {
              const m = data.all[k]
              const r = m ? rate(k, m.p75) : null
              return (
                <div key={k} className={`sp-tile ${r ? `sp-${r}` : ''}`} title={`${SPEED[k].tech}. ${SPEED[k].help}`}>
                  <span className="sp-l">{SPEED[k].label}</span>
                  <span className="sp-v tnum">{m ? fmtSpeed(k, m.p75) : '—'}</span>
                  <span className="sp-r">{r ? <><i className="sp-dot" aria-hidden="true" />{RATING_WORD[r]}</> : <span className="muted">Not measured by these browsers</span>}</span>
                </div>
              )
            })}
          </div>
          <p className="muted small sp-note">Three of four page loads were this fast or faster · {fmtNum(data.all.loads)} {data.all.loads === 1 ? 'load' : 'loads'} measured, only the first page of a visit{data.all.inp ? '' : ' · Safari and older browsers do not measure presses'}.</p>
          <div className="rows wide sp-rows">
            <div className="rows-h"><span>{SPEED_TABS.find(t => t.dim === dim)?.label.replace(/s$/, '')}</span>{SHOWN.map(k => <span key={k} className="num sp-cell" title={SPEED[k].tech}>{SPEED[k].short}</span>)}<span className="num">Loads</span></div>
            {data.rows.map(r => (
              <button key={r.value} type="button" className="rrow" title={`Only ${r.value}`} onClick={() => view.addFilter(dim, r.value)}>
                <span className="rlabel ellipsis">{label(r)}</span>
                {SHOWN.map(k => <SpeedCell key={k} k={k} row={r} />)}
                <span className="num tnum">{fmtNum(r.loads)}</span>
              </button>
            ))}
          </div>
        </>}
    </section>
  )
}

// ---- when they come ----

export function When({ site }: { site: Site }) {
  const data = useSiteData<WhenT>(site.id, 'when')
  const days = data ? Array.from({ length: 7 }, (_, i) => ((data.week_start % 7) + i) % 7) : []
  const dayName = (d: number, long = false) => new Date(2023, 0, 1 + d).toLocaleDateString(undefined, { weekday: long ? 'long' : 'short' }) // 1 Jan 2023 was a Sunday
  const hour = (h: number) => fmtTime(new Date(2023, 0, 1, h, 0)).replace(':00', '').replace(/^0(\d)/, '$1')
  let best = { d: 0, h: 0, n: 0 }
  data?.cells.forEach((row, d) => row.forEach((n, h) => { if (n > best.n) best = { d, h, n } }))
  return (
    <section className="card panel">
      <div className="panel-head"><h3><Icon name="clock" size={15} />When they come</h3></div>
      {!data ? <div className="rows rows-loading"><Loading /></div>
        : !data.visits ? <div className="rows rows-empty muted small">Nothing in this time.</div>
        : <div className="rows when">
          <div className="when-grid" role="img" aria-label={`Visits by day of the week and hour. Busiest: ${dayName(best.d, true)} around ${hour(best.h)}.`}>
            <span />
            {[0, 6, 12, 18].map(h => <span key={h} className="when-h muted" style={{ gridColumn: `${h + 2} / span 6` }}>{hour(h)}</span>)}
            {days.map(d => [
              <span key={`l${d}`} className="when-d muted">{dayName(d)}</span>,
              ...data.cells[d].map((n, h) => (
                <span key={`${d}-${h}`} className="when-c" style={{ '--v': n ? 0.12 + 0.88 * (n / data.max) : 0 } as React.CSSProperties}
                  title={`${dayName(d, true)}, ${hour(h)}–${hour((h + 1) % 24)}: ${n.toLocaleString()} ${n === 1 ? 'visit' : 'visits'}`} />
              )),
            ])}
          </div>
          <p className="muted small when-foot">Busiest: <b>{dayName(best.d, true)}</b> around <b>{hour(best.h)}</b> · darker means more visits · times in {data.tz.replace(/_/g, ' ')}</p>
        </div>}
    </section>
  )
}

// ---- the way through a page ----

export function PagePaths({ site, page }: { site: Site; page: string }) {
  const view = useView()
  const data = useSiteData<Paths>(site.id, 'paths', `&page=${encodeURIComponent(page)}&limit=7`)
  const col = (title: string, steps: Paths['before'], edge: string, icon: string) => {
    const max = Math.max(1, ...steps.map(s => s.times))
    return (
      <div className="pp-col">
        <div className="rows-h"><span>{title}</span><span className="num">Times</span></div>
        {steps.map(s => {
          const inner = <>
            <span className="rbar" style={{ width: `${(s.times / max) * 100}%` }} />
            <span className="rlabel ellipsis" title={s.value || edge}>{s.value ? s.value : <><Icon name={icon} size={13} className="muted" /><i>{edge}</i></>}</span>
            <span className="num tnum">{fmtNum(s.times)}</span>
          </>
          return s.value
            ? <button key={s.value} type="button" className="rrow" title={`Look at ${s.value} instead`} onClick={() => view.addFilter('page', s.value)}>{inner}</button>
            : <div key="" className="rrow static">{inner}</div>
        })}
      </div>
    )
  }
  return (
    <section className="card panel panel-wide">
      <div className="panel-head"><h3><Icon name="route" size={15} />The way through {page}</h3><span className="muted small">Press a page to follow the way</span></div>
      {!data ? <div className="rows rows-loading short"><Loading /></div>
        : !data.views ? <div className="rows rows-empty short muted small">Nobody saw this page in this time.</div>
        : <div className="pp">
          {col('Where they were before', data.before, 'Came in here', 'arrow-down')}
          <div className="pp-mid"><Icon name="chevron-right" size={18} className="muted" /><div className="pp-page"><b className="ellipsis" title={page}>{page}</b><span className="muted small">seen {fmtNum(data.views)} {data.views === 1 ? 'time' : 'times'}</span></div><Icon name="chevron-right" size={18} className="muted" /></div>
          {col('Where they went next', data.after, 'Left the site', 'log-out')}
        </div>}
    </section>
  )
}

// ---- notes: what happened on a day ----

export function NotesSheet({ site, onClose, onChanged }: { site: Site; onClose: () => void; onChanged: () => void }) {
  const { toast } = useToast()
  const [notes, setNotes] = useState<Note[] | null>(null)
  const [day, setDay] = useState(todayISO())
  const [text, setText] = useState('')
  const [edit, setEdit] = useState<Note | null>(null)
  const [del, setDel] = useState<Note | null>(null)
  const [busy, setBusy] = useState(false)
  const load = () => api.get<{ notes: Note[] }>(`${V1}/sites/${site.id}/notes`).then(r => setNotes([...r.notes].reverse())).catch(() => setNotes([]))
  useEffect(() => { load() }, [site.id]) // eslint-disable-line react-hooks/exhaustive-deps
  const save = async (e?: React.FormEvent) => {
    e?.preventDefault()
    if (!text.trim() || busy) return
    setBusy(true)
    try {
      if (edit) await api.put(`${V1}/notes/${edit.id}`, { day, text })
      else await api.post(`${V1}/sites/${site.id}/notes`, { day, text })
      setText(''); setEdit(null); setDay(todayISO())
      await load(); onChanged()
    } catch (x) { toast(errMsg(x), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title="What happened when" onClose={onClose} footer={<button type="button" className="btn primary" onClick={onClose}>Done</button>}>
      <p className="muted small note-lead">A note marks a day on the chart — a newsletter, a new design, a mention somewhere — so a rise or a drop explains itself later.</p>
      <form className="note-form" onSubmit={save}>
        <Field label="Day"><input type="date" value={day} max={todayISO()} onChange={e => setDay(e.target.value)} required /></Field>
        <Field label="What happened" className="grow"><input value={text} onChange={e => setText(e.target.value)} placeholder="The newsletter went out" maxLength={200} autoFocus /></Field>
        <button type="submit" className="btn primary" disabled={!text.trim() || busy}>{edit ? 'Save' : 'Add'}</button>
        {edit && <button type="button" className="btn" onClick={() => { setEdit(null); setText(''); setDay(todayISO()) }}>Cancel</button>}
      </form>
      {!notes ? <Loading /> : !notes.length ? <p className="muted small">No notes yet.</p> : <div className="list note-list">
        {notes.map(n => (
          <div key={n.id} className="list-row">
            <Icon name="flag" size={15} className="accent-ink" />
            <div className="grow"><div>{n.text}</div><div className="muted small">{fmtDate(n.day)}</div></div>
            <button type="button" className="btn ghost sm" onClick={() => { setEdit(n); setDay(n.day); setText(n.text) }}>Edit</button>
            <button type="button" className="btn ghost icon-only sm" aria-label="Delete" onClick={() => setDel(n)}><Icon name="trash" size={15} /></button>
          </div>
        ))}
      </div>}
      {del && <Confirm title="Delete this note?" message={`“${del.text}” on ${fmtDate(del.day)} goes from the chart.`} confirmLabel="Delete" onCancel={() => setDel(null)} onConfirm={async () => {
        try { await api.del(`${V1}/notes/${del.id}`); await load(); onChanged() } catch (x) { toast(errMsg(x), 'err') }
        setDel(null)
      }} />}
    </Modal>
  )
}
