"use client";

import { createContext, useCallback, useContext, useMemo, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import * as api from "@/lib/api/endpoints";
import { NeurunApiError } from "@/lib/api/errors";
import type { Session } from "@/lib/api/types";

export const SESSION_QUERY_KEY = ["neurun", "session"] as const;

interface SessionContextValue {
  session: Session | null;
  /** The first request has not answered yet, so signed-in is not yet known. */
  isLoading: boolean;
  /** Set when the session request itself failed for an unexpected reason. */
  error: unknown;
  login: (username: string, password: string) => Promise<Session>;
  register: (request: api.RegisterRequest) => Promise<Session | null>;
  logout: () => Promise<void>;
  isLoggingIn: boolean;
  loginError: unknown;
  isRegistering: boolean;
  registerError: unknown;
}

const SessionContext = createContext<SessionContextValue | null>(null);

/**
 * A 401 is the expected answer for "not signed in", so it resolves to null
 * rather than rejecting — a signed-out visitor is not an error condition.
 */
async function fetchSession(signal?: AbortSignal): Promise<Session | null> {
  try {
    return await api.getSession(signal);
  } catch (error) {
    if (error instanceof NeurunApiError && error.status === 401) return null;
    throw error;
  }
}

export function SessionProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();

  // Fetched once, for who the user is rather than whether they are signed in.
  // An expiry is discovered by the next request returning 401, which
  // `signOutOnUnauthorized` turns into a sign-out — so there is nothing to poll
  // for and no interval here.
  const sessionQuery = useQuery({
    queryKey: SESSION_QUERY_KEY,
    queryFn: ({ signal }) => fetchSession(signal),
    staleTime: Infinity,
    refetchOnWindowFocus: false,
    retry: false,
  });

  const loginMutation = useMutation({
    mutationFn: ({ username, password }: { username: string; password: string }) =>
      api.login(username, password),
    onSuccess: (session) => {
      queryClient.setQueryData(SESSION_QUERY_KEY, session);
    },
  });

  const login = useCallback(
    async (username: string, password: string) => {
      return loginMutation.mutateAsync({ username, password });
    },
    [loginMutation],
  );

  const registerMutation = useMutation({
    mutationFn: (request: api.RegisterRequest) => api.register(request),
    onSuccess: (session) => {
      // The server signs a new account in as part of registering, so seed the
      // cache rather than making the first paint wait on a round trip. When it
      // could not, leave the cache alone and let the sign-in form take over.
      if (session) {
        queryClient.setQueryData(SESSION_QUERY_KEY, session);
      }
    },
  });

  const register = useCallback(
    async (request: api.RegisterRequest) => registerMutation.mutateAsync(request),
    [registerMutation],
  );

  const logout = useCallback(async () => {
    try {
      await api.logout();
    } finally {
      // Clear regardless of the response: a failed sign-out must still drop this
      // user's cached evidence rather than leaving it on screen.
      queryClient.clear();
      queryClient.setQueryData(SESSION_QUERY_KEY, null);
    }
  }, [queryClient]);

  const value = useMemo<SessionContextValue>(
    () => ({
      session: sessionQuery.data ?? null,
      isLoading: sessionQuery.isPending,
      error: sessionQuery.error,
      login,
      register,
      logout,
      isLoggingIn: loginMutation.isPending,
      loginError: loginMutation.error,
      isRegistering: registerMutation.isPending,
      registerError: registerMutation.error,
    }),
    [
      sessionQuery.data,
      sessionQuery.isPending,
      sessionQuery.error,
      login,
      register,
      logout,
      loginMutation.isPending,
      loginMutation.error,
      registerMutation.isPending,
      registerMutation.error,
    ],
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
 * Cache partition for the signed-in user. Query keys carry this so one
 * user's evidence can never be read from another's cached data — the
 * session is also cleared outright on sign-out, so this is defence in depth.
 */
export function sessionScope(session: Session | null): string {
  if (!session) return "anonymous";
  return `${session.organization_id}#${session.user_id}`;
}
