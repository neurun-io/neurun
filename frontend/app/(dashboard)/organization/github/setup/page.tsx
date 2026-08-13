"use client";

/**
 * Where GitHub sends the browser after an install.
 *
 * This page exists because the session cookie is SameSite=Strict: the hop from
 * github.com is cross-site, so the control plane would not see the cookie if
 * GitHub redirected there directly. Landing on the dashboard first turns the
 * next request into a same-site one, which does carry it.
 */

import { use, useEffect, useRef } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";

import { InlineError } from "@/components/neurun/error-panel";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { Button } from "@/components/ui/button";
import { useRecordInstallationMutation } from "@/lib/api/queries";

export default function Page({
  searchParams,
}: {
  searchParams: Promise<{ installation_id?: string; setup_action?: string }>;
}) {
  const { installation_id: installationId, setup_action: action } = use(searchParams);
  const record = useRecordInstallationMutation();
  const router = useRouter();
  // GitHub sends the browser here on install and again on every configure, and
  // React runs effects twice in development; recording once is the point.
  const attempted = useRef(false);

  useEffect(() => {
    if (!installationId || attempted.current) return;
    attempted.current = true;
    record.mutate(
      { installationId, accountLogin: "" },
      {
        onSuccess: (installation) => {
          toast.success(`Connected ${installation.account_login || "GitHub"}`);
          router.replace("/organization");
        },
      },
    );
  }, [installationId, record, router]);

  return (
    <div>
      <PageHeader
        title="Connecting GitHub"
        description="Recording the installation GitHub just handed back."
      />
      <div className="space-y-4 p-6">
        <Panel label={action === "update" ? "Installation updated" : "Installation"}>
          {!installationId ? (
            <div className="space-y-3">
              <p className="text-sm text-fg-muted">
                GitHub did not send an installation id. Start the install from the organization
                page rather than opening this one directly.
              </p>
              <Button asChild size="sm" variant="secondary">
                <Link href="/organization">Back to organization</Link>
              </Button>
            </div>
          ) : record.isError ? (
            <div className="space-y-3">
              <InlineError error={record.error} />
              <Button asChild size="sm" variant="secondary">
                <Link href="/organization">Back to organization</Link>
              </Button>
            </div>
          ) : (
            <p className="text-sm text-fg-muted">Recording installation {installationId}…</p>
          )}
        </Panel>
      </div>
    </div>
  );
}
