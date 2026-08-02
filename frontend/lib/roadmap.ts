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
  sessions: {
    title: "Sessions",
    summary:
      "Browser sessions, live resource pressure and signed CDP access are not part of this release. No session endpoint or SSE usage contract exists in the current OpenAPI.",
    requires: [
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
  agents: {
    title: "Agents",
    summary:
      "Agent fleet state, capacity and bundle compatibility are not part of this release. No agent endpoint exists in the current OpenAPI.",
    requires: [
      "GET /v1/agents?status=&label=&limit=&cursor=",
      "GET /v1/agents/{id}",
      "installed function-bundle version and digest per agent, for incompatibility flagging",
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
      "Immutable identity version history and coherence-validation failures need an identity contract.",
    requires: ["identity list, detail and version-history contracts"],
  },
  profiles: {
    title: "Profiles",
    summary:
      "Profile metadata and version history need a profile contract. Import and export carry an elevated-scope warning and explicit confirmation when they ship.",
    requires: ["profile metadata and version-history contracts", "controlled import/export endpoints"],
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
