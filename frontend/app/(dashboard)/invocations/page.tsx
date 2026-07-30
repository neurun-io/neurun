"use client";

import { useEffect, useState } from "react";
import Link from "next/link";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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
import { EmptyState } from "@/components/neurun/feedback";
import { ErrorPanel } from "@/components/neurun/error-panel";
import { useInvocationsQuery } from "@/lib/api/queries";
import { useCursorPages } from "@/lib/view/use-cursor-pages";
import { formatDurationMs } from "@/lib/view/units";
import type { InvocationStatus } from "@/lib/api/types";

const ANY = "__any__";
const PAGE_SIZE = 50;

const STATUSES: InvocationStatus[] = [
  "accepted",
  "running",
  "succeeded",
  "rejected",
  "failed",
  "timed_out",
  "canceled",
];

/**
 * Direct and job-owned invocations.
 *
 * The contract has no reverse job/attempt filter yet, so this list is filtered
 * by function, version and status only. Job-owned work is reached from its job.
 */
export default function InvocationsPage() {
  const [status, setStatus] = useState<string>(ANY);
  const [functionName, setFunctionName] = useState("");
  const pages = useCursorPages();

  useEffect(() => {
    pages.reset();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [status, functionName]);

  const query = useInvocationsQuery({
    status: status === ANY ? undefined : status,
    function: functionName.trim() || undefined,
    limit: PAGE_SIZE,
    cursor: pages.cursor,
  });

  const invocations = query.data?.invocations ?? [];

  return (
    <div className="flex min-h-full flex-col">
      <PageHeader
        title="Invocations"
        description="Every function execution visible to this project, direct or job-owned."
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

      <div className="space-y-4 px-6 py-4">
        <Panel label="Filters">
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <div className="space-y-1.5">
              <Label htmlFor="filter-function">Function</Label>
              <Input
                id="filter-function"
                value={functionName}
                onChange={(event) => setFunctionName(event.target.value)}
                placeholder="system.echo"
                className="font-mono text-caption"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="filter-status">Status</Label>
              <Select value={status} onValueChange={setStatus}>
                <SelectTrigger id="filter-status" className="w-full font-mono text-caption">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={ANY}>any</SelectItem>
                  {STATUSES.map((option) => (
                    <SelectItem key={option} value={option} className="font-mono text-caption">
                      {option}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
        </Panel>

        {query.isError ? (
          <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
        ) : (
          <Panel
            label="Invocations"
            flush
            footer={
              <span>
                {query.isFetching ? "refreshing · " : ""}
                {invocations.length} on this page
                {query.data?.next_cursor ? " · more available" : ""}
              </span>
            }
          >
            {query.isPending ? (
              <div className="space-y-px p-3" aria-hidden>
                {Array.from({ length: 8 }).map((_, index) => (
                  <Skeleton key={index} className="h-(--nr-density-row) w-full" />
                ))}
              </div>
            ) : invocations.length === 0 ? (
              <EmptyState
                title="No invocations match this filter"
                description="Clear the filters, or invoke a function from the catalog."
                action={
                  <Button variant="secondary" size="sm" asChild>
                    <Link href="/functions">Open the catalog</Link>
                  </Button>
                }
              />
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead scope="col">Status</TableHead>
                    <TableHead scope="col">Invocation</TableHead>
                    <TableHead scope="col">Function</TableHead>
                    <TableHead scope="col">Schema</TableHead>
                    <TableHead scope="col" className="text-right">
                      Duration
                    </TableHead>
                    <TableHead scope="col">Created</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {invocations.map((invocation) => (
                    <TableRow key={invocation.invocation_id} className="h-(--nr-density-row)">
                      <TableCell>
                        <StatusBadge status={invocation.status} />
                      </TableCell>
                      <TableCell>
                        <Link
                          href={`/invocations/${invocation.invocation_id}`}
                          className="rounded-xs font-mono text-caption text-fg hover:underline"
                        >
                          {invocation.invocation_id}
                        </Link>
                      </TableCell>
                      <TableCell className="font-mono text-caption text-fg-secondary">
                        {invocation.function.name}
                        <span className="text-fg-muted">@{invocation.function.version}</span>
                      </TableCell>
                      <TableCell>
                        <span
                          className={
                            invocation.output_schema_valid
                              ? "font-mono text-micro text-fg-muted"
                              : "font-mono text-micro text-fg"
                          }
                        >
                          {invocation.output_schema_valid ? "valid" : "rejected"}
                        </span>
                      </TableCell>
                      <TableCell className="text-right font-mono text-caption tabular-nums text-fg-secondary">
                        {formatDurationMs(invocation.usage.duration_ms)}
                      </TableCell>
                      <TableCell>
                        <Timestamp value={invocation.created_at} className="text-fg-muted" />
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
