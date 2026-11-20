// Left column: add a website, then every website with the people on it
// now; the open site shows its own pages under it.
import { Link, NavLink, useLocation } from 'react-router-dom'
import { useApp } from '../lib/app-context'
import { useData } from '../lib/data'
import { Icon } from './icons'
import type { CreateKind } from './Layout'

export function Sidebar({ onCreate, onNavigate }: { onCreate: (k: CreateKind) => void; onNavigate: () => void }) {
  const { status, settings } = useApp()
  const { sites } = useData()
  const loc = useLocation()
  const title = settings?.title || status?.title || 'Pocket Analytics'
  const open = (loc.pathname.match(/^\/sites\/(\d+)/) || [])[1]
  // the range and filters go along between a site's pages
  const keep = new URLSearchParams(loc.search)
  keep.delete('visit')
  const q = keep.toString() ? `?${keep}` : ''
  return (
    <aside className="sidebar" aria-label="Pocket Analytics">
      <div className="sb-brand">
        <img src="/favicon.svg" alt="" className="sb-logo" />
        <span className="sb-title">{title !== 'Pocket Analytics' ? title : <>Pocket <b>Analytics</b></>}</span>
        {status?.version && <span className="sb-version" title={`Version ${status.version}`}>{/^v?\d+\.\d+/.test(status.version) ? 'v' + status.version.replace(/^v/, '').replace(/[-+].*$/, '') : status.version}</span>}
      </div>
      <button type="button" className="btn primary create-btn" onClick={() => onCreate('site')} title="Add a website (N)">
        <Icon name="plus" size={15} strokeWidth={2.2} />Add a website
      </button>

      <nav className="sb-nav" aria-label="Sections">
        <NavLink to="/" end onClick={onNavigate} className={({ isActive }) => `sb-nav-item ${isActive ? 'active' : ''}`}><Icon name="grid" size={16} />All websites</NavLink>
      </nav>

      {!!sites?.length && <section className="sb-sec">
        <h4 className="sb-h">Websites</h4>
        {sites.map(s => {
          const on = String(s.id) === open
          return (
            <div key={s.id} className="sb-site">
              <Link to={`/sites/${s.id}${on ? q : ''}`} className={`sb-row sb-list ${on && loc.pathname === `/sites/${s.id}` ? 'active' : ''}`} onClick={onNavigate} title={s.domain}>
                <span className="sb-list-ico" aria-hidden="true"><Icon name="globe" size={14} /></span>
                <span className="grow ellipsis">{s.label}</span>
                {s.live ? <span className="sb-live" title={`${s.live} on the site now`}><span className="live-dot sm" />{s.live}</span> : !s.installed ? <Icon name="hourglass" size={12} className="muted" /> : null}
              </Link>
              {on && <div className="sb-sub">
                <NavLink end to={`/sites/${s.id}/live`} onClick={onNavigate} className={({ isActive }) => `sb-subitem ${isActive ? 'active' : ''}`}><Icon name="activity" size={14} />Right now</NavLink>
                <NavLink end to={`/sites/${s.id}/visits${q}`} onClick={onNavigate} className={({ isActive }) => `sb-subitem ${isActive ? 'active' : ''}`}><Icon name="list" size={14} />Visits</NavLink>
                <NavLink to={`/sites/${s.id}/settings`} onClick={onNavigate} className={({ isActive }) => `sb-subitem ${isActive ? 'active' : ''}`}><Icon name="settings" size={14} />Site settings</NavLink>
              </div>}
            </div>
          )
        })}
      </section>}

      <div className="sb-foot">
        <Link to="/settings" className="sb-foot-link" onClick={onNavigate}><Icon name="settings" size={15} />Settings</Link>
        <button type="button" className="sb-foot-link" onClick={() => { onNavigate(); window.dispatchEvent(new CustomEvent('pan:shortcuts')) }}><Icon name="keyboard" size={15} />Shortcuts</button>
      </div>
    </aside>
  )
}
