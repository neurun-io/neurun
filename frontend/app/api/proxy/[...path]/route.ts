/**
 * Same-origin reverse proxy to the control plane.
 *
 * The control plane ships no CORS or `OPTIONS` middleware and serves no frontend
 * assets, so a browser cannot call `/v1` cross-origin. Everything the dashboard
 * does goes through here.
 *
 * The upstream target comes only from server configuration. The browser cannot
 * choose it, which is what stops this handler from being an open relay for
 * arbitrary outbound requests.
 *
 * Authentication is the user's `HttpOnly` session cookie, forwarded upstream
 * and back. This handler injects no credential of its own: it holds no API key,
 * so a request without a valid session simply gets the control plane's 401.
 */
import { NextResponse, type NextRequest } from "next/server";

const DEFAULT_BASE_URL = "http://localhost:1267";

/** Request headers worth forwarding upstream. Everything else is dropped. */
const FORWARDED_REQUEST_HEADERS = [
  // The session cookie. This is the whole authentication story.
  "cookie",
  // Still accepted so a script or curl session can use an API key against the
  // same proxy; the browser dashboard never sets it.
  "authorization",
  "content-type",
  "accept",
  "idempotency-key",
  "last-event-id",
];

/** Response headers the dashboard is allowed to read back. */
const EXPOSED_RESPONSE_HEADERS = [
  "content-type",
  "retry-after",
  "location",
  "www-authenticate",
];

function normalize(url: string): string {
  return url.trim().replace(/\/+$/, "");
}

function baseUrl(): string {
  return normalize(process.env.NEURUN_API_BASE_URL ?? DEFAULT_BASE_URL);
}

function errorResponse(status: number, code: string, message: string) {
  return NextResponse.json({ error: { code, message } }, { status });
}

async function forward(request: NextRequest, segments: string[]): Promise<Response> {
  const target = baseUrl();

  let parsed: URL;
  try {
    parsed = new URL(target);
  } catch {
    return errorResponse(
      500,
      "base_url_invalid",
      "NEURUN_API_BASE_URL is not a valid URL.",
    );
  }
  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
    return errorResponse(500, "base_url_invalid", "NEURUN_API_BASE_URL must be http or https.");
  }

  const search = request.nextUrl.search;
  const upstreamUrl = `${target}/${segments.map(encodeURIComponent).join("/")}${search}`;

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
      `Could not reach the control plane at ${target}.`,
    );
  }

  const responseHeaders = new Headers();
  for (const name of EXPOSED_RESPONSE_HEADERS) {
    const value = upstream.headers.get(name);
    if (value) responseHeaders.set(name, value);
  }
  // Set-Cookie must pass through verbatim and may appear more than once, so it
  // is appended rather than set — this is how login and logout reach the browser.
  for (const cookie of upstream.headers.getSetCookie()) {
    responseHeaders.append("set-cookie", cookie);
  }
  responseHeaders.set("Cache-Control", "no-store");

  return new Response(upstream.body, {
    status: upstream.status,
    statusText: upstream.statusText,
    headers: responseHeaders,
  });
}

type Context = {
  params: Promise<{ path: string[] }>;
};

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
