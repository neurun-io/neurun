/**
 * Public-site copy that makes a claim about the product.
 *
 * Kept out of the page components because every row here is checkable against
 * `api/openapi.yaml` or `lib/roadmap.ts`, and a claim that drifts from the
 * contract is the one kind of marketing bug that costs trust. Roadmap rows are
 * marked, never softened: nothing on the public site implies a capability works
 * before it does.
 */

export type CapabilityState = "available" | "partial" | "roadmap";

export interface Capability {
  name: string;
  detail: string;
  state: CapabilityState;
}


/**
 * The whole truth about 0.1.0. `partial` means the capability exists but is
 * narrower than the name suggests; `roadmap` means it is unbuilt, and unbuilt
 * capabilities are not sold on any plan.
 */
export const CAPABILITIES: Capability[] = [
  {
    name: "Projects and apps",
    detail: "Create, list, rename and delete, with a typed confirmation on the deletes that cascade.",
    state: "available",
  },
  {
    name: "Source deployments",
    detail: "POST /v1/deployments takes the archive as multipart and records its sha256 before building.",
    state: "available",
  },
  {
    name: "Builds",
    detail: "Numbered, immutable, building → ready or failed, with the artifacts each one produced.",
    state: "available",
  },
  {
    name: "Executions",
    detail: "Durably queued against the latest ready build, then queued → running → succeeded or failed.",
    state: "available",
  },
  {
    name: "Rerun",
    detail: "The same input against the same build. Refused if that build is no longer ready, rather than quietly running a newer one.",
    state: "available",
  },
  {
    name: "Users and scoped keys",
    detail: "Admin, operator and viewer roles; API keys carry scopes and show their secret exactly once.",
    state: "available",
  },
  {
    name: "Runtimes",
    detail: "Python only. The runtime field is a constant in the contract today rather than an open enum.",
    state: "partial",
  },
  {
    name: "AI stealth coherence",
    detail:
      "One declared profile across transport, presentation and behaviour, checked before the run and watched during it. A break names the layer that diverged.",
    state: "roadmap",
  },
  {
    name: "AI automation builder",
    detail: "Describe a site and the fields you want; a model writes the handler, its output schema and the pipeline. You read the code before it deploys.",
    state: "roadmap",
  },
  {
    name: "Runners",
    detail: "A server that holds one app resident and exposes an endpoint, billed for the time it is up instead of per execution.",
    state: "roadmap",
  },
  {
    name: "Fleet aggregation",
    detail: "GET /v1/dashboard/overview. Rates and spend are never derived in the browser.",
    state: "roadmap",
  },
  {
    name: "Proxies",
    detail: "Pool health, quarantine and per-target latency. Proxy secrets are write-only and never returned.",
    state: "roadmap",
  },
  {
    name: "Data health",
    detail: "Structural drift is a diff. Semantic drift is the residue, judged under a strict schema and always overrulable.",
    state: "roadmap",
  },
  {
    name: "Webhooks",
    detail: "Endpoints, subscribed events, delivery state, secret rotation and replay.",
    state: "roadmap",
  },
  {
    name: "Activity log",
    detail: "One append-only log for resource changes and security events, written in the same transaction as the change.",
    state: "roadmap",
  },
];

/** What the boundary refuses, stated as behaviour rather than reassurance. */
export const REFUSED = [
  "Direct database, queue and artifact-store access",
  "Client-side retry, claim or cost logic",
  "Captured output rendered in a trusted document",
  "Cross-project reads on a project-scoped key",
  "Secrets in localStorage, URLs or breadcrumbs",
  "Executions pinned to a build that is not ready",
];
