"use client";

import { useCallback, useMemo, useState } from "react";

/**
 * Opaque-cursor pagination.
 *
 * A cursor is never parsed, sorted or synthesised in the browser — the only
 * legal operations are "send back what the server gave me" and "recognise the
 * empty string as end-of-results". Going backwards therefore means remembering
 * the cursors already visited, not computing one.
 */
export interface CursorPages {
  /** Cursor for the current page; `undefined` on the first page. */
  cursor: string | undefined;
  pageIndex: number;
  canGoBack: boolean;
  /** Advance using the `next_cursor` the server returned. */
  next: (nextCursor: string) => void;
  back: () => void;
  /** Return to the first page — after a filter change, for instance. */
  reset: () => void;
}

export function useCursorPages(): CursorPages {
  // history[0] is always the first page, represented by `undefined`.
  const [history, setHistory] = useState<(string | undefined)[]>([undefined]);

  const next = useCallback((nextCursor: string) => {
    // An empty string means there is no next page; refuse to paginate past it.
    if (!nextCursor) return;
    setHistory((previous) => [...previous, nextCursor]);
  }, []);

  const back = useCallback(() => {
    setHistory((previous) => (previous.length > 1 ? previous.slice(0, -1) : previous));
  }, []);

  const reset = useCallback(() => setHistory([undefined]), []);

  return useMemo(
    () => ({
      cursor: history[history.length - 1],
      pageIndex: history.length - 1,
      canGoBack: history.length > 1,
      next,
      back,
      reset,
    }),
    [history, next, back, reset],
  );
}
