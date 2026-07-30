"use client";

import { useState } from "react";
import { ChevronRight } from "lucide-react";
import { cn } from "@/lib/utils";
import { JsonView } from "./json-view";
import { StatusBadge } from "./status-badge";
import { Timestamp } from "./timestamp";
import { EmptyState } from "./feedback";
import type { JobEvent } from "@/lib/api/types";

/**
 * The append-only job event stream, in ascending sequence order.
 *
 * The order is the server's, preserved exactly. In particular `job.accepted`
 * and `job.queued` both appear even though a polled job snapshot will usually
 * show only `queued` — the accepted state is often never observable as a polled
 * state, and dropping the event would erase the moment the job was taken.
 *
 * Nothing here coalesces or filters events.
 */
export function EventTimeline({ events, className }: { events: JobEvent[]; className?: string }) {
  if (events.length === 0) {
    return (
      <EmptyState
        title="No events recorded yet"
        description="Events appear as the job is accepted, queued, leased and run."
      />
    );
  }

  return (
    <ol className={cn("relative", className)}>
      {events.map((event, index) => (
        <EventRow key={event.id} event={event} isLast={index === events.length - 1} />
      ))}
    </ol>
  );
}

function EventRow({ event, isLast }: { event: JobEvent; isLast: boolean }) {
  const [open, setOpen] = useState(false);
  const hasPayload = event.payload !== undefined && event.payload !== null;

  return (
    <li className="relative flex gap-3 pl-1">
      {/* Connector: a hairline through the whole run, stopping at the last node. */}
      <div className="relative flex w-3 shrink-0 justify-center" aria-hidden>
        <span
          className={cn("absolute top-0 w-px bg-line-default", isLast ? "h-3" : "h-full")}
        />
        <span className="relative z-10 mt-2.25 size-1.5 rounded-full bg-line-strong" />
      </div>

      <div className="min-w-0 flex-1 border-b border-line py-2 last:border-b-0">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span className="font-mono text-micro text-fg-faint tabular-nums">
            {String(event.sequence).padStart(3, "0")}
          </span>
          <span className="font-mono text-caption text-fg">{event.type}</span>
          <StatusBadge status={event.state} />
          <span className="ml-auto shrink-0">
            <Timestamp value={event.created_at} className="text-fg-muted" />
          </span>
        </div>

        {event.attempt_id ? (
          <p className="mt-0.5 font-mono text-micro text-fg-muted">attempt {event.attempt_id}</p>
        ) : null}

        {hasPayload ? (
          <div className="mt-1.5">
            <button
              type="button"
              onClick={() => setOpen((previous) => !previous)}
              aria-expanded={open}
              className="inline-flex items-center gap-1 rounded-xs font-mono text-micro text-fg-muted hover:text-fg"
            >
              <ChevronRight
                aria-hidden
                className={cn("size-3 transition-transform duration-120", open && "rotate-90")}
                strokeWidth={1.5}
              />
              payload
            </button>
            {open ? (
              <JsonView value={event.payload} className="mt-1.5" downloadName={`${event.id}.json`} />
            ) : null}
          </div>
        ) : null}
      </div>
    </li>
  );
}
