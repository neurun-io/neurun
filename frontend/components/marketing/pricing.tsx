"use client";

import Link from "next/link";
import { useState } from "react";
import { ArrowRight } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Panel } from "@/components/neurun/panel";
import { Emphasised } from "./parts";
import { PLANS, type Cycle } from "@/lib/marketing/plans";
import { cn } from "@/lib/utils";

/** A two-or-three option segmented control, 28px high, 4px radius. */
function Segmented<T extends string>({
  options,
  value,
  onChange,
  label,
}: {
  options: { id: T; label: string }[];
  value: T;
  onChange: (id: T) => void;
  label: string;
}) {
  return (
    <div
      role="tablist"
      aria-label={label}
      className="inline-flex h-7 items-center gap-0.5 rounded-md border border-line-default bg-surface-sunken p-0.5"
    >
      {options.map((option) => (
        <button
          key={option.id}
          type="button"
          role="tab"
          aria-selected={value === option.id}
          onClick={() => onChange(option.id)}
          className={cn(
            "h-6 rounded-xs px-2.5 font-mono text-micro transition-colors duration-120 ease-mech",
            value === option.id
              ? "bg-surface-inverse text-fg-inverse"
              : "text-fg-muted hover:text-fg",
          )}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}

export function PlanGrid() {
  const [cycle, setCycle] = useState<Cycle>("monthly");
  const period = cycle === "annual" ? "/ year" : "/ month";

  return (
    <>
      <div className="mb-9 flex flex-col items-start gap-2">
        <Segmented
          label="Billing cycle"
          value={cycle}
          onChange={setCycle}
          options={[
            { id: "monthly", label: "Monthly" },
            { id: "annual", label: "Annual" },
          ]}
        />
        <p className="nr-label">
          {cycle === "annual" ? "two months free · billed once a year" : "cancel any month"}
        </p>
      </div>

      <div className="grid items-stretch gap-3 lg:grid-cols-3">
        {PLANS.map((plan) => (
          <div
            key={plan.id}
            className={cn(
              "flex flex-col gap-5 rounded-lg border p-6",
              plan.featured
                ? "border-line-default bg-surface-panel shadow-[inset_2px_0_0_var(--nr-accent)]"
                : "border-line",
            )}
          >
            <div className="flex items-center gap-2">
              <span className="nr-label">{plan.name}</span>
              <Badge variant={plan.featured ? "default" : "outline"} className="ml-auto">
                {plan.tag}
              </Badge>
            </div>

            <div className="flex items-baseline gap-2">
              <span
                className={cn(
                  "leading-none font-medium tabular-nums",
                  plan.custom ? "text-4xl tracking-display" : "text-[52px] tracking-[-0.04em]",
                )}
              >
                {plan.price[cycle]}
              </span>
              {plan.custom ? null : (
                <span className="font-mono text-meta text-fg-muted">{period}</span>
              )}
            </div>

            <p className="text-sm leading-[1.6] text-fg-secondary">{plan.summary}</p>

            <ul className="flex flex-1 flex-col">
              {plan.features.map((feature) => (
                <li
                  key={feature}
                  className="flex gap-2.5 border-t border-line py-2.5 text-caption leading-[1.45] text-fg-secondary"
                >
                  <span aria-hidden className="font-mono text-fg-faint">
                    ·
                  </span>
                  <span>
                    <Emphasised text={feature} />
                  </span>
                </li>
              ))}
            </ul>

            <Button asChild variant={plan.featured ? "default" : "secondary"} className="w-full">
              <Link href={plan.href}>
                {plan.cta}
                <ArrowRight aria-hidden strokeWidth={1.5} />
              </Link>
            </Button>
          </div>
        ))}
      </div>
    </>
  );
}

const SNIPPETS = {
  deploy: `# 1 · create the app, once
curl -sS -X POST api.neurun.dev/v1/apps \\
  -H "authorization: Bearer $NEURUN_KEY" \\
  -d '{"name":"pricing-crawler"}'

# 2 · deploy source to it
curl -sS -X POST api.neurun.dev/v1/deployments \\
  -H "authorization: Bearer $NEURUN_KEY" \\
  -F app_id=app_7QK2M0X4 \\
  -F runtime=python \\
  -F entrypoint=main.py:handler \\
  -F source=@dist.zip`,
  execute: `# executions are pinned to the ready build
curl -sS -X POST \\
  api.neurun.dev/v1/deployments/dep_01HXQ8F2K9/executions \\
  -H "authorization: Bearer $NEURUN_KEY" \\
  -d '{"input":{"url":"https://example.com"}}'

HTTP/1.1 202 Accepted
{"id":"exe_01HXQ8F2M4","status":"queued",
 "build_id":"bld_9F3AC41"}`,
  vpc: `# enterprise: licensed container in your VPC
docker login registry.neurun.dev

docker run --rm -p 8080:8080 \\
  -e NEURUN_LICENSE_KEY=$NEURUN_LICENSE \\
  -e DATABASE_URL=$POSTGRES_URL \\
  registry.neurun.dev/neurun:0.1.0`,
} as const;

type SnippetId = keyof typeof SNIPPETS;

export function FirstCall() {
  const [tab, setTab] = useState<SnippetId>("deploy");

  return (
    <Panel
      flush
      label="First call"
      actions={
        <Segmented
          label="Example"
          value={tab}
          onChange={setTab}
          options={[
            { id: "deploy", label: "deploy" },
            { id: "execute", label: "execute" },
            { id: "vpc", label: "self-managed" },
          ]}
        />
      }
      footer="every sample is a real /v1 call — generate your client from the OpenAPI document"
    >
      <pre className="overflow-x-auto p-4 font-mono text-meta leading-[1.6] text-(--nr-code-string)">
        <code>{SNIPPETS[tab]}</code>
      </pre>
    </Panel>
  );
}
