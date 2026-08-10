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
