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

export type Problem = Schemas["Problem"];
export type ErrorEnvelope = Schemas["ErrorEnvelope"];
export type Operator = Schemas["Operator"];
export type Role = Schemas["Role"];

export type Organization = Schemas["Organization"];
export type Member = Schemas["Member"];
export type Invite = Schemas["Invite"];
export type CreatedInvite = Schemas["CreatedInvite"];

export type Project = Schemas["Project"];
export type App = Schemas["App"];
export type Deployment = Schemas["Deployment"];
export type Build = Schemas["Build"];
export type Execution = Schemas["Execution"];
export type Artifact = Schemas["Artifact"];
export type Failure = Schemas["Failure"];
export type User = Schemas["User"];
export type APIKey = Schemas["APIKey"];
export type CreatedAPIKey = Schemas["CreatedAPIKey"];

/** What the server reports about itself. Not a component schema. */
export interface Version {
  version: string;
  commit: string;
  built_at: string;
  api_version: string;
  schema_version: string;
}

/** Execution states from which no further transition occurs. */
export const TERMINAL_EXECUTION_STATUSES = ["succeeded", "failed"] as const;

export function isTerminalExecutionStatus(status: string): boolean {
  return (TERMINAL_EXECUTION_STATUSES as readonly string[]).includes(status);
}

/** Build and deployment states from which no further transition occurs. */
export const TERMINAL_BUILD_STATUSES = ["ready", "failed"] as const;

export function isTerminalBuildStatus(status: string): boolean {
  return (TERMINAL_BUILD_STATUSES as readonly string[]).includes(status);
}
