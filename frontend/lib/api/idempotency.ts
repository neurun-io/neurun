/**
 * Idempotency-Key generation and reuse.
 *
 * The contract requires a key on `POST /v1/jobs` and on `/invoke` or `/fetch`
 * with `execution=async`. Cancellation does not take one — it is intrinsically
 * idempotent for an already-terminal job.
 *
 * The rule that matters after a network failure: the retry must carry the SAME
 * key as the attempt that may already have been accepted, and it must carry the
 * byte-equivalent logical request. Minting a fresh key on retry is how you
 * submit the same job twice. So keys are memoised against a stable fingerprint
 * of the logical request and only released once the request reaches a decisive
 * outcome (accepted, or rejected on its merits).
 */

/** Deterministic JSON: object keys sorted at every depth. */
export function stableStringify(value: unknown): string {
  if (value === null || typeof value !== "object") return JSON.stringify(value) ?? "null";
  if (Array.isArray(value)) return `[${value.map(stableStringify).join(",")}]`;

  const entries = Object.entries(value as Record<string, unknown>)
    .filter(([, v]) => v !== undefined)
    .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
    .map(([k, v]) => `${JSON.stringify(k)}:${stableStringify(v)}`);
  return `{${entries.join(",")}}`;
}

export function fingerprint(method: string, path: string, body: unknown): string {
  return `${method.toUpperCase()} ${path} ${stableStringify(body)}`;
}

function randomKey(): string {
  const globalCrypto = globalThis.crypto;
  if (globalCrypto?.randomUUID) return globalCrypto.randomUUID();
  // Non-secret correlation value; a timestamped random suffix is sufficient
  // wherever `crypto.randomUUID` is unavailable.
  return `idem_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 12)}`;
}

export class IdempotencyKeyStore {
  private readonly keys = new Map<string, string>();

  /**
   * Return the key for this logical request, minting one on first use and
   * returning the identical key for every subsequent attempt.
   */
  keyFor(method: string, path: string, body: unknown): string {
    const id = fingerprint(method, path, body);
    const existing = this.keys.get(id);
    if (existing) return existing;

    const key = randomKey();
    this.keys.set(id, key);
    return key;
  }

  /**
   * Release a key once its request reached a decisive outcome, so an operator
   * who intentionally submits the same payload again gets a new job.
   *
   * Not called after a transport failure: that is precisely the case where the
   * server may already hold the request and the key must be reused.
   */
  release(method: string, path: string, body: unknown): void {
    this.keys.delete(fingerprint(method, path, body));
  }

  /** Test/reset seam. */
  clear(): void {
    this.keys.clear();
  }

  get size(): number {
    return this.keys.size;
  }
}

/** Process-wide store. Keys are correlation values, never secrets. */
export const idempotencyKeys = new IdempotencyKeyStore();
