"use client";

import { useEffect, useState } from "react";
import Link from "next/link";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { CursorControls, PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { StatusBadge } from "@/components/neurun/status-badge";
import { Timestamp } from "@/components/neurun/timestamp";
import { Digest } from "@/components/neurun/copy-id";
import { EmptyState } from "@/components/neurun/feedback";
import { ErrorPanel } from "@/components/neurun/error-panel";
import { useJobsQuery } from "@/lib/api/queries";
import { useCursorPages } from "@/lib/view/use-cursor-pages";
import type { JobState } from "@/lib/api/types";

/**
 * Server-side job filters. Only these four exist in the current contract —
 * `tag`, mode, failure, function, agent and created-before are NOT sent,
 * because the server rejects `tag` outright rather than ignoring it, and
 * inventing client-side equivalents over a paginated list would produce filters
 * that silently lie about completeness.
 */
const FILTERABLE_STATES: JobState[] = [
  "accepted",
  "queued",
  "leased",
  "running",
  "retry_wait",
  "succeeded",
  "rejected",
  "failed",
  "canceled",
  "dead_lettered",
];

const PAGE_SIZE = 50;

export default function JobsPage() {
  const [selectedStates, setSelectedStates] = useState<JobState[]>([]);
  const pages = useCursorPages();

  // A filter change invalidates the cursor trail: cursors are opaque and
  // scoped to the query that produced them.
  useEffect(() => {
    pages.reset();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedStates.join(",")]);

  const query = useJobsQuery({
    status: selectedStates.length > 0 ? selectedStates : undefined,
    limit: PAGE_SIZE,
    cursor: pages.cursor,
  });

  const jobs = query.data?.jobs ?? [];

  return (
    <div className="flex min-h-full flex-col">
      <PageHeader
        title="Jobs"
        description="Asynchronous work accepted as immutable, digest-pinned jobs."
        actions={
          <CursorControls
            pageIndex={pages.pageIndex}
            canGoBack={pages.canGoBack}
            nextCursor={query.data?.next_cursor}
            onBack={pages.back}
            onNext={() => pages.next(query.data?.next_cursor ?? "")}
            isFetching={query.isFetching}
          />
        }
      />

      <div className="px-6 py-4">
        <StatusFilter selected={selectedStates} onChange={setSelectedStates} />

        {query.isError ? (
          <ErrorPanel error={query.error} onRetry={() => query.refetch()} className="mt-4" />
        ) : (
          <Panel
            label="Jobs"
            className="mt-4"
            flush
            footer={
              <span>
                {query.isFetching ? "refreshing · " : ""}
                {jobs.length} {jobs.length === 1 ? "job" : "jobs"} on this page
                {query.data?.next_cursor ? " · more available" : ""}
              </span>
            }
          >
            {query.isPending ? (
              <TableSkeleton />
            ) : jobs.length === 0 ? (
              <EmptyState
                title="No jobs match this filter"
                description="Clear the state filter or widen the time window."
                action={
                  selectedStates.length > 0 ? (
                    <Button variant="secondary" size="sm" onClick={() => setSelectedStates([])}>
                      Clear filter
                    </Button>
                  ) : null
                }
              />
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead scope="col">State</TableHead>
                    <TableHead scope="col">Job</TableHead>
                    <TableHead scope="col">Function</TableHead>
                    <TableHead scope="col" className="text-right">
                      Attempts
                    </TableHead>
                    <TableHead scope="col">Created</TableHead>
                    <TableHead scope="col">Updated</TableHead>
                    <TableHead scope="col">Next attempt</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {jobs.map((job) => (
                    <TableRow key={job.id} className="h-(--nr-density-row)">
                      <TableCell>
                        <StatusBadge status={job.state} />
                      </TableCell>
                      <TableCell>
                        <Link
                          href={`/jobs/${job.id}`}
                          className="rounded-xs font-mono text-caption text-fg hover:underline"
                        >
                          {job.id}
                        </Link>
                      </TableCell>
                      <TableCell>
                        <div className="flex min-w-0 flex-col">
                          <span className="font-mono text-caption text-fg-secondary">
                            {job.request.function.name}
                            <span className="text-fg-muted">@{job.request.function.version}</span>
                          </span>
                          <Digest value={job.request.function.digest} />
                        </div>
                      </TableCell>
                      <TableCell className="text-right font-mono text-caption tabular-nums text-fg-secondary">
                        {job.attempt_count}/{job.max_attempts}
                      </TableCell>
                      <TableCell>
                        <Timestamp value={job.created_at} className="text-fg-muted" />
                      </TableCell>
                      <TableCell>
                        <Timestamp value={job.updated_at} className="text-fg-muted" />
                      </TableCell>
                      <TableCell>
                        <Timestamp value={job.next_attempt_at} className="text-fg-muted" />
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </Panel>
        )}
      </div>
    </div>
  );
}

function StatusFilter({
  selected,
  onChange,
}: {
  selected: JobState[];
  onChange: (next: JobState[]) => void;
}) {
  return (
    <fieldset className="flex flex-wrap items-center gap-x-4 gap-y-2">
      <legend className="nr-label mb-1.5">Filter by state</legend>
      {FILTERABLE_STATES.map((state) => {
        const id = `state-${state}`;
        const checked = selected.includes(state);
        return (
          <div key={state} className="flex items-center gap-1.5">
            <Checkbox
              id={id}
              checked={checked}
              onCheckedChange={(next) => {
                onChange(
                  next === true ? [...selected, state] : selected.filter((item) => item !== state),
                );
              }}
            />
            <Label htmlFor={id} className="font-mono text-micro font-normal text-fg-secondary">
              {state}
            </Label>
          </div>
        );
      })}
      {selected.length > 0 ? (
        <Button variant="ghost" size="xs" onClick={() => onChange([])}>
          Clear
        </Button>
      ) : null}
    </fieldset>
  );
}

function TableSkeleton() {
  return (
    <div className="space-y-px p-3" aria-hidden>
      {Array.from({ length: 8 }).map((_, index) => (
        <Skeleton key={index} className="h-(--nr-density-row) w-full" />
      ))}
    </div>
  );
}
