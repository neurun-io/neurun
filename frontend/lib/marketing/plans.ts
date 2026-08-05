/**
 * The commercial model.
 *
 * An app is executed, not hosted: the meter is the compute an execution
 * consumes — memory reserved multiplied by wall time, from the moment a worker
 * claims the execution to the moment it goes terminal. Queued time is not
 * billed, because a queued execution is holding no worker.
 *
 * Runners invert that and are metered by resident time instead. They are
 * unbuilt, so they are priced nowhere and promised on no plan — see
 * `ROADMAP.runners`.
 */

export type Cycle = "monthly" | "annual";

export interface Plan {
  id: string;
  name: string;
  /** Mono badge in the plan header. */
  tag: string;
  price: Record<Cycle, string>;
  /** Set when the plan is not metered — Enterprise carries no list price. */
  custom?: boolean;
  summary: string;
  features: string[];
  cta: string;
  href: string;
  featured?: boolean;
}

export const UNIT = "GB-hour";

export const PLANS: Plan[] = [
  {
    id: "solo",
    name: "Solo",
    tag: "1 seat",
    price: { monthly: "$29", annual: "$290" },
    summary:
      "One engineer, one pipeline, real provenance. Enough headroom to run a nightly crawl and still read back last week's failure.",
    features: [
      "**50** GB-hours of execution compute",
      "1 project",
      "7-day execution retention",
      "Full /v1 contract and OpenAPI document",
      "Community support",
    ],
    cta: "Start free",
    href: "/register",
  },
  {
    id: "team",
    name: "Team",
    tag: "most teams",
    price: { monthly: "$349", annual: "$3,490" },
    summary:
      "A data team running production pipelines, where somebody who did not write the handler still has to explain why it failed at 03:00.",
    features: [
      "**1,000** GB-hours of execution compute",
      "10 projects, 10 seats",
      "90-day execution retention",
      "Scoped API keys and activity log",
      "Priority support, next business day",
    ],
    cta: "Start free",
    href: "/register",
    featured: true,
  },
  {
    id: "enterprise",
    name: "Enterprise",
    tag: "annual",
    price: { monthly: "Custom", annual: "Custom" },
    custom: true,
    summary:
      "Compute you commit to, deployed where your compliance team can point at it. Licensed container inside your own network, or a dedicated plane run for you.",
    features: [
      "Committed compute, no per-seat charge",
      "Self-managed in your VPC, or dedicated",
      "Retention and residency you set",
      "Export written into the contract",
      "An availability commitment and a named contact",
    ],
    cta: "Talk to engineering",
    href: "mailto:sales@neurun.dev",
  },
];

/** In every plan, including the smallest. Never an upgrade. */
export const ALWAYS_INCLUDED = [
  "Immutable builds and the digest that answered",
  "Executions pinned to a build, and rerun against it",
  "Logs, output and structured failures",
  "The full /v1 contract and OpenAPI document",
  "The operator dashboard, in full",
  "Export, on every plan and on exit",
];

export const COMPARISON: { label: string; values: [string, string, string] }[] = [
  { label: "Included compute", values: ["50 GB-hours", "1,000 GB-hours", "committed"] },
  { label: "Projects", values: ["1", "10", "unlimited"] },
  { label: "Seats", values: ["1", "10", "unlimited"] },
  { label: "Execution retention", values: ["7 days", "90 days", "you set it"] },
  { label: "Scoped API keys", values: ["1 key", "unlimited", "unlimited"] },
  { label: "Deployment", values: ["hosted", "hosted", "your VPC"] },
  { label: "Runners", values: ["roadmap", "roadmap", "roadmap"] },
  { label: "Support", values: ["community", "next business day", "named contact"] },
];

export const FAQ = [
  {
    q: "What exactly is metered?",
    a: "Memory reserved multiplied by wall time, from the moment a worker claims the execution to the moment it goes terminal. Queued time is free. A build is free. A rerun costs whatever it consumes, like any other execution.",
  },
  {
    q: "What happens at the plan limit?",
    a: "By default you continue and the overage is itemised. Turn on the hard cap and further executions are refused with a documented error instead — never silently dropped.",
  },
  {
    q: "Can I run it inside my own network?",
    a: "On Enterprise. You get the licensed container and run it in your VPC; execution output never leaves your infrastructure. Source code is not distributed.",
  },
  {
    q: "If I leave, do I keep my history?",
    a: "Yes. Export is an endpoint on every plan, not a retention lever. Apps, deployments, builds and executions come out as newline-delimited JSON.",
  },
];
