"use client";

import type { SVGProps } from "react";
import { Compass } from "lucide-react";

import type { BrowserKind } from "@/lib/api/resource-types";
import { cn } from "@/lib/utils";

/**
 * Chrome's mark, as a single monochrome path from Simple Icons (CC0). It is
 * filled rather than stroked, so it takes the current colour and sits in the
 * strictly monochrome palette beside the Lucide set, which ships no brand icons.
 *
 * Safari has no such path here, so it borrows Lucide's compass — the same shape
 * the mark is built on, and near enough at 14px.
 */
const CHROME =
  "M12 0C8.21 0 4.831 1.757 2.632 4.501l3.953 6.848A5.454 5.454 0 0 1 12 6.545h10.691A12 12 0 0 0 12 0zM1.931 5.47A11.943 11.943 0 0 0 0 12c0 6.012 4.42 10.991 10.189 11.864l3.953-6.847a5.45 5.45 0 0 1-6.865-2.29zm13.342 2.166a5.446 5.446 0 0 1 1.45 7.09l.002.001h-.002l-5.344 9.257c.206.01.413.016.621.016 6.627 0 12-5.373 12-12 0-1.54-.29-3.011-.818-4.364zM12 16.364a4.364 4.364 0 1 1 0-8.728 4.364 4.364 0 0 1 0 8.728Z";

export function BrowserIcon({
  browser,
  className,
  ...props
}: { browser: BrowserKind } & SVGProps<SVGSVGElement>) {
  if (browser === "safari") {
    return <Compass aria-hidden className={cn("size-3.5 shrink-0", className)} />;
  }
  return (
    <svg
      aria-hidden
      viewBox="0 0 24 24"
      fill="currentColor"
      className={cn("size-3.5 shrink-0", className)}
      {...props}
    >
      <path d={CHROME} />
    </svg>
  );
}
