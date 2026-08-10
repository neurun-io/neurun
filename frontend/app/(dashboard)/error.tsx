"use client";

import { useEffect } from "react";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { ErrorPanel } from "@/components/neurun/error-panel";

/**
 * Route-level error boundary.
 *
 * Scoped to the dashboard segment so a failing route falls back inside the
 * shell — the user keeps their navigation and can move somewhere else
 * instead of losing the whole application.
 */
export default function DashboardError({
  error,
  unstable_retry,
}: {
  error: Error & { digest?: string };
  unstable_retry: () => void;
}) {
  useEffect(() => {
    // Deliberately console-only: no error-reporting transport is configured,
    // and breadcrumbs are one of the places an API key must never end up.
    console.error(error);
  }, [error]);

  return (
    <div className="mx-auto w-full max-w-2xl px-6 py-10">
      <ErrorPanel
        error={error}
        title="This route failed to render"
        onRetry={() => unstable_retry()}
      />
      <div className="mt-4 flex gap-2">
        <Button variant="secondary" size="sm" onClick={() => unstable_retry()}>
          Try again
        </Button>
        <Button variant="ghost" size="sm" asChild>
          <Link href="/jobs">Back to jobs</Link>
        </Button>
      </div>
      {error.digest ? (
        <p className="mt-3 font-mono text-micro text-fg-muted">digest {error.digest}</p>
      ) : null}
    </div>
  );
}
