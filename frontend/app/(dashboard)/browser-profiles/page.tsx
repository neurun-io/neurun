"use client";

import { useState, type FormEvent } from "react";
import { toast } from "sonner";

import { ConfirmDeleteDialog } from "@/components/neurun/confirm-delete-dialog";
import { ErrorPanel, InlineError } from "@/components/neurun/error-panel";
import { Callout, EmptyState } from "@/components/neurun/feedback";
import { JsonView } from "@/components/neurun/json-view";
import { KeyValue } from "@/components/neurun/key-value";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { Timestamp } from "@/components/neurun/timestamp";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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
  useBrowserProfileStateQuery,
  useBrowserProfilesQuery,
  useCreateBrowserProfileMutation,
  useDeleteBrowserProfileMutation,
} from "@/lib/api/queries";
import type { BrowserKind, BrowserProfile } from "@/lib/api/resource-types";

export default function BrowserProfilesPage() {
  const query = useBrowserProfilesQuery();
  const create = useCreateBrowserProfileMutation();
  const remove = useDeleteBrowserProfileMutation();

  const [name, setName] = useState("");
  const [browser, setBrowser] = useState<BrowserKind>("chrome");
  const [doomed, setDoomed] = useState<BrowserProfile | null>(null);
  const [revealing, setRevealing] = useState<string | null>(null);

  function submit(event: FormEvent) {
    event.preventDefault();
    // No identity: a profile starts as a plain browser and is given a persona
    // afterwards. Asking for twenty fingerprint fields up front would make the
    // common case the hard one.
    create.mutate({ name, browser }, { onSuccess: () => setName("") });
  }

  return (
    <div>
      <PageHeader
        title="Browser profiles"
        description="Who a browser appears to be, and what it remembers between sessions."
      />
      <div className="space-y-4 p-6">
        <Callout kind="note" title="Sessions are opened by the SDK">
          A profile is read by your app at runtime, worn by a browser the SDK
          launches on loopback, and written back when the session closes. Nothing
          is launched from here.
        </Callout>

        <Panel label="New profile">
          <form onSubmit={submit} className="grid gap-3 md:grid-cols-3">
            <div className="space-y-1.5">
              <Label htmlFor="browser-profile-name">Name</Label>
              <Input
                id="browser-profile-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                required
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="browser-profile-browser">Browser</Label>
              <Select
                value={browser}
                onValueChange={(value) => setBrowser(value as BrowserKind)}
              >
                <SelectTrigger id="browser-profile-browser">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="chrome">Chrome</SelectItem>
                  <SelectItem value="firefox">Firefox</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="md:col-span-3">
              {create.isError ? <InlineError error={create.error} /> : null}
              <Button className="mt-2" disabled={create.isPending}>
                Create profile
              </Button>
            </div>
          </form>
        </Panel>

        {query.isError ? (
          <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
        ) : (
          <Panel label="Profiles" flush>
            {query.isPending ? (
              <p className="p-4 text-fg-muted">Loading…</p>
            ) : query.data.browser_profiles.length === 0 ? (
              <EmptyState
                title="No browser profiles"
                description="Create one above. A profile keeps its cookies and storage between sessions."
              />
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>Browser</TableHead>
                    <TableHead>Identity</TableHead>
                    <TableHead>Remembers</TableHead>
                    <TableHead>Updated</TableHead>
                    <TableHead />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {query.data.browser_profiles.map((profile) => (
                    <TableRow key={profile.id}>
                      <TableCell>{profile.name}</TableCell>
                      <TableCell className="font-mono text-micro">
                        {profile.browser}
                      </TableCell>
                      <TableCell>
                        {profile.identity ? (
                          <Badge>
                            {profile.identity.brand} on {profile.identity.os}
                          </Badge>
                        ) : (
                          <span className="text-fg-muted">Plain browser</span>
                        )}
                      </TableCell>
                      <TableCell className="text-micro text-fg-secondary">
                        {profile.cookies.length} cookies
                        {profile.storage_origins.length > 0
                          ? `, ${profile.storage_origins.length} origins`
                          : null}
                      </TableCell>
                      <TableCell>
                        <Timestamp value={profile.updated_at} />
                      </TableCell>
                      <TableCell className="space-x-2 text-right">
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => setRevealing(profile.id)}
                        >
                          State
                        </Button>
                        <Button
                          size="sm"
                          variant="destructive"
                          onClick={() => setDoomed(profile)}
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

        {revealing ? (
          <ProfileState id={revealing} onClose={() => setRevealing(null)} />
        ) : null}
      </div>

      <ConfirmDeleteDialog
        open={doomed !== null}
        onOpenChange={(next) => !next && setDoomed(null)}
        kind="browser profile"
        name={doomed?.name ?? ""}
        consequence="Its cookies and stored logins go with it. Anything signed in through this profile will have to sign in again."
        error={remove.isError ? String(remove.error) : undefined}
        pending={remove.isPending}
        onConfirm={() => {
          if (!doomed) return;
          remove.mutate(doomed.id, {
            onSuccess: () => {
              setDoomed(null);
              toast.success("Browser profile deleted");
            },
          });
        }}
      />
    </div>
  );
}

/**
 * The profile's cookies and storage, in the clear.
 *
 * Fetched only when asked for, and never cached: this is the one response that
 * carries live credentials, so it should not sit in the query cache waiting for
 * somebody to open devtools.
 */
function ProfileState({ id, onClose }: { id: string; onClose: () => void }) {
  const query = useBrowserProfileStateQuery(id);

  return (
    <Panel
      label="Profile state"
      actions={
        <Button size="sm" variant="ghost" onClick={onClose}>
          Hide
        </Button>
      }
    >
      <Callout kind="warning" title="These are live credentials">
        Session cookies here are as good as being signed in. Copying them out is
        exporting the account.
      </Callout>
      {query.isError ? (
        <InlineError error={query.error} />
      ) : query.isPending ? (
        <p className="text-fg-muted">Loading…</p>
      ) : (
        <div className="mt-3 space-y-4">
          <KeyValue
            rows={[
              { label: "Cookies", value: query.data.cookies.length },
              {
                label: "Local storage origins",
                value: Object.keys(query.data.local_storage).length,
              },
              {
                label: "Session storage origins",
                value: Object.keys(query.data.session_storage).length,
              },
            ]}
          />
          <JsonView value={query.data} preRedacted />
        </div>
      )}
    </Panel>
  );
}
