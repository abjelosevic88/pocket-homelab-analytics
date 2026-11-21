package store

import (
	"encoding/json"
)

// User is an account on this box.
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"` // admin | member
	CreatedAt    string `json:"created_at"`
}

// APIToken is a personal access token (hash stored, plaintext shown once).
type APIToken struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// SiteOptions are what a site counts and how.
type SiteOptions struct {
	Outbound     bool     `json:"outbound"`      // clicks on links to other sites, and on email and phone links
	Downloads    bool     `json:"downloads"`     // clicks on files (pdf, zip, …)
	Clicks       bool     `json:"clicks"`        // clicks on any link or button (what people press)
	Forms        bool     `json:"forms"`         // forms people send (never what they typed)
	NotFound     bool     `json:"not_found"`     // pages that do not exist, and where the link to them was
	Search       bool     `json:"search"`        // what people search for on the site (?q=…)
	SearchParams []string `json:"search_params"` // the names in the address that hold the search words
	Speed        bool     `json:"speed"`         // how fast pages show and react
	Hash         bool     `json:"hash"`          // pages are told apart by #… (single-page apps)
	Localhost    bool     `json:"localhost"`     // count hits from localhost (testing)
	Exclude      []string `json:"exclude"`       // paths not counted (* = any part)
	IgnoreIPs    []string `json:"ignore_ips"`    // addresses not counted (an IP or a range like 203.0.113.0/24): your own
	KeepDays     int      `json:"keep_days"`     // visits older than this are removed (0 = keep)
	Spike        bool     `json:"spike"`         // tell me when far more people than usual come
	Quiet        bool     `json:"quiet"`         // tell me when nobody came for a day (tracking broken?)
	Weekly       bool     `json:"weekly"`        // in the weekly report
}

// DefaultSearchParams are the usual names of a site search in the address.
var DefaultSearchParams = []string{"q", "s", "search", "query"}

// DefaultOptions are what a new site starts with (and what a site saved
// before an option existed gets for it).
func DefaultOptions() SiteOptions {
	return SiteOptions{Outbound: true, Downloads: true, Forms: true, NotFound: true, Search: true, Speed: true,
		SearchParams: append([]string{}, DefaultSearchParams...), Spike: true, Quiet: true, Weekly: true, Exclude: []string{}, IgnoreIPs: []string{}}
}

// Site is one website being counted.
type Site struct {
	ID        int64       `json:"id"`
	UserID    int64       `json:"-"`
	Domain    string      `json:"domain"`
	Name      string      `json:"name"`
	TZ        string      `json:"tz"`
	Also      []string    `json:"also"`
	Options   SiteOptions `json:"options"`
	ProxyKey  string      `json:"proxy_key"`
	Position  int         `json:"position"`
	FirstHit  int64       `json:"first_hit"`
	LastHit   int64       `json:"last_hit"`
	CreatedAt string      `json:"created_at"`
	UpdatedAt string      `json:"updated_at"`
}

// Label is the site's name, else its domain.
func (s *Site) Label() string {
	if s.Name != "" {
		return s.Name
	}
	return s.Domain
}

// Goal is something a visit can do that counts.
type Goal struct {
	ID        int64  `json:"id"`
	SiteID    int64  `json:"site_id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"` // page | event | robot (pages read by robots, e.g. install.sh by curl)
	Match     string `json:"match"`
	Robot     string `json:"robot"` // robot goals: only this robot ("" = any)
	CreatedAt string `json:"created_at"`
}

// Step is one step of a funnel.
type Step struct {
	Kind  string `json:"kind"` // page | event | robot (only last: not tied to a visit)
	Match string `json:"match"`
	Robot string `json:"robot,omitempty"`
	Label string `json:"label,omitempty"`
}

// Funnel is steps a visit should take in order.
type Funnel struct {
	ID        int64  `json:"id"`
	SiteID    int64  `json:"site_id"`
	Name      string `json:"name"`
	Steps     []Step `json:"steps"`
	CreatedAt string `json:"created_at"`
}

// Visit is one visit (a session).
type Visit struct {
	ID             int64  `json:"id"`
	SiteID         int64  `json:"site_id"`
	Visitor        string `json:"-"`
	Started        int64  `json:"started"`
	Last           int64  `json:"last"`
	Hostname       string `json:"hostname"`
	Entry          string `json:"entry"`
	Exit           string `json:"exit"`
	Pageviews      int    `json:"pageviews"`
	Events         int    `json:"events"`
	Engaged        int64  `json:"engaged"`
	Referrer       string `json:"referrer"`
	Source         string `json:"source"`
	Channel        string `json:"channel"`
	UTMSource      string `json:"utm_source"`
	UTMMedium      string `json:"utm_medium"`
	UTMCampaign    string `json:"utm_campaign"`
	UTMContent     string `json:"utm_content"`
	UTMTerm        string `json:"utm_term"`
	Country        string `json:"country"`
	Region         string `json:"region"`
	City           string `json:"city"`
	Device         string `json:"device"`
	Browser        string `json:"browser"`
	BrowserVersion string `json:"browser_version"`
	OS             string `json:"os"`
	OSVersion      string `json:"os_version"`
	Language       string `json:"language"`
}

// Hit is a page seen or an event in a visit.
type Hit struct {
	ID      int64             `json:"id"`
	VisitID int64             `json:"visit_id"`
	TS      int64             `json:"ts"`
	Name    string            `json:"name"`
	Path    string            `json:"path"`
	Props   map[string]string `json:"props,omitempty"`
	Engaged int64             `json:"engaged"`
	Scroll  int               `json:"scroll"`
	// the parts of the page that were on screen (data-pocket-section)
	Sections []string `json:"sections,omitempty"`
	// money the event brought, when it said so
	Revenue  float64 `json:"revenue,omitempty"`
	Currency string  `json:"currency,omitempty"`
	// how fast the page was (nil = not measured)
	LCP *int64   `json:"lcp,omitempty"`
	INP *int64   `json:"inp,omitempty"`
	CLS *float64 `json:"cls,omitempty"`
}

// Vitals are how fast a page was for one visitor: milliseconds until the
// server answered (TTFB), something showed (FCP) and the main content
// showed (LCP), how long the slowest press took to show a result (INP),
// and how much the layout moved (CLS). nil = not measured.
type Vitals struct {
	TTFB, FCP, LCP, INP *int64
	CLS                 *float64
}

// Empty reports whether nothing was measured.
func (v Vitals) Empty() bool {
	return v.TTFB == nil && v.FCP == nil && v.LCP == nil && v.INP == nil && v.CLS == nil
}

// Note is what happened on a day, shown on a site's chart.
type Note struct {
	ID        int64  `json:"id"`
	SiteID    int64  `json:"site_id"`
	Day       string `json:"day"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
}

// Conn is one person's connection to a sibling Pocket app.
type Conn struct {
	Mode      string `json:"mode,omitempty"`
	URL       string `json:"url,omitempty"`
	Token     string `json:"token,omitempty"`
	PublicURL string `json:"public_url,omitempty"`
}

// Settings are the box-wide values plus one person's preferences and their
// own connections.
type Settings struct {
	Title         string `json:"title"`
	SetupComplete bool   `json:"setup_complete"`

	DateFormat string `json:"date_format"`
	TimeFormat string `json:"time_format"`
	WeekStart  int    `json:"week_start"` // 1 = Monday, 7 = Sunday

	// New sites start in this time zone ("" = the box's).
	TZ string `json:"tz"`

	// The weekly report (Monday, at the person's hour): as a message, and
	// as an email through Pocket Mail.
	WeeklyReport bool   `json:"weekly_report"`
	ReportEmail  bool   `json:"report_email"`
	ReportTo     string `json:"report_to"` // "" = the person's own address

	AIHelp bool `json:"ai_help"` // the Pocket AI buttons

	// Notifications (spikes, quiet sites, the weekly report).
	NotifyHour   int    `json:"notify_hour"`
	NotifyURL    string `json:"notify_url"`
	NotifyFormat string `json:"notify_format"` // apprise | ntfy | json
	PushPhone    bool   `json:"push_phone"`    // also to the phones with Pocket Hub (through Core)

	// Google Search Console: a service account key (the JSON file), to
	// read the words people searched on Google before they came.
	GoogleKey string `json:"google_key"`

	// Each sibling Pocket app, by slug.
	Conns map[string]Conn `json:"conns"`

	// AI: Pocket AI through Core, or an OpenAI-compatible API with the
	// person's own key.
	AIProvider    string `json:"ai_provider"` // auto | pocket | openai
	AIPocketModel string `json:"ai_pocket_model"`
	OpenAIKey     string `json:"openai_api_key"`
	OpenAIModel   string `json:"openai_model"`
	OpenAIBaseURL string `json:"openai_base_url"`
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
