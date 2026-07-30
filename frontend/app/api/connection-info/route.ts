import { NextResponse } from "next/server";

const DEFAULT_BASE_URL = "http://localhost:8080";

function normalize(url: string): string {
  return url.trim().replace(/\/+$/, "");
}

/**
 * Which control planes this deployment's proxy will forward to.
 *
 * This is the operator's own deployment configuration, not a secret, and
 * surfacing it lets the connection screen state what it can actually reach
 * instead of letting the operator discover the allowlist through a 400.
 */
export function GET() {
  const defaultBaseUrl = normalize(process.env.NEURUN_API_BASE_URL ?? DEFAULT_BASE_URL);
  const additional = (process.env.NEURUN_ALLOWED_BASE_URLS ?? "")
    .split(",")
    .map((entry) => normalize(entry))
    .filter(Boolean);

  return NextResponse.json(
    {
      default_base_url: defaultBaseUrl,
      allowed_base_urls: Array.from(new Set([defaultBaseUrl, ...additional])),
    },
    { headers: { "Cache-Control": "no-store" } },
  );
}
