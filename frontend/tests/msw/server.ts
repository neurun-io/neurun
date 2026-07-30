import { setupServer } from "msw/node";
import { http, HttpResponse } from "msw";
import * as fixtures from "./fixtures";

/**
 * The proxy path the client actually calls on this origin.
 *
 * Authentication is an `HttpOnly` cookie the browser attaches, so the client
 * sends no credential these handlers can meaningfully inspect. Tests that care
 * about the unauthenticated path override a specific route with a 401 rather
 * than simulating cookie storage, which jsdom does not model for HttpOnly
 * cookies by design. The real cookie round-trip — flags included — is covered by
 * the Go tests in `internal/api/operatorauth_test.go`.
 */
export const proxy = (path: string) => `http://localhost:3000/api/proxy${path}`;

export const OPERATOR_PASSWORD = "correct horse battery staple";

export const OPERATOR = {
  operator_id: "opr_01HXQ8F2ALICE",
  username: "alice",
  role: "admin",
  project_id: "prj_local",
  scopes: ["*"],
  session_id: "oses_01HXQ8F2",
  expires_at: "2026-07-30T23:00:00Z",
};

/**
 * Default handlers describe a healthy control plane with an operator signed in
 * and volatile jobs enabled. Individual tests override with `server.use(...)` to
 * describe the case they are actually about.
 */
export const handlers = [
  /* ---------------------------------- auth ---------------------------------- */

  http.post(proxy("/v1/auth/login"), async ({ request }) => {
    const body = (await request.json()) as { username?: string; password?: string };
    if (body.username !== OPERATOR.username || body.password !== OPERATOR_PASSWORD) {
      return HttpResponse.json(fixtures.invalidCredentials, { status: 401 });
    }
    return HttpResponse.json(
      { operator: OPERATOR, request_id: "req_01HXQ8F2LOGIN" },
      {
        status: 200,
        headers: {
          "Set-Cookie":
            "neurun_operator_session=opaque-token; Path=/; HttpOnly; Secure; SameSite=Strict",
        },
      },
    );
  }),

  http.post(proxy("/v1/auth/logout"), () => new HttpResponse(null, { status: 204 })),

  http.get(proxy("/v1/auth/session"), () => HttpResponse.json({ operator: OPERATOR })),

  /* --------------------------------- health --------------------------------- */

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

  /* -------------------------------- functions ------------------------------- */

  http.get(proxy("/v1/functions"), () =>
    HttpResponse.json({ functions: [fixtures.echoManifest, fixtures.browserManifest] }),
  ),

  http.get(proxy("/v1/functions/:name/versions/:version"), () =>
    HttpResponse.json(fixtures.echoManifest),
  ),

  http.post(proxy("/v1/functions/:name/invoke"), async ({ request }) => {
    const body = (await request.json()) as { execution?: string };
    if (body.execution === "async") {
      return HttpResponse.json(fixtures.acceptedJob, {
        status: 202,
        headers: { "Neurun-Job-Durability": "process_local" },
      });
    }
    return HttpResponse.json(fixtures.succeededInvocation);
  }),

  /* ----------------------------------- jobs --------------------------------- */

  http.get(proxy("/v1/jobs"), ({ request }) => {
    const cursor = new URL(request.url).searchParams.get("cursor");
    // Two pages; the empty string on the last one means "no more".
    if (cursor === "cursor-page-2") {
      return HttpResponse.json({ jobs: [fixtures.succeededJob], next_cursor: "" });
    }
    return HttpResponse.json({
      jobs: [fixtures.queuedJob, fixtures.unknownStateJob],
      next_cursor: "cursor-page-2",
    });
  }),

  http.post(proxy("/v1/jobs"), () =>
    HttpResponse.json(fixtures.acceptedJob, {
      status: 202,
      headers: {
        "Neurun-Job-Durability": "process_local",
        "Request-ID": "req_01HXQ8F2ACCEPT",
        Location: `/v1/jobs/${fixtures.JOB_ID}`,
      },
    }),
  ),

  http.get(proxy("/v1/jobs/:jobId"), () => HttpResponse.json(fixtures.queuedJob)),

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

  /* ------------------------------- invocations ------------------------------ */

  http.get(proxy("/v1/function-invocations"), () =>
    HttpResponse.json({
      invocations: [fixtures.succeededInvocation, fixtures.schemaRejectedInvocation],
      next_cursor: "",
    }),
  ),

  http.get(proxy("/v1/function-invocations/:id"), () =>
    HttpResponse.json(fixtures.succeededInvocation),
  ),

  /* ---------------------------------- fetch --------------------------------- */

  http.post(proxy("/v1/fetch"), () => HttpResponse.json(fixtures.succeededInvocation)),
];

export const server = setupServer(...handlers);
