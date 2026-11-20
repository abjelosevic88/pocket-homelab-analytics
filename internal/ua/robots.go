package ua

import (
	"regexp"
	"strings"
)

// The kinds of robots a site's owner wants to tell apart.
const (
	RobotAI      = "ai"      // AI assistants: learning from the site, or fetching a page for someone's question
	RobotSearch  = "search"  // search engines
	RobotPreview = "preview" // someone pasted a link: the app fetches the page to show a preview
	RobotSEO     = "seo"     // SEO tools
	RobotMonitor = "monitor" // uptime and speed checks
	RobotOther   = "other"   // scripts, feed readers, everything unnamed
)

type robotRule struct {
	re         *regexp.Regexp
	name, kind string
}

func rr(pattern, name, kind string) robotRule {
	return robotRule{regexp.MustCompile(`(?i)` + pattern), name, kind}
}

// The first match wins: the exact names come before the general ones.
var robotRules = []robotRule{
	// AI assistants
	rr(`ChatGPT-User`, "ChatGPT (answering someone)", RobotAI),
	rr(`OAI-SearchBot`, "ChatGPT search", RobotAI),
	rr(`GPTBot`, "OpenAI (learning)", RobotAI),
	rr(`Claude-User`, "Claude (answering someone)", RobotAI),
	rr(`Claude-SearchBot`, "Claude search", RobotAI),
	rr(`ClaudeBot|anthropic-ai|Claude-Web`, "Anthropic (learning)", RobotAI),
	rr(`Perplexity-User`, "Perplexity (answering someone)", RobotAI),
	rr(`PerplexityBot`, "Perplexity search", RobotAI),
	rr(`MistralAI-User`, "Mistral (answering someone)", RobotAI),
	rr(`DuckAssistBot`, "DuckDuckGo AI", RobotAI),
	rr(`Google-CloudVertexBot|Google-Extended|GoogleAgent|Gemini-Deep-Research|Google-NotebookLM`, "Google AI", RobotAI),
	rr(`meta-externalagent|meta-externalfetcher|meta-webindexer|FacebookBot`, "Meta AI", RobotAI),
	rr(`Bytespider|TikTokSpider`, "ByteDance (TikTok)", RobotAI),
	rr(`Amazonbot|NovaAct`, "Amazon", RobotAI),
	rr(`Applebot-Extended`, "Apple AI", RobotAI),
	rr(`CCBot`, "Common Crawl", RobotAI),
	rr(`cohere-ai|cohere-training`, "Cohere", RobotAI),
	rr(`YouBot`, "You.com", RobotAI),
	rr(`Diffbot`, "Diffbot", RobotAI),
	rr(`AI2Bot|Timpibot|ImagesiftBot|Omgili|PanguBot|Kangaroo Bot|ICC-Crawler|img2dataset|FirecrawlAgent|Crawl4AI|Brightbot|iaskspider|DeepSeekBot|xAI-Bot|GrokBot`, "Other AI crawlers", RobotAI),

	// link previews: iMessage says it is both Facebook and Twitter
	rr(`facebookexternalhit.*Twitterbot`, "iMessage", RobotPreview),
	rr(`facebookexternalhit|Facebot|facebookcatalog`, "Facebook", RobotPreview),
	rr(`Twitterbot`, "X (Twitter)", RobotPreview),
	rr(`LinkedInBot`, "LinkedIn", RobotPreview),
	rr(`Slackbot|Slack-ImgProxy`, "Slack", RobotPreview),
	rr(`Discordbot`, "Discord", RobotPreview),
	rr(`TelegramBot`, "Telegram", RobotPreview),
	rr(`WhatsApp`, "WhatsApp", RobotPreview),
	rr(`SkypeUriPreview`, "Skype / Teams", RobotPreview),
	rr(`Pinterestbot|Pinterest/0\.`, "Pinterest", RobotPreview),
	rr(`redditbot`, "Reddit", RobotPreview),
	rr(`Mastodon|Pleroma|Akkoma|Misskey`, "Mastodon", RobotPreview),
	rr(`Bluesky|Cardyb`, "Bluesky", RobotPreview),
	rr(`Snap URL Preview`, "Snapchat", RobotPreview),
	rr(`Iframely|Embedly|opengraph|MetaInspector|vkShare|Google-PageRenderer`, "Other link previews", RobotPreview),

	// search engines
	rr(`Googlebot|Google-InspectionTool|Storebot-Google|AdsBot-Google|Mediapartners-Google|APIs-Google|GoogleOther|Google-Site-Verification|Google Favicon`, "Google", RobotSearch),
	rr(`bingbot|BingPreview|msnbot|adidxbot|MicrosoftPreview`, "Bing", RobotSearch),
	rr(`DuckDuckBot|DuckDuckGo-Favicons`, "DuckDuckGo", RobotSearch),
	rr(`Yandex\w*Bot|YandexImages|YandexMetrika|YandexRender|YandexFavicons`, "Yandex", RobotSearch),
	rr(`Baiduspider`, "Baidu", RobotSearch),
	rr(`Applebot`, "Apple", RobotSearch),
	rr(`Slurp`, "Yahoo", RobotSearch),
	rr(`SeznamBot`, "Seznam", RobotSearch),
	rr(`Qwantbot|Qwantify`, "Qwant", RobotSearch),
	rr(`Yeti/|NaverBot`, "Naver", RobotSearch),
	rr(`Sogou \w*spider|Sogou web`, "Sogou", RobotSearch),
	rr(`MojeekBot`, "Mojeek", RobotSearch),
	rr(`kagibot|Kagi-`, "Kagi", RobotSearch),
	rr(`Bravebot`, "Brave", RobotSearch),
	rr(`PetalBot`, "Petal (Huawei)", RobotSearch),
	rr(`ia_archiver|archive\.org_bot`, "Internet Archive", RobotSearch),

	// SEO tools
	rr(`AhrefsBot|AhrefsSiteAudit`, "Ahrefs", RobotSEO),
	rr(`SemrushBot|SiteAuditBot|SplitSignalBot`, "Semrush", RobotSEO),
	rr(`MJ12bot`, "Majestic", RobotSEO),
	rr(`DotBot|rogerbot`, "Moz", RobotSEO),
	rr(`DataForSeoBot|SerpstatBot|BLEXBot|Screaming Frog|SEOkicks|Barkrowler|serpapi|SISTRIX|linkdexbot|seobility|Seekport`, "Other SEO tools", RobotSEO),

	// uptime and speed checks
	rr(`UptimeRobot`, "UptimeRobot", RobotMonitor),
	rr(`Pingdom`, "Pingdom", RobotMonitor),
	rr(`Uptime-Kuma`, "Uptime Kuma", RobotMonitor),
	rr(`Better Uptime|BetterStack`, "Better Stack", RobotMonitor),
	rr(`Chrome-Lighthouse|Lighthouse|PageSpeed|PTST|GTmetrix`, "Speed tests", RobotMonitor),
	rr(`StatusCake|Site24x7|Datadog|NewRelic|Checkly|HetrixTools|Freshping|monitor|uptime`, "Other monitors", RobotMonitor),

	// the rest, by what they are
	rr(`Feedly|FreshRSS|Miniflux|NetNewsWire|Feedbin|Inoreader|NewsBlur|Tiny Tiny RSS|feedfetcher|\brss\b|feedparser|FeedBurner`, "Feed readers", RobotOther),
	rr(`headless|phantom|puppeteer|playwright|selenium|cypress`, "Automated browsers", RobotOther),
	rr(`curl/|wget/|python-|aiohttp|httpx|go-http|java/|okhttp|axios|node-fetch|undici|httpclient|libwww|Scrapy|Ruby/|PHP/|Faraday|Guzzle|Postman|insomnia`, "Scripts and tools", RobotOther),
}

// Robot names a robot from its User-Agent: who it is and what kind. ok is
// false for a person's browser.
func Robot(agent string) (name, kind string, ok bool) {
	agent = strings.TrimSpace(agent)
	for _, r := range robotRules {
		if r.re.MatchString(agent) {
			return r.name, r.kind, true
		}
	}
	if IsBot(agent) {
		return "Other robots", RobotOther, true
	}
	return "", "", false
}
