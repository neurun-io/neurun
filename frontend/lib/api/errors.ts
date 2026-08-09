import type { ErrorEnvelope, Problem } from "./types";

/** A failed API response, carrying the standard envelope. */
export class NeurunApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details?: Problem["details"];
  readonly retryAfter?: string;
  /** The parsed body, when the server returned a well-formed envelope. */
  readonly envelope?: ErrorEnvelope;

  constructor(init: {
    status: number;
    code: string;
    message: string;
    details?: Problem["details"];
    retryAfter?: string;
    envelope?: ErrorEnvelope;
  }) {
    super(init.message);
    this.name = "NeurunApiError";
    this.status = init.status;
    this.code = init.code;
    this.details = init.details;
    this.retryAfter = init.retryAfter;
    this.envelope = init.envelope;
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
