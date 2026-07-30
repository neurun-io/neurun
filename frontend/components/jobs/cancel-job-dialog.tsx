"use client";

import { useState } from "react";
import { toast } from "sonner";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { InlineError } from "@/components/neurun/error-panel";
import { useCancelJobMutation } from "@/lib/api/queries";
import { isTerminalJobState, type Job } from "@/lib/api/types";

/**
 * Cancellation confirmation.
 *
 * Cancellation is intrinsically idempotent for an already-terminal job, so this
 * carries no `Idempotency-Key` — but it still asks, because it stops in-flight
 * work an operator may not be able to restart.
 */
export function CancelJobDialog({
  job,
  open,
  onOpenChange,
}: {
  job: Job;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [reason, setReason] = useState("");
  const cancel = useCancelJobMutation();

  async function confirm() {
    try {
      const result = await cancel.mutateAsync({
        jobId: job.id,
        reason: reason.trim() || undefined,
      });
      onOpenChange(false);
      setReason("");
      toast.success(
        result.duplicate
          ? "This job had already reached a terminal state."
          : `Cancellation recorded for ${job.id}.`,
      );
    } catch {
      // Rendered inline below; the dialog stays open so the operator can retry.
    }
  }

  const alreadyTerminal = isTerminalJobState(job.state);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Cancel {job.id}?</DialogTitle>
          <DialogDescription>
            Job-owned execution is cancelled through the job API. The in-flight attempt is marked
            canceled and every event recorded so far is kept.
          </DialogDescription>
        </DialogHeader>

        {alreadyTerminal ? (
          <p className="text-sm text-fg-muted">
            This job is already{" "}
            <code className="font-mono text-caption text-fg-secondary">{job.state}</code>.
            Cancelling again is a no-op.
          </p>
        ) : null}

        <div className="space-y-1.5">
          <Label htmlFor="cancel-reason">Reason (optional)</Label>
          <Textarea
            id="cancel-reason"
            value={reason}
            onChange={(event) => setReason(event.target.value)}
            maxLength={512}
            rows={3}
            placeholder="Recorded with the cancellation."
          />
          <p className="text-micro text-fg-muted">{reason.length}/512</p>
        </div>

        {cancel.isError ? <InlineError error={cancel.error} /> : null}

        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)} disabled={cancel.isPending}>
            Keep running
          </Button>
          <Button variant="destructive" onClick={confirm} disabled={cancel.isPending}>
            {cancel.isPending ? "Cancelling…" : "Cancel job"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
