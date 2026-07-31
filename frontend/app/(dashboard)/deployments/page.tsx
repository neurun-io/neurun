"use client";

import { useState, type FormEvent } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";

import { ErrorPanel, InlineError } from "@/components/neurun/error-panel";
import { EmptyState } from "@/components/neurun/feedback";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { StatusBadge } from "@/components/neurun/status-badge";
import { Timestamp } from "@/components/neurun/timestamp";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  useAppsQuery,
  useCreateDeploymentMutation,
  useDeploymentsQuery,
} from "@/lib/api/queries";

export default function DeploymentsPage() {
  const query = useDeploymentsQuery();
  const appsQuery = useAppsQuery();
  const create = useCreateDeploymentMutation();
  const router = useRouter();
  const [file, setFile] = useState<File | null>(null);
  const [appId, setAppId] = useState("");
  const [entrypoint, setEntrypoint] = useState("main.py:handler");

  function submit(event: FormEvent) {
    event.preventDefault();
    if (!appId || !file) return;
    create.mutate(
      { appId, source: file, entrypoint },
      {
        onSuccess: (deployment) => {
          toast.success(`Built ${deployment.id}`);
          router.push(`/deployments/${deployment.id}`);
        },
      },
    );
  }

  return (
    <div>
      <PageHeader
        title="Deployments"
        description="Upload Python source and produce an immutable build for repeatable execution."
      />
      <div className="space-y-4 p-6">
        <Panel label="New deployment">
          <form onSubmit={submit} className="grid items-end gap-3 lg:grid-cols-[1fr_1fr_1fr_auto]">
            <div className="space-y-1.5">
              <Label htmlFor="app">App</Label>
              <Select value={appId} onValueChange={setAppId} disabled={appsQuery.isPending}>
                <SelectTrigger id="app">
                  <SelectValue placeholder="Select an app" />
                </SelectTrigger>
                <SelectContent>
                  {appsQuery.data?.apps.map((app) => (
                    <SelectItem key={app.id} value={app.id}>
                      {app.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="source">Source ZIP</Label>
              <Input
                id="source"
                type="file"
                accept=".zip,application/zip"
                onChange={(event) => setFile(event.target.files?.[0] ?? null)}
                required
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="entrypoint">Entrypoint</Label>
              <Input
                id="entrypoint"
                value={entrypoint}
                onChange={(event) => setEntrypoint(event.target.value)}
                required
              />
            </div>
            <Button disabled={!appId || !file || create.isPending}>
              {create.isPending ? "Building…" : "Upload and build"}
            </Button>
          </form>
          {appsQuery.isError ? <InlineError className="mt-3" error={appsQuery.error} /> : null}
          {!appsQuery.isPending && appsQuery.data?.apps.length === 0 ? (
            <p className="mt-3 text-sm text-fg-muted">
              Deploy an annotated program from the SDK first; its App is created automatically.
            </p>
          ) : null}
          {create.isError ? <InlineError className="mt-3" error={create.error} /> : null}
        </Panel>

        {query.isError ? (
          <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
        ) : (
          <Panel label="Deployments" flush>
            {query.isPending ? (
              <p className="p-4 text-fg-muted">Loading…</p>
            ) : query.data.deployments.length === 0 ? (
              <EmptyState title="No deployments" description="Upload a source ZIP above." />
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
          </Panel>
        )}
      </div>
    </div>
  );
}
