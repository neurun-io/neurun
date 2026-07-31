"use client";

import Link from "next/link";

import { ErrorPanel } from "@/components/neurun/error-panel";
import { EmptyState } from "@/components/neurun/feedback";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { Timestamp } from "@/components/neurun/timestamp";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useAppsQuery } from "@/lib/api/queries";

export default function AppsPage() {
  const query = useAppsQuery();

  return (
    <div>
      <PageHeader
        title="Apps"
        description="Project-owned programs discovered automatically when annotated code deploys."
      />
      <div className="p-6">
        {query.isError ? (
          <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
        ) : (
          <Panel label="Apps" flush>
            {query.isPending ? (
              <p className="p-4 text-fg-muted">Loading…</p>
            ) : query.data.apps.length === 0 ? (
              <EmptyState
                title="No apps"
                description="Deploy an annotated program; Neurun derives its App name automatically."
              />
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>App</TableHead>
                    <TableHead>Name</TableHead>
                    <TableHead>Project</TableHead>
                    <TableHead>Updated</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {query.data.apps.map((app) => (
                    <TableRow key={app.id}>
                      <TableCell>
                        <Link className="font-mono underline" href={`/apps/${app.id}`}>
                          {app.id}
                        </Link>
                      </TableCell>
                      <TableCell>{app.name}</TableCell>
                      <TableCell>
                        <Link className="font-mono underline" href={`/projects/${app.project_id}`}>
                          {app.project_id}
                        </Link>
                      </TableCell>
                      <TableCell>
                        <Timestamp value={app.updated_at} />
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
