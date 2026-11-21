package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-analytics/internal/store"
)

// ---- Pocket AI: the numbers in plain words ----

func (s *Server) withAI(w http.ResponseWriter, r *http.Request) (context.Context, func(), bool) {
	select {
	case s.aiSem <- struct{}{}:
	default:
		writeErr(w, http.StatusTooManyRequests, "AI is busy with other work; try again in a moment")
		return nil, nil, false
	}
	ctx, cancel := context.WithTimeout(r.Context(), 150*time.Second)
	return ctx, func() { cancel(); <-s.aiSem }, true
}

// digest is a site's range as text for the AI: totals against the period
// before, and the top of each list.
func (s *Server) digest(ctx context.Context, site *store.Site, q store.Query, sp span) string {
	var b strings.Builder
	t, _ := s.store.Totals(ctx, q)
	pq := q
	pq.From, pq.To = sp.PFrom.Unix(), sp.PTo.Unix()
	p, _ := s.store.Totals(ctx, pq)
	fmt.Fprintf(&b, "Website: %s\nPeriod: %s (%s to %s)\n", site.Label(), sp.Label, sp.FromDay, sp.ToDay)
	if len(q.Filters) > 0 {
		var fs []string
		for _, f := range q.Filters {
			op := "is"
			if f.Not {
				op = "is not"
			}
			fs = append(fs, f.Dim+" "+op+" "+f.Value)
		}
		fmt.Fprintf(&b, "Only visits where: %s\n", strings.Join(fs, ", "))
	}
	fmt.Fprintf(&b, "Visitors %d (before: %d), visits %d (%d), pages seen %d (%d), pages per visit %.1f (%.1f), bounce rate %.0f%% (%.0f%%), average visit %s (%s), events %d (%d)\n",
		t.Visitors, p.Visitors, t.Visits, p.Visits, t.Pageviews, p.Pageviews, t.ViewsPerVisit, p.ViewsPerVisit, t.BounceRate, p.BounceRate, fmtDur(t.Duration), fmtDur(p.Duration), t.Events, p.Events)
	list := func(title, dim string, n int) {
		rows, _ := s.store.Breakdown(ctx, q, dim, "", "", n, 0)
		prows, _ := s.store.Breakdown(ctx, pq, dim, "", "", 50, 0)
		before := map[string]int{}
		for _, r := range prows {
			before[r.Value] = r.Visitors
		}
		if len(rows) == 0 {
			return
		}
		fmt.Fprintf(&b, "\n%s:\n", title)
		for _, r := range rows {
			extra := ""
			if dim == "page" {
				extra = fmt.Sprintf(", %d views, %s on the page, read %.0f%% down", r.Pageviews, fmtDur(r.TimeOnPage), r.Scroll)
			} else if r.Visits > 0 {
				extra = fmt.Sprintf(", bounce %.0f%%, visit %s", r.BounceRate, fmtDur(r.Duration))
			}
			fmt.Fprintf(&b, "- %s: %d visitors (before %d)%s\n", r.Value, r.Visitors, before[r.Value], extra)
		}
	}
	list("Channels", "channel", 8)
	list("Sources", "source", 10)
	list("Campaigns", "utm_campaign", 5)
	list("Pages", "page", 10)
	list("Entry pages", "entry", 5)
	list("Countries", "country", 8)
	list("Devices", "device", 4)
	list("Browsers", "browser", 5)
	list("Events", "event", 8)
	list("Links to other sites", "outbound", 5)
	if kw := s.keywords(ctx, site, q, sp, "query", 8); kw.State == "ok" && len(kw.Rows) > 0 {
		fmt.Fprintf(&b, "\nWords people searched on Google before they came (%s to %s, from Google Search Console):\n", kw.From, kw.To)
		for _, r := range kw.Rows {
			fmt.Fprintf(&b, "- %s: %.0f clicks, shown %.0f times, place %.1f in the results\n", r.Words, r.Clicks, r.Impressions, r.Position)
		}
	}
	if robots, total, err := s.store.Robots(ctx, store.RobotQuery{SiteID: site.ID, From: q.From, To: q.To, By: "name", Limit: 8}); err == nil && total > 0 {
		b.WriteString("\nRobots that read the site (never counted as visitors; kind ai = AI assistants, search = search engines, preview = someone shared a link):\n")
		for _, r := range robots {
			fmt.Fprintf(&b, "- %s (%s): read %d pages, %d different\n", r.Value, r.Kind, r.Hits, r.Pages)
		}
	}
	list("Searches on the site", "search", 6)
	list("Forms sent", "form", 5)
	list("Email and phone links pressed", "contact", 4)
	list("Pages that do not exist (someone followed a broken link)", "notfound", 5)
	if all, _, err := s.store.Speed(ctx, q, "page", 1); err == nil && all.Loads >= 10 {
		b.WriteString("\nSpeed (what three of four page loads stayed under):")
		if all.LCP != nil {
			fmt.Fprintf(&b, " the main content showed in %.1f s (good: up to 2.5 s);", all.LCP.P75/1000)
		}
		if all.INP != nil {
			fmt.Fprintf(&b, " a press showed a result in %.0f ms (good: up to 200 ms);", all.INP.P75)
		}
		if all.CLS != nil {
			fmt.Fprintf(&b, " the layout moved %.2f (good: up to 0.1);", all.CLS.P75)
		}
		if all.TTFB != nil {
			fmt.Fprintf(&b, " the server answered in %.0f ms (good: up to 800 ms);", all.TTFB.P75)
		}
		fmt.Fprintf(&b, " %d page loads measured\n", all.Loads)
	}
	goals, _ := s.store.ListGoals(ctx, site.UserID, site.ID)
	if conv, _ := s.store.Conversions(ctx, q, goals, t.Visitors); len(conv) > 0 {
		b.WriteString("\nGoals:\n")
		for _, c := range conv {
			money := ""
			for _, m := range c.Revenue {
				money += fmt.Sprintf(", brought %.2f %s", m.Amount, m.Currency)
			}
			if c.Robot {
				fmt.Fprintf(&b, "- %s: read %d times by robots (not visitors, e.g. an install script run)\n", c.Goal.Name, c.Conversions)
				continue
			}
			fmt.Fprintf(&b, "- %s: %d visitors (%.1f%%), %d times%s\n", c.Goal.Name, c.Visitors, c.Rate, c.Conversions, money)
		}
	}
	return b.String()
}

const explainSystem = `You explain website statistics to the site's owner, who is not an analyst. Use plain, friendly words, short sentences, no jargon (say "people who left after one page", not "bounce rate").
Write Markdown with these parts:
1. One or two sentences: how the period went against the one before.
2. "What stood out": 3 to 5 bullets with concrete numbers (sources, pages, countries, devices, goals).
3. "Worth trying": 2 or 3 small, practical suggestions that follow from the numbers.
Only use the numbers given. When there is too little data (under about 20 visitors), say so and keep it short. Don't invent causes; say "maybe" when guessing. Under 220 words.`

// handleExplain asks the AI to explain a site's range (cached by the numbers).
func (s *Server) handleExplain(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	uid := site.UserID
	st, _ := s.store.GetSettings(r.Context(), uid)
	if !st.AIHelp {
		writeErr(w, http.StatusBadRequest, "Pocket AI help is switched off in Settings → AI")
		return
	}
	q, sp := s.queryFor(r, site)
	text := s.digest(r.Context(), site, q, sp)
	sum := sha256.Sum256([]byte(text + st.AIProvider + st.AIPocketModel + st.OpenAIModel))
	key := "explain:" + hex.EncodeToString(sum[:12])
	if v, at, ok := s.store.CacheGet(r.Context(), key); ok && !queryBool(r, "fresh") {
		writeJSON(w, http.StatusOK, map[string]any{"text": v, "at": at, "cached": true})
		return
	}
	cl, _, why := s.aiFor(r.Context(), uid, st)
	if cl == nil {
		writeErr(w, http.StatusBadRequest, firstNonEmpty(why, "no AI is set up: connect Pocket Core (Pocket AI) or add an OpenAI key in Settings → AI"))
		return
	}
	ctx, done, ok := s.withAI(w, r)
	if !ok {
		return
	}
	defer done()
	out, err := cl.Complete(ctx, explainSystem, text, false)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AI: "+err.Error())
		return
	}
	out = strings.TrimSpace(out)
	_ = s.store.CachePut(r.Context(), key, out)
	writeJSON(w, http.StatusOK, map[string]any{"text": out, "at": store.Now()})
}

// ---- keeping it elsewhere: a note, a task ----

// handleMake saves text (an explanation, a report) as a note or makes a task.
func (s *Server) handleMake(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	var in struct {
		Target string `json:"target"` // note | task
		Title  string `json:"title"`
		Text   string `json:"text"`
		Due    string `json:"due"`
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.Title) == "" {
		writeErr(w, http.StatusBadRequest, "a title is needed")
		return
	}
	if in.Due != "" && !validDate(in.Due) {
		writeErr(w, http.StatusBadRequest, "the due day is YYYY-MM-DD")
		return
	}
	ctx := r.Context()
	c := s.clients(ctx, site.UserID)
	link := fmt.Sprintf("%s/sites/%d", s.base(), site.ID)
	var app pocketApp
	var res map[string]any
	var path string
	var err error
	switch in.Target {
	case "note":
		app = appNotes
		var sib *siblingConn
		if sib, err = c.conn(ctx, appNotes, scopeNotesWrite); err == nil {
			res, err = postJSONTo(ctx, sib, "/api/v1/notes", map[string]any{"title": clip(in.Title, 200), "content": clip(in.Text, 50000) + "\n\n— Pocket Analytics, " + site.Label() + " (" + link + ")"})
			path = "/note/" + idOf(res)
		}
	case "task":
		app = appTasks
		var sib *siblingConn
		if sib, err = c.conn(ctx, appTasks, scopeTasksWrite); err == nil {
			body := map[string]any{"title": clip(in.Title, 200), "notes": clip(in.Text, 20000), "url": link}
			if in.Due != "" {
				body["due"] = in.Due
			}
			res, err = postJSONTo(ctx, sib, "/api/v1/tasks", body)
			path = "/today?task=" + idOf(res)
		}
	default:
		writeErr(w, http.StatusBadRequest, "make a note or a task")
		return
	}
	if err != nil {
		if isNotConnected(err) {
			writeErr(w, http.StatusConflict, err.Error())
			return
		}
		writeErr(w, http.StatusBadGateway, app.name+": "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "app": app.name, "url": s.siblingLink(ctx, site.UserID, app, path)})
}

// handleApps says, per sibling app, whether it can be used.
func (s *Server) handleApps(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	out := map[string]any{}
	for _, a := range pocketApps {
		_, via, detail := s.connStatus(ctx, userFrom(ctx), st, a)
		out[a.slug] = map[string]any{"ready": via != "", "detail": detail, "name": a.name}
	}
	writeJSON(w, http.StatusOK, out)
}
