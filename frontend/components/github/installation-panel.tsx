"use client";

import { useState } from "react";
import { Cable } from "lucide-react";
import { toast } from "sonner";

import { InstallLink } from "@/components/github/install-link";
import { ConfirmDeleteDialog } from "@/components/neurun/confirm-delete-dialog";
import { InlineError } from "@/components/neurun/error-panel";
import { EmptyState } from "@/components/neurun/feedback";
import { KeyValue } from "@/components/neurun/key-value";
import { Timestamp } from "@/components/neurun/timestamp";
import { Button } from "@/components/ui/button";
import {
  useDeleteInstallationMutation,
  useInstallationQuery,
} from "@/lib/api/queries";

export function InstallationPanel() {
  const query = useInstallationQuery();
  const remove = useDeleteInstallationMutation();
  const [confirming, setConfirming] = useState(false);
  const installation = query.data;

  return (
    <div className="space-y-3">
      {query.isPending ? (
        <p className="text-fg-muted">Loading…</p>
      ) : query.isError ? (
        <InlineError error={query.error} />
      ) : installation ? (
        <>
          <KeyValue
            rows={[
              { label: "Account", value: installation.account_login },
              { label: "Installation", value: String(installation.installation_id) },
              { label: "Connected", value: <Timestamp value={installation.created_at} /> },
            ]}
          />
          <div className="flex items-center gap-2">
            <InstallLink label="Configure on GitHub" variant="secondary" />
            <Button
              size="sm"
              variant="ghost"
              onClick={() => {
                remove.reset();
                setConfirming(true);
              }}
            >
              Disconnect
            </Button>
          </div>
        </>
      ) : (
        <EmptyState
          className="nr-blink-icon"
          icon={Cable}
          title="GitHub is not connected"
          action={<InstallLink label="Install on GitHub" />}
        />
      )}

      <ConfirmDeleteDialog
        open={confirming}
        onOpenChange={(next) => !next && setConfirming(false)}
        kind="GitHub installation"
        name={installation?.account_login ?? ""}
        consequence="Pushes stop deploying and repository deploys refuse until it is connected again. The app stays installed on GitHub until you remove it there too."
        error={remove.isError ? String(remove.error) : undefined}
        pending={remove.isPending}
        onConfirm={() =>
          remove.mutate(undefined, {
            onSuccess: () => {
              setConfirming(false);
              toast.success("GitHub disconnected");
            },
          })
        }
      />
    </div>
  );
}

