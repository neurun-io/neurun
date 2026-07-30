"use client";

/**
 * Connection state: control-plane base URL + API key.
 *
 * Storage rules, from the spec and not negotiable:
 * - The key lives in memory by default.
 * - An explicit "remember for this browser session" option may use
 *   `sessionStorage`.
 * - Never `localStorage`, IndexedDB, service-worker caches, analytics payloads,
 *   URLs, or error-report breadcrumbs.
 *
 * The authenticated project is whatever the key resolves to. A locally selected
 * project ID is never treated as authority; project discovery and switching
 * need the future operator-session and project APIs.
 */
import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useSyncExternalStore,
  type ReactNode,
} from "react";
import { useQueryClient } from "@tanstack/react-query";
import type { Connection } from "@/lib/api/client";
import { createPersistedStore } from "@/lib/storage/persisted-store";

const SESSION_STORAGE_KEY = "neurun.connection";

interface StoredConnection {
  connection: Connection | null;
  remembered: boolean;
}

/**
 * `sessionStorage`, never `localStorage`: the key must not outlive the tab.
 * Read through an external store so hydration is correct without an effect.
 */
const connectionStore = createPersistedStore<StoredConnection>({
  key: SESSION_STORAGE_KEY,
  initial: { connection: null, remembered: false },
  area: () => window.sessionStorage,
  parse: (raw) => {
    try {
      const parsed = JSON.parse(raw) as Partial<Connection>;
      if (typeof parsed.baseUrl !== "string" || typeof parsed.apiKey !== "string") return null;
      return {
        connection: { baseUrl: parsed.baseUrl, apiKey: parsed.apiKey },
        remembered: true,
      };
    } catch {
      return null;
    }
  },
  serialize: ({ connection, remembered }) =>
    connection && remembered ? JSON.stringify(connection) : null,
});

export interface ConnectionContextValue {
  connection: Connection | null;
  /** False until the session-storage read has run, to avoid an SSR mismatch. */
  hydrated: boolean;
  remembered: boolean;
  connect: (connection: Connection, remember: boolean) => void;
  disconnect: () => void;
}

const ConnectionContext = createContext<ConnectionContextValue | null>(null);

/**
 * The non-secret half of an API key: `neu_<environment>_<prefix>` from
 * `neu_<environment>_<prefix>.<secret>`. Safe to display and to use as a cache
 * partition. The secret is everything after the first `.` and never leaves
 * memory.
 */
export function apiKeyIdentity(apiKey: string): string {
  const separator = apiKey.indexOf(".");
  return separator === -1 ? apiKey.slice(0, 12) : apiKey.slice(0, separator);
}

/**
 * Cache partition for a connection. Query keys carry this so a different key or
 * control plane can never read another one's cached data.
 */
export function connectionScope(connection: Connection | null): string {
  if (!connection) return "disconnected";
  return `${connection.baseUrl}#${apiKeyIdentity(connection.apiKey)}`;
}

export function ConnectionProvider({ children }: { children: ReactNode }) {
  const snapshot = useSyncExternalStore(
    connectionStore.subscribe,
    connectionStore.getSnapshot,
    connectionStore.getServerSnapshot,
  );
  const queryClient = useQueryClient();

  const connect = useCallback(
    (next: Connection, remember: boolean) => {
      // Switching connections must not leave the previous project's evidence
      // readable in the cache.
      queryClient.clear();
      connectionStore.set({ connection: next, remembered: remember }, remember);
    },
    [queryClient],
  );

  const disconnect = useCallback(() => {
    queryClient.clear();
    connectionStore.clear();
  }, [queryClient]);

  const value = useMemo<ConnectionContextValue>(
    () => ({
      connection: snapshot.value.connection,
      remembered: snapshot.value.remembered,
      hydrated: snapshot.hydrated,
      connect,
      disconnect,
    }),
    [snapshot, connect, disconnect],
  );

  return <ConnectionContext.Provider value={value}>{children}</ConnectionContext.Provider>;
}

export function useConnection(): ConnectionContextValue {
  const context = useContext(ConnectionContext);
  if (!context) {
    throw new Error("useConnection must be used inside a ConnectionProvider.");
  }
  return context;
}

/**
 * The connection, asserted present. Use inside routes that already sit behind
 * the connection gate.
 */
export function useRequiredConnection(): Connection {
  const { connection } = useConnection();
  if (!connection) {
    throw new Error("This route requires an active control-plane connection.");
  }
  return connection;
}

/** Test seam: drop any in-memory or stored connection. */
export function resetConnectionStore() {
  connectionStore.clear();
}
