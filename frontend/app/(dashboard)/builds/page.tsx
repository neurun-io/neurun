"use client";

import Link from "next/link";

import { ErrorPanel } from "@/components/neurun/error-panel";
import { EmptyState } from "@/components/neurun/feedback";
import { PageHeader } from "@/components/neurun/page-header";
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
      <PageHeader title="Builds" description="What deployments produced." />
      <div className="p-6">
        {query.isError ? (
          <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
        ) : query.isPending ? (
          <p className="text-fg-muted">Loading…</p>
        ) : query.data.builds.length === 0 ? (
          <EmptyState title="No builds" description="Deploy an app to produce one." />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Build</TableHead>
                <TableHead>Runtime</TableHead>
                <TableHead>Entrypoint</TableHead>
                <TableHead>Deployment</TableHead>
                <TableHead>Layers</TableHead>
                <TableHead>Made</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {query.data.builds.map((build) => (
                <TableRow key={build.id}>
                  <TableCell>
                    <Link className="font-mono underline" href={`/builds/${build.id}`}>
                      {build.id}
                    </Link>
                  </TableCell>
                  <TableCell className="font-mono">{build.runtime}</TableCell>
                  <TableCell className="font-mono">{build.entrypoint || "—"}</TableCell>
                  <TableCell>
                    {build.deployment_id ? (
                      <Link
                        className="font-mono underline"
                        href={`/deployments/${build.deployment_id}`}
                      >
                        {build.deployment_id}
                      </Link>
                    ) : (
                      "—"
                    )}
                  </TableCell>
                  <TableCell>{build.artifacts.length}</TableCell>
                  <TableCell>
                    <Timestamp value={build.created_at} />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </div>
    </div>
  );
}
