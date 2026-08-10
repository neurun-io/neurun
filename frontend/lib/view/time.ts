/**
 * Time handling.
 *
 * - RFC 3339 timestamps are parsed once and the original instant is kept.
 * - The default display is relative, with the exact UTC value one hover away.
 * - The user chooses UTC or local for the exact value; the choice is a
 *   display preference and never changes the stored instant.
 */

export type TimeZoneMode = "utc" | "local";

/** Parse an RFC 3339 timestamp, returning null rather than an Invalid Date. */
export function parseInstant(value: string | undefined | null): Date | null {
  if (!value) return null;
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? null : parsed;
}

const UNITS: [limit: number, divisor: number, unit: Intl.RelativeTimeFormatUnit][] = [
  [60_000, 1_000, "second"],
  [3_600_000, 60_000, "minute"],
  [86_400_000, 3_600_000, "hour"],
  [604_800_000, 86_400_000, "day"],
  [2_629_800_000, 604_800_000, "week"],
  [31_557_600_000, 2_629_800_000, "month"],
  [Number.POSITIVE_INFINITY, 31_557_600_000, "year"],
];

const relativeFormatter = new Intl.RelativeTimeFormat("en", { numeric: "auto" });

/**
 * Relative time against `now`. Sub-5-second differences read as "just now"
 * rather than flickering between "in 1 second" and "1 second ago" on a poll.
 */
export function formatRelative(date: Date, now: Date = new Date()): string {
  const deltaMs = date.getTime() - now.getTime();
  const magnitude = Math.abs(deltaMs);

  if (magnitude < 5_000) return "just now";

  for (const [limit, divisor, unit] of UNITS) {
    if (magnitude < limit) {
      return relativeFormatter.format(Math.round(deltaMs / divisor), unit);
    }
  }
  return date.toISOString();
}

const UTC_FORMAT: Intl.DateTimeFormatOptions = {
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hour12: false,
  timeZone: "UTC",
};

const LOCAL_FORMAT: Intl.DateTimeFormatOptions = {
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hour12: false,
};

/** The exact instant, in the user's chosen zone, with the zone named. */
export function formatAbsolute(date: Date, mode: TimeZoneMode = "utc"): string {
  if (mode === "utc") {
    return `${new Intl.DateTimeFormat("en-GB", UTC_FORMAT).format(date).replace(",", "")} UTC`;
  }
  const formatted = new Intl.DateTimeFormat("en-GB", LOCAL_FORMAT).format(date).replace(",", "");
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone ?? "local";
  return `${formatted} ${zone}`;
}

/** Full precision, for copy actions and tooltips that must be unambiguous. */
export function formatIso(date: Date): string {
  return date.toISOString();
}

/**
 * Elapsed time between two instants, or from `start` to now when `end` is
 * absent — the case for an attempt that has not reported back yet.
 */
export function elapsedMs(start: Date, end: Date | null, now: Date = new Date()): number {
  return (end ?? now).getTime() - start.getTime();
}
