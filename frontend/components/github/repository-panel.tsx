"use client";

import { useState, type FormEvent } from "react";
import { GitBranch, Rocket } from "lucide-react";
import { toast } from "sonner";

import { InlineError } from "@/components/neurun/error-panel";
import { EmptyState } from "@/components/neurun/feedback";
import { KeyValue } from "@/components/neurun/key-value";
import { Panel } from "@/components/neurun/panel";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useConnectRepositoryMutation, useDeployRefMutation } from "@/lib/api/queries";
import type { NeurunApp } from "@/lib/api/resource-types";

/**
 * The repository an app deploys from.
 *
 * Connecting resolves the ref through the installation before storing anything,
 * so a repository the app cannot read is refused here rather than on the first
 * push.
 */
export function RepositoryPanel({ app }: { app: NeurunApp }) {
  const connect = useConnectRepositoryMutation();
  const deploy = useDeployRefMutation();
  const [editing, setEditing] = useState(false);
  const [repository, setRepository] = useState(app.repository ?? "");
  const [productionRef, setProductionRef] = useState(app.production_ref ?? "");

  function submit(event: FormEvent) {
    event.preventDefault();
    connect.mutate(
      { id: app.id, repository: repository.trim(), productionRef: productionRef.trim() },
      {
        onSuccess: (updated) => {
          setEditing(false);
          toast.success(
            updated.repository ? `Connected ${updated.repository}` : "Repository disconnected",
          );
        },
      },
    );
  }

  const connected = Boolean(app.repository) && !editing;

  return (
    <Panel
      label="Repository"
      actions={
        connected ? (
          <div className="-my-1 flex items-center">
            <Button
              size="sm"
              variant="ghost"
              disabled={deploy.isPending}
              onClick={() =>
                deploy.mutate(
                  { appId: app.id, ref: "" },
                  { onSuccess: (record) => toast.success(`Built ${record.id}`) },
                )
              }
            >
              <Rocket aria-hidden />
              {deploy.isPending ? "Deploying…" : "Deploy now"}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => {
                connect.reset();
                setEditing(true);
              }}
            >
              Edit
            </Button>
          </div>
        ) : null
      }
    >
      {connected ? (
        <div className="space-y-3">
          <KeyValue
            rows={[
              { label: "Repository", value: app.repository },
              {
                label: "Production ref",
                value: app.production_ref || "default branch",
                hint: "Every push to this ref deploys the commit that was pushed.",
              },
            ]}
          />
          {deploy.isError ? <InlineError error={deploy.error} /> : null}
        </div>
      ) : editing || app.repository ? (
        <form onSubmit={submit} className="space-y-3">
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="repository">Repository</Label>
              <Input
                id="repository"
                placeholder="owner/name"
                value={repository}
                onChange={(event) => setRepository(event.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="production-ref">Production ref</Label>
              <Input
                id="production-ref"
                placeholder="main"
                value={productionRef}
                onChange={(event) => setProductionRef(event.target.value)}
              />
            </div>
          </div>
          <p className="text-sm text-fg-muted">
            Leave the ref empty to follow the repository&apos;s default branch. Clearing the
            repository disconnects the app.
          </p>
          {connect.isError ? <InlineError error={connect.error} /> : null}
          <div className="flex items-center gap-2">
            <Button disabled={connect.isPending}>
              {connect.isPending ? "Connecting…" : "Connect"}
            </Button>
            {app.repository ? (
              <Button
                type="button"
                variant="ghost"
                disabled={connect.isPending}
                onClick={() => {
                  setRepository(app.repository ?? "");
                  setProductionRef(app.production_ref ?? "");
                  setEditing(false);
                }}
              >
                Cancel
              </Button>
            ) : null}
          </div>
        </form>
      ) : (
        <EmptyState
          icon={GitBranch}
          title="No repository"
          description="Connect this app to a repository the GitHub App can read. Every push to its production ref then builds that commit."
          action={
            <Button
              size="sm"
              onClick={() => {
                connect.reset();
                setEditing(true);
              }}
            >
              <GitBranch aria-hidden />
              Connect repository
            </Button>
          }
        />
      )}
    </Panel>
  );
}
