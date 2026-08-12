"use client";

import { useState } from "react";
import Link from "next/link";
import {
  Cookie,
  Cpu,
  Database,
  Laptop,
  MemoryStick,
  Monitor,
  Plus,
  Smartphone,
} from "lucide-react";
import { toast } from "sonner";

import { Fact } from "@/components/browser-profiles/fact";
import { BrowserIcon } from "@/components/neurun/browser-icon";
import { ConfirmDeleteDialog } from "@/components/neurun/confirm-delete-dialog";
import { ErrorPanel } from "@/components/neurun/error-panel";
import { EmptyState } from "@/components/neurun/feedback";
import { PageHeader } from "@/components/neurun/page-header";
import { Timestamp } from "@/components/neurun/timestamp";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  useBrowserProfilesQuery,
  useDeleteBrowserProfileMutation,
} from "@/lib/api/queries";
import type { BrowserProfile } from "@/lib/api/resource-types";

export default function BrowserProfilesPage() {
  const query = useBrowserProfilesQuery();
  const remove = useDeleteBrowserProfileMutation();
  const [doomed, setDoomed] = useState<BrowserProfile | null>(null);

  return (
    <div>
      <PageHeader
        title="Browser profiles"
        description="Who a browser appears to be, and what it remembers between sessions."
      />
      <div className="space-y-4 p-6">
        <Button asChild>
          <Link href="/browser-profiles/new">
            <Plus aria-hidden />
            Create profile
          </Link>
        </Button>

        {query.isError ? (
          <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
        ) : query.isPending ? (
          <p className="text-fg-muted">Loading…</p>
        ) : query.data.browser_profiles.length === 0 ? (
          <EmptyState
            title="No browser profiles"
            description="A profile keeps its cookies and storage between sessions."
          />
        ) : (
          <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {query.data.browser_profiles.map((profile) => (
              <ProfileCard
                key={profile.id}
                profile={profile}
                onDelete={() => {
                  remove.reset();
                  setDoomed(profile);
                }}
              />
            ))}
          </ul>
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

/**
 * One profile as it looks from outside: the browser it launches, the machine it
 * presents as, and what it remembers. A profile with no identity says so — that
 * is the difference between a persona and a plain browser.
 */
function ProfileCard({
  profile,
  onDelete,
}: {
  profile: BrowserProfile;
  onDelete: () => void;
}) {
  const identity = profile.identity;
  const mobile = identity?.os === "Android" || identity?.os === "Ios";

  return (
    <li className="flex min-w-0 flex-col gap-2 rounded-lg border border-line bg-surface-panel p-3">
      <div className="flex items-start justify-between gap-2">
        <Link
          href={`/browser-profiles/${profile.id}`}
          className="min-w-0 truncate font-medium underline-offset-4 hover:underline"
        >
          {profile.name}
        </Link>
        <div className="-my-1 flex shrink-0 items-center">
          <Button asChild size="sm" variant="ghost">
            <Link href={`/browser-profiles/${profile.id}`}>Edit</Link>
          </Button>
          <Button size="sm" variant="ghost" onClick={onDelete}>
            Delete
          </Button>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1 font-mono text-micro text-fg-secondary">
        <Fact icon={<BrowserIcon browser={profile.browser} />}>{profile.browser}</Fact>
        {identity ? (
          <>
            <Fact icon={mobile ? <Smartphone /> : <Laptop />}>
              {identity.os.toLowerCase()} {identity.os_version}
            </Fact>
            <Fact icon={<Monitor />}>
              {identity.screen?.logical_width}×{identity.screen?.logical_height}
            </Fact>
            <Fact icon={<Cpu />}>{identity.hardware_concurrency} cores</Fact>
            <Fact icon={<MemoryStick />}>{identity.memory} GiB</Fact>
            {/* The claim is only worth saying when it is not the engine's own name. */}
            {identity.brand !== profile.browser ? (
              <Badge variant="outline">as {identity.brand}</Badge>
            ) : null}
            {identity.proxy_set ? <Badge variant="outline">proxy</Badge> : null}
          </>
        ) : (
          <span className="font-sans text-micro font-semibold text-fg-muted">
            · no-stealth
          </span>
        )}
      </div>

      <div className="flex flex-wrap items-center gap-x-2.5 font-mono text-micro text-fg-muted">
        <Fact icon={<Cookie />}>{profile.cookies.length}</Fact>
        <Fact icon={<Database />}>{profile.storage_origins.length}</Fact>
        <Timestamp value={profile.updated_at} />
      </div>
    </li>
  );
}
