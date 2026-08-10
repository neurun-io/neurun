/**
 * Status presentation.
 *
 * Two rules govern this file.
 *
 * **State is carried without colour.** The palette is monochrome, so a status
 * is distinguished by glyph + border style + fill + pattern. Colour, where it
 * exists at all, is supplementary.
 *
 * **Unknown values render, they do not crash.** A status this table does not
 * know is shown as a neutral badge carrying its raw text. It is never mapped
 * onto success, and it never takes down a route. That is what lets the
 * dashboard survive a server that learns a new state before the client does.
 */

/** Visual treatments from the design system's status legend. */
export type StatusTreatment =
  | "solid" // solid border + check — settled successfully
  | "pulse" // pulsing node — actively running
  | "dashed" // dashed border — accepted, waiting to start
  | "hatch" // 45° hatch — degraded, waiting to retry
  | "rejected" // strong border + slash — refused on its merits
  | "inverted" // inverted fill — terminal failure
  | "strike" // strike-through, muted — deliberately stopped
  | "neutral"; // neutral inset — unknown

/** Semantic tone, used for screen-reader text and grouping, never for hue. */
export type StatusTone =
  | "success"
  | "active"
  | "informational"
  | "warning"
  | "danger"
  | "neutral"
  | "unknown";

export interface StatusDescriptor {
  /** The raw API value, always displayed verbatim. */
  value: string;
  treatment: StatusTreatment;
  tone: StatusTone;
  /** Lucide icon name, or null where the treatment itself is the signal. */
  icon: string | null;
  /** Whether this value is one the client recognises. */
  known: boolean;
  /** Plain-language gloss for assistive technology. */
  description: string;
}

interface Entry {
  treatment: StatusTreatment;
  tone: StatusTone;
  icon: string | null;
  description: string;
}

/**
 * The status legend. Job states, invocation statuses and attempt states share
 * it, because a user reading a job detail should not have to learn three
 * vocabularies for the same ideas.
 */
const LEGEND: Record<string, Entry> = {
  // settled successfully
  succeeded: {
    treatment: "solid",
    tone: "success",
    icon: "check",
    description: "Completed successfully",
  },

  // accepted, not yet started
  accepted: {
    treatment: "dashed",
    tone: "informational",
    icon: null,
    description: "Accepted, not yet queued",
  },
  queued: {
    treatment: "dashed",
    tone: "informational",
    icon: null,
    description: "Queued, waiting for an agent",
  },
  ready: { treatment: "dashed", tone: "informational", icon: null, description: "Ready" },

  // actively running
  leased: {
    treatment: "pulse",
    tone: "active",
    icon: null,
    description: "Leased to an agent",
  },
  running: { treatment: "pulse", tone: "active", icon: null, description: "Running now" },
  connected: { treatment: "pulse", tone: "active", icon: null, description: "Connected" },
  provisioning: { treatment: "pulse", tone: "active", icon: null, description: "Provisioning" },

  // degraded, still recoverable
  retry_wait: {
    treatment: "hatch",
    tone: "warning",
    icon: "rotate-ccw",
    description: "Waiting before the next attempt",
  },
  lease_expired: {
    treatment: "hatch",
    tone: "warning",
    icon: "timer-off",
    description: "Lease expired before the attempt reported back",
  },
  disconnected: {
    treatment: "hatch",
    tone: "warning",
    icon: "unplug",
    description: "Disconnected",
  },

  // refused on its merits — distinct from a transport failure
  rejected: {
    treatment: "rejected",
    tone: "warning",
    icon: "slash",
    description: "Rejected — the request or its data failed validation",
  },

  // terminal failure
  failed: {
    treatment: "inverted",
    tone: "danger",
    icon: "x",
    description: "Failed",
  },
  timed_out: {
    treatment: "inverted",
    tone: "danger",
    icon: "timer-off",
    description: "Timed out",
  },
  dead_lettered: {
    treatment: "inverted",
    tone: "danger",
    icon: "archive-x",
    description: "Dead-lettered after exhausting its attempts",
  },
  crashed: { treatment: "inverted", tone: "danger", icon: "zap-off", description: "Crashed" },
  oom_killed: {
    treatment: "inverted",
    tone: "danger",
    icon: "zap-off",
    description: "Killed after exceeding its memory limit",
  },

  // deliberately stopped
  canceled: { treatment: "strike", tone: "neutral", icon: null, description: "Canceled" },
  cancel_requested: {
    treatment: "hatch",
    tone: "warning",
    icon: "circle-stop",
    description: "Cancellation requested",
  },
  closed: { treatment: "strike", tone: "neutral", icon: null, description: "Closed" },
  expired: { treatment: "strike", tone: "neutral", icon: null, description: "Expired" },
};

const UNKNOWN: Entry = {
  treatment: "neutral",
  tone: "unknown",
  icon: "circle-help",
  description: "Unrecognised status reported by the server",
};

/**
 * Describe a status value. Always returns a descriptor — an unrecognised value
 * comes back neutral, marked `known: false`, carrying its raw text.
 */
export function describeStatus(value: string | undefined | null): StatusDescriptor {
  const raw = (value ?? "").trim();
  if (!raw) {
    return { value: "unknown", ...UNKNOWN, known: false };
  }

  const entry = LEGEND[raw];
  if (!entry) {
    return {
      value: raw,
      ...UNKNOWN,
      known: false,
      description: `Unrecognised status "${raw}" reported by the server`,
    };
  }

  return { value: raw, ...entry, known: true };
}

export function isKnownStatus(value: string): boolean {
  return Object.hasOwn(LEGEND, value);
}
