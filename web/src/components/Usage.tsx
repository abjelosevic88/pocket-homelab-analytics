// How people use a site: the pages they land on and how those visits go,
// the ways they take through it, how far down a page they read, what AI
// assistants do with it, what they searched for, and the robots without a name.
import React, { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import type { AIView, Goal, Journey, Landing as LandingT, Reading as ReadingT, RobotAgent, SearchRow, Site } from '../lib/types'
import { ago, fmtDur, fmtNum, fmtPct, useView } from '../lib/analytics'
import { Loading, Modal, Segmented } from './ui'
import { Select } from './Select'
import { Icon } from './icons'

/** Loads one of a site's lists for the range and filters in the address. */
function useSiteData<T>(siteID: number, what: string, extra = ''): T | null {
  const view = useView()
  const [data, setData] = useState<T | null>(null)
  useEffect(() => {
    let live = true
    setData(null)
    api.get<T>(`${V1}/sites/${siteID}/${what}?${view.query}${extra}`).then(d => { if (live) setData(d) }).catch(() => { if (live) setData(null) })
    return () => { live = false }
  }, [siteID, what, view.query, extra])
  return data
}

/** The site's goals people can reach in a visit (robot goals are not part of a visit). */
function useVisitGoals(siteID: number): Goal[] {
  const [goals, setGoals] = useState<Goal[]>([])
  useEffect(() => {
    let live = true
    api.get<Site>(`${V1}/sites/${siteID}`).then(s => { if (live) setGoals((s.goals || []).filter(g => g.kind !== 'robot')) }).catch(() => {})
    return () => { live = false }
  }, [siteID])
  return goals
}

function GoalPick({ goals, value, onChange }: { goals: Goal[]; value: string; onChange: (v: string) => void }) {
  if (!goals.length) return null
  return <Select ariaLabel="Which goal counts" value={value} onChange={onChange} options={[{ value: '', label: 'Any goal' }, ...goals.map(g => ({ value: String(g.id), label: g.name }))]} />
}

// ---- the pages visits start on ----

/** Each page visits start on: how many left at once, how long they stayed, and how many reached a goal. */
export function Landing({ site }: { site: Site }) {
  const view = useView()
  const goals = useVisitGoals(site.id)
  const [goal, setGoal] = useState('')
  const [all, setAll] = useState(false)
  const data = useSiteData<{ rows: LandingT[]; goals: boolean }>(site.id, 'landing', `&limit=${all ? 100 : 8}${goal ? `&goal=${goal}` : ''}`)
  const max = Math.max(1, ...(data?.rows || []).map(r => r.visits))
  return (
    <section className="card panel panel-wide">
      <div className="panel-head">
        <h3><Icon name="sunrise" size={15} />The pages people land on</h3>
        <GoalPick goals={goals} value={goal} onChange={setGoal} />
      </div>
      {!data ? <div className="rows rows-loading short"><Loading /></div>
        : !data.rows.length ? <div className="rows rows-empty short muted small">No visits in this time.</div>
        : <>
          <div className="rows wide us-table">
            <div className="rows-h"><span>Came in on</span><span className="num">Visits</span><span className="num" title="Saw only this page and did nothing else">Left at once</span><span className="num us-hide" title="Time with the site in front, per visit">Time</span><span className="num us-hide" title="Pages per visit">Pages</span>{data.goals && <span className="num" title="Visits that reached a goal">Reached goal</span>}</div>
            {data.rows.map(r => (
              <button key={r.page} type="button" className="rrow" title={`Only visits that came in on ${r.page}`} onClick={() => view.addFilter('entry', r.page)}>
                <span className="rbar" style={{ width: `${(r.visits / max) * 100}%` }} />
                <span className="rlabel ellipsis" title={r.page}>{r.page}</span>
                <span className="num tnum">{fmtNum(r.visits)}</span>
                <span className={`num tnum ${r.bounce >= 70 ? 'us-bad' : 'muted'}`}>{fmtPct(r.bounce)}</span>
                <span className="num tnum muted us-hide">{fmtDur(r.engaged)}</span>
                <span className="num tnum muted us-hide">{r.pages.toFixed(1)}</span>
                {data.goals && <span className={`num tnum ${r.converted ? 'us-good' : 'muted'}`} title={`${fmtNum(r.converted)} of ${fmtNum(r.visits)} visits`}>{fmtPct(r.rate)}</span>}
              </button>
            ))}
          </div>
          {data.rows.length >= 8 && !all && <button type="button" className="panel-more" onClick={() => setAll(true)}>Show all<Icon name="chevron-right" size={14} /></button>}
          <p className="muted small sp-note">The first page decides a lot: a high “left at once” means people did not find a reason to look further. {data.goals ? 'A page goal counts when people get to it from here, not when they land on it.' : 'Add a goal (an install, a sign-up) to see which first pages lead to it.'}</p>
        </>}
    </section>
  )
}

// ---- the ways through the site ----

/** The most common first pages of a visit, in order. */
export function Journeys({ site }: { site: Site }) {
  const view = useView()
  const goals = useVisitGoals(site.id)
  const [steps, setSteps] = useState(3)
  const [goal, setGoal] = useState('')
  const [all, setAll] = useState(false)
  const data = useSiteData<{ rows: Journey[]; visits: number }>(site.id, 'journeys', `&steps=${steps}&limit=${all ? 50 : 8}${goal ? `&goal=${goal}` : ''}`)
  const max = Math.max(1, ...(data?.rows || []).map(r => r.visits))
  const hasGoals = goals.length > 0
  return (
    <section className="card panel panel-wide">
      <div className="panel-head">
        <h3><Icon name="route" size={15} />The ways people take</h3>
        <Segmented size="sm" label="Pages" value={steps} onChange={setSteps} options={[{ value: 2, label: '2 pages' }, { value: 3, label: '3' }, { value: 4, label: '4' }]} />
        <GoalPick goals={goals} value={goal} onChange={setGoal} />
      </div>
      {!data ? <div className="rows rows-loading short"><Loading /></div>
        : !data.rows.length ? <div className="rows rows-empty short muted small">No visits in this time.</div>
        : <>
          <div className="rows wide">
            <div className="rows-h"><span>First pages of a visit</span><span className="num us-hide" title="Of these, visits that went on to more pages">Went on</span>{hasGoals && <span className="num" title="Of these, visits that reached a goal">Reached goal</span>}<span className="num">Visits</span></div>
            {data.rows.map(r => (
              <button key={r.pages.join('>')} type="button" className="rrow" title={`Only visits that came in on ${r.pages[0]}`} onClick={() => view.addFilter('entry', r.pages[0])}>
                <span className="rbar" style={{ width: `${(r.visits / max) * 100}%` }} />
                <span className="rlabel us-way">
                  {r.pages.map((p, i) => <React.Fragment key={i}>{i > 0 && <Icon name="chevron-right" size={12} className="muted" />}<span className="us-step ellipsis" title={p}>{p}</span></React.Fragment>)}
                  {r.pages.length < steps && <span className="muted small us-left">then left</span>}
                </span>
                <span className="num tnum muted us-hide">{r.more ? fmtNum(r.more) : '—'}</span>
                {hasGoals && <span className={`num tnum ${r.converted ? 'us-good' : 'muted'}`}>{r.converted ? fmtNum(r.converted) : '—'}</span>}
                <span className="num tnum">{fmtNum(r.visits)}</span>
              </button>
            ))}
          </div>
          {data.rows.length >= 8 && !all && <button type="button" className="panel-more" onClick={() => setAll(true)}>Show more<Icon name="chevron-right" size={14} /></button>}
          <p className="muted small sp-note">{fmtNum(data.visits)} visits, by their first {steps} pages (a reload counts once). Filter by a source above to see the way people from Google or ChatGPT take.</p>
        </>}
    </section>
  )
}

// ---- how far down a page ----

/** How far down a page people read: by scroll depth, and by the parts the site marks with data-pocket-section. */
export function Reading({ site, page: pinned }: { site: Site; page?: string }) {
  const [page, setPage] = useState(pinned || '')
  useEffect(() => { if (pinned) setPage(pinned) }, [pinned])
  const data = useSiteData<{ reading: ReadingT; marked: string[] }>(site.id, 'reading', page ? `&page=${encodeURIComponent(page)}` : '')
  const rd = data?.reading
  const options = Array.from(new Set([...(data?.marked || []), ...(rd ? [rd.page] : [])]))
  return (
    <section className="card panel">
      <div className="panel-head">
        <h3><Icon name="layers" size={15} />How far they read</h3>
        {options.length > 1 && <Select ariaLabel="Page" value={rd?.page || page} onChange={setPage} options={options.map(p => ({ value: p, label: p }))} onCreate={v => { setPage(v); return v }} createLabel="Show" />}
      </div>
      {!rd ? <div className="rows rows-loading"><Loading /></div>
        : !rd.views ? <div className="rows rows-empty muted small">Nobody saw {rd.page} in this time.</div>
        : <div className="rows us-read">
          <div className="rows-h"><span>Got this far down <code>{rd.page}</code></span><span className="num">Views</span></div>
          {rd.depth.map(d => (
            <div key={d.at} className="rrow static">
              <span className="rbar" style={{ width: `${d.rate}%` }} />
              <span className="rlabel">{d.at === 100 ? 'To the end' : `${d.at}% of the page`}</span>
              <span className="num tnum">{rd.measured ? fmtPct(d.rate) : '—'}</span>
            </div>
          ))}
          {rd.sections.length ? <>
            <div className="rows-h us-sub"><span>Parts of the page they had on screen</span><span className="num">Views</span></div>
            {rd.sections.map(s => (
              <div key={s.name} className="rrow static" title={`${fmtNum(s.reached)} of ${fmtNum(rd.marked)} views`}>
                <span className="rbar goal" style={{ width: `${s.rate}%` }} />
                <span className="rlabel ellipsis">{s.name}</span>
                <span className="num tnum">{fmtPct(s.rate)}</span>
              </div>
            ))}
          </> : <p className="muted small us-hint">Mark the parts of a page to see which ones people reach: <code>{'<section data-pocket-section="Pricing">'}</code></p>}
          <p className="muted small sp-note">{fmtNum(rd.views)} views{rd.measured < rd.views ? `, ${fmtNum(rd.measured)} with how far down` : ''}.</p>
        </div>}
    </section>
  )
}

// ---- AI assistants ----

/** What AI assistants do with the site: the pages their robots read, and the people their answers send. */
export function AIPanel({ site }: { site: Site }) {
  const view = useView()
  const data = useSiteData<AIView>(site.id, 'ai', '&limit=10')
  if (data && !data.reads && !data.visits) return null
  const maxR = Math.max(1, ...(data?.pages || []).map(p => p.reads))
  const maxV = Math.max(1, ...(data?.pages || []).map(p => p.visits))
  return (
    <section className="card panel panel-wide">
      <div className="panel-head"><h3><Icon name="sparkles" size={15} />AI assistants</h3></div>
      {!data ? <div className="rows rows-loading short"><Loading /></div> : <>
        <div className="sp-tiles us-tiles">
          <div className="sp-tile"><span className="sp-l">People they sent</span><span className="sp-v tnum">{fmtNum(data.visits)}</span><span className="sp-r muted ellipsis">{data.sources.map(s => `${s.name} ${s.n}`).join(' · ') || '—'}</span></div>
          <div className="sp-tile"><span className="sp-l">Pages their robots read</span><span className="sp-v tnum">{fmtNum(data.reads)}</span><span className="sp-r muted ellipsis">{data.robots.slice(0, 3).map(s => `${s.name} ${fmtNum(s.n)}`).join(' · ') || '—'}</span></div>
        </div>
        <div className="rows wide">
          <div className="rows-h"><span>Page</span><span className="num" title="Times AI robots read it">Read</span><span className="num" title="People who came from an AI assistant and landed here">People</span></div>
          {data.pages.map(p => (
            <button key={p.page} type="button" className="rrow" title={`Only visits that saw ${p.page}`} onClick={() => view.addFilter('page', p.page)}>
              <span className="rbar" style={{ width: `${(p.visits ? p.visits / maxV : p.reads / maxR / 4) * 100}%` }} />
              <span className="rlabel ellipsis" title={p.page}>{p.page}</span>
              <span className="num tnum muted">{p.reads ? fmtNum(p.reads) : '—'}</span>
              <span className="num tnum">{p.visits ? fmtNum(p.visits) : '—'}</span>
            </button>
          ))}
        </div>
        <p className="muted small sp-note">Pages AI robots read are what assistants can answer from; pages with people are where their answers point. A page read often but never visited is known but not recommended. Robots are never counted as visitors.</p>
      </>}
    </section>
  )
}

// ---- searches ----

/** What people searched for on the site, and what found nothing. */
export function Searches({ site }: { site: Site }) {
  const [empty, setEmpty] = useState(false)
  const data = useSiteData<{ rows: SearchRow[]; nothing_found: number }>(site.id, 'searches', `&limit=8${empty ? '&empty=1' : ''}`)
  const max = Math.max(1, ...(data?.rows || []).map(r => r.visitors))
  const counts = !!data?.rows.some(r => r.results !== null)
  return <>
    {(empty || !!data?.nothing_found) && <Segmented size="sm" className="kw-by" label="Show" value={empty ? 'none' : 'all'} onChange={v => setEmpty(v === 'none')} options={[{ value: 'all', label: 'All' }, { value: 'none', label: `Found nothing${data?.nothing_found ? ` · ${data.nothing_found}` : ''}` }]} />}
    {!data ? <div className="rows rows-loading"><Loading /></div>
      : !data.rows.length ? <div className="rows rows-empty muted small">{empty ? 'Every search found something.' : 'No searches in this time. They are counted from ?q=… in the address, or when your site sends pocket(\'Search\', { props: { term, results } }).'}</div>
      : <div className="rows">
        <div className="rows-h"><span>Searched for</span>{counts && <span className="num" title="Results found (the fewest, when searched more than once)">Results</span>}<span className="num">People</span></div>
        {data.rows.map(r => (
          <div key={r.term} className="rrow static" title={`${r.times} ${r.times === 1 ? 'time' : 'times'}${r.where ? ` · ${r.where}` : ''}`}>
            <span className="rbar" style={{ width: `${(r.visitors / max) * 100}%` }} />
            <span className="rlabel ellipsis">{r.term}</span>
            {counts && <span className={`num tnum ${r.results === 0 ? 'us-bad' : 'muted'}`}>{r.results === null ? '—' : r.results === 0 ? 'none' : fmtNum(r.results)}</span>}
            <span className="num tnum">{fmtNum(r.visitors)}</span>
          </div>
        ))}
      </div>}
  </>
}

// ---- robots without a name ----

/** What the robots without a name call themselves (their User-Agent). */
export function UnnamedRobots({ site, onClose }: { site: Site; onClose: () => void }) {
  const data = useSiteData<{ rows: RobotAgent[] }>(site.id, 'robots/unnamed', '&limit=200')
  return (
    <Modal title="Robots without a name" size="lg" onClose={onClose} footer={<button type="button" className="btn primary" onClick={onClose}>Done</button>}>
      <p className="muted small note-lead">No rule knows these, so they are counted as “Other robots”. This is what each one called itself (a robot's User-Agent, not a person's), how often it came, and the last page it read. The most recent 500 are kept.</p>
      {!data ? <Loading /> : !data.rows.length ? <p className="muted">None seen in this time. They are kept from version 0.5.0 on.</p>
        : <div className="rows wide">
          <div className="rows-h"><span>Calls itself</span><span className="num us-hide">Last read</span><span className="num">Seen</span><span className="num">Times</span></div>
          {data.rows.map(r => (
            <div key={r.agent} className="rrow static us-agent">
              <span className="rlabel"><code className="us-ua" title={r.agent}>{r.agent || '(sent no name at all)'}</code></span>
              <span className="num muted ellipsis us-hide us-path" title={r.path}>{r.path}</span>
              <span className="num muted small">{ago(r.last)}</span>
              <span className="num tnum">{fmtNum(r.hits)}</span>
            </div>
          ))}
        </div>}
    </Modal>
  )
}
