export type PythonRuntime = "python";
export type BuildStatus = "building" | "ready" | "failed";
export type DeploymentStatus = "uploaded" | BuildStatus;
export type ExecutionStatus = "queued" | "running" | "succeeded" | "failed";
export type UserRole = "admin" | "operator" | "viewer";

export interface Project {
  id: string;
  name: string;
  created_at: string;
  updated_at: string;
}

export interface NeurunApp {
  id: string;
  project_id: string;
  name: string;
  created_at: string;
  updated_at: string;
}

export interface Artifact {
  id: string;
  kind: string;
  name: string;
  media_type: string;
  size_bytes: number;
  sha256: string;
  created_at: string;
}

export interface Failure {
  code: string;
  message: string;
}

export interface Build {
  id: string;
  project_id: string;
  deployment_id: string;
  number: number;
  status: BuildStatus;
  runtime: PythonRuntime;
  entrypoint: string;
  source_sha256: string;
  artifacts: Artifact[];
  failure?: Failure | null;
  started_at: string;
  finished_at?: string | null;
}

export interface Deployment {
  id: string;
  project_id: string;
  app_id: string;
  runtime: PythonRuntime;
  entrypoint: string;
  status: DeploymentStatus;
  source: Artifact;
  builds: Build[];
  created_at: string;
  updated_at: string;
}

export interface Execution {
  id: string;
  project_id: string;
  deployment_id: string;
  build_id: string;
  status: ExecutionStatus;
  input: unknown;
  output?: unknown;
  logs?: string;
  failure?: Failure | null;
  created_at: string;
  started_at?: string | null;
  finished_at?: string | null;
  rerun_of_execution_id?: string;
}

export interface User {
  id: string;
  email: string;
  disabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface ApiKey {
  id: string;
  name: string;
  user_id?: string | null;
  scopes: string[];
  prefix: string;
  revoked_at?: string | null;
  created_at: string;
}

export interface CreatedApiKey extends ApiKey {
  secret: string;
}

export function isTerminalExecutionStatus(status: string): boolean {
  return status === "succeeded" || status === "failed";
}
