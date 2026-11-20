package server

import (
	"context"
	"fmt"
	"html"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-analytics/internal/notify"
	"github.com/abjelosevic88/pocket-homelab-analytics/internal/store"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
)

// ---- telling the owner: the weekly report, spikes, quiet sites ----

// sendMessage goes to the person's message URL (Apprise, ntfy or a webhook)
// and, through Pocket Core, to their phones with Pocket Hub.
func (s *Server) sendMessage(ctx context.Context, uid int64, st store.Settings, title, body, link, prio string) bool {
	sent := false
	if st.NotifyURL != "" {
		n := notify.New(st.NotifyURL, st.NotifyFormat)
		if err := n.Send(ctx, notify.Message{Title: title, Body: body, Priority: prio, Tags: []string{"chart_with_upwards_trend"}, Link: link}); err != nil {
			s.log.Warn("notify failed", "user", uid, "err", err)
		} else {
			sent = true
		}
	}
	if st.PushPhone && s.core.Configured() {
		if u, err := s.store.GetUser(ctx, uid); err == nil {
			res, err := s.core.Notify(ctx, u.Email, core.PushMessage{Title: title, Body: body, URL: link, Source: "analytics", Thread: "analytics"})
			if err == nil && res != nil && res.Sent > 0 {
				sent = true
			} else if err != nil {
				s.log.Debug("push", "user", uid, "err", err)
			}
		}
	}
	return sent
}

// once is true the first time it is asked for a key.
func (s *Server) once(ctx context.Context, key string) bool {
	if _, _, done := s.store.CacheGet(ctx, key); done {
		return false
	}
	_ = s.store.CachePut(ctx, key, "1")
	return true
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// change is "+12%" / "−8%" / "" between two numbers.
func change(now, before float64) string {
	if before <= 0 {
		return ""
	}
	p := math.Round((now - before) * 100 / before)
	switch {
	case p > 0:
		return fmt.Sprintf("+%.0f%%", p)
	case p < 0:
		return fmt.Sprintf("−%.0f%%", -p)
	}
	return "same as before"
}

func fmtDur(sec float64) string {
	d := int(math.Round(sec))
	if d < 60 {
		return fmt.Sprintf("%ds", d)
	}
	return fmt.Sprintf("%dm %02ds", d/60, d%60)
}

// ---- the weekly report ----

// reports runs every minute; on Monday at each person's hour it sends last
// week's report.
func (s *Server) reports(ctx context.Context) {
	ids, err := s.store.SettingsUsers(ctx)
	if err != nil {
		return
	}
	for _, uid := range ids {
		st, err := s.store.GetSettings(ctx, uid)
		if err != nil || (!st.WeeklyReport && !st.ReportEmail) {
			continue
		}
		now := time.Now().In(zone(st))
		if now.Weekday() != time.Monday || now.Hour() != st.NotifyHour || now.Minute() > 10 {
			continue
		}
		if !s.once(ctx, fmt.Sprintf("weekly:%d:%s", uid, now.Format("2006-01-02"))) {
			continue
		}
		s.sendWeekly(ctx, uid, st, now)
	}
}

// siteWeek is one site's last seven days against the seven before.
type siteWeek struct {
	Site        *store.Site
	Now, Before store.Totals
	Sources     []store.Row
	Pages       []store.Row
	Countries   []store.Row
}

// week counts each of the person's sites for the seven days before `now`'s day.
func (s *Server) week(ctx context.Context, uid int64, now time.Time) []siteWeek {
	sites, _ := s.store.ListSites(ctx, uid)
	var out []siteWeek
	for _, site := range sites {
		if !site.Options.Weekly {
			continue
		}
		loc := s.zoneOf(ctx, site)
		end := dayStart(now.In(loc))
		q := store.Query{SiteID: site.ID, From: end.AddDate(0, 0, -7).Unix(), To: end.Unix()}
		w := siteWeek{Site: site, Now: s.periodTotals(ctx, site, end.AddDate(0, 0, -7), end), Before: s.periodTotals(ctx, site, end.AddDate(0, 0, -14), end.AddDate(0, 0, -7))}
		w.Sources, _ = s.store.Breakdown(ctx, q, "source", "", "", 3, 0)
		w.Pages, _ = s.store.Breakdown(ctx, q, "page", "", "", 3, 0)
		w.Countries, _ = s.store.Breakdown(ctx, q, "country", "", "", 3, 0)
		out = append(out, w)
	}
	return out
}

func names(rows []store.Row) string {
	var p []string
	for _, r := range rows {
		p = append(p, fmt.Sprintf("%s (%d)", r.Value, r.Visitors))
	}
	return strings.Join(p, ", ")
}

// weeklyText is the report in plain words.
func weeklyText(weeks []siteWeek) string {
	var b strings.Builder
	for i, w := range weeks {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "%s: %d %s", w.Site.Label(), w.Now.Visitors, plural(w.Now.Visitors, "visitor", "visitors"))
		if c := change(float64(w.Now.Visitors), float64(w.Before.Visitors)); c != "" {
			fmt.Fprintf(&b, " (%s)", c)
		}
		fmt.Fprintf(&b, ", %d pages seen", w.Now.Pageviews)
		if w.Now.Visits > 0 {
			fmt.Fprintf(&b, ", visits %s on average", fmtDur(w.Now.Duration))
		}
		b.WriteString(".")
		if len(w.Sources) > 0 {
			b.WriteString("\nFrom: " + names(w.Sources) + ".")
		}
		if len(w.Pages) > 0 {
			b.WriteString("\nTop pages: " + names(w.Pages) + ".")
		}
		if len(w.Countries) > 0 {
			b.WriteString("\nCountries: " + names(w.Countries) + ".")
		}
	}
	return b.String()
}

// sendWeekly sends the report as a message and, when asked, by email.
func (s *Server) sendWeekly(ctx context.Context, uid int64, st store.Settings, now time.Time) (string, bool, error) {
	weeks := s.week(ctx, uid, now)
	if len(weeks) == 0 {
		return "", false, nil
	}
	text := weeklyText(weeks)
	sent := false
	if st.WeeklyReport {
		sent = s.sendMessage(ctx, uid, st, "Your websites last week", text, s.base()+"/", "low")
	}
	var mailErr error
	if st.ReportEmail {
		if mailErr = s.mailReport(ctx, uid, st, weeks, text); mailErr == nil {
			sent = true
		} else {
			s.log.Warn("weekly report email failed", "user", uid, "err", mailErr)
		}
	}
	total := 0
	for _, w := range weeks {
		total += w.Now.Visitors
	}
	s.publish(ctx, uid, "analytics.weekly", "Your websites last week", map[string]any{"text": text, "visitors": total, "sites": len(weeks)})
	return text, sent, mailErr
}

// mailReport emails the report through the person's Pocket Mail.
func (s *Server) mailReport(ctx context.Context, uid int64, st store.Settings, weeks []siteWeek, text string) error {
	if s.cfg.DemoData {
		return nil
	}
	u, err := s.store.GetUser(ctx, uid)
	if err != nil {
		return err
	}
	c := s.clients(ctx, uid)
	sib, err := c.conn(ctx, appMail, scopeMailSend)
	if err != nil {
		return err
	}
	var h strings.Builder
	esc := html.EscapeString
	h.WriteString(`<!doctype html><html><body style="margin:0;padding:24px 12px;background:#f5f6f8;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;color:#1f2433">`)
	h.WriteString(`<div style="max-width:560px;margin:0 auto;background:#fff;border:1px solid #e3e6ee;border-top:5px solid #7048e8;border-radius:12px;padding:24px">`)
	h.WriteString(`<h2 style="margin:0 0 16px;font-size:18px">Your websites last week</h2>`)
	for _, w := range weeks {
		h.WriteString(`<h3 style="margin:18px 0 6px;font-size:15px">` + esc(w.Site.Label()) + `</h3>`)
		h.WriteString(`<table role="presentation" style="width:100%;border-collapse:collapse;font-size:14px">`)
		row := func(k, v string) {
			h.WriteString(`<tr><td style="padding:6px 0;border-top:1px solid #eef0f4;color:#646b7d;width:38%">` + esc(k) + `</td><td style="padding:6px 0 6px 10px;border-top:1px solid #eef0f4">` + esc(v) + `</td></tr>`)
		}
		row("Visitors", fmt.Sprintf("%d %s", w.Now.Visitors, change(float64(w.Now.Visitors), float64(w.Before.Visitors))))
		row("Pages seen", fmt.Sprint(w.Now.Pageviews))
		row("Average visit", fmtDur(w.Now.Duration))
		if len(w.Sources) > 0 {
			row("Came from", names(w.Sources))
		}
		if len(w.Pages) > 0 {
			row("Top pages", names(w.Pages))
		}
		if len(w.Countries) > 0 {
			row("Countries", names(w.Countries))
		}
		h.WriteString(`</table>`)
	}
	if b := s.base(); b != "" {
		h.WriteString(`<p style="margin:20px 0 0"><a href="` + esc(b) + `/" style="display:inline-block;background:#7048e8;color:#fff;text-decoration:none;padding:10px 18px;border-radius:8px;font-weight:600">Open Pocket Analytics</a></p>`)
	}
	h.WriteString(`</div></body></html>`)
	to := firstNonEmpty(st.ReportTo, u.Email)
	body := map[string]any{"to": []map[string]string{{"name": u.Name, "addr": to}}, "subject": "Your websites last week", "body_text": text, "body_html": h.String()}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var out map[string]any
	return sib.client.Do(ctx, http.MethodPost, "/api/v1/send", nil, body, &out)
}

// handleReportNow sends the weekly report now (Settings → Notifications).
func (s *Server) handleReportNow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	st, _ := s.store.GetSettings(ctx, uid)
	if !st.WeeklyReport && !st.ReportEmail {
		st.WeeklyReport = true
	}
	text, sent, err := s.sendWeekly(ctx, uid, st, time.Now().In(zone(st)))
	out := map[string]any{"sent": sent, "text": text}
	if err != nil {
		out["error"] = err.Error()
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- spikes and quiet sites ----

// alerts runs every ten minutes: a site with far more visitors in the last
// hour than usual, or one that usually has visitors and had none for a day.
func (s *Server) alerts(ctx context.Context) {
	sites, err := s.store.AllSites(ctx)
	if err != nil {
		return
	}
	now := time.Now()
	for _, site := range sites {
		if site.FirstHit == 0 || now.Unix()-site.FirstHit < 7*86400 {
			continue
		}
		st, _ := s.store.GetSettings(ctx, site.UserID)
		hour := s.store.VisitorsIn(ctx, site.ID, now.Add(-time.Hour).Unix(), now.Unix())
		twoWeeks := s.store.CountVisitsSince(ctx, site.ID, now.AddDate(0, 0, -14).Unix())
		perHour := float64(twoWeeks) / (14 * 24)
		if site.Options.Spike && hour >= 20 && float64(hour) >= 5*perHour {
			if s.once(ctx, fmt.Sprintf("spike:%d:%s", site.ID, now.Format("2006-01-02"))) {
				q := store.Query{SiteID: site.ID, From: now.Add(-time.Hour).Unix(), To: now.Unix() + 1}
				src, _ := s.store.Breakdown(ctx, q, "source", "", "", 2, 0)
				pages, _ := s.store.Breakdown(ctx, q, "page", "", "", 2, 0)
				body := fmt.Sprintf("%d visitors in the last hour on %s — usually about %.0f an hour.", hour, site.Label(), math.Max(perHour, 1))
				if len(src) > 0 {
					body += " Most came from " + names(src) + "."
				}
				if len(pages) > 0 {
					body += " Reading: " + names(pages) + "."
				}
				s.sendMessage(ctx, site.UserID, st, "More visitors than usual: "+site.Label(), body, fmt.Sprintf("%s/sites/%d/live", s.base(), site.ID), "default")
				s.publish(ctx, site.UserID, "analytics.spike", site.Label(), map[string]any{"site": site.Domain, "site_id": site.ID, "visitors_hour": hour, "usual_hour": math.Round(perHour*10) / 10, "text": body})
			}
		}
		if site.Options.Quiet && twoWeeks >= 14*5 && now.Unix()-site.LastHit > 24*3600 {
			if s.once(ctx, fmt.Sprintf("quiet:%d:%d", site.ID, site.LastHit)) {
				body := fmt.Sprintf("Nobody was counted on %s for a day; it usually has about %d visits a day. The site may be down, or the script may be missing from its pages.", site.Label(), twoWeeks/14)
				s.sendMessage(ctx, site.UserID, st, "No visitors on "+site.Label(), body, fmt.Sprintf("%s/sites/%d/settings", s.base(), site.ID), "high")
				s.publish(ctx, site.UserID, "analytics.quiet", site.Label(), map[string]any{"site": site.Domain, "site_id": site.ID, "last_hit": site.LastHit, "text": body})
			}
		}
	}
}

func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	if !s.sendMessage(ctx, actorID(ctx), st, "Pocket Analytics", notifyTestBody, s.base()+"/", "info") {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "nothing could be sent: add a notification address, or connect Pocket Core and Pocket Hub"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

const notifyTestBody = "Notifications work. The weekly report about your websites, a note when far more people than usual come, and one when a site goes quiet will arrive here."
