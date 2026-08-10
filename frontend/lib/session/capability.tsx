"use client";

/**
 * Observed capabilities of the connected control plane.
 *
 * The current contract has no capability-discovery endpoint, so the session
 * learns two things only by attempting work, and must remember them:
 *
 * 1. **Durability.** Every accepted asynchronous mutation returns `durability`
 *    and the `Neurun-Job-Durability` header. The all-in-one server returns
 *    `process_local`, meaning queued jobs disappear on restart. List and detail
 *    Job objects do not repeat the field, so the warning is held at the
 *    connection level — inventing a per-job guarantee would be a lie.
 * 2. **Whether async is enabled at all.** A 503 `durable_backend_unavailable`
 *    means asynchronous mutations are off. Synchronous execution is unaffected
 *    and must stay available.
 *
 * Both reset when the session changes. A future `GET /version` carrying
 * `async_jobs_enabled` and `job_durability` should replace this inference.
 */
import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";
import { sessionScope, useSession } from "@/lib/session/store";

export type AsyncAvailability = "unknown" | "available" | "unavailable";

interface CapabilityContextValue {
  /** `process_local`, `durable`, or a future value, as reported by the server. */
  durability: string | null;
  asyncAvailability: AsyncAvailability;
  /** True only when the server explicitly said `durable`. */
  isDurable: boolean;
  /** True when the server said `process_local` — jobs are lost on restart. */
  isProcessLocal: boolean;
  recordDurability: (durability: string | undefined) => void;
  recordAsyncUnavailable: () => void;
}

const CapabilityContext = createContext<CapabilityContextValue | null>(null);

export function CapabilityProvider({ children }: { children: ReactNode }) {
  const { session } = useSession();
  const scope = sessionScope(session);

  const [observed, setObserved] = useState<{
    scope: string;
    durability: string | null;
    asyncAvailability: AsyncAvailability;
  }>({ scope, durability: null, asyncAvailability: "unknown" });

  // What was true for one session says nothing about the next. Adjusting
  // during render rather than in an effect means no frame ever shows the
  // previous connection's durability against the new one.
  if (observed.scope !== scope) {
    setObserved({ scope, durability: null, asyncAvailability: "unknown" });
  }

  const recordDurability = useCallback((value: string | undefined) => {
    if (!value) return;
    setObserved((previous) => ({
      ...previous,
      durability: value,
      // An accepted async mutation proves async is enabled here.
      asyncAvailability: "available",
    }));
  }, []);

  const recordAsyncUnavailable = useCallback(() => {
    setObserved((previous) => ({ ...previous, asyncAvailability: "unavailable" }));
  }, []);

  const { durability, asyncAvailability } =
    observed.scope === scope
      ? observed
      : { durability: null, asyncAvailability: "unknown" as AsyncAvailability };

  const value = useMemo<CapabilityContextValue>(
    () => ({
      durability,
      asyncAvailability,
      isDurable: durability === "durable",
      isProcessLocal: durability === "process_local",
      recordDurability,
      recordAsyncUnavailable,
    }),
    [durability, asyncAvailability, recordDurability, recordAsyncUnavailable],
  );

  return <CapabilityContext.Provider value={value}>{children}</CapabilityContext.Provider>;
}

export function useCapability(): CapabilityContextValue {
  const context = useContext(CapabilityContext);
  if (!context) {
    throw new Error("useCapability must be used inside a CapabilityProvider.");
  }
  return context;
}
