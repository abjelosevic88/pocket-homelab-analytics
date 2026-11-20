# Reverse proxy

Set `BASE_URL` to the app's address and `SECURE_COOKIES=true` behind HTTPS. See [deploy/reverse-proxy/Caddyfile](../deploy/reverse-proxy/Caddyfile).

Websites on the internet must reach `/js/script.js` and `/api/event`. Either make the app public, or keep it private and give a second hostname to the same container with `PUBLIC_URL`: on that name only the script, the collector and the icon answer (everything else is a 404). A Cloudflare tunnel ingress for that hostname works the same way.

The visitor's address comes from `Cf-Connecting-Ip` (Cloudflare), else `X-Forwarded-For` when `TRUST_PROXY=true`, else the connection. A proxy you run yourself in front of a site (a Cloudflare Worker that serves the script from the site's own domain) proves itself with the site's proxy key in `X-Pocket-Proxy-Key` and then sends `X-Pocket-Client-IP`, `X-Pocket-Country`, `X-Pocket-Region` and `X-Pocket-City`; the app shows the exact Worker code under a site's Install → Cloudflare.
