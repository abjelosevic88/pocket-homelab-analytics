// README screenshots and a browser smoke test, against a demo instance (DEMO_DATA=true seeds two websites with four months of visits):
//
//   DATA_DIR=/tmp/analytics-demo DEMO_DATA=true GEO_DB=off ADMIN_EMAIL=demo@example.com ADMIN_PASSWORD=demodemo1 ADMIN_NAME=Lejla PORT=18282 ./pocket-analytics
//   CDP=http://127.0.0.1:9338 BASE=http://127.0.0.1:18282 node scripts/screenshots.mjs   # any Chrome with remote debugging
//   SHOTS=0 … node scripts/screenshots.mjs                                                # checks only
// playwright-core comes from web/node_modules, or set PW_FROM to a package.json next to an install of it.
import { createRequire } from 'node:module'
import assert from 'node:assert/strict'

const require = createRequire(process.env.PW_FROM || new URL('../web/package.json', import.meta.url))
let pw
try { pw = require('playwright') } catch { pw = require('playwright-core') }
const { chromium } = pw
const base = process.env.BASE || 'http://127.0.0.1:18282'
const shots = process.env.SHOTS !== '0'
const only = process.env.ONLY || ''
const dir = new URL('../docs/screenshots/', import.meta.url).pathname
const browser = process.env.CDP ? await chromium.connectOverCDP(process.env.CDP) : await chromium.launch()
const errors = []

async function session({ phone = false, dark = false } = {}) {
  const ctx = await browser.newContext({
    viewport: phone ? { width: 390, height: 844 } : { width: 1440, height: 900 },
    deviceScaleFactor: phone ? 3 : 2, isMobile: phone, hasTouch: phone, colorScheme: dark ? 'dark' : 'light',
    userAgent: phone ? 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1' : undefined,
  })
  const page = await ctx.newPage()
  page.on('pageerror', e => errors.push(`pageerror: ${e}`))
  page.on('console', m => { if (m.type() === 'error' && !/Failed to load resource|Content Security Policy/.test(m.text())) errors.push(m.text()) })
  await page.goto(base + '/login')
  const res = await page.evaluate(() => fetch('/api/v1/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: 'demo@example.com', password: 'demodemo1' }) }).then(r => r.status))
  assert.equal(res, 200, 'demo sign-in')
  return { ctx, page }
}
async function go(page, path, wait = 1500, frozen = true) {
  await page.goto(base + path, { waitUntil: 'networkidle' })
  // headless Chrome freezes CSS fade-ins: show everything at once
  if (frozen) await page.addStyleTag({ content: '*, *::before, *::after { transition: none !important; animation: none !important; }' }).catch(() => {})
  await page.evaluate(() => document.fonts.ready)
  await page.waitForTimeout(wait)
}
async function shot(page, name) { if (shots && (!only || only.split(',').includes(name))) await page.screenshot({ path: dir + name + '.png' }) }
const api = async (page, path) => page.evaluate(p => fetch('/api/v1' + p).then(r => r.json()), path)
const siteID = async (page, domain) => (await api(page, '/sites')).sites.find(s => s.domain === domain).id

// ---- desktop ----
{
  const { ctx, page } = await session()
  const bakery = await siteID(page, 'maplebakery.example')
  const blog = await siteID(page, 'fieldnotes.example')
  await go(page, '/', 1500)
  assert.ok((await page.content()).includes('Maple Bakery'), 'the sites are listed')
  await shot(page, 'desktop-sites')
  await go(page, `/sites/${bakery}?range=30d`, 2500)
  assert.ok((await page.content()).includes('Where they came from'), 'the site page opens')
  await shot(page, 'desktop-dashboard')
  await page.evaluate(() => window.scrollTo(0, 0))
  await page.locator('.content').evaluate(el => el.scrollTo(0, 620))
  await page.waitForTimeout(600)
  await shot(page, 'desktop-panels')
  assert.ok((await page.content()).includes('How fast the pages are'), 'speed shows')
  assert.ok(await page.locator('.when-c').count() === 168, 'a week of hours shows')
  await page.locator('.content').evaluate(el => el.scrollTo(0, el.querySelector('.when-grid').closest('.panel').offsetTop - 16))
  await page.waitForTimeout(600)
  await shot(page, 'desktop-speed')
  await go(page, `/sites/${bakery}?range=30d&f=page:/menu`, 2500)
  assert.ok((await page.content()).includes('The way through'), 'the way through a page shows')
  await shot(page, 'desktop-paths')
  await go(page, `/sites/${bakery}?range=30d&f=event:Order+placed`, 2000)
  assert.ok((await page.content()).includes('All events'), 'an event opens its details')
  await go(page, `/sites/${bakery}/settings/links`, 1000)
  assert.ok((await page.content()).includes('A link that says where it was'), 'campaign links open')
  await go(page, `/sites/${blog}?range=30d&f=source:Hacker+News`, 2500)
  assert.ok((await page.content()).includes('Hacker News'), 'a filter shows')
  await shot(page, 'desktop-filtered')
  await go(page, `/sites/${bakery}/live`, 2000)
  assert.ok((await page.content()).includes('The last half hour'), 'right now opens')
  await shot(page, 'desktop-live')
  const visits = await api(page, `/sites/${bakery}/visits?range=7d&limit=50`)
  const long = visits.visits.find(v => v.pageviews >= 3 && v.events > 0) || visits.visits[0]
  await go(page, `/sites/${bakery}/visits?range=7d&visit=${long.id}`, 2000)
  assert.ok((await page.content()).includes('One visit'), 'a visit opens')
  await shot(page, 'desktop-visit')
  await go(page, `/sites/${bakery}/settings/install`, 1500)
  assert.ok((await page.content()).includes('Put this in every page'), 'install opens')
  await shot(page, 'desktop-install')
  await go(page, `/sites/${bakery}/settings/funnels`, 1200)
  await go(page, '/settings/notifications', 1000)
  await shot(page, 'desktop-settings')
  await ctx.close()
}

// ---- dark ----
{
  const { ctx, page } = await session({ dark: true })
  const bakery = await siteID(page, 'maplebakery.example')
  await go(page, `/sites/${bakery}?range=12mo`, 2500)
  await shot(page, 'desktop-dark')
  await ctx.close()
}

// ---- phone ----
{
  const { ctx, page } = await session({ phone: true })
  const bakery = await siteID(page, 'maplebakery.example')
  await go(page, '/', 1500)
  await shot(page, 'phone-sites')
  await go(page, `/sites/${bakery}?range=7d`, 2500)
  await shot(page, 'phone-dashboard')
  await go(page, `/sites/${bakery}/live`, 2000)
  await shot(page, 'phone-live')
  await ctx.close()
}

await browser.close()
if (errors.length) { console.error(errors.join('\n')); process.exit(1) }
console.log('ok')
