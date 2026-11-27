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
  var forms = !s.hasAttribute('data-no-forms')
  var speed = !s.hasAttribute('data-no-speed')
  var missing = !s.hasAttribute('data-no-404')
  var identify = s.hasAttribute('data-identify')
  // data-file-types="pdf,zip" replaces the list; "+env,yml" adds to it
  var defaultExts = 'pdf,zip,dmg,pkg,exe,msi,iso,img,apk,deb,rpm,gz,tgz,7z,rar,csv,xls,xlsx,doc,docx,ppt,pptx,epub,mp3,mp4,mov,txt,json'
  var fileTypes = (attr('data-file-types') || '').replace(/\s+/g, '')
  var exts = (fileTypes.charAt(0) === '+' ? defaultExts + ',' + fileTypes.slice(1) : fileTypes || defaultExts).toLowerCase().split(',')

  // "Don't count me": ?pocket_ignore=1 once, in this browser
  try {
    var q = new URLSearchParams(l.search).get('pocket_ignore')
    if (q === '1' || q === '0') {
      if (q === '1') localStorage.setItem('pocket_ignore', '1'); else localStorage.removeItem('pocket_ignore')
      alert(q === '1' ? 'Pocket Analytics will not count this browser on ' + domain + '.' : 'Pocket Analytics counts this browser again.')
    }
  } catch (e) {}
  var ignored = function () { try { return localStorage.getItem('pocket_ignore') === '1' } catch (e) { return false } }

  // ---- what the browser says about itself. data-device: the screen, time
  // zone, languages and the like, nothing kept in the browser. data-identify
  // (only behind the visitor's consent): also a random ID this browser keeps
  // (returning visitors) and the hardware. Neither when the browser asks not
  // to be tracked (Global Privacy Control, Do Not Track) ----
  var device = s.hasAttribute('data-device') || identify
  var asked = !!(n.globalPrivacyControl || n.doNotTrack === '1' || w.doNotTrack === '1')
  if (asked) { device = false; identify = false }
  var bid = ''
  try {
    if (identify) {
      bid = localStorage.getItem('pocket_id') || ''
      if (!/^[\w-]{8,64}$/.test(bid)) {
        var r = new Uint8Array(12); (w.crypto || w.msCrypto).getRandomValues(r)
        bid = Array.prototype.map.call(r, function (b) { return ('0' + b.toString(16)).slice(-2) }).join('')
        localStorage.setItem('pocket_id', bid)
      }
    } else if (localStorage.getItem('pocket_id')) localStorage.removeItem('pocket_id')
  } catch (e) { bid = '' }
  function about() {
    var x = {}, sc = w.screen || {}, c = n.connection || n.mozConnection || n.webkitConnection, mq = function (q) { try { return w.matchMedia(q).matches } catch (e) { return false } }
    try {
      x.screen = sc.width + '×' + sc.height
      x.window = (w.innerWidth || 0) + '×' + (w.innerHeight || 0)
      x.dpr = Math.round((w.devicePixelRatio || 1) * 100) / 100
      x.timezone = Intl.DateTimeFormat().resolvedOptions().timeZone
      x.utc_offset = -new Date().getTimezoneOffset()
      x.languages = n.languages ? [].slice.call(n.languages, 0, 10) : n.language
      x.touch = n.maxTouchPoints || 0
      x.dark = mq('(prefers-color-scheme: dark)')
      if (mq('(prefers-reduced-motion: reduce)')) x.reduced_motion = true
      if (c) { x.net = c.effectiveType; if (c.type) x.net_type = c.type; if (c.saveData) x.save_data = true }
      if (mq('(display-mode: standalone)')) x.installed_app = true
    } catch (e) {}
    if (!identify) return x
    try {
      x.colors = sc.colorDepth
      x.platform = n.platform
      x.cores = n.hardwareConcurrency
      if (n.deviceMemory) x.memory_gb = n.deviceMemory
      if (c && c.downlink) x.downlink_mbps = c.downlink
      if (c && c.rtt) x.rtt_ms = c.rtt
      x.cookies = !!n.cookieEnabled
      if (n.pdfViewerEnabled) x.pdf_viewer = true
      var cv = d.createElement('canvas'), gl = cv.getContext('webgl') || cv.getContext('experimental-webgl')
      var ext = gl && gl.getExtension('WEBGL_debug_renderer_info')
      if (ext) { x.gpu = gl.getParameter(ext.UNMASKED_RENDERER_WEBGL); x.gpu_vendor = gl.getParameter(ext.UNMASKED_VENDOR_WEBGL) }
    } catch (e) {}
    return x
  }
  // Chrome, Edge and others say more when asked: the real system version, the phone's model
  function aboutMore(x, done) {
    var ua = n.userAgentData
    if (!identify || !ua || !ua.getHighEntropyValues) { done(x); return }
    var t = setTimeout(function () { done(x); done = function () {} }, 300)
    ua.getHighEntropyValues(['model', 'platformVersion', 'architecture', 'bitness', 'fullVersionList']).then(function (h) {
      if (h.model) x.model = h.model
      if (h.platform) x.os_name = h.platform
      if (h.platformVersion) x.os_version = h.platformVersion
      if (h.architecture) x.arch = h.architecture + (h.bitness ? '-' + h.bitness : '')
      var b = (h.fullVersionList || []).filter(function (v) { return !/Not.?A.?Brand|Chromium/i.test(v.brand) })[0]
      if (b) x.browser_version = b.brand + ' ' + b.version
      if (ua.mobile) x.mobile = true
      clearTimeout(t); done(x); done = function () {}
    }, function () { clearTimeout(t); done(x); done = function () {} })
  }

  function send(name, opts, cb) {
    opts = opts || {}
    if (n.webdriver || w._phantom || w.__nightmare || w.Cypress || ignored() || /^file:/.test(l.protocol)) { cb && cb(); return }
    var p = { n: name, u: opts.u || l.href, d: domain, r: opts.r === undefined ? null : opts.r, w: w.innerWidth }
    if (hash) p.h = 1
    if (opts.props) p.p = opts.props
    if (name === 'engagement') { p.e = opts.e; p.sd = opts.sd; if (opts.v) p.v = opts.v; if (opts.sec && opts.sec.length) p.sec = opts.sec }
    if (opts.rev) p.$ = opts.rev
    if (bid && name !== 'engagement') p.id = bid
    if (asked) p.np = 1
    if (opts.x) p.x = opts.x
    var body = JSON.stringify(p)
    try {
      if (n.sendBeacon && n.sendBeacon(api, new Blob([body], { type: 'text/plain' }))) { cb && cb(); return }
    } catch (e) {}
    try {
      fetch(api, { method: 'POST', body: body, keepalive: true, headers: { 'Content-Type': 'text/plain' } }).then(function () { cb && cb() }, function () { cb && cb() })
    } catch (e) { cb && cb() }
  }

  // ---- how fast the page was: only the page the browser loaded, and only
  // when it was in front from the start ----
  var vit = {}, hid = d.visibilityState !== 'visible', loaded = l.href, PO = w.PerformanceObserver
  function watch(type, fn, more) {
    try {
      if ((PO.supportedEntryTypes || []).indexOf(type) < 0) return
      var o = { type: type, buffered: true }
      if (more) o.durationThreshold = more
      new PO(function (list) { list.getEntries().forEach(fn) }).observe(o)
    } catch (e) {}
  }
  if (speed && PO && !hid) {
    try {
      var nav = performance.getEntriesByType('navigation')[0]
      if (nav && nav.responseStart > 0) vit.t = Math.round(nav.responseStart)
    } catch (e) {}
    watch('paint', function (e) { if (e.name === 'first-contentful-paint' && !hid) vit.f = Math.round(e.startTime) })
    watch('largest-contentful-paint', function (e) { if (!hid) vit.l = Math.round(e.startTime) })
    // how much the layout moved: the worst burst of shifts (under a second apart, five seconds at most)
    var cv = 0, cf = 0, cl = 0
    if ((PO.supportedEntryTypes || []).indexOf('layout-shift') >= 0) vit.c = 0
    watch('layout-shift', function (e) {
      if (e.hadRecentInput) return
      if (cv && e.startTime - cl < 1000 && e.startTime - cf < 5000) cv += e.value
      else { cv = e.value; cf = e.startTime }
      cl = e.startTime
      if (cv > vit.c) vit.c = Math.round(cv * 1000) / 1000
    })
    // how long a press takes to show something: the slowest (with many presses, nearly the slowest)
    var slow = [], presses = 0
    var press = function (e) {
      if (!e.interactionId) return
      var k, x = null
      for (k = 0; k < slow.length; k++) if (slow[k].id === e.interactionId) x = slow[k]
      if (x) x.d = Math.max(x.d, e.duration)
      else { presses++; slow.push({ id: e.interactionId, d: e.duration }) }
      slow.sort(function (a, b) { return b.d - a.d })
      if (slow.length > 10) slow.length = 10
      vit.i = Math.round(slow[Math.min(slow.length - 1, Math.floor(presses / 50))].d)
    }
    watch('event', press, 40)
    watch('first-input', press)
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
  // ---- the parts of the page that were on screen: elements marked
  // data-pocket-section="Pricing" (a quarter of one in view, or half the screen) ----
  var secOrder = [], secSeen = {}, secSent = 0, secCount = 0, secObs = null
  function watchSections() {
    if (secObs) secObs.disconnect()
    secOrder = []; secSeen = {}; secSent = 0; secCount = 0
    var els = d.querySelectorAll('[data-pocket-section]'), i, nm
    if (!els.length || !w.IntersectionObserver) return
    secObs = new IntersectionObserver(function (es) {
      es.forEach(function (e) {
        var vh = w.innerHeight || 1
        if (!e.isIntersecting || (e.intersectionRatio < 0.25 && e.intersectionRect.height < vh / 2)) return
        var n = e.target.getAttribute('data-pocket-section')
        if (n && !secSeen[n]) { secSeen[n] = 1; secCount++ }
        secObs.unobserve(e.target)
      })
    }, { threshold: [0, 0.1, 0.25, 0.5] })
    for (i = 0; i < els.length; i++) {
      nm = (els[i].getAttribute('data-pocket-section') || '').trim()
      if (nm && secOrder.indexOf(nm) < 0) secOrder.push(nm)
      secObs.observe(els[i])
    }
  }
  function sectionsSeen() {
    var out = [], i
    for (i = 0; i < secOrder.length; i++) if (secSeen[secOrder[i]]) out.push(secOrder[i])
    return out
  }
  function report() {
    if (!page) return
    var t = engaged()
    if (t < 1000 && maxScroll <= sentScroll && secCount <= secSent) return
    send('engagement', { u: page, e: t, sd: maxScroll, v: page === loaded && speed && PO ? vit : null, sec: secCount > secSent ? sectionsSeen() : null })
    shown = 0; since = d.visibilityState === 'visible' ? now() : 0; sentScroll = maxScroll; secSent = secCount
  }
  // the clock runs only while the page is in front: leaving it (another tab,
  // another page) stops it, also when the browser keeps the page for Back
  function away() { report(); if (since) { shown += now() - since; since = 0 } }
  function back() { if (!since && d.visibilityState === 'visible') since = now() }
  d.addEventListener('visibilitychange', function () {
    if (d.visibilityState === 'hidden') { away(); hid = true }
    else back()
  })
  w.addEventListener('pagehide', away)
  w.addEventListener('pageshow', function (e) { if (e.persisted) back() })
  w.addEventListener('scroll', function () { w.requestAnimationFrame ? w.requestAnimationFrame(onScroll) : onScroll() }, { passive: true })

  // ---- pages ----
  var first = true, last = ''
  function pageview() {
    var where = l.pathname + (hash ? l.hash : '')
    if (where === last) return
    if (page) report()
    if (!first) loaded = ''
    last = where
    page = l.href
    shown = 0; since = d.visibilityState === 'visible' ? now() : 0
    maxScroll = 0; sentScroll = 0
    if (secObs) secObs.disconnect()
    secOrder = []; secSeen = {}; secSent = 0; secCount = 0
    setTimeout(function () { onScroll(); watchSections() }, 300)
    var r = first ? d.referrer || null : null, was = first
    if (device && first) aboutMore(about(), function (x) { send('pageview', { r: r, x: x }); if (missing && notFound()) send('404', { props: { from: cameFrom() } }) })
    else {
      send('pageview', { r: r })
      if (was && missing && notFound()) send('404', { props: { from: cameFrom() } })
    }
    first = false
  }
  // a page that does not exist: the server said 404 (or the page says so with data-pocket-404)
  function notFound() {
    try {
      var nav = performance.getEntriesByType('navigation')[0]
      if (nav && (nav.responseStatus === 404 || nav.responseStatus === 410)) return true
    } catch (e) {}
    return !!d.querySelector('[data-pocket-404]')
  }
  // the page with the link that led here: a path of this site, or another site's page
  function cameFrom() {
    try {
      var u = new URL(d.referrer)
      return (u.host === l.host ? '' : u.host) + u.pathname
    } catch (e) { return '' }
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
    if (outbound && /^(mailto|tel):/i.test(href)) {
      var to = href.replace(/^\w+:/, '').split('?')[0]
      try { to = decodeURIComponent(to) } catch (x) {}
      to = to.trim()
      if (to) send('Contact Link: Click', { props: { kind: /^m/i.test(href) ? 'email' : 'phone', to: to } })
    } else if (href) {
      var u
      try { u = new URL(href, l.href) } catch (x) { u = null }
      if (u && /^https?:$/.test(u.protocol)) {
        var ext = (u.pathname.split('.').pop() || '').toLowerCase()
        // a link marked download is a file whatever its ending
        if (downloads && (el.hasAttribute('download') || (u.pathname.indexOf('.') > 0 && exts.indexOf(ext) >= 0))) send('File Download', { props: { url: u.origin + u.pathname } })
        else if (outbound && u.host !== l.host) send('Outbound Link: Click', { props: { url: u.origin + u.pathname } })
      }
    }
    if (clicks && !t) send('Click', { props: { text: text(el) || '(' + el.tagName.toLowerCase() + ')' } })
  }
  d.addEventListener('click', onClick, true)
  d.addEventListener('auxclick', onClick, true)

  // ---- forms people send: which form, never what was typed ----
  if (forms) d.addEventListener('submit', function (e) {
    var f = e.target
    if (!f || f.tagName !== 'FORM' || f.hasAttribute('data-pocket-ignore')) return
    // a search box is not a form sent: its words show under searches
    if (f.getAttribute('role') === 'search' || f.querySelector('input[type="search"]')) return
    var where = ''
    try { where = new URL(f.getAttribute('action') || l.href, l.href).pathname } catch (x) {}
    var name = f.getAttribute('data-pocket-form') || f.getAttribute('aria-label') || f.id || f.getAttribute('name') || where || '(form)'
    send('Form: Submission', { props: { form: name.length > 80 ? name.slice(0, 79) + '…' : name } })
  }, true)

  // ---- the site's own events: pocket('Signed up', { props: { plan: 'pro' } }),
  // with money: pocket('Order placed', { revenue: { amount: 24.5, currency: 'EUR' } }) ----
  function api_(name, o) { o = o || {}; send(name, { props: o.props, u: o.u, rev: o.revenue }, o.callback) }
  var queued = (w.pocket && w.pocket.q) || []
  w.pocket = api_
  if (!w.plausible) w.plausible = api_
  for (var i = 0; i < queued.length; i++) api_.apply(this, queued[i])

  if (d.visibilityState === 'prerender') d.addEventListener('visibilitychange', function f() { if (d.visibilityState === 'visible') { d.removeEventListener('visibilitychange', f); pageview() } })
  else pageview()
})()
