"use client";

import Link from "next/link";

import { ErrorPanel } from "@/components/neurun/error-panel";
import { EmptyState } from "@/components/neurun/feedback";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { StatusBadge } from "@/components/neurun/status-badge";
import { Timestamp } from "@/components/neurun/timestamp";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useBuildsQuery } from "@/lib/api/queries";

export default function BuildsPage() {
  const query = useBuildsQuery();
  return (
    <div>
      <PageHeader title="Builds" description="Immutable Python builds produced by deployments." />
      <div className="p-6">
        {query.isError ? (
          <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
        ) : (
          <Panel label="Builds" flush>
            {query.isPending ? (
              <p className="p-4 text-fg-muted">Loading…</p>
            ) : query.data.builds.length === 0 ? (
              <EmptyState title="No builds" description="Upload a deployment to create one." />
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Status</TableHead>
                    <TableHead>Build</TableHead>
                    <TableHead>Deployment</TableHead>
                    <TableHead>Entrypoint</TableHead>
                    <TableHead>Started</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {query.data.builds.map((build) => (
                    <TableRow key={build.id}>
                      <TableCell>
                        <StatusBadge status={build.status} />
                      </TableCell>
                      <TableCell>
                        <Link className="font-mono underline" href={`/builds/${build.id}`}>
                          {build.id}
                        </Link>
                      </TableCell>
                      <TableCell>
                        <Link
                          className="font-mono underline"
                          href={`/deployments/${build.deployment_id}`}
                        >
                          {build.deployment_id}
                        </Link>
                      </TableCell>
                      <TableCell className="font-mono">{build.entrypoint}</TableCell>
                      <TableCell>
                        <Timestamp value={build.started_at} />
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </Panel>
        )}
      </div>
    </div>
  );
}
