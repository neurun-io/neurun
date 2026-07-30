/**
 * Same-origin backend-for-frontend.
 *
 * The control plane ships no CORS or `OPTIONS` middleware and serves no
 * frontend assets, so a browser dashboard cannot call `/v1` cross-origin. This
 * route handler is the same-origin reverse proxy the spec requires: the browser
 * talks only to this app's own origin, and this handler forwards to the
 * configured control plane.
 *
 * What this is NOT: it is not the production auth boundary. The API key still
 * originates in the browser and rides the `Authorization` header through here.
 * Before a production browser dashboard ships, the backend must add either an
 * API-key exchange endpoint that issues a short-lived `HttpOnly`, `Secure`,
 * `SameSite=Strict` operator session, or a BFF that holds the key server-side.
 * That is a release blocker, not a frontend workaround.
 */
import { NextResponse, type NextRequest } from "next/server";

const BASE_URL_HEADER = "x-neurun-base-url";
const DEFAULT_BASE_URL = "http://localhost:8080";

/** Request headers worth forwarding upstream. Everything else is dropped. */
const FORWARDED_REQUEST_HEADERS = [
  "authorization",
  "content-type",
  "accept",
  "idempotency-key",
  "last-event-id",
];

/** Response headers the dashboard is allowed to read back. */
const EXPOSED_RESPONSE_HEADERS = [
  "content-type",
  "request-id",
  "trace-id",
  "retry-after",
  "location",
  "idempotent-replayed",
  "neurun-job-durability",
  "ratelimit-limit",
  "ratelimit-remaining",
  "ratelimit-reset",
  "www-authenticate",
];

function normalize(url: string): string {
  return url.trim().replace(/\/+$/, "");
}

/**
 * The set of control planes this deployment may forward to.
 *
 * Without an allowlist the handler would be an open proxy: any visitor could
 * make the server issue arbitrary outbound requests. `NEURUN_API_BASE_URL` is
 * the deployment's own control plane; `NEURUN_ALLOWED_BASE_URLS` adds others
 * for operators who point one dashboard at several environments.
 */
function allowedBaseUrls(): string[] {
  const configured = [
    process.env.NEURUN_API_BASE_URL ?? DEFAULT_BASE_URL,
    ...(process.env.NEURUN_ALLOWED_BASE_URLS ?? "")
      .split(",")
      .map((entry) => entry.trim())
      .filter(Boolean),
  ];
  return Array.from(new Set(configured.map(normalize)));
}

function errorResponse(status: number, code: string, message: string) {
  return NextResponse.json({ error: { code, message } }, { status });
}

function resolveTarget(request: NextRequest): { baseUrl: string } | { error: NextResponse } {
  const requested = request.headers.get(BASE_URL_HEADER);
  const allowed = allowedBaseUrls();

  if (!requested) return { baseUrl: allowed[0] };

  const candidate = normalize(requested);
  if (!allowed.includes(candidate)) {
    return {
      error: errorResponse(
        400,
        "base_url_not_allowed",
        `This dashboard is not configured to reach ${candidate}. Set NEURUN_API_BASE_URL, or add the origin to NEURUN_ALLOWED_BASE_URLS.`,
      ),
    };
  }

  let parsed: URL;
  try {
    parsed = new URL(candidate);
  } catch {
    return { error: errorResponse(400, "base_url_invalid", "Control-plane base URL is not a valid URL.") };
  }
  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
    return { error: errorResponse(400, "base_url_invalid", "Control-plane base URL must be http or https.") };
  }

  return { baseUrl: candidate };
}

async function forward(request: NextRequest, segments: string[]): Promise<Response> {
  const target = resolveTarget(request);
  if ("error" in target) return target.error;

  const search = request.nextUrl.search;
  const upstreamUrl = `${target.baseUrl}/${segments.map(encodeURIComponent).join("/")}${search}`;

  const headers = new Headers();
  for (const name of FORWARDED_REQUEST_HEADERS) {
    const value = request.headers.get(name);
    if (value) headers.set(name, value);
  }

  const method = request.method;
  const body = method === "GET" || method === "HEAD" ? undefined : await request.arrayBuffer();

  let upstream: Response;
  try {
    upstream = await fetch(upstreamUrl, {
      method,
      headers,
      body,
      cache: "no-store",
      redirect: "manual",
    });
  } catch {
    return errorResponse(
      502,
      "control_plane_unreachable",
      `Could not reach the control plane at ${target.baseUrl}.`,
    );
  }

  const responseHeaders = new Headers();
  for (const name of EXPOSED_RESPONSE_HEADERS) {
    const value = upstream.headers.get(name);
    if (value) responseHeaders.set(name, value);
  }
  responseHeaders.set("Cache-Control", "no-store");

  return new Response(upstream.body, {
    status: upstream.status,
    statusText: upstream.statusText,
    headers: responseHeaders,
  });
}

type Context = RouteContext<"/api/proxy/[...path]">;

export async function GET(request: NextRequest, context: Context) {
  const { path } = await context.params;
  return forward(request, path);
}

export async function POST(request: NextRequest, context: Context) {
  const { path } = await context.params;
  return forward(request, path);
}

export async function PATCH(request: NextRequest, context: Context) {
  const { path } = await context.params;
  return forward(request, path);
}

export async function DELETE(request: NextRequest, context: Context) {
  const { path } = await context.params;
  return forward(request, path);
}
