import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

/** Docs body: 16px / 1.55, capped at 74ch. Never a full 1360px column. */
export function P({ children }: { children: ReactNode }) {
  return <p className="text-base leading-[1.55] text-fg-secondary">{children}</p>;
}

export function Lead({ children }: { children: ReactNode }) {
  return <p className="nr-measure text-lg leading-[1.55] text-fg-secondary">{children}</p>;
}

/** Section heading. The id is the TOC anchor and the scroll-spy target. */
export function H2({ id, children }: { id: string; children: ReactNode }) {
  return (
    <h2 id={id} className="scroll-mt-22 pt-2 text-2xl tracking-title">
      {children}
    </h2>
  );
}

export function C({ children }: { children: ReactNode }) {
  return <code className="font-mono text-[0.9em] text-fg">{children}</code>;
}

export function Snippet({
  filename,
  children,
  language,
}: {
  filename?: string;
  children: string;
  language?: string;
}) {
  return (
    <div className="overflow-hidden rounded-lg border border-line bg-surface-panel">
      {filename ? (
        <div className="flex h-8.5 items-center gap-2 border-b border-line px-3">
          <span className="nr-label">{filename}</span>
          {language ? (
            <span className="ml-auto font-mono text-micro text-fg-faint">{language}</span>
          ) : null}
        </div>
      ) : null}
      <pre className="overflow-x-auto p-3.5 font-mono text-meta leading-[1.6] text-(--nr-code-string)">
        <code>{children}</code>
      </pre>
    </div>
  );
}

/** Two request/response panels side by side. */
export function Split({ children }: { children: ReactNode }) {
  return <div className="grid gap-3 sm:grid-cols-2">{children}</div>;
}

/** A definition table: term on the left, prose on the right. */
export function Rows({
  items,
  termClassName,
}: {
  items: { term: ReactNode; body: ReactNode }[];
  termClassName?: string;
}) {
  return (
    <dl className="flex flex-col">
      {items.map((item, index) => (
        <div
          key={index}
          className="grid grid-cols-[132px_minmax(0,1fr)] items-baseline gap-4 border-t border-line py-2.5 last:border-b"
        >
          <dt className={cn("font-mono text-caption", termClassName)}>{item.term}</dt>
          <dd className="text-sm leading-[1.5] text-fg-secondary">{item.body}</dd>
        </div>
      ))}
    </dl>
  );
}
