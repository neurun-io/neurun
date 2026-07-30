import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import { NO_VALUE } from "@/lib/view/units";

export interface KeyValueRow {
  label: string;
  value: ReactNode;
  /** Rendered beneath the value as a quiet gloss. */
  hint?: ReactNode;
}

/**
 * A metadata list: mono uppercase label, value on the right.
 *
 * Built on `<dl>` so the label/value relationship survives for assistive
 * technology, and a missing value reads as `—` rather than being dropped or
 * shown as zero — an absent metric and a measured zero are different facts.
 */
export function KeyValue({
  rows,
  className,
  columns = 1,
}: {
  rows: KeyValueRow[];
  className?: string;
  columns?: 1 | 2;
}) {
  return (
    <dl
      className={cn(
        "grid gap-x-6",
        columns === 2 ? "sm:grid-cols-2" : "grid-cols-1",
        className,
      )}
    >
      {rows.map((row) => (
        <div
          key={row.label}
          className="flex min-w-0 items-baseline justify-between gap-4 border-b border-line py-1.5 last:border-b-0"
        >
          <dt className="nr-label shrink-0 pt-0.5">{row.label}</dt>
          <dd className="min-w-0 text-right font-mono text-caption text-fg-secondary">
            {row.value ?? <span className="text-fg-muted">{NO_VALUE}</span>}
            {row.hint ? <div className="mt-0.5 text-micro text-fg-muted">{row.hint}</div> : null}
          </dd>
        </div>
      ))}
    </dl>
  );
}
