import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

/** Mono infrastructure label preceded by a 20px hairline. */
export function Eyebrow({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <p className={cn("nr-label flex items-center gap-2.5", className)}>
      <span aria-hidden className="h-px w-5 bg-fg-faint" />
      {children}
    </p>
  );
}

/** A marketing band: hairline top, 1360px column, 112px vertical rhythm. */
export function Section({
  id,
  children,
  className,
  innerClassName,
  bare = false,
}: {
  id?: string;
  children: ReactNode;
  className?: string;
  innerClassName?: string;
  /** Skip the top hairline — for the hero and any band that follows a full rule. */
  bare?: boolean;
}) {
  return (
    <section id={id} className={cn(!bare && "border-t border-line", className)}>
      <div
        className={cn(
          "mx-auto w-full max-w-(--nr-container-max) px-6 py-20 sm:py-28",
          innerClassName,
        )}
      >
        {children}
      </div>
    </section>
  );
}

export function SectionHeading({
  eyebrow,
  title,
  lead,
  className,
  actions,
}: {
  eyebrow: string;
  title: ReactNode;
  lead?: ReactNode;
  className?: string;
  actions?: ReactNode;
}) {
  return (
    <div className={cn("flex flex-wrap items-end gap-8", className)}>
      <div className="flex max-w-[820px] min-w-0 flex-col gap-4">
        <Eyebrow>{eyebrow}</Eyebrow>
        <h2 className="text-[clamp(30px,3.4vw,52px)] leading-[1.02] tracking-display">{title}</h2>
        {lead ? (
          <p className="max-w-[640px] text-lg leading-[1.55] text-fg-secondary">{lead}</p>
        ) : null}
      </div>
      {actions ? <div className="ml-auto shrink-0">{actions}</div> : null}
    </div>
  );
}

/** `**bold**` is the only markup the plan feature strings need. */
export function Emphasised({ text }: { text: string }) {
  return (
    <>
      {text.split(/(\*\*[^*]+\*\*)/).map((part, i) =>
        part.startsWith("**") && part.endsWith("**") ? (
          <strong key={i} className="font-medium text-fg">
            {part.slice(2, -2)}
          </strong>
        ) : (
          part
        ),
      )}
    </>
  );
}
