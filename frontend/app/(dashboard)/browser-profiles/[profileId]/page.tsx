"use client";

import { useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { toast } from "sonner";

import { ProfileForm, type ProfileValues } from "@/components/browser-profiles/profile-form";
import { ConfirmDeleteDialog } from "@/components/neurun/confirm-delete-dialog";
import { CopyButton } from "@/components/neurun/copy-id";
import { ErrorPanel, InlineError } from "@/components/neurun/error-panel";
import { Callout } from "@/components/neurun/feedback";
import { JsonView } from "@/components/neurun/json-view";
import { KeyValue } from "@/components/neurun/key-value";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { Timestamp } from "@/components/neurun/timestamp";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  useBrowserProfileQuery,
  useBrowserProfileStateQuery,
  useDeleteBrowserProfileMutation,
  useUpdateBrowserProfileMutation,
} from "@/lib/api/queries";
import { changedRows, profileSummary } from "@/lib/view/browser-identity";

/** Shown in the header rather than repeated as rows. */
const IN_HEADER = ["Name", "Browser", "Cookies", "Storage origins"];

export default function BrowserProfilePage() {
  const { profileId } = useParams<{ profileId: string }>();
  const router = useRouter();
  const query = useBrowserProfileQuery(profileId);
  const update = useUpdateBrowserProfileMutation();
  const remove = useDeleteBrowserProfileMutation();

  // What the last save moved, and a counter that remounts the form on a save so
  // the fields are re-read from the stored profile rather than from the draft
  // that produced it — the proxy in particular, which never comes back.
  const [changed, setChanged] = useState<string[]>([]);
  const [saves, setSaves] = useState(0);
  const [revealing, setRevealing] = useState(false);
  const [doomed, setDoomed] = useState(false);

  if (query.isError) {
    return (
      <div className="p-6">
        <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
      </div>
    );
  }
  if (!query.data) return <p className="p-6 text-fg-muted">Loading…</p>;

  const profile = query.data;
  const identity = profile.identity;

  function save(values: ProfileValues) {
    update.mutate(
      { id: profile.id, name: values.name, identity: values.identity },
      {
        onSuccess: (saved) => {
          setChanged(changedRows(profile, saved));
          setSaves((count) => count + 1);
          toast.success("Browser profile updated");
        },
      },
    );
  }

  return (
    <div>
      <PageHeader
        crumbs={[{ label: "Browser profiles", href: "/browser-profiles" }]}
        title={profile.name}
        meta={
          <>
            <Badge variant="outline">{profile.browser}</Badge>
            {identity ? (
              <Badge>
                {identity.brand} on {identity.os} {identity.os_version}
              </Badge>
            ) : (
              <Badge variant="dotted">plain browser</Badge>
            )}
            {identity?.proxy_set ? <Badge variant="outline">proxy</Badge> : null}
            <span className="font-mono text-micro text-fg-muted">
              {profile.cookies.length} cookies · {profile.storage_origins.length} origins
            </span>
            <span className="font-mono text-micro text-fg-muted">
              updated <Timestamp value={profile.updated_at} />
            </span>
            <span className="flex items-center gap-1 font-mono text-micro text-fg-faint">
              {profile.id}
              <CopyButton value={profile.id} label="Copy profile ID" />
            </span>
          </>
        }
        actions={
          <>
            <Button size="sm" variant="ghost" onClick={() => setRevealing(!revealing)}>
              {revealing ? "Hide state" : "State"}
            </Button>
            <Button
              size="sm"
              variant="destructive"
              onClick={() => {
                remove.reset();
                setDoomed(true);
              }}
            >
              Delete
            </Button>
          </>
        }
      />

      <div className="grid gap-4 p-6 xl:grid-cols-3">
        <Panel label="Identity" className="xl:col-span-2">
          <ProfileForm
            key={saves}
            profile={profile}
            submitLabel="Save profile"
            pending={update.isPending}
            error={update.isError ? <InlineError error={update.error} /> : null}
            onSubmit={save}
          />
        </Panel>

        <Panel
          label="Record"
          footer={
            changed.length > 0
              ? `Last save changed ${changed.length === 1 ? "1 field" : `${changed.length} fields`}`
              : undefined
          }
        >
          <KeyValue
            rows={[
              ...profileSummary(profile)
                .filter((row) => !IN_HEADER.includes(row.label))
                .map((row) => ({
                  label: row.label,
                  value: changed.includes(row.label) ? (
                    <span className="inline-flex items-baseline gap-2">
                      {row.value}
                      <Badge variant="secondary">changed</Badge>
                    </span>
                  ) : (
                    row.value
                  ),
                })),
              { label: "Created", value: <Timestamp value={profile.created_at} /> },
            ]}
          />
        </Panel>

        {revealing ? (
          <ProfileState id={profile.id} onHide={() => setRevealing(false)} />
        ) : null}
      </div>

      <ConfirmDeleteDialog
        open={doomed}
        onOpenChange={setDoomed}
        kind="browser profile"
        name={profile.name}
        consequence="Its cookies and stored logins go with it. Anything signed in through this profile will have to sign in again."
        error={remove.isError ? String(remove.error) : undefined}
        pending={remove.isPending}
        onConfirm={() =>
          remove.mutate(profile.id, {
            onSuccess: () => {
              toast.success("Browser profile deleted");
              router.push("/browser-profiles");
            },
          })
        }
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
function ProfileState({ id, onHide }: { id: string; onHide: () => void }) {
  const query = useBrowserProfileStateQuery(id);

  return (
    <Panel
      label="State"
      className="xl:col-span-3"
      actions={
        <Button size="sm" variant="ghost" onClick={onHide}>
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
        <p className="mt-3 text-fg-muted">Loading…</p>
      ) : (
        <div className="mt-3 space-y-3">
          <KeyValue
            columns={2}
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
