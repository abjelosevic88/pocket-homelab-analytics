// Package refs says where a visit came from: a named source (Google,
// Hacker News, a newsletter) and its channel (Search, Social, AI…), from the
// referrer and the utm_ tags in the address.
package refs

import (
	"net/url"
	"strings"
)

// Channels, in plain words.
const (
	Direct   = "Direct"
	Search   = "Search"
	Social   = "Social"
	AI       = "AI assistants"
	Email    = "Email"
	Paid     = "Paid"
	Referral = "Other sites"
	Campaign = "Campaigns"
)

type known struct {
	name, channel string
}

// hosts maps a host (or its end) to a source; checked from the most specific.
var hosts = map[string]known{
	"google":                              {"Google", Search},
	"bing.com":                            {"Bing", Search},
	"duckduckgo.com":                      {"DuckDuckGo", Search},
	"search.yahoo.com":                    {"Yahoo", Search},
	"yahoo.com":                           {"Yahoo", Search},
	"yandex":                              {"Yandex", Search},
	"baidu.com":                           {"Baidu", Search},
	"ecosia.org":                          {"Ecosia", Search},
	"search.brave.com":                    {"Brave Search", Search},
	"startpage.com":                       {"Startpage", Search},
	"qwant.com":                           {"Qwant", Search},
	"kagi.com":                            {"Kagi", Search},
	"search.naver.com":                    {"Naver", Search},
	"seznam.cz":                           {"Seznam", Search},
	"chatgpt.com":                         {"ChatGPT", AI},
	"chat.openai.com":                     {"ChatGPT", AI},
	"perplexity.ai":                       {"Perplexity", AI},
	"claude.ai":                           {"Claude", AI},
	"gemini.google.com":                   {"Gemini", AI},
	"copilot.microsoft.com":               {"Copilot", AI},
	"you.com":                             {"You.com", AI},
	"phind.com":                           {"Phind", AI},
	"chat.mistral.ai":                     {"Le Chat", AI},
	"deepseek.com":                        {"DeepSeek", AI},
	"facebook.com":                        {"Facebook", Social},
	"fb.me":                               {"Facebook", Social},
	"m.facebook.com":                      {"Facebook", Social},
	"l.facebook.com":                      {"Facebook", Social},
	"lm.facebook.com":                     {"Facebook", Social},
	"instagram.com":                       {"Instagram", Social},
	"l.instagram.com":                     {"Instagram", Social},
	"t.co":                                {"X (Twitter)", Social},
	"twitter.com":                         {"X (Twitter)", Social},
	"x.com":                               {"X (Twitter)", Social},
	"linkedin.com":                        {"LinkedIn", Social},
	"lnkd.in":                             {"LinkedIn", Social},
	"reddit.com":                          {"Reddit", Social},
	"out.reddit.com":                      {"Reddit", Social},
	"news.ycombinator.com":                {"Hacker News", Social},
	"lobste.rs":                           {"Lobsters", Social},
	"youtube.com":                         {"YouTube", Social},
	"youtu.be":                            {"YouTube", Social},
	"tiktok.com":                          {"TikTok", Social},
	"pinterest.com":                       {"Pinterest", Social},
	"pin.it":                              {"Pinterest", Social},
	"threads.net":                         {"Threads", Social},
	"threads.com":                         {"Threads", Social},
	"bsky.app":                            {"Bluesky", Social},
	"mastodon.social":                     {"Mastodon", Social},
	"vk.com":                              {"VK", Social},
	"telegram.org":                        {"Telegram", Social},
	"t.me":                                {"Telegram", Social},
	"web.whatsapp.com":                    {"WhatsApp", Social},
	"discord.com":                         {"Discord", Social},
	"slack.com":                           {"Slack", Social},
	"producthunt.com":                     {"Product Hunt", Social},
	"github.com":                          {"GitHub", Referral},
	"stackoverflow.com":                   {"Stack Overflow", Referral},
	"medium.com":                          {"Medium", Referral},
	"substack.com":                        {"Substack", Referral},
	"wikipedia.org":                       {"Wikipedia", Referral},
	"mail.google.com":                     {"Gmail", Email},
	"outlook.live.com":                    {"Outlook", Email},
	"outlook.office.com":                  {"Outlook", Email},
	"mail.yahoo.com":                      {"Yahoo Mail", Email},
	"mail.proton.me":                      {"Proton Mail", Email},
	"app.fastmail.com":                    {"Fastmail", Email},
	"android-app://com.google.android.gm": {"Gmail", Email},
}

// words are how utm_source / ref often names a place, without a domain.
var words = map[string]string{"google": "google.com", "bing": "bing.com", "duckduckgo": "duckduckgo.com", "ddg": "duckduckgo.com",
	"facebook": "facebook.com", "fb": "facebook.com", "instagram": "instagram.com", "ig": "instagram.com", "twitter": "x.com", "x": "x.com",
	"linkedin": "linkedin.com", "reddit": "reddit.com", "hn": "news.ycombinator.com", "hackernews": "news.ycombinator.com",
	"youtube": "youtube.com", "tiktok": "tiktok.com", "pinterest": "pinterest.com", "threads": "threads.net", "bluesky": "bsky.app",
	"mastodon": "mastodon.social", "producthunt": "producthunt.com", "github": "github.com", "chatgpt": "chatgpt.com", "openai": "chatgpt.com",
	"chatgpt.com": "chatgpt.com", "perplexity": "perplexity.ai", "claude": "claude.ai", "gemini": "gemini.google.com", "copilot": "copilot.microsoft.com",
	"telegram": "t.me", "whatsapp": "web.whatsapp.com", "discord": "discord.com", "slack": "slack.com", "substack": "substack.com", "medium": "medium.com"}

// Lookup names a referrer host, "" when it is not a known one.
func Lookup(host string) (name, channel string) {
	host = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(host)), "www.")
	if h, ok := words[strings.ReplaceAll(host, " ", "")]; ok {
		host = h
	}
	if k, ok := hosts[host]; ok {
		return k.name, k.channel
	}
	// the end of the host: news.ycombinator.com, de.search.yahoo.com, old.reddit.com
	parts := strings.Split(host, ".")
	for i := 1; i < len(parts)-1; i++ {
		if k, ok := hosts[strings.Join(parts[i:], ".")]; ok {
			return k.name, k.channel
		}
	}
	// Google and Yandex live under many country domains
	for _, p := range parts {
		if k, ok := hosts[p]; ok && (p == "google" || p == "yandex") {
			if p == "google" && parts[0] == "mail" {
				return "Gmail", Email
			}
			return k.name, k.channel
		}
	}
	if strings.Contains(host, "mastodon") || strings.HasSuffix(host, ".social") {
		return "Mastodon", Social
	}
	return "", ""
}

// Source is where a visit came from.
type Source struct {
	Referrer                          string // host + path, no query
	Name                              string
	Channel                           string
	UTMSource, UTMMedium, UTMCampaign string
	UTMContent, UTMTerm               string
}

var paidMediums = map[string]bool{"cpc": true, "ppc": true, "paid": true, "paidsearch": true, "paid_social": true, "paidsocial": true, "display": true, "banner": true, "cpm": true, "ads": true, "ad": true}

// Classify works out a visit's source from the page address, the referrer
// and the site's own host names (a referrer from the site itself is no source).
func Classify(page *url.URL, referrer string, own func(host string) bool) Source {
	var s Source
	q := url.Values{}
	if page != nil {
		q = page.Query()
	}
	s.UTMSource = clip(firstNonEmpty(q.Get("utm_source"), q.Get("ref"), q.Get("source")), 120)
	s.UTMMedium = clip(strings.ToLower(q.Get("utm_medium")), 120)
	s.UTMCampaign = clip(q.Get("utm_campaign"), 160)
	s.UTMContent = clip(q.Get("utm_content"), 160)
	s.UTMTerm = clip(q.Get("utm_term"), 160)
	var refHost string
	if r, err := url.Parse(referrer); err == nil && r.Host != "" && (r.Scheme == "http" || r.Scheme == "https") {
		refHost = strings.TrimPrefix(strings.ToLower(r.Hostname()), "www.")
		if own != nil && own(r.Hostname()) {
			refHost = ""
		} else {
			s.Referrer = clip(refHost+strings.TrimRight(r.EscapedPath(), "/"), 300)
		}
	} else if strings.HasPrefix(referrer, "android-app://") {
		refHost = referrer
		s.Referrer = clip(referrer, 300)
	}
	if refHost != "" {
		s.Name, s.Channel = Lookup(refHost)
		if s.Name == "" {
			s.Name, s.Channel = refHost, Referral
		}
	}
	if s.UTMSource != "" {
		if n, c := Lookup(s.UTMSource); n != "" {
			s.Name = n
			if s.Channel == "" || s.Channel == Referral {
				s.Channel = c
			}
		} else if s.Name == "" {
			s.Name = s.UTMSource
		}
	}
	switch {
	case paidMediums[s.UTMMedium] || q.Get("gclid") != "" || q.Get("msclkid") != "" || q.Get("fbclid") != "" && s.UTMMedium != "":
		s.Channel = Paid
	case s.UTMMedium == "email" || s.UTMMedium == "newsletter" || strings.Contains(strings.ToLower(s.UTMSource), "newsletter"):
		s.Channel = Email
	case s.UTMMedium == "social" || s.UTMMedium == "social-media":
		if s.Channel == "" || s.Channel == Referral {
			s.Channel = Social
		}
	}
	if s.Name == "" && q.Get("gclid") != "" {
		s.Name = "Google"
	}
	if s.Channel == "" && s.Name != "" {
		s.Channel = Campaign
	}
	if s.Name == "" {
		s.Name, s.Channel = Direct, Direct
	}
	return s
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return ""
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
