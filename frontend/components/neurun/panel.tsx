import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

/**
 * The workhorse container: a hairline frame with a 34px mono header and an
 * optional mono footer. Reach for this before inventing a container.
 *
 * Depth comes from layered ink plus a hairline, never shadow, and nesting stops
 * at three levels.
 */
export interface PanelProps {
  /** Mono, uppercase, 11px at 0.16em — an infrastructure label, not a heading. */
  label?: ReactNode;
  /** Right-aligned controls in the header. */
  actions?: ReactNode;
  footer?: ReactNode;
  children: ReactNode;
  className?: string;
  bodyClassName?: string;
  /** Drop the body padding, e.g. when the body is a full-bleed table. */
  flush?: boolean;
  /** Element to render as. Use `section` when the panel is a landmark. */
  as?: "div" | "section" | "article";
}

export function Panel({
  label,
  actions,
  footer,
  children,
  className,
  bodyClassName,
  flush = false,
  as: Component = "section",
}: PanelProps) {
  return (
    <Component
      className={cn(
        "flex min-w-0 flex-col overflow-hidden rounded-lg border border-line bg-surface-panel",
        className,
      )}
    >
      {label || actions ? (
        <header className="flex h-8.5 shrink-0 items-center justify-between gap-3 border-b border-line px-3">
          {label ? <span className="nr-label truncate">{label}</span> : <span />}
          {actions ? <div className="flex shrink-0 items-center gap-1">{actions}</div> : null}
        </header>
      ) : null}

      <div className={cn(!flush && "p-3", "min-w-0 flex-1", bodyClassName)}>{children}</div>

      {footer ? (
        <footer className="flex min-h-8.5 shrink-0 items-center gap-3 border-t border-line px-3 py-1.5 font-mono text-micro text-fg-muted">
          {footer}
        </footer>
      ) : null}
    </Component>
  );
}

/** A metadata separator, matching the system's `·` convention. */
export function Dot() {
  return (
    <span aria-hidden className="text-fg-faint">
      ·
    </span>
  );
}
