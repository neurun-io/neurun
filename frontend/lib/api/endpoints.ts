/**
 * Typed wrappers over the published `/v1` operations.
 *
 * One function per contract operation, named for its `operationId`. Nothing
 * here infers internal server state, and nothing here reaches an endpoint the
 * current OpenAPI does not publish.
 */
import { type ApiResult, request } from "./client";
import {
  acceptedJobSchema,
  cancelInvocationResponseSchema,
  cancelJobResponseSchema,
  functionDefinitionSchema,
  functionListSchema,
  functionManifestSchema,
  invocationListSchema,
  invocationSchema,
  jobAttemptListSchema,
  jobEventListSchema,
  jobListSchema,
  jobSchema,
  manifestBundleSchema,
  operatorEnvelopeSchema,
  versionSchema,
} from "./runtime";
import type {
  AcceptedJob,
  CreateJobRequest,
  FetchRequest,
  FunctionDefinition,
  FunctionList,
  FunctionManifest,
  Invocation,
  InvocationList,
  InvokeFunctionRequest,
  Job,
  JobAttempt,
  JobEvent,
  JobList,
  JobState,
  ManifestBundle,
  Operator,
  Version,
} from "./types";

/* -------------------------------------------------------------------------- */
/* Auth                                                                        */
/* -------------------------------------------------------------------------- */

/**
 * Exchange a username and password for a session.
 *
 * The token is never visible here: the server returns it as an `HttpOnly`
 * cookie, and this response carries only the operator projection.
 */
export async function operatorLogin(username: string, password: string) {
  const { data } = await request<{ operator: Operator }>(
    { method: "POST", path: "/v1/auth/login", body: { username, password } },
    operatorEnvelopeSchema as never,
  );
  return data.operator;
}

/** Revoke the current session. Idempotent, and always succeeds. */
export async function operatorLogout(): Promise<void> {
  await request({ method: "POST", path: "/v1/auth/logout" });
}

/** The signed-in operator, or a 401 when there is no live session. */
export async function getOperatorSession(signal?: AbortSignal) {
  const { data } = await request<{ operator: Operator }>(
    { path: "/v1/auth/session", signal },
    operatorEnvelopeSchema as never,
  );
  return data.operator;
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
/* Functions                                                                   */
/* -------------------------------------------------------------------------- */

export interface ListFunctionsParams {
  category?: string;
  capability?: string;
  /** The current release supports only `available`. */
  status?: "available";
}

export function listFunctions(
  params: ListFunctionsParams = {},
  signal?: AbortSignal,
) {
  return request<FunctionList>({ path: "/v1/functions", query: { ...params }, signal },
    functionListSchema as never,
  );
}

export function getFunction(functionName: string, signal?: AbortSignal) {
  return request<FunctionDefinition>({ path: `/v1/functions/${functionName}`, signal },
    functionDefinitionSchema as never,
  );
}

export function getFunctionVersion(
  functionName: string,
  version: string,
  signal?: AbortSignal,
) {
  return request<FunctionManifest>({ path: `/v1/functions/${functionName}/versions/${version}`, signal },
    functionManifestSchema as never,
  );
}

export function getFunctionManifestBundle(signal?: AbortSignal) {
  return request<ManifestBundle>({ path: "/v1/function-manifest-bundle", signal },
    manifestBundleSchema as never,
  );
}

/**
 * A synchronous invocation resolves to a completed `Invocation`; an accepted
 * asynchronous one resolves to an `AcceptedJob`. The caller must branch on
 * `kind` rather than assume — a 202 does not carry an invocation.
 */
export type InvokeOutcome =
  | { kind: "invocation"; result: ApiResult<Invocation> }
  | { kind: "accepted"; result: ApiResult<AcceptedJob> };

export async function invokeFunction(
  functionName: string,
  body: InvokeFunctionRequest,
): Promise<InvokeOutcome> {
  const isAsync = body.execution === "async";
  const result = await request<unknown>({
    method: "POST",
    path: `/v1/functions/${functionName}/invoke`,
    body,
    // Required when `execution=async`; ignored for synchronous execution.
    idempotent: isAsync,
  });

  if (result.meta.status === 202) {
    return {
      kind: "accepted",
      result: result as ApiResult<AcceptedJob>,
    };
  }
  return { kind: "invocation", result: result as ApiResult<Invocation> };
}

/* -------------------------------------------------------------------------- */
/* Invocations                                                                 */
/* -------------------------------------------------------------------------- */

export interface ListInvocationsParams {
  function?: string;
  version?: string;
  status?: string;
  limit?: number;
  cursor?: string;
}

export function listFunctionInvocations(
  params: ListInvocationsParams = {},
  signal?: AbortSignal,
) {
  return request<InvocationList>({ path: "/v1/function-invocations", query: { ...params }, signal },
    invocationListSchema as never,
  );
}

export function getFunctionInvocation(
  invocationId: string,
  signal?: AbortSignal,
) {
  return request<Invocation>({ path: `/v1/function-invocations/${invocationId}`, signal },
    invocationSchema as never,
  );
}

/**
 * Signals cancellation to a running *direct* invocation. Job-owned execution is
 * canceled through its job endpoint instead.
 */
export function cancelFunctionInvocation(invocationId: string) {
  return request(
    { method: "POST", path: `/v1/function-invocations/${invocationId}/cancel` },
    cancelInvocationResponseSchema as never,
  );
}

/* -------------------------------------------------------------------------- */
/* Jobs                                                                        */
/* -------------------------------------------------------------------------- */

export interface ListJobsParams {
  /** Repeats as `?status=a&status=b`. */
  status?: JobState[];
  created_after?: string;
  limit?: number;
  cursor?: string;
}

/**
 * Server-side job filters are limited to status, created-after, cursor and
 * limit. Do not send `tag`, mode, failure, function, agent or created-before —
 * the current server rejects `tag` rather than ignoring it.
 */
export function listJobs(params: ListJobsParams = {}, signal?: AbortSignal) {
  return request<JobList>({
      path: "/v1/jobs",
      query: {
        status: params.status,
        created_after: params.created_after,
        limit: params.limit,
        cursor: params.cursor,
      },
      signal,
    },
    jobListSchema as never,
  );
}

export function createJob(body: CreateJobRequest) {
  return request<AcceptedJob>({ method: "POST", path: "/v1/jobs", body, idempotent: true },
    acceptedJobSchema as never,
  );
}

export function getJob(jobId: string, signal?: AbortSignal) {
  return request<Job>({ path: `/v1/jobs/${jobId}`, signal }, jobSchema as never);
}

export async function listJobEvents(
  jobId: string,
  signal?: AbortSignal,
): Promise<ApiResult<JobEvent[]>> {
  const { data, meta } = await request<{ events: JobEvent[] }>({ path: `/v1/jobs/${jobId}/events`, signal },
    jobEventListSchema as never,
  );
  return { data: data.events, meta };
}

export async function listJobAttempts(
  jobId: string,
  signal?: AbortSignal,
): Promise<ApiResult<JobAttempt[]>> {
  const { data, meta } = await request<{ attempts: JobAttempt[] }>({ path: `/v1/jobs/${jobId}/attempts`, signal },
    jobAttemptListSchema as never,
  );
  return { data: data.attempts, meta };
}

/** Cancellation is intrinsically idempotent, so it takes no Idempotency-Key. */
export function cancelJob(jobId: string, reason?: string) {
  return request<{ job: Job; duplicate: boolean; request_id: string }>({
      method: "POST",
      path: `/v1/jobs/${jobId}/cancel`,
      body: reason ? { reason } : undefined,
    },
    cancelJobResponseSchema as never,
  );
}

/* -------------------------------------------------------------------------- */
/* Fetch                                                                       */
/* -------------------------------------------------------------------------- */

/**
 * The release-owned `http.fetch` instrument. Synchronous HTTP work must come
 * through here rather than the generic `/invoke`, because a public client
 * cannot assign the trusted HTTP execution context that generic route requires.
 */
export async function fetchUrl(
  body: FetchRequest,
): Promise<InvokeOutcome> {
  const isAsync = body.execution === "async";
  const result = await request<unknown>({
    method: "POST",
    path: "/v1/fetch",
    body,
    idempotent: isAsync,
  });

  if (result.meta.status === 202) {
    return { kind: "accepted", result: result as ApiResult<AcceptedJob> };
  }
  return { kind: "invocation", result: result as ApiResult<Invocation> };
}
