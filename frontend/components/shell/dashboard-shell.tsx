"use client";

import type { ReactNode } from "react";
import { Loader2 } from "lucide-react";

import { TopNav } from "./top-nav";
import { SideNav } from "./side-nav";
import { ErrorPanel } from "@/components/neurun/error-panel";
import { LoginScreen } from "@/components/auth/login-screen";
import { OrganizationSetup } from "@/components/auth/organization-setup";
import { useSession } from "@/lib/session/store";

/**
 * The dashboard shell, and the sign-in gate in front of it.
 *
 * Gating happens here rather than through a redirect so that a deep link —
 * `/executions/exe_01HXQ…` pasted into a chat — survives signing in. The user
 * lands on the login screen and, once signed in, is already looking at the
 * execution they were sent.
 */
export function DashboardShell({ children }: { children: ReactNode }) {
  const { isLoading, error, session } = useSession();

  if (isLoading) {
    return (
      <div className="flex min-h-dvh items-center justify-center" role="status">
        <Loader2 aria-hidden className="size-4 animate-spin text-fg-muted" strokeWidth={1.5} />
        <span className="sr-only">Checking your session</span>
      </div>
    );
  }

  // The session probe itself failed — usually the control plane being
  // unreachable. Say so rather than showing a login form that cannot work.
  if (!session && error) {
    return (
      <main id="main" className="mx-auto w-full max-w-2xl px-6 py-10">
        <ErrorPanel error={error} title="Could not reach the control plane" />
      </main>
    );
  }

  if (!session) return <LoginScreen />;

  // Signed in, but belonging nowhere. Nothing below an organization exists yet,
  // so this is the whole surface until there is one.
  if (!session.organization_id) return <OrganizationSetup />;

  return (
    // The shell owns the viewport and the main pane scrolls inside it, so the
    // nav stays put instead of scrolling away with the page.
    <div className="flex h-dvh flex-col overflow-hidden">
      <TopNav />
      <div className="flex min-h-0 flex-1">
        <SideNav className="hidden md:block" />
        <main id="main" className="min-w-0 flex-1 overflow-y-auto overflow-x-hidden">
          {children}
        </main>
      </div>
    </div>
  );
}

