import type { ReactNode } from "react";

/**
 * One fact about a profile, prefixed by its icon. The icon is sized and muted
 * here rather than at each call site, so a row of them reads as one line.
 */
export function Fact({ icon, children }: { icon: ReactNode; children: ReactNode }) {
  return (
    <span className="flex items-center gap-1">
      <span aria-hidden className="[&>svg]:size-3.5 [&>svg]:shrink-0 [&>svg]:text-fg-faint">
        {icon}
      </span>
      {children}
    </span>
  );
}
