"use client";

import Link from "next/link";

import { ErrorPanel } from "@/components/neurun/error-panel";
import { EmptyState } from "@/components/neurun/feedback";
import { PageHeader } from "@/components/neurun/page-header";
import { StatusBadge } from "@/components/neurun/status-badge";
import { Timestamp } from "@/components/neurun/timestamp";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useBrowserSessionsQuery } from "@/lib/api/queries";

export default function BrowserSessionsPage() {
  const query = useBrowserSessionsQuery();

  return (
    <div>
      <PageHeader
        title="Browser sessions"
        description="Browsers open right now, and what each one is running as."
      />
      <div className="space-y-4 p-6">
        {query.isError ? (
          <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
        ) : query.isPending ? (
          <p className="text-fg-muted">Loading…</p>
        ) : query.data.browser_sessions.length === 0 ? (
          <EmptyState
            title="No live sessions"
            description="A session appears while a handler has a browser open, and leaves when it closes."
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Status</TableHead>
                <TableHead>Session</TableHead>
                <TableHead>App</TableHead>
                <TableHead>Profile</TableHead>
                <TableHead>Browser</TableHead>
                <TableHead>Started</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {query.data.browser_sessions.map((session) => (
                <TableRow key={session.id}>
                  <TableCell>
                    <StatusBadge status={session.status} />
                  </TableCell>
                  <TableCell>
                    <Link
                      className="font-mono underline"
                      href={`/browser-sessions/${session.id}`}
                    >
                      {session.id}
                    </Link>
                  </TableCell>
                  <TableCell>
                    <Link className="font-mono underline" href={`/apps/${session.app_id}`}>
                      {session.app_id}
                    </Link>
                  </TableCell>
                  <TableCell>
                    {session.browser_profile_id ? (
                      <Link
                        className="font-mono underline"
                        href={`/browser-profiles/${session.browser_profile_id}`}
                      >
                        {session.browser_profile_id}
                      </Link>
                    ) : (
                      <span className="font-mono text-micro text-fg-muted">no profile</span>
                    )}
                  </TableCell>
                  <TableCell className="font-mono">
                    {session.browser}
                    {session.has_display ? null : (
                      <Badge className="ml-2" variant="outline">
                        headless
                      </Badge>
                    )}
                  </TableCell>
                  <TableCell>
                    <Timestamp value={session.started_at} />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </div>
    </div>
  );
}
