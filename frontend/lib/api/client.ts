/**
 * The single authenticated API client.
 *
 * Every network call in the dashboard goes through here. Two consequences that
 * are load-bearing rather than stylistic:
 *
 * - **Same-origin only.** The control plane ships no CORS or `OPTIONS`
 *   middleware, so the browser never talks to it directly. Requests go to this
 *   app's own `/api/proxy/*` route handler, which forwards them. See
 *   `app/api/proxy/[...path]/route.ts`.
 * - **The key lives in memory.** It is passed in per call from the connection
 *   store and is never written to `localStorage`, IndexedDB, a service-worker
 *   cache, a URL, or an error breadcrumb.
 */
import {
  NeurunApiError,
  NeurunTransportError,
  parseErrorEnvelope,
} from "./errors";
import { idempotencyKeys } from "./idempotency";
import { validateResponse } from "./runtime";
import type { z } from "zod";

/** Where the proxy route lives on this origin. */
export const PROXY_PREFIX = "/api/proxy";

/** Header the proxy reads to learn the upstream control plane, then strips. */
export const BASE_URL_HEADER = "X-Neurun-Base-Url";

export interface Connection {
  /** Control-plane base URL, e.g. `http://localhost:8080`. */
  baseUrl: string;
  /** `neu_<environment>_<prefix>.<secret>`. Held in memory. */
  apiKey: string;
}

export interface RequestOptions {
  method?: "GET" | "POST";
  /** Path below the control-plane root, e.g. `/v1/jobs`. */
  path: string;
  query?: Record<string, string | number | string[] | undefined>;
  body?: unknown;
  /** Send an `Idempotency-Key`, generated and reused per logical request. */
  idempotent?: boolean;
  signal?: AbortSignal;
}

/** Response metadata an operator may need to correlate or escalate. */
export interface ResponseMeta {
  status: number;
  requestId?: string;
  traceId?: string;
  /** Present on accepted asynchronous mutations. */
  durability?: string;
  /** `"true"` when the server replayed a prior idempotent request. */
  idempotentReplayed: boolean;
  location?: string;
  retryAfter?: string;
}

export interface ApiResult<T> {
  data: T;
  meta: ResponseMeta;
}

function buildQuery(query: RequestOptions["query"]): string {
  if (!query) return "";
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value === undefined) continue;
    if (Array.isArray(value)) {
      // `status` repeats rather than comma-joins; the contract declares
      // style: form, explode: true.
      for (const item of value) params.append(key, item);
    } else {
      params.append(key, String(value));
    }
  }
  const encoded = params.toString();
  return encoded ? `?${encoded}` : "";
}

function readMeta(response: Response): ResponseMeta {
  return {
    status: response.status,
    requestId: response.headers.get("Request-ID") ?? undefined,
    traceId: response.headers.get("Trace-ID") ?? undefined,
    durability: response.headers.get("Neurun-Job-Durability") ?? undefined,
    idempotentReplayed: response.headers.get("Idempotent-Replayed") === "true",
    location: response.headers.get("Location") ?? undefined,
    retryAfter: response.headers.get("Retry-After") ?? undefined,
  };
}

async function decodeBody(response: Response): Promise<unknown> {
  if (response.status === 204) return undefined;
  const text = await response.text();
  if (!text) return undefined;
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

/**
 * Perform one authenticated request and return the decoded body plus metadata.
 * Throws `NeurunApiError` for any non-2xx response and `NeurunTransportError`
 * when no response arrived at all.
 */
export async function request<T>(
  connection: Connection,
  options: RequestOptions,
  schema?: z.ZodType<T>,
): Promise<ApiResult<T>> {
  const method = options.method ?? "GET";
  const path = options.path;
  const url = `${PROXY_PREFIX}${path}${buildQuery(options.query)}`;

  const headers = new Headers({
    Accept: "application/json",
    [BASE_URL_HEADER]: connection.baseUrl,
  });
  if (connection.apiKey) {
    headers.set("Authorization", `Bearer ${connection.apiKey}`);
  }
  if (options.body !== undefined) {
    headers.set("Content-Type", "application/json");
  }
  if (options.idempotent) {
    headers.set("Idempotency-Key", idempotencyKeys.keyFor(method, path, options.body));
  }

  let response: Response;
  try {
    response = await fetch(url, {
      method,
      headers,
      body: options.body === undefined ? undefined : JSON.stringify(options.body),
      signal: options.signal,
      // Never let a browser or intermediary cache an operator's evidence.
      cache: "no-store",
      credentials: "same-origin",
    });
  } catch (cause) {
    // Deliberately do NOT release the idempotency key here: the server may
    // already hold this request, so the retry must reuse the same key.
    if (cause instanceof DOMException && cause.name === "AbortError") throw cause;
    throw new NeurunTransportError(
      "Could not reach the control plane. Check the base URL and that the server is running.",
      cause,
    );
  }

  const meta = readMeta(response);
  const body = await decodeBody(response);

  if (!response.ok) {
    const envelope = parseErrorEnvelope(body);
    if (options.idempotent && response.status >= 400 && response.status < 500) {
      // The server decided on the merits; a fresh submission deserves a fresh key.
      idempotencyKeys.release(method, path, options.body);
    }
    throw new NeurunApiError({
      status: response.status,
      code: envelope?.error.code ?? `http_${response.status}`,
      message:
        envelope?.error.message ??
        (typeof body === "string" && body
          ? body
          : `Request failed with HTTP ${response.status}.`),
      details: envelope?.error.details,
      requestId: envelope?.request_id ?? meta.requestId,
      traceId: meta.traceId,
      retryAfter: meta.retryAfter,
      envelope,
    });
  }

  if (options.idempotent) {
    idempotencyKeys.release(method, path, options.body);
  }

  const data = schema ? validateResponse(schema, path, body) : (body as T);
  return { data, meta };
}

/** Convenience wrapper for callers that do not need response metadata. */
export async function requestData<T>(
  connection: Connection,
  options: RequestOptions,
  schema?: z.ZodType<T>,
): Promise<T> {
  const { data } = await request(connection, options, schema);
  return data;
}
