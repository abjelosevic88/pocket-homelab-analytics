/* Pocket Analytics — counts visits without cookies. https://github.com/abjelosevic88/pocket-homelab-analytics */
(function () {
  'use strict'
  var d = document, w = window, l = location, n = navigator
  var s = d.currentScript
  if (!s || w.__pocketAnalytics) return
  w.__pocketAnalytics = 1
  var attr = function (k) { return s.getAttribute(k) }
  var api = attr('data-api') || new URL('/api/event', s.src).href
  var domain = attr('data-domain') || l.hostname.replace(/^www\./, '')
  var hash = s.hasAttribute('data-hash')
  var outbound = !s.hasAttribute('data-no-outbound')
  var downloads = !s.hasAttribute('data-no-downloads')
  var clicks = s.hasAttribute('data-clicks')
  var exts = (attr('data-file-types') || 'pdf,zip,dmg,pkg,exe,msi,iso,img,apk,deb,rpm,gz,tgz,7z,rar,csv,xls,xlsx,doc,docx,ppt,pptx,epub,mp3,mp4,mov,txt,json').split(',')

  // "Don't count me": ?pocket_ignore=1 once, in this browser
  try {
    var q = new URLSearchParams(l.search).get('pocket_ignore')
    if (q === '1' || q === '0') {
      if (q === '1') localStorage.setItem('pocket_ignore', '1'); else localStorage.removeItem('pocket_ignore')
      alert(q === '1' ? 'Pocket Analytics will not count this browser on ' + domain + '.' : 'Pocket Analytics counts this browser again.')
    }
  } catch (e) {}
  var ignored = function () { try { return localStorage.getItem('pocket_ignore') === '1' } catch (e) { return false } }

  function send(name, opts, cb) {
    opts = opts || {}
    if (n.webdriver || w._phantom || w.__nightmare || w.Cypress || ignored() || /^file:/.test(l.protocol)) { cb && cb(); return }
    var p = { n: name, u: opts.u || l.href, d: domain, r: opts.r === undefined ? null : opts.r, w: w.innerWidth }
    if (hash) p.h = 1
    if (opts.props) p.p = opts.props
    if (name === 'engagement') { p.e = opts.e; p.sd = opts.sd }
    var body = JSON.stringify(p)
    try {
      if (n.sendBeacon && n.sendBeacon(api, new Blob([body], { type: 'text/plain' }))) { cb && cb(); return }
    } catch (e) {}
    try {
      fetch(api, { method: 'POST', body: body, keepalive: true, headers: { 'Content-Type': 'text/plain' } }).then(function () { cb && cb() }, function () { cb && cb() })
    } catch (e) { cb && cb() }
  }

  // ---- time on page and how far down ----
  var shown = 0, since = 0, maxScroll = 0, sentScroll = 0, page = ''
  function now() { return Date.now() }
  function docHeight() {
    var b = d.body || {}, e = d.documentElement || {}
    return Math.max(b.scrollHeight || 0, b.offsetHeight || 0, e.clientHeight || 0, e.scrollHeight || 0, e.offsetHeight || 0)
  }
  function scrolled() {
    var h = docHeight(), v = w.innerHeight || 0, y = w.scrollY || (d.documentElement || {}).scrollTop || 0
    return h <= v ? 100 : Math.min(100, Math.round((y + v) / h * 100))
  }
  function onScroll() { var p = scrolled(); if (p > maxScroll) maxScroll = p }
  function engaged() {
    var t = shown
    if (since) t += now() - since
    return t
  }
  function report() {
    if (!page) return
    var t = engaged()
    if (t < 1000 && maxScroll <= sentScroll) return
    send('engagement', { u: page, e: t, sd: maxScroll })
    shown = 0; since = d.visibilityState === 'visible' ? now() : 0; sentScroll = maxScroll
  }
  d.addEventListener('visibilitychange', function () {
    if (d.visibilityState === 'hidden') report()
    else if (!since) since = now()
  })
  w.addEventListener('pagehide', report)
  w.addEventListener('scroll', function () { w.requestAnimationFrame ? w.requestAnimationFrame(onScroll) : onScroll() }, { passive: true })

  // ---- pages ----
  var first = true, last = ''
  function pageview() {
    var where = l.pathname + (hash ? l.hash : '')
    if (where === last) return
    if (page) report()
    last = where
    page = l.href
    shown = 0; since = d.visibilityState === 'visible' ? now() : 0
    maxScroll = 0; sentScroll = 0
    setTimeout(onScroll, 300)
    send('pageview', { r: first ? d.referrer || null : null })
    first = false
  }
  var hs = w.history
  if (hs && hs.pushState) {
    var push = hs.pushState
    hs.pushState = function () { push.apply(this, arguments); setTimeout(pageview, 0) }
    w.addEventListener('popstate', function () { setTimeout(pageview, 0) })
  }
  if (hash) w.addEventListener('hashchange', pageview)

  // ---- clicks: other sites, files, tagged elements, everything ----
  function text(el) {
    var t = (el.getAttribute('aria-label') || el.innerText || el.value || el.title || el.getAttribute('alt') || '').replace(/\s+/g, ' ').trim()
    return t.length > 80 ? t.slice(0, 79) + '…' : t
  }
  function tagged(el) {
    var name = el.getAttribute('data-pocket-event'), props = {}, i, a
    var m = (el.className && typeof el.className === 'string' ? el.className : '').match(/plausible-event-name=(\S+)/)
    if (!name && m) name = m[1].replace(/\+/g, ' ')
    if (!name) return null
    for (i = 0; i < el.attributes.length; i++) {
      a = el.attributes[i]
      if (a.name.indexOf('data-pocket-prop-') === 0) props[a.name.slice(17)] = a.value
    }
    ;((el.className && typeof el.className === 'string' ? el.className : '').match(/plausible-event-(?!name)[^=\s]+=\S+/g) || []).forEach(function (c) {
      var kv = c.slice(16).split('='); props[kv[0]] = kv.slice(1).join('=').replace(/\+/g, ' ')
    })
    return { name: name, props: props }
  }
  function onClick(e) {
    if (e.type === 'auxclick' && e.button !== 1) return
    var el = e.target && e.target.closest ? e.target.closest('a, button, [role="button"], input[type="submit"], input[type="button"], [data-pocket-event], [class*="plausible-event-name="]') : null
    if (!el) return
    var t = tagged(el) || (el.parentElement && el.parentElement.closest && el.parentElement.closest('[data-pocket-event]') ? tagged(el.parentElement.closest('[data-pocket-event]')) : null)
    var href = el.tagName === 'A' ? el.href : ''
    // sendBeacon and keepalive outlive the page, so nothing waits here
    if (t) send(t.name, { props: t.props })
    if (href) {
      var u
      try { u = new URL(href, l.href) } catch (x) { u = null }
      if (u && /^https?:$/.test(u.protocol)) {
        var ext = (u.pathname.split('.').pop() || '').toLowerCase()
        if (downloads && u.pathname.indexOf('.') > 0 && exts.indexOf(ext) >= 0) send('File Download', { props: { url: u.origin + u.pathname } })
        else if (outbound && u.host !== l.host) send('Outbound Link: Click', { props: { url: u.origin + u.pathname } })
      }
    }
    if (clicks && !t) send('Click', { props: { text: text(el) || '(' + el.tagName.toLowerCase() + ')' } })
  }
  d.addEventListener('click', onClick, true)
  d.addEventListener('auxclick', onClick, true)

  // ---- the site's own events: pocket('Signed up', { props: { plan: 'pro' } }) ----
  function api_(name, o) { o = o || {}; send(name, { props: o.props, u: o.u }, o.callback) }
  var queued = (w.pocket && w.pocket.q) || []
  w.pocket = api_
  if (!w.plausible) w.plausible = api_
  for (var i = 0; i < queued.length; i++) api_.apply(this, queued[i])

  if (d.visibilityState === 'prerender') d.addEventListener('visibilitychange', function f() { if (d.visibilityState === 'visible') { d.removeEventListener('visibilitychange', f); pageview() } })
  else pageview()
})()
