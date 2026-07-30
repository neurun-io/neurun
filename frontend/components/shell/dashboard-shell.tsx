"use client";

import type { ReactNode } from "react";
import { Loader2 } from "lucide-react";

import { TopNav } from "./top-nav";
import { SideNav } from "./side-nav";
import { Banner } from "@/components/neurun/feedback";
import { ErrorPanel } from "@/components/neurun/error-panel";
import { LoginScreen } from "@/components/auth/login-screen";
import { useSession } from "@/lib/session/store";
import { useCapability } from "@/lib/session/capability";

/**
 * The dashboard shell, and the sign-in gate in front of it.
 *
 * Gating happens here rather than through a redirect so that a deep link —
 * `/jobs/job_01HXQ…` pasted into a chat — survives signing in. The operator
 * lands on the login screen and, once authenticated, is already looking at the
 * job they were sent.
 */
export function DashboardShell({ children }: { children: ReactNode }) {
  const { status, error } = useSession();

  if (status === "loading") {
    return (
      <div className="flex min-h-dvh items-center justify-center" role="status">
        <Loader2 aria-hidden className="size-4 animate-spin text-fg-muted" strokeWidth={1.5} />
        <span className="sr-only">Checking your session</span>
      </div>
    );
  }

  // The session probe itself failed — usually the control plane being
  // unreachable. Say so rather than showing a login form that cannot work.
  if (status === "anonymous" && error) {
    return (
      <main id="main" className="mx-auto w-full max-w-2xl px-6 py-10">
        <ErrorPanel error={error} title="Could not reach the control plane" />
      </main>
    );
  }

  if (status !== "authenticated") return <LoginScreen />;

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
 * Shown for the whole session once the server has reported `process_local` on an
 * accepted job, because the Job schema does not repeat durability on list or
 * detail responses — there is no per-job guarantee to render, only a property of
 * the backend this dashboard is talking to.
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
