import { z } from "zod";

import { request } from "./client";
import type {
  ApiKey,
  BrowserIdentity,
  BrowserKind,
  BrowserProfile,
  BrowserProfileState,
  BrowserSession,
  Build,
  CreatedApiKey,
  Deployment,
  Execution,
  IdentityCatalog,
  Installation,
  NeurunApp,
  Repository,
  Project,
  User,
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
  runtime: z.enum(["python", "rust", "go", "ruby", "node"]),
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
  runtime: z.enum(["python", "rust", "go", "ruby", "node"]),
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
  repository: z.string().optional(),
  production_ref: z.string().optional(),
  created_at: timestampSchema,
  updated_at: timestampSchema,
});
const browserSessionSchema = z.looseObject({
  id: z.string(),
  app_id: z.string(),
  execution_id: z.string().optional(),
  browser_profile_id: z.string().optional(),
  browser: z.enum(["chrome", "safari"]),
  status: z.enum(["starting", "live", "failed"]),
  started_at: timestampSchema,
  updated_at: timestampSchema,
});
const repositorySchema = z.looseObject({
  full_name: z.string(),
  default_branch: z.string(),
  private: z.boolean(),
});
const installationSchema = z.looseObject({
  id: z.string(),
  organization_id: z.string(),
  installation_id: z.number(),
  account_login: z.string(),
  created_at: timestampSchema,
  updated_at: timestampSchema,
});
const userSchema = z.looseObject({
  id: z.string(),
  email: z.string(),
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

const redactedIdentitySchema = z.looseObject({
  os: z.string(),
  os_version: z.string(),
  browser: z.string(),
  geo: z.string(),
  proxy_set: z.boolean(),
});
const redactedCookieSchema = z.looseObject({
  name: z.string(),
  domain: z.string(),
  path: z.string(),
  expires: z.number().nullish(),
  secure: z.boolean(),
  http_only: z.boolean(),
  same_site: z.string().nullish(),
  value_size: z.number(),
});
const browserProfileSchema = z.looseObject({
  id: z.string(),
  name: z.string(),
  browser: z.enum(["chrome", "safari"]),
  identity: redactedIdentitySchema,
  cookies: z.array(redactedCookieSchema),
  storage_origins: z.array(z.string()),
  created_at: timestampSchema,
  updated_at: timestampSchema,
});
const browserStorageSchema = z.record(z.string(), z.record(z.string(), z.string()));
const browserProfileStateSchema = z.looseObject({
  cookies: z.array(redactedCookieSchema.omit({ value_size: true }).extend({
    value: z.string(),
  })),
  local_storage: browserStorageSchema,
  session_storage: browserStorageSchema,
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

export function createApp(
  projectId: string,
  name: string,
  repository: string,
  productionRef: string,
) {
  return request<NeurunApp>(
    {
      method: "POST",
      path: "/v1/apps",
      body: {
        project_id: projectId,
        name,
        repository,
        production_ref: productionRef,
      },
    },
    appSchema as never,
  );
}

/** Cascades to the app's deployments, builds and executions. */
export function deleteApp(id: string) {
  return request<void>({ method: "DELETE", path: `/v1/apps/${segment(id)}` });
}

/** An empty repository disconnects the app. */
export function connectRepository(id: string, repository: string, productionRef: string) {
  return request<NeurunApp>(
    {
      method: "PUT",
      path: `/v1/apps/${segment(id)}/repository`,
      body: { repository, production_ref: productionRef },
    },
    appSchema as never,
  );
}

export function listBrowserSessions(signal?: AbortSignal) {
  return request<{ browser_sessions: BrowserSession[] }>(
    { path: "/v1/browser-sessions", signal },
    z.looseObject({ browser_sessions: z.array(browserSessionSchema) }) as never,
  );
}

export function getBrowserSession(id: string, signal?: AbortSignal) {
  return request<BrowserSession>(
    { path: `/v1/browser-sessions/${segment(id)}`, signal },
    browserSessionSchema as never,
  );
}

/** Forgets the session here. The handler still owns stopping the browser. */
export function closeBrowserSession(id: string) {
  return request<void>({
    method: "DELETE",
    path: `/v1/browser-sessions/${segment(id)}`,
  });
}

export function listRepositories(signal?: AbortSignal) {
  return request<{ repositories: Repository[] }>(
    { path: "/v1/github/repositories", signal },
    z.looseObject({ repositories: z.array(repositorySchema) }) as never,
  );
}

export function listBranches(repository: string, signal?: AbortSignal) {
  return request<{ branches: string[] }>(
    { path: "/v1/github/branches", query: { repository }, signal },
    z.looseObject({ branches: z.array(z.string()) }) as never,
  );
}

export function getInstallation(signal?: AbortSignal) {
  return request<Installation>(
    { path: "/v1/github/installation", signal },
    installationSchema as never,
  );
}

export function recordInstallation(installationId: string) {
  return request<Installation>(
    {
      method: "POST",
      path: "/v1/github/installation",
      body: { installation_id: installationId },
    },
    installationSchema as never,
  );
}

export function deleteInstallation() {
  return request<void>({ method: "DELETE", path: "/v1/github/installation" });
}

/** Build a ref by hand. A push to the production ref does the same thing. */
export function deployRef(appId: string, ref: string) {
  return request<Deployment>(
    { method: "POST", path: "/v1/github/deployments", body: { app_id: appId, ref } },
    deploymentSchema as never,
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

export function updateUser(
  id: string,
  body: { email?: string; disabled?: boolean },
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

/**
 * Static reference data, so the shape is asserted only where the form binds on
 * it — everything else rides through as the server sent it.
 */
export function getIdentityCatalog(signal?: AbortSignal) {
  return request<IdentityCatalog>(
    { path: "/v1/identity-catalog", signal },
    z.looseObject({
      operating_systems: z.array(
        z.looseObject({
          os: z.string(),
          form_factor: z.string(),
          navigator_platform: z.string(),
          browsers: z.array(z.string()),
          versions: z.array(
            z.looseObject({ os_version: z.string(), platform_versions: z.array(z.string()) }),
          ),
          gpus: z.array(
            z.looseObject({
              vendor: z.string(),
              webgl_renderer: z.string(),
              webgl_vendor: z.string(),
            }),
          ),
        }),
      ),
      devices: z.array(
        z.looseObject({
          name: z.string(),
          os: z.string(),
          browsers: z.array(z.string()),
          models: z.array(z.string()),
          versions: z.array(
            z.looseObject({ os_version: z.string(), platform_versions: z.array(z.string()) }),
          ),
          navigator_platforms: z.array(z.string()),
          screen: z.looseObject({
            logical_width: z.number(),
            logical_height: z.number(),
            original_width: z.number(),
            original_height: z.number(),
            density_pixel_ratio: z.number(),
          }),
          hardware_concurrency: z.array(z.number()),
          memory: z.array(z.number()),
          gpus: z.array(
            z.looseObject({
              vendor: z.string(),
              webgl_renderer: z.string(),
              webgl_vendor: z.string(),
            }),
          ),
        }),
      ),
      browsers: z.array(z.looseObject({ browser: z.string(), versions: z.array(z.string()) })),
      screens: z.array(z.looseObject({ width: z.number(), height: z.number() })),
      density_pixel_ratios: z.array(z.number()),
      hardware_concurrency: z.array(z.number()),
      memory: z.array(z.number()),
      geos: z.array(
        z.looseObject({
          code: z.string(),
          languages: z.array(z.string()),
          timezone: z.string(),
        }),
      ),
    }) as never,
  );
}

export function listBrowserProfiles(signal?: AbortSignal) {
  return request<{ browser_profiles: BrowserProfile[] }>(
    { path: "/v1/browser-profiles", query: { limit: 200 }, signal },
    z.looseObject({ browser_profiles: z.array(browserProfileSchema) }) as never,
  );
}

export function getBrowserProfile(id: string, signal?: AbortSignal) {
  return request<BrowserProfile>(
    { path: `/v1/browser-profiles/${segment(id)}`, signal },
    browserProfileSchema as never,
  );
}

/** No identity means no opinion: the server draws one from the catalogue. */
export function createBrowserProfile(body: { name: string; identity?: BrowserIdentity }) {
  return request<BrowserProfile>(
    { method: "POST", path: "/v1/browser-profiles", body },
    browserProfileSchema as never,
  );
}

export function updateBrowserProfile(
  id: string,
  body: { name?: string; identity?: BrowserIdentity },
) {
  return request<BrowserProfile>(
    { method: "PATCH", path: `/v1/browser-profiles/${segment(id)}`, body },
    browserProfileSchema as never,
  );
}

export function deleteBrowserProfile(id: string) {
  return request<void>({
    method: "DELETE",
    path: `/v1/browser-profiles/${segment(id)}`,
  });
}

/**
 * Returns cookie values and storage contents in the clear, which is why it is
 * fetched on demand rather than folded into the profile.
 */
export function getBrowserProfileState(id: string, signal?: AbortSignal) {
  return request<BrowserProfileState>(
    { path: `/v1/browser-profiles/${segment(id)}/state`, signal },
    browserProfileStateSchema as never,
  );
}

