import type { ReactNode } from "react";
import { AlertTriangle, Info, Inbox, Map } from "lucide-react";
import { cn } from "@/lib/utils";
import { PageHeader } from "./page-header";

/* -------------------------------------------------------------------------- */
/* Callout                                                                     */
/* -------------------------------------------------------------------------- */

export type CalloutKind = "note" | "warning" | "roadmap";

const CALLOUT_STYLES: Record<CalloutKind, string> = {
  note: "border-line-default bg-surface-panel",
  // 45° hatch is the system's warning texture.
  warning: "border-line-strong bg-surface-panel",
  // Dashed for anything not yet shipped.
  roadmap: "border-dashed border-line-default bg-transparent",
};

const CALLOUT_ICONS: Record<CalloutKind, typeof Info> = {
  note: Info,
  warning: AlertTriangle,
  roadmap: Map,
};

export function Callout({
  kind = "note",
  title,
  children,
  className,
}: {
  kind?: CalloutKind;
  title?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  const Icon = CALLOUT_ICONS[kind];

  return (
    <div
      className={cn("relative overflow-hidden rounded-lg border p-3", CALLOUT_STYLES[kind], className)}
    >
      {kind === "warning" ? (
        <span aria-hidden className="nr-hatch absolute inset-y-0 left-0 w-1" />
      ) : null}
      <div className="flex gap-2.5">
        <Icon aria-hidden className="mt-0.5 size-4 shrink-0 text-fg-muted" strokeWidth={1.5} />
        <div className="min-w-0 flex-1">
          {title ? <p className="mb-1 text-sm font-medium text-fg">{title}</p> : null}
          <div className="text-sm text-fg-secondary [&_a]:underline [&_a]:underline-offset-3">
            {children}
          </div>
        </div>
      </div>
    </div>
  );
}


/* -------------------------------------------------------------------------- */
/* Empty state                                                                 */
/* -------------------------------------------------------------------------- */

export function EmptyState({
  title,
  description,
  action,
  icon: Icon = Inbox,
  className,
}: {
  title: string;
  description?: ReactNode;
  action?: ReactNode;
  icon?: typeof Inbox;
  className?: string;
}) {
  return (
    <div className={cn("flex flex-col items-center px-6 py-12 text-center", className)}>
      <Icon aria-hidden className="mb-3 size-6 text-fg-faint" strokeWidth={1.25} />
      <p className="text-sm font-medium text-fg">{title}</p>
      {description ? (
        <p className="mt-1 max-w-prose text-sm text-fg-muted">{description}</p>
      ) : null}
      {action ? <div className="mt-4">{action}</div> : null}
    </div>
  );
}

/* -------------------------------------------------------------------------- */
/* Unbuilt route                                                               */
/* -------------------------------------------------------------------------- */

/**
 * A route whose entity does not exist yet, laid out like every route whose
 * entity does. Nothing here fakes a record: the page is the empty state a real
 * one would show before its first row.
 */
export function UnbuiltRoute({
  title,
  summary,
  empty,
  children,
}: {
  title: string;
  summary: ReactNode;
  empty: { title: string; description: ReactNode };
  children?: ReactNode;
}) {
  return (
    <div>
      <PageHeader title={title} description={summary} />
      <div className="space-y-4 p-6">
        <EmptyState title={empty.title} description={empty.description} />
        {children}
      </div>
    </div>
  );
}
