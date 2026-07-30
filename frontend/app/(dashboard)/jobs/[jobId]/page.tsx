"use client";

import { useState } from "react";
import { useParams } from "next/navigation";
import { Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { StatusBadge } from "@/components/neurun/status-badge";
import { StateFlow } from "@/components/neurun/state-flow";
import { Timestamp } from "@/components/neurun/timestamp";
import { CopyId, Digest } from "@/components/neurun/copy-id";
import { CopyRedactedJson, JsonView } from "@/components/neurun/json-view";
import { KeyValue } from "@/components/neurun/key-value";
import { EventTimeline } from "@/components/neurun/event-timeline";
import { Callout, EmptyState } from "@/components/neurun/feedback";
import { ErrorPanel } from "@/components/neurun/error-panel";
import { CancelJobDialog } from "@/components/jobs/cancel-job-dialog";
import { AttemptList } from "@/components/jobs/attempt-list";
import {
  useJobAttemptsQuery,
  useJobEventsQuery,
  useJobQuery,
} from "@/lib/api/queries";
import { isTerminalJobState } from "@/lib/api/types";
import { formatNanoseconds } from "@/lib/view/units";

export default function JobDetailPage() {
  const params = useParams<{ jobId: string }>();
  const jobId = params.jobId;

  const job = useJobQuery(jobId);
  const live = job.data ? !isTerminalJobState(job.data.state) : true;
  const events = useJobEventsQuery(jobId, live);
  const attempts = useJobAttemptsQuery(jobId, live);

  const [cancelOpen, setCancelOpen] = useState(false);

  if (job.isError) {
    return (
      <div className="px-6 py-6">
        <ErrorPanel error={job.error} onRetry={() => job.refetch()} />
      </div>
    );
  }

  if (job.isPending) {
    return (
      <div className="space-y-4 px-6 py-6" role="status" aria-label="Loading job">
        <Skeleton className="h-8 w-72" />
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }

  const data = job.data;
  const terminal = isTerminalJobState(data.state);
  const retryPolicy = data.request.retry_policy;

  return (
    <div className="flex min-h-full flex-col">
      <PageHeader
        crumbs={[{ label: "Jobs", href: "/jobs" }, { label: data.id }]}
        title={
          <span className="flex flex-wrap items-center gap-3">
            <span className="font-mono text-xl">{data.id}</span>
            <StatusBadge status={data.state} />
          </span>
        }
        meta={
          <>
            <StateFlow state={data.state} />
            {live ? (
              <span className="inline-flex items-center gap-1.5 font-mono text-micro text-fg-muted">
                <Loader2 aria-hidden className="size-3 animate-spin" strokeWidth={1.5} />
                polling every 2s
              </span>
            ) : null}
          </>
        }
        actions={
          <>
            <CopyRedactedJson value={data.request} label="Copy immutable request (redacted)" />
            <Button
              variant="destructive"
              size="sm"
              onClick={() => setCancelOpen(true)}
              disabled={terminal}
              aria-disabled={terminal || undefined}
              title={terminal ? "This job has already reached a terminal state." : undefined}
            >
              Cancel job
            </Button>
          </>
        }
      />

      <div className="grid min-w-0 flex-1 gap-4 px-6 py-4 xl:grid-cols-[minmax(0,1fr)_380px]">
        <div className="min-w-0 space-y-4">
          <Tabs defaultValue="payload">
            <TabsList>
              <TabsTrigger value="payload">Payload</TabsTrigger>
              <TabsTrigger value="attempts">
                Attempts
                <span className="ml-1.5 font-mono text-micro text-fg-muted">
                  {attempts.data?.length ?? 0}
                </span>
              </TabsTrigger>
              <TabsTrigger value="events">
                Events
                <span className="ml-1.5 font-mono text-micro text-fg-muted">
                  {events.data?.length ?? 0}
                </span>
              </TabsTrigger>
              <TabsTrigger value="raw">Raw</TabsTrigger>
            </TabsList>

            <TabsContent value="payload" className="mt-3 space-y-4">
              <Panel label="Input · immutable">
                <JsonView value={data.request.input} downloadName={`${data.id}-input.json`} />
              </Panel>

              <Panel label={terminal ? "Terminal result" : "Result"}>
                {data.result === undefined || data.result === null ? (
                  <p className="font-mono text-caption text-fg-muted">
                    {terminal
                      ? "This job reached a terminal state without a recorded result."
                      : "No result yet — the job has not completed."}
                  </p>
                ) : (
                  <JsonView value={data.result} downloadName={`${data.id}-result.json`} />
                )}
              </Panel>

              {data.last_failure ? (
                <Panel label="Classified failure">
                  <JsonView value={data.last_failure} preRedacted />
                </Panel>
              ) : null}

              {data.last_retry ? (
                <Panel
                  label="Persisted retry decision"
                  footer="Retry policy is owned by the server. The dashboard displays it and never computes it."
                >
                  <JsonView value={data.last_retry} preRedacted />
                </Panel>
              ) : null}
            </TabsContent>

            <TabsContent value="attempts" className="mt-3">
              {attempts.isError ? (
                <ErrorPanel error={attempts.error} onRetry={() => attempts.refetch()} />
              ) : attempts.isPending ? (
                <Skeleton className="h-32 w-full" />
              ) : (
                <AttemptList attempts={attempts.data} />
              )}
            </TabsContent>

            <TabsContent value="events" className="mt-3">
              {events.isError ? (
                <ErrorPanel error={events.error} onRetry={() => events.refetch()} />
              ) : events.isPending ? (
                <Skeleton className="h-32 w-full" />
              ) : (
                <Panel
                  label="Append-only event stream"
                  footer="Ordered by job-local sequence. Nothing is coalesced or filtered."
                >
                  <EventTimeline events={events.data} />
                </Panel>
              )}
            </TabsContent>

            <TabsContent value="raw" className="mt-3">
              <Panel label="Job snapshot · raw JSON">
                <JsonView value={data} downloadName={`${data.id}.json`} />
              </Panel>
            </TabsContent>
          </Tabs>
        </div>

        <aside className="min-w-0 space-y-4">
          <Panel label="Function">
            <KeyValue
              rows={[
                { label: "Name", value: data.request.function.name },
                { label: "Version", value: data.request.function.version },
                {
                  label: "Digest",
                  value: <Digest value={data.request.function.digest} />,
                },
              ]}
            />
          </Panel>

          <Panel label="Lifecycle">
            <KeyValue
              rows={[
                { label: "State", value: <StatusBadge status={data.state} /> },
                {
                  label: "Attempts",
                  value: `${data.attempt_count}/${data.max_attempts}`,
                },
                { label: "Created", value: <Timestamp value={data.created_at} /> },
                { label: "Updated", value: <Timestamp value={data.updated_at} /> },
                { label: "Completed", value: <Timestamp value={data.completed_at} /> },
                { label: "Canceled", value: <Timestamp value={data.canceled_at} /> },
                { label: "Next attempt", value: <Timestamp value={data.next_attempt_at} /> },
              ]}
            />
          </Panel>

          {retryPolicy ? (
            <Panel
              label="Retry policy"
              footer="Backoff is a Go duration in nanoseconds; converted for display."
            >
              <KeyValue
                rows={[
                  {
                    label: "Initial backoff",
                    value: formatNanoseconds(retryPolicy.initial_backoff),
                    hint: `${retryPolicy.initial_backoff.toLocaleString("en-US")} ns`,
                  },
                  {
                    label: "Max backoff",
                    value: formatNanoseconds(retryPolicy.max_backoff),
                    hint: `${retryPolicy.max_backoff.toLocaleString("en-US")} ns`,
                  },
                ]}
              />
            </Panel>
          ) : null}

          <Panel label="Identifiers">
            <KeyValue
              rows={[
                { label: "Job ID", value: <CopyId value={data.id} label="job ID" truncate /> },
                {
                  label: "Project",
                  value: <CopyId value={data.project_id} label="project ID" truncate />,
                },
                {
                  label: "Current attempt",
                  value: data.current_attempt_id ? (
                    <CopyId value={data.current_attempt_id} label="attempt ID" truncate />
                  ) : null,
                },
                {
                  label: "Terminal attempt",
                  value: data.terminal_attempt_id ? (
                    <CopyId value={data.terminal_attempt_id} label="attempt ID" truncate />
                  ) : null,
                },
                { label: "Snapshot version", value: String(data.version) },
              ]}
            />
          </Panel>

          <Callout kind="roadmap" title="Manual retry">
            Retry is server-owned in this release. `next_attempt_at`, failure retryability and the
            persisted retry decision are shown above; a confirmed manual retry needs{" "}
            <code className="font-mono text-micro">POST /v1/jobs/&#123;job_id&#125;/retry</code>.
          </Callout>
        </aside>
      </div>

      <CancelJobDialog job={data} open={cancelOpen} onOpenChange={setCancelOpen} />
    </div>
  );
}

/** Exported for the empty-attempt case in tests and storybook-style review. */
export function NoAttempts() {
  return (
    <EmptyState
      title="No attempts yet"
      description="An attempt appears once an agent leases this job."
    />
  );
}
