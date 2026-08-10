"use client";

/**
 * Display preferences.
 *
 * Only non-secret presentation choices live here, so `localStorage` is fine —
 * unlike the API key, which never touches it. Theme is handled separately by
 * `next-themes` under the same storage convention.
 */
import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useSyncExternalStore,
  type ReactNode,
} from "react";
import type { TimeZoneMode } from "@/lib/view/time";
import { createPersistedStore } from "@/lib/storage/persisted-store";

const TIME_ZONE_KEY = "neurun.timezone";

// UTC is the default: it is the zone the API speaks, and the one two users
// in different places can agree on.
const timeZoneStore = createPersistedStore<TimeZoneMode>({
  key: TIME_ZONE_KEY,
  initial: "utc",
  area: () => window.localStorage,
  parse: (raw) => (raw === "utc" || raw === "local" ? raw : null),
  serialize: (value) => value,
});

interface PreferencesContextValue {
  timeZone: TimeZoneMode;
  setTimeZone: (mode: TimeZoneMode) => void;
  toggleTimeZone: () => void;
}

const PreferencesContext = createContext<PreferencesContextValue | null>(null);

export function PreferencesProvider({ children }: { children: ReactNode }) {
  const snapshot = useSyncExternalStore(
    timeZoneStore.subscribe,
    timeZoneStore.getSnapshot,
    timeZoneStore.getServerSnapshot,
  );
  const timeZone = snapshot.value;

  const setTimeZone = useCallback((mode: TimeZoneMode) => {
    timeZoneStore.set(mode);
  }, []);

  const toggleTimeZone = useCallback(() => {
    timeZoneStore.set(timeZone === "utc" ? "local" : "utc");
  }, [timeZone]);

  const value = useMemo(
    () => ({ timeZone, setTimeZone, toggleTimeZone }),
    [timeZone, setTimeZone, toggleTimeZone],
  );

  return <PreferencesContext.Provider value={value}>{children}</PreferencesContext.Provider>;
}

export function usePreferences(): PreferencesContextValue {
  const context = useContext(PreferencesContext);
  if (!context) {
    throw new Error("usePreferences must be used inside a PreferencesProvider.");
  }
  return context;
}
