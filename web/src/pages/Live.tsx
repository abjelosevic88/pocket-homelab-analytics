// Right now: the people on the site in the last minutes, what they read,
// where they came from — updated every ten seconds, in place.
import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useData } from '../lib/data'
import type { Live as LiveT, Row } from '../lib/types'
import { ago, countryName, eventLabel, deviceIcon, flag, fmtNum } from '../lib/analytics'
import { Loading } from '../components/ui'
import { Icon } from '../components/icons'
import { useCrumb } from '../components/Layout'

export default function Live() {
  const { id } = useParams()
  const { sites } = useData()
  const site = sites?.find(s => String(s.id) === id)
  const [live, setLive] = useState<LiveT | null>(null)
  const [, tick] = useState(0)
  useCrumb(site ? `${site.label} · now` : undefined)
  useEffect(() => {
    let on = true
    const load = () => { if (document.visibilityState === 'visible') api.get<LiveT>(`${V1}/sites/${id}/live`).then(l => { if (on) setLive(l) }).catch(() => {}) }
    load()
    const t = window.setInterval(() => { load(); tick(x => x + 1) }, 10000)
    return () => { on = false; window.clearInterval(t) }
  }, [id])
  if (!live || !site) return <Loading />
  const max = Math.max(1, ...live.minutes)
  return (
    <div className="page live">
      <div className="live-hero card">
        <div className="live-now"><span className="live-dot big" /><span className="tnum">{live.now}</span></div>
        <div className="muted">{live.now === 1 ? 'person' : 'people'} on {site.label} in the last 5 minutes</div>
        <div className="minutes" aria-label="Pages seen per minute, the last 30 minutes">
          {live.minutes.map((m, i) => <span key={i} className="minute" style={{ height: `${Math.max(3, (m / max) * 100)}%` }} title={`${30 - i} min ago: ${m} ${m === 1 ? 'page' : 'pages'}`} />)}
        </div>
        <div className="minutes-axis muted small"><span>30 min ago</span><span>now</span></div>
      </div>
      <div className="panels three">
        <LiveList title="Reading now" icon="file" rows={live.pages} empty="Nobody right now." />
        <LiveList title="Came from" icon="compass" rows={live.sources} empty="—" />
        <LiveList title="Where" icon="globe" rows={live.countries} country empty="—" />
      </div>
      <section className="card panel">
        <div className="panel-head"><h3><Icon name="activity" size={15} />The last half hour</h3><Link to={`/sites/${site.id}/visits?range=today`} className="btn ghost sm">All visits today</Link></div>
        {!live.recent.length ? <div className="rows rows-empty muted small">No pages seen in the last 30 minutes.</div> : <div className="feed">
          {live.recent.map((h, i) => (
            <Link key={i} to={`/sites/${site.id}/visits?range=today&visit=${h.visit_id}`} className="feed-row">
              <span className="feed-when muted small tnum">{ago(h.ts)}</span>
              <span className="feed-what ellipsis">{h.name === 'pageview' ? <><Icon name="file" size={13} className="muted" />{h.path}</> : <><Icon name="zap" size={13} className="accent-ink" />{eventLabel(h.name)}</>}</span>
              <span className="feed-who muted small ellipsis"><span className="flag">{flag(h.country)}</span>{h.city || countryName(h.country)} · <Icon name={deviceIcon(h.device)} size={12} /> {h.browser} · {h.source}</span>
            </Link>
          ))}
        </div>}
      </section>
    </div>
  )
}

function LiveList({ title, icon, rows, country, empty }: { title: string; icon: string; rows: Row[]; country?: boolean; empty: string }) {
  const max = Math.max(1, ...rows.map(r => r.visitors))
  return (
    <section className="card panel">
      <div className="panel-head"><h3><Icon name={icon} size={15} />{title}</h3></div>
      {!rows.length ? <div className="rows rows-empty muted small">{empty}</div> : <div className="rows">
        {rows.map(r => (
          <div key={r.value} className="rrow static">
            <span className="rbar" style={{ width: `${(r.visitors / max) * 100}%` }} />
            <span className="rlabel ellipsis">{country ? <><span className="flag">{flag(r.value)}</span>{countryName(r.value)}</> : r.value}</span>
            <span className="num tnum">{fmtNum(r.visitors)}</span>
          </div>
        ))}
      </div>}
    </section>
  )
}
