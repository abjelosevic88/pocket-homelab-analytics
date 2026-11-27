package ua

import "testing"

func TestRobot(t *testing.T) {
	for agent, want := range map[string][2]string{
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; GPTBot/1.2; +https://openai.com/gptbot)":                                                                  {"OpenAI (learning)", RobotAI},
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko); compatible; ChatGPT-User/1.0; +https://openai.com/bot":                                                               {"ChatGPT (answering someone)", RobotAI},
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; ClaudeBot/1.0; +claudebot@anthropic.com)":                                                                 {"Anthropic (learning)", RobotAI},
		"Mozilla/5.0 (compatible; Claude-User/1.0; +Claude-User@anthropic.com)":                                                                                                   {"Claude (answering someone)", RobotAI},
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; PerplexityBot/1.0; +https://perplexity.ai/perplexitybot)":                                                 {"Perplexity search", RobotAI},
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)":                                                                                                {"Google", RobotSearch},
		"Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)":                                                                                                 {"Bing", RobotSearch},
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15 (Applebot/0.1; +http://www.apple.com/go/applebot)": {"Apple", RobotSearch},
		"facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)":                                                                                               {"Facebook", RobotPreview},
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_11_1) AppleWebKit/601.2.4 (KHTML, like Gecko) Version/9.0.1 Safari/601.2.4 facebookexternalhit/1.1 Facebot Twitterbot/1.0":     {"iMessage", RobotPreview},
		"Slackbot-LinkExpanding 1.0 (+https://api.slack.com/robots)":                                                                                                              {"Slack", RobotPreview},
		"WhatsApp/2.23.20.0 A": {"WhatsApp", RobotPreview},
		"Mozilla/5.0 (compatible; AhrefsBot/7.0; +http://ahrefs.com/robot/)":                                                                                  {"Ahrefs", RobotSEO},
		"Mozilla/5.0 (compatible; UptimeRobot/2.0; http://www.uptimerobot.com/)":                                                                              {"UptimeRobot", RobotMonitor},
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; KeenableBot/1.0; +https://keenable.ai/bot)":                                           {"Keenable", RobotAI},
		"Mozilla/5.0 (compatible; ElevenlabsCrawler/1.0; +https://elevenlabs.io/docs/bots)":                                                                   {"ElevenLabs", RobotAI},
		"Mozilla/5.0 (compatible; ExaSearchBot/1.0; +https://crawler.exa.ai/)":                                                                                {"Exa", RobotAI},
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; Reflectionbot/1.0; +https://reflection.ai/bot) Chrome/151.0.0.0 Safari/537.36":        {"Reflection AI", RobotAI},
		"AgentTrustBot/1.0 (id=f175e0939c37; +https://agenttru.st/crawler; purpose=AI agent directory; contact=admin@agenttru.st)":                            {"AgentTrust", RobotAI},
		"Mozilla/5.0 (compatible; SERankingBacklinksBot/1.0; +https://seranking.com/backlinks-crawler)":                                                       {"SE Ranking", RobotSEO},
		"User-Agent:Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36 Edg/140.0.0.0; 360Spider": {"360 Search", RobotSearch},
		"Mozilla/5.0 (Windows NT 10.0; WOW64; Android) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/100.0.4489.114 Safari/537.36 HuaweiCrawler":              {"Huawei", RobotSearch},
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko); compatible; ShapBot/0.1.0":                                                                       {"ShapBot", RobotOther},
		"curl/8.5.0":                         {"Scripts and tools", RobotOther},
		"SomeNewCrawler/1.0 (+https://x.io)": {"Other robots", RobotOther},
		"":                                   {"Other robots", RobotOther},
	} {
		name, kind, ok := Robot(agent)
		if !ok || name != want[0] || kind != want[1] {
			t.Errorf("Robot(%q) = %q, %q, %v; want %v", agent, name, kind, ok, want)
		}
	}
	for _, person := range []string{
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1",
		"Mozilla/5.0 (X11; Linux x86_64; rv:133.0) Gecko/20100101 Firefox/133.0",
		"Mozilla/5.0 (Linux; Android 14; SM-S921B) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/26.0 Chrome/122.0.0.0 Mobile Safari/537.36",
		// apps people read in are people too
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Teams/24215.1007.3082.1590 Chrome/124.0.0.0 Electron/30.0.0 Safari/537.36",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 [Pinterest/iOS]",
		"Mozilla/5.0 (Linux; Android 13) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Mobile Safari/537.36 QwantMobile/4.5",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 YaBrowser/24.1.0 Safari/537.36",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 Instagram 300.0.0.0 (iPhone14,5; iOS 17_0)",
	} {
		if name, _, ok := Robot(person); ok {
			t.Errorf("a person's browser taken for %q: %s", name, person)
		}
	}
}
