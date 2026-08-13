"use client";

import { useState } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { ScreenShare } from "lucide-react";
import { toast } from "sonner";

import { DisplayStream } from "@/components/browser-sessions/display-stream";
import { ErrorPanel, InlineError } from "@/components/neurun/error-panel";
import { EmptyState } from "@/components/neurun/feedback";
import { KeyValue } from "@/components/neurun/key-value";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { StatusBadge } from "@/components/neurun/status-badge";
import { Timestamp } from "@/components/neurun/timestamp";
import { Button } from "@/components/ui/button";
import {
  useBrowserSessionQuery,
  useCloseBrowserSessionMutation,
} from "@/lib/api/queries";

export default function BrowserSessionPage() {
  const { sessionId } = useParams<{ sessionId: string }>();
  const query = useBrowserSessionQuery(sessionId);
  const close = useCloseBrowserSessionMutation();
  const router = useRouter();
  // The stream starts on request rather than on load: a framebuffer nobody is
  // watching is bandwidth spent on an open tab.
  const [watching, setWatching] = useState(false);

  if (query.isError) {
    return (
      <div className="p-6">
        <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
      </div>
    );
  }
  if (!query.data) return <p className="p-6 text-fg-muted">Loading…</p>;

  const session = query.data;
  return (
    <div>
      <PageHeader
        crumbs={[
          { label: "Browser sessions", href: "/browser-sessions" },
          { label: session.id },
        ]}
        title={session.id}
        actions={
          <Button
            size="sm"
            variant="ghost"
            disabled={close.isPending}
            onClick={() =>
              close.mutate(session.id, {
                onSuccess: () => {
                  toast.success("Session closed");
                  router.push("/browser-sessions");
                },
              })
            }
          >
            Close session
          </Button>
        }
      />
      <div className="mx-auto max-w-4xl space-y-4 p-6">
        <Panel label="Session">
          <KeyValue
            columns={2}
            rows={[
              { label: "Status", value: <StatusBadge status={session.status} /> },
              { label: "Browser", value: session.browser },
              {
                label: "App",
                value: (
                  <Link className="underline" href={`/apps/${session.app_id}`}>
                    {session.app_id}
                  </Link>
                ),
              },
              {
                label: "Profile",
                value: session.browser_profile_id ? (
                  <Link
                    className="underline"
                    href={`/browser-profiles/${session.browser_profile_id}`}
                  >
                    {session.browser_profile_id}
                  </Link>
                ) : (
                  "no profile"
                ),
              },
              { label: "Started", value: <Timestamp value={session.started_at} /> },
              { label: "Updated", value: <Timestamp value={session.updated_at} /> },
            ]}
          />
          {close.isError ? <InlineError className="mt-3" error={close.error} /> : null}
        </Panel>

        <Panel label="Display" flush>
          {watching ? (
            <div className="p-3">
              <DisplayStream sessionId={session.id} />
            </div>
          ) : (
            <EmptyState
              icon={ScreenShare}
              title="Display not connected"
              description="Watching streams a live view of a signed-in browser."
              action={
                <Button size="sm" onClick={() => setWatching(true)}>
                  <ScreenShare aria-hidden />
                  View display
                </Button>
              }
            />
          )}
        </Panel>
      </div>
    </div>
  );
}
