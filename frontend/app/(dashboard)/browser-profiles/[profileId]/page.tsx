"use client";

import { useState } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  Cookie,
  Cpu,
  Database,
  Laptop,
  MemoryStick,
  Monitor,
  Smartphone,
} from "lucide-react";
import { toast } from "sonner";

import { ConfirmFingerprintChange } from "@/components/browser-profiles/confirm-fingerprint-change";
import { Fact } from "@/components/browser-profiles/fact";
import { BrowserIcon } from "@/components/neurun/browser-icon";
import { ProfileForm, type ProfileValues } from "@/components/browser-profiles/profile-form";
import { ConfirmDeleteDialog } from "@/components/neurun/confirm-delete-dialog";
import { CopyButton } from "@/components/neurun/copy-id";
import { ErrorPanel, InlineError } from "@/components/neurun/error-panel";
import { Callout } from "@/components/neurun/feedback";
import { JsonView } from "@/components/neurun/json-view";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { Timestamp } from "@/components/neurun/timestamp";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  useBrowserProfileQuery,
  useBrowserProfileStateQuery,
  useDeleteBrowserProfileMutation,
  useUpdateBrowserProfileMutation,
} from "@/lib/api/queries";
import type { BrowserProfile } from "@/lib/api/resource-types";
import { identityChanged, versionMove } from "@/lib/view/browser-identity";

export default function BrowserProfilePage() {
  const { profileId } = useParams<{ profileId: string }>();
  const router = useRouter();
  const query = useBrowserProfileQuery(profileId);
  const update = useUpdateBrowserProfileMutation();
  const remove = useDeleteBrowserProfileMutation();

  // A counter that remounts the form on a save, so the fields are re-read from
  // the stored profile rather than from the draft that produced it — the proxy
  // in particular, which never comes back.
  const [saves, setSaves] = useState(0);
  const [revealing, setRevealing] = useState(false);
  const [doomed, setDoomed] = useState(false);
  // A save held back until the fingerprint change is confirmed.
  const [pending, setPending] = useState<ProfileValues | null>(null);

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
  const remembers = profile.cookies.length > 0 || profile.storage_origins.length > 0;
  const mobile = identity.os === "Android" || identity.os === "Ios";

  function commit(values: ProfileValues) {
    update.mutate(
      { id: profile.id, name: values.name, identity: values.identity },
      {
        onSuccess: (saved) => {
          setSaves((count) => count + 1);
          setPending(null);
          toast.success("Browser profile updated", {
            description:
              versionMove(identity, saved.identity) === "forward"
                ? "The browser version moved forward, which is what a real install does on its own."
                : undefined,
          });
        },
      },
    );
  }

  /**
   * Two changes a machine cannot make on its own, both gated behind a typed
   * confirmation: moving a field the fingerprint is seeded from while the
   * profile keeps the cookies that outlived it, and moving the browser version
   * backwards. Everything else — a rename, a forward version, a locale — saves
   * straight through.
   */
  function objections(values: ProfileValues): string[] {
    const reasons: string[] = [];
    if (remembers && identityChanged(identity, values.identity)) {
      reasons.push(
        "The identity, on a profile that already remembers something — its canvas, " +
          "audio and WebGL hashes move with it, while the cookies do not",
      );
    }
    if (versionMove(identity, values.identity) === "backward") {
      reasons.push(
        `The browser version, from ${(identity?.browser_version ?? []).join(".")} ` +
          `back to ${(values.identity?.browser_version ?? []).join(".")} — installs only update forwards`,
      );
    }
    return reasons;
  }

  function save(values: ProfileValues) {
    if (objections(values).length > 0) {
      update.reset();
      setPending(values);
      return;
    }
    commit(values);
  }

  return (
    <div>
      <PageHeader
        crumbs={[{ label: "Browser profiles", href: "/browser-profiles" }]}
        title={profile.name}
        meta={
          <>
            <Fact icon={<BrowserIcon browser={profile.browser} />}>{profile.browser}</Fact>
            <Fact icon={mobile ? <Smartphone /> : <Laptop />}>
              {identity.os.toLowerCase()} {identity.os_version}
            </Fact>
            <Fact icon={<Monitor />}>
              {identity.screen?.logical_width}×{identity.screen?.logical_height}
            </Fact>
            <Fact icon={<Cpu />}>{identity.hardware_concurrency} cores</Fact>
            <Fact icon={<MemoryStick />}>{identity.memory} GiB</Fact>
            {identity.proxy_set ? <Badge variant="outline">proxy</Badge> : null}
            <Fact icon={<Cookie />}>{profile.cookies.length}</Fact>
            <Fact icon={<Database />}>{profile.storage_origins.length}</Fact>
            <Timestamp value={profile.updated_at} />
            <span className="flex items-center gap-1 font-mono text-micro text-fg-faint">
              {profile.id}
              <CopyButton value={profile.id} label="Copy profile ID" />
            </span>
          </>
        }
        actions={
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

        <ProfileState
          profile={profile}
          revealing={revealing}
          onReveal={() => setRevealing(!revealing)}
        />
      </div>

      <ConfirmFingerprintChange
        open={pending !== null}
        onOpenChange={(next) => !next && setPending(null)}
        changes={pending ? objections(pending) : []}
        cookies={profile.cookies.length}
        origins={profile.storage_origins.length}
        error={update.isError ? String(update.error) : undefined}
        pending={update.isPending}
        onConfirm={() => pending && commit(pending)}
      />

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
 * What the profile remembers.
 *
 * The names, domains and sizes come free with the profile — the list response
 * carries them redacted, which is enough to see what it is signed into and to
 * decide to delete it. The values are a separate, write-scoped call that returns
 * live credentials, so they are fetched only when asked for and never cached.
 */
function ProfileState({
  profile,
  revealing,
  onReveal,
}: {
  profile: BrowserProfile;
  revealing: boolean;
  onReveal: () => void;
}) {
  const remembers = profile.cookies.length > 0 || profile.storage_origins.length > 0;

  return (
    <Panel
      label="Remembers"
      actions={
        remembers ? (
          <Button size="sm" variant="ghost" onClick={onReveal}>
            {revealing ? "Hide values" : "Reveal values"}
          </Button>
        ) : undefined
      }
      footer={`${profile.cookies.length} cookies · ${profile.storage_origins.length} origins`}
    >
      {!remembers ? (
        <p className="text-caption text-fg-muted">
          Nothing yet. A session writes its cookies and storage back when it closes.
        </p>
      ) : (
        <div className="space-y-3">
          <ScrollArea className="max-h-64">
            <ul className="space-y-1">
              {profile.cookies.map((cookie) => (
                <li
                  key={`${cookie.domain}${cookie.path}${cookie.name}`}
                  className="flex items-baseline justify-between gap-3 font-mono text-micro"
                >
                  <span className="min-w-0 truncate text-fg-secondary">{cookie.name}</span>
                  <span className="min-w-0 flex-1 truncate text-fg-muted">
                    {cookie.domain}
                  </span>
                  <span className="shrink-0 text-fg-faint">{cookie.value_size} B</span>
                </li>
              ))}
              {profile.storage_origins.map((origin) => (
                <li key={origin} className="truncate font-mono text-micro text-fg-muted">
                  {origin}
                </li>
              ))}
            </ul>
          </ScrollArea>

          {revealing ? <RevealedState id={profile.id} /> : null}
        </div>
      )}
    </Panel>
  );
}

/** The one response that carries credentials. Never cached. */
function RevealedState({ id }: { id: string }) {
  const query = useBrowserProfileStateQuery(id);

  return (
    <div className="space-y-3 border-t border-line pt-3">
      <Callout kind="warning" title="These are live credentials">
        Session cookies here are as good as being signed in. Copying them out is
        exporting the account.
      </Callout>
      {query.isError ? (
        <InlineError error={query.error} />
      ) : query.isPending ? (
        <p className="text-fg-muted">Loading…</p>
      ) : (
        <JsonView value={query.data} preRedacted />
      )}
    </div>
  );
}
