"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { toast } from "sonner";

import { ErrorPanel, InlineError } from "@/components/neurun/error-panel";
import { JsonView } from "@/components/neurun/json-view";
import { KeyValue } from "@/components/neurun/key-value";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { StatusBadge } from "@/components/neurun/status-badge";
import { Timestamp } from "@/components/neurun/timestamp";
import { Button } from "@/components/ui/button";
import { useExecutionQuery, useRerunExecutionMutation } from "@/lib/api/queries";
import { isTerminalExecutionStatus } from "@/lib/api/resource-types";

export default function ExecutionPage() {
  const { executionId } = useParams<{ executionId: string }>();
  const query = useExecutionQuery(executionId);
  const rerun = useRerunExecutionMutation();
  const router = useRouter();

  if (query.isError) {
    return (
      <div className="p-6">
        <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
      </div>
    );
  }
  if (!query.data) return <p className="p-6 text-fg-muted">Loading…</p>;

  const execution = query.data;
  function repeat() {
    rerun.mutate(execution.id, {
      onSuccess: (next) => {
        toast.success(`Queued ${next.id}`);
        router.push(`/executions/${next.id}`);
      },
    });
  }

  return (
    <div>
      <PageHeader
        crumbs={[{ label: "Executions", href: "/executions" }, { label: execution.id }]}
        title={
          <span className="flex items-center gap-3 font-mono">
            {execution.id}
            <StatusBadge status={execution.status} />
          </span>
        }
        actions={
          <Button
            onClick={repeat}
            disabled={!isTerminalExecutionStatus(execution.status) || rerun.isPending}
          >
            Rerun exact build
          </Button>
        }
      />
      <div className="mx-auto max-w-4xl space-y-4 p-6">
        {rerun.isError ? <InlineError error={rerun.error} /> : null}
        <Panel label="Execution">
          <KeyValue
            columns={2}
            rows={[
              {
                label: "App",
                value: (
                  <Link className="underline" href={`/apps/${execution.app_id}`}>
                    {execution.app_id}
                  </Link>
                ),
              },
              {
                label: "Build",
                value: (
                  <Link className="underline" href={`/builds/${execution.build_id}`}>
                    {execution.build_id}
                  </Link>
                ),
              },
              { label: "Created", value: <Timestamp value={execution.created_at} /> },
              { label: "Started", value: <Timestamp value={execution.started_at} /> },
              { label: "Finished", value: <Timestamp value={execution.finished_at} /> },
              { label: "Rerun of", value: execution.rerun_of_execution_id },
            ]}
          />
        </Panel>
        {execution.failure ? (
          <Panel label="Failure">
            <p className="font-mono">{execution.failure.code}</p>
            <p>{execution.failure.message}</p>
          </Panel>
        ) : null}
        <Panel label="Input">
          <JsonView value={execution.input} preRedacted />
        </Panel>
        <Panel label="Output">
          {execution.output === undefined ? (
            <p className="font-mono text-fg-muted">No output recorded.</p>
          ) : (
            <JsonView value={execution.output} preRedacted />
          )}
        </Panel>
        <Panel label="Logs">
          <pre className="max-h-96 overflow-auto whitespace-pre-wrap font-mono text-caption">
            {execution.logs || "No logs recorded."}
          </pre>
        </Panel>
      </div>
    </div>
  );
}
