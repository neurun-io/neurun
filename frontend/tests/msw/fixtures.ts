/**
 * Fixtures shaped to the published contract.
 *
 * The important one is `acceptedJob`: the accepted response carries a `queued`
 * job snapshot, while the event stream holds BOTH `job.accepted` and
 * `job.queued` in sequence order. That asymmetry is real — `accepted` is
 * normally never observable as a polled state — and the dashboard has to
 * preserve it.
 */
import type {
  AcceptedJob,
  FunctionManifest,
  Invocation,
  Job,
  JobAttempt,
  JobEvent,
} from "@/lib/api/types";

export const PROJECT_ID = "prj_01HXQ8F2TEST";
export const JOB_ID = "job_01HXQ8F2ABCDEF";
export const INVOCATION_ID = "fni_01HXQ8F2ABCDEF";
export const DIGEST = `sha256:${"9f3a".repeat(16)}`;

export const echoManifest: FunctionManifest = {
  name: "system.echo",
  version: "1",
  digest: DIGEST,
  category: "system",
  description: "Return the supplied input unchanged.",
  execution_context: "none",
  side_effects: "pure",
  timeout: { default_ms: 5_000, maximum_ms: 30_000 },
  capabilities: ["none"],
  input_schema: {
    type: "object",
    required: ["message"],
    properties: {
      message: { type: "string", maxLength: 256 },
      repeat: { type: "integer", minimum: 1, maximum: 10 },
      loud: { type: "boolean" },
      // Exercises the secret-field treatment in the generated form.
      api_key: { type: "string" },
    },
  },
  output_schema: {
    type: "object",
    properties: { message: { type: "string" } },
  },
  retry: { max_attempts: 3, retryable_categories: ["transient"] },
};

/** Requires a browser context, so it must not offer async execution. */
export const browserManifest: FunctionManifest = {
  ...echoManifest,
  name: "browser.screenshot",
  execution_context: "browser_attempt",
  side_effects: "non_idempotent",
};

export const queuedJob: Job = {
  id: JOB_ID,
  project_id: PROJECT_ID,
  request: {
    project_id: PROJECT_ID,
    function: { name: "system.echo", version: "1", digest: DIGEST },
    input: { message: "hello" },
    max_attempts: 3,
    retry_policy: {
      // Go durations, encoded as nanoseconds: 1s and 30s.
      initial_backoff: 1_000_000_000,
      max_backoff: 30_000_000_000,
    },
    digest: DIGEST,
  },
  state: "queued",
  attempt_count: 0,
  max_attempts: 3,
  version: 1,
  created_at: "2026-07-29T10:00:00Z",
  updated_at: "2026-07-29T10:00:00Z",
};

export const succeededJob: Job = {
  ...queuedJob,
  state: "succeeded",
  attempt_count: 1,
  terminal_attempt_id: "att_01HXQ8F2ABCDEF",
  result: { message: "hello" },
  version: 4,
  updated_at: "2026-07-29T10:00:04Z",
  completed_at: "2026-07-29T10:00:04Z",
};

/** A state this client build does not know. Must render, must not crash. */
export const unknownStateJob: Job = {
  ...queuedJob,
  id: "job_01HXQ8F2UNKNOWN",
  state: "quarantined" as Job["state"],
};

export const acceptedJob: AcceptedJob = {
  job: queuedJob,
  job_id: JOB_ID,
  duplicate: false,
  // The all-in-one server's answer. Never label this durable.
  durability: "process_local",
  request_id: "req_01HXQ8F2ACCEPT",
};

export const jobEvents: JobEvent[] = [
  {
    id: "evt_01",
    job_id: JOB_ID,
    sequence: 1,
    type: "job.accepted",
    state: "accepted",
    created_at: "2026-07-29T10:00:00.000Z",
  },
  {
    id: "evt_02",
    job_id: JOB_ID,
    sequence: 2,
    type: "job.queued",
    state: "queued",
    created_at: "2026-07-29T10:00:00.010Z",
  },
  {
    id: "evt_03",
    job_id: JOB_ID,
    attempt_id: "att_01HXQ8F2ABCDEF",
    sequence: 3,
    type: "attempt.leased",
    state: "leased",
    payload: { agent_id: "agt_01" },
    created_at: "2026-07-29T10:00:01.000Z",
  },
];

export const jobAttempts: JobAttempt[] = [
  {
    id: "att_01HXQ8F2ABCDEF",
    job_id: JOB_ID,
    number: 1,
    agent_id: "agt_01",
    state: "succeeded",
    fence: 1,
    lease_expires_at: "2026-07-29T10:00:31Z",
    trace_id: "trc_01HXQ8F2",
    created_at: "2026-07-29T10:00:01Z",
    started_at: "2026-07-29T10:00:01.100Z",
    finished_at: "2026-07-29T10:00:04Z",
    result: { message: "hello" },
  },
];

export const succeededInvocation: Invocation = {
  invocation_id: INVOCATION_ID,
  project_id: PROJECT_ID,
  function: { name: "system.echo", version: "1", digest: DIGEST },
  status: "succeeded",
  side_effect_class: "pure",
  output: { message: "hello" },
  output_schema_valid: true,
  usage: { duration_ms: 412, cpu_seconds: 0.08, network_bytes: 2048 },
  trace_id: "trc_01HXQ8F2",
  span_id: "spn_01HXQ8F2",
  created_at: "2026-07-29T10:00:00Z",
  started_at: "2026-07-29T10:00:00.010Z",
  finished_at: "2026-07-29T10:00:00.422Z",
};

/**
 * Transport succeeded; the returned data failed the published output schema.
 * The dashboard must show these as different outcomes.
 */
export const schemaRejectedInvocation: Invocation = {
  ...succeededInvocation,
  invocation_id: "fni_01HXQ8F2REJECT",
  output_schema_valid: false,
};

export const durableBackendUnavailable = {
  error: {
    code: "durable_backend_unavailable",
    message:
      "Asynchronous jobs are disabled because no durable backend is configured. Set NEURUN_ALLOW_VOLATILE_JOBS=true to enable volatile development jobs.",
  },
  request_id: "req_01HXQ8F2UNAVAIL",
};

export const unauthorized = {
  error: { code: "unauthorized", message: "401 unauthorized — key rejected for this project" },
  request_id: "req_01HXQ8F2UNAUTH",
};

export const invalidRequest = {
  error: {
    code: "invalid_request",
    // Human-readable path inside the message, as the current server emits.
    message: "$.input.message: must be a string",
  },
  request_id: "req_01HXQ8F2INVALID",
};
