import { beforeEach, describe, expect, it } from "vitest";
import { http, HttpResponse } from "msw";

import { request } from "@/lib/api/client";
import * as api from "@/lib/api/endpoints";
import * as resources from "@/lib/api/resources";
import { NeurunApiError, NeurunContractError } from "@/lib/api/errors";
import { shouldRetry } from "@/lib/api/query-client";
import { idempotencyKeys, stableStringify } from "@/lib/api/idempotency";
import { operatorEnvelopeSchema, validateResponse } from "@/lib/api/runtime";
import { OPERATOR, OPERATOR_PASSWORD, proxy, server } from "./msw/server";
import * as fixtures from "./msw/fixtures";

beforeEach(() => idempotencyKeys.clear());

describe("authentication", () => {
  it("signs in with an email and password", async () => {
    const operator = await api.operatorLogin(OPERATOR.email, OPERATOR_PASSWORD);
    expect(operator.email).toBe("alice@example.com");
    expect(operator.role).toBe("admin");
    expect(operator.scopes).toContain("*");
  });

  it("carries the organization the session acts in", async () => {
    const operator = await api.getOperatorSession();
    expect(operator.organization_id).toBe("org_01HXQ8F2ACME");
  });

  it("reports bad credentials without distinguishing which part was wrong", async () => {
    const error = await api
      .operatorLogin("alice@example.com", "not the password")
      .catch((cause: unknown) => cause);

    expect(error).toBeInstanceOf(NeurunApiError);
    expect((error as NeurunApiError).status).toBe(401);
    expect((error as NeurunApiError).code).toBe("invalid_credentials");
  });

  it("reports when sign-in is unavailable on this control plane", async () => {
    server.use(
      http.post(proxy("/v1/auth/login"), () =>
        HttpResponse.json(fixtures.signInUnavailable, { status: 503 }),
      ),
    );

    const error = await api
      .operatorLogin("alice@example.com", OPERATOR_PASSWORD)
      .catch((cause: unknown) => cause);

    expect(error).toBeInstanceOf(NeurunApiError);
    expect((error as NeurunApiError).status).toBe(503);
  });
});

describe("registration", () => {
  it("starts an organization and lands signed in", async () => {
    server.use(
      http.post(proxy("/v1/auth/register"), () =>
        HttpResponse.json(
          {
            user: { id: "usr_01HXQ8F2NEW", email: "ada@example.com" },
            organization: { id: "org_01HXQ8F2ACME", name: "Acme Data" },
            member: { role: "admin" },
            operator: OPERATOR,
          },
          { status: 201 },
        ),
      ),
    );

    const operator = await api.register({
      email: "ada@example.com",
      password: "a-long-enough-password",
      organization_name: "Acme Data",
    });
    expect(operator?.organization_id).toBe("org_01HXQ8F2ACME");
  });

  it("returns null when the account was made but sign-in did not follow", async () => {
    server.use(
      http.post(proxy("/v1/auth/register"), () =>
        HttpResponse.json(
          {
            user: { id: "usr_01HXQ8F2NEW", email: "ada@example.com" },
            organization: { id: "org_01HXQ8F2NEW", name: "Acme Data" },
            member: { role: "admin" },
          },
          { status: 201 },
        ),
      ),
    );

    await expect(
      api.register({
        email: "ada@example.com",
        password: "a-long-enough-password",
        organization_name: "Acme Data",
      }),
    ).resolves.toBeNull();
  });

  it("refuses an invitation issued to another address", async () => {
    server.use(
      http.post(proxy("/v1/auth/register"), () =>
        HttpResponse.json(
          {
            error: {
              code: "invite_address_mismatch",
              message: "this invitation was issued to a different address",
            },
            request_id: "req_01HXQ8F2INVITE",
          },
          { status: 403 },
        ),
      ),
    );

    const error = await api
      .register({
        email: "wrong@example.com",
        password: "a-long-enough-password",
        invite_token: "a-token",
      })
      .catch((cause: unknown) => cause);

    expect(error).toBeInstanceOf(NeurunApiError);
    expect((error as NeurunApiError).code).toBe("invite_address_mismatch");
  });

  it("names the organization an invitation would join, without spending it", async () => {
    server.use(
      http.get(proxy("/v1/invites/lookup"), ({ request: incoming }) => {
        expect(new URL(incoming.url).searchParams.get("token")).toBe("a-token");
        return HttpResponse.json({
          organization: { id: "org_01HXQ8F2ACME", name: "Acme Data" },
          email: "ada@example.com",
          role: "operator",
        });
      }),
    );

    const preview = await api.lookupInvite("a-token");
    expect(preview.organization.name).toBe("Acme Data");
    expect(preview.role).toBe("operator");
  });
});

describe("error envelope", () => {
  it("surfaces the code and status the server sent", async () => {
    server.use(
      http.get(proxy("/v1/projects"), () =>
        HttpResponse.json(fixtures.unauthorized, { status: 401 }),
      ),
    );

    const error = await resources.listProjects().catch((cause: unknown) => cause);
    expect(error).toBeInstanceOf(NeurunApiError);
    expect((error as NeurunApiError).status).toBe(401);
  });

  it("rejects a response that does not match the contract", () => {
    expect(() =>
      validateResponse(operatorEnvelopeSchema as never, "/v1/auth/session", {
        operator: { operator_id: 12 },
      }),
    ).toThrow(NeurunContractError);
  });
});

describe("retry policy", () => {
  it("retries a transport failure but never a refusal", () => {
    expect(shouldRetry(0, new Error("network down"))).toBe(true);
    expect(shouldRetry(0, new NeurunApiError({ status: 422, code: "invalid_request", message: "refused" }))).toBe(false);
    expect(shouldRetry(0, new NeurunApiError({ status: 401, code: "unauthorized", message: "expired" }))).toBe(false);
  });
});

describe("idempotency keys", () => {
  it("is stable across key order, so a replay is byte-equivalent", () => {
    expect(stableStringify({ b: 1, a: 2 })).toBe(stableStringify({ a: 2, b: 1 }));
  });
});

describe("client", () => {
  it("reaches the proxy rather than the control plane directly", async () => {
    let sawPath: string | undefined;
    server.use(
      http.get(proxy("/version"), ({ request: incoming }) => {
        sawPath = new URL(incoming.url).pathname;
        return HttpResponse.json({
          version: "0.1.0",
          commit: "9f3a41c",
          built_at: "2026-08-04T00:00:00Z",
          api_version: "v1",
          schema_version: "12",
        });
      }),
    );

    await request({ path: "/version" });
    expect(sawPath).toContain("/version");
  });
});
