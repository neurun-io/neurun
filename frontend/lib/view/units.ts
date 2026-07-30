/**
 * Unit handling.
 *
 * - Fields named `*_ms` are milliseconds.
 * - `DurableRequest.retry_policy.initial_backoff` and `max_backoff` are Go
 *   durations encoded as **nanoseconds**. They are converted explicitly here.
 *   Never infer a unit from an untyped retry payload; a future contract should
 *   normalise these to named millisecond fields.
 * - Bytes are displayed in binary units, with the exact byte count preserved.
 * - Rates, gauges and cumulative counters are distinguished at the call site.
 */

const NS_PER_MS = 1_000_000;

/** Convert a Go nanosecond duration to milliseconds. */
export function nanosecondsToMs(nanoseconds: number): number {
  return nanoseconds / NS_PER_MS;
}

/**
 * Format a Go nanosecond duration. Separate from `formatDurationMs` on purpose:
 * the conversion should be visible at every call site that consumes a retry
 * policy, not hidden behind a shared formatter.
 */
export function formatNanoseconds(nanoseconds: number): string {
  return formatDurationMs(nanosecondsToMs(nanoseconds));
}

/** Human duration from milliseconds, precise at small magnitudes. */
export function formatDurationMs(ms: number): string {
  if (!Number.isFinite(ms)) return "—";
  const absolute = Math.abs(ms);

  if (absolute < 1) return `${Math.round(ms * 1000)}µs`;
  if (absolute < 1_000) return `${Math.round(ms)}ms`;
  if (absolute < 60_000) {
    const seconds = ms / 1_000;
    return `${seconds >= 10 ? seconds.toFixed(1) : seconds.toFixed(2)}s`;
  }
  if (absolute < 3_600_000) {
    const minutes = Math.floor(ms / 60_000);
    const seconds = Math.round((ms % 60_000) / 1_000);
    return `${minutes}m ${seconds}s`;
  }
  const hours = Math.floor(ms / 3_600_000);
  const minutes = Math.round((ms % 3_600_000) / 60_000);
  return `${hours}h ${minutes}m`;
}

const BINARY_UNITS = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"] as const;

/**
 * Binary byte units for display. The exact byte count stays available for
 * detail views — see `formatBytesExact`.
 */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes)) return "—";
  if (bytes === 0) return "0 B";

  const exponent = Math.min(
    Math.floor(Math.log2(Math.abs(bytes)) / 10),
    BINARY_UNITS.length - 1,
  );
  const scaled = bytes / 1024 ** exponent;
  const digits = exponent === 0 ? 0 : scaled >= 100 ? 0 : scaled >= 10 ? 1 : 2;
  return `${scaled.toFixed(digits)} ${BINARY_UNITS[exponent]}`;
}

/** Rounded binary value plus the exact count, for detail rows. */
export function formatBytesExact(bytes: number): string {
  if (!Number.isFinite(bytes)) return "—";
  if (bytes < 1024) return `${bytes.toLocaleString("en-US")} B`;
  return `${formatBytes(bytes)} (${bytes.toLocaleString("en-US")} bytes)`;
}

export function formatCount(value: number): string {
  return value.toLocaleString("en-US");
}

/** CPU seconds, a cumulative counter rather than a rate. */
export function formatCpuSeconds(seconds: number): string {
  if (!Number.isFinite(seconds)) return "—";
  if (seconds < 1) return `${(seconds * 1000).toFixed(0)}ms CPU`;
  return `${seconds.toFixed(2)}s CPU`;
}

/**
 * The em dash is the system's "no value". A missing metric is never rendered
 * as zero — an absent number and a measured zero are different facts.
 */
export const NO_VALUE = "—";

export function orNoValue(
  value: number | string | undefined | null,
  format: (value: never) => string = String as never,
): string {
  if (value === undefined || value === null) return NO_VALUE;
  return format(value as never);
}
