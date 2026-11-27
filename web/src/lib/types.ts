export interface User { id: number; email: string; name: string; role: 'admin' | 'member'; created_at: string }
export interface APIToken { id: number; user_id: number; name: string; prefix: string; last_used_at: string | null; created_at: string }

export type ConnMode = 'core' | 'direct' | 'off'
export type ConnVia = '' | 'core' | 'direct'
export type AppSlug = 'mail' | 'notes' | 'tasks'

type ConnFields = { [K in AppSlug as `${K}_mode`]: ConnMode } & { [K in AppSlug as `${K}_via`]: ConnVia } & { [K in AppSlug as `${K}_detail` | `${K}_url` | `${K}_token` | `${K}_public_url` | `${K}_open_url`]: string } & { [K in AppSlug as `${K}_configured`]: boolean }

export type Settings = ConnFields & {
  core_configured: boolean
  core_registered?: boolean
  title: string; date_format: string; time_format: string; week_start: number
  tz: string; tz_set: string
  weekly_report: boolean; report_email: boolean; report_to: string; ai_help: boolean
  notify_hour: number; push_phone: boolean; notify_url: string; notify_format: string; notify_configured: boolean
  collect_url: string; public_url: string; geo: string; geo_kind: string
  ai_provider: 'auto' | 'pocket' | 'openai'; ai_pocket_model: string; openai_api_key: string; openai_model: string; openai_base_url: string
  setup_complete: boolean; base_url: string; demo: boolean
  google_configured: boolean; google_email: string
}

export interface SiteOptions {
  outbound: boolean; downloads: boolean; clicks: boolean; forms: boolean; not_found: boolean; search: boolean; search_params: string[]; speed: boolean
  hash: boolean; localhost: boolean; exclude: string[]; ignore_ips: string[]; keep_days: number
  identify?: boolean; browser_id?: boolean; forget_days?: number
  spike: boolean; quiet: boolean; weekly: boolean
}

export interface Goal { id: number; site_id: number; name: string; kind: 'page' | 'event' | 'robot'; match: string; robot?: string; created_at: string }
export interface Step { kind: 'page' | 'event' | 'robot'; match: string; robot?: string; label?: string }
export interface Funnel { id: number; site_id: number; name: string; steps: Step[]; created_at: string }

export interface Site {
  id: number; domain: string; name: string; label: string; tz: string; tz_set: string; also: string[]; options: SiteOptions; proxy_key: string; position: number
  first_hit: number; last_hit: number; installed: boolean; created_at: string; script_url: string; event_url: string; snippet: string; ignore_url: string
  // the list adds
  spark?: number[]; today?: number; live?: number; visitors_30d?: number; visitors_prev_30d?: number
  // one site adds
  goals?: Goal[]; funnels?: Funnel[]
}

export interface Totals { visitors: number; visits: number; pageviews: number; views_per_visit: number; bounce_rate: number; duration: number; events: number }
export interface Point { start: number; visitors: number; visits: number; pageviews: number; bounce_rate: number; duration: number }
export interface Filter { dim: string; value: string; not?: boolean }
export interface Range { key: string; interval: 'hour' | 'day' | 'week' | 'month'; from: string; to: string; label: string }

export interface Note { id: number; site_id: number; day: string; text: string; created_at: string; ts?: number }

export interface Stats { site: Site; range: Range; filters: Filter[]; totals: Totals; previous: Totals; series: Point[]; previous_series: Point[]; notes: Note[]; robots?: boolean; now: number; live: number }

export interface Row { value: string; country?: string; visitors: number; visits?: number; pageviews?: number; events?: number; bounce_rate?: number; duration?: number; time_on_page?: number; scroll?: number }

export interface Money { currency: string; amount: number }
export interface Conversion { goal: Goal; visitors: number; conversions: number; rate: number; revenue?: Money[]; robot?: boolean }

/** One speed measure: three of four page loads stayed under p75; n loads measured. */
export interface Metric { p75: number; n: number }
export interface SpeedRow { value: string; loads: number; lcp?: Metric; inp?: Metric; cls?: Metric; ttfb?: Metric; fcp?: Metric }
export interface Speed { dim: string; all: SpeedRow; rows: SpeedRow[]; on: boolean }

/** Visits by day of the week (0 = Sunday) and hour, in the site's time zone. */
export interface When { cells: number[][]; max: number; visits: number; week_start: number; tz: string }

/** What people typed into Google before they came (from Google Search Console). */
export interface KeywordRow { words?: string; clicks: number; impressions: number; ctr: number; position: number }
export interface Keywords { by?: 'query' | 'page'; state: 'ok' | 'off' | 'no_property' | 'error'; error?: string; email?: string; property?: string; seen?: string[]; from?: string; to?: string; rows: KeywordRow[]; totals?: KeywordRow; partial?: boolean; demo?: boolean }

/** A robot (or a page robots read): how many pages it read. Robots are never visitors. */
export interface RobotRow { value: string; kind?: string; hits: number; pages?: number; last?: number }
export interface Robots { rows: RobotRow[]; total: number; seen: boolean }

export interface PathStep { value: string; times: number }
export interface Paths { page: string; views: number; before: PathStep[]; after: PathStep[] }
export interface StepCount { step: Step; visitors: number; dropped: number; rate: number; next: number; robot?: boolean }

export interface LiveHit { ts: number; name: string; path: string; country: string; city: string; device: string; browser: string; source: string; visit_id: number }
export interface Live { now: number; minutes: number[]; pages: Row[]; sources: Row[]; countries: Row[]; recent: LiveHit[] }

export interface Visit {
  id: number; site_id: number; started: number; last: number; hostname: string; entry: string; exit: string; pageviews: number; events: number; engaged: number
  referrer: string; source: string; channel: string; utm_source: string; utm_medium: string; utm_campaign: string; utm_content: string; utm_term: string
  country: string; region: string; city: string; device: string; browser: string; browser_version: string; os: string; os_version: string; language: string
  // who it was: only on sites that know who visits
  ip?: string; network?: string; asn?: number; postal?: string; lat?: number; lon?: number; client_id?: string; agent?: string; ref_url?: string
  details?: Record<string, string | number | boolean>
}
/** Another visit by the same browser (its own ID) or from the same address. */
export interface SameVisitor { id: number; started: number; last: number; entry: string; pageviews: number; engaged: number; source: string; city: string; country: string; browser: string; os: string; by: 'browser' | 'address' | 'both' }
export interface VisitDetail { visit: Visit; hits: Hit[]; others?: SameVisitor[]; others_total?: number; first_seen?: number; host?: string }
export interface Hit { id: number; visit_id: number; ts: number; name: string; path: string; props?: Record<string, string>; engaged: number; scroll: number; revenue?: number; currency?: string; lcp?: number; inp?: number; cls?: number }

// ---- how people use the site (0.5.0) ----
export interface Landing { page: string; visits: number; visitors: number; bounce: number; engaged: number; pages: number; converted: number; rate: number }
export interface Journey { pages: string[]; visits: number; more: number; converted: number }
export interface SectionSeen { name: string; reached: number; rate: number }
export interface Reading { page: string; views: number; measured: number; depth: { at: number; rate: number }[]; marked: number; sections: SectionSeen[] }
export interface AIPage { page: string; reads: number; robots: number; visits: number }
export interface AIView { reads: number; visits: number; pages: AIPage[]; robots: { name: string; n: number }[]; sources: { name: string; n: number }[] }
export interface SearchRow { term: string; visitors: number; times: number; results: number | null; where?: string }
export interface RobotAgent { agent: string; hits: number; first: number; last: number; path: string }
