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
 * - **The browser holds no credential it can read.** Authentication is an
 *   `HttpOnly` session cookie issued by `POST /v1/auth/login`. There is
 *   no API key in any client module, no bearer header assembled here, and
 *   nothing written to `sessionStorage`, `localStorage` or a URL. The cookie
 *   rides along because the browser attaches it, and script cannot read it.
 */
import { NeurunApiError, NeurunTransportError, parseErrorEnvelope } from "./errors";
import { validateResponse } from "./runtime";
import type { z } from "zod";

/** Where the proxy route lives on this origin. */
export const PROXY_PREFIX = "/api/proxy";

export interface RequestOptions {
  method?: "GET" | "POST" | "PATCH" | "DELETE";
  /** Path below the control-plane root, e.g. `/v1/jobs`. */
  path: string;
  query?: Record<string, string | number | string[] | undefined>;
  body?: unknown;
  signal?: AbortSignal;
}

/** Response metadata a user may need to correlate or escalate. */
export interface ResponseMeta {
  status: number;
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
 * Perform one request and return the decoded body plus metadata. Throws
 * `NeurunApiError` for any non-2xx response and `NeurunTransportError` when no
 * response arrived at all.
 */
export async function request<T>(
  options: RequestOptions,
  schema?: z.ZodType<T>,
): Promise<ApiResult<T>> {
  const method = options.method ?? "GET";
  const path = options.path;
  const url = `${PROXY_PREFIX}${path}${buildQuery(options.query)}`;

  const headers = new Headers({ Accept: "application/json" });
  const isFormData = typeof FormData !== "undefined" && options.body instanceof FormData;
  if (options.body !== undefined && !isFormData) {
    headers.set("Content-Type", "application/json");
  }
  let response: Response;
  try {
    response = await fetch(url, {
      method,
      headers,
      body:
        options.body === undefined
          ? undefined
          : isFormData
            ? (options.body as FormData)
            : JSON.stringify(options.body),
      signal: options.signal,
      // Never let a browser or intermediary cache a user's evidence.
      cache: "no-store",
      // Sends the HttpOnly session cookie; this is the whole authentication.
      credentials: "same-origin",
    });
  } catch (cause) {
    if (cause instanceof DOMException && cause.name === "AbortError") throw cause;
    throw new NeurunTransportError(
      "Could not reach the control plane. Check that the server is running.",
      cause,
    );
  }

  const meta = readMeta(response);
  const body = await decodeBody(response);

  if (!response.ok) {
    const envelope = parseErrorEnvelope(body);
    throw new NeurunApiError({
      status: response.status,
      code: envelope?.error.code ?? `http_${response.status}`,
      message:
        envelope?.error.message ??
        (typeof body === "string" && body ? body : `Request failed with HTTP ${response.status}.`),
      details: envelope?.error.details,
      retryAfter: meta.retryAfter,
      envelope,
    });
  }

  const data = schema ? validateResponse(schema, path, body) : (body as T);
  return { data, meta };
}

/** Convenience wrapper for callers that do not need response metadata. */
export async function requestData<T>(
  options: RequestOptions,
  schema?: z.ZodType<T>,
): Promise<T> {
  const { data } = await request(options, schema);
  return data;
}
