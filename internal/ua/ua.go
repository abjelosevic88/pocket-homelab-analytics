// Package ua reads a browser's User-Agent: the browser, the operating
// system, the kind of device, and whether it is a robot.
package ua

import (
	"regexp"
	"strings"
)

// Info is what a User-Agent tells.
type Info struct {
	Browser        string
	BrowserVersion string // major only
	OS             string
	OSVersion      string
	Device         string // Phone | Tablet | Laptop | Desktop
	Bot            bool
}

var reBot = regexp.MustCompile(`(?i)bot\b|bot/|crawl|spider|slurp|scrape|headless|phantom|lighthouse|pagespeed|pingdom|uptime|monitor|preview|facebookexternalhit|embedly|quora link|whatsapp|telegrambot|discordbot|slackbot|vkshare|curl/|wget/|python-|go-http|java/|okhttp|axios|node-fetch|httpclient|libwww|feedfetcher|rss|validator|ahrefs|semrush|mj12|dotbot|petalbot|bytespider|gptbot|claudebot|anthropic|perplexitybot|ccbot|amazonbot|applebot|yandex(bot|images)|baiduspider|duckduckbot|bingpreview|google-inspectiontool|chrome-lighthouse|cypress|puppeteer|playwright|selenium|electron/[0-9.]+ .*headless`)

// IsBot reports whether the User-Agent is a robot (or empty).
func IsBot(s string) bool {
	return strings.TrimSpace(s) == "" || len(s) < 12 || reBot.MatchString(s)
}

type rule struct {
	re   *regexp.Regexp
	name string
}

// order matters: Edge and Opera say Chrome, Chrome says Safari.
var browsers = []rule{
	{regexp.MustCompile(`FBAN|FBAV|FB_IAB`), "Facebook"},
	{regexp.MustCompile(`Instagram`), "Instagram"},
	{regexp.MustCompile(`LinkedInApp`), "LinkedIn"},
	{regexp.MustCompile(`Snapchat`), "Snapchat"},
	{regexp.MustCompile(`TikTok|musical_ly|BytedanceWebview`), "TikTok"},
	{regexp.MustCompile(`Edg(e|A|iOS)?/(\d+)`), "Edge"},
	{regexp.MustCompile(`(OPR|Opera|OPiOS|OPT)/(\d+)`), "Opera"},
	{regexp.MustCompile(`SamsungBrowser/(\d+)`), "Samsung Internet"},
	{regexp.MustCompile(`YaBrowser/(\d+)`), "Yandex Browser"},
	{regexp.MustCompile(`UCBrowser/(\d+)`), "UC Browser"},
	{regexp.MustCompile(`Vivaldi/(\d+)`), "Vivaldi"},
	{regexp.MustCompile(`DuckDuckGo/(\d+)|Ddg/(\d+)`), "DuckDuckGo"},
	{regexp.MustCompile(`Brave/(\d+)`), "Brave"},
	{regexp.MustCompile(`(Firefox|FxiOS)/(\d+)`), "Firefox"},
	{regexp.MustCompile(`(CriOS|Chrome|Chromium)/(\d+)`), "Chrome"},
	{regexp.MustCompile(`Version/(\d+)[.\d]* (Mobile/\S+ )?Safari/`), "Safari"},
	{regexp.MustCompile(`AppleWebKit/.*Mobile/`), "Safari (in an app)"},
	{regexp.MustCompile(`Trident/|MSIE (\d+)`), "Internet Explorer"},
}

var versionRe = func() map[string]*regexp.Regexp {
	out := map[string]*regexp.Regexp{}
	for name, key := range map[string]string{"Edge": `Edg(?:e|A|iOS)?`, "Opera": `(?:OPR|Opera|OPiOS|OPT)`, "Samsung Internet": `SamsungBrowser`, "Yandex Browser": `YaBrowser`,
		"UC Browser": `UCBrowser`, "Vivaldi": `Vivaldi`, "DuckDuckGo": `(?:DuckDuckGo|Ddg)`, "Brave": `Brave`, "Firefox": `(?:Firefox|FxiOS)`,
		"Chrome": `(?:CriOS|Chrome|Chromium)`, "Safari": `Version`, "Internet Explorer": `(?:MSIE |rv:)`} {
		out[name] = regexp.MustCompile(key + `/?(\d+)`)
	}
	return out
}()

func browserVersion(s, name string) string {
	re := versionRe[name]
	if re == nil {
		return ""
	}
	if m := re.FindStringSubmatch(s); len(m) > 1 {
		return m[1]
	}
	return ""
}

var (
	reIOS     = regexp.MustCompile(`(?:iPhone|CPU) OS (\d+)`)
	reIPad    = regexp.MustCompile(`iPad.*OS (\d+)`)
	reAndroid = regexp.MustCompile(`Android (\d+)`)
	reMac     = regexp.MustCompile(`Mac OS X (\d+)[._](\d+)`)
	reWin     = regexp.MustCompile(`Windows NT (\d+\.\d+)`)
	reCrOS    = regexp.MustCompile(`CrOS`)
)

// Parse reads a User-Agent. width is the screen width the script sent (0 =
// unknown); it decides the kind of device when it is there.
func Parse(s string, width int) Info {
	in := Info{Bot: IsBot(s)}
	for _, r := range browsers {
		if r.re.MatchString(s) {
			in.Browser = r.name
			break
		}
	}
	if in.Browser == "" {
		in.Browser = "Other"
	}
	in.BrowserVersion = browserVersion(s, in.Browser)
	switch {
	case reIPad.MatchString(s):
		in.OS, in.OSVersion = "iPadOS", reIPad.FindStringSubmatch(s)[1]
	case strings.Contains(s, "iPhone") || strings.Contains(s, "iPod"):
		in.OS = "iOS"
		if m := reIOS.FindStringSubmatch(s); m != nil {
			in.OSVersion = m[1]
		}
	case reAndroid.MatchString(s):
		in.OS, in.OSVersion = "Android", reAndroid.FindStringSubmatch(s)[1]
	case strings.Contains(s, "Android"):
		in.OS = "Android"
	case reCrOS.MatchString(s):
		in.OS = "ChromeOS"
	case reMac.MatchString(s):
		m := reMac.FindStringSubmatch(s)
		in.OS, in.OSVersion = "macOS", m[1]
		if m[1] == "10" {
			in.OSVersion = m[1] + "." + m[2]
		}
	case reWin.MatchString(s):
		in.OS = "Windows"
		in.OSVersion = map[string]string{"10.0": "10/11", "6.3": "8.1", "6.2": "8", "6.1": "7"}[reWin.FindStringSubmatch(s)[1]]
	case strings.Contains(s, "Windows"):
		in.OS = "Windows"
	case strings.Contains(s, "Linux") || strings.Contains(s, "X11"):
		in.OS = "Linux"
	default:
		in.OS = "Other"
	}
	mobile := strings.Contains(s, "Mobi") || in.OS == "iOS"
	tablet := in.OS == "iPadOS" || strings.Contains(s, "Tablet") || (in.OS == "Android" && !strings.Contains(s, "Mobile"))
	switch {
	case width > 0 && width < 576:
		in.Device = "Phone"
	case width > 0 && width < 992:
		in.Device = "Tablet"
	case width > 0 && width < 1440:
		in.Device = "Laptop"
	case width > 0:
		in.Device = "Desktop"
	case tablet:
		in.Device = "Tablet"
	case mobile:
		in.Device = "Phone"
	default:
		in.Device = "Desktop"
	}
	return in
}
