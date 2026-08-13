/**
 * Routes whose backing contracts do not exist yet.
 *
 * Each entry names the endpoints the backend must publish before the page can
 * be built. Keeping the gap in one file means the backend-gap checklist and the
 * UI can never drift apart: delete an entry here when its contract ships.
 *
 * Nothing on these pages renders mock data. They render the empty state a real
 * route shows before its first record, and never a fabricated one.
 */

export interface UnbuiltEntry {
  title: string;
  summary: string;
  empty: { title: string; description: string };
  readonly requires: readonly string[];
}

export const ROADMAP = {
  overview: {
    title: "Overview",
    summary: "Fleet-wide rates, usage and cost across every project.",
    empty: {
      title: "Nothing to summarise yet",
      description:
        "Rates and spend are aggregated by the server, never derived in the browser from an unbounded job list.",
    },
    requires: [
      "GET /v1/dashboard/overview?window=&project_id=",
      "browser-image and compatibility versions in GET /version",
    ],
  },
  stealth: {
    title: "AI stealth coherence",
    summary:
      "One declared identity across transport, presentation and behaviour, checked before a run and watched during it.",
    empty: {
      title: "No coherence profiles",
      description:
        "Anti-bot systems catch contradictions, not bad user agents. A profile pins TLS, headers, fingerprint and behaviour together, and a break names the layer that diverged.",
    },
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
  aiAutomationBuilder: {
    title: "AI automation builder",
    summary: "Describe a site and the fields you want; a model writes the handler you deploy.",
    empty: {
      title: "No generated apps",
      description:
        "The generated code is the artifact, not a hidden prompt: you read it, edit it, and it deploys through the same path as anything you wrote yourself. The model writes code and never runs your scrape.",
    },
    requires: [
      "a model call under a strict JSON schema returning a handler, its requirements and an output schema, so a bad generation fails validation instead of deploying",
      "a dry run against the target URL before the first deploy, under the same egress and robots policy an execution gets, so the draft is checked against the real page rather than the model's guess of it",
      "a template catalogue the generator starts from, and a diff against it, so a regeneration is reviewable rather than a black box that changed its mind",
      "the generated output schema handed to the data-health baseline, so drift is measured against what the generator promised on day one",
      "generation recorded against the app — prompt, model, template version — because a handler nobody can explain is worse than one nobody wrote",
    ],
  },
  servers: {
    title: "Servers",
    summary: "A machine that holds one app resident behind an endpoint, billed for the time it is up.",
    empty: {
      title: "No servers",
      description:
        "An app is executed, not hosted: an execution invokes a build once and bills the compute it used. A server inverts that, which is the right shape for a long-lived crawler or a warm browser pool.",
    },
    requires: [
      "server create / list / detail / delete, pinned to one app and one ready build",
      "a lifecycle the control plane owns — starting, ready, draining, stopped — with the reason a server left ready",
      "the exposed endpoint and its authentication, scoped to the server rather than to the project key",
      "resident-time metering separate from per-execution compute, so an invoice can show both without double-counting",
      "GET /v1/servers/{id}/logs and health, since a resident process has no execution record to attach either to",
    ],
  },
  proxies: {
    title: "Proxies",
    summary: "The pool a run reaches the internet through, and how each exit is behaving.",
    empty: {
      title: "No proxies",
      description:
        "Proxy secrets are write-only: they are never returned, cached or rendered once saved.",
    },
    requires: [
      "proxy list and detail contracts, with health, quarantine and concurrency",
      "target-specific latency and outcome observations",
      "a proxy test action returning a structured result",
    ],
  },
  environmentVariables: {
    title: "Environment variables",
    summary: "The configuration and secrets an app's code reads at execution time.",
    empty: {
      title: "No environment variables",
      description:
        "A variable belongs to an app and is resolved when an execution starts, so changing one never rebuilds. Secret values are write-only: set once, never read back.",
    },
    requires: [
      "GET / PUT / DELETE /v1/apps/{app_id}/environment, with a variable marked secret at creation and never returned after it",
      "resolution pinned to the execution rather than the build, so a rotated secret takes effect without a rebuild and an old execution still shows what it ran with",
      "the values injected into the runtime process by the worker instead of baked into an artifact, since a build is immutable and shared",
      "a redaction pass over execution logs and captured output, so a secret that a handler prints does not become evidence",
    ],
  },
  dataHealth: {
    title: "Data health",
    summary: "Whether a scrape is still returning what it used to, field by field.",
    empty: {
      title: "No baselines yet",
      description:
        "Scraped payloads rot quietly: a field disappears, a price reads \"Sign in to see price\" and still passes every validator downstream. Structural drift is a diff; semantic drift is the residue, and always overrulable.",
    },
    requires: [
      "a per-app output baseline derived from prior executions, versioned so a deliberate schema change resets it rather than alarming forever",
      "a deterministic structural pass — missing field, type change, cardinality collapse — that runs first, and is the only thing that runs when it already explains the drift",
      "a semantic judge over the residue: an llm model call under a strict JSON schema, returning per-field verdict, confidence and the offending value rather than one score",
      "GET /v1/executions/{id}/data-health, with a user override recorded beside the verdict rather than replacing it",
    ],
  },
  activity: {
    title: "Activity",
    summary: "Who changed what, and when. Read under Users, beside the people it attributes.",
    empty: {
      title: "No activity",
      description:
        "One append-only log rather than two: resource changes such as a deployment created or a key revoked, alongside the security events that are not resource changes at all, such as a sign-in.",
    },
    requires: [
      "GET /v1/activity?actor_id=&subject_type=&subject_id=&project_id=&event_type=&created_after=&limit=&cursor=",
      "an append-only record written inside the same transaction as the change it describes, so a successful write can never lack its entry",
      "actor attribution that survives the actor: a deleted user's entries keep their recorded username",
    ],
  },
} as const satisfies Record<string, UnbuiltEntry>;
