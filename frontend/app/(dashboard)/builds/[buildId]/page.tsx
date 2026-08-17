"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { Check } from "lucide-react";
import { toast } from "sonner";

import { ErrorPanel } from "@/components/neurun/error-panel";
import { KeyValue } from "@/components/neurun/key-value";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { Timestamp } from "@/components/neurun/timestamp";
import { Button } from "@/components/ui/button";
import { useActivateBuildMutation, useAppQuery, useBuildQuery } from "@/lib/api/queries";

export default function BuildPage() {
  const { buildId } = useParams<{ buildId: string }>();
  const query = useBuildQuery(buildId);
  const appQuery = useAppQuery(query.data?.app_id ?? "");
  const activate = useActivateBuildMutation();
  if (query.isError) {
    return (
      <div className="p-6">
        <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
      </div>
    );
  }
  if (!query.data) return <p className="p-6 text-fg-muted">Loading…</p>;

  const build = query.data;
  const appId = build.app_id;
  const active = Boolean(appId) && appQuery.data?.active_build_id === build.id;
  return (
    <div>
      <PageHeader
        crumbs={[{ label: "Builds", href: "/builds" }, { label: build.id }]}
        title={<span className="font-mono">{build.id}</span>}
        description="What one deployment produced. How it went is on the deployment."
        actions={
          appId ? (
            <Button
              size="sm"
              variant={active ? "ghost" : "secondary"}
              disabled={activate.isPending}
              onClick={() =>
                activate.mutate(
                  { id: appId, buildId: active ? "" : build.id },
                  { onSuccess: () => toast.success(active ? "Released" : "Now active") },
                )
              }
            >
              {active ? <Check aria-hidden className="size-3.5" strokeWidth={1.5} /> : null}
              {active ? "Active" : "Use this build"}
            </Button>
          ) : null
        }
      />
      <div className="mx-auto max-w-4xl space-y-4 p-6">
        {activate.isError ? (
          <ErrorPanel error={activate.error} title="Could not change the active build" />
        ) : null}
        <Panel label="Build">
          <KeyValue
            columns={2}
            rows={[
              {
                label: "Deployment",
                value: build.deployment_id ? (
                  <Link className="underline" href={`/deployments/${build.deployment_id}`}>
                    {build.deployment_id}
                  </Link>
                ) : (
                  "—"
                ),
              },
              {
                label: "App",
                value: build.app_id ? (
                  <Link className="underline" href={`/apps/${build.app_id}`}>
                    {build.app_id}
                  </Link>
                ) : (
                  "—"
                ),
              },
              { label: "Runtime", value: build.runtime },
              { label: "Source SHA-256", value: build.source_sha256 },
              { label: "Made", value: <Timestamp value={build.created_at} /> },
            ]}
          />
        </Panel>
      </div>
    </div>
  );
}
