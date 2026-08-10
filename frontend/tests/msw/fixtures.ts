/**
 * Fixtures shaped to the published contract.
 */
import type { App, Build, Deployment, Execution, Project } from "@/lib/api/types";

export const ORGANIZATION_ID = "org_01HXQ8F2ACME";
export const PROJECT_ID = "prj_01HXQ8F2TEST";
export const APP_ID = "app_01HXQ8F2TEST";
export const DEPLOYMENT_ID = "dep_01HXQ8F2TEST";
export const BUILD_ID = "bld_01HXQ8F2TEST";
export const EXECUTION_ID = "exe_01HXQ8F2TEST";
export const SOURCE_SHA256 = "9f3a".repeat(16);

export const project: Project = {
  id: PROJECT_ID,
  organization_id: ORGANIZATION_ID,
  name: "Integration",
  created_at: "2026-07-29T10:00:00Z",
  updated_at: "2026-07-29T10:00:00Z",
};

export const app: App = {
  id: APP_ID,
  project_id: PROJECT_ID,
  name: "pricing-crawler",
  created_at: "2026-07-29T10:00:00Z",
  updated_at: "2026-07-29T10:00:00Z",
};

export const readyBuild: Build = {
  id: BUILD_ID,
  project_id: PROJECT_ID,
  deployment_id: DEPLOYMENT_ID,
  number: 1,
  status: "ready",
  runtime: "python",
  entrypoint: "main.py:handler",
  source_sha256: SOURCE_SHA256,
  artifacts: [],
  started_at: "2026-07-29T10:00:01Z",
  finished_at: "2026-07-29T10:00:09Z",
};

export const readyDeployment: Deployment = {
  id: DEPLOYMENT_ID,
  project_id: PROJECT_ID,
  app_id: APP_ID,
  runtime: "python",
  entrypoint: "main.py:handler",
  status: "ready",
  source: {
    id: "art_01HXQ8F2SRC",
    kind: "source",
    name: "dist.zip",
    media_type: "application/zip",
    size_bytes: 2048,
    sha256: SOURCE_SHA256,
    created_at: "2026-07-29T10:00:00Z",
  },
  builds: [readyBuild],
  created_at: "2026-07-29T10:00:00Z",
  updated_at: "2026-07-29T10:00:09Z",
};

export const queuedExecution: Execution = {
  id: EXECUTION_ID,
  project_id: PROJECT_ID,
  deployment_id: DEPLOYMENT_ID,
  build_id: BUILD_ID,
  status: "queued",
  input: { url: "https://example.com" },
  logs: "",
  created_at: "2026-07-29T10:01:00Z",
};

export const succeededExecution: Execution = {
  ...queuedExecution,
  status: "succeeded",
  output: { records: 40 },
  logs: "fetched 40 records\n",
  started_at: "2026-07-29T10:01:01Z",
  finished_at: "2026-07-29T10:01:04Z",
};

/** A status no client build knows. It must render raw, never be coerced. */
export const unknownStateExecution: Execution = {
  ...queuedExecution,
  id: "exe_01HXQ8F2UNKNOWN",
  status: "quarantined" as Execution["status"],
};

export const invalidCredentials = {
  error: { code: "invalid_credentials", message: "invalid email or password" },
};

export const signInUnavailable = {
  error: {
    code: "signin_unavailable",
    message: "sign-in is not configured on this server",
  },
};

export const unauthorized = {
  error: { code: "unauthorized", message: "your session expired; sign in again" },
};

export const invalidRequest = {
  error: {
    code: "invalid_request",
    // Human-readable path inside the message, as the current server emits.
    message: "$.input.message: must be a string",
  },
};
