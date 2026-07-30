"use client";

import { useState } from "react";
import { useParams } from "next/navigation";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { PageHeader } from "@/components/neurun/page-header";
import { StatusBadge } from "@/components/neurun/status-badge";
import { ErrorPanel, InlineError } from "@/components/neurun/error-panel";
import { InvocationResult } from "@/components/functions/invocation-result";
import { useCancelInvocationMutation, useInvocationQuery } from "@/lib/api/queries";
import { isTerminalInvocationStatus } from "@/lib/api/types";

export default function InvocationDetailPage() {
  const params = useParams<{ invocationId: string }>();
  const invocationId = params.invocationId;
  const query = useInvocationQuery(invocationId);
  const [cancelOpen, setCancelOpen] = useState(false);

  if (query.isError) {
    return (
      <div className="px-6 py-6">
        <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
      </div>
    );
  }

  if (query.isPending) {
    return (
      <div className="space-y-4 px-6 py-6" role="status" aria-label="Loading invocation">
        <Skeleton className="h-8 w-72" />
        <Skeleton className="h-48 w-full" />
      </div>
    );
  }

  const invocation = query.data;
  const terminal = isTerminalInvocationStatus(invocation.status);
  // Job-owned execution is canceled through its job, not here.
  const jobOwned = Boolean(invocation.context?.job_id);

  return (
    <div className="flex min-h-full flex-col">
      <PageHeader
        crumbs={[{ label: "Invocations", href: "/invocations" }, { label: invocation.invocation_id }]}
        title={
          <span className="flex flex-wrap items-center gap-3">
            <span className="font-mono text-xl">{invocation.invocation_id}</span>
            <StatusBadge status={invocation.status} />
          </span>
        }
        meta={
          !terminal ? (
            <span className="inline-flex items-center gap-1.5 font-mono text-micro text-fg-muted">
              <Loader2 aria-hidden className="size-3 animate-spin" strokeWidth={1.5} />
              polling every 2s
            </span>
          ) : null
        }
        actions={
          <Button
            variant="destructive"
            size="sm"
            onClick={() => setCancelOpen(true)}
            disabled={terminal || jobOwned}
            aria-disabled={terminal || jobOwned || undefined}
            title={
              jobOwned
                ? "Job-owned execution is canceled through its job."
                : terminal
                  ? "This invocation has already reached a terminal state."
                  : undefined
            }
          >
            Cancel invocation
          </Button>
        }
      />

      <div className="mx-auto w-full max-w-4xl px-6 py-4">
        <InvocationResult invocation={invocation} />
      </div>

      <CancelInvocationDialog
        invocationId={invocation.invocation_id}
        open={cancelOpen}
        onOpenChange={setCancelOpen}
      />
    </div>
  );
}

function CancelInvocationDialog({
  invocationId,
  open,
  onOpenChange,
}: {
  invocationId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const cancel = useCancelInvocationMutation();

  async function confirm() {
    try {
      await cancel.mutateAsync(invocationId);
      onOpenChange(false);
      toast.success(`Cancellation signal delivered to ${invocationId}.`);
    } catch {
      // Rendered inline; the dialog stays open.
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Cancel {invocationId}?</DialogTitle>
          <DialogDescription>
            A cancellation signal is delivered to the running direct invocation. Evidence recorded
            so far is kept.
          </DialogDescription>
        </DialogHeader>

        {cancel.isError ? <InlineError error={cancel.error} /> : null}

        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)} disabled={cancel.isPending}>
            Keep running
          </Button>
          <Button variant="destructive" onClick={confirm} disabled={cancel.isPending}>
            {cancel.isPending ? "Cancelling…" : "Cancel invocation"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
