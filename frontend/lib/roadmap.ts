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
  stealth: {
    title: "AI stealth coherence",
    summary:
      "Anti-bot systems catch contradictions, not bad user agents: a ClientHello that says Chrome while the header order says Go, a datacenter ASN whose timezone is residential Berlin, a cursor that reaches every button in a straight line. So this is one declared profile across transport, presentation and behaviour, checked for coherence before the first request and watched for drift during the run. When a target starts refusing, the output names the layer that diverged and when — not a retry through a different proxy.",
    requires: [
      "a versioned profile — transport, presentation, behaviour — pinned to an execution the way a build is",
      "transport checks: TLS (JA3/JA4) and HTTP/2 SETTINGS against the claimed browser, ALPN, header order and casing, Sec-Fetch-* and Sec-CH-UA completeness",
      "network checks: ASN class, exit geography against timezone and Accept-Language, rDNS, WebRTC and DNS resolving outside the tunnel",
      "presentation checks: UA against navigator.platform and WebGL vendor, fonts against the claimed OS, screen and outer/inner deltas, hardwareConcurrency and deviceMemory",
      "leak probes for what the automation adds: navigator.webdriver, CDP artifacts, patched natives that stop reading as native, headless codec and matchMedia gaps",
      "behaviour scored rather than asserted: pointer velocity and overshoot, click scatter inside a target, dwell and inter-keystroke timing, scroll momentum, cadence variance, honeypot contact",
      "stability across runs: a canvas hash that changes every time is as loud as a known-bad one, and one fingerprint hopping ASNs is louder still",
      "a detection oracle: challenge-page classification, 403/429 clustering by profile cohort rather than by app, and silent degradation read off the data-health baseline instead of a second one",
      "a preflight verdict that refuses the run, and a break recorded as an event on the execution with the offending layer and its last coherent version",
    ],
  },
  browsers: {
    title: "Browsers",
    summary:
      "The browsers available to run against, and the live sessions running on them. Uses what is already installed on the host rather than shipping its own: the server reports what it found, an operator can import one by pointing at an executable, enable or disable it, mark a default, and request one be added when the browser they need is not present. A session is one running instance of a registered browser, with resource pressure and signed CDP access — which is why the two are one page: a session list with nothing registered to launch is an empty page explaining another empty page.",
    requires: [
      "a discovery pass that reports installed browsers with their executable path, version and channel",
      "import by path, with the server verifying the executable is a browser it can drive before accepting it",
      "enable / disable, a per-project default, and a request record for an absent browser",
      "session create / list / detail / keepalive / screenshot / save-profile / usage / history / close",
      "an authenticated session event stream (SSE) with Last-Event-ID resume",
    ],
  },
  aiAutomationBuilder: {
    title: "AI automation builder",
    summary:
      "Prompt to pipeline. Describe the site and the fields you want, and a model writes the handler, its output schema and the pipeline around it — a working scrape app in under five minutes. The generated code is the artifact, not a hidden prompt: you read it, edit it, and it deploys through the same path as anything you wrote yourself, pinned to a build like any other. The model writes code and never runs your scrape, which is the whole line — a generator that also executes is one whose mistakes you find in production rather than in a diff.",
    requires: [
      "a model call under a strict JSON schema returning a handler, its requirements and an output schema, so a bad generation fails validation instead of deploying",
      "a dry run against the target URL before the first deploy, under the same egress and robots policy an execution gets, so the draft is checked against the real page rather than the model's guess of it",
      "a template catalogue the generator starts from, and a diff against it, so a regeneration is reviewable rather than a black box that changed its mind",
      "the generated output schema handed to the data-health baseline, so drift is measured against what the generator promised on day one",
      "generation recorded against the app — prompt, model, template version — because a handler nobody can explain is worse than one nobody wrote",
    ],
  },
  runners: {
    title: "Runners",
    summary:
      "An app is executed, not hosted: a deployment produces a build, an execution invokes it once, and the compute it consumed is what gets billed. A runner inverts that. It is a server that holds one app resident and exposes an endpoint, so callers reach the app directly instead of creating an execution per call — which is the right shape for a long-lived crawler, a warm browser pool, or anything whose startup cost dwarfs its work. It is also a different billing unit: a runner is charged for the time it is up, not per invocation, so the two models cannot share a meter. Nothing here is rendered until a runner has a lifecycle the server owns; a page that lists runners it cannot start, stop or bill is a page that lies about what the account is spending.",
    requires: [
      "runner create / list / detail / delete, pinned to one app and one ready build",
      "a lifecycle the server owns — starting, ready, draining, stopped — with the reason a runner left ready",
      "the exposed endpoint and its authentication, scoped to the runner rather than to the project key",
      "resident-time metering separate from per-execution compute, so an invoice can show both without double-counting",
      "GET /v1/runners/{id}/logs and health, since a resident process has no execution record to attach either to",
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
  dataHealth: {
    title: "Data health",
    summary:
      'Scraped payloads rot quietly. A field disappears, a list that returned forty rows returns none, a price that read "$41.99" reads "Sign in to see price" — still a string, still non-empty, still passing every validator downstream. Structural drift is a diff and needs no model: compare an execution\'s output against the shape its predecessors agreed on and name the missing key, the changed type, the collapsed cardinality. Semantic drift is what is left over, and it is the only part worth spending a model on — a field whose shape is intact but whose meaning is gone. The verdict is advisory and recorded: a run is never failed on one model call an operator cannot read and overrule.',
    requires: [
      "a per-app output baseline derived from prior executions, versioned so a deliberate schema change resets it rather than alarming forever",
      "a deterministic structural pass — missing field, type change, cardinality collapse — that runs first, and is the only thing that runs when it already explains the drift",
      "a semantic judge over the residue: an llm model call under a strict JSON schema, returning per-field verdict, confidence and the offending value rather than one score",
      "GET /v1/executions/{id}/data-health, with an operator override recorded beside the verdict rather than replacing it",
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
      "Who a browser appears to be, and what it remembers. Presentation is the fingerprint surface a site measures — user agent, locale, timezone, screen metrics, fonts, canvas and WebGL signatures — and it is coherent when those agree with each other, incoherent when they contradict, such as claiming macOS Safari while carrying Linux fonts and a Chrome WebGL vendor. State is what survives between sessions: cookies, localStorage, IndexedDB and logged-in state, and one identity may carry several sets of it. Both are versioned and immutable, because changing either mid-run makes the evidence unreadable. Exporting state exports live session cookies, which is exporting credentials, so import and export carry an elevated-scope warning and explicit confirmation.",
    requires: [
      "a registered browser to present the identity and own the state",
      "identity and stored-state list, detail and immutable version-history contracts",
      "coherence validation, reporting which fields contradict rather than a single pass or fail",
      "controlled state import/export endpoints, scoped separately from ordinary reads",
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
  activity: {
    title: "Activity",
    summary:
      "Who changed what, and when: every mutating call against a resource — a deployment created, an app deleted, a key revoked — alongside the security and administrative events that are not resource changes at all, such as a sign-in or a widened scope. One append-only log rather than two, because splitting them means an operator reconstructing an incident has to read both and guess which one holds the next event. The server writes no such log today, and deriving one in the browser would mean inventing history the backend never agreed to.",
    requires: [
      "GET /v1/activity?actor_id=&subject_type=&subject_id=&project_id=&event_type=&created_after=&limit=&cursor=",
      "an append-only record written inside the same transaction as the change it describes, so a successful write can never lack its entry",
      "actor attribution that survives the actor: a deleted user's entries keep their recorded username",
    ],
  },
} as const satisfies Record<string, RoadmapEntry>;
