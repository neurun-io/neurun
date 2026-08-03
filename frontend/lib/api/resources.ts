import { z } from "zod";

import { request } from "./client";
import type {
  ApiKey,
  Build,
  CreatedApiKey,
  Deployment,
  Execution,
  NeurunApp,
  Project,
  User,
  UserRole,
} from "./resource-types";

const timestampSchema = z.string();
const failureSchema = z.looseObject({ code: z.string(), message: z.string() });
const artifactSchema = z.looseObject({
  id: z.string(),
  kind: z.string(),
  name: z.string(),
  media_type: z.string(),
  size_bytes: z.number(),
  sha256: z.string(),
  created_at: timestampSchema,
});
const buildSchema = z.looseObject({
  id: z.string(),
  project_id: z.string(),
  deployment_id: z.string(),
  number: z.number(),
  status: z.enum(["building", "ready", "failed"]),
  runtime: z.literal("python"),
  entrypoint: z.string(),
  source_sha256: z.string(),
  artifacts: z.array(artifactSchema),
  failure: failureSchema.nullish(),
  started_at: timestampSchema,
  finished_at: timestampSchema.nullish(),
});
const deploymentSchema = z.looseObject({
  id: z.string(),
  project_id: z.string(),
  app_id: z.string(),
  runtime: z.literal("python"),
  entrypoint: z.string(),
  status: z.enum(["uploaded", "building", "ready", "failed"]),
  source: artifactSchema,
  builds: z.array(buildSchema),
  created_at: timestampSchema,
  updated_at: timestampSchema,
});
const executionSchema = z.looseObject({
  id: z.string(),
  project_id: z.string(),
  deployment_id: z.string(),
  build_id: z.string(),
  status: z.enum(["queued", "running", "succeeded", "failed"]),
  input: z.unknown(),
  output: z.unknown().optional(),
  logs: z.string().optional(),
  failure: failureSchema.nullish(),
  created_at: timestampSchema,
  started_at: timestampSchema.nullish(),
  finished_at: timestampSchema.nullish(),
  rerun_of_execution_id: z.string().optional(),
});
const projectSchema = z.looseObject({
  id: z.string(),
  name: z.string(),
  created_at: timestampSchema,
  updated_at: timestampSchema,
});
const appSchema = z.looseObject({
  id: z.string(),
  project_id: z.string(),
  name: z.string(),
  created_at: timestampSchema,
  updated_at: timestampSchema,
});
const userSchema = z.looseObject({
  id: z.string(),
  username: z.string(),
  display_name: z.string(),
  role: z.enum(["admin", "operator", "viewer"]),
  disabled: z.boolean(),
  created_at: timestampSchema,
  updated_at: timestampSchema,
});
const keySchema = z.looseObject({
  id: z.string(),
  user_id: z.string().nullish(),
  name: z.string(),
  prefix: z.string(),
  scopes: z.array(z.string()),
  created_at: timestampSchema,
  revoked_at: timestampSchema.nullish(),
});

function segment(identifier: string): string {
  return encodeURIComponent(identifier);
}

export function listProjects(signal?: AbortSignal) {
  return request<{ projects: Project[] }>(
    { path: "/v1/projects", query: { limit: 200 }, signal },
    z.looseObject({ projects: z.array(projectSchema) }) as never,
  );
}

export function createProject(name: string) {
  return request<Project>(
    { method: "POST", path: "/v1/projects", body: { name } },
    projectSchema as never,
  );
}

/** Cascades to the project's apps, deployments, builds and executions. */
export function deleteProject(id: string) {
  return request<void>({ method: "DELETE", path: `/v1/projects/${segment(id)}` });
}

export function getProject(id: string, signal?: AbortSignal) {
  return request<Project>(
    { path: `/v1/projects/${segment(id)}`, signal },
    projectSchema as never,
  );
}

export function updateProject(id: string, name: string) {
  return request<Project>(
    { method: "PATCH", path: `/v1/projects/${segment(id)}`, body: { name } },
    projectSchema as never,
  );
}

export function listApps(signal?: AbortSignal) {
  return request<{ apps: NeurunApp[] }>(
    { path: "/v1/apps", query: { limit: 200 }, signal },
    z.looseObject({ apps: z.array(appSchema) }) as never,
  );
}

export function getApp(id: string, signal?: AbortSignal) {
  return request<NeurunApp>(
    { path: `/v1/apps/${segment(id)}`, signal },
    appSchema as never,
  );
}

export function createApp(projectId: string, name: string) {
  return request<NeurunApp>(
    { method: "POST", path: "/v1/apps", body: { project_id: projectId, name } },
    appSchema as never,
  );
}

/** Cascades to the app's deployments, builds and executions. */
export function deleteApp(id: string) {
  return request<void>({ method: "DELETE", path: `/v1/apps/${segment(id)}` });
}

export function updateApp(id: string, name: string) {
  return request<NeurunApp>(
    { method: "PATCH", path: `/v1/apps/${segment(id)}`, body: { name } },
    appSchema as never,
  );
}

export function listDeployments(appId?: string, signal?: AbortSignal) {
  return request<{ deployments: Deployment[] }>(
    { path: "/v1/deployments", query: { app_id: appId, limit: 200 }, signal },
    z.looseObject({ deployments: z.array(deploymentSchema) }) as never,
  );
}

export function getDeployment(id: string, signal?: AbortSignal) {
  return request<Deployment>(
    { path: `/v1/deployments/${segment(id)}`, signal },
    deploymentSchema as never,
  );
}

export function createDeployment(appId: string, source: File, entrypoint: string) {
  const body = new FormData();
  body.set("app_id", appId);
  body.set("runtime", "python");
  body.set("entrypoint", entrypoint);
  body.set("source", source);
  return request<Deployment>(
    { method: "POST", path: "/v1/deployments", body },
    deploymentSchema as never,
  );
}

export function listBuilds(deploymentId?: string, signal?: AbortSignal) {
  return request<{ builds: Build[] }>(
    {
      path: "/v1/builds",
      query: { deployment_id: deploymentId, limit: 200 },
      signal,
    },
    z.looseObject({ builds: z.array(buildSchema) }) as never,
  );
}

export function getBuild(id: string, signal?: AbortSignal) {
  return request<Build>(
    { path: `/v1/builds/${segment(id)}`, signal },
    buildSchema as never,
  );
}

export function listExecutions(deploymentId?: string, signal?: AbortSignal) {
  return request<{ executions: Execution[] }>(
    {
      path: "/v1/executions",
      query: { deployment_id: deploymentId, limit: 200 },
      signal,
    },
    z.looseObject({ executions: z.array(executionSchema) }) as never,
  );
}

export function getExecution(id: string, signal?: AbortSignal) {
  return request<Execution>(
    { path: `/v1/executions/${segment(id)}`, signal },
    executionSchema as never,
  );
}

export function createExecution(deploymentId: string, input: unknown) {
  return request<Execution>(
    {
      method: "POST",
      path: `/v1/deployments/${segment(deploymentId)}/executions`,
      body: { input },
    },
    executionSchema as never,
  );
}

export function rerunExecution(id: string) {
  return request<Execution>(
    { method: "POST", path: `/v1/executions/${segment(id)}/rerun` },
    executionSchema as never,
  );
}

export function listUsers(signal?: AbortSignal) {
  return request<{ users: User[] }>(
    { path: "/v1/users", query: { limit: 200 }, signal },
    z.looseObject({ users: z.array(userSchema) }) as never,
  );
}

export function createUser(body: {
  username: string;
  display_name: string;
  role: UserRole;
  password: string;
}) {
  return request<User>({ method: "POST", path: "/v1/users", body }, userSchema as never);
}

export function updateUser(
  id: string,
  body: { display_name?: string; role?: UserRole; disabled?: boolean },
) {
  return request<User>(
    { method: "PATCH", path: `/v1/users/${segment(id)}`, body },
    userSchema as never,
  );
}

export function listAPIKeys(signal?: AbortSignal) {
  return request<{ api_keys: ApiKey[] }>(
    { path: "/v1/api-keys", query: { limit: 200 }, signal },
    z.looseObject({ api_keys: z.array(keySchema) }) as never,
  );
}

export function createAPIKey(body: { name: string; user_id?: string; scopes: string[] }) {
  return request<CreatedApiKey>(
    { method: "POST", path: "/v1/api-keys", body },
    keySchema.extend({ secret: z.string() }) as never,
  );
}

export function revokeAPIKey(id: string) {
  return request<ApiKey>(
    { method: "DELETE", path: `/v1/api-keys/${segment(id)}` },
    keySchema as never,
  );
}
