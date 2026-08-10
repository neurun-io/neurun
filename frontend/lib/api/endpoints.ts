/**
 * Typed wrappers over the published /v1 operations.
 *
 * One function per contract operation, named for its operationId. Nothing here
 * infers internal server state, and nothing here reaches an endpoint the
 * current OpenAPI does not publish. Resource operations live in ./resources.
 */
import { request } from "./client";
import {
  invitePreviewSchema,
  memberSchema,
  sessionEnvelopeSchema,
  organizationSchema,
  registrationSchema,
  versionSchema,
} from "./runtime";
import type { Session, Version } from "./types";

export interface InvitePreview {
  organization: { id: string; name: string };
  email: string;
  role: string;
}

/* -------------------------------------------------------------------------- */
/* Auth                                                                        */
/* -------------------------------------------------------------------------- */

export interface RegisterRequest {
  email: string;
  password: string;
  /** Start an organization you own. Mutually exclusive with invite_token. */
  organization_name?: string;
  /** Join one you were invited to. Mutually exclusive with organization_name. */
  invite_token?: string;
}

/**
 * Create an account and a session for it.
 *
 * Sign-up is the only way an account comes into being — there is no CLI
 * bootstrap. The session arrives as the same `HttpOnly` cookie sign-in issues,
 * so a successful registration lands already authenticated. `session` is
 * absent only when the account was created but signing it in failed, in which
 * case the caller signs in normally rather than being told nothing happened.
 */
export async function register(body: RegisterRequest) {
  const { data } = await request<{ session?: Session }>(
    { method: "POST", path: "/v1/auth/register", body },
    registrationSchema as never,
  );
  return data.session ?? null;
}

/**
 * Exchange an email and password for a session.
 *
 * The token is never visible here: the server returns it as an `HttpOnly`
 * cookie, and this response carries only the user projection.
 */
export async function login(email: string, password: string) {
  const { data } = await request<{ session: Session }>(
    { method: "POST", path: "/v1/auth/login", body: { email, password } },
    sessionEnvelopeSchema as never,
  );
  return data.session;
}

/** Name the organization an invitation would join, without spending it. */
export async function lookupInvite(token: string, signal?: AbortSignal) {
  const { data } = await request<InvitePreview>(
    { path: "/v1/invites/lookup", query: { token }, signal },
    invitePreviewSchema as never,
  );
  return data;
}

/** Revoke the current session. Idempotent, and always succeeds. */
export async function logout(): Promise<void> {
  await request({ method: "POST", path: "/v1/auth/logout" });
}

/** The signed-in session, or a 401 when there is no live session. */
export async function getSession(signal?: AbortSignal) {
  const { data } = await request<{ session: Session }>(
    { path: "/v1/auth/session", signal },
    sessionEnvelopeSchema as never,
  );
  return data.session;
}

/* -------------------------------------------------------------------------- */
/* Health                                                                      */
/* -------------------------------------------------------------------------- */

export function getVersion(signal?: AbortSignal) {
  return request<Version>({ path: "/version", signal }, versionSchema as never);
}

export async function getReadiness(signal?: AbortSignal): Promise<boolean> {
  try {
    await request({ path: "/readyz", signal });
    return true;
  } catch {
    return false;
  }
}

/* -------------------------------------------------------------------------- */
/* Organization                                                                */
/* -------------------------------------------------------------------------- */

/**
 * Start an organization from an account that has none. The server re-issues the
 * session cookie, so the new membership is live without signing in again.
 */
export async function createOrganization(name: string) {
  const { data } = await request<{ id: string; name: string }>(
    { method: "POST", path: "/v1/organizations", body: { name } },
    organizationSchema as never,
  );
  return data;
}

export interface OrganizationSummary {
  id: string;
  name: string;
  owner_user_id?: string;
}

/** Every organization the account belongs to, owned or joined. */
export async function listOrganizations(signal?: AbortSignal) {
  const { data } = await request<{ organizations: OrganizationSummary[] }>({
    path: "/v1/organizations",
    signal,
  });
  return data.organizations;
}

/** Accept an invitation as a signed-in account. */
export async function acceptInvite(token: string) {
  const { data } = await request<{ role: string }>(
    { method: "POST", path: "/v1/invites/accept", body: { token } },
    memberSchema as never,
  );
  return data;
}
