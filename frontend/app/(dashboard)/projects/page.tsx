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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  useCreateProjectMutation,
  useDeleteProjectMutation,
  useProjectsQuery,
} from "@/lib/api/queries";
import type { Project } from "@/lib/api/resource-types";

export default function ProjectsPage() {
  const query = useProjectsQuery();
  const [pendingDelete, setPendingDelete] = useState<Project | null>(null);
  const remove = useDeleteProjectMutation();

  return (
    <div>
      <PageHeader
        title="Projects"
        description="The ownership boundary for apps, deployments, builds and executions. Users and API keys belong to the installation, not to a project."
      />
      <div className="p-6">
        {query.isError ? (
          <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
        ) : (
          <Panel label="Projects" actions={<CreateProjectDialog />} flush>
            {query.isPending ? (
              <p className="p-4 text-fg-muted">Loading…</p>
            ) : query.data.projects.length === 0 ? (
              <EmptyState
                title="No projects"
                description="Create one before deploying — an app must belong to a project."
              />
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Project</TableHead>
                    <TableHead>Name</TableHead>
                    <TableHead>Updated</TableHead>
                    <TableHead className="w-0 text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {query.data.projects.map((project) => (
                    <TableRow key={project.id}>
                      <TableCell>
                        <Link className="font-mono underline" href={`/projects/${project.id}`}>
                          {project.id}
                        </Link>
                      </TableCell>
                      <TableCell>{project.name}</TableCell>
                      <TableCell>
                        <Timestamp value={project.updated_at} />
                      </TableCell>
                      <TableCell className="text-right">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => {
                            remove.reset();
                            setPendingDelete(project);
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
        kind="project"
        name={pendingDelete?.name ?? ""}
        consequence={
          <>
            This deletes every app, deployment, build and execution under{" "}
            <span className="font-mono">{pendingDelete?.id}</span>. Users and API keys
            are not touched. It cannot be undone.
          </>
        }
        pending={remove.isPending}
        error={remove.isError ? remove.error.message : undefined}
        onConfirm={() => {
          if (!pendingDelete) return;
          remove.mutate(
            { id: pendingDelete.id },
            { onSuccess: () => setPendingDelete(null) },
          );
        }}
      />
    </div>
  );
}

function CreateProjectDialog() {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const create = useCreateProjectMutation();

  const submit = () => {
    const trimmed = name.trim();
    if (!trimmed || create.isPending) return;
    create.mutate({ name: trimmed }, { onSuccess: () => setOpen(false) });
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next) {
          setName("");
          create.reset();
        }
      }}
    >
      <DialogTrigger asChild>
        <Button size="sm" variant="secondary">
          New project
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>New project</DialogTitle>
        </DialogHeader>
        <form
          className="grid gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <Label htmlFor="project-name" className="text-caption text-fg-muted">
            Name
          </Label>
          <Input
            id="project-name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            autoComplete="off"
            aria-invalid={create.isError ? true : undefined}
          />
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
          <Button onClick={submit} disabled={!name.trim() || create.isPending}>
            {create.isPending ? "Creating…" : "Create project"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
