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

function VisitSheet({ siteID, visitID, onClose }: { siteID: number; visitID: number; onClose: () => void }) {
  const [data, setData] = useState<{ visit: Visit; hits: Hit[] } | null>(null)
  const [err, setErr] = useState(false)
  useEffect(() => { api.get<{ visit: Visit; hits: Hit[] }>(`${V1}/sites/${siteID}/visits/${visitID}`).then(setData).catch(() => setErr(true)) }, [siteID, visitID])
  const v = data?.visit
  return (
    <Modal title="One visit" onClose={onClose} size="lg">
      {err ? <Empty title="This visit is gone" icon="alert" /> : !v || !data ? <Loading /> : <>
        <dl className="visit-facts">
          <div><dt>When</dt><dd>{fmtDateTime(new Date(v.started * 1000).toISOString())} · {fmtDur(v.last - v.started)}</dd></div>
          <div><dt>Came from</dt><dd>{v.source}{v.referrer ? ` (${v.referrer})` : ''}{v.utm_campaign ? ` · campaign “${v.utm_campaign}”` : ''}</dd></div>
          <div><dt>Where</dt><dd><span className="flag">{flag(v.country)}</span>{[v.city, v.region, countryName(v.country)].filter(Boolean).join(', ')}</dd></div>
          <div><dt>With</dt><dd>{v.device} · {v.browser} {v.browser_version} · {v.os} {v.os_version}{v.language ? ` · ${languageName(v.language)}` : ''}</dd></div>
        </dl>
        <ol className="journey">
          {data.hits.map((h, i) => (
            <li key={h.id} className={h.name === 'pageview' ? 'jp' : 'je'}>
              <span className="j-at muted small tnum">+{fmtDur(h.ts - v.started)}</span>
              <span className="j-ico"><Icon name={h.name === 'pageview' ? 'file' : 'zap'} size={13} /></span>
              <span className="j-what">
                {h.name === 'pageview' ? <b>{h.path}</b> : <><b>{eventLabel(h.name)}</b>{h.revenue ? <span className="badge ok-badge">{fmtMoney(h.revenue, h.currency)}</span> : null}{h.props && <span className="muted small"> {Object.entries(h.props).map(([k, val]) => `${k}: ${val || '—'}`).join(' · ')}</span>}</>}
                {h.name === 'pageview' && (h.engaged > 0 || h.scroll > 0) && <span className="muted small"> · {fmtDur(h.engaged / 1000)} on the page{h.scroll ? `, read ${h.scroll}% down` : ''}</span>}
                {h.name === 'pageview' && h.lcp ? <span className={`small sp-inline sp-${rate('lcp', h.lcp)}`} title="How long until the main part of the page was on the screen"> · <i className="sp-dot" aria-hidden="true" />showed in {fmtSpeed('lcp', h.lcp)}</span> : null}
                {i === 0 && <span className="badge">came in</span>}
              </span>
            </li>
          ))}
        </ol>
        <p className="muted small">Nothing here tells who this was: no IP address, no cookie. The same person tomorrow is a new visitor.</p>
      </>}
    </Modal>
  )
}
