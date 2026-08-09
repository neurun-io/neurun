/**
 * Runtime validation at the network boundary.
 *
 * These schemas deliberately do NOT re-encode the generated types. They assert
 * only the shape the UI actually reads, and enums stay open: status-like fields
 * validate as `string`, never as a closed union. A future server value must
 * render as a neutral badge showing its raw text — it may never crash a route,
 * and it may never be silently mapped onto success. Classification is a
 * presentation concern; see `lib/view/status.ts`.
 */
import { z } from "zod";
import { NeurunContractError } from "./errors";

const timestamp = z.string();

/**
 * The signed-in operator. `role` stays an open string for the same reason every
 * other enum does: a server that learns a new role must not break the shell.
 */
export const operatorSchema = z.looseObject({
  operator_id: z.string(),
  email: z.string(),
  organization_id: z.string(),
  role: z.string(),
  scopes: z.array(z.string()),
  session_id: z.string(),
  expires_at: timestamp,
});

export const operatorEnvelopeSchema = z.looseObject({
  operator: operatorSchema,
});

/**
 * Registration answers with the account, and with the operator only when the
 * server also managed to sign it in.
 */
export const registrationSchema = z.looseObject({
  user: z.looseObject({ id: z.string(), email: z.string() }),
  organization: z.looseObject({ id: z.string(), name: z.string() }).optional(),
  member: z.looseObject({ role: z.string() }).optional(),
  operator: operatorSchema.optional(),
});

export const organizationSchema = z.looseObject({
  id: z.string(),
  name: z.string(),
  owner_user_id: z.string().optional(),
});

export const memberSchema = z.looseObject({
  user_id: z.string().optional(),
  email: z.string().optional(),
  role: z.string(),
  owner: z.boolean().optional(),
});

/** What a sign-up page may show before the account exists. */
export const invitePreviewSchema = z.looseObject({
  organization: z.looseObject({ id: z.string(), name: z.string() }),
  email: z.string(),
  role: z.string(),
});

export const versionSchema = z.looseObject({
  version: z.string(),
  commit: z.string(),
  built_at: z.string(),
  api_version: z.string(),
  schema_version: z.string(),
  function_bundle: z.string(),
});

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
