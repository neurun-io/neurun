import { cn } from "@/lib/utils";
import { describeStatus } from "@/lib/view/status";
import { isTerminalJobState } from "@/lib/api/types";

/**
 * The job lifecycle, drawn as a pipeline.
 *
 * The stages are the states a job passes *through*; whichever terminal state it
 * actually reached is shown as the final stage rather than assuming success.
 * An unrecognised state does not break the flow — it is appended as its own
 * terminal stage carrying its raw value.
 */
const PIPELINE = ["accepted", "queued", "leased", "running"] as const;

export function StateFlow({ state, className }: { state: string; className?: string }) {
  const descriptor = describeStatus(state);
  const terminal = isTerminalJobState(state) || !descriptor.known;
  const currentIndex = PIPELINE.indexOf(state as (typeof PIPELINE)[number]);

  const stages: { value: string; status: "done" | "current" | "pending" }[] = PIPELINE.map(
    (stage, index) => {
      if (terminal) return { value: stage, status: "done" as const };
      if (currentIndex === -1) return { value: stage, status: "pending" as const };
      if (index < currentIndex) return { value: stage, status: "done" as const };
      if (index === currentIndex) return { value: stage, status: "current" as const };
      return { value: stage, status: "pending" as const };
    },
  );

  if (terminal) {
    stages.push({ value: state, status: "current" });
  }

  return (
    <ol
      className={cn("flex flex-wrap items-center gap-1", className)}
      aria-label={`Job lifecycle — currently ${descriptor.value}`}
    >
      {stages.map((stage, index) => (
        <li key={`${stage.value}-${index}`} className="flex items-center gap-1">
          {index > 0 ? (
            <span aria-hidden className="text-fg-faint">
              →
            </span>
          ) : null}
          <span
            aria-current={stage.status === "current" ? "step" : undefined}
            className={cn(
              "rounded-xs border px-1.5 py-0.5 font-mono text-micro whitespace-nowrap",
              stage.status === "current" && "border-line-inverse text-fg",
              stage.status === "done" && "border-line text-fg-muted",
              stage.status === "pending" && "border-dashed border-line text-fg-faint",
            )}
          >
            {stage.value}
          </span>
        </li>
      ))}
    </ol>
  );
}
