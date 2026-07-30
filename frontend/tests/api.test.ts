import { beforeEach, describe, expect, it } from "vitest";
import { http, HttpResponse } from "msw";

import { request } from "@/lib/api/client";
import * as api from "@/lib/api/endpoints";
import { NeurunApiError, NeurunContractError } from "@/lib/api/errors";
import { shouldRetry } from "@/lib/api/query-client";
import { idempotencyKeys, stableStringify } from "@/lib/api/idempotency";
import { jobListSchema, jobSchema, validateResponse } from "@/lib/api/runtime";
import { OPERATOR, OPERATOR_PASSWORD, proxy, server } from "./msw/server";
import * as fixtures from "./msw/fixtures";

beforeEach(() => idempotencyKeys.clear());

describe("authentication", () => {
  it("signs in with a username and password", async () => {
    const operator = await api.operatorLogin(OPERATOR.username, OPERATOR_PASSWORD);
    expect(operator.username).toBe("alice");
    expect(operator.role).toBe("admin");
    expect(operator.scopes).toContain("*");
  });

  it("reports bad credentials without distinguishing which part was wrong", async () => {
    const error = await api
      .operatorLogin("alice", "not the password")
      .catch((cause) => cause);

    expect(error).toBeInstanceOf(NeurunApiError);
    expect(error.status).toBe(401);
    expect(error.code).toBe("invalid_credentials");
    expect(error.requestId).toBe("req_01HXQ8F2BADLOGIN");
  });

  it("reports when the server has no operator accounts configured", async () => {
    server.use(
      http.post(proxy("/v1/auth/login"), () =>
        HttpResponse.json(fixtures.signInUnavailable, { status: 503 }),
      ),
    );

    const error = await api.operatorLogin("alice", OPERATOR_PASSWORD).catch((cause) => cause);
    expect(error.status).toBe(503);
    expect(error.code).toBe("operator_signin_unavailable");
  });

  it("authenticates with the session cookie and assembles no credential itself", async () => {
    let authorization: string | null = "unset";
    let cookie: string | null = "unset";
    let credentials: RequestCredentials | undefined;

    server.use(
      http.get(proxy("/v1/functions"), ({ request: intercepted }) => {
        authorization = intercepted.headers.get("authorization");
        cookie = intercepted.headers.get("cookie");
        credentials = intercepted.credentials;
        return HttpResponse.json({ functions: [] });
      }),
    );

    // Sign in here rather than relying on a previous test, so the round-trip
    // being asserted is this test's own.
    await api.operatorLogin(OPERATOR.username, OPERATOR_PASSWORD);
    await api.listFunctions();

    // No bearer token is assembled anywhere in the client — there is no API key
    // in any client module to assemble one from.
    expect(authorization).toBeNull();
    // The request opts into cookie credentials rather than carrying a header.
    expect(credentials).toBe("same-origin");
    // And the session issued by login is what actually authenticates the call.
    expect(cookie).toContain("neurun_operator_session=");
  });

  it("never puts a secret in the request URL", async () => {
    let seenUrl = "";
    server.use(
      http.get(proxy("/v1/functions"), ({ request: intercepted }) => {
        seenUrl = intercepted.url;
        return HttpResponse.json({ functions: [] });
      }),
    );

    await api.listFunctions();
    expect(seenUrl).not.toContain("password");
    expect(seenUrl).not.toContain("neu_");
    expect(seenUrl).not.toContain("token");
  });

  it("resolves the signed-in operator", async () => {
    const operator = await api.getOperatorSession();
    expect(operator.operator_id).toBe(OPERATOR.operator_id);
    expect(operator.project_id).toBe("prj_local");
  });

  it("surfaces an expired session as a 401", async () => {
    server.use(
      http.get(proxy("/v1/auth/session"), () =>
        HttpResponse.json(fixtures.unauthorized, { status: 401 }),
      ),
    );

    const error = await api.getOperatorSession().catch((cause) => cause);
    expect(error).toBeInstanceOf(NeurunApiError);
    expect(error.isUnauthenticated).toBe(true);
  });

  it("signs out without needing the session to be live", async () => {
    await expect(api.operatorLogout()).resolves.toBeUndefined();
  });

  it("stops retrying after a 401 — an expired session will not revive", () => {
    const unauthorized = new NeurunApiError({ status: 401, code: "unauthorized", message: "no" });
    const serverError = new NeurunApiError({ status: 500, code: "internal", message: "boom" });

    expect(shouldRetry(0, unauthorized)).toBe(false);
    expect(shouldRetry(0, new NeurunApiError({ status: 403, code: "f", message: "f" }))).toBe(false);
    expect(shouldRetry(0, new NeurunContractError("/v1/jobs", []))).toBe(false);
    expect(shouldRetry(0, serverError)).toBe(true);
  });
});

describe("error envelope", () => {
  it("parses code, message and request ID from the standard envelope", async () => {
    server.use(
      http.post(proxy("/v1/functions/:name/invoke"), () =>
        HttpResponse.json(fixtures.invalidRequest, { status: 400 }),
      ),
    );

    const error = await api
      .invokeFunction("system.echo", { version: "1", execution: "sync", input: {} })
      .catch((cause) => cause);

    expect(error).toBeInstanceOf(NeurunApiError);
    expect(error.code).toBe("invalid_request");
    // Human-readable `$.path` messages are preserved verbatim, not parsed.
    expect(error.message).toBe("$.input.message: must be a string");
    expect(error.requestId).toBe("req_01HXQ8F2INVALID");
  });

  it("recognises durable_backend_unavailable without conflating it with other 503s", async () => {
    server.use(
      http.post(proxy("/v1/jobs"), () =>
        HttpResponse.json(fixtures.durableBackendUnavailable, { status: 503 }),
      ),
    );

    const error = await api
      .createJob({ function: { name: "system.echo", version: "1" }, input: {} })
      .catch((cause) => cause);

    expect(error.isDurableBackendUnavailable).toBe(true);
    expect(error.status).toBe(503);
  });
});

describe("accepted asynchronous work", () => {
  it("reads durability from both the body and the header", async () => {
    const { data, meta } = await api.createJob({
      function: { name: "system.echo", version: "1" },
      input: { message: "hello" },
    });

    expect(meta.status).toBe(202);
    expect(data.durability).toBe("process_local");
    expect(meta.durability).toBe("process_local");
    // A 202 alone never implies durable persistence.
    expect(data.durability).not.toBe("durable");
  });

  it("returns a queued snapshot whose events still record the accepted step", async () => {
    const { data } = await api.createJob({
      function: { name: "system.echo", version: "1" },
      input: { message: "hello" },
    });
    const { data: events } = await api.listJobEvents(data.job_id);

    expect(data.job.state).toBe("queued");
    expect(events.map((event) => event.type)).toEqual([
      "job.accepted",
      "job.queued",
      "attempt.leased",
    ]);
    expect(events.map((event) => event.sequence)).toEqual([1, 2, 3]);
  });
});

describe("idempotency keys", () => {
  it("sends a key on async mutations and omits it on cancellation", async () => {
    const seen: (string | null)[] = [];
    server.use(
      http.post(proxy("/v1/jobs"), ({ request: intercepted }) => {
        seen.push(intercepted.headers.get("idempotency-key"));
        return HttpResponse.json(fixtures.acceptedJob, { status: 202 });
      }),
      http.post(proxy("/v1/jobs/:jobId/cancel"), ({ request: intercepted }) => {
        seen.push(intercepted.headers.get("idempotency-key"));
        return HttpResponse.json({
          job: fixtures.queuedJob,
          duplicate: false,
          request_id: "req_x",
        });
      }),
    );

    await api.createJob({ function: { name: "system.echo", version: "1" }, input: {} });
    await api.cancelJob(fixtures.JOB_ID);

    expect(seen[0]).toBeTruthy();
    expect(seen[1]).toBeNull();
  });

  it("reuses the key when the same logical request is retried after a network failure", async () => {
    const keys: (string | null)[] = [];
    let attempt = 0;

    server.use(
      http.post(proxy("/v1/jobs"), ({ request: intercepted }) => {
        keys.push(intercepted.headers.get("idempotency-key"));
        attempt += 1;
        if (attempt === 1) return HttpResponse.error();
        return HttpResponse.json(fixtures.acceptedJob, { status: 202 });
      }),
    );

    const body = {
      function: { name: "system.echo", version: "1" },
      input: { message: "hello" },
    };

    await api.createJob(body).catch(() => undefined);
    await api.createJob(body);

    expect(keys).toHaveLength(2);
    // The whole point: the server may already hold the first request.
    expect(keys[0]).toBe(keys[1]);
  });

  it("mints a fresh key once a request reached a decisive outcome", async () => {
    const keys: (string | null)[] = [];
    server.use(
      http.post(proxy("/v1/jobs"), ({ request: intercepted }) => {
        keys.push(intercepted.headers.get("idempotency-key"));
        return HttpResponse.json(fixtures.acceptedJob, { status: 202 });
      }),
    );

    const body = {
      function: { name: "system.echo", version: "1" },
      input: { message: "hello" },
    };
    await api.createJob(body);
    await api.createJob(body);

    expect(keys[0]).not.toBe(keys[1]);
  });

  it("fingerprints logically identical bodies the same regardless of key order", () => {
    expect(stableStringify({ b: 1, a: { d: 2, c: 3 } })).toBe(
      stableStringify({ a: { c: 3, d: 2 }, b: 1 }),
    );
  });
});

describe("cursor pagination", () => {
  it("follows the server's opaque cursor and stops on the empty string", async () => {
    const first = await api.listJobs({ limit: 50 });
    expect(first.data.jobs).toHaveLength(2);
    expect(first.data.next_cursor).toBe("cursor-page-2");

    const second = await api.listJobs({ cursor: first.data.next_cursor });
    expect(second.data.jobs).toHaveLength(1);
    // Empty string means there is no next page.
    expect(second.data.next_cursor).toBe("");
  });

  it("repeats the status filter rather than comma-joining it", async () => {
    let seen = "";
    server.use(
      http.get(proxy("/v1/jobs"), ({ request: intercepted }) => {
        seen = new URL(intercepted.url).search;
        return HttpResponse.json({ jobs: [], next_cursor: "" });
      }),
    );

    await api.listJobs({ status: ["queued", "running"] });
    expect(seen).toContain("status=queued");
    expect(seen).toContain("status=running");
    expect(seen).not.toContain("queued%2Crunning");
  });
});

describe("runtime boundary validation", () => {
  it("accepts a state this build does not know", () => {
    const parsed = validateResponse(jobSchema, "/v1/jobs", {
      ...fixtures.queuedJob,
      state: "quarantined",
    });
    expect(parsed.state).toBe("quarantined");
  });

  it("preserves unknown keys instead of dropping them", () => {
    const parsed = validateResponse(jobSchema, "/v1/jobs", {
      ...fixtures.queuedJob,
      future_field: { nested: true },
    }) as Record<string, unknown>;

    expect(parsed.future_field).toEqual({ nested: true });
  });

  it("rejects a payload missing a field the UI reads", () => {
    const withoutId: Record<string, unknown> = { ...fixtures.queuedJob };
    delete withoutId.id;

    expect(() => validateResponse(jobSchema, "/v1/jobs", withoutId)).toThrow(NeurunContractError);
  });

  it("names the offending path when the contract is violated", () => {
    const error = (() => {
      try {
        validateResponse(jobListSchema, "/v1/jobs", { jobs: "not-an-array", next_cursor: "" });
      } catch (cause) {
        return cause as NeurunContractError;
      }
    })();

    expect(error).toBeInstanceOf(NeurunContractError);
    expect(error!.issues.join(" ")).toContain("jobs");
  });

  it("accepts an operator role this build does not know", async () => {
    server.use(
      http.get(proxy("/v1/auth/session"), () =>
        HttpResponse.json({ operator: { ...OPERATOR, role: "auditor", scopes: ["jobs:read"] } }),
      ),
    );

    const operator = await api.getOperatorSession();
    expect(operator.role).toBe("auditor");
  });
});

describe("transport", () => {
  it("reports an unreachable control plane distinctly from an API error", async () => {
    server.use(http.get(proxy("/version"), () => HttpResponse.error()));

    const error = await request({ path: "/version" }).catch((cause) => cause);

    expect(error).not.toBeInstanceOf(NeurunApiError);
    expect(error.name).toBe("NeurunTransportError");
  });
});
