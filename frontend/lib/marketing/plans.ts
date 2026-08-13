/**
 * The commercial model.
 *
 * An app is executed, not hosted: the meter is the compute an execution
 * consumes — memory reserved multiplied by wall time, from the moment a worker
 * claims the execution to the moment it goes terminal. Queued time is not
 * billed, because a queued execution is holding no worker.
 *
 * Servers invert that and are metered by resident time instead. They are
 * unbuilt, so they are priced nowhere and promised on no plan — see
 * `ROADMAP.servers`.
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
    href: "/auth",
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
    href: "/auth",
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
  "The dashboard, in full",
  "Export, on every plan and on exit",
];