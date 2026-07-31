"use client";

import { useState, type FormEvent } from "react";
import { toast } from "sonner";

import { ErrorPanel, InlineError } from "@/components/neurun/error-panel";
import { Callout, EmptyState } from "@/components/neurun/feedback";
import { JsonView } from "@/components/neurun/json-view";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { Timestamp } from "@/components/neurun/timestamp";
import { Button } from "@/components/ui/button";
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
  useAPIKeysQuery,
  useCreateAPIKeyMutation,
  useRevokeAPIKeyMutation,
} from "@/lib/api/queries";
import type { CreatedApiKey } from "@/lib/api/resource-types";

const DEFAULT_SCOPES = [
  "projects:read",
  "apps:read",
  "apps:write",
  "deployments:read",
  "deployments:write",
  "builds:read",
  "executions:read",
  "executions:write",
].join(" ");

export default function APIKeysPage() {
  const query = useAPIKeysQuery();
  const create = useCreateAPIKeyMutation();
  const revoke = useRevokeAPIKeyMutation();
  const [name, setName] = useState("");
  const [userId, setUserId] = useState("");
  const [scopes, setScopes] = useState(DEFAULT_SCOPES);
  const [created, setCreated] = useState<CreatedApiKey | null>(null);

  function submit(event: FormEvent) {
    event.preventDefault();
    create.mutate(
      {
        name,
        user_id: userId.trim() || undefined,
        scopes: scopes.split(/\s+/).filter(Boolean),
      },
      {
        onSuccess: (apiKey) => {
          setCreated(apiKey);
          setName("");
        },
      },
    );
  }

  return (
    <div>
      <PageHeader title="API keys" description="Scoped credentials for SDKs and automation." />
      <div className="space-y-4 p-6">
        {created ? (
          <Callout kind="warning" title="Copy this secret now">
            <p className="mb-2">It is shown once and cannot be recovered.</p>
            <JsonView value={{ secret: created.secret }} preRedacted />
          </Callout>
        ) : null}
        <Panel label="New API key">
          <form onSubmit={submit} className="grid gap-3 md:grid-cols-3">
            <Field id="api-key-name" label="Name" value={name} setValue={setName} />
            <Field
              id="api-key-user"
              label="User ID (optional)"
              value={userId}
              setValue={setUserId}
              required={false}
            />
            <Field
              id="api-key-scopes"
              label="Scopes (space separated)"
              value={scopes}
              setValue={setScopes}
            />
            <div className="md:col-span-3">
              {create.isError ? <InlineError error={create.error} /> : null}
              <Button className="mt-2" disabled={create.isPending}>Create API key</Button>
            </div>
          </form>
        </Panel>
        {query.isError ? (
          <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
        ) : (
          <Panel label="API keys" flush>
            {query.isPending ? (
              <p className="p-4 text-fg-muted">Loading…</p>
            ) : query.data.api_keys.length === 0 ? (
              <EmptyState title="No API keys" description="Create a scoped API key above." />
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>API key</TableHead>
                    <TableHead>Scopes</TableHead>
                    <TableHead>Created</TableHead>
                    <TableHead />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {query.data.api_keys.map((apiKey) => (
                    <TableRow key={apiKey.id}>
                      <TableCell>{apiKey.name}</TableCell>
                      <TableCell className="font-mono">{apiKey.prefix}</TableCell>
                      <TableCell className="font-mono text-micro">{apiKey.scopes.join(" ")}</TableCell>
                      <TableCell><Timestamp value={apiKey.created_at} /></TableCell>
                      <TableCell className="text-right">
                        <Button
                          variant="destructive"
                          size="sm"
                          disabled={Boolean(apiKey.revoked_at) || revoke.isPending}
                          onClick={() =>
                            revoke.mutate(apiKey.id, {
                              onSuccess: () => toast.success("API key revoked"),
                            })
                          }
                        >
                          {apiKey.revoked_at ? "Revoked" : "Revoke"}
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
    </div>
  );
}

function Field({
  id,
  label,
  value,
  setValue,
  required = true,
}: {
  id: string;
  label: string;
  value: string;
  setValue: (value: string) => void;
  required?: boolean;
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        value={value}
        onChange={(event) => setValue(event.target.value)}
        required={required}
      />
    </div>
  );
}
