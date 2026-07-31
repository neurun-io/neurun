"use client";

import Link from "next/link";
import { useParams } from "next/navigation";

import { ErrorPanel } from "@/components/neurun/error-panel";
import { JsonView } from "@/components/neurun/json-view";
import { KeyValue } from "@/components/neurun/key-value";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { StatusBadge } from "@/components/neurun/status-badge";
import { Timestamp } from "@/components/neurun/timestamp";
import { useBuildQuery } from "@/lib/api/queries";

export default function BuildPage() {
  const { buildId } = useParams<{ buildId: string }>();
  const query = useBuildQuery(buildId);
  if (query.isError) {
    return (
      <div className="p-6">
        <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
      </div>
    );
  }
  if (!query.data) return <p className="p-6 text-fg-muted">Loading…</p>;

  const build = query.data;
  return (
    <div>
      <PageHeader
        crumbs={[{ label: "Builds", href: "/builds" }, { label: build.id }]}
        title={
          <span className="flex items-center gap-3 font-mono">
            {build.id}
            <StatusBadge status={build.status} />
          </span>
        }
      />
      <div className="mx-auto max-w-4xl space-y-4 p-6">
        <Panel label="Build">
          <KeyValue
            columns={2}
            rows={[
              { label: "Number", value: `#${build.number}` },
              {
                label: "Deployment",
                value: (
                  <Link className="underline" href={`/deployments/${build.deployment_id}`}>
                    {build.deployment_id}
                  </Link>
                ),
              },
              { label: "Runtime", value: build.runtime },
              { label: "Entrypoint", value: build.entrypoint },
              { label: "Source SHA-256", value: build.source_sha256 },
              { label: "Started", value: <Timestamp value={build.started_at} /> },
              { label: "Finished", value: <Timestamp value={build.finished_at} /> },
            ]}
          />
        </Panel>
        {build.failure ? (
          <Panel label="Failure">
            <p className="font-mono">{build.failure.code}</p>
            <p>{build.failure.message}</p>
          </Panel>
        ) : null}
        <Panel label="Artifacts">
          <JsonView value={build.artifacts} preRedacted />
        </Panel>
      </div>
    </div>
  );
}
