/**
 * Fixtures shaped to the published contract.
 */
import type { App, Build, Deployment, Execution, Project } from "@/lib/api/types";
import type { IdentityCatalog } from "@/lib/api/resource-types";

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
  runtime: "python",
  entrypoint: "main.py:handler",
  source_sha256: SOURCE_SHA256,
  artifacts: [],
  created_at: "2026-07-29T10:00:09Z",
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
  build: readyBuild,
  logs: "$ compile Python source\n",
  started_at: "2026-07-29T10:00:01Z",
  finished_at: "2026-07-29T10:00:09Z",
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

export const unauthorized = {
  error: { code: "unauthorized", message: "your session expired; sign in again" },
};

/**
 * A catalogue shaped like the server's, cut down to what a form test needs: two
 * operating systems that disagree about which browsers and cards exist under
 * them.
 */
export const identityCatalog: IdentityCatalog = {
  operating_systems: [
    {
      os: "Windows",
      form_factor: "desktop",
      navigator_platform: "Win32",
      bitness: "64",
      architecture: "x86",
      browsers: ["chrome"],
      versions: [
        { os_version: "11", platform_versions: ["15.0.0"] },
        { os_version: "7", platform_versions: ["0.0.0"] },
      ],
      gpus: [
        {
          vendor: "Intel",
          webgl_renderer: "ANGLE (Intel(R) HD Graphics 620 Direct3D11 vs_5_0 ps_5_0)",
          webgl_vendor: "Google Inc. (Intel)",
        },
      ],
    },
    {
      os: "Macintosh",
      form_factor: "desktop",
      navigator_platform: "MacIntel",
      bitness: "64",
      architecture: "x86",
      browsers: ["chrome", "safari"],
      versions: [{ os_version: "14", platform_versions: ["14.6"] }],
      gpus: [
        { vendor: "Apple", webgl_renderer: "Apple GPU", webgl_vendor: "Apple Inc." },
      ],
    },
    {
      os: "Android",
      form_factor: "mobile",
      navigator_platform: "",
      bitness: "",
      architecture: "",
      browsers: ["chrome"],
      versions: [],
      gpus: [],
    },
  ],
  devices: [
    {
      name: "Samsung Galaxy S23",
      os: "Android",
      browsers: ["chrome"],
      models: ["SM-S911B"],
      versions: [{ os_version: "13", platform_versions: ["13.0.0"] }],
      navigator_platforms: ["Linux armv8l"],
      screen: {
        logical_width: 360,
        logical_height: 780,
        original_width: 1080,
        original_height: 2340,
        density_pixel_ratio: 3,
      },
      hardware_concurrency: [8],
      memory: [8],
      gpus: [
        {
          vendor: "Qualcomm",
          webgl_renderer: "ANGLE (Qualcomm, Adreno (TM) 740, OpenGL ES 3.2)",
          webgl_vendor: "Qualcomm",
        },
      ],
    },
  ],
  browsers: [
    { browser: "chrome", versions: ["139.0.6889.109"] },
    { browser: "safari", versions: ["18"] },
  ],
  screens: [{ width: 1920, height: 1080, share: 28.58 }],
  density_pixel_ratios: [1, 2],
  hardware_concurrency: [4, 8],
  memory: [4, 8],
  geos: [{ code: "US", languages: ["en-US", "en"], timezone: "America/New_York" }],
};
