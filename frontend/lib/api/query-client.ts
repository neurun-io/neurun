import { QueryClient } from "@tanstack/react-query";
import { NeurunApiError, NeurunContractError } from "./errors";

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

export function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: shouldRetry,
        // Evidence goes stale quickly; an operator reading a job wants the
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
}
