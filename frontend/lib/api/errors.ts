import type { ErrorEnvelope, Problem } from "./types";

/**
 * The error code returned (with HTTP 503) when asynchronous mutations are
 * disabled because no durable backend is configured and volatile development
 * jobs were not explicitly enabled. Synchronous execution remains available,
 * so the UI disables only the async control on this code.
 */
export const DURABLE_BACKEND_UNAVAILABLE = "durable_backend_unavailable";

/**
 * A failed API response, carrying the standard envelope plus the transport
 * correlation IDs an operator needs to file a report.
 *
 * Every surface that renders one of these must expose a copyable request ID.
 */
export class NeurunApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details?: Problem["details"];
  readonly requestId?: string;
  readonly traceId?: string;
  readonly retryAfter?: string;
  /** The parsed body, when the server returned a well-formed envelope. */
  readonly envelope?: ErrorEnvelope;

  constructor(init: {
    status: number;
    code: string;
    message: string;
    details?: Problem["details"];
    requestId?: string;
    traceId?: string;
    retryAfter?: string;
    envelope?: ErrorEnvelope;
  }) {
    super(init.message);
    this.name = "NeurunApiError";
    this.status = init.status;
    this.code = init.code;
    this.details = init.details;
    this.requestId = init.requestId;
    this.traceId = init.traceId;
    this.retryAfter = init.retryAfter;
    this.envelope = init.envelope;
  }

  /** Async mutations are disabled on this deployment; sync still works. */
  get isDurableBackendUnavailable() {
    return this.status === 503 && this.code === DURABLE_BACKEND_UNAVAILABLE;
  }

  /** The key was rejected. Stop retrying and send the operator back to connect. */
  get isUnauthenticated() {
    return this.status === 401;
  }

  get isForbidden() {
    return this.status === 403;
  }

  get isNotFound() {
    return this.status === 404;
  }

  /**
   * A terminal function failure carries the persisted invocation snapshot in
   * `error.details.invocation` (HTTP 502 / 504).
   */
  get invocationSnapshot(): unknown {
    const details = this.details as Record<string, unknown> | undefined;
    return details?.invocation;
  }
}

/** A transport-level failure: the request never produced an HTTP response. */
export class NeurunTransportError extends Error {
  readonly cause?: unknown;

  constructor(message: string, cause?: unknown) {
    super(message);
    this.name = "NeurunTransportError";
    this.cause = cause;
  }
}

/**
 * A response whose body did not match the published contract. Surfaced rather
 * than swallowed: a silently coerced payload is how a dashboard starts lying.
 */
export class NeurunContractError extends Error {
  readonly path: string;
  readonly issues: string[];

  constructor(path: string, issues: string[]) {
    super(`Response from ${path} did not match the published contract.`);
    this.name = "NeurunContractError";
    this.path = path;
    this.issues = issues;
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

/** Narrow an unknown body to the standard error envelope, if it is one. */
export function parseErrorEnvelope(body: unknown): ErrorEnvelope | undefined {
  if (!isRecord(body) || !isRecord(body.error)) return undefined;
  const { code, message } = body.error;
  if (typeof code !== "string" || typeof message !== "string") return undefined;
  return body as unknown as ErrorEnvelope;
}

export function isNeurunApiError(error: unknown): error is NeurunApiError {
  return error instanceof NeurunApiError;
}

/**
 * Current validation failures expose human-readable paths such as `$.field`
 * inside the message rather than structured JSON Pointer violations. Preserve
 * and display the message as written — do not parse it into field state.
 * Add field-level mapping only once the backend publishes structured
 * violations.
 */
export function errorMessageFor(error: unknown): string {
  if (error instanceof NeurunApiError) return error.message;
  if (error instanceof NeurunContractError) return error.message;
  if (error instanceof NeurunTransportError) return error.message;
  if (error instanceof Error) return error.message;
  return "Unknown error";
}
