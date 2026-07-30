/**
 * Convenience aliases over the generated OpenAPI models.
 *
 * `lib/api/schema.ts` is generated from `api/openapi.yaml` — never hand-edit it,
 * and never hand-maintain a copy of a response interface that can be generated.
 * Run `npm run generate:api` after the contract changes; `npm run check:api-drift`
 * fails CI when the committed client no longer matches the published contract.
 *
 * UI-only view models are welcome, but they belong in `lib/view/`, not here.
 */
import type { components } from "./schema";

type Schemas = components["schemas"];

export type Health = Schemas["Health"];
export type Ready = Schemas["Ready"];
export type Version = Schemas["Version"];

export type Problem = Schemas["Problem"];
export type ErrorEnvelope = Schemas["ErrorEnvelope"];

export type FunctionRef = Schemas["FunctionRef"];
export type RequestedFunctionRef = Schemas["RequestedFunctionRef"];
export type JSONSchema = Schemas["JSONSchema"];
export type FunctionManifest = Schemas["FunctionManifest"];
export type FunctionList = Schemas["FunctionList"];
export type FunctionDefinition = Schemas["FunctionDefinition"];
export type ManifestBundle = Schemas["ManifestBundle"];

export type ExecutionContext = Schemas["ExecutionContext"];
export type InvokeFunctionRequest = Schemas["InvokeFunctionRequest"];
export type InvocationStatus = Schemas["InvocationStatus"];
export type Invocation = Schemas["Invocation"];
export type InvocationList = Schemas["InvocationList"];

export type Failure = Schemas["Failure"];
export type Usage = Schemas["Usage"];

export type CreateJobRequest = Schemas["CreateJobRequest"];
export type DurableRequest = Schemas["DurableRequest"];
export type JobState = Schemas["JobState"];
export type Job = Schemas["Job"];
export type AcceptedJob = Schemas["AcceptedJob"];
export type JobDurability = Schemas["JobDurability"];
export type JobList = Schemas["JobList"];
export type JobEvent = Schemas["JobEvent"];
export type JobAttempt = Schemas["JobAttempt"];
export type JobAttemptState = JobAttempt["state"];

export type HTTPFetchInput = Schemas["HTTPFetchInput"];
export type FetchRequest = Schemas["FetchRequest"];

/**
 * The manifest's execution context. Only `none` and `http_attempt` accept
 * asynchronous execution in the current release.
 */
export type FunctionExecutionContext = FunctionManifest["execution_context"];

/** Execution contexts the current server will accept as an async job. */
export const ASYNC_CAPABLE_EXECUTION_CONTEXTS = ["none", "http_attempt"] as const;

export function acceptsAsyncExecution(manifest: Pick<FunctionManifest, "execution_context">) {
  return (ASYNC_CAPABLE_EXECUTION_CONTEXTS as readonly string[]).includes(
    manifest.execution_context,
  );
}

/**
 * `http.fetch` is the current generic-invoke exception: a public client cannot
 * assign the trusted HTTP context a synchronous generic `/invoke` requires, so
 * synchronous HTTP work must go through `POST /v1/fetch`.
 */
export const HTTP_FETCH_FUNCTION_NAME = "http.fetch";

export function requiresFetchEndpointForSync(functionName: string) {
  return functionName === HTTP_FETCH_FUNCTION_NAME;
}

/** Job states from which no further transition occurs. */
export const TERMINAL_JOB_STATES = [
  "succeeded",
  "rejected",
  "failed",
  "canceled",
  "dead_lettered",
] as const satisfies readonly JobState[];

export function isTerminalJobState(state: string): boolean {
  return (TERMINAL_JOB_STATES as readonly string[]).includes(state);
}

/** Invocation statuses from which no further transition occurs. */
export const TERMINAL_INVOCATION_STATUSES = [
  "succeeded",
  "rejected",
  "failed",
  "timed_out",
  "canceled",
] as const satisfies readonly InvocationStatus[];

export function isTerminalInvocationStatus(status: string): boolean {
  return (TERMINAL_INVOCATION_STATUSES as readonly string[]).includes(status);
}
