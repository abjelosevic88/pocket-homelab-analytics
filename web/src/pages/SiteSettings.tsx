// A site's settings: installing it, what is counted, goals and funnels,
// alerts, and its data.
import React, { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { announceChange, useData } from '../lib/data'
import type { Funnel, Goal, Site, SiteOptions, Step } from '../lib/types'
import { zones } from '../lib/analytics'
import { Confirm, CopyField, Empty, Field, Loading, Modal, Segmented, SettingRow, Switch, Tabs, errMsg, useIsMobile, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { useCrumb } from '../components/Layout'
import { CheckButton, Snippet } from './Dashboard'

type TabId = 'install' | 'counting' | 'goals' | 'funnels' | 'links' | 'alerts' | 'general' | 'data'
const TABS: { id: TabId; label: string; icon: string }[] = [
  { id: 'install', label: 'Install', icon: 'code' }, { id: 'counting', label: 'What is counted', icon: 'sliders' }, { id: 'goals', label: 'Goals', icon: 'target' },
  { id: 'funnels', label: 'Funnels', icon: 'trending-down' }, { id: 'links', label: 'Campaign links', icon: 'link' }, { id: 'alerts', label: 'Alerts & report', icon: 'bell' }, { id: 'general', label: 'Name & address', icon: 'globe' },
  { id: 'data', label: 'Data', icon: 'database' },
]

export default function SiteSettings() {
  const { id, tab: raw } = useParams()
  const navigate = useNavigate()
  const mobile = useIsMobile()
  const [site, setSite] = useState<Site | null>(null)
  const [err, setErr] = useState('')
  const tab: TabId = TABS.find(t => t.id === raw)?.id || 'install'
  useCrumb(site ? `${site.label} · settings` : undefined)
  const load = () => api.get<Site>(`${V1}/sites/${id}`).then(setSite).catch(e => setErr(errMsg(e)))
  useEffect(() => { load() }, [id]) // eslint-disable-line react-hooks/exhaustive-deps
  if (err) return <Empty title="This site could not be opened" icon="alert">{err}</Empty>
  if (!site) return <Loading />
  const go = (t: TabId) => navigate(`/sites/${site.id}/settings/${t}`, { replace: true })
  return (
    <div className="page site-set">
      <div className="page-head"><h1 className="page-title">{site.label}</h1><p className="muted small">{site.domain}</p></div>
      <div className="site-set-body">
        {mobile ? <div className="scroll-x"><Tabs tabs={TABS} value={tab} onChange={go} /></div> : <nav className="set-nav side" aria-label="Site settings">
          {TABS.map(t => <button key={t.id} type="button" className={`set-nav-item ${t.id === tab ? 'active' : ''}`} onClick={() => go(t.id)}><Icon name={t.icon} size={16} />{t.label}</button>)}
        </nav>}
        <div className="site-set-main">
          {tab === 'install' && <Install site={site} />}
          {tab === 'counting' && <Counting site={site} onSaved={setSite} />}
          {tab === 'goals' && <Goals site={site} onChanged={load} />}
          {tab === 'funnels' && <Funnels site={site} onChanged={load} />}
          {tab === 'links' && <CampaignLinks site={site} />}
          {tab === 'alerts' && <Alerts site={site} onSaved={setSite} />}
          {tab === 'general' && <General site={site} onSaved={setSite} />}
          {tab === 'data' && <DataTab site={site} />}
        </div>
      </div>
    </div>
  )
}

function useSave(site: Site, onSaved: (s: Site) => void) {
  const { toast } = useToast()
  return async (patch: Partial<Site> & { options?: SiteOptions }) => {
    try {
      const s = await api.put<Site>(`${V1}/sites/${site.id}`, patch)
      onSaved({ ...site, ...s }); announceChange()
      toast('Saved', 'ok')
    } catch (e) { toast(errMsg(e), 'err') }
  }
}

function Section({ title, sub, children }: { title: string; sub?: React.ReactNode; children: React.ReactNode }) {
  return <section className="ss-sec"><h2>{title}</h2>{sub && <p className="muted">{sub}</p>}<div className="card pad-card">{children}</div></section>
}

// ---- install ----

function Install({ site }: { site: Site }) {
  const [way, setWay] = useState<'html' | 'wordpress' | 'worker' | 'spa' | 'events'>('html')
  const worker = `// In your Worker's fetch(request, env, ctx), before your own routes. Visits
// then go through your own domain: ad blockers leave them alone, and the
// visitor's place comes from Cloudflare.
const ANALYTICS = '${site.event_url.replace(/\/api\/event$/, '')}'
const KEY = '${site.proxy_key}'
const url = new URL(request.url)
if (url.pathname === '/_p/script.js') {
  const r = await fetch(ANALYTICS + '/js/script.js', { cf: { cacheTtl: 3600 } })
  return new Response(r.body, { headers: { 'Content-Type': 'application/javascript; charset=utf-8', 'Cache-Control': 'public, max-age=3600' } })
}
if (url.pathname === '/_p/event' && request.method === 'POST') {
  const h = request.headers
  return fetch(ANALYTICS + '/api/event', { method: 'POST', body: await request.text(), headers: {
    'Content-Type': 'text/plain', 'User-Agent': h.get('User-Agent') || '', 'Accept-Language': h.get('Accept-Language') || '',
    'X-Pocket-Proxy-Key': KEY, 'X-Pocket-Client-IP': h.get('CF-Connecting-IP') || '',
    'X-Pocket-Country': request.cf?.country || '', 'X-Pocket-Region': request.cf?.region || '', 'X-Pocket-City': request.cf?.city || '',
  } })
}
// Robots (search engines, AI assistants, link previews) never run the
// script. Tell Analytics about the pages they read; people are left out.
const ua = request.headers.get('User-Agent') || ''
if (request.method === 'GET' && !/\\.[a-z0-9]{2,5}$/i.test(url.pathname) && /bot|crawl|spider|slurp|GPT|OAI-|Claude|anthropic|Perplexity|facebookexternalhit|meta-external|WhatsApp|Preview|curl\\/|wget\\/|python|Go-http|uptime|monitor|lighthouse|headless|feed/i.test(ua)) {
  ctx.waitUntil(fetch(ANALYTICS + '/api/robot', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-Pocket-Proxy-Key': KEY },
    body: JSON.stringify({ d: '${site.domain}', u: request.url, ua }) }).catch(() => {}))
}`
  const proxied = site.snippet.replace(/src="[^"]*"/, 'data-api="/_p/event" src="/_p/script.js"')
  return <>
    <Section title="Put this in every page" sub={<>Paste it into the <code>&lt;head&gt;</code> of your site's pages (in the template that all pages share). That is all.</>}>
      <Snippet site={site} />
      <div className="row gap mt">
        <CheckButton site={site} />
        <span className="muted small">{site.installed ? <><Icon name="check-circle" size={14} className="ok" /> Visits arrive. The last one: {new Date(site.last_hit * 1000).toLocaleString()}</> : 'No visit has arrived yet.'}</span>
      </div>
    </Section>
    <Section title="How to add it">
      <Segmented value={way} onChange={setWay} className="mb" options={[{ value: 'html', label: 'Any site' }, { value: 'wordpress', label: 'WordPress' }, { value: 'worker', label: 'Cloudflare' }, { value: 'spa', label: 'Apps' }, { value: 'events', label: 'Your events' }]} />
      {way === 'html' && <ol className="howto">
        <li>Open the file or template every page uses (often <code>index.html</code>, <code>header.php</code>, <code>layout.html</code> or <code>_document</code>).</li>
        <li>Paste the line above just before <code>&lt;/head&gt;</code>.</li>
        <li>Publish, open your site, and press <b>Check now</b>.</li>
      </ol>}
      {way === 'wordpress' && <ol className="howto">
        <li>In WordPress, add the free plugin <b>WPCode</b> (or “Insert Headers and Footers”).</li>
        <li>Code Snippets → Header &amp; Footer → paste the line into <b>Header</b> and save.</li>
        <li>Open your site and press <b>Check now</b>.</li>
      </ol>}
      {way === 'worker' && <>
        <p className="muted small">When your site runs on Cloudflare Workers (or Pages Functions), send visits through your own domain. Add this to the Worker's <code>fetch</code>, then use the second line on your pages.</p>
        <Snippet site={site} code={worker} />
        <div className="mt"><Snippet site={site} code={proxied} /></div>
        <p className="muted small">The key proves the Worker is yours, so the place it sends is believed. Keep it out of public code; make a new one under Data if it leaks.</p>
        <p className="muted small">The last part is how <b>robots</b> are seen (which AI assistants and search engines read your site, and who shared a link). The Worker must run for your pages for it: with static assets, add your pages to <code>run_worker_first</code>.</p>
      </>}
      {way === 'spa' && <ol className="howto">
        <li>Single-page apps (React, Vue, Svelte, Next.js…) work as they are: page changes are seen through the browser history.</li>
        <li>If your app tells pages apart by <code>#…</code> in the address, switch on <b>Pages by #</b> under What is counted.</li>
        <li>In Next.js, put the line in <code>app/layout.tsx</code> (or <code>pages/_document.tsx</code>) inside <code>&lt;head&gt;</code>.</li>
      </ol>}
      {way === 'events' && <>
        <p className="muted small">Count what people do — a sign-up, an order, a button. Either mark the element:</p>
        <Snippet site={site} code={`<button data-pocket-event="Signed up" data-pocket-prop-plan="pro">Start</button>`} />
        <p className="muted small mt">or call it from your code (after the script line):</p>
        <Snippet site={site} code={`window.pocket('Order placed', { props: { type: 'pickup' } })`} />
        <p className="muted small mt">Plausible's way works too: <code>plausible('Signup')</code> and <code>class="plausible-event-name=Signup"</code>. Then make the event a goal.</p>
        <p className="muted small mt"><b>Money.</b> Say what an order was worth and the goal shows what it brought:</p>
        <Snippet site={site} code={`window.pocket('Order placed', { revenue: { amount: 24.50, currency: 'EUR' } })`} />
        <p className="muted small mt"><b>Pages that do not exist.</b> Counted by themselves when your server answers “404”. If your not-found page answers “200” (many apps do), mark it:</p>
        <Snippet site={site} code={`<body data-pocket-404>`} />
        <p className="muted small mt"><b>Searches.</b> Counted when the address of your results page holds the words (<code>/search?q=cake</code>). For a search that never changes the address:</p>
        <Snippet site={site} code={`window.pocket('Search', { props: { term: 'cake' } })`} />
        <p className="muted small mt"><b>Forms.</b> Every form sent is counted under its name (<code>id</code>, <code>name</code> or <code>aria-label</code>), never what was typed. Give one a clear name with <code>data-pocket-form="Contact"</code>, or leave one out with <code>data-pocket-ignore</code>.</p>
      </>}
    </Section>
    <Section title="Don't count yourself" sub="Open this link once in each browser you use; that browser is not counted on this site any more.">
      <a className="btn" href={site.ignore_url} target="_blank" rel="noreferrer"><Icon name="eye-off" size={15} />Stop counting this browser</a>
    </Section>
  </>
}

// ---- what is counted ----

function Counting({ site, onSaved }: { site: Site; onSaved: (s: Site) => void }) {
  const save = useSave(site, onSaved)
  const o = site.options
  const set = (patch: Partial<SiteOptions>) => save({ options: { ...o, ...patch } })
  const [exclude, setExclude] = useState(o.exclude.join('\n'))
  const [params, setParams] = useState((o.search_params || []).join(', '))
  const [ips, setIps] = useState((o.ignore_ips || []).join('\n'))
  const [finding, setFinding] = useState(false)
  const { toast } = useToast()
  useEffect(() => { setIps((o.ignore_ips || []).join('\n')) }, [(o.ignore_ips || []).join('\n')]) // eslint-disable-line react-hooks/exhaustive-deps
  const saveIps = (text: string) => { const list = text.split(/[\s,]+/).map(x => x.trim()).filter(Boolean); if (list.join('\n') !== (o.ignore_ips || []).join('\n')) set({ ignore_ips: list }) }
  // the address this browser reaches the counter from: what your own visits come from
  const addMine = async () => {
    setFinding(true)
    try {
      const r = await fetch(site.event_url.replace(/\/api\/event$/, '/api/address'), { cache: 'no-store' }).then(x => x.json()) as { ip: string }
      if (!r.ip) throw new Error('no address')
      const mine = r.ip.includes(':') ? r.ip.split(':').slice(0, 4).join(':') + '::/64' : r.ip
      const list = ips.split(/[\s,]+/).filter(Boolean)
      if (list.includes(mine)) { toast(`${mine} is already on the list`, 'ok'); return }
      const next = [...list, mine].join('\n')
      setIps(next); saveIps(next)
    } catch { toast('Could not find your address. Type it in instead.', 'err') } finally { setFinding(false) }
  }
  return <>
    <Section title="Your own visits" sub="Your visits to your own site make the numbers look better than they are. Leave them out here.">
      <Field label="Addresses not counted" help="Your home's internet address (and your phone's, at work…), one per line; a range works too: 203.0.113.0/24. Your address can change: check this when your own visits show up again.">
        <textarea rows={2} className="mono-area" value={ips} onChange={e => setIps(e.target.value)} onBlur={() => saveIps(ips)} placeholder="203.0.113.7" spellCheck={false} />
      </Field>
      <div className="row gap wrap">
        <button type="button" className="btn sm" disabled={finding} onClick={addMine}><Icon name="plus" size={14} />{finding ? 'Looking…' : 'Add the address I am on now'}</button>
        <a className="btn sm ghost" href={site.ignore_url} target="_blank" rel="noreferrer"><Icon name="eye-off" size={14} />Do not count this browser</a>
      </div>
      <p className="muted small mt">“Do not count this browser” opens your site once and remembers it in that browser only: do it on each phone and computer you use, also when you are away from home.</p>
      <Field label="The link that stops counting a browser" help="Send it to your phone or another computer and open it there once. Opening it with =0 at the end counts that browser again." className="mt" group>
        <CopyField value={site.ignore_url} label="The link that stops counting a browser" />
      </Field>
    </Section>
    <Section title="Know who visits" sub="Keeps what reaches your server anyway: each visitor's IP address, who owns it (an internet provider, a company, a data centre), roughly where on the map, and their whole browser line. Nothing is read from or kept in their browser, and browsers that ask not to be tracked (Global Privacy Control, Do Not Track) are only counted.">
      <SettingRow title="Know who visits" help="The line on your pages stays as it is. The IP address is personal data: say in your privacy notice what you keep and for how long."><Switch checked={!!o.identify} onChange={v => set({ identify: v })} /></SettingRow>
      {o.identify && <SettingRow title="Forget who they were after" help="The visits and their pages stay; the address, network, map place and details are wiped.">
        <Select value={String(o.forget_days || 0)} onChange={v => set({ forget_days: Number(v) })} options={[{ value: '0', label: 'Never' }, { value: '30', label: '30 days' }, { value: '90', label: '90 days' }, { value: '180', label: '6 months' }, { value: '365', label: '1 year' }]} />
      </SettingRow>}
      {o.identify && <SettingRow title="Browser ID and device details" help="The browser keeps a random ID, so the same person is known on another day and from another address, and tells its screen, time zone, languages and hardware (graphics chip, memory). In the EU and UK this needs the visitor's consent first: switch it on only behind a consent banner. Copy the line again after changing it."><Switch checked={!!o.browser_id} onChange={v => set({ browser_id: v })} /></SettingRow>}
    </Section>
    <Section title="What people press">
      <SettingRow title="Links to other sites" help="Which links people follow away from your site, and which email addresses and phone numbers they press."><Switch checked={o.outbound} onChange={v => set({ outbound: v })} /></SettingRow>
      <SettingRow title="Files" help="Which PDFs, zips and other files people download."><Switch checked={o.downloads} onChange={v => set({ downloads: v })} /></SettingRow>
      <SettingRow title="Forms" help="Which forms people send (contact, order, newsletter) — never what they typed."><Switch checked={o.forms} onChange={v => set({ forms: v })} /></SettingRow>
      <SettingRow title="Every link and button" help="What people press, by its words (“Order a cake”). More to store; the line to paste changes."><Switch checked={o.clicks} onChange={v => set({ clicks: v })} /></SettingRow>
    </Section>
    <Section title="Pages">
      <SettingRow title="How fast pages are" help="How long a page takes to show and to answer a press, measured in each visitor's browser."><Switch checked={o.speed} onChange={v => set({ speed: v })} /></SettingRow>
      <SettingRow title="Pages that do not exist" help="Addresses people land on that your site does not have, and the page with the link to them."><Switch checked={o.not_found} onChange={v => set({ not_found: v })} /></SettingRow>
      <SettingRow title="Searches on your site" help="What people type into your site's search. The words are read from the address of the results page."><Switch checked={o.search} onChange={v => set({ search: v })} /></SettingRow>
      {o.search && <Field label="Where the search words are" help="The names in the address that hold them, with commas: /search?q=cake → q. WordPress uses s.">
        <input value={params} onChange={e => setParams(e.target.value)} onBlur={() => { const list = params.split(/[\s,]+/).map(x => x.trim()).filter(Boolean); if (list.join(',') !== (o.search_params || []).join(',')) set({ search_params: list }) }} placeholder="q, s, search, query" autoCapitalize="off" spellCheck={false} />
      </Field>}
      <SettingRow title="Pages by #" help="For apps whose pages differ only after # in the address. The line to paste changes."><Switch checked={o.hash} onChange={v => set({ hash: v })} /></SettingRow>
      <SettingRow title="Count localhost" help="Visits from your own computer while you build the site."><Switch checked={o.localhost} onChange={v => set({ localhost: v })} /></SettingRow>
      <Field label="Pages not counted" help="One per line; * stands for anything: /admin*, /preview/*">
        <textarea rows={3} value={exclude} onChange={e => setExclude(e.target.value)} onBlur={() => { if (exclude !== o.exclude.join('\n')) set({ exclude: exclude.split('\n').map(x => x.trim()).filter(Boolean) }) }} placeholder="/admin*" />
      </Field>
    </Section>
    <Section title="The line on your pages" sub="Everything above works with the line you already have. Copy it again only after changing “Know who visits”, “Browser ID and device details”, “Every link and button” or “Pages by #”.">
      <Snippet site={site} />
    </Section>
  </>
}

// ---- campaign links ----

const MEDIUMS = [{ value: 'email', label: 'Email / newsletter' }, { value: 'social', label: 'Social post' }, { value: 'paid_social', label: 'Paid social ad' }, { value: 'cpc', label: 'Search ad' }, { value: 'qr', label: 'QR code / print' }, { value: 'referral', label: 'Another site' }]

/** Builds a link that says where a visit came from, so a newsletter or an ad shows under Campaigns. */
function CampaignLinks({ site }: { site: Site }) {
  const [page, setPage] = useState('/')
  const [source, setSource] = useState('')
  const [medium, setMedium] = useState('email')
  const [campaign, setCampaign] = useState('')
  const slug = (v: string) => v.trim().toLowerCase().replace(/\s+/g, '-').replace(/[^a-z0-9._~-]/g, '')
  let link = ''
  if (source.trim()) {
    try {
      const u = new URL(/^https?:\/\//.test(page) ? page : `https://${site.domain}${page.startsWith('/') ? '' : '/'}${page}`)
      u.searchParams.set('utm_source', slug(source))
      if (medium) u.searchParams.set('utm_medium', medium)
      if (slug(campaign)) u.searchParams.set('utm_campaign', slug(campaign))
      link = u.toString()
    } catch { link = '' }
  }
  return <>
    <Section title="A link that says where it was" sub="Emails, apps and QR codes do not tell a site where a visit came from. Put this link in them and the visits show under Where they came from → Campaigns.">
      <div className="camp-grid">
        <Field label="The page it opens" className="full" help={`A page of ${site.domain}, like /cakes — or a whole address.`}><input value={page} onChange={e => setPage(e.target.value)} placeholder="/" autoCapitalize="off" spellCheck={false} /></Field>
        <Field label="Where the link will be" help="The newsletter, the place, the partner.">
          <input value={source} onChange={e => setSource(e.target.value)} placeholder="newsletter" autoCapitalize="off" />
        </Field>
        <Field label="What kind of place"><Select value={medium} onChange={setMedium} options={MEDIUMS} /></Field>
        <Field label="Campaign (optional)" className="full" help="One name for everything that belongs together: spring-cakes."><input value={campaign} onChange={e => setCampaign(e.target.value)} placeholder="spring-cakes" autoCapitalize="off" /></Field>
      </div>
      <div className="camp-ideas">
        <span className="muted small">Quick start:</span>
        {[['newsletter', 'email'], ['instagram', 'social'], ['facebook', 'social'], ['flyer', 'qr'], ['google', 'cpc']].map(([s, m]) => <button key={s} type="button" className="btn sm" onClick={() => { setSource(s); setMedium(m) }}>{s}</button>)}
      </div>
    </Section>
    <Section title="Your link">
      {link ? <CopyField value={link} label="Campaign link" /> : <p className="muted small">Say where the link will be, and it appears here.</p>}
      <p className="muted small mt">Use it only in places outside your site. A link from one of your own pages to another needs nothing.</p>
    </Section>
  </>
}

// ---- goals ----

function Goals({ site, onChanged }: { site: Site; onChanged: () => void }) {
  const [edit, setEdit] = useState<Partial<Goal> | null>(null)
  const [del, setDel] = useState<Goal | null>(null)
  const { toast } = useToast()
  const goals = site.goals || []
  return <>
    <Section title="Goals" sub="What counts as success: a page reached (an order's thank-you page) or an event (“Signed up”). The site's page shows how many people got there.">
      {!goals.length && <p className="muted">No goals yet.</p>}
      <div className="list">
        {goals.map(g => (
          <div key={g.id} className="list-row">
            <Icon name={g.kind === 'event' ? 'zap' : g.kind === 'robot' ? 'server' : 'file'} size={16} className="muted" />
            <div className="grow"><div>{g.name}</div><div className="muted small">{g.kind === 'event' ? 'Event' : g.kind === 'robot' ? `Read by ${g.robot || 'any robot'}` : 'Page'}: <code>{g.match}</code></div></div>
            <button type="button" className="btn ghost sm" onClick={() => setEdit(g)}>Edit</button>
            <button type="button" className="btn ghost icon-only sm" aria-label="Delete" onClick={() => setDel(g)}><Icon name="trash" size={15} /></button>
          </div>
        ))}
      </div>
      <button type="button" className="btn mt" onClick={() => setEdit({ kind: 'event', name: '', match: '' })}><Icon name="plus" size={15} />Add a goal</button>
    </Section>
    {edit && <GoalSheet site={site} goal={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); onChanged() }} />}
    {del && <Confirm title="Delete this goal?" message={`“${del.name}” goes; the visits stay.`} confirmLabel="Delete" onCancel={() => setDel(null)} onConfirm={async () => {
      try { await api.del(`${V1}/goals/${del.id}`); onChanged() } catch (e) { toast(errMsg(e), 'err') }
      setDel(null)
    }} />}
  </>
}

/** The robots seen on a site lately, by name ("Scripts and tools" first: install scripts). */
function useRobotNames(siteID: number) {
  const [names, setNames] = useState<string[]>(['Scripts and tools'])
  useEffect(() => {
    api.get<{ rows: { value: string }[] }>(`${V1}/sites/${siteID}/robots?range=30d&by=name&limit=100`).then(r => setNames(Array.from(new Set(['Scripts and tools', ...r.rows.map(x => x.value)])))).catch(() => {})
  }, [siteID])
  return names
}

function useEventNames(siteID: number) {
  const [names, setNames] = useState<string[]>([])
  useEffect(() => { api.get<{ events: string[] }>(`${V1}/sites/${siteID}/events`).then(r => setNames(r.events)).catch(() => {}) }, [siteID])
  return names
}

function GoalSheet({ site, goal, onClose, onSaved }: { site: Site; goal: Partial<Goal>; onClose: () => void; onSaved: () => void }) {
  const [g, setG] = useState(goal)
  const [err, setErr] = useState('')
  const names = useEventNames(site.id)
  const robotNames = useRobotNames(site.id)
  const save = async () => {
    try {
      if (g.id) await api.put(`${V1}/goals/${g.id}`, g)
      else await api.post(`${V1}/sites/${site.id}/goals`, g)
      onSaved()
    } catch (e) { setErr(errMsg(e)) }
  }
  return (
    <Modal title={g.id ? 'Edit the goal' : 'A new goal'} onClose={onClose} footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={save}>Save</button></>}>
      <Field label="What counts"><Segmented value={g.kind || 'event'} onChange={v => setG({ ...g, kind: v })} options={[{ value: 'event', label: 'An event' }, { value: 'page', label: 'A page' }, { value: 'robot', label: 'Read by a robot' }]} /></Field>
      {g.kind === 'robot' ? <>
        <p className="muted small note-lead">For things that happen outside a browser: <code>curl … /install.sh | sh</code> fetching your install script is an install run. Robots are not visits, so this goal counts times, and filters do not apply to it.</p>
        <Field label="The page" help="The path a robot reads, like /install.sh. * stands for anything."><input value={g.match || ''} onChange={e => setG({ ...g, match: e.target.value })} placeholder="/install.sh" autoFocus /></Field>
        <Field label="Which robot" help="“Scripts and tools” is curl, wget and the like. Any robot also counts search engines that read the page."><Select value={g.robot || ''} onChange={v => setG({ ...g, robot: v })} options={[{ value: '', label: 'Any robot' }, ...robotNames.map(n => ({ value: n, label: n }))]} onCreate={v => { setG({ ...g, robot: v }); return v }} createLabel="Use" /></Field>
      </> : g.kind === 'page'
        ? <Field label="The page" help="The path, like /order/thanks. * stands for anything: /blog/*"><input value={g.match || ''} onChange={e => setG({ ...g, match: e.target.value })} placeholder="/order/thanks" autoFocus /></Field>
        : <Field label="The event" help={names.length ? 'Events seen on this site lately, or type a new one.' : 'The name your site sends, like “Signed up”.'}>
          {names.length ? <Select value={g.match || ''} onChange={v => setG({ ...g, match: v })} options={names.map(n => ({ value: n, label: n }))} onCreate={v => { setG({ ...g, match: v }); return v }} createLabel="Use" placeholder="Pick or type an event" /> : <input value={g.match || ''} onChange={e => setG({ ...g, match: e.target.value })} placeholder="Signed up" autoFocus />}
        </Field>}
      <Field label="Name" help="Leave empty to use the page or the event."><input value={g.name || ''} onChange={e => setG({ ...g, name: e.target.value })} /></Field>
      {err && <p className="help bad">{err}</p>}
    </Modal>
  )
}

// ---- funnels ----

function Funnels({ site, onChanged }: { site: Site; onChanged: () => void }) {
  const [edit, setEdit] = useState<Partial<Funnel> | null>(null)
  const [del, setDel] = useState<Funnel | null>(null)
  const { toast } = useToast()
  const funnels = site.funnels || []
  return <>
    <Section title="Funnels" sub="Steps people should take in order — the menu, the order form, ordered — and where they drop off.">
      {!funnels.length && <p className="muted">No funnels yet.</p>}
      <div className="list">
        {funnels.map(f => (
          <div key={f.id} className="list-row">
            <Icon name="trending-down" size={16} className="muted" />
            <div className="grow"><div>{f.name}</div><div className="muted small ellipsis">{f.steps.map(s => s.label || s.match).join(' → ')}</div></div>
            <button type="button" className="btn ghost sm" onClick={() => setEdit(f)}>Edit</button>
            <button type="button" className="btn ghost icon-only sm" aria-label="Delete" onClick={() => setDel(f)}><Icon name="trash" size={15} /></button>
          </div>
        ))}
      </div>
      <button type="button" className="btn mt" onClick={() => setEdit({ name: '', steps: [{ kind: 'page', match: '/' }, { kind: 'page', match: '' }] })}><Icon name="plus" size={15} />Make a funnel</button>
    </Section>
    {edit && <FunnelSheet site={site} funnel={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); onChanged() }} />}
    {del && <Confirm title="Delete this funnel?" message={`“${del.name}” goes; the visits stay.`} confirmLabel="Delete" onCancel={() => setDel(null)} onConfirm={async () => {
      try { await api.del(`${V1}/funnels/${del.id}`); onChanged() } catch (e) { toast(errMsg(e), 'err') }
      setDel(null)
    }} />}
  </>
}

function FunnelSheet({ site, funnel, onClose, onSaved }: { site: Site; funnel: Partial<Funnel>; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(funnel.name || '')
  const [steps, setSteps] = useState<Step[]>(funnel.steps || [])
  const [err, setErr] = useState('')
  const names = useEventNames(site.id)
  const robotNames = useRobotNames(site.id)
  const put = (i: number, p: Partial<Step>) => setSteps(steps.map((s, j) => (j === i ? { ...s, ...p } : s)))
  const save = async () => {
    try {
      if (funnel.id) await api.put(`${V1}/funnels/${funnel.id}`, { name, steps })
      else await api.post(`${V1}/sites/${site.id}/funnels`, { name, steps })
      onSaved()
    } catch (e) { setErr(errMsg(e)) }
  }
  return (
    <Modal title={funnel.id ? 'Edit the funnel' : 'A new funnel'} size="lg" onClose={onClose} footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={save}>Save</button></>}>
      <Field label="Name"><input value={name} onChange={e => setName(e.target.value)} placeholder="From the menu to an order" autoFocus /></Field>
      <ol className="fsteps">
        {steps.map((s, i) => (
          <li key={i} className="fstep-edit">
            <span className="fstep-n">{i + 1}</span>
            <Segmented size="sm" value={s.kind} onChange={v => put(i, { kind: v })} options={[{ value: 'page', label: 'Page' }, { value: 'event', label: 'Event' }, ...(i > 0 && i === steps.length - 1 ? [{ value: 'robot' as const, label: 'Robot', title: 'Read by a robot (install.sh by curl): only as the last step' }] : [])]} />
            {s.kind === 'robot' && <Select ariaLabel="Which robot" value={s.robot || ''} onChange={v => put(i, { robot: v })} options={[{ value: '', label: 'Any robot' }, ...robotNames.map(n => ({ value: n, label: n }))]} />}
            {s.kind === 'event' && names.length
              ? <Select ariaLabel="Event" value={s.match} onChange={v => put(i, { match: v })} options={names.map(n => ({ value: n, label: n }))} onCreate={v => { put(i, { match: v }); return v }} createLabel="Use" placeholder="Pick an event" className="grow" />
              : <input className="grow" value={s.match} onChange={e => put(i, { match: e.target.value })} placeholder={s.kind === 'page' ? '/order' : s.kind === 'robot' ? '/install.sh' : 'Order placed'} />}
            <input className="fstep-label" value={s.label || ''} onChange={e => put(i, { label: e.target.value })} placeholder="Label (optional)" />
            <button type="button" className="btn ghost icon-only sm" aria-label="Remove the step" disabled={steps.length <= 2} onClick={() => setSteps(steps.filter((_, j) => j !== i))}><Icon name="x" size={15} /></button>
          </li>
        ))}
      </ol>
      <button type="button" className="btn sm" disabled={steps.length >= 8} onClick={() => setSteps([...steps.map(s => s.kind === 'robot' ? { ...s, kind: 'page' as const, robot: undefined } : s), { kind: 'page', match: '' }])}><Icon name="plus" size={14} />Add a step</button>
      {steps.some(s => s.kind === 'robot') && <p className="muted small mt">A robot step counts times (an install script fetched), not people: it is not part of anyone's visit, so it shows as a ratio to the step before.</p>}
      {err && <p className="help bad">{err}</p>}
    </Modal>
  )
}

// ---- alerts ----

function Alerts({ site, onSaved }: { site: Site; onSaved: (s: Site) => void }) {
  const save = useSave(site, onSaved)
  const o = site.options
  const set = (patch: Partial<SiteOptions>) => save({ options: { ...o, ...patch } })
  return (
    <Section title="Tell me" sub="Messages go where Settings → Notifications says (your phone with Pocket Hub, ntfy, Apprise…).">
      <SettingRow title="When far more people come than usual" help="Five times the usual hour and at least 20 people — someone shared you somewhere. Once a day at most."><Switch checked={o.spike} onChange={v => set({ spike: v })} /></SettingRow>
      <SettingRow title="When nobody came for a day" help="For a site that usually has visitors: the site may be down or the script gone."><Switch checked={o.quiet} onChange={v => set({ quiet: v })} /></SettingRow>
      <SettingRow title="In the weekly report" help="Monday morning: last week against the week before."><Switch checked={o.weekly} onChange={v => set({ weekly: v })} /></SettingRow>
    </Section>
  )
}

// ---- name & address ----

function General({ site, onSaved }: { site: Site; onSaved: (s: Site) => void }) {
  const save = useSave(site, onSaved)
  const [name, setName] = useState(site.name)
  const [domain, setDomain] = useState(site.domain)
  const [also, setAlso] = useState(site.also.join('\n'))
  const tzs = zones()
  return (
    <Section title="Name & address">
      <Field label="Name" help="How the site is called here."><input value={name} onChange={e => setName(e.target.value)} onBlur={() => name !== site.name && save({ name })} placeholder={site.domain} /></Field>
      <Field label="Address" help="The domain the script sends (data-domain). Changing it means changing the line on your pages too."><input value={domain} onChange={e => setDomain(e.target.value)} onBlur={() => domain !== site.domain && save({ domain })} /></Field>
      <Field label="Other addresses" help="Other domains of the same site, one per line. Names under the address (blog.example.com) count already."><textarea rows={2} value={also} onChange={e => setAlso(e.target.value)} onBlur={() => also !== site.also.join('\n') && save({ also: also.split('\n').map(x => x.trim()).filter(Boolean) })} /></Field>
      <Field label="Time zone" help="Where a day starts and ends for this site.">
        <Select value={site.tz_set} onChange={v => save({ tz: v })} options={[{ value: '', label: `Yours (${site.tz})` }, ...tzs.map(z => ({ value: z, label: z.replace(/_/g, ' ') }))]} />
      </Field>
    </Section>
  )
}

// ---- data ----

function DataTab({ site }: { site: Site }) {
  const navigate = useNavigate()
  const { toast } = useToast()
  const { reload } = useData()
  const [confirm, setConfirm] = useState<'' | 'reset' | 'delete' | 'key'>('')
  const [keep, setKeep] = useState(String(site.options.keep_days || 0))
  const save = async (days: number) => {
    try { await api.put(`${V1}/sites/${site.id}`, { options: { ...site.options, keep_days: days } }); toast('Saved', 'ok') } catch (e) { toast(errMsg(e), 'err') }
  }
  return <>
    <Section title="Keep visits">
      <SettingRow title="How long" help="Older visits are removed every hour. Totals for removed days are gone too.">
        <Select value={keep} onChange={v => { setKeep(v); save(Number(v)) }} options={[{ value: '0', label: 'Always' }, { value: '365', label: '1 year' }, { value: '730', label: '2 years' }, { value: '1095', label: '3 years' }, { value: '180', label: '6 months' }]} />
      </SettingRow>
    </Section>
    <Section title="Take it with you">
      <div className="row gap wrap">
        <a className="btn" href={`${V1}/sites/${site.id}/export.csv?range=all`} download><Icon name="download" size={15} />Every visit (spreadsheet)</a>
        <a className="btn" href={`${V1}/sites/${site.id}/export.csv?range=all&dim=page`} download><Icon name="download" size={15} />Pages (spreadsheet)</a>
      </div>
    </Section>
    <Section title="Careful">
      <SettingRow title="A new proxy key" help="For the Cloudflare Worker way: the old key stops working at once."><button type="button" className="btn" onClick={() => setConfirm('key')}>Make a new key</button></SettingRow>
      <SettingRow title="Start again" help="Every visit of this site is forgotten; the site, goals and funnels stay."><button type="button" className="btn danger" onClick={() => setConfirm('reset')}>Forget all visits</button></SettingRow>
      <SettingRow title="Delete this site" help="The site, its goals, funnels and every visit."><button type="button" className="btn danger" onClick={() => setConfirm('delete')}>Delete the site</button></SettingRow>
    </Section>
    {confirm && <Confirm title={confirm === 'delete' ? `Delete ${site.domain}?` : confirm === 'reset' ? 'Forget every visit?' : 'Make a new proxy key?'}
      message={confirm === 'delete' ? 'The site and everything counted for it go. This cannot be undone.' : confirm === 'reset' ? 'All visits of this site go. This cannot be undone.' : 'Put the new key in your Worker; until then visits through it lose their place.'}
      confirmLabel={confirm === 'delete' ? 'Delete' : confirm === 'reset' ? 'Forget' : 'Make'} danger={confirm !== 'key'}
      onCancel={() => setConfirm('')} onConfirm={async () => {
        try {
          if (confirm === 'delete') { await api.del(`${V1}/sites/${site.id}`); await reload(); navigate('/') }
          else if (confirm === 'reset') { await api.post(`${V1}/sites/${site.id}/reset`); toast('Forgotten', 'ok'); reload() }
          else { await api.post(`${V1}/sites/${site.id}/proxy-key`); toast('A new key is ready under Install → Cloudflare', 'ok') }
        } catch (e) { toast(errMsg(e), 'err') }
        setConfirm('')
      }} />}
  </>
}

// ---- adding a site ----

export function AddSite({ onClose }: { onClose: () => void }) {
  const navigate = useNavigate()
  const { reload } = useData()
  const [domain, setDomain] = useState('')
  const [name, setName] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const add = async (e?: React.FormEvent) => {
    e?.preventDefault()
    setBusy(true); setErr('')
    try {
      const s = await api.post<Site>(`${V1}/sites`, { domain, name })
      await reload()
      onClose()
      navigate(`/sites/${s.id}`)
    } catch (x) { setErr(errMsg(x)) } finally { setBusy(false) }
  }
  return (
    <Modal title="Add a website" onClose={onClose} footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" disabled={busy || !domain.trim()} onClick={() => add()}>Add</button></>}>
      <form onSubmit={add}>
        <Field label="Its address" help="Without https:// — like example.com. Pages under it (blog.example.com) count too."><input value={domain} onChange={e => setDomain(e.target.value)} placeholder="example.com" autoFocus inputMode="url" autoCapitalize="off" spellCheck={false} /></Field>
        <Field label="Name (optional)"><input value={name} onChange={e => setName(e.target.value)} placeholder="My bakery" /></Field>
        {err && <p className="help bad">{err}</p>}
        <button type="submit" hidden />
      </form>
    </Modal>
  )
}
