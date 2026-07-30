/**
 * Runtime validation at the network boundary.
 *
 * These schemas deliberately do NOT re-encode the generated types. They assert
 * only the shape the UI actually reads, and they do it with two standing rules:
 *
 * 1. **Enums stay open.** Status-like fields validate as `string`, never as a
 *    closed union. A future server value must render as a neutral badge showing
 *    its raw text — it may never crash a route, and it may never be silently
 *    mapped onto success. Classification is a presentation concern; see
 *    `lib/view/status.ts`.
 * 2. **Unknown keys survive.** Several manifest policy, retry, redaction,
 *    telemetry, artifact and resource-policy objects are still open extension
 *    maps in the contract. `looseObject` preserves them so nothing is dropped
 *    on the way through.
 */
import { z } from "zod";
import { NeurunContractError } from "./errors";

const timestamp = z.string();
const openRecord = z.looseObject({});

export const functionRefSchema = z.looseObject({
  name: z.string(),
  version: z.string(),
  digest: z.string(),
});

export const failureSchema = z.looseObject({
  category: z.string(),
  code: z.string().optional(),
  message: z.string().optional(),
  retryable: z.boolean(),
  details: z.record(z.string(), z.string()).optional(),
});

export const usageSchema = z.looseObject({
  duration_ms: z.number(),
  cpu_seconds: z.number().optional(),
  peak_rss_bytes: z.number().optional(),
  network_bytes: z.number().optional(),
  artifact_bytes: z.number().optional(),
});

export const executionContextSchema = z.looseObject({
  project_id: z.string().optional(),
  job_id: z.string().optional(),
  attempt_id: z.string().optional(),
  session_id: z.string().optional(),
  workflow_step_id: z.string().optional(),
  ephemeral_http: z.boolean().optional(),
  ephemeral_browser: z.boolean().optional(),
  capabilities: z.array(z.string()).optional(),
});

export const invocationSchema = z.looseObject({
  invocation_id: z.string(),
  request_id: z.string().optional(),
  project_id: z.string(),
  function: functionRefSchema,
  // open on purpose — see rule 1
  status: z.string(),
  side_effect_class: z.string().optional(),
  input_hash: z.string().optional(),
  redacted_input: z.unknown().optional(),
  output: z.unknown().optional(),
  output_schema_valid: z.boolean(),
  failure: failureSchema.optional(),
  usage: usageSchema,
  artifacts: z.array(openRecord).optional(),
  trace_id: z.string(),
  span_id: z.string(),
  context: executionContextSchema.optional(),
  created_at: timestamp,
  started_at: timestamp.optional(),
  finished_at: timestamp.optional(),
});

export const invocationListSchema = z.looseObject({
  invocations: z.array(invocationSchema),
  next_cursor: z.string(),
});

export const durableRequestSchema = z.looseObject({
  project_id: z.string(),
  function: functionRefSchema,
  input: z.unknown(),
  max_attempts: z.number(),
  retry_policy: z
    .looseObject({
      // Go durations encoded as nanoseconds. Converted explicitly in the view
      // model — never inferred from an untyped retry payload.
      initial_backoff: z.number(),
      max_backoff: z.number(),
    })
    .optional(),
  digest: z.string(),
});

export const jobSchema = z.looseObject({
  id: z.string(),
  project_id: z.string(),
  request: durableRequestSchema,
  state: z.string(),
  attempt_count: z.number(),
  max_attempts: z.number(),
  current_attempt_id: z.string().optional(),
  terminal_attempt_id: z.string().optional(),
  next_attempt_at: timestamp.optional(),
  last_failure: openRecord.optional(),
  last_retry: openRecord.optional(),
  result: z.unknown().optional(),
  version: z.number(),
  created_at: timestamp,
  updated_at: timestamp,
  completed_at: timestamp.optional(),
  canceled_at: timestamp.optional(),
});

export const jobListSchema = z.looseObject({
  jobs: z.array(jobSchema),
  next_cursor: z.string(),
});

export const acceptedJobSchema = z.looseObject({
  job: jobSchema,
  job_id: z.string(),
  duplicate: z.boolean(),
  // Required on every accepted asynchronous mutation. A 202 alone does not
  // imply durable persistence.
  durability: z.string(),
  request_id: z.string(),
});

export const jobEventSchema = z.looseObject({
  id: z.string(),
  job_id: z.string(),
  attempt_id: z.string().optional(),
  sequence: z.number(),
  type: z.string(),
  state: z.string(),
  payload: z.unknown().optional(),
  created_at: timestamp,
});

export const jobEventListSchema = z.looseObject({
  events: z.array(jobEventSchema),
});

export const jobAttemptSchema = z.looseObject({
  id: z.string(),
  job_id: z.string(),
  number: z.number(),
  agent_id: z.string(),
  state: z.string(),
  fence: z.number(),
  lease_expires_at: timestamp,
  trace_id: z.string().optional(),
  failure: openRecord.optional(),
  retry: openRecord.optional(),
  result: z.unknown().optional(),
  created_at: timestamp,
  started_at: timestamp.optional(),
  finished_at: timestamp.optional(),
});

export const jobAttemptListSchema = z.looseObject({
  attempts: z.array(jobAttemptSchema),
});

export const cancelJobResponseSchema = z.looseObject({
  job: jobSchema,
  duplicate: z.boolean(),
  request_id: z.string(),
});

export const cancelInvocationResponseSchema = z.looseObject({
  invocation_id: z.string(),
  request_id: z.string(),
  status: z.string(),
});

/** A JSON Schema subset, recursive on `properties` and `items`. */
export const jsonSchemaSchema: z.ZodType<Record<string, unknown>> = z.lazy(() =>
  z.looseObject({
    type: z.string().optional(),
    required: z.array(z.string()).optional(),
    properties: z.record(z.string(), jsonSchemaSchema).optional(),
    additionalProperties: z.union([z.boolean(), jsonSchemaSchema]).optional(),
    items: jsonSchemaSchema.optional(),
    enum: z.array(z.unknown()).optional(),
    minimum: z.number().optional(),
    maximum: z.number().optional(),
    minLength: z.number().optional(),
    maxLength: z.number().optional(),
  }),
);

export const functionManifestSchema = z.looseObject({
  name: z.string(),
  version: z.string(),
  digest: z.string(),
  category: z.string(),
  description: z.string().optional(),
  execution_context: z.string(),
  side_effects: z.string(),
  timeout: z.looseObject({
    default_ms: z.number(),
    maximum_ms: z.number(),
  }),
  capabilities: z.array(z.string()).optional(),
  permissions: z.array(z.string()).optional(),
  input_schema: jsonSchemaSchema,
  output_schema: jsonSchemaSchema,
  // still open extension maps in the contract
  resource_policy: openRecord.optional(),
  artifacts: openRecord.optional(),
  redaction: openRecord.optional(),
  retry: openRecord.optional(),
  telemetry: openRecord.optional(),
});

export const functionListSchema = z.looseObject({
  functions: z.array(functionManifestSchema),
});

export const functionDefinitionSchema = z.looseObject({
  name: z.string(),
  versions: z.array(functionManifestSchema),
});

export const manifestBundleSchema = z.looseObject({
  schema_version: z.string(),
  bundle_version: z.string(),
  digest: z.string(),
  signature: z.string().optional(),
  manifests: z.array(functionManifestSchema),
});

export const versionSchema = z.looseObject({
  version: z.string(),
  commit: z.string(),
  built_at: z.string(),
  api_version: z.string(),
  schema_version: z.string(),
  function_bundle: z.string(),
});

export const healthSchema = z.looseObject({ status: z.string() });

/**
 * Validate a decoded response body, raising a contract error that names the
 * offending paths rather than letting a malformed payload propagate into the UI.
 */
export function validateResponse<T>(schema: z.ZodType<T>, path: string, body: unknown): T {
  const result = schema.safeParse(body);
  if (result.success) return result.data;

  const issues = result.error.issues
    .slice(0, 8)
    .map((issue) => `${issue.path.join(".") || "<root>"}: ${issue.message}`);
  throw new NeurunContractError(path, issues);
}
