"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Check, Copy } from "lucide-react";
import { cn } from "@/lib/utils";

export interface CopyButtonProps {
  value: string;
  /** Announced to assistive technology, e.g. "Copy request ID". */
  label: string;
  className?: string;
}

/**
 * Copy-to-clipboard with a transient confirmation.
 *
 * The confirmation is text as well as glyph, and it is announced politely — an
 * user using a screen reader needs to know the copy landed too.
 */
export function CopyButton({ value, label, className }: CopyButtonProps) {
  const [copied, setCopied] = useState(false);
  const timeout = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (timeout.current) clearTimeout(timeout.current);
    };
  }, []);

  const copy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      if (timeout.current) clearTimeout(timeout.current);
      timeout.current = setTimeout(() => setCopied(false), 1600);
    } catch {
      // Clipboard access can be denied. The value stays selectable on screen.
    }
  }, [value]);

  return (
    <button
      type="button"
      onClick={copy}
      className={cn(
        "inline-flex size-6 shrink-0 items-center justify-center rounded-xs text-fg-muted",
        "transition-colors duration-120 ease-mech hover:bg-surface-hover hover:text-fg",
        className,
      )}
      aria-label={label}
    >
      {copied ? (
        <Check aria-hidden className="size-3" strokeWidth={1.5} />
      ) : (
        <Copy aria-hidden className="size-3" strokeWidth={1.5} />
      )}
      <span role="status" aria-live="polite" className="sr-only">
        {copied ? `${label} — copied` : ""}
      </span>
    </button>
  );
}

export interface CopyIdProps {
  value: string;
  label?: string;
  /** Show only the first and last characters, with the middle elided. */
  truncate?: boolean;
  className?: string;
}

/**
 * A copyable identifier. Identifiers are mono, always selectable, and never
 * abbreviated in a way that loses the value — truncation is visual only, the
 * full string is what gets copied and what a title reveals.
 */
export function CopyId({ value, label = "identifier", truncate = false, className }: CopyIdProps) {
  const display = truncate && value.length > 20 ? `${value.slice(0, 10)}…${value.slice(-6)}` : value;

  return (
    <span className={cn("inline-flex min-w-0 items-center gap-1", className)}>
      <code className="min-w-0 truncate font-mono text-caption text-fg-secondary" title={value}>
        {display}
      </code>
      <CopyButton value={value} label={`Copy ${label}`} />
    </span>
  );
}

/**
 * A digest, shown at the length a user can actually compare, with the full
 * value one copy away. The resolved digest is never hidden — it is the proof
 * that a run used the code you think it used.
 */
export function Digest({ value, className }: { value: string; className?: string }) {
  const short = value.startsWith("sha256:")
    ? `sha256:${value.slice(7, 15)}…${value.slice(-4)}`
    : value;

  return (
    <span className={cn("inline-flex min-w-0 items-center gap-1", className)}>
      <code className="font-mono text-caption text-fg-muted" title={value}>
        {short}
      </code>
      <CopyButton value={value} label="Copy digest" />
    </span>
  );
}
