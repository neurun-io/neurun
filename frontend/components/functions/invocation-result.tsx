"use client";

import Link from "next/link";
import { Check, X } from "lucide-react";

import { Panel } from "@/components/neurun/panel";
import { StatusBadge } from "@/components/neurun/status-badge";
import { CopyId, Digest } from "@/components/neurun/copy-id";
import { KeyValue } from "@/components/neurun/key-value";
import { JsonView } from "@/components/neurun/json-view";
import { Timestamp } from "@/components/neurun/timestamp";
import { Callout } from "@/components/neurun/feedback";
import { formatBytesExact, formatCpuSeconds, formatDurationMs } from "@/lib/view/units";
import type { Invocation } from "@/lib/api/types";

/**
 * Invocation evidence.
 *
 * The distinction this panel exists to make: transport succeeding and the data
 * being valid are two different outcomes. A `succeeded` invocation whose
 * `output_schema_valid` is false transported fine and produced data the schema
 * refused — that has to be visible, not buried.
 */
export function InvocationResult({ invocation }: { invocation: Invocation }) {
  const schemaValid = invocation.output_schema_valid;

  return (
    <div className="space-y-4">
      {!schemaValid ? (
        <Callout kind="warning" title="Output failed schema validation">
          Transport completed, but the returned data did not match the function&apos;s published
          output schema. This is a validation rejection, not a transport failure.
        </Callout>
      ) : null}

      <Panel
        label="Invocation"
        actions={<StatusBadge status={invocation.status} />}
        footer={
          <Link
            href={`/invocations/${invocation.invocation_id}`}
            className="underline underline-offset-3"
          >
            Open full invocation evidence →
          </Link>
        }
      >
        <KeyValue
          columns={2}
          rows={[
            {
              label: "Invocation ID",
              value: <CopyId value={invocation.invocation_id} label="invocation ID" truncate />,
            },
            {
              label: "Function",
              value: `${invocation.function.name}@${invocation.function.version}`,
            },
            { label: "Digest", value: <Digest value={invocation.function.digest} /> },
            {
              label: "Output schema",
              value: (
                <span className="inline-flex items-center gap-1">
                  {schemaValid ? (
                    <Check aria-hidden className="size-3" strokeWidth={1.5} />
                  ) : (
                    <X aria-hidden className="size-3" strokeWidth={1.5} />
                  )}
                  {schemaValid ? "valid" : "rejected"}
                </span>
              ),
            },
            { label: "Side effects", value: invocation.side_effect_class },
            { label: "Created", value: <Timestamp value={invocation.created_at} /> },
            { label: "Started", value: <Timestamp value={invocation.started_at} /> },
            { label: "Finished", value: <Timestamp value={invocation.finished_at} /> },
          ]}
        />
      </Panel>

      {invocation.failure ? (
        <Panel label="Classified failure">
          <KeyValue
            rows={[
              { label: "Category", value: invocation.failure.category },
              { label: "Code", value: invocation.failure.code },
              {
                label: "Retryable",
                value: invocation.failure.retryable ? "yes" : "no",
                hint: "Server-owned classification. The dashboard never decides retryability.",
              },
            ]}
          />
          {invocation.failure.message ? (
            <p className="mt-3 border-t border-line pt-3 text-sm text-fg-secondary">
              {invocation.failure.message}
            </p>
          ) : null}
        </Panel>
      ) : null}

      <Panel label="Output">
        {invocation.output === undefined || invocation.output === null ? (
          <p className="font-mono text-caption text-fg-muted">No output recorded.</p>
        ) : (
          <JsonView
            value={invocation.output}
            downloadName={`${invocation.invocation_id}-output.json`}
          />
        )}
      </Panel>

      {invocation.redacted_input !== undefined ? (
        <Panel
          label="Input · server-redacted"
          footer="Redacted by the server according to the function manifest."
        >
          <JsonView value={invocation.redacted_input} preRedacted />
        </Panel>
      ) : null}

      <Panel label="Usage">
        <KeyValue
          columns={2}
          rows={[
            { label: "Duration", value: formatDurationMs(invocation.usage.duration_ms) },
            {
              label: "CPU",
              value:
                invocation.usage.cpu_seconds === undefined
                  ? null
                  : formatCpuSeconds(invocation.usage.cpu_seconds),
            },
            {
              label: "Peak RSS",
              value:
                invocation.usage.peak_rss_bytes === undefined
                  ? null
                  : formatBytesExact(invocation.usage.peak_rss_bytes),
            },
            {
              label: "Network",
              value:
                invocation.usage.network_bytes === undefined
                  ? null
                  : formatBytesExact(invocation.usage.network_bytes),
            },
            {
              label: "Artifacts",
              value:
                invocation.usage.artifact_bytes === undefined
                  ? null
                  : formatBytesExact(invocation.usage.artifact_bytes),
            },
          ]}
        />
      </Panel>

      <Panel label="Trace">
        <KeyValue
          rows={[
            { label: "Trace ID", value: <CopyId value={invocation.trace_id} label="trace ID" truncate /> },
            { label: "Span ID", value: <CopyId value={invocation.span_id} label="span ID" truncate /> },
            {
              label: "Job",
              value: invocation.context?.job_id ? (
                <Link
                  href={`/jobs/${invocation.context.job_id}`}
                  className="font-mono text-caption underline underline-offset-3"
                >
                  {invocation.context.job_id}
                </Link>
              ) : null,
            },
            { label: "Attempt", value: invocation.context?.attempt_id },
          ]}
        />
      </Panel>

      {invocation.artifacts && invocation.artifacts.length > 0 ? (
        <Panel
          label="Artifacts"
          footer="Artifact metadata only. Signed download endpoints are a future contract."
        >
          <JsonView value={invocation.artifacts} preRedacted />
        </Panel>
      ) : null}
    </div>
  );
}
