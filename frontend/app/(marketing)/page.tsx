import type { Metadata } from "next";
import Link from "next/link";
import { ArrowRight, BookOpen, Check, GitBranch, TriangleAlert } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Callout } from "@/components/neurun/feedback";
import { Logo } from "@/components/neurun/logo";
import { Eyebrow, Section, SectionHeading } from "@/components/marketing/parts";
import { RuntimeField } from "@/components/marketing/runtime-field";
import { PlanGrid } from "@/components/marketing/pricing";
import { CAPABILITIES, REFUSED, type CapabilityState } from "@/lib/marketing/content";
import { ALWAYS_INCLUDED, UNIT } from "@/lib/marketing/plans";

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
                  <Link href="/auth">
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
              Web scraping and stealth browsers are not difficult, it just lacks the environment to support it.
            </p>
            <p className="text-[17px] leading-[1.6] text-fg-secondary">
              Our AI systems and kernels contantly evaluate your automation processes both in build and in runtime. We something breaks, we fix it.
            </p>
          </div>

          <div className="flex min-w-0 flex-col gap-3">
            <RuntimeField />
          </div>
        </div>
      </Section>

      {/* ------------------------------------------------------------- boundary */}
      <Section id="boundary">
        <SectionHeading
          className="mb-13"
          eyebrow="Boundary"
          title="We solve stealth."
          lead={
            <>
              Primed AI agent that contantly evolves specifically to your systems and targets.
              Contantly updated kernels that observe your scraping and browser workloads at OS and code level.
            </>
          }
        />

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
              Axons priced per GB-hour.
              <br />
              Never per seat.
            </h2>
            <p className="text-[17px] leading-[1.6] text-fg-secondary">
              An axon is one automation, deployed and run. A GB-hour is a gigabyte of memory held
              for an hour — you pay for what your app holds while it works, and nothing while it
              waits.
            </p>

            <div className="mt-2 flex flex-wrap gap-2.5">
              <Button asChild>
                <Link href="/auth">
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
          <span className="nr-label">Charges per minute</span>
        </div>
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
    </>
  );
}
