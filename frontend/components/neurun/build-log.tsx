"use client";

import { useEffect, useRef } from "react";

import { cn } from "@/lib/utils";

/**
 * What a toolchain printed.
 *
 * The palette is the system's, which is monochrome: emphasis separates the
 * stages a build ran from the noise between them, and the accent is reserved
 * for the lines that say it went wrong — the ones somebody opened the log for.
 */
function toneOf(line: string): string {
  if (line.startsWith("$ ")) return "text-fg";
  if (/^\s*(error(\[|:|\s)|thread .+ panicked|fatal|E:\s|npm ERR!)/i.test(line)) {
    return "text-destructive";
  }
  if (/^\s*warning/i.test(line)) return "text-fg-secondary";
  if (/^\s*(compiling|downloaded|finished|installing|collecting|fetching|added)/i.test(line)) {
    return "text-fg-secondary";
  }
  return "text-fg-muted";
}

export function BuildLog({ logs, following }: { logs: string; following?: boolean }) {
  const end = useRef<HTMLDivElement>(null);

  // A running build writes to the bottom, which is where the reader wants to be.
  useEffect(() => {
    if (following) end.current?.scrollIntoView({ block: "nearest" });
  }, [logs, following]);

  if (!logs) {
    return <p className="text-fg-muted">No output yet.</p>;
  }
  return (
    <div className="max-h-[60vh] overflow-auto font-mono text-micro leading-relaxed">
      {logs.split("\n").map((line, index) => (
        <div
          key={index}
          className={cn("whitespace-pre-wrap", toneOf(line))}
        >
          {line || "\u00a0"}
        </div>
      ))}
      <div ref={end} />
    </div>
  );
}
