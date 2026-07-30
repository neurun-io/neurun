"use client";

import { Panel } from "@/components/neurun/panel";
import { StatusBadge } from "@/components/neurun/status-badge";
import { Timestamp } from "@/components/neurun/timestamp";
import { CopyId } from "@/components/neurun/copy-id";
import { KeyValue } from "@/components/neurun/key-value";
import { JsonView } from "@/components/neurun/json-view";
import { EmptyState } from "@/components/neurun/feedback";
import { parseInstant } from "@/lib/view/time";
import { formatDurationMs } from "@/lib/view/units";
import type { JobAttempt } from "@/lib/api/types";

/**
 * Attempts, in creation order.
 *
 * Each attempt is an immutable execution record: which agent held the lease,
 * when the lease expires, how long the run took, and the trace it emitted.
 * Retries are first-class objects here, not log lines.
 */
export function AttemptList({ attempts }: { attempts: JobAttempt[] }) {
  if (attempts.length === 0) {
    return (
      <Panel label="Attempts">
        <EmptyState
          title="No attempts yet"
          description="An attempt appears once an agent leases this job."
        />
      </Panel>
    );
  }

  return (
    <div className="space-y-3">
      {attempts.map((attempt) => (
        <AttemptCard key={attempt.id} attempt={attempt} total={attempts.length} />
      ))}
    </div>
  );
}

function AttemptCard({ attempt, total }: { attempt: JobAttempt; total: number }) {
  const started = parseInstant(attempt.started_at);
  const finished = parseInstant(attempt.finished_at);
  const runDuration =
    started && finished ? formatDurationMs(finished.getTime() - started.getTime()) : null;

  return (
    <Panel
      label={`Attempt ${attempt.number}/${total}`}
      actions={<StatusBadge status={attempt.state} />}
    >
      <div className="space-y-3">
        <KeyValue
          columns={2}
          rows={[
            { label: "Attempt ID", value: <CopyId value={attempt.id} label="attempt ID" truncate /> },
            { label: "Agent", value: <CopyId value={attempt.agent_id} label="agent ID" truncate /> },
            { label: "Created", value: <Timestamp value={attempt.created_at} /> },
            { label: "Started", value: <Timestamp value={attempt.started_at} /> },
            { label: "Finished", value: <Timestamp value={attempt.finished_at} /> },
            { label: "Run duration", value: runDuration },
            { label: "Lease expires", value: <Timestamp value={attempt.lease_expires_at} /> },
            { label: "Fence", value: String(attempt.fence) },
            {
              label: "Trace ID",
              value: attempt.trace_id ? (
                <CopyId value={attempt.trace_id} label="trace ID" truncate />
              ) : null,
            },
          ]}
        />

        {attempt.failure ? (
          <section>
            <p className="nr-label mb-1.5">Failure</p>
            <JsonView value={attempt.failure} preRedacted />
          </section>
        ) : null}

        {attempt.retry ? (
          <section>
            <p className="nr-label mb-1.5">Retry decision</p>
            <JsonView value={attempt.retry} preRedacted />
          </section>
        ) : null}

        {attempt.result !== undefined && attempt.result !== null ? (
          <section>
            <p className="nr-label mb-1.5">Result</p>
            <JsonView value={attempt.result} downloadName={`${attempt.id}-result.json`} />
          </section>
        ) : null}
      </div>
    </Panel>
  );
}
