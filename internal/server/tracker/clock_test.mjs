// Reading time must not grow while a page waits in the back/forward cache.
// Runs the tracker against a fake browser with a clock we move by hand:
//   node internal/server/tracker/clock_test.mjs
import fs from 'fs'
import vm from 'vm'
const src = fs.readFileSync(new URL('./script.js', import.meta.url), 'utf8')
let clock = 1_000_000
const sent = []
const dl = {}, wl = {}
const on = (m) => (t, f) => { (m[t] = m[t] || []).push(f) }
const fire = (m, t, e = {}) => (m[t] || []).forEach(f => f({ type: t, ...e }))
const document = {
  currentScript: { getAttribute: k => ({ 'data-domain': 'example.com', 'data-api': '/e' })[k] || null, hasAttribute: () => false, src: 'https://example.com/js/script.js' },
  visibilityState: 'visible', referrer: '', addEventListener: on(dl), removeEventListener() {},
  querySelectorAll: () => [], querySelector: () => null, body: { scrollHeight: 3000, offsetHeight: 3000 }, documentElement: { clientHeight: 800, scrollHeight: 3000, offsetHeight: 3000, scrollTop: 0 },
}
const location = { href: 'https://example.com/a', pathname: '/a', hash: '', search: '', hostname: 'example.com', host: 'example.com', protocol: 'https:' }
const window = { addEventListener: on(wl), innerWidth: 1200, innerHeight: 800, scrollY: 0, history: {}, requestAnimationFrame: f => f() }
const navigator = { sendBeacon: (u, b) => { sent.push(JSON.parse(b.text)); return true } }
class Blob { constructor(parts) { this.text = parts.join('') } }
const RealDate = Date
class FakeDate extends RealDate { static now() { return clock } }
const ctx = vm.createContext({ document, window, location, navigator, Blob, Date: FakeDate, URL, URLSearchParams, localStorage: { getItem: () => null, setItem() {}, removeItem() {} }, setTimeout: f => f(), performance: { getEntriesByType: () => [] }, alert() {}, console })
vm.runInContext(src, ctx)
const wait = s => { clock += s * 1000 }
const hide = () => { document.visibilityState = 'hidden'; fire(dl, 'visibilitychange') }
const show = () => { document.visibilityState = 'visible'; fire(dl, 'visibilitychange') }

// reads /a for 5 s, follows a link (the page goes to the back/forward cache),
// is away for 60 s, presses Back, reads 4 s more, closes the tab
wait(5)
fire(wl, 'pagehide', { persisted: true }); hide()
wait(60)
fire(wl, 'pageshow', { persisted: true }); show()
wait(4)
fire(wl, 'pagehide', { persisted: false }); hide()

const eng = sent.filter(x => x.n === 'engagement').map(x => Math.round(x.e / 1000))
const total = eng.reduce((a, b) => a + b, 0)
console.log('pageviews:', sent.filter(x => x.n === 'pageview').length, '| reading reports (s):', eng.join(' + '), '=', eng.reduce((a, b) => a + b, 0), '(the person read 9 s)')
if (total !== 9) { console.error('FAIL: time away was counted as reading'); process.exit(1) }
