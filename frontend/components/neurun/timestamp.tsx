"use client";

import { useSyncExternalStore } from "react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { usePreferences } from "@/lib/preferences/store";
import { formatAbsolute, formatIso, formatRelative, parseInstant } from "@/lib/view/time";
import { NO_VALUE } from "@/lib/view/units";

/* -------------------------------------------------------------------------- */
/* A single shared clock                                                       */
/* -------------------------------------------------------------------------- */

/**
 * Relative labels are refreshed from one 30-second tick shared by every
 * `Timestamp` on the page. Per-component intervals would mean a render storm on
 * a dense table, and a per-second tick would buy precision nobody reads.
 */
const TICK_MS = 30_000;

let tickHandle: ReturnType<typeof setInterval> | null = null;
let tickValue = 0;
const listeners = new Set<() => void>();

function subscribe(listener: () => void) {
  listeners.add(listener);
  if (!tickHandle) {
    tickHandle = setInterval(() => {
      tickValue += 1;
      for (const notify of listeners) notify();
    }, TICK_MS);
  }
  return () => {
    listeners.delete(listener);
    if (listeners.size === 0 && tickHandle) {
      clearInterval(tickHandle);
      tickHandle = null;
    }
  };
}

function useTick() {
  return useSyncExternalStore(
    subscribe,
    () => tickValue,
    () => 0,
  );
}

const noopSubscribe = () => () => {};

/**
 * True only after hydration. Relative time is computed against the client's
 * clock, so the server pass must render the exact value instead — otherwise the
 * two disagree and React reports a mismatch.
 */
function useHydrated() {
  return useSyncExternalStore(
    noopSubscribe,
    () => true,
    () => false,
  );
}

/* -------------------------------------------------------------------------- */

export interface TimestampProps {
  /** RFC 3339 string from the API. */
  value: string | undefined | null;
  className?: string;
  /** Show the exact value inline instead of relative-with-tooltip. */
  absolute?: boolean;
}

/**
 * An instant, shown relative with the exact value one hover — or one focus —
 * away. The original instant is preserved; UTC versus local is a display
 * choice the user makes in the shell.
 */
export function Timestamp({ value, className, absolute = false }: TimestampProps) {
  const { timeZone } = usePreferences();
  useTick();
  const hydrated = useHydrated();

  const date = parseInstant(value);
  if (!date) {
    return (
      <span className={cn("font-mono text-caption text-fg-muted", className)} aria-label="No value">
        {NO_VALUE}
      </span>
    );
  }

  const exact = formatAbsolute(date, timeZone);

  if (absolute || !hydrated) {
    return (
      <time dateTime={formatIso(date)} className={cn("font-mono text-caption", className)}>
        {exact}
      </time>
    );
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <time
          dateTime={formatIso(date)}
          className={cn(
            "cursor-help font-mono text-caption underline decoration-dotted decoration-from-font underline-offset-3",
            className,
          )}
        >
          {formatRelative(date)}
        </time>
      </TooltipTrigger>
      <TooltipContent>
        <span className="font-mono text-micro">{exact}</span>
      </TooltipContent>
    </Tooltip>
  );
}
