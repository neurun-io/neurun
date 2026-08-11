"use client";

import { useState } from "react";
import Link from "next/link";
import { toast } from "sonner";

import { ProfileForm, type ProfileValues } from "@/components/browser-profiles/profile-form";
import { ConfirmDeleteDialog } from "@/components/neurun/confirm-delete-dialog";
import { ErrorPanel, InlineError } from "@/components/neurun/error-panel";
import { Callout, EmptyState } from "@/components/neurun/feedback";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { Timestamp } from "@/components/neurun/timestamp";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  useBrowserProfilesQuery,
  useCreateBrowserProfileMutation,
  useDeleteBrowserProfileMutation,
} from "@/lib/api/queries";
import type { BrowserProfile } from "@/lib/api/resource-types";

export default function BrowserProfilesPage() {
  const query = useBrowserProfilesQuery();
  const create = useCreateBrowserProfileMutation();
  const remove = useDeleteBrowserProfileMutation();

  // Bumped on a successful create, which remounts the form rather than reaching
  // into it to clear twenty-odd fields one at a time.
  const [createdCount, setCreatedCount] = useState(0);
  const [doomed, setDoomed] = useState<BrowserProfile | null>(null);

  function submitNew(values: ProfileValues) {
    create.mutate(values, {
      onSuccess: () => {
        setCreatedCount((count) => count + 1);
        toast.success("Browser profile created");
      },
    });
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
          <ProfileForm
            key={createdCount}
            submitLabel="Create profile"
            pending={create.isPending}
            error={create.isError ? <InlineError error={create.error} /> : null}
            onSubmit={submitNew}
          />
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
                    <TableHead className="w-0 text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {query.data.browser_profiles.map((profile) => (
                    <TableRow key={profile.id}>
                      <TableCell>
                        <Link className="underline" href={`/browser-profiles/${profile.id}`}>
                          {profile.name}
                        </Link>
                      </TableCell>
                      <TableCell className="font-mono text-micro">
                        {profile.browser}
                      </TableCell>
                      <TableCell>
                        {profile.identity ? (
                          <span className="flex flex-wrap items-center gap-1">
                            <Badge>
                              {profile.identity.brand} on {profile.identity.os}
                            </Badge>
                            {profile.identity.proxy_set ? (
                              <Badge variant="outline">proxy</Badge>
                            ) : null}
                          </span>
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
                      <TableCell className="text-right">
                        <Button
                          size="sm"
                          variant="destructive"
                          onClick={() => {
                            remove.reset();
                            setDoomed(profile);
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
