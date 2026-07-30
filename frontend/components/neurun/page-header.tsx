import type { ReactNode } from "react";
import Link from "next/link";
import { ChevronRight } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";

export interface Crumb {
  label: string;
  href?: string;
}

export function PageHeader({
  title,
  description,
  crumbs,
  actions,
  meta,
  className,
}: {
  title: ReactNode;
  description?: ReactNode;
  crumbs?: Crumb[];
  actions?: ReactNode;
  /** Mono metadata line beneath the title. */
  meta?: ReactNode;
  className?: string;
}) {
  return (
    <header className={cn("border-b border-line px-6 py-4", className)}>
      {crumbs && crumbs.length > 0 ? (
        <nav aria-label="Breadcrumb" className="mb-1.5">
          <ol className="flex flex-wrap items-center gap-1 font-mono text-micro text-fg-muted">
            {crumbs.map((crumb, index) => (
              <li key={`${crumb.label}-${index}`} className="flex items-center gap-1">
                {index > 0 ? (
                  <span aria-hidden className="text-fg-faint">
                    /
                  </span>
                ) : null}
                {crumb.href ? (
                  <Link href={crumb.href} className="rounded-xs hover:text-fg hover:underline">
                    {crumb.label}
                  </Link>
                ) : (
                  <span className="text-fg-secondary">{crumb.label}</span>
                )}
              </li>
            ))}
          </ol>
        </nav>
      ) : null}

      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0">
          <h1 className="text-2xl">{title}</h1>
          {description ? (
            <p className="nr-measure mt-1 text-sm text-fg-secondary">{description}</p>
          ) : null}
          {meta ? (
            <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1">{meta}</div>
          ) : null}
        </div>
        {actions ? <div className="flex shrink-0 items-center gap-2">{actions}</div> : null}
      </div>
    </header>
  );
}

/** Previous/next controls for an opaque-cursor list. */
export function CursorControls({
  pageIndex,
  canGoBack,
  nextCursor,
  onBack,
  onNext,
  isFetching,
}: {
  pageIndex: number;
  canGoBack: boolean;
  /** Empty string means there is no next page. */
  nextCursor: string | undefined;
  onBack: () => void;
  onNext: () => void;
  isFetching?: boolean;
}) {
  const hasNext = Boolean(nextCursor);

  return (
    <div className="flex items-center gap-2">
      <span className="font-mono text-micro text-fg-muted">page {pageIndex + 1}</span>
      <Button variant="ghost" size="sm" onClick={onBack} disabled={!canGoBack || isFetching}>
        Previous
      </Button>
      <Button variant="ghost" size="sm" onClick={onNext} disabled={!hasNext || isFetching}>
        Next
        <ChevronRight aria-hidden className="size-3" strokeWidth={1.5} />
      </Button>
    </div>
  );
}
