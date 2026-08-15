"use client";

import Link from "next/link";
import { RotateCcw } from "lucide-react";
import { useParams, useRouter } from "next/navigation";
import { toast } from "sonner";

import { BuildLog } from "@/components/neurun/build-log";
import { ErrorPanel } from "@/components/neurun/error-panel";
import { KeyValue } from "@/components/neurun/key-value";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { StatusBadge } from "@/components/neurun/status-badge";
import { Timestamp } from "@/components/neurun/timestamp";
import { Button } from "@/components/ui/button";
import { useDeploymentQuery, useRetryDeploymentMutation } from "@/lib/api/queries";
import { isDeploymentRunning } from "@/lib/api/resource-types";

export default function DeploymentPage() {
  const { deploymentId } = useParams<{ deploymentId: string }>();
  const query = useDeploymentQuery(deploymentId);
  const retry = useRetryDeploymentMutation();
  const router = useRouter();

  if (query.isError) {
    return (
      <div className="p-6">
        <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
      </div>
    );
  }
  if (!query.data) return <p className="p-6 text-fg-muted">Loading…</p>;

  const deployment = query.data;
  const running = isDeploymentRunning(deployment.status);

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
        meta={
          <>
            <span className="nr-label">
              Started <Timestamp value={deployment.started_at} />
            </span>
            {deployment.finished_at ? (
              <span className="nr-label">
                Finished <Timestamp value={deployment.finished_at} />
              </span>
            ) : null}
          </>
        }
        actions={
          <Button
            size="sm"
            variant="secondary"
            disabled={running || retry.isPending}
            onClick={() =>
              retry.mutate(deployment.id, {
                onSuccess: (queued) => {
                  toast.success(`Queued ${queued.id}`);
                  router.push(`/deployments/${queued.id}`);
                },
              })
            }
          >
            <RotateCcw aria-hidden className="size-3.5" strokeWidth={1.5} />
            {retry.isPending ? "Queuing…" : "Retry"}
          </Button>
        }
      />
      <div className="space-y-4 p-6">
        {retry.isError ? <ErrorPanel error={retry.error} title="Retry failed" /> : null}
        <div className="grid gap-4 lg:grid-cols-3">
          <Panel label="Deployment" className="lg:col-span-1">
            <KeyValue
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
                { label: "Commit", value: deployment.commit_sha?.slice(0, 12) ?? "—" },
                { label: "Ref", value: deployment.git_ref ?? "—" },
              ]}
            />
          </Panel>
          <Panel label={deployment.failure ? "Failure" : "Build"} className="lg:col-span-2">
            {deployment.build ? (
              <div className="space-y-2">
                <Link className="font-mono underline" href={`/builds/${deployment.build.id}`}>
                  {deployment.build.id}
                </Link>
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
              <div className="space-y-1">
                <p className="font-mono text-micro tracking-wide text-fg-muted uppercase">
                  {deployment.failure.code}
                </p>
                <p className="text-sm text-fg-secondary">{deployment.failure.message}</p>
              </div>
            ) : (
              <p className="text-fg-muted">Nothing built yet.</p>
            )}
          </Panel>
        </div>
        <Panel label="Log">
          <BuildLog logs={deployment.logs} following={running} />
        </Panel>
      </div>
    </div>
  );
}
