import { setupServer } from "msw/node";
import { http, HttpResponse } from "msw";
import * as fixtures from "./fixtures";

/**
 * The control-plane origin the client actually calls.
 *
 * Authentication is an `HttpOnly` cookie the browser attaches, so the client
 * sends no credential these handlers can meaningfully inspect. Tests that care
 * about the unauthenticated path override a specific route with a 401 rather
 * than simulating cookie storage, which jsdom does not model for HttpOnly
 * cookies by design. The real cookie round-trip — flags included — is covered by
 * the Go tests in `internal/api/sessionauth_test.go`.
 */
export const apiUrl = (path: string) => `http://localhost:1267${path}`;

export const SESSION_PASSWORD = "correct horse battery staple";

export const SESSION = {
  user_id: "opr_01HXQ8F2ALICE",
  email: "alice@example.com",
  organization_id: "org_01HXQ8F2ACME",
  role: "admin",
  scopes: ["*"],
  expires_at: "2026-07-30T23:00:00Z",
};

/**
 * Default handlers describe a healthy control plane with a user signed in
 * and volatile jobs enabled. Individual tests override with `server.use(...)` to
 * describe the case they are actually about.
 */
export const handlers = [
  /* ---------------------------------- auth ---------------------------------- */

  http.post(apiUrl("/v1/auth/login"), async ({ request }) => {
    const body = (await request.json()) as { email?: string; password?: string };
    if (body.email !== SESSION.email || body.password !== SESSION_PASSWORD) {
      return HttpResponse.json(fixtures.invalidCredentials, { status: 401 });
    }
    return HttpResponse.json(
      { session: SESSION },
      {
        status: 200,
        headers: {
          "Set-Cookie":
            "neurun_session=opaque-token; Path=/; HttpOnly; Secure; SameSite=Strict",
        },
      },
    );
  }),

  http.post(apiUrl("/v1/auth/logout"), () => new HttpResponse(null, { status: 204 })),

  http.get(apiUrl("/v1/auth/session"), () => HttpResponse.json({ session: SESSION })),

  /* --------------------------------- health --------------------------------- */

  http.get(apiUrl("/version"), () =>
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

  /* -------------------------------- resources ------------------------------- */

  http.get(apiUrl("/v1/projects"), () =>
    HttpResponse.json({ projects: [fixtures.project] }),
  ),

  http.get(apiUrl("/v1/apps"), () => HttpResponse.json({ apps: [fixtures.app] })),

  http.get(apiUrl("/v1/deployments"), () =>
    HttpResponse.json({ deployments: [fixtures.readyDeployment] }),
  ),

  http.get(apiUrl("/v1/builds"), () =>
    HttpResponse.json({ builds: [fixtures.readyBuild] }),
  ),

  http.get(apiUrl("/v1/executions"), () =>
    HttpResponse.json({
      executions: [fixtures.succeededExecution, fixtures.unknownStateExecution],
    }),
  ),
];

export const server = setupServer(...handlers);
