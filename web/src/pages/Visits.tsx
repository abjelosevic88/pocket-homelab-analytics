// Visits one by one: where each came from, which pages in which order, how
// long — never who it was.
import { useEffect, useState } from 'react'
import { useParams, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useData } from '../lib/data'
import type { Hit, Visit } from '../lib/types'
import { RANGES, countryName, eventLabel, deviceIcon, flag, fmtDur, fmtMoney, fmtSpeed, languageName, rate, useView } from '../lib/analytics'
import { fmtDateTime } from '../lib/format'
import { Empty, Loading, type MenuItem, MenuButton, Modal } from '../components/ui'
import { Icon } from '../components/icons'
import { useCrumb } from '../components/Layout'

const PAGE = 50

export default function Visits() {
  const { id } = useParams()
  const { sites } = useData()
  const site = sites?.find(s => String(s.id) === id)
  const view = useView()
  const [sp, setSp] = useSearchParams()
  const [list, setList] = useState<Visit[] | null>(null)
  const [total, setTotal] = useState(0)
  const [busy, setBusy] = useState(false)
  useCrumb(site ? `${site.label} · visits` : undefined)
  useEffect(() => {
    let on = true
    setList(null)
    api.get<{ visits: Visit[]; total: number }>(`${V1}/sites/${id}/visits?${view.query}&limit=${PAGE}`).then(r => { if (on) { setList(r.visits); setTotal(r.total) } }).catch(() => { if (on) setList([]) })
    return () => { on = false }
  }, [id, view.query])
  const more = () => {
    if (!list) return
    setBusy(true)
    api.get<{ visits: Visit[] }>(`${V1}/sites/${id}/visits?${view.query}&limit=${PAGE}&offset=${list.length}`).then(r => setList([...list, ...r.visits])).finally(() => setBusy(false))
  }
  const open = Number(sp.get('visit') || 0)
  const setOpen = (v: number) => setSp(prev => { const q = new URLSearchParams(prev); if (v) q.set('visit', String(v)); else q.delete('visit'); return q }, { replace: true })
  const rangeItems: MenuItem[] = RANGES.map(r => ({ label: r.label, checked: view.range === r.key, onSelect: () => view.setRange(r.key) }))
  return (
    <div className="page">
      <div className="page-head row">
        <div className="grow">
          <h1 className="page-title">Visits</h1>
          <p className="muted small">{list ? `${total.toLocaleString()} ${total === 1 ? 'visit' : 'visits'}` : ' '}{view.filters.length ? ' with the filters from the site page' : ''}. A visit is one person's pages with less than 30 minutes between them.</p>
        </div>
        <MenuButton label="Time range" className="btn range-btn" items={rangeItems} align="right" width={200}><Icon name="calendar" size={15} />{RANGES.find(r => r.key === view.range)?.label || 'Dates'}<Icon name="chevron-down" size={14} /></MenuButton>
      </div>
      {!list ? <Loading /> : !list.length ? <Empty title="No visits in this time" icon="activity" /> : <div className="card visit-list">
        {list.map(v => (
          <button key={v.id} type="button" className="visit-row" onClick={() => setOpen(v.id)}>
            <span className="vr-when tnum">{fmtDateTime(new Date(v.started * 1000).toISOString())}</span>
            <span className="vr-path ellipsis"><b>{v.entry}</b>{v.pageviews > 1 && <span className="muted"> → {v.pageviews - 1} more{v.exit !== v.entry ? ` · left from ${v.exit}` : ''}</span>}</span>
            <span className="vr-meta muted small ellipsis"><span className="flag">{flag(v.country)}</span>{v.city || countryName(v.country)} · <Icon name={deviceIcon(v.device)} size={12} /> {v.browser} · {v.source}</span>
            <span className="vr-dur muted small tnum">{fmtDur(v.last - v.started || v.engaged / 1000)}</span>
          </button>
        ))}
        {list.length < total && <button type="button" className="panel-more" disabled={busy} onClick={more}>{busy ? 'Loading…' : `Show ${Math.min(PAGE, total - list.length)} more`}</button>}
      </div>}
      {open > 0 && <VisitSheet siteID={Number(id)} visitID={open} onClose={() => setOpen(0)} />}
    </div>
  )
}

// ---- one visit as a story: the pages are the steps, and what the person
// did on a page sits under it ----

interface Act { hit: Hit; out?: string; pressed?: boolean; led?: string }
interface Step { n: number; page?: Hit; times: number; acts: Act[]; via?: Act }
interface Story { steps: Step[]; longest: number; left: string; leftVia?: Act }

const OUT = 'Outbound Link: Click'

function tell(hits: Hit[]): Story {
  const steps: Step[] = []
  const seen: Record<string, number> = {}
  let cur: Step | undefined
  let left = ''
  let leftVia: Act | undefined
  hits.forEach((h, i) => {
    // a site's own event right before a new page, a link out, a file or an email
    // link was the press that did it: "Menu", then /docs
    const prev = cur?.acts[cur.acts.length - 1]
    const mine = prev && !(prev.hit.name in ACT_ICON) ? prev : undefined
    if (h.name === 'pageview') {
      const via = mine && h.ts - mine.hit.ts <= 2 ? mine : undefined
      if (via) { via.pressed = true; via.led = h.path; cur!.acts.pop() }
      seen[h.path] = (seen[h.path] || 0) + 1
      cur = { n: Object.values(seen).reduce((a, b) => a + b, 0), page: h, times: seen[h.path], acts: [], via }
      steps.push(cur)
      return
    }
    if (!cur) { cur = { n: 0, times: 0, acts: [] }; steps.push(cur) }
    const last = i === hits.length - 1
    if (h.name === OUT) {
      const url = h.props?.url || ''
      // a named press and the link it followed are one thing: "Open on GitHub", then out
      const same = prev && prev.hit.name !== OUT && !prev.out && Math.abs(h.ts - prev.hit.ts) <= 1
      if (last) left = url
      if (same) { prev.pressed = true; if (last) { leftVia = prev; cur.acts.pop() } else prev.out = url; return }
      if (last && url) return
    }
    if (mine && h.name in ACT_ICON && h.name !== '404' && h.name !== 'Search' && Math.abs(h.ts - mine.hit.ts) <= 1) mine.pressed = true
    cur.acts.push({ hit: h })
  })
  return { steps, left, leftVia, longest: Math.max(0, ...steps.map(x => x.page?.engaged || 0)) }
}

/** 0:45 since the visit began (1:02:07 after an hour). */
function clock(sec: number): string {
  const s = Math.max(0, Math.round(sec))
  const m = Math.floor(s / 60) % 60, h = Math.floor(s / 3600)
  return `${h ? `${h}:${String(m).padStart(2, '0')}` : m}:${String(s % 60).padStart(2, '0')}`
}

const short = (url: string) => url.replace(/^https?:\/\/(www\.)?/, '').replace(/\/$/, '')
const hostOf = (url: string) => short(url).split('/')[0]
const ACT_ICON: Record<string, string> = { [OUT]: 'external', 'File Download': 'download', 'Contact Link: Click': 'mail', 'Form: Submission': 'send', Search: 'search', '404': 'alert', Click: 'hand' }
// the detail that says what it was, by event; for a site's own events, the first one
const ACT_MAIN: Record<string, string> = { [OUT]: 'url', 'File Download': 'url', 'Contact Link: Click': 'to', 'Form: Submission': 'form', Search: 'term', '404': 'from', Click: 'text' }
// details that say where on the page: the word is enough
const PLACE = new Set(['where', 'place', 'position', 'placement', 'location', 'section'])

/** What an act was, in a few words, and the rest of it for a tip. */
function actWords(act: Act, started: number) {
  const h = act.hit
  const own = !(h.name in ACT_ICON) // an event the site itself names: "Menu", "Order placed"
  const props = Object.entries(h.props || {}).filter(([, v]) => v !== '')
  const mainKey = ACT_MAIN[h.name] || props.find(([k]) => !PLACE.has(k))?.[0]
  const main = props.find(([k]) => k === mainKey)?.[1]
  const rest = props.filter(([k]) => k !== mainKey && !(h.name === 'Contact Link: Click' && k === 'kind'))
  const shown = !main ? '' : mainKey === 'url' ? (h.name === 'File Download' ? main.split('/').pop() || short(main) : short(main)) : h.name === 'Search' || h.name === 'Click' ? `“${main}”` : main
  const tip = [
    `${clock(h.ts - started)} into the visit`,
    main && mainKey !== 'url' ? `${mainKey}: ${main}` : main || '',
    ...rest.map(([k, v]) => `${k}: ${v}`),
    act.out ? `went to ${short(act.out)}` : '',
    own ? `“${h.name}” is the name your site gives it` : '',
  ].filter(Boolean).join(' · ')
  const icon = own ? 'check-circle' : h.name === 'Contact Link: Click' && h.props?.kind === 'phone' ? 'phone' : ACT_ICON[h.name]
  return { label: eventLabel(h.name), shown, tip, icon }
}

/** The link or button that brought them to the next page: "via Menu". */
function Via({ act, started }: { act?: Act; started: number }) {
  if (!act) return null
  const w = actWords(act, started)
  return <span className="vt-via" title={w.tip}>via {w.label}</span>
}

/** One visit as a table like the ones on the site page: a line per page, in
 * the order they were opened, with a bar for the time spent on it. */
function VisitStory({ visit, hits }: { visit: Visit; hits: Hit[] }) {
  const story = tell(hits)
  return (
    <div className="rows wide vt">
      <div className="rows-h"><span>Pages, in order</span><span className="num">Time</span><span className="num vt-read">Read</span></div>
      {story.steps.map((x, i) => {
        const p = x.page
        const slow = p?.lcp ? rate('lcp', p.lcp) : 'good'
        return (
          <div key={p ? p.id : `a${i}`} className="vt-step">
            {p && <div className="rrow static" title={`${clock(p.ts - visit.started)} into the visit${p.lcp ? ` · showed in ${fmtSpeed('lcp', p.lcp)}` : ''}`}>
              <span className="rbar" style={{ width: `${p.engaged > 0 && story.longest ? Math.max(1, p.engaged / story.longest * 100) : 0}%` }} />
              <span className="rlabel grow">
                <b className="ellipsis">{p.path}</b>
                {x.times > 1 && <span className="vt-via">again</span>}
                <Via act={x.via} started={visit.started} />
                {slow !== 'good' && <span className={`vt-via sp-${slow}`}><i className="sp-dot" aria-hidden="true" />slow to show</span>}
              </span>
              <span className="num tnum">{p.engaged >= 1000 ? fmtDur(p.engaged / 1000) : <span className="muted">–</span>}</span>
              <span className="num tnum vt-read">{p.scroll > 0 ? `${p.scroll}%` : <span className="muted">–</span>}</span>
            </div>}
            {x.acts.length > 0 && <div className="vt-acts">
              {x.acts.map(a => {
                const w = actWords(a, visit.started)
                return <span key={a.hit.id} title={w.tip}><Icon name={w.icon} size={13} />{w.label}{w.shown ? ` · ${w.shown}` : ''}{a.hit.revenue ? <span className="badge ok-badge">{fmtMoney(a.hit.revenue, a.hit.currency)}</span> : null}</span>
              })}
            </div>}
          </div>
        )
      })}
      {story.left && <div className="rrow static vt-left">
        <span className="rlabel grow"><Icon name="external" size={14} className="muted" /><span className="ellipsis">Left for <a href={story.left} target="_blank" rel="noreferrer noopener">{short(story.left)}</a></span><Via act={story.leftVia} started={visit.started} /></span>
      </div>}
    </div>
  )
}

function VisitSheet({ siteID, visitID, onClose }: { siteID: number; visitID: number; onClose: () => void }) {
  const [data, setData] = useState<{ visit: Visit; hits: Hit[] } | null>(null)
  const [err, setErr] = useState(false)
  useEffect(() => { api.get<{ visit: Visit; hits: Hit[] }>(`${V1}/sites/${siteID}/visits/${visitID}`).then(setData).catch(() => setErr(true)) }, [siteID, visitID])
  const v = data?.visit
  return (
    <Modal title="One visit" onClose={onClose} size="lg">
      {err ? <Empty title="This visit is gone" icon="alert" /> : !v || !data ? <Loading /> : <>
        <p className="vt-when">{fmtDateTime(new Date(v.started * 1000).toISOString())}{v.last - v.started >= 1 ? <span className="muted"> · {fmtDur(v.last - v.started)}</span> : null}</p>
        <div className="vt-who">
          <span title={[v.city, v.region, countryName(v.country)].filter(Boolean).join(', ')}><span className="flag">{flag(v.country)}</span>{[v.city, countryName(v.country)].filter(Boolean).join(', ')}</span>
          <span title={v.device}><Icon name={deviceIcon(v.device)} size={13} />{v.browser} {v.browser_version} · {v.os} {v.os_version}</span>
          {v.language && <span title="The language of their browser"><Icon name="globe" size={13} />{languageName(v.language)}</span>}
          <span title="Where they came from"><Icon name="link" size={13} />{v.source}{v.referrer ? ` (${v.referrer})` : ''}{v.utm_campaign ? ` · “${v.utm_campaign}”` : ''}</span>
        </div>
        <VisitStory visit={v} hits={data.hits} />
        <p className="muted small">Nothing here tells who this was: no IP address, no cookie. The same person tomorrow is a new visitor.</p>
      </>}
    </Modal>
  )
}
