import {
  ArchiveX,
  Check,
  CircleHelp,
  CircleStop,
  RotateCcw,
  Slash,
  TimerOff,
  Unplug,
  X,
  ZapOff,
  type LucideIcon,
} from "lucide-react";

import { cn } from "@/lib/utils";
import { describeStatus, type StatusTreatment } from "@/lib/view/status";

const ICONS: Record<string, LucideIcon> = {
  check: Check,
  "rotate-ccw": RotateCcw,
  "timer-off": TimerOff,
  unplug: Unplug,
  slash: Slash,
  x: X,
  "archive-x": ArchiveX,
  "zap-off": ZapOff,
  "circle-help": CircleHelp,
  "circle-stop": CircleStop,
};

/**
 * The design system's status legend, expressed in classes. Every state is
 * distinguishable with colour removed: border style, fill, pattern and glyph
 * do the work.
 */
const TREATMENTS: Record<StatusTreatment, string> = {
  solid: "border-solid border-line-inverse text-fg",
  pulse: "border-solid border-line-default text-fg",
  dashed: "border-dashed border-line-default text-fg-secondary",
  hatch: "border-solid border-line-default text-fg nr-hatch",
  rejected: "border-solid border-line-strong text-fg",
  inverted: "border-solid border-transparent bg-surface-inverse text-fg-inverse",
  strike: "border-solid border-line text-fg-muted line-through",
  neutral: "border-solid border-line bg-surface-inset text-fg-muted",
};

export interface StatusBadgeProps {
  /** The raw API value. Rendered verbatim, whether or not it is recognised. */
  status: string | undefined | null;
  className?: string;
  /** Hide the glyph when the surrounding row already carries one. */
  hideIcon?: boolean;
}

/**
 * Renders any status the server reports.
 *
 * An unrecognised value is not an error: it renders neutral, carrying its raw
 * text and an explicit "unrecognised" note for assistive technology. It is
 * never silently mapped onto success, and it never crashes the route.
 */
export function StatusBadge({ status, className, hideIcon = false }: StatusBadgeProps) {
  const descriptor = describeStatus(status);
  const Icon = descriptor.icon ? ICONS[descriptor.icon] : null;
  const showPulse = descriptor.treatment === "pulse";

  return (
    <span
      className={cn(
        "inline-flex w-fit shrink-0 items-center gap-1.5 rounded-xs border px-1.5 py-0.5",
        "font-mono text-micro leading-none whitespace-nowrap",
        TREATMENTS[descriptor.treatment],
        className,
      )}
      data-status={descriptor.value}
      data-treatment={descriptor.treatment}
      data-known={descriptor.known}
    >
      {showPulse ? (
        <span
          aria-hidden
          className="size-1.5 shrink-0 rounded-full bg-current motion-safe:animate-pulse-node"
        />
      ) : null}
      {!hideIcon && Icon ? <Icon aria-hidden className="size-3 shrink-0" strokeWidth={1.5} /> : null}
      <span>{descriptor.value}</span>
      {/* Status is conveyed as text, icon and treatment — never colour alone. */}
      <span className="sr-only">{` — ${descriptor.description}`}</span>
    </span>
  );
}
