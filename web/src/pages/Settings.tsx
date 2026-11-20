import { AskButton } from '../components/AskButton'
import React, { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { APIToken, AppSlug, ConnMode, Settings as SettingsT, User } from '../lib/types'
import { DATE_FORMATS, TIME_FORMATS, fmtBytes, fmtDateTime, type DateFormat, type TimeFormat } from '../lib/format'
import { Avatar, Confirm, CopyField, CoreHint, Field, Loading, Modal, Segmented, SettingRow, Switch, Tabs, useAsync, useIsMobile, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { ShortcutsSheet } from '../components/ShortcutsSheet'
import { PocketModelSelect } from '../components/PocketModelSelect'
import { zones } from '../lib/analytics'
import { announceChange } from '../lib/data'

const ZONES = zones()
const APP_INFO: Record<AppSlug, { name: string; icon: string }> = { mail: { name: 'Pocket Mail', icon: 'mail' }, notes: { name: 'Pocket Notes', icon: 'note' }, tasks: { name: 'Pocket Tasks', icon: 'check-square' } }

type TabId = 'general' | 'notifications' | 'apps' | 'ai' | 'google' | 'profile' | 'people' | 'tokens' | 'data' | 'about'

const TABS: { id: TabId; label: string; icon: string; sub: string }[] = [
  { id: 'general', label: 'General', icon: 'home', sub: 'Your time zone, dates and how new websites start.' },
  { id: 'notifications', label: 'Report & alerts', icon: 'bell', sub: 'The weekly report, and a message when far more people than usual come or a site goes quiet.' },
  { id: 'apps', label: 'Connected apps', icon: 'layers', sub: 'Pocket Mail for the report by email, Pocket Notes and Pocket Tasks to keep what the numbers say.' },
  { id: 'ai', label: 'AI', icon: 'sparkles', sub: 'Who explains the numbers: Pocket AI through Core, or your own OpenAI key.' },
  { id: 'google', label: 'Google Search', icon: 'search', sub: 'The words people typed into Google before they came, from Google Search Console.' },
  { id: 'profile', label: 'Profile', icon: 'user', sub: 'Your name, email and password.' },
  { id: 'people', label: 'People', icon: 'users', sub: 'Everyone who has an account on this box. Each person has their own websites.' },
  { id: 'tokens', label: 'API & shortcuts', icon: 'key', sub: 'Personal tokens for Siri Shortcuts, scripts, dashboards and other tools.' },
  { id: 'data', label: 'Backup & data', icon: 'database', sub: 'Export your websites with their goals and funnels, bring them back, download the database.' },
  { id: 'about', label: 'About', icon: 'info', sub: 'Version, numbers, where places come from, and the server.' },
]

export default function Settings() {
  const { tab: raw } = useParams()
  const navigate = useNavigate()
  const mobile = useIsMobile()
  const tab: TabId = TABS.find(t => t.id === raw)?.id || 'general'
  const current = TABS.find(t => t.id === tab)!
  const go = (id: TabId) => navigate(`/settings/${id}`, { replace: true })
  useEffect(() => { document.querySelector('.set-content')?.scrollTo({ top: 0 }) }, [tab])
  useEffect(() => {
    const h = (e: KeyboardEvent) => { if (e.key === 'Escape' && !document.querySelector('.modal-backdrop, .menu, .sel-pop')) navigate('/') }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [navigate])
  return (
    <div className="set-page">
      <header className="set-header">
        <button type="button" className="btn ghost icon-only" aria-label="Back" title="Back (esc)" onClick={() => navigate('/')}><Icon name="chevron-left" size={18} /></button>
        <img src="/favicon.svg" alt="" width={24} height={24} className="set-logo" />
        <h1>Settings</h1>
        <span style={{ flex: 1 }} />
        <AskButton />
      </header>
      {mobile && <div className="set-tabs-mobile"><Tabs tabs={TABS.map(t => ({ id: t.id, label: t.label, icon: t.icon }))} value={tab} onChange={go} /></div>}
      <div className="set-body">
        {!mobile && (
          <nav className="set-nav" aria-label="Settings sections">
            {TABS.map(t => (
              <button key={t.id} type="button" className={`set-nav-item ${t.id === tab ? 'active' : ''}`} aria-current={t.id === tab ? 'page' : undefined} onClick={() => go(t.id)}>
                <Icon name={t.icon} size={16} />{t.label}
              </button>
            ))}
          </nav>
        )}
        <main className="set-content">
          <div className="set-inner" key={tab}>
            <div className="set-title"><h2>{current.label}</h2><p>{current.sub}</p></div>
            {tab === 'general' && <General />}
            {tab === 'notifications' && <Notifications />}
            {tab === 'apps' && <Integrations />}
            {tab === 'ai' && <AITab />}
            {tab === 'google' && <GoogleTab />}
            {tab === 'profile' && <Profile />}
            {tab === 'people' && <Users />}
            {tab === 'tokens' && <Tokens />}
            {tab === 'data' && <Data />}
            {tab === 'about' && <About />}
          </div>
        </main>
      </div>
    </div>
  )
}

function Section({ title, sub, icon, actions, children }: { title: React.ReactNode; sub?: React.ReactNode; icon?: string; actions?: React.ReactNode; children?: React.ReactNode }) {
  return (
    <section className="set-section">
      <div className="set-section-head">
        {icon && <span className="set-app-ico"><Icon name={icon} size={17} /></span>}
        <div className="grow"><h3>{title}</h3>{sub && <p>{sub}</p>}</div>
        {actions && <div className="row">{actions}</div>}
      </div>
      {children && <div className="set-section-body">{children}</div>}
    </section>
  )
}

function useSettingsForm() {
  const { settings, setSettings, user } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState<SettingsT | null>(settings)
  const [busy, setBusy] = useState(false)
  useEffect(() => { if (!f && settings) setF(settings) }, [settings]) // eslint-disable-line react-hooks/exhaustive-deps
  const save = async (patch?: Partial<SettingsT>) => {
    if (!f) return
    setBusy(true)
    try { const body = { ...f, ...patch } as Record<string, unknown>; body.tz = body.tz_set; const s = await api.put<SettingsT>(`${V1}/settings`, body); setSettings(s); setF(s); toast('Saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return { f, setF, save, busy, isAdmin: user?.role === 'admin' }
}

function useTester() {
  const [test, setTest] = useState<Record<string, string>>({})
  const run = async (which: string, url: string, key: string, extra: Record<string, string> = {}) => {
    setTest(t => ({ ...t, [which]: 'testing…' }))
    try { const r = await api.post<{ ok: boolean; message?: string; error?: string }>(`${V1}/integrations/test`, { which, url, key, ...extra }); setTest(t => ({ ...t, [which]: r.ok ? `✓ ${r.message}` : `✕ ${r.error}` })) } catch (e) { setTest(t => ({ ...t, [which]: `✕ ${(e as Error).message}` })) }
  }
  const status = (which: string) => <span className={`small ${test[which]?.startsWith('✓') ? 'ok' : test[which]?.startsWith('✕') ? 'bad' : 'muted'}`}>{test[which]}</span>
  return { run, status }
}

const HOURS = Array.from({ length: 24 }, (_, h) => ({ value: String(h), label: `${String(h).padStart(2, '0')}:00` }))

function General() {
  const { f, setF, save, busy, isAdmin } = useSettingsForm()
  if (!f) return <Loading />
  return (
    <div className="stack loose">
      <Section title="Time zone" icon="globe" sub="Where a day starts and ends for your websites (each site can have its own), and when the weekly report comes.">
        <SettingRow title="Your time zone" help={f.tz_set ? undefined : `The box's zone (${f.tz}).`}><Select ariaLabel="Time zone" value={f.tz_set || f.tz} onChange={v => setF({ ...f, tz_set: v, tz: v })} options={[...new Set([f.tz, ...ZONES])].map(z => ({ value: z, label: z.replace(/_/g, ' ') }))} /></SettingRow>
      </Section>
      <Section title="Dates" icon="calendar">
        <SettingRow title="Week starts on" help="For charts by week."><Segmented size="sm" value={f.week_start} onChange={v => setF({ ...f, week_start: v })} options={[{ value: 1, label: 'Monday' }, { value: 6, label: 'Saturday' }, { value: 7, label: 'Sunday' }]} /></SettingRow>
        <SettingRow title="Date format"><Select ariaLabel="Date format" value={f.date_format} onChange={v => setF({ ...f, date_format: v as DateFormat })} options={DATE_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></SettingRow>
        <SettingRow title="Time format"><Select ariaLabel="Time format" value={f.time_format} onChange={v => setF({ ...f, time_format: v as TimeFormat })} options={TIME_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></SettingRow>
        {isAdmin && <SettingRow title="App title" help="Shown in the sidebar for everyone on this box."><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} /></SettingRow>}
      </Section>
      <Section title="Where websites send visits" icon="link" sub={<>Every site's script and its visits go to this address. {f.public_url ? 'It is the public one; the app itself stays private.' : 'Websites on the internet can only reach it if it is public (PUBLIC_URL, a tunnel or a proxy in front of /js/ and /api/event).'}</>}>
        <SettingRow title="Address"><code>{f.collect_url || '(BASE_URL is not set)'}</code></SettingRow>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}

function Notifications() {
  const { f, setF, save, busy } = useSettingsForm()
  const { toast } = useToast()
  const { run, status } = useTester()
  if (!f) return <Loading />
  const mailOn = f.mail_mode !== 'off' && f.mail_via !== ''
  const now = async () => { try { const r = await api.post<{ sent: boolean; text: string; error?: string }>(`${V1}/notify/report`); toast(r.error ? `Email: ${r.error}` : r.sent ? 'Sent' : r.text ? 'Nothing sent: no message address, no phone and no email set' : 'No websites in the report yet', r.error ? 'err' : 'ok') } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <div className="stack loose">
      <Section title="The weekly report" icon="chart" sub="Monday morning: each website's visitors, pages, sources and countries against the week before. Sites can be left out under their own settings.">
        <SettingRow title="As a message" help="Through the address below and to your phones."><Switch checked={f.weekly_report} onChange={v => setF({ ...f, weekly_report: v })} label="Weekly message" /></SettingRow>
        <SettingRow title="By email" help={mailOn ? 'From your own mailbox through Pocket Mail.' : `Pocket Mail is not connected: ${f.mail_detail || 'Connected apps'}.`}><Switch checked={f.report_email} onChange={v => setF({ ...f, report_email: v })} label="Weekly email" disabled={!mailOn && !f.report_email} /></SettingRow>
        {f.report_email && <SettingRow title="Email to" help="Empty: your own address."><input type="email" value={f.report_to} onChange={e => setF({ ...f, report_to: e.target.value })} placeholder="you@example.com" /></SettingRow>}
        <SettingRow title="At" help="Your time zone, on Mondays."><Select ariaLabel="Hour" value={String(f.notify_hour)} onChange={v => setF({ ...f, notify_hour: Number(v) })} options={HOURS} /></SettingRow>
      </Section>
      <Section title="Where messages go" icon="bell" actions={<span className={`badge ${f.notify_configured ? 'ok' : ''}`}>{f.notify_configured ? 'on' : 'not set'}</span>}
        sub={<>The report, spikes and quiet sites: an Apprise gateway (<code>…/notify/&lt;key&gt;</code>), an ntfy topic URL, or any webhook that takes JSON. Which sites alert is set per site.</>}>
        <SettingRow title="URL" stack><input value={f.notify_url} onChange={e => setF({ ...f, notify_url: e.target.value })} placeholder="http://apprise:8000/notify/analytics" /></SettingRow>
        <SettingRow title="Format"><Segmented value={f.notify_format} onChange={v => setF({ ...f, notify_format: v })} options={[{ value: 'apprise', label: 'Apprise' }, { value: 'ntfy', label: 'ntfy' }, { value: 'json', label: 'JSON' }]} size="sm" /></SettingRow>
        <SettingRow title="Also to my phones" help={f.core_configured ? 'Through Pocket Core to the phones with Pocket Hub.' : 'Needs Pocket Core.'}><Switch checked={f.push_phone} onChange={v => setF({ ...f, push_phone: v })} label="Phones" disabled={!f.core_configured} /></SettingRow>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button>
        <button type="button" className="btn" onClick={() => run('notify', f.notify_url, '', { format: f.notify_format })} disabled={!f.notify_url}><Icon name="mail" size={15} />Send a test</button>
        <button type="button" className="btn" onClick={now}><Icon name="send" size={15} />Send the report now</button>
        {status('notify')}
      </div>
    </div>
  )
}

const APPS: { slug: AppSlug; title: string; sub: string; token: string; port: string }[] = [
  { slug: 'mail', title: 'Pocket Mail — the weekly report by email', sub: 'Last week of each website in your inbox on Monday, sent from your own mailbox.', token: 'Pocket Mail → Settings → API tokens', port: '8208' },
  { slug: 'notes', title: 'Pocket Notes — keep what the numbers say', sub: "Keep Pocket AI's explanation of a period as a note.", token: 'Pocket Notes → Settings → API tokens', port: '8182' },
  { slug: 'tasks', title: 'Pocket Tasks — act on it', sub: 'Make a task from what the numbers show: a page people leave, a link that broke, a site gone quiet.', token: 'Pocket Tasks → Settings → API tokens', port: '8184' },
]

function Integrations() {
  const { f, setF, save, busy } = useSettingsForm()
  const { run, status } = useTester()
  if (!f) return <Loading />
  const coreOn = f.core_configured && f.core_registered !== false
  const eff = (m: ConnMode): ConnMode => !coreOn && m === 'core' ? 'off' : m
  const set = (k: string, v: string) => setF({ ...f, [k]: v } as SettingsT)
  return (
    <div className="stack loose">
      <div className="callout info"><Icon name="info" size={15} /><div>These connections are yours. Analytics only adds things in them when you ask (and the weekly email when you switch it on). Through <b>Pocket Core</b> Analytics acts as you in your own Pocket apps with nothing to paste (switch it off for yourself in Pocket Core → Settings → Apps acting for me); <b>Direct</b> uses a URL and an API token you enter.{!coreOn && <CoreHint />}</div></div>
      {APPS.map(a => {
        const mode = f[`${a.slug}_mode`]
        const via = f[`${a.slug}_via`]
        const name = APP_INFO[a.slug].name
        return (
          <Section key={a.slug} title={a.title} icon={APP_INFO[a.slug].icon} actions={connBadge(eff(mode), via)} sub={a.sub}>
            <Connection app={name} mode={mode} via={via} detail={f[`${a.slug}_detail`]} core={coreOn} hasToken={!!f[`${a.slug}_token`]}
              onMode={m => save({ [`${a.slug}_mode`]: m } as Partial<SettingsT>)} onDisconnect={() => save({ [`${a.slug}_url`]: '', [`${a.slug}_token`]: '' } as Partial<SettingsT>)}
              test={<><button type="button" className="btn sm" onClick={() => run(a.slug, f[`${a.slug}_url`], f[`${a.slug}_token`])}>Test connection</button>{status(a.slug)}</>}
              direct={<div className="two-col tight">
                <Field label="URL" help="Inside Docker use http://host.docker.internal:PORT or the service name."><input value={f[`${a.slug}_url`]} onChange={e => set(`${a.slug}_url`, e.target.value)} placeholder={`http://host.docker.internal:${a.port}`} /></Field>
                <Field label="API token" help={a.token}><input value={f[`${a.slug}_token`]} onChange={e => set(`${a.slug}_token`, e.target.value)} autoComplete="off" /></Field>
              </div>} />
          </Section>
        )
      })}
      <Section title="Other Pocket apps" icon="layers" sub="Analytics offers your websites' numbers to the rest of the box through Pocket Core: Pocket Day can show yesterday's visitors, Pocket Search finds your sites and pages, a blog can show its view counts, and Pocket AI can answer “how many people came this week?”. Visits one by one are never offered. Switched off until you allow them in Pocket Core → Links.">
        <dl className="set-dl">
          <div><dt>analytics:read</dt><dd className="muted">Your websites and their numbers: visitors, pages, sources, countries, devices, goals, funnels, the people on a site now.</dd></div>
          <div><dt>events:read</dt><dd className="muted"><code>analytics.spike</code>, <code>analytics.quiet</code>, <code>analytics.weekly</code> — for Pocket Flows.</dd></div>
        </dl>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}

function AITab() {
  const { f, setF, save, busy } = useSettingsForm()
  const { run, status } = useTester()
  const { data: ai } = useAsync(() => api.get<{ ready: boolean; provider: string; detail: string }>(`${V1}/ai/status`), [])
  if (!f) return <Loading />
  const coreOn = f.core_configured && f.core_registered !== false
  const shown = !coreOn && f.ai_provider === 'pocket' ? 'openai' : (f.ai_provider || 'auto')
  return (
    <div className="stack loose">
      <Section title="What the AI does" icon="sparkles" sub="“Explain” on a site's page: Pocket AI reads that period's numbers (totals, top sources, pages, countries, goals — never single visits) and says in plain words what stood out and what to try. Only when you press it.">
        <SettingRow title="Pocket AI buttons" help="Off: the buttons are gone and nothing is sent to an AI."><Switch checked={f.ai_help} onChange={v => save({ ai_help: v })} label="AI buttons" /></SettingRow>
        {ai && (ai.ready ? <p className="muted small row" style={{ gap: 6 }}><Icon name="sparkles" size={13} />{ai.provider === 'pocket' ? 'Pocket AI' : 'Your OpenAI key'} is ready.</p>
          : <div className="callout info"><Icon name="info" size={15} /><div>No AI connected{ai.detail ? ` (${ai.detail})` : ''}.{!coreOn && <CoreHint />}</div></div>)}
      </Section>
      <Section title="Provider" icon="sparkles">
        <div className="grid" style={{ gap: 12 }}>
          <Field group label="Who writes" help="Automatic prefers your enabled Pocket AI connection; without one it uses your OpenAI settings.">
            <Select ariaLabel="AI provider" value={shown} onChange={v => setF({ ...f, ai_provider: v as SettingsT['ai_provider'] })} options={[{ value: 'auto', label: coreOn ? 'Automatic — prefer Pocket AI' : 'Automatic' }, { value: 'pocket', label: 'Pocket AI', disabled: !coreOn, hint: coreOn ? undefined : 'needs Pocket Core' }, { value: 'openai', label: 'OpenAI / compatible API' }]} />
          </Field>
          {!coreOn && <CoreHint />}
          {coreOn && shown !== 'openai' && <PocketModelSelect value={f.ai_pocket_model || ''} onChange={v => setF({ ...f, ai_pocket_model: v })} />}
          {(shown === 'openai' || (!coreOn && shown === 'auto')) && <>
            <Field label="OpenAI API key" help="platform.openai.com → API keys. Stored on the server; shown masked."><input value={f.openai_api_key} onChange={e => setF({ ...f, openai_api_key: e.target.value })} autoComplete="off" placeholder="sk-…" /></Field>
            <div className="two-col tight">
              <Field label="Model" help="Default gpt-4o-mini."><input value={f.openai_model} onChange={e => setF({ ...f, openai_model: e.target.value })} placeholder="gpt-4o-mini" /></Field>
              <Field label="Base URL (optional)" help="Ollama, LM Studio, OpenRouter…"><input value={f.openai_base_url} onChange={e => setF({ ...f, openai_base_url: e.target.value })} placeholder="https://api.openai.com/v1" /></Field>
            </div>
          </>}
        </div>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button>
        {(shown === 'openai' || (!coreOn && shown === 'auto')) && <><button type="button" className="btn" onClick={() => run('openai', f.openai_base_url, f.openai_api_key)} disabled={!f.openai_api_key}>Test the OpenAI key</button>{status('openai')}</>}
      </div>
    </div>
  )
}

type GoogleTest = { ok: boolean; error?: string; email?: string; properties?: string[]; sites?: { site: string; domain: string; property: string }[] }

/** Google Search Console: one key file, pasted once. */
function GoogleTab() {
  const { settings, setSettings } = useApp()
  const { toast } = useToast()
  const [key, setKey] = useState('')
  const [busy, setBusy] = useState(false)
  const [test, setTest] = useState<GoogleTest | null>(null)
  const [testing, setTesting] = useState(false)
  const [remove, setRemove] = useState(false)
  if (!settings) return <Loading />
  const on = settings.google_configured
  const run = async (typed = '') => {
    setTesting(true)
    try { setTest(await api.post<GoogleTest>(`${V1}/google/test`, { key: typed })) } catch (e) { setTest({ ok: false, error: (e as Error).message }) } finally { setTesting(false) }
  }
  const save = async () => {
    setBusy(true)
    try { const s = await api.put<SettingsT>(`${V1}/settings`, { google_key: key }); setSettings(s); setKey(''); toast('Saved'); run() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const pick = (file?: File) => { if (file) file.text().then(setKey).catch(() => toast('That file could not be read', 'err')) }
  const missing = test?.ok ? (test.sites || []).filter(s => !s.property) : []
  return (
    <div className="stack loose">
      <Section title="What this adds" icon="search" sub="A website never learns which words someone searched for: Google keeps that to itself and shows it only to the site's owner, in Google Search Console. With a key from your Google account, the words show on each site's page under Where they came from → Search words: what was searched, how often you were shown, your place in the results, and the clicks." />
      {on && <Section title="Connected" icon="check-circle" sub="This is the key's address. Every site you want words for must have it as a user in Search Console (step 4 below).">
        <CopyField value={settings.google_email || 'a key whose address could not be read'} label="The key's address" />
        <div className="row wrap mt">
          <button type="button" className="btn" disabled={testing} onClick={() => run()}>{testing ? 'Asking Google…' : 'Test'}</button>
          <button type="button" className="btn danger" onClick={() => setRemove(true)}>Remove the key</button>
        </div>
      </Section>}
      {test && <div className={`callout ${test.ok && !missing.length ? 'ok' : test.ok ? 'info' : 'warn'}`}><Icon name={test.ok ? 'check-circle' : 'alert'} size={16} /><div>
        {!test.ok ? <>Google said: {test.error}</> : <>
          <b>Google accepts the key.</b>
          <ul className="g-sites">
            {(test.sites || []).map(s => <li key={s.domain}>{s.property ? <><Icon name="check" size={13} className="ok" /> <b>{s.site}</b>: words will show</> : <><Icon name="x" size={13} className="bad" /> <b>{s.site}</b>: not shared with the key yet. In Search Console, add <code>{test.email}</code> as a user of {s.domain}.</>}</li>)}
          </ul>
          {!test.sites?.length && <span>Add a website here first.</span>}
        </>}
      </div></div>}
      <Section title={on ? 'Use another key' : 'Set it up'} icon="key" sub="About ten minutes, once. Everything happens in your own Google account; it costs nothing.">
        <ol className="howto g-steps">
          <li>Your site must be in <a href="https://search.google.com/search-console" target="_blank" rel="noreferrer">Google Search Console</a>. If it is not, add it there first (Google asks you to prove the site is yours).</li>
          <li>Open <a href="https://console.cloud.google.com/apis/library/searchconsole.googleapis.com" target="_blank" rel="noreferrer">Google Cloud → Search Console API</a>, make a project when asked (any name), and press <b>Enable</b>.</li>
          <li>Open <a href="https://console.cloud.google.com/iam-admin/serviceaccounts" target="_blank" rel="noreferrer">Service accounts</a> → <b>Create service account</b> (any name, no roles needed) → open it → <b>Keys</b> → <b>Add key</b> → <b>JSON</b>. A file downloads. Choose it below and save.</li>
          <li>In Search Console open your site → <b>Settings</b> → <b>Users and permissions</b> → <b>Add user</b>. Paste the key's address (it shows here after saving; it ends in <code>iam.gserviceaccount.com</code>) and choose <b>Restricted</b>.</li>
          <li>Press <b>Test</b>. Words appear for the days Google already has; it runs two or three days behind.</li>
        </ol>
        <Field label="The key file" help="The JSON file from step 3. It stays on your server and is never shown again.">
          <textarea rows={4} value={key} onChange={e => setKey(e.target.value)} placeholder={'{ "type": "service_account", "client_email": "…", "private_key": "…" }'} spellCheck={false} autoCapitalize="off" className="mono-area" />
        </Field>
        <div className="row wrap">
          <label className="btn"><Icon name="upload" size={15} />Choose the file<input type="file" accept=".json,application/json" hidden onChange={e => { pick(e.target.files?.[0]); e.target.value = '' }} /></label>
          <button type="button" className="btn primary" disabled={busy || !key.trim()} onClick={save}>{on ? 'Replace the key' : 'Save'}</button>
          {!on && <button type="button" className="btn" disabled={testing || !key.trim()} onClick={() => run(key)}>Test without saving</button>}
        </div>
      </Section>
      {remove && <Confirm title="Remove the Google key?" message="Search words stop showing on your sites. Nothing changes in your Google account." confirmLabel="Remove" onCancel={() => setRemove(false)} onConfirm={async () => {
        try { const s = await api.put<SettingsT>(`${V1}/settings`, { google_key: '' }); setSettings(s); setTest(null); toast('Removed') } catch (e) { toast((e as Error).message, 'err') }
        setRemove(false)
      }} />}
    </div>
  )
}

function Connection({ app, mode: saved, via, detail, core, onMode, onDisconnect, hasToken, test, direct }: {
  app: string; mode: ConnMode; via: string; detail: string; core: boolean; onMode: (m: ConnMode) => void; onDisconnect: () => void
  hasToken: boolean; test: React.ReactNode; direct: React.ReactNode
}) {
  const [advanced, setAdvanced] = useState(saved === 'direct')
  const mode: ConnMode = !core && saved === 'core' ? 'off' : saved
  const ok = via !== '' && mode !== 'off'
  const showDirect = !core || mode === 'direct' || advanced
  const line = mode === 'off' ? `Off: Analytics does not reach ${app}.`
    : mode === 'core' ? (ok ? `Connected through Pocket Core, as you${detail ? ` (${detail})` : ''}. Nothing to fill in; switch it off for yourself in Pocket Core → Settings → Apps acting for me.` : `Not connected: ${detail}.`)
    : (ok ? `Direct connection with your own ${app} URL and token; use Test connection to check it.` : `Not connected: ${detail || 'add the URL and a token'}.`)
  return (
    <div className="conn-block">
      <div className="conn-mode">
        <Segmented size="sm" value={mode} onChange={m => onMode(m as ConnMode)} options={[
          { value: 'core', label: 'Pocket Core', title: core ? 'Through Pocket Core: per person, nothing to paste' : 'Needs Pocket Core: there is none on this box', disabled: !core },
          ...(showDirect ? [{ value: 'direct', label: 'Direct', title: 'Your own URL and API token' }] : []),
          { value: 'off', label: 'Off' },
        ]} />
        {!showDirect && <button type="button" className="conn-advanced" onClick={() => setAdvanced(true)}>Advanced: connect directly with a URL and key</button>}
      </div>
      <div className={`conn-status ${mode === 'off' ? '' : ok ? 'ok' : 'bad'}`}><span className="dot" />{line}</div>
      {mode === 'direct' && <>
        {direct}
        <div className="row wrap">{test}{hasToken && <button type="button" className="btn sm ghost danger-text" onClick={onDisconnect} title="Remove the saved URL and token">Disconnect</button>}</div>
      </>}
      {mode === 'core' && <div className="row wrap">{test}</div>}
    </div>
  )
}

const connBadge = (mode: ConnMode, via: string) => mode === 'off' ? <span className="badge">off</span>
  : via ? <span className="badge ok">{via === 'core' ? 'Pocket Core' : 'direct'}</span>
  : <span className="badge warn">not connected</span>

function Users() {
  const { user, users, reloadMeta, status } = useApp()
  const { toast } = useToast()
  const [edit, setEdit] = useState<Partial<User> | null>(null)
  const [del, setDel] = useState<User | null>(null)
  const isAdmin = user?.role === 'admin'
  const run = async (fn: () => Promise<unknown>, msg: string) => { try { await fn(); await reloadMeta(); toast(msg) } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <Section title="People on this box" actions={isAdmin && <button type="button" className="btn primary sm" onClick={() => setEdit({ role: 'member' })}><Icon name="plus" size={14} />Add person</button>}
      sub="Everyone signs in with their own account and has their own websites, settings and connections. Admins manage accounts; they do not see anyone else’s websites.">
      <div className="set-people">
        {users.map(u => (
          <div key={u.id} className="set-person">
            <Avatar name={u.name} email={u.email} size={34} />
            <div className="grow">
              <div className="set-person-name">{u.name || <span className="muted">—</span>}{u.id === user?.id && <span className="badge">you</span>}{u.role === 'admin' && <span className="badge accent">admin</span>}</div>
              <div className="muted small ellipsis">{u.email}</div>
            </div>
            {isAdmin && <span className="row" style={{ justifyContent: 'flex-end' }}><button type="button" className="btn ghost sm" onClick={() => setEdit(u)}>Edit</button>{u.id !== user?.id && <button type="button" className="btn ghost sm danger-text" onClick={() => setDel(u)}>Remove</button>}</span>}
          </div>
        ))}
      </div>
      {status?.auth_mode !== 'local' && <p className="muted small mt">Auth mode is <code>{status?.auth_mode}</code>: users are created automatically on first visit; passwords are not used here.</p>}
      {edit && <UserModal user={edit} local={status?.auth_mode === 'local'} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reloadMeta() }} />}
      {del && <Confirm confirmLabel="Remove" title={`Remove ${del.name || del.email}?`} message="Their account is deleted together with their websites and everything counted for them." onCancel={() => setDel(null)} onConfirm={() => { const d = del; setDel(null); run(() => api.del(`${V1}/users/${d.id}`), 'User removed') }} />}
    </Section>
  )
}

function UserModal({ user, local, onClose, onSaved }: { user: Partial<User>; local: boolean; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ name: user.name || '', email: user.email || '', password: '', role: user.role || 'member' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { if (user.id) await api.put(`${V1}/users/${user.id}`, f); else await api.post(`${V1}/users`, f); toast(user.id ? 'User saved' : 'User added'); onSaved() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={user.id ? 'Edit person' : 'Add a person'} onClose={onClose} size="sm">
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} autoFocus required maxLength={60} /></Field>
        <Field label="Email" help="Used to sign in."><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} required /></Field>
        {local && <Field label={user.id ? 'New password (leave empty to keep)' : 'Password'} help="At least 8 characters."><input type="password" value={f.password} onChange={e => setF({ ...f, password: e.target.value })} minLength={user.id ? 0 : 8} required={!user.id} autoComplete="new-password" /></Field>}
        <Field group label="Role"><Segmented value={f.role} onChange={v => setF({ ...f, role: v })} options={[{ value: 'member', label: 'Member' }, { value: 'admin', label: 'Admin' }]} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{user.id ? 'Save' : 'Add person'}</button></div>
      </form>
    </Modal>
  )
}

function Profile() {
  const { user, setUser, status } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState({ name: user?.name || '', email: user?.email || '', current_password: '', new_password: '' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { setUser(await api.put(`${V1}/auth/me`, f)); setF(x => ({ ...x, current_password: '', new_password: '' })); toast('Profile saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <form onSubmit={save} className="stack loose">
      <div className="set-profile">
        <Avatar name={user?.name} email={user?.email} size={56} />
        <div className="grow"><h3>{user?.name || user?.email}</h3><p className="muted small">{user?.email} · {user?.role}</p></div>
      </div>
      <Section title="You">
        <div className="stack">
          <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} /></Field>
          <Field label="Email"><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} /></Field>
        </div>
      </Section>
      {status?.auth_mode === 'local' && <Section title="Password" sub="Leave empty to keep the current one."><div className="stack">
        <Field label="Current password"><input type="password" value={f.current_password} onChange={e => setF({ ...f, current_password: e.target.value })} autoComplete="current-password" /></Field>
        <Field label="New password" help="At least 8 characters."><input type="password" value={f.new_password} onChange={e => setF({ ...f, new_password: e.target.value })} autoComplete="new-password" /></Field>
      </div></Section>}
      <div className="row"><button className="btn primary" disabled={busy}>Save changes</button></div>
    </form>
  )
}

function Tokens() {
  const { toast } = useToast()
  const { settings } = useApp()
  const { data, reload } = useAsync(() => api.get<APIToken[]>(`${V1}/tokens`), [])
  const [name, setName] = useState('')
  const [fresh, setFresh] = useState('')
  const [del, setDel] = useState<APIToken | null>(null)
  const create = async (e: React.FormEvent) => {
    e.preventDefault()
    try { const r = await api.post<{ token: string }>(`${V1}/tokens`, { name: name || 'token' }); setFresh(r.token); setName(''); reload() } catch (e) { toast((e as Error).message, 'err') }
  }
  const base = settings?.base_url || window.location.origin
  return (
    <div className="stack loose">
      <Section title="Personal tokens" icon="key" sub={<>Use as <code>Authorization: Bearer &lt;token&gt;</code>. A token acts as you: it can read and change everything, feed a Homepage widget (<code>GET /api/v1/summary</code>) or a Siri Shortcut (“how many people came today?”). See <code>docs/api.md</code>.</>}>
        <form onSubmit={create} className="row mb"><input value={name} onChange={e => setName(e.target.value)} placeholder="Token name (e.g. iPhone Shortcut)" /><button className="btn primary"><Icon name="plus" size={15} />Create</button></form>
        {fresh && <div className="set-fresh mb"><div className="row"><Icon name="check-circle" size={16} /><b>Copy it now — it is shown once.</b></div><pre className="mono">{fresh}</pre></div>}
        {data && data.length > 0 ? (
          <table className="table compact">
            <thead><tr><th>Name</th><th>Prefix</th><th>Last used</th><th>Created</th><th /></tr></thead>
            <tbody>{data.map(t => <tr key={t.id}><td>{t.name}</td><td className="mono">{t.prefix}…</td><td className="muted">{t.last_used_at ? fmtDateTime(t.last_used_at) : 'never'}</td><td className="muted">{fmtDateTime(t.created_at)}</td><td className="right"><button type="button" className="btn ghost sm danger-text" onClick={() => setDel(t)}>Revoke</button></td></tr>)}</tbody>
          </table>
        ) : <p className="muted">No tokens yet.</p>}
        {del && <Confirm confirmLabel="Revoke" title={`Revoke “${del.name}”?`} message="Anything using this token stops working immediately." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/tokens/${del.id}`); setDel(null); reload() }} />}
      </Section>
      <Section title="Endpoints" icon="code" sub={<>At <code>{base}/api/v1</code>.</>}>
        <dl className="set-dl">
          <div><dt><code>GET /sites</code></dt><dd className="muted">Your websites with today, the last 30 days and the people on each now.</dd></div>
          <div><dt><code>GET /sites/&lt;id&gt;/stats?range=7d</code></dt><dd className="muted">Totals, the period before and the chart. <code>range</code>: today, yesterday, 7d, 30d, month, lastmonth, 12mo, year, all, custom (<code>from</code>/<code>to</code>); filters as <code>f=country:BA</code>.</dd></div>
          <div><dt><code>GET /sites/&lt;id&gt;/breakdown?dim=source</code></dt><dd className="muted">A top list: source, channel, referrer, utm_campaign, page, entry, exit, country, region, city, device, browser, os, language, event, outbound, download, click.</dd></div>
          <div><dt><code>GET /summary</code></dt><dd className="muted">Every site's today, yesterday and week — for Homepage.</dd></div>
          <div><dt><code>GET /day/YYYY-MM-DD</code></dt><dd className="muted">Visitors per site on a day — what Pocket Day reads.</dd></div>
        </dl>
      </Section>
    </div>
  )
}

function Data() {
  const { user } = useApp()
  const { toast } = useToast()
  const pick = async (file: File) => {
    try {
      const r = await fetch(`${V1}/import`, { method: 'POST', body: await file.text(), credentials: 'same-origin', headers: { 'Content-Type': 'application/json' } })
      const j = await r.json()
      if (!r.ok) throw new Error(j.error || r.statusText)
      toast(`Added ${j.sites} ${j.sites === 1 ? 'site' : 'sites'}, ${j.goals} goals, ${j.funnels} funnels${j.skipped ? ` (${j.skipped} skipped)` : ''}`)
      announceChange()
    } catch (e) { toast((e as Error).message, 'err') }
  }
  return (
    <div className="stack loose">
      <Section title="Export" icon="archive" sub="Your websites with their settings, goals and funnels, as one JSON file — to bring back here or on another box. Visits are downloaded per site (site settings → Data) as spreadsheets.">
        <a className="btn" href={`${V1}/export.json`}><Icon name="download" size={15} />Export JSON</a>
      </Section>
      <Section title="Bring sites back" icon="upload" sub="A Pocket Analytics export. Sites already here keep their settings; goals and funnels with the same name are not added twice.">
        <label className="btn"><Icon name="upload" size={15} />Choose a file…<input type="file" accept="application/json,.json" hidden onChange={e => { const fl = e.target.files?.[0]; if (fl) pick(fl); e.target.value = '' }} /></label>
      </Section>
      {user?.role === 'admin' && <Section title="Database backup" icon="database" sub="The whole box, every account — admins only.">
        <p className="muted">A consistent copy of the SQLite database (<code>VACUUM INTO</code>). Restore by stopping the app and replacing <code>pocket-analytics.db</code> in the data folder.</p>
        <a className="btn" href={`${V1}/backup`}><Icon name="database" size={15} />Download database</a>
      </Section>}
    </div>
  )
}

function About() {
  const { data, error } = useAsync(() => api.get<Record<string, unknown>>(`${V1}/settings/system`), [])
  const [keys, setKeys] = useState(false)
  if (error) return <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>
  if (!data) return <Loading />
  const counts = (data.counts || {}) as Record<string, number>
  const core = (data.core || {}) as { configured?: boolean; registered?: boolean; url?: string; error?: string }
  const tiles: [string, number | undefined, string][] = [['Websites', counts.sites, 'globe'], ['Goals', counts.goals, 'target'], ['Funnels', counts.funnels, 'trending-down'], ['API tokens', counts.api_tokens, 'key']]
  const rows: [string, React.ReactNode][] = [
    ['Version', <code key="v">{String(data.version)}</code>],
    ['Uptime', String(data.uptime)],
    ['Runtime', `${String(data.go)} · ${String(data.os)}/${String(data.arch)}`],
    ['Time zone', String(data.timezone)],
    ['Sign-in', String(data.auth_mode)],
    ['Pocket Core', core.configured ? (core.registered ? <span key="c" className="ok">connected · {core.url}</span> : <span key="c" className="bad">{core.error || 'not registered yet'}</span>) : <span key="c" className="muted">not configured</span>],
    ['Database', <span key="db"><code>{String(data.db_path)}</code> · {fmtBytes(Number(data.db_bytes))}</span>],
    ['People', `${counts.users ?? 0} accounts on this box`],
    ['Places', <span key="g">{String(data.geo || '')}</span>],
  ]
  return (
    <div className="stack loose">
      <div className="set-about-hero">
        <img src="/favicon.svg" alt="" width={48} height={48} />
        <div>
          <h3>Pocket Analytics</h3>
          <p className="muted small">Who visits your websites, without cookies or a banner: visitors, pages, sources, places, devices, goals and funnels. The IP address is never kept, and a visitor cannot be followed from one day to the next. One binary, one SQLite file, one volume.</p>
        </div>
      </div>
      <div className="set-stats">
        {tiles.map(([label, n, icon]) => (
          <div key={label} className="set-stat"><Icon name={icon} size={16} /><span className="set-stat-n tnum">{(n ?? 0).toLocaleString()}</span><span className="set-stat-l">{label}</span></div>
        ))}
      </div>
      <Section title="This server">
        <dl className="set-dl">
          {rows.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}
        </dl>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn" onClick={() => setKeys(true)}><Icon name="keyboard" size={15} />Keyboard shortcuts</button>
        <a className="btn" href="https://github.com/abjelosevic88/pocket-homelab-analytics" target="_blank" rel="noreferrer"><Icon name="external" size={15} />Project on GitHub</a>
      </div>
      <p className="muted small">Open source (MIT). Countries and cities come from <a href="https://db-ip.com" target="_blank" rel="noreferrer">IP Geolocation by DB-IP</a> (CC BY 4.0), looked up on this box. Part of the Pocket apps: Pocket Core connects them — Mail, Notes, Tasks, Day, Search, Pocket AI and more.</p>
      {keys && <ShortcutsSheet onClose={() => setKeys(false)} />}
    </div>
  )
}

