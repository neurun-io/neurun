import { setupServer } from "msw/node";
import { http, HttpResponse } from "msw";
import * as fixtures from "./fixtures";

export const BASE_URL = "http://localhost:8080";
export const API_KEY = "neu_local_abc123.supersecretvalue";

/** The proxy path the client actually calls on this origin. */
export const proxy = (path: string) => `http://localhost:3000/api/proxy${path}`;

function requireBearer(request: Request) {
  const header = request.headers.get("authorization");
  if (header !== `Bearer ${API_KEY}`) {
    return HttpResponse.json(fixtures.unauthorized, {
      status: 401,
      headers: { "Request-ID": "req_01HXQ8F2UNAUTH" },
    });
  }
  return null;
}

/**
 * Default handlers describe a healthy control plane with volatile jobs enabled.
 * Individual tests override with `server.use(...)` to describe the case they
 * are actually about.
 */
export const handlers = [
  http.get(proxy("/version"), () =>
    HttpResponse.json({
      version: "0.1.0",
      commit: "abc1234",
      built_at: "2026-07-29T09:00:00Z",
      api_version: "0.1.0",
      schema_version: "1",
      function_bundle: "1.4.2",
    }),
  ),

  http.get(proxy("/v1/functions"), ({ request }) => {
    const unauthenticated = requireBearer(request);
    if (unauthenticated) return unauthenticated;
    return HttpResponse.json({ functions: [fixtures.echoManifest, fixtures.browserManifest] });
  }),

  http.get(proxy("/v1/functions/:name/versions/:version"), ({ request }) => {
    const unauthenticated = requireBearer(request);
    if (unauthenticated) return unauthenticated;
    return HttpResponse.json(fixtures.echoManifest);
  }),

  http.get(proxy("/v1/jobs"), ({ request }) => {
    const unauthenticated = requireBearer(request);
    if (unauthenticated) return unauthenticated;

    const url = new URL(request.url);
    const cursor = url.searchParams.get("cursor");

    // Two pages; the empty string on the last one means "no more".
    if (cursor === "cursor-page-2") {
      return HttpResponse.json({ jobs: [fixtures.succeededJob], next_cursor: "" });
    }
    return HttpResponse.json({
      jobs: [fixtures.queuedJob, fixtures.unknownStateJob],
      next_cursor: "cursor-page-2",
    });
  }),

  http.post(proxy("/v1/jobs"), async ({ request }) => {
    const unauthenticated = requireBearer(request);
    if (unauthenticated) return unauthenticated;

    return HttpResponse.json(fixtures.acceptedJob, {
      status: 202,
      headers: {
        "Neurun-Job-Durability": "process_local",
        "Request-ID": "req_01HXQ8F2ACCEPT",
        Location: `/v1/jobs/${fixtures.JOB_ID}`,
      },
    });
  }),

  http.get(proxy("/v1/jobs/:jobId"), ({ request }) => {
    const unauthenticated = requireBearer(request);
    if (unauthenticated) return unauthenticated;
    return HttpResponse.json(fixtures.queuedJob);
  }),

  http.get(proxy("/v1/jobs/:jobId/events"), () =>
    HttpResponse.json({ events: fixtures.jobEvents }),
  ),

  http.get(proxy("/v1/jobs/:jobId/attempts"), () =>
    HttpResponse.json({ attempts: fixtures.jobAttempts }),
  ),

  http.post(proxy("/v1/jobs/:jobId/cancel"), () =>
    HttpResponse.json({
      job: { ...fixtures.queuedJob, state: "canceled", canceled_at: "2026-07-29T10:00:05Z" },
      duplicate: false,
      request_id: "req_01HXQ8F2CANCEL",
    }),
  ),

  http.get(proxy("/v1/function-invocations"), ({ request }) => {
    const unauthenticated = requireBearer(request);
    if (unauthenticated) return unauthenticated;
    return HttpResponse.json({
      invocations: [fixtures.succeededInvocation, fixtures.schemaRejectedInvocation],
      next_cursor: "",
    });
  }),

  http.get(proxy("/v1/function-invocations/:id"), () =>
    HttpResponse.json(fixtures.succeededInvocation),
  ),

  http.post(proxy("/v1/functions/:name/invoke"), async ({ request }) => {
    const unauthenticated = requireBearer(request);
    if (unauthenticated) return unauthenticated;

    const body = (await request.json()) as { execution?: string };
    if (body.execution === "async") {
      return HttpResponse.json(fixtures.acceptedJob, {
        status: 202,
        headers: { "Neurun-Job-Durability": "process_local" },
      });
    }
    return HttpResponse.json(fixtures.succeededInvocation);
  }),

  http.post(proxy("/v1/fetch"), async ({ request }) => {
    const unauthenticated = requireBearer(request);
    if (unauthenticated) return unauthenticated;
    return HttpResponse.json(fixtures.succeededInvocation);
  }),
];

export const server = setupServer(...handlers);
