"use client";

import Link from "next/link";
import { useParams } from "next/navigation";

import { RepositoryPanel } from "@/components/github/repository-panel";
import { ErrorPanel } from "@/components/neurun/error-panel";
import { EmptyState } from "@/components/neurun/feedback";
import { KeyValue } from "@/components/neurun/key-value";
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
import { useAppQuery, useDeploymentsQuery } from "@/lib/api/queries";

export default function AppPage() {
  const { appId } = useParams<{ appId: string }>();
  const appQuery = useAppQuery(appId);
  const deploymentsQuery = useDeploymentsQuery(appId);

  if (appQuery.isError) {
    return (
      <div className="p-6">
        <ErrorPanel error={appQuery.error} onRetry={() => appQuery.refetch()} />
      </div>
    );
  }
  if (!appQuery.data) return <p className="p-6 text-fg-muted">Loading…</p>;

  const app = appQuery.data;
  return (
    <div>
      <PageHeader crumbs={[{ label: "Apps", href: "/apps" }, { label: app.id }]} title={app.name} />
      <div className="mx-auto max-w-4xl space-y-4 p-6">
        <Panel label="App">
          <KeyValue
            columns={2}
            rows={[
              { label: "ID", value: app.id },
              {
                label: "Project",
                value: (
                  <Link className="underline" href={`/projects/${app.project_id}`}>
                    {app.project_id}
                  </Link>
                ),
              },
              { label: "Created", value: <Timestamp value={app.created_at} /> },
              { label: "Updated", value: <Timestamp value={app.updated_at} /> },
            ]}
          />
        </Panel>
        <RepositoryPanel app={app} />
        {deploymentsQuery.isError ? (
          <ErrorPanel error={deploymentsQuery.error} onRetry={() => deploymentsQuery.refetch()} />
        ) : (
          <Panel label="Deployments" flush>
            {deploymentsQuery.isPending ? (
              <p className="p-4 text-fg-muted">Loading…</p>
            ) : deploymentsQuery.data.deployments.length === 0 ? (
              <EmptyState
                title="No deployments"
                description="Push to this app's production ref, or deploy the current one from the repository panel above."
              />
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Status</TableHead>
                    <TableHead>Deployment</TableHead>
                    <TableHead>Entrypoint</TableHead>
                    <TableHead>Updated</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {deploymentsQuery.data.deployments.map((deployment) => (
                    <TableRow key={deployment.id}>
                      <TableCell><StatusBadge status={deployment.status} /></TableCell>
                      <TableCell>
                        <Link className="font-mono underline" href={`/deployments/${deployment.id}`}>
                          {deployment.id}
                        </Link>
                      </TableCell>
                      <TableCell className="font-mono">{deployment.entrypoint}</TableCell>
                      <TableCell><Timestamp value={deployment.updated_at} /></TableCell>
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
