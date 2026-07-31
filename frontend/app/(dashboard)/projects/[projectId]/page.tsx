"use client";

import type { FormEvent } from "react";
import { useParams } from "next/navigation";
import { toast } from "sonner";

import { ErrorPanel, InlineError } from "@/components/neurun/error-panel";
import { KeyValue } from "@/components/neurun/key-value";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { Timestamp } from "@/components/neurun/timestamp";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useProjectQuery, useUpdateProjectMutation } from "@/lib/api/queries";

export default function ProjectPage() {
  const { projectId } = useParams<{ projectId: string }>();
  const query = useProjectQuery(projectId);
  const update = useUpdateProjectMutation(projectId);

  if (query.isError) {
    return (
      <div className="p-6">
        <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
      </div>
    );
  }
  if (!query.data) return <p className="p-6 text-fg-muted">Loading…</p>;

  const project = query.data;
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const name = String(new FormData(event.currentTarget).get("name") ?? "").trim();
    update.mutate(
      { name },
      { onSuccess: () => toast.success("Project updated") },
    );
  }

  return (
    <div>
      <PageHeader
        crumbs={[{ label: "Projects", href: "/projects" }, { label: project.id }]}
        title={project.name}
      />
      <div className="mx-auto max-w-3xl space-y-4 p-6">
        <Panel label="Project">
          <KeyValue
            rows={[
              { label: "ID", value: project.id },
              { label: "Created", value: <Timestamp value={project.created_at} /> },
              { label: "Updated", value: <Timestamp value={project.updated_at} /> },
            ]}
          />
        </Panel>
        <Panel label="Settings">
          <form onSubmit={submit} className="space-y-3">
            <div className="space-y-1.5">
              <Label htmlFor="project-name">Name</Label>
              <Input
                id="project-name"
                name="name"
                defaultValue={project.name}
                required
              />
            </div>
            {update.isError ? <InlineError error={update.error} /> : null}
            <Button disabled={update.isPending}>Save project</Button>
          </form>
        </Panel>
      </div>
    </div>
  );
}
