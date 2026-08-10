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
 * A destructive confirmation that only unlocks once the user types the
 * resource's own name, the way GitHub and AWS gate an irreversible delete.
 *
 * The name rather than the identifier: the identifier is already on screen next
 * to the button, so echoing it is a copy-paste rather than a decision. Typing
 * the name means reading which resource this is.
 *
 * The gate lives here and not in the API. A programmatic client confirms
 * nothing — it already said what it wanted by calling DELETE. This is a
 * property of the interface a person drives.
 */
export interface ConfirmDeleteDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** What is being destroyed, e.g. `project` or `app`. Lower case. */
  kind: string;
  /** The exact string the user must type. */
  name: string;
  /** What else disappears. State the cascade plainly. */
  consequence: ReactNode;
  /** Surfaced above the actions when the delete itself fails. */
  error?: ReactNode;
  pending?: boolean;
  onConfirm: () => void;
}

export function ConfirmDeleteDialog({
  open,
  onOpenChange,
  kind,
  name,
  consequence,
  error,
  pending = false,
  onConfirm,
}: ConfirmDeleteDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        {/*
          Keyed by the resource, so opening the dialog for a different one
          remounts the form. Carrying a previous attempt's text across would let
          a second delete arrive already confirmed.
        */}
        <ConfirmDeleteForm
          key={name}
          kind={kind}
          name={name}
          consequence={consequence}
          error={error}
          pending={pending}
          onCancel={() => onOpenChange(false)}
          onConfirm={onConfirm}
        />
      </DialogContent>
    </Dialog>
  );
}

function ConfirmDeleteForm({
  kind,
  name,
  consequence,
  error,
  pending,
  onCancel,
  onConfirm,
}: {
  kind: string;
  name: string;
  consequence: ReactNode;
  error?: ReactNode;
  pending: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const [typed, setTyped] = useState("");
  const inputId = useId();
  const matches = typed === name;

  return (
    <>
      <DialogHeader>
        <DialogTitle>Delete {kind}?</DialogTitle>
        <DialogDescription>{consequence}</DialogDescription>
      </DialogHeader>

      <form
        className="grid gap-2"
        onSubmit={(event) => {
          event.preventDefault();
          if (matches && !pending) onConfirm();
        }}
      >
        <Label htmlFor={inputId} className="text-caption text-fg-muted">
          Type <span className="font-mono text-fg">{name}</span> to confirm
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
          {pending ? `Deleting ${kind}…` : `Delete ${kind}`}
        </Button>
      </DialogFooter>
    </>
  );
}
