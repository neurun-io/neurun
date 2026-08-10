/**
 * A `useSyncExternalStore`-compatible store backed by Web Storage.
 *
 * Browser storage is an external system, so it is read through a subscription
 * rather than restored inside an effect. That avoids the cascading render the
 * effect-then-setState pattern causes, and — more importantly — it gives a
 * correct server snapshot, so the server and the first client render agree and
 * hydration never mismatches.
 *
 * `hydrated` is part of the snapshot: consumers can distinguish "no value" from
 * "not read yet", which is the difference between showing a connection screen
 * and briefly flashing one at a user who is already connected.
 */

export interface PersistedSnapshot<T> {
  value: T;
  hydrated: boolean;
}

export interface PersistedStore<T> {
  subscribe: (listener: () => void) => () => void;
  getSnapshot: () => PersistedSnapshot<T>;
  getServerSnapshot: () => PersistedSnapshot<T>;
  /** Update the in-memory value; `persist` controls whether it is written. */
  set: (value: T, persist?: boolean) => void;
  /** Reset to the initial value and remove any stored copy. */
  clear: () => void;
}

export function createPersistedStore<T>(options: {
  key: string;
  initial: T;
  /** Which storage area to use, read lazily so SSR never touches it. */
  area: () => Storage;
  /** Return null for a value that should be ignored. */
  parse: (raw: string) => T | null;
  /** Return null to remove the stored copy instead of writing one. */
  serialize: (value: T) => string | null;
}): PersistedStore<T> {
  const { key, initial, area, parse, serialize } = options;

  const serverSnapshot: PersistedSnapshot<T> = { value: initial, hydrated: false };
  let snapshot: PersistedSnapshot<T> = serverSnapshot;
  let restored = false;

  const listeners = new Set<() => void>();

  function emit() {
    for (const listener of listeners) listener();
  }

  function restore() {
    if (restored) return;
    restored = true;

    let value = initial;
    try {
      const raw = area().getItem(key);
      if (raw !== null) {
        const parsed = parse(raw);
        if (parsed !== null) value = parsed;
      }
    } catch {
      // Storage can be unavailable (private mode, blocked cookies). The
      // in-memory default stands.
    }
    snapshot = { value, hydrated: true };
  }

  return {
    subscribe(listener) {
      // Subscription happens after mount, which is exactly when reading
      // browser storage becomes safe.
      restore();
      listeners.add(listener);
      // The restore may have produced a new snapshot; tell this subscriber.
      listener();
      return () => {
        listeners.delete(listener);
      };
    },

    getSnapshot() {
      return snapshot;
    },

    getServerSnapshot() {
      return serverSnapshot;
    },

    set(value, persist = true) {
      restored = true;
      snapshot = { value, hydrated: true };
      try {
        const serialized = persist ? serialize(value) : null;
        if (serialized === null) {
          area().removeItem(key);
        } else {
          area().setItem(key, serialized);
        }
      } catch {
        // Not persisted; the in-memory value still works for this tab.
      }
      emit();
    },

    clear() {
      restored = true;
      snapshot = { value: initial, hydrated: true };
      try {
        area().removeItem(key);
      } catch {
        // nothing to clear
      }
      emit();
    },
  };
}
