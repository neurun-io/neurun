"use client";

import { useState, type FormEvent } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { toast } from "sonner";

import { ErrorPanel, InlineError } from "@/components/neurun/error-panel";
import { JsonView } from "@/components/neurun/json-view";
import { KeyValue } from "@/components/neurun/key-value";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { StatusBadge } from "@/components/neurun/status-badge";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useCreateExecutionMutation, useDeploymentQuery } from "@/lib/api/queries";

export default function DeploymentPage() {
  const { deploymentId } = useParams<{ deploymentId: string }>();
  const query = useDeploymentQuery(deploymentId);
  const create = useCreateExecutionMutation();
  const router = useRouter();
  const [input, setInput] = useState("{}");
  const [parseError, setParseError] = useState<string | null>(null);

  if (query.isError) {
    return (
      <div className="p-6">
        <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
      </div>
    );
  }
  if (!query.data) return <p className="p-6 text-fg-muted">Loading…</p>;

  const deployment = query.data;
  function submit(event: FormEvent) {
    event.preventDefault();
    let parsed: unknown;
    try {
      parsed = JSON.parse(input);
      setParseError(null);
    } catch {
      setParseError("Input must be valid JSON.");
      return;
    }
    create.mutate(
      { deploymentId, input: parsed },
      {
        onSuccess: (execution) => {
          toast.success(`Queued ${execution.id}`);
          router.push(`/executions/${execution.id}`);
        },
      },
    );
  }

  return (
    <div>
      <PageHeader
        crumbs={[{ label: "Deployments", href: "/deployments" }, { label: deployment.id }]}
        title={
          <span className="flex items-center gap-3 font-mono">
            {deployment.id}
            <StatusBadge status={deployment.status} />
          </span>
        }
      />
      <div className="mx-auto max-w-4xl space-y-4 p-6">
        <Panel label="Deployment">
          <KeyValue
            columns={2}
            rows={[
              {
                label: "App",
                value: (
                  <Link className="underline" href={`/apps/${deployment.app_id}`}>
                    {deployment.app_id}
                  </Link>
                ),
              },
              {
                label: "Project",
                value: (
                  <Link className="underline" href={`/projects/${deployment.project_id}`}>
                    {deployment.project_id}
                  </Link>
                ),
              },
              { label: "Runtime", value: deployment.runtime },
              { label: "Entrypoint", value: deployment.entrypoint },
              { label: "Source SHA-256", value: deployment.source.sha256 },
            ]}
          />
        </Panel>
        <Panel label="Execute">
          <form onSubmit={submit} className="space-y-3">
            <div className="space-y-1.5">
              <Label htmlFor="execution-input">JSON input</Label>
              <Textarea
                id="execution-input"
                className="min-h-36 font-mono"
                value={input}
                onChange={(event) => setInput(event.target.value)}
              />
            </div>
            {parseError ? <p className="text-sm text-destructive">{parseError}</p> : null}
            {create.isError ? <InlineError error={create.error} /> : null}
            <Button disabled={deployment.status !== "ready" || create.isPending}>
              {create.isPending ? "Queuing…" : "Execute latest build"}
            </Button>
          </form>
        </Panel>
        <Panel label="Output">
          {deployment.build ? (
            <div className="space-y-2">
              <p className="font-mono text-sm">{deployment.build.id}</p>
              {deployment.build.artifacts.map((artifact) => (
                <div
                  key={artifact.id}
                  className="flex items-center justify-between border-b border-line py-2 font-mono text-sm last:border-0"
                >
                  <span>{artifact.name}</span>
                  <span className="text-fg-muted">{artifact.kind}</span>
                </div>
              ))}
            </div>
          ) : deployment.failure ? (
            <p className="text-sm text-fg-secondary">
              <span className="font-mono text-fg-muted">{deployment.failure.code}</span>{" "}
              {deployment.failure.message}
            </p>
          ) : (
            <p className="text-fg-muted">Nothing built yet.</p>
          )}
        </Panel>
        <Panel label="Log">
          {deployment.logs ? (
            <pre className="max-h-96 overflow-auto font-mono text-micro whitespace-pre-wrap text-fg-secondary">
              {deployment.logs}
            </pre>
          ) : (
            <p className="text-fg-muted">No output yet.</p>
          )}
        </Panel>
        <Panel label="Source metadata">
          <JsonView value={deployment.source} preRedacted />
        </Panel>
      </div>
    </div>
  );
}
