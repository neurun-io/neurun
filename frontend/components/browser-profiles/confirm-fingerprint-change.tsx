"use client";

import { useId, useState, type ReactNode } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

/**
 * The gate on a save that claims something a machine cannot do.
 *
 * Two get here. Moving a field the fingerprint is seeded from gives the profile
 * a new canvas, audio and WebGL signature while it keeps the cookies that
 * outlived it — to every site it is signed into, one account has moved to a
 * different computer. Moving the browser version backwards claims an install
 * that downgraded itself, which does not happen.
 *
 * The typed confirmation is here because the cost is invisible: the save
 * succeeds, and the run fails a week later.
 */
export function ConfirmFingerprintChange({
  open,
  onOpenChange,
  changes,
  cookies,
  origins,
  error,
  pending = false,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The seeded fields this save would move. */
  changes: string[];
  cookies: number;
  origins: number;
  error?: ReactNode;
  pending?: boolean;
  onConfirm: () => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        {/* Keyed so reopening never arrives already confirmed. */}
        <ConfirmForm
          key={changes.join("|")}
          changes={changes}
          cookies={cookies}
          origins={origins}
          error={error}
          pending={pending}
          onCancel={() => onOpenChange(false)}
          onConfirm={onConfirm}
        />
      </DialogContent>
    </Dialog>
  );
}

const REQUIRED = "confirm";

function ConfirmForm({
  changes,
  cookies,
  origins,
  error,
  pending,
  onCancel,
  onConfirm,
}: {
  changes: string[];
  cookies: number;
  origins: number;
  error?: ReactNode;
  pending: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const [typed, setTyped] = useState("");
  const inputId = useId();
  const matches = typed.trim().toLowerCase() === REQUIRED;

  return (
    <>
      <DialogHeader>
        <DialogTitle>A machine cannot do this to itself</DialogTitle>
        <DialogDescription>
          This profile remembers <span className="font-mono text-fg">{cookies}</span>{" "}
          cookies across <span className="font-mono text-fg">{origins}</span> storage
          origins, so it has a past. The changes below contradict it — the sites it is
          signed into would see one account carry on from hardware that was replaced
          overnight.
        </DialogDescription>
      </DialogHeader>

      <ul className="space-y-1 rounded-md border border-line-strong p-3">
        {changes.map((change) => (
          <li key={change} className="flex gap-2 font-mono text-caption text-fg-secondary">
            <span aria-hidden className="text-fg-faint">
              —
            </span>
            <span className="min-w-0 break-words">{change}</span>
          </li>
        ))}
      </ul>

      <p className="nr-measure text-caption text-fg-muted">
        Creating a second profile keeps this one intact. Edit these fields only when the
        persona is meant to be a different machine.
      </p>

      <form
        className="grid gap-2"
        onSubmit={(event) => {
          event.preventDefault();
          if (matches && !pending) onConfirm();
        }}
      >
        <Label htmlFor={inputId} className="text-caption text-fg-muted">
          Type <span className="font-mono text-fg">{REQUIRED}</span> to change it anyway
        </Label>
        <Input
          id={inputId}
          value={typed}
          onChange={(event) => setTyped(event.target.value)}
          autoComplete="off"
          autoCorrect="off"
          spellCheck={false}
          className="font-mono"
          aria-describedby={error ? `${inputId}-error` : undefined}
          aria-invalid={error ? true : undefined}
        />
        {error ? (
          <p id={`${inputId}-error`} role="alert" className="text-caption text-fg">
            {error}
          </p>
        ) : null}
      </form>

      <DialogFooter>
        <Button variant="ghost" onClick={onCancel} disabled={pending}>
          Cancel
        </Button>
        <Button variant="destructive" onClick={onConfirm} disabled={!matches || pending}>
          {pending ? "Saving…" : "Change the fingerprint"}
        </Button>
      </DialogFooter>
    </>
  );
}
