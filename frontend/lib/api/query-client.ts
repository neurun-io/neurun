import { MutationCache, QueryCache, QueryClient } from "@tanstack/react-query";
import { NeurunApiError, NeurunContractError } from "./errors";
import { SESSION_QUERY_KEY } from "@/lib/session/store";

/** Poll cadence for non-terminal jobs, their attempts and their event stream. */
export const LIVE_POLL_INTERVAL_MS = 2_000;

/**
 * Retrying an authentication or authorization failure just burns requests
 * against a key the server has already rejected — and after a revocation the
 * dashboard must stop, not hammer. Contract violations are not retryable
 * either: the same response will fail the same way.
 */
export function shouldRetry(failureCount: number, error: unknown): boolean {
  if (error instanceof NeurunContractError) return false;
  if (error instanceof NeurunApiError) {
    if (error.status === 401 || error.status === 403 || error.status === 404) return false;
    if (error.status >= 400 && error.status < 500) return false;
  }
  return failureCount < 2;
}

/**
 * A 401 anywhere means the session is gone — expired, revoked, or the account
 * disabled. Being authenticated but not permitted is a 403, so this is never
 * ambiguous, and it is why nothing polls to ask whether the session still
 * holds: the next request the user makes says so.
 *
 * The session request itself resolves on 401 rather than rejecting, so this
 * never fires for a visitor who is simply not signed in.
 */
function signOutOnUnauthorized(client: QueryClient, error: unknown) {
  if (!(error instanceof NeurunApiError) || error.status !== 401) return;
  client.removeQueries({ predicate: (query) => query.queryKey[1] !== "session" });
  client.setQueryData(SESSION_QUERY_KEY, null);
}

export function createQueryClient(): QueryClient {
  const client: QueryClient = new QueryClient({
    queryCache: new QueryCache({
      onError: (error) => signOutOnUnauthorized(client, error),
    }),
    mutationCache: new MutationCache({
      onError: (error) => signOutOnUnauthorized(client, error),
    }),
    defaultOptions: {
      queries: {
        retry: shouldRetry,
        // Evidence goes stale quickly; a user reading a job wants the
        // current snapshot, not a cached one from two minutes ago.
        staleTime: 5_000,
        gcTime: 5 * 60_000,
        // Refetch the moment the document becomes visible again.
        refetchOnWindowFocus: true,
        refetchOnReconnect: true,
        // Nonessential polling pauses while the document is hidden.
        refetchIntervalInBackground: false,
      },
      mutations: {
        retry: false,
      },
    },
  });
  return client;
}
