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
import { useExecutionsQuery } from "@/lib/api/queries";

export default function ExecutionsPage() {
  const query = useExecutionsQuery();
  return (
    <div>
      <PageHeader
        title="Executions"
        description="Durable worker executions pinned to an immutable build."
      />
      <div className="p-6">
        {query.isError ? (
          <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
        ) : query.isPending ? (
          <p className="text-fg-muted">Loading…</p>
        ) : query.data.executions.length === 0 ? (
          <EmptyState
            title="No executions"
            description="Execute a ready deployment to queue one."
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Status</TableHead>
                <TableHead>Execution</TableHead>
                <TableHead>App</TableHead>
                <TableHead>Build</TableHead>
                <TableHead>Created</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {query.data.executions.map((execution) => (
                <TableRow key={execution.id}>
                  <TableCell>
                    <StatusBadge status={execution.status} />
                  </TableCell>
                  <TableCell>
                    <Link className="font-mono underline" href={`/executions/${execution.id}`}>
                      {execution.id}
                    </Link>
                  </TableCell>
                  <TableCell>
                    <Link
                      className="font-mono underline"
                      href={`/apps/${execution.app_id}`}
                    >
                      {execution.app_id}
                    </Link>
                  </TableCell>
                  <TableCell>
                    <Link className="font-mono underline" href={`/builds/${execution.build_id}`}>
                      {execution.build_id}
                    </Link>
                  </TableCell>
                  <TableCell>
                    <Timestamp value={execution.created_at} />
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
