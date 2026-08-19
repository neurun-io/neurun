"use client";

/**
 * Where GitHub sends the browser after an install.
 *
 * This page exists because the session cookie is SameSite=Strict: the hop from
 * github.com is cross-site, so the control plane would not see the cookie if
 * GitHub redirected there directly. Landing on the dashboard first turns the
 * next request into a same-site one, which does carry it.
 */

import { Suspense, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { toast } from "sonner";

import { InlineError } from "@/components/neurun/error-panel";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { Button } from "@/components/ui/button";
import { useRecordInstallationMutation } from "@/lib/api/queries";

function SetupView() {
  // Read from the hook rather than the page's `searchParams` prop. Unwrapping
  // that promise suspends, and this route prerenders — with no boundary above
  // to suspend into, the id never arrived and the page reported that GitHub had
  // not sent one. Development renders on demand and never suspends, so it only
  // ever went wrong once built.
  const searchParams = useSearchParams();
  const installationId = searchParams.get("installation_id") ?? undefined;
  const action = searchParams.get("setup_action");
  const record = useRecordInstallationMutation();
  const router = useRouter();
  // GitHub sends the browser here on install and again on every configure, and
  // React runs effects twice in development; recording once is the point.
  const attempted = useRef(false);
  // How it went is kept here rather than read back off the mutation. That
  // second run of the effect is a remount, and a remount detaches a mutation
  // from the request it already started with nothing to reattach it: its
  // status, its error and its callbacks all stop moving, whatever the server
  // answers. The promise is the only part that still settles.
  const [failure, setFailure] = useState<unknown>(null);

  useEffect(() => {
    if (!installationId || attempted.current) return;
    attempted.current = true;
    record
      .mutateAsync(installationId)
      .then((installation) => {
        toast.success(`Connected ${installation.account_login}`);
        router.replace("/organization");
      })
      .catch(setFailure);
  }, [installationId, record, router]);

  const failed = !installationId || failure !== null;

  return (
    <div>
      <PageHeader
        title={failed ? "GitHub not connected" : "Connecting GitHub"}
        description={
          failed
            ? "Nothing was recorded."
            : "Recording the installation GitHub just handed back."
        }
      />
      <div className="space-y-4 p-6">
        <Panel
          label={
            failed
              ? "Not recorded"
              : action === "update"
                ? "Installation updated"
                : "Installation"
          }
        >
          {!installationId ? (
            <div className="space-y-3">
              <p className="text-sm text-fg-muted">
                GitHub did not send an installation id. Start the install from
                the organization page rather than opening this one directly.
              </p>
              <Button asChild size="sm" variant="secondary">
                <Link href="/organization">Back to organization</Link>
              </Button>
            </div>
          ) : failure !== null ? (
            <div className="space-y-3">
              <InlineError error={failure} />
              <Button asChild size="sm" variant="secondary">
                <Link href="/organization">Back to organization</Link>
              </Button>
            </div>
          ) : (
            <p className="text-sm text-fg-muted">
              Recording installation {installationId}…
            </p>
          )}
        </Panel>
      </div>
    </div>
  );
}

export default function Page() {
  return (
    <Suspense fallback={null}>
      <SetupView />
    </Suspense>
  );
}
