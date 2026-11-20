import { Modal } from './ui'

const GROUPS: { title: string; keys: [string[], string][] }[] = [
  { title: 'Go to', keys: [[['h'], 'All websites'], [['1'], 'The first website (1–9)'], [['o'], "The site's overview"], [['l'], 'Right now'], [['v'], 'Visits'], [['s'], 'Site settings'], [[','], 'Settings'], [['/'], 'Search & commands'], [['⌘', 'K'], 'Search & commands']] },
  { title: 'Time', keys: [[['d'], 'Today'], [['e'], 'Yesterday'], [['w'], 'Last 7 days'], [['t'], 'Last 30 days'], [['m'], 'This month'], [['y'], 'Last 12 months'], [['a'], 'All time']] },
  { title: 'Make', keys: [[['n'], 'Add a website']] },
  { title: 'Window', keys: [[['['], 'Show or hide the sidebar'], [['esc'], 'Close a dialog or menu']] },
  { title: 'Help', keys: [[['?'], 'This sheet']] },
]

export function ShortcutsSheet({ onClose }: { onClose: () => void }) {
  return (
    <Modal title="Keyboard shortcuts" onClose={onClose} size="lg">
      <div className="shortcuts">
        {GROUPS.map(g => (
          <section key={g.title}>
            <h4 className="sc-h">{g.title}</h4>
            {g.keys.map(([keys, label]) => (
              <div key={label + keys.join('')} className="sc-row"><span>{label}</span><span className="sc-keys">{keys.map(k => <kbd key={k}>{k}</kbd>)}</span></div>
            ))}
          </section>
        ))}
      </div>
    </Modal>
  )
}
