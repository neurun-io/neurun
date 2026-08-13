"use client";

import Link from "next/link";
import { Plus } from "lucide-react";
import { useState } from "react";

import { ConfirmDeleteDialog } from "@/components/neurun/confirm-delete-dialog";
import { InstallLink } from "@/components/github/install-link";
import { ErrorPanel, InlineError } from "@/components/neurun/error-panel";
import { EmptyState } from "@/components/neurun/feedback";
import { PageHeader } from "@/components/neurun/page-header";
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
  useBranchesQuery,
  useCreateAppMutation,
  useDeleteAppMutation,
  useProjectsQuery,
  useRepositoriesQuery,
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
        description="A deployable scraper or browser automation program."
      />
      <div className="space-y-4 p-6">
        <CreateAppDialog />

        {query.isError ? (
          <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
        ) : query.isPending ? (
          <p className="text-fg-muted">Loading…</p>
        ) : query.data.apps.length === 0 ? (
          <EmptyState
            title="No apps"
            description="Create one, then deploy to it."
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
  const [repository, setRepository] = useState("");
  const [productionRef, setProductionRef] = useState("");
  const projects = useProjectsQuery();
  const repositories = useRepositoriesQuery();
  const branches = useBranchesQuery(repository);
  const create = useCreateAppMutation();
  const granted = repositories.data?.repositories ?? [];

  const submit = () => {
    const trimmed = name.trim();
    const repo = repository.trim();
    if (!trimmed || !projectId || !repo || create.isPending) return;
    create.mutate(
      { projectId, name: trimmed, repository: repo, productionRef: productionRef.trim() },
      { onSuccess: () => setOpen(false) },
    );
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next) {
          setName("");
          setProjectId("");
          setRepository("");
          setProductionRef("");
          create.reset();
        }
      }}
    >
      <DialogTrigger asChild>
        <Button>
          <Plus aria-hidden />
          Create app
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

          <div className="grid gap-2">
            <Label htmlFor="app-repository" className="text-caption text-fg-muted">
              Repository
            </Label>
            <Select
              value={repository}
              onValueChange={(next) => {
                setRepository(next);
                // The default branch is what HEAD resolves to, so it is the ref
                // to offer until somebody picks another.
                setProductionRef(
                  granted.find((record) => record.full_name === next)?.default_branch ?? "",
                );
              }}
            >
              <SelectTrigger id="app-repository">
                <SelectValue
                  placeholder={
                    repositories.isPending ? "Loading repositories…" : "Select a repository"
                  }
                />
              </SelectTrigger>
              <SelectContent>
                {granted.map((record) => (
                  <SelectItem key={record.full_name} value={record.full_name}>
                    {record.full_name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {repositories.isError ? (
              <InlineError error={repositories.error} />
            ) : repositories.isSuccess && granted.length === 0 ? (
              <div className="flex flex-col items-start gap-2">
                <p className="text-caption text-fg-muted">
                  The GitHub App reads no repositories yet.
                </p>
                <InstallLink label="Add repositories on GitHub" variant="secondary" />
              </div>
            ) : null}
          </div>

          <div className="grid gap-2">
            <Label htmlFor="app-production-ref" className="text-caption text-fg-muted">
              Production ref
            </Label>
            <Select
              value={productionRef}
              onValueChange={setProductionRef}
              disabled={!repository || branches.isPending}
            >
              <SelectTrigger id="app-production-ref">
                <SelectValue
                  placeholder={branches.isPending ? "Loading branches…" : "Select a branch"}
                />
              </SelectTrigger>
              <SelectContent>
                {(branches.data?.branches ?? []).map((branch) => (
                  <SelectItem key={branch} value={branch}>
                    {branch}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {branches.isError ? <InlineError error={branches.error} /> : null}
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
          <Button
            onClick={submit}
            disabled={!name.trim() || !projectId || !repository.trim() || create.isPending}
          >
            {create.isPending ? "Creating…" : "Create app"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
