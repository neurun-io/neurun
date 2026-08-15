"use client";

import { useEffect, useRef } from "react";

/** What a toolchain printed, as it printed it. */
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
    <div className="max-h-[60vh] overflow-auto">
      <pre className="font-mono text-micro leading-relaxed whitespace-pre-wrap text-fg-secondary">
        {logs}
      </pre>
      <div ref={end} />
    </div>
  );
}
