"use client";

import type { ReactNode } from "react";
import { Loader2 } from "lucide-react";

import { TopNav } from "./top-nav";
import { SideNav } from "./side-nav";
import { Banner } from "@/components/neurun/feedback";
import { ConnectionScreen } from "@/components/connection/connection-screen";
import { useConnection } from "@/lib/connection/store";
import { useCapability } from "@/lib/connection/capability";

/**
 * The dashboard shell, and the gate in front of it.
 *
 * Gating happens here rather than through a redirect so that a deep link —
 * `/jobs/job_01HXQ…` pasted into a chat — survives connecting. The operator
 * lands on the connection screen and, once the key is verified, is already
 * looking at the job they were sent.
 */
export function DashboardShell({ children }: { children: ReactNode }) {
  const { connection, hydrated } = useConnection();

  if (!hydrated) {
    return (
      <div className="flex min-h-dvh items-center justify-center" role="status">
        <Loader2 aria-hidden className="size-4 animate-spin text-fg-muted" strokeWidth={1.5} />
        <span className="sr-only">Restoring session</span>
      </div>
    );
  }

  if (!connection) return <ConnectionScreen />;

  return (
    <div className="flex min-h-dvh flex-col">
      <TopNav />
      <DurabilityBanner />
      <div className="flex min-h-0 flex-1">
        <SideNav className="hidden md:block" />
        <main id="main" className="min-w-0 flex-1 overflow-x-hidden">
          {children}
        </main>
      </div>
    </div>
  );
}

/**
 * The process-local warning.
 *
 * Shown for the whole connection once the server has reported `process_local`
 * on an accepted job, because the Job schema does not repeat durability on list
 * or detail responses — there is no per-job guarantee to render, only a
 * property of the backend this dashboard is talking to.
 */
function DurabilityBanner() {
  const { isProcessLocal, asyncAvailability } = useCapability();

  if (asyncAvailability === "unavailable") {
    return (
      <Banner>
        Asynchronous jobs are disabled on this control plane — no durable backend is configured.
        Synchronous invocation still works.
      </Banner>
    );
  }

  if (!isProcessLocal) return null;

  return (
    <Banner>
      <strong className="font-medium text-fg">process_local</strong> — async work runs in the server
      process. Queued jobs disappear on restart. This is a development mode, not a durable backend.
    </Banner>
  );
}
