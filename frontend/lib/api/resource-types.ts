export type PythonRuntime = "python";
export type BuildStatus = "building" | "ready" | "failed";
export type DeploymentStatus = "uploaded" | BuildStatus;
export type ExecutionStatus = "queued" | "running" | "succeeded" | "failed";

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

export type BrowserKind = "chrome" | "firefox";

/** What a browser claims to be. There is no Firefox brand upstream. */
export type BrowserBrand = "chrome" | "safari" | "edge";

export interface BrowserIdentity {
  device_model?: string;
  has_battery: boolean;
  has_mouse: boolean;
  has_touch: boolean;
  os: "Windows" | "Macintosh" | "Linux" | "Android" | "Ios";
  os_version: string;
  platform: {
    bitness?: string;
    architecture?: string;
    navigator_platform: string;
    version: string;
  };
  brand: BrowserBrand;
  browser_version: number[];
  screen: {
    logical_width: number;
    logical_height: number;
    original_width: number;
    original_height: number;
    density_pixel_ratio: number;
  };
  hardware_concurrency: number;
  memory: number;
  gpu: { vendor: string; webgl_renderer: string; webgl_vendor: string };
  geo: string;
  language: string[];
  history_count?: number;
  /** Write-only. Never present on a response — see `proxy_set`. */
  proxy?: string;
  timezone?: string;
}

/** The identity as the API returns it: the proxy is reported, never disclosed. */
export interface RedactedBrowserIdentity extends Omit<BrowserIdentity, "proxy"> {
  proxy_set: boolean;
}

/**
 * A cookie as the profile endpoints return it — enough to see what a profile is
 * logged into, and to delete it, without reading the credential.
 */
export interface RedactedCookie {
  name: string;
  domain: string;
  path: string;
  expires?: number;
  secure: boolean;
  http_only: boolean;
  same_site?: string;
  value_size: number;
}

export interface BrowserProfile {
  id: string;
  name: string;
  browser: BrowserKind;
  identity: RedactedBrowserIdentity | null;
  cookies: RedactedCookie[];
  storage_origins: string[];
  created_at: string;
  updated_at: string;
}

export interface BrowserCookie extends Omit<RedactedCookie, "value_size"> {
  value: string;
}

/** Origin to key to value. */
export type BrowserStorage = Record<string, Record<string, string>>;

export interface BrowserProfileState {
  cookies: BrowserCookie[];
  local_storage: BrowserStorage;
  session_storage: BrowserStorage;
}

export function isTerminalExecutionStatus(status: string): boolean {
  return status === "succeeded" || status === "failed";
}
