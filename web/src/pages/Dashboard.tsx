// One website: the numbers, the chart, where people came from, what they
// read, where they are, what they used and did — every row filters.
import React, { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useData } from '../lib/data'
import type { Conversion, Funnel, Row, Site, StepCount, Stats } from '../lib/types'
import { DID_DIMS, DIM_LABEL, FILTERABLE, RANGES, countryName, delta, deviceIcon, eventLabel, flag, fmtDur, fmtMoney, fmtNum, fmtPct, useView, valueLabel } from '../lib/analytics'
import { Empty, Loading, SiteIcon, Modal, type MenuItem, MenuButton, Segmented, errMsg, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { MainChart } from '../components/MainChart'
import { useCrumb } from '../components/Layout'
import { Select } from '../components/Select'
import { Keywords, NotesSheet, PagePaths, Robots, Speed, When } from '../components/Insights'
import { AIPanel, Journeys, Landing, Reading, Searches } from '../components/Usage'

type Metric = 'visitors' | 'visits' | 'pageviews' | 'views_per_visit' | 'bounce_rate' | 'duration'

const METRICS: { key: Metric; label: string; fmt: (v: number) => string; lower?: boolean; series: (p: Stats['series'][0]) => number }[] = [
  { key: 'visitors', label: 'Visitors', fmt: fmtNum, series: p => p.visitors },
  { key: 'visits', label: 'Visits', fmt: fmtNum, series: p => p.visits },
  { key: 'pageviews', label: 'Pages seen', fmt: fmtNum, series: p => p.pageviews },
  { key: 'views_per_visit', label: 'Pages per visit', fmt: v => v.toFixed(1), series: p => (p.visits ? p.pageviews / p.visits : 0) },
  { key: 'bounce_rate', label: 'Left after one page', fmt: fmtPct, lower: true, series: p => p.bounce_rate },
  { key: 'duration', label: 'Visit length', fmt: fmtDur, series: p => p.duration },
]

export default function Dashboard() {
  const { id } = useParams()
  const { sites } = useData()
  const site = sites?.find(s => String(s.id) === id)
  const view = useView()
  const [stats, setStats] = useState<Stats | null>(null)
  const [err, setErr] = useState('')
  const [metric, setMetric] = useState<Metric>('visitors')
  const [explain, setExplain] = useState(false)
  const [notes, setNotes] = useState(false)
  useCrumb(site?.label)
  const reload = () => api.get<Stats>(`${V1}/sites/${id}/stats?${view.query}`).then(setStats).catch(() => {})

  useEffect(() => {
    let live = true
    setErr('')
    api.get<Stats>(`${V1}/sites/${id}/stats?${view.query}`).then(s => { if (live) setStats(s) }).catch(e => { if (live) setErr(errMsg(e)) })
    return () => { live = false }
  }, [id, view.query])
  // today's numbers move: look again each minute, quietly
  useEffect(() => {
    if (view.range !== 'today') return
    const t = window.setInterval(() => {
      if (document.visibilityState === 'visible') api.get<Stats>(`${V1}/sites/${id}/stats?${view.query}`).then(setStats).catch(() => {})
    }, 60000)
    return () => window.clearInterval(t)
  }, [id, view.query, view.range])

  if (err) return <Empty title="This site could not be opened" icon="alert">{err}</Empty>
  if (!stats || !site) return <Loading />
  if (!site.installed) return <NotYet site={site} />
  const m = METRICS.find(x => x.key === metric)!
  // the page looked at, when the filters name exactly one: its way in and out shows
  const onPage = view.filters.find(f => f.dim === 'page' && !f.not)?.value
  const onEvent = view.filters.find(f => f.dim === 'event' && !f.not)?.value
  // a bucket of the period before that ended before the site counted anything has no number, not a 0
  const counted = (end: number) => stats.site.first_hit > 0 && end > stats.site.first_hit
  const prevCounted = stats.previous_series.map((p, i) => counted(stats.previous_series[i + 1]?.start ?? stats.series[0]?.start ?? p.start))
  const hasBefore = prevCounted.some(Boolean)
  return (
    <div className="dash">
      <div className="dash-head">
        <div className="dash-title">
          <h1><SiteIcon site={site} size={22} className="dash-ico" />{site.label}</h1>
          <Link to={`/sites/${site.id}/live`} className="live-pill" title="People on the site in the last 5 minutes"><span className="live-dot" />{stats.live} now</Link>
        </div>
        <div className="dash-tools">
          <RangePicker />
          <ExplainButton onClick={() => setExplain(true)} />
        </div>
      </div>
      <FilterBar />
      <div className="kpis">
        {METRICS.map(x => {
          const now = stats.totals[x.key] as number, before = stats.previous[x.key] as number
          const d = hasBefore ? delta(now, before, x.lower) : null
          return (
            <button key={x.key} type="button" className={`kpi ${metric === x.key ? 'on' : ''}`} onClick={() => setMetric(x.key)} aria-pressed={metric === x.key}>
              <span className="kpi-l">{x.label}</span>
              <span className="kpi-v tnum">{x.fmt(now)}</span>
              <span className={`kpi-d ${d ? (d.good ? 'good' : 'bad') : ''}`} title={hasBefore ? `Before: ${x.fmt(before)}` : 'Nothing was counted in the period before'}>{d ? <><Icon name={d.dir === 'up' ? 'trending-up' : d.dir === 'down' ? 'trending-down' : 'minus'} size={13} />{d.text}</> : <span className="muted">—</span>}</span>
            </button>
          )
        })}
      </div>
      <div className="card chart-card">
        <div className="chart-head">
          <span className="muted small">{m.label} · {stats.range.label}{hasBefore && <span className="legend-prev">the period before</span>}</span>
          <button type="button" className="btn ghost sm" onClick={() => setNotes(true)} title="Mark what happened on a day"><Icon name="flag" size={14} />Notes{stats.notes?.length ? <span className="count muted">{stats.notes.length}</span> : null}</button>
          <IntervalPicker stats={stats} />
        </div>
        <MainChart starts={stats.series.map(p => p.start)} values={stats.series.map(m.series)} previous={hasBefore ? stats.previous_series.map((p, i) => (prevCounted[i] ? m.series(p) : undefined)) : undefined} interval={stats.range.interval} format={m.fmt} now={stats.now} label={m.label} notes={stats.notes} tz={stats.site.tz} />
      </div>
      <div className="panels">
        {onPage && <PagePaths site={site} page={onPage} />}
        <Panel site={site} title="Where they came from" icon="compass" tabs={[
          { dim: 'channel', label: 'Channels' }, { dim: 'source', label: 'Sources' }, { dim: 'referrer', label: 'Links' }, { dim: 'utm_campaign', label: 'Campaigns' }, { dim: 'keywords', label: 'Search words' },
        ]} />
        <Panel site={site} title="What they read" icon="file" tabs={[
          { dim: 'page', label: 'Pages', extra: 'page' }, { dim: 'entry', label: 'Came in on' }, { dim: 'exit', label: 'Left from' }, ...(site.options.not_found ? [{ dim: 'notfound', label: 'Not found' }] : []),
        ]} />
        <Landing site={site} />
        <Journeys site={site} />
        <Panel site={site} title="Where they are" icon="globe" tabs={[
          { dim: 'country', label: 'Countries' }, { dim: 'region', label: 'Regions' }, { dim: 'city', label: 'Cities' }, { dim: 'language', label: 'Languages' },
        ]} />
        <Panel site={site} title="What they used" icon="monitor" tabs={[
          { dim: 'device', label: 'Devices' }, { dim: 'browser', label: 'Browsers' }, { dim: 'os', label: 'Systems' },
        ]} />
        {site.options.identify && <Panel site={site} title="Who they are" icon="users" tabs={[
          { dim: 'network', label: 'Networks' }, { dim: 'ip', label: 'Addresses' }, { dim: 'returning', label: 'New or back' }, ...(site.options.browser_id ? [{ dim: 'screen', label: 'Screens' }, { dim: 'timezone', label: 'Time zones' }] : []),
        ]} />}
        <Goals site={site} />
        {onEvent ? <EventDetails site={site} event={onEvent} /> : <Panel site={site} title="What they did" icon="zap" tabs={[
          { dim: 'event', label: 'Events' }, ...(site.options.forms ? [{ dim: 'form', label: 'Forms' }] : []), ...(site.options.search ? [{ dim: 'search', label: 'Searches' }] : []), ...(site.options.clicks ? [{ dim: 'click', label: 'Clicks' }] : []),
        ]} />}
        <Reading site={site} page={onPage && !onPage.includes('*') ? onPage : undefined} />
        <Panel site={site} title="Links they followed" icon="external" tabs={[
          { dim: 'outbound', label: 'Other sites' }, { dim: 'download', label: 'Files' }, { dim: 'contact', label: 'Email & phone' },
        ]} />
        <When site={site} />
        <Speed site={site} />
        <AIPanel site={site} />
        {stats.robots && <Robots site={site} />}
        <Funnels site={site} />
      </div>
      {explain && <ExplainSheet site={site} onClose={() => setExplain(false)} />}
      {notes && <NotesSheet site={site} onClose={() => setNotes(false)} onChanged={reload} />}
    </div>
  )
}

// ---- range ----

function RangePicker() {
  const view = useView()
  const [custom, setCustom] = useState(false)
  const label = view.range === 'custom' ? `${view.from} – ${view.to}` : RANGES.find(r => r.key === view.range)?.label || 'Last 30 days'
  const items: MenuItem[] = [
    ...RANGES.map(r => ({ label: r.label, checked: view.range === r.key, hint: r.hint, onSelect: () => view.setRange(r.key) })),
    'sep', { label: 'Pick dates…', icon: 'calendar', checked: view.range === 'custom', onSelect: () => setCustom(true) },
  ]
  return <>
    <MenuButton label="Time range" className="btn range-btn" items={items} align="right" width={220}><Icon name="calendar" size={15} />{label}<Icon name="chevron-down" size={14} /></MenuButton>
    {custom && <CustomRange onClose={() => setCustom(false)} />}
  </>
}

function CustomRange({ onClose }: { onClose: () => void }) {
  const view = useView()
  const today = new Date().toISOString().slice(0, 10)
  const [a, setA] = useState(view.from || today)
  const [b, setB] = useState(view.to || today)
  return (
    <Modal title="Pick dates" onClose={onClose} size="sm" footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={() => { view.setRange('custom', a, b); onClose() }}>Show</button></>}>
      <div className="row gap">
        <label className="field grow"><span className="field-label">From</span><input type="date" value={a} max={today} onChange={e => setA(e.target.value)} /></label>
        <label className="field grow"><span className="field-label">To</span><input type="date" value={b} max={today} onChange={e => setB(e.target.value)} /></label>
      </div>
    </Modal>
  )
}

function IntervalPicker({ stats }: { stats: Stats }) {
  const view = useView()
  const days = Math.round((new Date(stats.range.to).getTime() - new Date(stats.range.from).getTime()) / 86400000) + 1
  const opts = ([['hour', 'Hours', days <= 7], ['day', 'Days', days <= 750 && days > 1], ['week', 'Weeks', days >= 14], ['month', 'Months', days >= 60]] as const).filter(o => o[2])
  if (opts.length < 2) return null
  return <Segmented size="sm" label="Show by" value={stats.range.interval} onChange={v => view.setInterval(v)} options={opts.map(([v, l]) => ({ value: v, label: l }))} />
}

// ---- filters ----

function FilterBar() {
  const view = useView()
  if (!view.filters.length) return null
  return (
    <div className="filter-bar" aria-label="Filters">
      <Icon name="sliders" size={15} className="muted" />
      {view.filters.map(f => (
        <span key={f.dim} className="fchip">
          <span className="muted">{DIM_LABEL[f.dim] || f.dim} {f.not ? 'is not' : 'is'}</span>
          <b className="ellipsis">{f.dim === 'country' ? `${flag(f.value)} ${countryName(f.value)}` : valueLabel(f.dim, f.value)}</b>
          <button type="button" className="fchip-x" aria-label={`Remove the ${DIM_LABEL[f.dim] || f.dim} filter`} onClick={() => view.removeFilter(f.dim)}><Icon name="x" size={13} /></button>
        </span>
      ))}
      {view.filters.length > 1 && <button type="button" className="btn ghost sm" onClick={view.clearFilters}>Clear all</button>}
    </div>
  )
}

// ---- the lists ----

type Tab = { dim: string; label: string; extra?: string }

function rowLabel(dim: string, r: Row): React.ReactNode {
  if (dim === 'country') return <><span className="flag" aria-hidden="true">{flag(r.value)}</span>{countryName(r.value)}</>
  if ((dim === 'region' || dim === 'city' || dim === 'ip' || dim === 'postal') && r.country) return <><span className="flag" aria-hidden="true">{flag(r.country)}</span>{r.value}</>
  if (dim === 'device') return <><Icon name={deviceIcon(r.value)} size={14} className="muted" />{r.value}</>
  if (dim === 'source' && r.value !== 'Direct') return <><Favicon host={r.value} />{r.value}</>
  if (dim === 'referrer') return <><Favicon host={r.value.split('/')[0]} />{r.value}</>
  if (dim === 'contact') return <><Icon name={r.value.includes('@') ? 'mail' : 'phone'} size={14} className="muted" />{r.value}</>
  if (dim === 'from') return r.value === '(none)' ? <i className="muted">Not known (typed, a bookmark, an app)</i> : r.value.startsWith('/') ? <><Icon name="file" size={14} className="muted" />{r.value}<span className="muted small">this site</span></> : <><Favicon host={r.value.split('/')[0]} />{r.value}</>
  return valueLabel(dim, r.value)
}

/** A small site icon from the site itself; nothing when it has none. */
function Favicon({ host }: { host: string }) {
  const [bad, setBad] = useState(false)
  if (bad || !/\./.test(host)) return <Icon name="globe" size={14} className="muted" />
  return <img className="fav" src={`https://${host}/favicon.ico`} alt="" width={14} height={14} loading="lazy" referrerPolicy="no-referrer" onError={() => setBad(true)} />
}

function useRows(siteID: number, dim: string, limit: number, extra = '') {
  const view = useView()
  const [rows, setRows] = useState<Row[] | null>(null)
  useEffect(() => {
    if (dim === 'keywords') return // Google's words have their own list
    let live = true
    api.get<{ rows: Row[] }>(`${V1}/sites/${siteID}/breakdown?${view.query}&dim=${dim}&limit=${limit}${extra}`).then(r => { if (live) setRows(r.rows) }).catch(() => { if (live) setRows([]) })
    return () => { live = false }
  }, [siteID, dim, limit, view.query, extra])
  return rows
}

function Panel({ site, title, icon, tabs }: { site: Site; title: string; icon: string; tabs: Tab[] }) {
  const [tab, setTab] = useState(tabs[0].dim)
  const t = tabs.find(x => x.dim === tab) || tabs[0]
  const rows = useRows(site.id, t.dim, 8)
  const [more, setMore] = useState(false)
  const [missing, setMissing] = useState('')
  const onPick = t.dim === 'notfound' ? (r: Row) => setMissing(r.value) : undefined
  return (
    <section className="card panel">
      <div className="panel-head">
        <h3><Icon name={icon} size={15} />{title}</h3>
        {tabs.length > 1 && <div className="ptabs" role="tablist">
          {tabs.map(x => <button key={x.dim} type="button" role="tab" aria-selected={x.dim === t.dim} className={`ptab ${x.dim === t.dim ? 'on' : ''}`} onClick={() => setTab(x.dim)}>{x.label}</button>)}
        </div>}
      </div>
      {t.dim === 'keywords' ? <Keywords site={site} /> : t.dim === 'search' ? <Searches site={site} /> : <RowList dim={t.dim} rows={rows} onPick={onPick} />}
      {t.dim !== 'keywords' && t.dim !== 'search' && rows && rows.length >= 8 && <button type="button" className="panel-more" onClick={() => setMore(true)}>Show all<Icon name="chevron-right" size={14} /></button>}
      {more && <AllRows site={site} tab={t} onClose={() => setMore(false)} onPick={onPick} />}
      {missing && <MissingSheet site={site} path={missing} onClose={() => setMissing('')} />}
    </section>
  )
}

const EMPTY: Record<string, string> = {
  event: 'No events in this time. Your own events show here: a sign-up, an order, a button.',
  utm_campaign: 'No campaign links (utm_campaign) in this time. Make one under Site settings → Campaign links.',
  notfound: 'Nobody landed on a page that does not exist. Good.',
  search: 'No searches in this time. They are counted when the address of your search page holds the words (?q=…).',
  form: 'No forms sent in this time.',
  contact: 'No email or phone links pressed in this time.',
  outbound: 'No links to other sites followed in this time.',
  download: 'No files downloaded in this time.',
}

function RowList({ dim, rows, wide, onPick, pickTitle, head }: { dim: string; rows: Row[] | null; wide?: boolean; onPick?: (r: Row) => void; pickTitle?: string; head?: string }) {
  const view = useView()
  if (!rows) return <div className="rows rows-loading"><Loading /></div>
  if (!rows.length) return <div className="rows rows-empty muted small">{EMPTY[dim] || 'Nothing in this time.'}</div>
  const max = Math.max(1, ...rows.map(r => r.visitors))
  const can = FILTERABLE.has(dim) && !(dim === 'event')
  const isPage = dim === 'page'
  return (
    <div className={`rows ${wide ? 'wide' : ''}`}>
      <div className="rows-h"><span>{head || DIM_LABEL[dim] || dim}</span>{wide && isPage && <><span className="num">Time</span><span className="num">Read</span></>}{wide && !isPage && rows[0].visits !== undefined && rows[0].visits > 0 && <span className="num">Left at once</span>}<span className="num">{DID_DIMS.has(dim) ? 'People' : 'Visitors'}</span></div>
      {rows.map(r => {
        const inner = <>
          <span className="rbar" style={{ width: `${(r.visitors / max) * 100}%` }} />
          <span className="rlabel ellipsis" title={r.value}>{rowLabel(dim, r)}</span>
          {wide && isPage && <><span className="num muted">{fmtDur(r.time_on_page)}</span><span className="num muted">{r.scroll ? fmtPct(r.scroll) : '—'}</span></>}
          {wide && !isPage && r.visits !== undefined && r.visits > 0 && <span className="num muted">{fmtPct(r.bounce_rate)}</span>}
          <span className="num tnum">{fmtNum(r.visitors)}</span>
        </>
        return onPick ? <button key={r.value} type="button" className="rrow" title={pickTitle || 'Where the link to it is'} onClick={() => onPick(r)}>{inner}</button>
          : can ? <button key={r.value + (r.country || '')} type="button" className="rrow" title={`Only ${DIM_LABEL[dim]?.toLowerCase()} ${valueLabel(dim, r.value)}`} onClick={() => view.addFilter(dim, r.value)}>{inner}</button>
          : dim === 'event' ? <button key={r.value} type="button" className="rrow" title={`Only visits with “${eventLabel(r.value)}”, and its details`} onClick={() => view.addFilter('event', r.value)}>{inner}</button>
          : <div key={r.value + (r.country || '')} className="rrow static">{inner}</div>
      })}
    </div>
  )
}

function AllRows({ site, tab, onClose, onPick }: { site: Site; tab: Tab; onClose: () => void; onPick?: (r: Row) => void }) {
  const view = useView()
  const rows = useRows(site.id, tab.dim, 100)
  return (
    <Modal title={tab.label} onClose={() => onClose()} size="lg" footer={<>
      <a className="btn" href={`${V1}/sites/${site.id}/export.csv?${view.query}&dim=${tab.dim}`} download><Icon name="download" size={15} />Download as a spreadsheet</a>
      <button type="button" className="btn primary" onClick={onClose}>Done</button>
    </>}>
      <div onClick={e => { if ((e.target as HTMLElement).closest('.rrow:not(.static)')) onClose() }}><RowList dim={tab.dim} rows={rows} wide onPick={onPick} /></div>
    </Modal>
  )
}

/** A page that does not exist: the pages whose links lead to it. */
function MissingSheet({ site, path, onClose }: { site: Site; path: string; onClose: () => void }) {
  const rows = useRows(site.id, 'prop', 50, `&event=404&key=from&path=${encodeURIComponent(path)}`)
  return (
    <Modal title={<>Who links to <code>{path}</code></>} onClose={onClose} size="lg" footer={<button type="button" className="btn primary" onClick={onClose}>Done</button>}>
      <p className="muted small note-lead">This page does not exist, yet people land on it. These are the pages with the link: fix the link there, or send this address on to the right page.</p>
      <RowList dim="from" rows={rows} wide />
    </Modal>
  )
}

/** One event up close: what was sent with it (plan: pro, type: pickup…). */
function EventDetails({ site, event }: { site: Site; event: string }) {
  const view = useView()
  const [keys, setKeys] = useState<string[] | null>(null)
  const [key, setKey] = useState('')
  useEffect(() => {
    let live = true
    setKeys(null)
    api.get<{ keys?: string[] }>(`${V1}/sites/${site.id}/events?event=${encodeURIComponent(event)}`).then(r => { if (live) { setKeys(r.keys || []); setKey(k => (r.keys || []).includes(k) ? k : (r.keys || [])[0] || '') } }).catch(() => { if (live) setKeys([]) })
    return () => { live = false }
  }, [site.id, event])
  const rows = useRows(site.id, 'prop', 8, `&event=${encodeURIComponent(event)}&key=${encodeURIComponent(key)}`)
  // "On which page": where the event happened (also all there is when nothing more was sent)
  const [pagesTab, setPagesTab] = useState(false)
  const onPages = pagesTab || (keys !== null && !keys.length)
  const [pages, setPages] = useState<Row[] | null>(null)
  useEffect(() => {
    if (!onPages) return
    let live = true
    setPages(null)
    api.get<{ rows: Row[] }>(`${V1}/sites/${site.id}/event-pages?${view.query}&event=${encodeURIComponent(event)}&limit=8`).then(r => { if (live) setPages(r.rows) }).catch(() => { if (live) setPages([]) })
    return () => { live = false }
  }, [site.id, event, onPages, view.query])
  return (
    <section className="card panel">
      <div className="panel-head">
        <h3><Icon name="zap" size={15} /><span className="ellipsis">{eventLabel(event)}</span></h3>
        <button type="button" className="btn ghost sm" onClick={() => view.removeFilter('event')}><Icon name="chevron-left" size={14} />All events</button>
      </div>
      {!keys ? <div className="rows rows-loading"><Loading /></div> : <>
        <div className="ptabs detail-keys" role="tablist">
          {keys.map(k => <button key={k} type="button" role="tab" aria-selected={!onPages && k === key} className={`ptab ${!onPages && k === key ? 'on' : ''}`} onClick={() => { setKey(k); setPagesTab(false) }}>{k}</button>)}
          <button type="button" role="tab" aria-selected={onPages} className={`ptab ${onPages ? 'on' : ''}`} onClick={() => setPagesTab(true)}>On which page</button>
        </div>
        {onPages ? <RowList dim="prop" head="Page" rows={pages} /> : <RowList dim="prop" head={key} rows={key ? rows : null} />}
        {!keys.length && <p className="muted small sp-note">Nothing more was sent with this event. Details come with it like this: <code>pocket('{event}', {'{'} props: {'{'} plan: 'pro' {'}'} {'}'})</code></p>}
      </>}
    </section>
  )
}

// ---- goals and funnels ----

function Goals({ site }: { site: Site }) {
  const view = useView()
  const [data, setData] = useState<{ goals: Conversion[] } | null>(null)
  useEffect(() => {
    let live = true
    api.get<{ goals: Conversion[] }>(`${V1}/sites/${site.id}/conversions?${view.query}`).then(d => { if (live) setData(d) }).catch(() => { if (live) setData({ goals: [] }) })
    return () => { live = false }
  }, [site.id, view.query])
  const goals = data?.goals
  const money = !!goals?.some(g => g.revenue?.length)
  return (
    <section className="card panel">
      <div className="panel-head">
        <h3><Icon name="target" size={15} />Goals</h3>
        <Link className="btn ghost sm" to={`/sites/${site.id}/settings/goals`}><Icon name="plus" size={14} />Add</Link>
      </div>
      {!goals ? <div className="rows rows-loading"><Loading /></div>
        : !goals.length ? <div className="rows rows-empty muted small">Goals count what matters: an order, a sign-up, a page reached. <Link to={`/sites/${site.id}/settings/goals`}>Add your first goal</Link>.</div>
        : <div className={`rows ${money ? 'has-money' : ''}`}>
          <div className="rows-h"><span>Goal</span>{money && <span className="num money">Brought</span>}<span className="num rate">Rate</span><span className="num">People</span></div>
          {goals.map(g => g.robot ? (
            <div key={g.goal.id} className="rrow static" title={`Read by ${g.goal.robot || 'any robot'}: ${g.goal.match}. Robots are not visits, so this is a count of times and the filters above do not apply.`}>
              <span className="rlabel ellipsis"><Icon name="server" size={13} className="muted" />{g.goal.name}</span>
              {money && <span className="num money muted">—</span>}
              <span className="num muted rate small">times</span>
              <span className="num tnum">{fmtNum(g.conversions)}</span>
            </div>
          ) : (
            <button key={g.goal.id} type="button" className="rrow" title="Only visits that reached it" onClick={() => view.addFilter('goal', String(g.goal.id))}>
              <span className="rbar goal" style={{ width: `${Math.min(100, g.rate)}%` }} />
              <span className="rlabel ellipsis"><Icon name={g.goal.kind === 'event' ? 'zap' : 'file'} size={13} className="muted" />{g.goal.name}</span>
              {money && <span className="num money tnum" title={(g.revenue || []).map(m => fmtMoney(m.amount, m.currency)).join(' + ')}>{g.revenue?.length ? <>{fmtMoney(g.revenue[0].amount, g.revenue[0].currency)}{g.revenue.length > 1 && <span className="muted"> +{g.revenue.length - 1}</span>}</> : <span className="muted">—</span>}</span>}
              <span className="num muted rate">{g.rate.toFixed(1)}%</span>
              <span className="num tnum">{fmtNum(g.visitors)}</span>
            </button>
          ))}
        </div>}
    </section>
  )
}

function Funnels({ site }: { site: Site }) {
  const view = useView()
  const funnels = site.funnels
  const [full, setFull] = useState<Site | null>(null)
  useEffect(() => { api.get<Site>(`${V1}/sites/${site.id}`).then(setFull).catch(() => {}) }, [site.id])
  const list: Funnel[] = (funnels || full?.funnels || [])
  const [fid, setFid] = useState(0)
  const cur = list.find(f => f.id === fid) || list[0]
  const [steps, setSteps] = useState<StepCount[] | null>(null)
  useEffect(() => {
    if (!cur) return
    let live = true
    setSteps(null)
    api.get<{ steps: StepCount[] }>(`${V1}/sites/${site.id}/funnels/${cur.id}/counts?${view.query}`).then(r => { if (live) setSteps(r.steps) }).catch(() => { if (live) setSteps([]) })
    return () => { live = false }
  }, [site.id, cur?.id, view.query]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!full) return null
  return (
    <section className="card panel panel-wide">
      <div className="panel-head">
        <h3><Icon name="trending-down" size={15} />Funnels</h3>
        {list.length > 1 && <Select ariaLabel="Funnel" value={String(cur?.id || '')} onChange={v => setFid(Number(v))} options={list.map(f => ({ value: String(f.id), label: f.name }))} />}
        <Link className="btn ghost sm" to={`/sites/${site.id}/settings/funnels`}><Icon name="plus" size={14} />{list.length ? 'Edit' : 'Add'}</Link>
      </div>
      {!cur ? <div className="rows rows-empty muted small">A funnel shows where people drop off on the way to something: menu → order form → ordered. <Link to={`/sites/${site.id}/settings/funnels`}>Make a funnel</Link>.</div>
        : !steps ? <div className="rows rows-loading"><Loading /></div>
        : <div className="funnel">
          {steps.map((s, i) => (
            <div key={i} className="fstep">
              <div className="fstep-top"><span className="fstep-n">{i + 1}</span><span className="ellipsis">{s.step.label || s.step.match}</span><span className="num tnum">{fmtNum(s.visitors)}</span></div>
              <div className="fstep-bar"><span style={{ width: `${steps[0].visitors ? (s.visitors / steps[0].visitors) * 100 : 0}%` }} /></div>
              <div className="fstep-foot muted small">{i === 0 ? 'started' : s.robot ? <>times read by {s.step.robot || 'robots'} · {s.next.toFixed(0)}% of the step before (robots are not tied to a visit, so this is a ratio, not people)</> : <>{s.next.toFixed(0)}% went on · {fmtNum(s.dropped)} left here · {s.rate.toFixed(1)}% of all who started</>}</div>
            </div>
          ))}
        </div>}
    </section>
  )
}

// ---- Pocket AI ----

function ExplainButton({ onClick }: { onClick: () => void }) {
  const { settings } = useApp()
  if (settings && !settings.ai_help) return null
  return <button type="button" className="btn" onClick={onClick} title="Pocket AI explains the numbers"><Icon name="sparkles" size={15} />Explain</button>
}

function ExplainSheet({ site, onClose }: { site: Site; onClose: () => void }) {
  const view = useView()
  const { toast } = useToast()
  const [text, setText] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const [saving, setSaving] = useState('')
  const run = (fresh = false) => {
    setBusy(true); setErr('')
    api.post<{ text: string }>(`${V1}/sites/${site.id}/explain?${view.query}${fresh ? '&fresh=1' : ''}`).then(r => setText(r.text)).catch(e => setErr(errMsg(e))).finally(() => setBusy(false))
  }
  useEffect(() => { run() }, []) // eslint-disable-line react-hooks/exhaustive-deps
  const keep = async (target: 'note' | 'task') => {
    setSaving(target)
    try {
      const label = RANGES.find(r => r.key === view.range)?.label || `${view.from} – ${view.to}`
      const r = await api.post<{ app: string; url: string }>(`${V1}/sites/${site.id}/make`, { target, title: target === 'note' ? `${site.label}: ${label.toLowerCase()}` : `Look at ${site.label}'s numbers`, text })
      toast(`Saved in ${r.app}`, 'ok')
    } catch (e) { toast(errMsg(e), 'err') } finally { setSaving('') }
  }
  const html = useMemo(() => md(text), [text])
  return (
    <Modal title={<><Icon name="sparkles" size={16} /> What the numbers say</>} onClose={onClose} size="lg" footer={<>
      <button type="button" className="btn" disabled={!text || !!saving} onClick={() => keep('note')}><Icon name="note" size={15} />Keep as a note</button>
      <button type="button" className="btn" disabled={!text || !!saving} onClick={() => keep('task')}><Icon name="check-square" size={15} />Make a task</button>
      <span className="spacer" />
      <button type="button" className="btn" disabled={busy} onClick={() => run(true)}><Icon name="refresh" size={15} />Again</button>
      <button type="button" className="btn primary" onClick={onClose}>Done</button>
    </>}>
      {busy && !text && <Loading label="Pocket AI is reading the numbers…" />}
      {err && <div className="callout warn"><Icon name="alert" size={16} />{err}</div>}
      {text && <div className={`ai-text ${busy ? 'is-busy' : ''}`} dangerouslySetInnerHTML={{ __html: html }} />}
    </Modal>
  )
}

/** A very small Markdown: headings, bullets, bold — escaped first. */
function md(s: string): string {
  const esc = s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  const out: string[] = []
  let list = false
  for (const raw of esc.split('\n')) {
    const line = raw.trim()
    const b = line.replace(/\*\*(.+?)\*\*/g, '<b>$1</b>')
    if (/^[-*•] /.test(line)) { if (!list) { out.push('<ul>'); list = true } out.push(`<li>${b.replace(/^[-*•] /, '')}</li>`); continue }
    if (list) { out.push('</ul>'); list = false }
    if (/^#{1,4} /.test(line)) out.push(`<h4>${b.replace(/^#{1,4} /, '')}</h4>`)
    else if (/^\d+\. /.test(line)) out.push(`<h4>${b.replace(/^\d+\. /, '')}</h4>`)
    else if (line) out.push(`<p>${b}</p>`)
  }
  if (list) out.push('</ul>')
  return out.join('')
}

// ---- before the first visit ----

export function NotYet({ site }: { site: Site }) {
  const navigate = useNavigate()
  return (
    <div className="notyet">
      <div className="notyet-ico"><Icon name="activity" size={28} /></div>
      <h1>Waiting for the first visit to {site.domain}</h1>
      <p className="muted">Put the line below in the <code>&lt;head&gt;</code> of every page of your site. The first visit shows here within seconds.</p>
      <Snippet site={site} />
      <div className="row gap center">
        <CheckButton site={site} />
        <button type="button" className="btn" onClick={() => navigate(`/sites/${site.id}/settings/install`)}><Icon name="help" size={15} />Other ways to install</button>
      </div>
    </div>
  )
}

export function Snippet({ site, code }: { site: Site; code?: string }) {
  const { toast } = useToast()
  const text = code || site.snippet
  return (
    <div className="snippet">
      <pre><code>{text}</code></pre>
      <button type="button" className="btn sm" onClick={() => navigator.clipboard?.writeText(text).then(() => toast('Copied', 'ok'))}><Icon name="copy" size={14} />Copy</button>
    </div>
  )
}

export function CheckButton({ site }: { site: Site }) {
  const { toast } = useToast()
  const { reload } = useData()
  const [busy, setBusy] = useState(false)
  const check = async () => {
    setBusy(true)
    try {
      const r = await api.get<{ installed: boolean }>(`${V1}/sites/${site.id}/check`)
      if (r.installed) { toast('Visits are arriving', 'ok'); reload() } else toast('Nothing yet. Open your site in another tab, then check again.', 'ok')
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return <button type="button" className="btn primary" disabled={busy} onClick={check}><Icon name="refresh" size={15} />{busy ? 'Checking…' : 'Check now'}</button>
}
