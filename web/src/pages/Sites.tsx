// Every website, with today, the last 30 days and the people on it now.
import { useCallback, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { useData } from '../lib/data'
import { delta, fmtNum } from '../lib/analytics'
import { Sparkline } from '../components/Chart'
import { Icon } from '../components/icons'
import { SiteIcon } from '../components/ui'
import { openCreate } from '../components/Layout'

export default function Sites() {
  const { sites } = useData()
  if (!sites) return null
  if (!sites.length) return (
    <div className="welcome">
      <img src="/favicon.svg" alt="" width={72} height={72} />
      <h1>See who visits your websites</h1>
      <p className="muted">Add a website, put one small line in its pages, and you see visitors, pages, where people came from, countries, devices and what they clicked — without cookies, without a banner, and nothing that follows a person around.</p>
      <button type="button" className="btn primary lg" onClick={() => openCreate('site')}><Icon name="plus" size={16} />Add a website</button>
      <ul className="welcome-points">
        <li><Icon name="shield" size={16} />The IP address is never kept; a visitor cannot be followed from one day to the next.</li>
        <li><Icon name="zap" size={16} />A 3 KB script that does not slow your site down.</li>
        <li><Icon name="sparkles" size={16} />Pocket AI explains the numbers in plain words.</li>
      </ul>
    </div>
  )
  return (
    <div className="page">
      <div className="page-head"><h1 className="page-title">Your websites</h1></div>
      <div className="site-grid">
        {sites.map(s => {
          const d = delta(s.visitors_30d || 0, s.visitors_prev_30d || 0)
          return (
            <Link key={s.id} to={`/sites/${s.id}`} className="site-card">
              <div className="sc-head">
                <SiteIcon site={s} size={18} className="sc-fav" />
                <span className="sc-name ellipsis">{s.label}</span>
                {!!s.live && <span className="live-pill sm"><span className="live-dot" />{s.live}</span>}
              </div>
              <div className="sc-domain muted small ellipsis">{s.domain}</div>
              {s.installed ? <>
                <div className="sc-nums">
                  <div><span className="sc-v tnum">{fmtNum(s.today)}</span><span className="muted small">today</span></div>
                  <div><span className="sc-v tnum">{fmtNum(s.visitors_30d)}</span><span className="muted small">30 days {d && <span className={d.good ? 'good' : 'bad'}>{d.text}</span>}</span></div>
                </div>
                <Spark values={s.spark || []} />
              </> : <div className="sc-wait"><Icon name="hourglass" size={15} />Waiting for the first visit — <span className="link-like">set it up</span></div>}
            </Link>
          )
        })}
        <button type="button" className="site-card add" onClick={() => openCreate('site')}><Icon name="plus" size={22} /><span>Add a website</span></button>
      </div>
    </div>
  )
}

/** The last 30 days as a small line, as wide as the card. */
function Spark({ values }: { values: number[] }) {
  const [w, setW] = useState(240)
  const ro = useRef<ResizeObserver | null>(null)
  const ref = useCallback((el: HTMLDivElement | null) => {
    ro.current?.disconnect(); ro.current = null
    if (!el) return
    const measure = () => { const cw = Math.round(el.getBoundingClientRect().width); if (cw > 0) setW(cw) }
    measure(); ro.current = new ResizeObserver(measure); ro.current.observe(el)
  }, [])
  return <div ref={ref} className="sc-spark"><Sparkline values={values} color="var(--accent)" width={w} height={40} /></div>
}
