export type Runtime = "python" | "rust" | "go" | "ruby" | "node";
export type DeploymentStatus =
  | "queued"
  | "building"
  | "publishing"
  | "ready"
  | "failed";
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
  /** owner/name on GitHub. Absent until the app is connected. */
  repository?: string;
  /** The ref whose pushes deploy. Absent follows the default branch. */
  production_ref?: string;
  /** The build the app runs. Absent runs the newest one it has ready. */
  active_build_id?: string;
  created_at: string;
  updated_at: string;
}

export type BrowserSessionStatus = "starting" | "live" | "failed";

/**
 * One browser a handler has open. It is live state rather than a record: a
 * session that ends stops being listed, and there is no closed one to read.
 */
export interface BrowserSession {
  id: string;
  app_id: string;
  execution_id?: string;
  browser_profile_id?: string;
  browser: BrowserKind;
  status: BrowserSessionStatus;
  started_at: string;
  updated_at: string;
}

/** A repository the installation grants access to. */
export interface Repository {
  full_name: string;
  default_branch: string;
  private: boolean;
}

/** One organization's GitHub App installation. */
export interface Installation {
  id: string;
  organization_id: string;
  installation_id: number;
  account_login: string;
  created_at: string;
  updated_at: string;
}

/** One stored ZIP. The name is the layer it is: code, install, source. */
export interface Artifact {
  id: string;
  name: string;
  size_bytes: number;
  sha256: string;
  created_at: string;
}

export interface Failure {
  code: string;
  message: string;
}

/** A runnable app. How it came to exist belongs to the deployment. */
export interface Build {
  id: string;
  app_id: string;
  deployment_id: string;
  runtime: Runtime;
  source_sha256: string;
  artifacts: Artifact[];
  created_at: string;
}

export interface Deployment {
  id: string;
  project_id: string;
  app_id: string;
  runtime: Runtime;
  status: DeploymentStatus;
  commit_sha?: string;
  git_ref?: string;
  build?: Build | null;
  failure?: Failure | null;
  /** What the toolchain printed, arriving while the deployment still runs. */
  logs: string;
  started_at?: string | null;
  finished_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface Execution {
  id: string;
  project_id: string;
  app_id: string;
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

/**
 * Chrome is the only browser that launches; Safari is a Chrome wearing Safari,
 * which is what a site reads either way.
 */
export type BrowserKind = "chrome" | "safari";

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
  /** What it claims to be, not what runs. */
  browser: BrowserKind;
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

/**
 * The values an identity is assembled from.
 *
 * Several are binding rather than independent: an OS fixes navigator.platform,
 * bitness and architecture, limits which browsers, releases and cards exist
 * under it, and each release carries its own UA-CH platform versions — Windows
 * 11 reports 15.0.0, and 7 and 8 both report 0.0.0.
 */
export interface IdentityCatalog {
  operating_systems: CatalogOS[];
  devices: CatalogDevice[];
  browsers: { browser: BrowserKind; versions: string[] }[];
  screens: { width: number; height: number; share: number }[];
  density_pixel_ratios: number[];
  hardware_concurrency: number[];
  memory: number[];
  geos: CatalogGeo[];
}

export interface CatalogOSVersion {
  os_version: string;
  platform_versions: string[];
}

export interface CatalogOS {
  os: BrowserIdentity["os"];
  /** Mobile carries no platform, releases or cards: the handset does. */
  form_factor: "desktop" | "mobile";
  navigator_platform: string;
  bitness: string;
  architecture: string;
  /** What runs on the platform. There is no Safari on Windows. */
  browsers: BrowserKind[];
  versions: CatalogOSVersion[];
  gpus: CatalogGPU[];
}

/**
 * A handset, and the binding unit on mobile: one model fixes the screen, the
 * ratio, the GPU, the cores and the memory together, because they shipped in one
 * box.
 */
export interface CatalogDevice {
  name: string;
  os: BrowserIdentity["os"];
  browsers: BrowserKind[];
  models: string[];
  versions: CatalogOSVersion[];
  navigator_platforms: string[];
  screen: BrowserIdentity["screen"];
  hardware_concurrency: number[];
  memory: number[];
  gpus: CatalogGPU[];
}

/**
 * One card's WebGL strings. Where it runs is the list it is in — Direct3D is
 * Windows, and only there.
 */
export interface CatalogGPU {
  vendor: string;
  webgl_renderer: string;
  webgl_vendor: string;
}

export interface CatalogGeo {
  code: string;
  languages: string[];
  timezone: string;
}

export interface BrowserProfile {
  id: string;
  name: string;
  /** Repeats identity.browser, so a list does not have to reach into it. */
  browser: BrowserKind;
  identity: RedactedBrowserIdentity;
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

/** Still going, so its output is still arriving. */
export function isDeploymentRunning(status: string): boolean {
  return status === "queued" || status === "building" || status === "publishing";
}
