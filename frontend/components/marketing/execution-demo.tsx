"use client";

import { useEffect, useState, useSyncExternalStore } from "react";

import { Panel } from "@/components/neurun/panel";
import { KeyValue } from "@/components/neurun/key-value";
import { cn } from "@/lib/utils";
import { NO_VALUE } from "@/lib/view/units";

/**
 * A run, rendered by the real interface rather than photographed.
 *
 * The system ships no product shots on purpose: where a screenshot would sit,
 * the page shows the thing itself. The stages and fields are the ones the
 * Execution schema actually carries — `queued → running → succeeded`, with
 * `build_id` as the provenance that makes a rerun mean anything.
 */
const STAGES = ["queued", "running", "succeeded"] as const;

const EVENTS = [
  { label: "execution.accepted", at: "12:04:19.221", note: "202" },
  { label: "execution.claimed", at: "12:04:19.310", note: "wrk_02" },
  { label: "execution.running", at: "12:04:19.402", note: "bld_9F3AC41" },
  { label: "execution.succeeded", at: "12:04:19.814", note: "412ms" },
];

const REQUEST = `curl -sS -X POST \\
  api.neurun.dev/v1/deployments/dep_01HXQ8F2K9/executions \\
  -H "authorization: Bearer $NEURUN_KEY" \\
  -d '{
    "input": { "url": "https://example.com" }
  }'`;

const MOTION_QUERY = "(prefers-reduced-motion: reduce)";

function subscribeMotion(onChange: () => void) {
  const media = window.matchMedia(MOTION_QUERY);
  media.addEventListener("change", onChange);
  return () => media.removeEventListener("change", onChange);
}

/** Tick → phase. Roughly 3.8s of run, then back to queued. */
function phaseFor(tick: number) {
  if (tick < 10) return { step: 0, ms: 0 };
  if (tick < 42) return { step: 1, ms: Math.round((tick - 10) * 12.9) };
  return { step: 2, ms: 412 };
}

export function ExecutionDemo() {
  const [tick, setTick] = useState(0);
  const reduced = useSyncExternalStore(
    subscribeMotion,
    () => window.matchMedia(MOTION_QUERY).matches,
    () => false,
  );

  useEffect(() => {
    if (reduced) return;
    const timer = setInterval(() => setTick((value) => (value >= 88 ? 0 : value + 1)), 90);
    return () => clearInterval(timer);
  }, [reduced]);

  // Reduced motion gets the settled run rather than a frozen mid-state: the
  // panel still has to read as a finished execution.
  const { step, ms } = reduced ? { step: 2, ms: 412 } : phaseFor(tick);
  const done = step === 2;
  const visibleEvents = EVENTS.slice(0, step === 0 ? 1 : step === 1 ? 3 : 4);

  return (
    <Panel
      flush
      label="Run"
      actions={
        <span className="font-mono text-micro text-fg-muted">
          exe_01HXQ8F2M4 · pricing-crawler · build 4
        </span>
      }
      footer={
        done
          ? "succeeded · 412ms · build bld_9F3AC41 · 40 records in output"
          : "polling every 2s · pinned to the ready build at accept time"
      }
    >
      <div className="border-b border-line px-4 py-3">
        <ol className="flex flex-wrap items-center gap-1" aria-label={`Execution — ${STAGES[step]}`}>
          {STAGES.map((stage, index) => (
            <li key={stage} className="flex items-center gap-1">
              {index > 0 ? (
                <span aria-hidden className="text-fg-faint">
                  →
                </span>
              ) : null}
              <span
                aria-current={index === step ? "step" : undefined}
                className={cn(
                  "rounded-xs border px-1.5 py-0.5 font-mono text-micro whitespace-nowrap",
                  index === step && "border-line-inverse text-fg",
                  index < step && "border-line text-fg-muted",
                  index > step && "border-dashed border-line text-fg-faint",
                )}
              >
                {stage}
              </span>
            </li>
          ))}
        </ol>
      </div>

      <div className="grid lg:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_minmax(0,1fr)]">
        <Column title="Request" meta="POST · executions" className="lg:border-r">
          <pre className="overflow-x-auto p-3 font-mono text-micro leading-[1.6] text-(--nr-code-string)">
            <code>{REQUEST}</code>
          </pre>
        </Column>

        <Column title="Execution" meta={STAGES[step]} className="lg:border-r">
          <KeyValue
            className="px-3 py-2"
            rows={[
              { label: "id", value: "exe_01HXQ8F2M4" },
              { label: "app", value: "pricing-crawler" },
              { label: "build_id", value: "bld_9F3AC41" },
              { label: "status", value: STAGES[step] },
              { label: "elapsed", value: step >= 1 ? `${ms}ms` : NO_VALUE },
              { label: "output", value: done ? "40 records" : NO_VALUE },
              { label: "compute", value: done ? "0.41 GB-s" : NO_VALUE },
            ]}
          />
        </Column>

        <Column title="Logs" meta="append-only">
          <ol className="flex flex-col gap-2 p-3">
            {visibleEvents.map((event, index) => (
              <li key={event.label} className="flex min-w-0 items-baseline gap-2">
                <span
                  aria-hidden
                  className={cn(
                    "mt-1.5 size-1.5 shrink-0 rounded-full",
                    index === visibleEvents.length - 1 && !done
                      ? "bg-(--nr-accent) motion-safe:animate-pulse-node"
                      : "bg-fg-faint",
                  )}
                />
                <span className="min-w-0 flex-1 truncate font-mono text-micro text-fg-secondary">
                  {event.label}
                </span>
                <span className="shrink-0 font-mono text-micro text-fg-faint">{event.note}</span>
              </li>
            ))}
          </ol>
        </Column>
      </div>
    </Panel>
  );
}

function Column({
  title,
  meta,
  children,
  className,
}: {
  title: string;
  meta: string;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("min-w-0 border-line max-lg:border-b", className)}>
      <div className="flex h-8.5 items-center gap-2 border-b border-line px-3">
        <span className="nr-label">{title}</span>
        <span className="ml-auto truncate font-mono text-micro text-fg-muted">{meta}</span>
      </div>
      {children}
    </div>
  );
}
