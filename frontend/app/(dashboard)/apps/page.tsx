"use client";

import Link from "next/link";
import { useState } from "react";

import { ConfirmDeleteDialog } from "@/components/neurun/confirm-delete-dialog";
import { ErrorPanel } from "@/components/neurun/error-panel";
import { EmptyState } from "@/components/neurun/feedback";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { Timestamp } from "@/components/neurun/timestamp";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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
  useCreateAppMutation,
  useDeleteAppMutation,
  useProjectsQuery,
} from "@/lib/api/queries";
import type { NeurunApp } from "@/lib/api/resource-types";

export default function AppsPage() {
  const query = useAppsQuery();
  const [pendingDelete, setPendingDelete] = useState<NeurunApp | null>(null);
  const remove = useDeleteAppMutation();

  return (
    <div>
      <PageHeader
        title="Apps"
        description="What a deployment targets. An app must exist before you deploy to it — deploying never creates one, so a typo in a client fails loudly instead of quietly making a second app."
      />
      <div className="p-6">
        {query.isError ? (
          <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
        ) : (
          <Panel label="Apps" actions={<CreateAppDialog />} flush>
            {query.isPending ? (
              <p className="p-4 text-fg-muted">Loading…</p>
            ) : query.data.apps.length === 0 ? (
              <EmptyState
                title="No apps"
                description="Create one, then deploy to it by its identifier."
              />
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>App</TableHead>
                    <TableHead>Name</TableHead>
                    <TableHead>Project</TableHead>
                    <TableHead>Updated</TableHead>
                    <TableHead className="w-0 text-right">Actions</TableHead>
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
                      <TableCell className="text-right">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => {
                            remove.reset();
                            setPendingDelete(app);
                          }}
                        >
                          Delete
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </Panel>
        )}
      </div>

      <ConfirmDeleteDialog
        open={pendingDelete !== null}
        onOpenChange={(open) => {
          if (!open) setPendingDelete(null);
        }}
        kind="app"
        name={pendingDelete?.name ?? ""}
        consequence={
          <>
            This deletes every deployment, build and execution under{" "}
            <span className="font-mono">{pendingDelete?.id}</span>. It cannot be undone.
          </>
        }
        pending={remove.isPending}
        error={remove.isError ? remove.error.message : undefined}
        onConfirm={() => {
          if (!pendingDelete) return;
          remove.mutate({ id: pendingDelete.id }, { onSuccess: () => setPendingDelete(null) });
        }}
      />
    </div>
  );
}

function CreateAppDialog() {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [projectId, setProjectId] = useState("");
  const projects = useProjectsQuery();
  const create = useCreateAppMutation();

  const submit = () => {
    const trimmed = name.trim();
    if (!trimmed || !projectId || create.isPending) return;
    create.mutate({ projectId, name: trimmed }, { onSuccess: () => setOpen(false) });
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next) {
          setName("");
          setProjectId("");
          create.reset();
        }
      }}
    >
      <DialogTrigger asChild>
        <Button size="sm" variant="secondary">
          New app
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>New app</DialogTitle>
        </DialogHeader>
        <form
          className="grid gap-4"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <div className="grid gap-2">
            <Label htmlFor="app-project" className="text-caption text-fg-muted">
              Project
            </Label>
            <Select value={projectId} onValueChange={setProjectId}>
              <SelectTrigger id="app-project">
                <SelectValue
                  placeholder={projects.isPending ? "Loading projects…" : "Select a project"}
                />
              </SelectTrigger>
              <SelectContent>
                {(projects.data?.projects ?? []).map((project) => (
                  <SelectItem key={project.id} value={project.id}>
                    {project.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {projects.isSuccess && projects.data.projects.length === 0 ? (
              <p className="text-caption text-fg-muted">
                No projects yet. Create one first — an app must belong to a project.
              </p>
            ) : null}
          </div>

          <div className="grid gap-2">
            <Label htmlFor="app-name" className="text-caption text-fg-muted">
              Name
            </Label>
            <Input
              id="app-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              autoComplete="off"
              aria-invalid={create.isError ? true : undefined}
            />
          </div>

          {create.isError ? (
            <p role="alert" className="text-caption text-fg">
              {create.error.message}
            </p>
          ) : null}
        </form>
        <DialogFooter>
          <Button variant="ghost" onClick={() => setOpen(false)} disabled={create.isPending}>
            Cancel
          </Button>
          <Button onClick={submit} disabled={!name.trim() || !projectId || create.isPending}>
            {create.isPending ? "Creating…" : "Create app"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
