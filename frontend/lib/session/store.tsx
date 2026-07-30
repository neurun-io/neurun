"use client";

/**
 * Operator session state.
 *
 * There is no credential in this module, and deliberately no storage of any
 * kind. Authentication lives in an `HttpOnly` cookie the browser attaches and
 * script cannot read, so the client's only job is to ask the server who it is
 * and react when the answer changes.
 *
 * That is the whole reason the previous API-key screen is gone: a key held in
 * JavaScript is readable by any script on the page and survives in memory for
 * the life of the tab. This design removes the client's ability to leak it.
 */
import { createContext, useCallback, useContext, useMemo, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import * as api from "@/lib/api/endpoints";
import { NeurunApiError } from "@/lib/api/errors";
import type { Operator } from "@/lib/api/types";

/** Why the dashboard is not showing evidence. */
export type SessionStatus =
  | "loading"
  | "authenticated"
  | "anonymous"
  /** The server has no operator accounts configured. */
  | "unavailable";

export const SESSION_QUERY_KEY = ["neurun", "session"] as const;

interface SessionContextValue {
  operator: Operator | null;
  status: SessionStatus;
  /** Set when the session probe itself failed for an unexpected reason. */
  error: unknown;
  login: (username: string, password: string) => Promise<Operator>;
  logout: () => Promise<void>;
  isLoggingIn: boolean;
  loginError: unknown;
}

const SessionContext = createContext<SessionContextValue | null>(null);

type Probe =
  | { kind: "operator"; operator: Operator }
  | { kind: "anonymous" }
  | { kind: "unavailable" };

/**
 * A 401 is the expected answer for "not signed in", so it resolves rather than
 * rejects — an unauthenticated visitor is not an error condition.
 */
async function probeSession(signal?: AbortSignal): Promise<Probe> {
  try {
    return { kind: "operator", operator: await api.getOperatorSession(signal) };
  } catch (error) {
    if (error instanceof NeurunApiError) {
      if (error.status === 401) return { kind: "anonymous" };
      if (error.status === 503) return { kind: "unavailable" };
    }
    throw error;
  }
}

export function SessionProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();

  const probe = useQuery({
    queryKey: SESSION_QUERY_KEY,
    queryFn: ({ signal }) => probeSession(signal),
    // Sessions expire on an absolute deadline, so re-check periodically and
    // whenever the operator returns to the tab.
    staleTime: 60_000,
    refetchInterval: 5 * 60_000,
    refetchOnWindowFocus: true,
    retry: false,
  });

  const loginMutation = useMutation({
    mutationFn: ({ username, password }: { username: string; password: string }) =>
      api.operatorLogin(username, password),
    onSuccess: (operator) => {
      queryClient.setQueryData(SESSION_QUERY_KEY, { kind: "operator", operator } satisfies Probe);
    },
  });

  const login = useCallback(
    async (username: string, password: string) => {
      return loginMutation.mutateAsync({ username, password });
    },
    [loginMutation],
  );

  const logout = useCallback(async () => {
    try {
      await api.operatorLogout();
    } finally {
      // Clear regardless of the response: a failed sign-out must still drop this
      // operator's cached evidence rather than leaving it on screen.
      queryClient.clear();
      queryClient.setQueryData(SESSION_QUERY_KEY, { kind: "anonymous" } satisfies Probe);
    }
  }, [queryClient]);

  const status: SessionStatus = useMemo(() => {
    if (probe.isPending) return "loading";
    switch (probe.data?.kind) {
      case "operator":
        return "authenticated";
      case "unavailable":
        return "unavailable";
      default:
        return "anonymous";
    }
  }, [probe.isPending, probe.data]);

  const value = useMemo<SessionContextValue>(
    () => ({
      operator: probe.data?.kind === "operator" ? probe.data.operator : null,
      status,
      error: probe.error,
      login,
      logout,
      isLoggingIn: loginMutation.isPending,
      loginError: loginMutation.error,
    }),
    [probe.data, probe.error, status, login, logout, loginMutation.isPending, loginMutation.error],
  );

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): SessionContextValue {
  const context = useContext(SessionContext);
  if (!context) {
    throw new Error("useSession must be used inside a SessionProvider.");
  }
  return context;
}

/**
 * The operator, asserted present. Use inside routes that already sit behind the
 * session gate.
 */
export function useRequiredOperator(): Operator {
  const { operator } = useSession();
  if (!operator) {
    throw new Error("This route requires a signed-in operator.");
  }
  return operator;
}

/**
 * Cache partition for the signed-in operator. Query keys carry this so one
 * operator's evidence can never be read from another's cached data — the
 * session is also cleared outright on sign-out, so this is defence in depth.
 */
export function sessionScope(operator: Operator | null): string {
  if (!operator) return "anonymous";
  return `${operator.project_id}#${operator.operator_id}`;
}

/** True when the operator's role grants the scope. */
export function hasScope(operator: Operator | null, scope: string): boolean {
  if (!operator) return false;
  return operator.scopes.some((granted) => granted === "*" || granted === scope);
}
