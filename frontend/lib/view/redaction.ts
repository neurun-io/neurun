/**
 * Client-side secret redaction.
 *
 * Applied before an immutable request is copied to the clipboard, and before
 * any payload is shown in a JSON viewer. This is defence in depth, not a
 * substitute for the server's own redaction: the server owns the authoritative
 * `redaction` policy in each function manifest, and this pass only stops the
 * obvious things an operator would otherwise paste into a ticket.
 */

const SECRET_KEY_PATTERN =
  /(^|[_-])(secret|password|passwd|token|api[_-]?key|apikey|authorization|auth|credential|credentials|private[_-]?key|session[_-]?key|cookie|bearer|signature|salt)([_-]|$)/i;

/** Placeholder shown in place of a redacted value. */
export const REDACTED = "[redacted]";

/** Whether a key name looks like it holds a secret. */
export function isSecretKey(key: string): boolean {
  return SECRET_KEY_PATTERN.test(key);
}

/**
 * A Neurun API key in `neu_<environment>_<prefix>.<secret>` form, wherever it
 * appears inside a string. The non-secret prefix is preserved so the value
 * remains identifiable.
 */
const API_KEY_PATTERN = /\b(neu_[a-z0-9]+_[A-Za-z0-9]+)\.[A-Za-z0-9._-]+/g;

/** `Bearer <token>` inside a header string. */
const BEARER_PATTERN = /\b(Bearer)\s+[A-Za-z0-9._~+/-]+=*/gi;

/**
 * Order matters: the bearer pattern runs first so a full `Bearer neu_x_y.z`
 * header is replaced once. Running the key pattern first would rewrite the
 * token's tail, and the bearer pattern would then redact the surviving head a
 * second time — producing `Bearer [redacted][redacted]`.
 */
export function redactString(value: string): string {
  return value.replace(BEARER_PATTERN, `$1 ${REDACTED}`).replace(API_KEY_PATTERN, `$1.${REDACTED}`);
}

/**
 * Walk a payload and redact secret-looking values.
 *
 * Structure is preserved: an operator comparing a redacted copy against the
 * original should see the same shape, with the sensitive leaves replaced.
 * Cycles are tolerated, so this is safe on arbitrary server payloads.
 */
export function redactSecrets<T>(value: T): T {
  return redact(value, new WeakSet()) as T;
}

function redact(value: unknown, seen: WeakSet<object>): unknown {
  if (typeof value === "string") return redactString(value);
  if (value === null || typeof value !== "object") return value;

  if (seen.has(value)) return "[circular]";
  seen.add(value);

  if (Array.isArray(value)) {
    return value.map((item) => redact(item, seen));
  }

  const result: Record<string, unknown> = {};
  for (const [key, entry] of Object.entries(value as Record<string, unknown>)) {
    result[key] = isSecretKey(key) ? REDACTED : redact(entry, seen);
  }
  return result;
}

/** Redacted, pretty-printed JSON — what the copy actions put on the clipboard. */
export function toRedactedJson(value: unknown): string {
  return JSON.stringify(redactSecrets(value), null, 2);
}
