import type { Metadata } from "next";
import Link from "next/link";
import { ArrowRight, BookOpen, Check, GitBranch, TriangleAlert } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Callout } from "@/components/neurun/feedback";
import { Logo } from "@/components/neurun/logo";
import { Panel } from "@/components/neurun/panel";
import { Eyebrow, Section, SectionHeading } from "@/components/marketing/parts";
import { FirstCall, PlanGrid } from "@/components/marketing/pricing";
import { CAPABILITIES, REFUSED, type CapabilityState } from "@/lib/marketing/content";
import { ALWAYS_INCLUDED, COMPARISON, FAQ, UNIT } from "@/lib/marketing/plans";

export const metadata: Metadata = {
  title: "Neurun — run the web, skip the infra",
  description:
    "An execution plane for your scrapers, crawlers, HTTP pipelines and Browsers. Every run pins an immutable build and leaves a record you can query afterwards. Priced per GB-hour.",
};

const STATE_BADGE: Record<CapabilityState, { variant: "outline" | "secondary" | "dotted"; icon: typeof Check }> = {
  available: { variant: "outline", icon: Check },
  partial: { variant: "secondary", icon: TriangleAlert },
  roadmap: { variant: "dotted", icon: GitBranch },
};

const BOUNDARY = [
  { label: "Client", value: "your pipeline", meta: "airflow · dagster · cron" },
  { label: "Boundary", value: "POST /v1/…", meta: "bearer neu_live_…", framed: true },
  { label: "Control plane", value: "claim · classify", meta: "queue · outbox" },
  { label: "Worker", value: "execution", meta: "pinned to a build" },
  { label: "Egress", value: "the open web", meta: "declared per app" },
];

export default function Home() {
  return (
    <>
      {/* ---------------------------------------------------------------- hero */}
      <section className="relative overflow-hidden">
        <div
          aria-hidden
          className="nr-grid-field absolute inset-0"
          style={{
            maskImage: "radial-gradient(115% 85% at 46% 0%, #000 18%, transparent 76%)",
            WebkitMaskImage: "radial-gradient(115% 85% at 46% 0%, #000 18%, transparent 76%)",
          }}
        />
        <div className="relative mx-auto w-full max-w-(--nr-container-max) px-6 pt-16 sm:pt-22">
          <div className="grid items-center gap-14 lg:grid-cols-[minmax(0,7fr)_minmax(0,4fr)]">
            <div className="flex flex-col gap-6.5">
              <h1 className="text-[clamp(44px,6.4vw,104px)] leading-[0.96] tracking-display">
                Run the web.
                <br />
                Skip the infra.
              </h1>
              <p className="max-w-[600px] text-xl leading-[1.45]">
                AI has evolved rapidly, but web automation infrastructure is stuck a decade
                behind. We&rsquo;re bridging that gap.
              </p>
              <p className="max-w-[560px] text-lg leading-[1.55] text-fg-secondary">
                An execution plane for scrapers, crawlers, browsers and HTTP pipelines.
              </p>

              <div className="flex flex-wrap gap-3">
                <Button asChild size="lg">
                  <Link href="/register">
                    Start free
                    <ArrowRight aria-hidden strokeWidth={1.5} />
                  </Link>
                </Button>
                <Button asChild size="lg" variant="secondary">
                  <Link href="/overview">
                    Open the dashboard
                    <ArrowRight aria-hidden strokeWidth={1.5} />
                  </Link>
                </Button>
              </div>

              <p className="flex max-w-[560px] items-center gap-2.5 overflow-hidden rounded-md border border-line bg-surface-sunken px-3 py-2.5 font-mono text-caption text-fg-secondary">
                <span className="text-fg-muted">$</span>
                <span className="truncate">curl -X POST api.neurun.dev/v1/deployments</span>
                <span
                  aria-hidden
                  className="ml-auto h-3.5 w-1.5 shrink-0 bg-(--nr-accent) motion-safe:animate-caret"
                />
              </p>
            </div>

            <div className="grid place-items-center max-lg:hidden">
              <Logo className="size-120" />
            </div>
          </div>
        </div>
      </section>

      
      {/* ----------------------------------------------------------- executions */}
      <Section id="executions" className="bg-surface-sunken mt-18 mb-18">
        <div className="grid items-start gap-11 lg:grid-cols-2 lg:gap-14">
          <div className="flex flex-col gap-4.5 lg:sticky lg:top-24">
            <Eyebrow>Executions</Eyebrow>
            <h2 className="text-[clamp(30px,3.1vw,46px)] leading-[1.04] tracking-display">
              Intelligent Runtime for
              <br />
              Automation workloads.
            </h2>
            <p className="text-[17px] leading-[1.6] text-fg-secondary">
              The environment already carries what a scraper needs to be fast — the browser stack,
              the HTTP and parsing libraries, the system packages a headless run depends on — so a
              handler ships as code rather than as a Dockerfile that reinstalls the world on every
              build. Egress is declared per app, and the plane owns the queueing, the claim and the
              retry so your script does not.
            </p>
            <p className="text-[17px] leading-[1.6] text-fg-secondary">
              Every execution stays pinned to the build that was ready when it was created, so a
              rerun repeats the same input against the same code — and refuses if that build is no
              longer ready rather than quietly running a newer one.
            </p>
          </div>

          <div className="flex min-w-0 flex-col gap-3">
            <Panel flush label="Executions" actions={<span className="font-mono text-micro text-fg-muted">dep_01HXQ8F2K9 · 3 of 3</span>}>
              <div className="nr-label grid grid-cols-[64px_minmax(0,1fr)_92px_72px] items-center gap-3 border-b border-line px-3.5 py-2.5">
                <span>#</span>
                <span>Started</span>
                <span>Status</span>
                <span className="text-right">Ran</span>
              </div>
              {[
                { n: "1", at: "12:03:58.104", status: "failed", ran: "30.0s", note: "worker_restarted" },
                { n: "2", at: "12:04:34.902", status: "failed", ran: "15.0s", note: "handler_error" },
                { n: "3", at: "12:05:22.318", status: "succeeded", ran: "2.8s", note: "" },
              ].map((row, index, rows) => (
                <div
                  key={row.n}
                  className={cnRow(index === rows.length - 1)}
                >
                  <span className="font-mono text-meta text-fg">{row.n} / 3</span>
                  <span className="min-w-0 truncate font-mono text-meta text-fg-muted">
                    {row.at}
                    {row.note ? <span className="text-fg-faint"> · {row.note}</span> : null}
                  </span>
                  <Badge variant={row.status === "succeeded" ? "outline" : "secondary"}>
                    {row.status}
                  </Badge>
                  <span className="text-right font-mono text-meta text-fg-muted">{row.ran}</span>
                </div>
              ))}
            </Panel>

            <div className="grid gap-3 sm:grid-cols-2">
              <Panel label="Rerun" actions={<span className="font-mono text-micro text-fg-muted">server-owned</span>}>
                <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1.5 font-mono text-meta">
                  {[
                    ["build_id", "bld_9F3AC41"],
                    ["source_sha256", "9f3a…c41"],
                    ["rerun_of", "exe_01HXQ8F2M4"],
                    ["refused_when", "build not ready"],
                  ].map(([key, value]) => (
                    <div key={key} className="contents">
                      <dt className="text-fg-muted">{key}</dt>
                      <dd className="min-w-0 truncate text-fg-secondary">{value}</dd>
                    </div>
                  ))}
                </dl>
              </Panel>
              <Panel label="Compute" actions={<span className="font-mono text-micro text-fg-muted">metered</span>}>
                <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1.5 font-mono text-meta">
                  {[
                    ["memory", "512 MiB"],
                    ["ran", "2.8s"],
                    ["billed", "1.43 GB-s"],
                    ["queued_time", "not billed"],
                  ].map(([key, value]) => (
                    <div key={key} className="contents">
                      <dt className="text-fg-muted">{key}</dt>
                      <dd className="min-w-0 truncate text-fg-secondary">{value}</dd>
                    </div>
                  ))}
                </dl>
              </Panel>
            </div>
          </div>
        </div>
      </Section>

      {/* ------------------------------------------------------------- boundary */}
      <Section id="boundary">
        <SectionHeading
          className="mb-13"
          eyebrow="Boundary"
          title="One authenticated edge. Everything behind it is refused."
          lead={
            <>
              Your pipeline talks to a public <code className="font-mono text-base text-fg">/v1</code>{" "}
              contract and nothing else. No client reaches the database, the queue or the artifact
              store.
            </>
          }
        />

        <div className="flex items-center overflow-x-auto rounded-lg border border-line bg-surface-panel px-6 py-8">
          {BOUNDARY.map((node, index) => (
            <div key={node.label} className="flex shrink-0 items-center">
              {index > 0 ? (
                <span
                  aria-hidden
                  className="mx-4 h-px w-14 shrink-0 bg-[repeating-linear-gradient(to_right,var(--nr-border-strong)_0_6px,transparent_6px_14px)]"
                />
              ) : null}
              <div
                className={
                  node.framed
                    ? "flex min-w-[150px] flex-col gap-1.5 rounded-md border border-line-default bg-surface-raised px-3.5 py-2.5"
                    : "flex min-w-[132px] flex-col gap-1.5"
                }
              >
                <span className="nr-label">{node.label}</span>
                <span className={node.framed ? "font-mono text-caption" : "text-[15px]"}>
                  {node.value}
                </span>
                <span className="font-mono text-micro text-fg-faint">{node.meta}</span>
              </div>
            </div>
          ))}
        </div>

        <ul className="mt-11 grid gap-x-8 sm:grid-cols-2 lg:grid-cols-3">
          {REFUSED.map((item) => (
            <li key={item} className="flex gap-3 border-t border-line py-4">
              <span className="shrink-0 font-mono text-meta text-fg-faint">refused</span>
              <span className="text-sm leading-[1.5] text-fg-secondary">{item}</span>
            </li>
          ))}
        </ul>
      </Section>

      {/* ----------------------------------------------------------- capability */}
      <Section id="capability">
        <SectionHeading
          className="mb-11"
          eyebrow="Capability matrix"
          title="Shipped, partial, and not yet."
          lead="0.1.0 is a foundation. This table is the whole truth about it — roadmap rows are not sold on any plan, and the dashboard disables their controls with a reason."
          actions={
            <ul className="nr-label flex flex-col gap-2">
              <li className="flex items-center gap-2">
                <span aria-hidden className="size-3.5 rounded-xs border border-line-default" />
                available
              </li>
              <li className="flex items-center gap-2">
                <span aria-hidden className="nr-hatch size-3.5 rounded-xs border border-line-strong" />
                partial
              </li>
              <li className="flex items-center gap-2">
                <span aria-hidden className="size-3.5 rounded-xs border border-dashed border-line-strong" />
                roadmap
              </li>
            </ul>
          }
        />

        <div className="flex flex-col">
          <div className="nr-label grid grid-cols-[minmax(0,248px)_minmax(0,1fr)_112px] items-center gap-6 border-b border-line-default py-3 max-md:hidden">
            <span>Capability</span>
            <span>Detail</span>
            <span className="text-right">State</span>
          </div>
          {CAPABILITIES.map((capability) => {
            const { variant, icon: Icon } = STATE_BADGE[capability.state];
            return (
              <div
                key={capability.name}
                className="grid items-center gap-2 border-b border-line py-4 md:grid-cols-[minmax(0,248px)_minmax(0,1fr)_112px] md:gap-6"
              >
                <span className="font-mono text-caption">{capability.name}</span>
                <span className="text-caption leading-[1.5] text-fg-muted">{capability.detail}</span>
                <span className="flex md:justify-end">
                  <Badge variant={variant}>
                    <Icon aria-hidden strokeWidth={1.5} />
                    {capability.state}
                  </Badge>
                </span>
              </div>
            );
          })}
        </div>
      </Section>

      {/* -------------------------------------------------------------- pricing */}
      <section
        id="pricing"
        data-theme="dark"
        className="relative overflow-hidden border-t border-line bg-surface-base text-fg"
      >
        <div aria-hidden className="nr-dither absolute inset-0 opacity-50" />
        <div className="relative mx-auto grid w-full max-w-(--nr-container-max) gap-14 px-6 py-20 sm:py-28 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)]">
          <div className="flex flex-col gap-5">
            <Eyebrow>Pricing</Eyebrow>
            <h2 className="text-[clamp(30px,3.1vw,46px)] leading-[1.04] tracking-display">
              Priced per GB-hour.
              <br />
              Never per seat.
            </h2>
            <p className="text-[17px] leading-[1.6] text-fg-secondary">
              An app is executed, not hosted. A GB-hour is memory reserved multiplied by wall time,
              counted from the moment a worker claims an execution to the moment it goes terminal.
              Queued time is free, because a queued execution is holding no worker. Builds are free.
              Plans differ by volume, retention and support, never by whether you can explain a
              failure.
            </p>

            <div className="mt-1 flex flex-col">
              <p className="nr-label pb-2.5">In every plan, including the smallest</p>
              {ALWAYS_INCLUDED.map((item, index) => (
                <span
                  key={item}
                  className="flex gap-3 border-t border-line py-2.5 text-caption leading-[1.5] text-fg-secondary last:border-b"
                >
                  <span className="font-mono text-meta text-fg-faint">
                    {String(index + 1).padStart(2, "0")}
                  </span>
                  {item}
                </span>
              ))}
            </div>

            <div className="mt-2 flex flex-wrap gap-2.5">
              <Button asChild>
                <Link href="/register">
                  Start free
                  <ArrowRight aria-hidden strokeWidth={1.5} />
                </Link>
              </Button>
              <Button asChild variant="secondary">
                <Link href="/docs">
                  <BookOpen aria-hidden strokeWidth={1.5} />
                  Read the docs
                </Link>
              </Button>
            </div>
          </div>

          <div className="flex min-w-0 flex-col gap-3">
            <FirstCall />
            <Callout kind="warning" title="A 202 is not a finished execution">
              Acceptance means the execution is queued and pinned to a build. Poll{" "}
              <code>GET /v1/executions/{"{execution_id}"}</code> until <code>status</code> is{" "}
              <code>succeeded</code> or <code>failed</code>; only then are <code>output</code>,{" "}
              <code>logs</code> and <code>failure</code> settled.
            </Callout>
          </div>
        </div>
      </section>

      {/* ---------------------------------------------------------------- plans */}
      <Section id="plans">
        <SectionHeading
          className="mb-9"
          eyebrow="Plans"
          title="Volume, retention, support."
          lead={`That is the whole difference between these three columns. Compute is metered in ${UNIT}s and itemised per project, so an invoice names the app that spent the money.`}
        />

        <PlanGrid />

        <div className="mt-3.5 flex flex-wrap items-center gap-4.5 rounded-md border border-line bg-surface-sunken px-4 py-3.5 font-mono text-meta text-fg-secondary">
          <span className="nr-label">overage</span>
          <span>$0.06 per additional {UNIT}</span>
          <span aria-hidden className="h-3.5 w-px bg-line" />
          <span className="nr-label">billed</span>
          <span>monthly in arrears, itemised per project</span>
          <span className="hidden flex-1 lg:block" />
          <span className="text-fg-muted">
            a hard cap is available if you would rather fail closed than be billed
          </span>
        </div>

        <div className="mt-14 overflow-x-auto">
          <div className="min-w-[640px]">
            <div className="nr-label grid grid-cols-[minmax(0,1.5fr)_repeat(3,minmax(128px,1fr))] items-center gap-6 border-b border-line-default py-3">
              <span />
              <span>Solo</span>
              <span>Team</span>
              <span>Enterprise</span>
            </div>
            {COMPARISON.map((row) => (
              <div
                key={row.label}
                className="grid grid-cols-[minmax(0,1.5fr)_repeat(3,minmax(128px,1fr))] items-center gap-6 border-b border-line py-3.5"
              >
                <span className="text-sm">{row.label}</span>
                {row.values.map((value, index) => (
                  <span
                    key={index}
                    className={
                      value === "—" || value === "roadmap" || value === "community"
                        ? "font-mono text-caption text-fg-muted"
                        : "font-mono text-caption"
                    }
                  >
                    {value}
                  </span>
                ))}
              </div>
            ))}
          </div>
        </div>

        <div className="mt-14 grid gap-x-10 sm:grid-cols-2">
          {FAQ.map((item) => (
            <div key={item.q} className="flex flex-col gap-2 border-t border-line py-5">
              <h3 className="text-[15px] font-medium">{item.q}</h3>
              <p className="text-sm leading-[1.55] text-fg-secondary">{item.a}</p>
            </div>
          ))}
        </div>

        <Callout kind="roadmap" title="Roadmap rows are not sold on any plan" className="mt-9 max-w-[820px]">
          A capability marked roadmap is unbuilt, so it is not bundled, discounted or promised
          against a date. Runners are the next one: a server that holds one app resident and exposes
          an endpoint, metered by the time it is up rather than per execution. Each roadmap route in
          the dashboard names the contracts it is still waiting on.
        </Callout>
      </Section>

      {/* ---------------------------------------------------------------- cards */}
      <Section>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {[
            {
              eyebrow: "10 minutes",
              title: "Quickstart",
              body: "Issue a key, create an app, deploy source and watch an execution reach a terminal state.",
              meta: "docs/quickstart",
              href: "/docs/quickstart",
            },
            {
              eyebrow: "Operate",
              title: "Operator dashboard",
              body: "Projects, apps, deployments, builds and executions — every route in one clickable surface.",
              meta: "dashboard/executions",
              href: "/overview",
            },
            {
              eyebrow: "Contract",
              title: "OpenAPI 3.1",
              body: "The transport authority. Generate your client from it and fail CI on contract drift.",
              meta: "api/openapi.yaml",
              href: "/docs/errors",
            },
            {
              eyebrow: "Free credit",
              title: "Start free",
              body: "Credit on the account from the first execution. Deploy something, break it on purpose, read what it left behind.",
              meta: "account/credit",
              href: "/register",
            },
          ].map((card) => (
            <Link
              key={card.title}
              href={card.href}
              className="group flex flex-col gap-2.5 rounded-lg border border-line p-5 transition-colors duration-120 ease-mech hover:border-line-default hover:bg-surface-panel"
            >
              <span className="nr-label">{card.eyebrow}</span>
              <span className="text-lg font-medium tracking-title">{card.title}</span>
              <span className="flex-1 text-caption leading-[1.55] text-fg-muted">{card.body}</span>
              <span className="flex items-center gap-1.5 font-mono text-micro text-fg-faint">
                {card.meta}
                <ArrowRight
                  aria-hidden
                  className="size-3 transition-transform duration-120 ease-mech group-hover:translate-x-0.5"
                  strokeWidth={1.5}
                />
              </span>
            </Link>
          ))}
        </div>
      </Section>
    </>
  );
}

/** Execution table rows: hairline between, and the last one inverted-marked. */
function cnRow(last: boolean) {
  return [
    "grid grid-cols-[64px_minmax(0,1fr)_92px_72px] items-center gap-3 px-3.5 py-2",
    last ? "bg-surface-raised shadow-[inset_2px_0_0_var(--nr-accent)]" : "border-b border-line",
  ].join(" ");
}
