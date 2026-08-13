"use client";

import Link from "next/link";

import { ErrorPanel } from "@/components/neurun/error-panel";
import { EmptyState } from "@/components/neurun/feedback";
import { PageHeader } from "@/components/neurun/page-header";
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
import { useDeploymentsQuery } from "@/lib/api/queries";

export default function DeploymentsPage() {
  const query = useDeploymentsQuery();

  return (
    <div>
      <PageHeader
        title="Deployments"
        description="One commit of an app's repository, built into an immutable artifact. A push to the app's production ref makes one."
      />
      <div className="space-y-4 p-6">
        {query.isError ? (
          <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
        ) : query.isPending ? (
          <p className="text-fg-muted">Loading…</p>
        ) : query.data.deployments.length === 0 ? (
          <EmptyState
            title="No deployments"
            description="Connect an app to a repository; the next push to its production ref deploys the commit that was pushed."
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Status</TableHead>
                <TableHead>Deployment</TableHead>
                <TableHead>App</TableHead>
                <TableHead>Entrypoint</TableHead>
                <TableHead>Builds</TableHead>
                <TableHead>Updated</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {query.data.deployments.map((deployment) => (
                <TableRow key={deployment.id}>
                  <TableCell>
                    <StatusBadge status={deployment.status} />
                  </TableCell>
                  <TableCell>
                    <Link
                      className="font-mono underline"
                      href={`/deployments/${deployment.id}`}
                    >
                      {deployment.id}
                    </Link>
                  </TableCell>
                  <TableCell>
                    <Link className="font-mono underline" href={`/apps/${deployment.app_id}`}>
                      {deployment.app_id}
                    </Link>
                  </TableCell>
                  <TableCell className="font-mono">{deployment.entrypoint}</TableCell>
                  <TableCell>{deployment.builds.length}</TableCell>
                  <TableCell>
                    <Timestamp value={deployment.updated_at} />
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
