/**
 * Routes whose backing contracts do not exist yet.
 *
 * Each entry names the endpoints the backend must publish before the page can
 * be built. Keeping the gap in one file means the backend-gap checklist and the
 * UI can never drift apart: delete an entry here when its contract ships.
 *
 * Nothing on these pages renders mock data. A dashboard that fakes a fleet view
 * is worse than one that admits it has none.
 */

export interface RoadmapEntry {
  title: string;
  summary: string;
  readonly requires: readonly string[];
}

export const ROADMAP = {
  overview: {
    title: "Overview",
    summary:
      "Fleet-wide rates, usage and cost need a server-side aggregation. Deriving them in the browser would mean downloading unbounded job lists and computing numbers the server never agreed to.",
    requires: [
      "GET /v1/dashboard/overview?window=&project_id=",
      "capability discovery through GET /version, including async_jobs_enabled and job_durability",
      "browser-image and compatibility versions in GET /version",
    ],
  },
  browsers: {
    title: "Browsers",
    summary:
      "The browsers available to run against. Uses what is already installed on the host rather than shipping its own: the server reports what it found, an operator can import one by pointing at an executable, enable or disable it, mark a default, and request one be added when the browser they need is not present. Sessions, identities and profiles all attach to a browser, so this is the first of the four.",
    requires: [
      "a discovery pass that reports installed browsers with their executable path, version and channel",
      "import by path, with the server verifying the executable is a browser it can drive before accepting it",
      "enable / disable and a per-project default",
      "a request record for an absent browser, so an operator asks once rather than filing it elsewhere",
    ],
  },
  sessions: {
    title: "Sessions",
    summary:
      "A live instance of one of those browsers, with resource pressure and signed CDP access. Depends on Browsers: a session is an instance of something, and nothing is registered yet. No session endpoint or SSE usage contract exists in the current OpenAPI.",
    requires: [
      "a registered browser to launch",
      "session create / list / detail / keepalive / screenshot / save-profile / usage / history",
      "an authenticated session event stream (SSE) with Last-Event-ID resume",
      "session close endpoint",
    ],
  },
  proxies: {
    title: "Proxies",
    summary:
      "Proxy health, quarantine and per-target observations are not part of this release. Proxy secrets are write-only and will never be returned, cached or rendered.",
    requires: [
      "proxy list and detail contracts, with health, quarantine and concurrency",
      "target-specific latency and outcome observations",
      "a proxy test action returning a structured result",
    ],
  },
  projects: {
    title: "Projects",
    summary:
      "Quotas, domain policy, robots mode, retention and allowed origins need project contracts. The current API key determines the project; a locally selected project ID is never treated as authority.",
    requires: ["GET /v1/projects", "GET /v1/projects/{id}", "PATCH /v1/projects/{id}"],
  },
  apiKeys: {
    title: "API keys",
    summary:
      "Key creation, scoping and revocation are not part of this release. When they ship, the complete key is shown exactly once at creation and never again.",
    requires: [
      "GET /v1/api-keys",
      "POST /v1/api-keys",
      "POST /v1/api-keys/{id}/revoke",
    ],
  },
  identities: {
    title: "Identities",
    summary:
      "Who a browser appears to be: the fingerprint surface a site measures — user agent, locale, timezone, screen metrics, fonts, canvas and WebGL signatures. An identity is coherent when those agree with each other and incoherent when they contradict, such as claiming macOS Safari while carrying Linux fonts and a Chrome WebGL vendor. Validating that is the whole point, so versions are immutable: changing a fingerprint mid-run makes the evidence unreadable.",
    requires: [
      "a registered browser to present the identity",
      "identity list, detail and immutable version-history contracts",
      "coherence validation, reporting which fields contradict rather than a single pass or fail",
    ],
  },
  profiles: {
    title: "Profiles",
    summary:
      "What a browser remembers between sessions: cookies, localStorage, IndexedDB and logged-in state. The counterpart to an identity — identity is presentation, profile is state, and one identity may carry several profiles. Exporting a profile exports live session cookies, which is exporting credentials, so import and export carry an elevated-scope warning and explicit confirmation.",
    requires: [
      "a registered browser to own the state",
      "profile metadata and version-history contracts",
      "controlled import/export endpoints, scoped separately from ordinary profile reads",
    ],
  },
  webhooks: {
    title: "Webhooks",
    summary:
      "Endpoint registration, delivery state, secret rotation and replay need a webhook contract.",
    requires: [
      "webhook endpoint and subscription contracts",
      "recent delivery state, with replay for eligible failed deliveries",
      "secret rotation",
    ],
  },
  audit: {
    title: "Audit",
    summary:
      "Security and administrative audit events need an append-only, cursor-paginated contract.",
    requires: ["GET /v1/audit-events?type=&created_after=&limit=&cursor="],
  },
  activity: {
    title: "Activity",
    summary:
      "Who changed what, and when. Distinct from Audit: audit records security and administrative events, activity records every mutating call against a resource — a deployment created, an app deleted, a key revoked. The server writes no such log today, and deriving one in the browser would mean inventing history the backend never agreed to.",
    requires: [
      "GET /v1/activity?actor_id=&subject_type=&subject_id=&project_id=&created_after=&limit=&cursor=",
      "an append-only record written inside the same transaction as the change it describes, so a successful write can never lack its entry",
      "actor attribution that survives the actor: a deleted user's entries keep their recorded username",
    ],
  },
} as const satisfies Record<string, RoadmapEntry>;
