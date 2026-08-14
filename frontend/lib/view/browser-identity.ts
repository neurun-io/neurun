/**
 * The identity half of a browser profile, as a form holds it.
 *
 * Two jobs. Every field is text while it is being typed — a number input that is
 * mid-edit is an empty string, not a NaN, and the parse happens once on submit.
 * And the fields bind: the catalogue the server publishes says which releases,
 * browsers and cards exist under an operating system, so choosing one narrows
 * the next rather than leaving twenty free-text boxes that can contradict each
 * other. A Direct3D renderer on a Mac is not a typo, it is a caught bot.
 */
import type {
  BrowserIdentity,
  BrowserKind,
  CatalogDevice,
  CatalogGPU,
  CatalogOS,
  CatalogOSVersion,
  IdentityCatalog,
  RedactedBrowserIdentity,
} from "@/lib/api/resource-types";

export type IdentityOS = BrowserIdentity["os"];

export interface IdentityDraft {
  os: IdentityOS;
  /** The handset, on mobile. Form state: the record carries its model, not it. */
  device: string;
  os_version: string;
  device_model: string;
  navigator_platform: string;
  platform_version: string;
  bitness: string;
  architecture: string;
  browser: BrowserKind;
  browser_version: string;
  logical_width: string;
  logical_height: string;
  original_width: string;
  original_height: string;
  density_pixel_ratio: string;
  hardware_concurrency: string;
  memory: string;
  gpu_vendor: string;
  webgl_renderer: string;
  webgl_vendor: string;
  geo: string;
  language: string;
  timezone: string;
  history_count: string;
  proxy: string;
  has_battery: boolean;
  has_mouse: boolean;
  has_touch: boolean;
}

/* -------------------------------------------------------------------------- */
/* Catalogue lookups                                                           */
/* -------------------------------------------------------------------------- */

export function osEntry(catalog: IdentityCatalog, os: string): CatalogOS | undefined {
  return catalog.operating_systems.find((entry) => entry.os === os);
}

export function osVersionsFor(
  catalog: IdentityCatalog,
  os: string,
  device?: string,
): string[] {
  return releasesFor(catalog, os, device).map((version) => version.os_version);
}

/** What the release actually reports to UA-CH, which is a different string. */
export function platformVersionsFor(
  catalog: IdentityCatalog,
  os: string,
  osVersion: string,
  device?: string,
): string[] {
  return (
    releasesFor(catalog, os, device).find((version) => version.os_version === osVersion)
      ?.platform_versions ?? []
  );
}

export function browsersFor(catalog: IdentityCatalog, os: string): BrowserKind[] {
  return osEntry(catalog, os)?.browsers ?? [];
}

export function browserVersionsFor(catalog: IdentityCatalog, browser: string): string[] {
  return catalog.browsers.find((entry) => entry.browser === browser)?.versions ?? [];
}

export function isMobile(catalog: IdentityCatalog, os: string): boolean {
  return osEntry(catalog, os)?.form_factor === "mobile";
}

export function devicesFor(catalog: IdentityCatalog, os: string): CatalogDevice[] {
  return catalog.devices.filter((device) => device.os === os);
}

export function deviceNamed(
  catalog: IdentityCatalog,
  name: string,
): CatalogDevice | undefined {
  return catalog.devices.find((device) => device.name === name);
}

/** Which handset ships a given Sec-CH-UA-Model code. */
export function deviceForModel(
  catalog: IdentityCatalog,
  model: string | undefined,
): string | undefined {
  if (!model) return undefined;
  return catalog.devices.find((device) => device.models.includes(model))?.name;
}

/**
 * On a phone the handset owns the card list; on a desktop the OS does. Either
 * way the answer is bound — a renderer the platform cannot report is not on
 * offer, because it is not in that platform's list at all.
 */
export function gpusFor(
  catalog: IdentityCatalog,
  os: string,
  device?: string,
): CatalogGPU[] {
  if (isMobile(catalog, os)) {
    return device ? (deviceNamed(catalog, device)?.gpus ?? []) : [];
  }
  return osEntry(catalog, os)?.gpus ?? [];
}

/** Releases come from the handset on mobile and from the system on a desktop. */
export function releasesFor(
  catalog: IdentityCatalog,
  os: string,
  device?: string,
): CatalogOSVersion[] {
  if (isMobile(catalog, os)) {
    return device ? (deviceNamed(catalog, device)?.versions ?? []) : [];
  }
  return osEntry(catalog, os)?.versions ?? [];
}

/* -------------------------------------------------------------------------- */
/* Bindings                                                                    */
/* -------------------------------------------------------------------------- */

/** Physical pixels follow from the logical screen and the ratio; never typed. */
function derive(draft: IdentityDraft): IdentityDraft {
  const ratio = Number(draft.density_pixel_ratio) || 1;
  return {
    ...draft,
    original_width: String(Math.round((Number(draft.logical_width) || 0) * ratio)),
    original_height: String(Math.round((Number(draft.logical_height) || 0) * ratio)),
  };
}

export function withGPU(draft: IdentityDraft, gpu: CatalogGPU): IdentityDraft {
  return {
    ...draft,
    gpu_vendor: gpu.vendor,
    webgl_renderer: gpu.webgl_renderer,
    webgl_vendor: gpu.webgl_vendor,
  };
}

/** A browser fixes its own version list; nothing else moves with it. */
export function withBrowser(
  draft: IdentityDraft,
  catalog: IdentityCatalog,
  browser: BrowserKind,
): IdentityDraft {
  const versions = browserVersionsFor(catalog, browser);
  return {
    ...draft,
    browser,
    browser_version: versions.includes(draft.browser_version)
      ? draft.browser_version
      : (versions[0] ?? draft.browser_version),
  };
}

export function withOSVersion(
  draft: IdentityDraft,
  catalog: IdentityCatalog,
  osVersion: string,
): IdentityDraft {
  const platformVersions = platformVersionsFor(catalog, draft.os, osVersion, draft.device);
  return {
    ...draft,
    os_version: osVersion,
    platform_version: platformVersions[0] ?? draft.platform_version,
  };
}

/**
 * A handset settles almost everything at once. Its screen, ratio, cores, memory
 * and card shipped together, so they are applied together rather than offered as
 * five independent choices that could describe no phone ever built.
 */
export function withDevice(
  draft: IdentityDraft,
  catalog: IdentityCatalog,
  name: string,
): IdentityDraft {
  const device = deviceNamed(catalog, name);
  if (!device) return { ...draft, device: name };

  const next: IdentityDraft = {
    ...draft,
    device: device.name,
    device_model: device.models[0] ?? "",
    navigator_platform: device.navigator_platforms[0] ?? "",
    bitness: "",
    architecture: "",
    logical_width: String(device.screen.logical_width),
    logical_height: String(device.screen.logical_height),
    original_width: String(device.screen.original_width),
    original_height: String(device.screen.original_height),
    density_pixel_ratio: String(device.screen.density_pixel_ratio),
    hardware_concurrency: String(device.hardware_concurrency[0] ?? draft.hardware_concurrency),
    memory: String(device.memory[0] ?? draft.memory),
    // A phone is battery-powered and touched, not moused.
    has_battery: true,
    has_mouse: false,
    has_touch: true,
  };

  const withCard = device.gpus[0] ? withGPU(next, device.gpus[0]) : next;
  return withOSVersion(withCard, catalog, device.versions[0]?.os_version ?? withCard.os_version);
}

/**
 * The widest binding. An OS fixes navigator.platform, bitness and architecture
 * outright, and everything below it — release, browser, GPU — is re-chosen
 * rather than carried across, because carrying it across is how a Mac ends up
 * claiming Win32.
 */
export function withOS(
  draft: IdentityDraft,
  catalog: IdentityCatalog,
  os: string,
): IdentityDraft {
  const entry = osEntry(catalog, os);
  if (!entry) return draft;

  const browser = entry.browsers.includes(draft.browser) ? draft.browser : entry.browsers[0];

  if (entry.form_factor === "mobile") {
    const device = devicesFor(catalog, entry.os)[0];
    const phone: IdentityDraft = { ...draft, os: entry.os, browser };
    return device ? withDevice(phone, catalog, device.name) : phone;
  }

  let next: IdentityDraft = {
    ...draft,
    os: entry.os,
    device: "",
    device_model: "",
    navigator_platform: entry.navigator_platform,
    bitness: entry.bitness,
    architecture: entry.architecture,
    has_touch: false,
    has_mouse: true,
  };
  // The card list is the system's, so a renderer from the last one cannot stay.
  const gpus = gpusFor(catalog, entry.os);
  if (gpus.length > 0 && !gpus.some((gpu) => gpu.webgl_renderer === next.webgl_renderer)) {
    next = withGPU(next, gpus[0]);
  }
  return withBrowser(
    withOSVersion(next, catalog, entry.versions[0]?.os_version ?? next.os_version),
    catalog,
    browser,
  );
}

/** A country implies a language list and a clock. Disagreeing with either is a tell. */
export function withGeo(
  draft: IdentityDraft,
  catalog: IdentityCatalog,
  code: string,
): IdentityDraft {
  const geo = catalog.geos.find((entry) => entry.code === code);
  if (!geo) return { ...draft, geo: code };
  return { ...draft, geo: code, language: geo.languages.join(", "), timezone: geo.timezone };
}

export function withScreen(draft: IdentityDraft, width: number, height: number): IdentityDraft {
  return derive({ ...draft, logical_width: String(width), logical_height: String(height) });
}

export function withRatio(draft: IdentityDraft, ratio: string): IdentityDraft {
  return derive({ ...draft, density_pixel_ratio: ratio });
}

/** The most ordinary machine the catalogue describes: a starting point to edit. */
export function catalogDraft(catalog: IdentityCatalog): IdentityDraft {
  const screen = catalog.screens[0];
  const base: IdentityDraft = {
    os: "Windows",
    device: "",
    os_version: "",
    device_model: "",
    navigator_platform: "",
    platform_version: "",
    bitness: "",
    architecture: "",
    browser: "chrome",
    browser_version: "",
    logical_width: String(screen?.width ?? 1920),
    logical_height: String(screen?.height ?? 1080),
    original_width: "",
    original_height: "",
    density_pixel_ratio: String(catalog.density_pixel_ratios[0] ?? 1),
    hardware_concurrency: String(catalog.hardware_concurrency[3] ?? 8),
    memory: String(catalog.memory[catalog.memory.length - 1] ?? 8),
    gpu_vendor: "",
    webgl_renderer: "",
    webgl_vendor: "",
    geo: "",
    language: "",
    timezone: "",
    history_count: "",
    proxy: "",
    has_battery: false,
    has_mouse: true,
    has_touch: false,
  };

  const system = catalog.operating_systems[0];
  const geo = catalog.geos[0];
  return derive(
    withGeo(withOS(base, catalog, system?.os ?? base.os), catalog, geo?.code ?? "US"),
  );
}

/* -------------------------------------------------------------------------- */
/* Draft ↔ identity                                                            */
/* -------------------------------------------------------------------------- */

/**
 * A draft from whatever the API returned. The boundary schema is loose, so a
 * field the server has stopped sending arrives as undefined rather than as a
 * crash, and lands here as an empty input.
 */
export function toDraft(
  identity: BrowserIdentity | RedactedBrowserIdentity,
  catalog?: IdentityCatalog,
): IdentityDraft {
  return {
    os: identity.os ?? "Windows",
    // The record stores a model, not a handset; the catalogue says which phone
    // shipped that model, so an edit reopens on the device it was built from.
    device: catalog ? (deviceForModel(catalog, identity.device_model) ?? "") : "",
    os_version: identity.os_version ?? "",
    device_model: identity.device_model ?? "",
    navigator_platform: identity.platform?.navigator_platform ?? "",
    platform_version: identity.platform?.version ?? "",
    bitness: identity.platform?.bitness ?? "",
    architecture: identity.platform?.architecture ?? "",
    browser: identity.browser ?? "chrome",
    browser_version: (identity.browser_version ?? []).join("."),
    logical_width: text(identity.screen?.logical_width),
    logical_height: text(identity.screen?.logical_height),
    original_width: text(identity.screen?.original_width),
    original_height: text(identity.screen?.original_height),
    density_pixel_ratio: text(identity.screen?.density_pixel_ratio),
    hardware_concurrency: text(identity.hardware_concurrency),
    memory: text(identity.memory),
    gpu_vendor: identity.gpu?.vendor ?? "",
    webgl_renderer: identity.gpu?.webgl_renderer ?? "",
    webgl_vendor: identity.gpu?.webgl_vendor ?? "",
    geo: identity.geo ?? "US",
    language: (identity.language ?? []).join(", "),
    timezone: identity.timezone ?? "",
    history_count: text(identity.history_count),
    // Never returned by the API, so an edit starts blank whatever is stored.
    proxy: "",
    has_battery: identity.has_battery ?? false,
    has_mouse: identity.has_mouse ?? true,
    has_touch: identity.has_touch ?? false,
  };
}

export function fromDraft(draft: IdentityDraft): BrowserIdentity {
  return {
    device_model: draft.device_model.trim(),
    has_battery: draft.has_battery,
    has_mouse: draft.has_mouse,
    has_touch: draft.has_touch,
    os: draft.os,
    os_version: draft.os_version.trim(),
    platform: {
      bitness: draft.bitness.trim(),
      architecture: draft.architecture.trim(),
      navigator_platform: draft.navigator_platform.trim(),
      version: draft.platform_version.trim(),
    },
    browser: draft.browser,
    browser_version: numbers(draft.browser_version),
    screen: {
      logical_width: number(draft.logical_width),
      logical_height: number(draft.logical_height),
      original_width: number(draft.original_width),
      original_height: number(draft.original_height),
      density_pixel_ratio: number(draft.density_pixel_ratio),
    },
    hardware_concurrency: number(draft.hardware_concurrency),
    memory: number(draft.memory),
    gpu: {
      vendor: draft.gpu_vendor.trim(),
      webgl_renderer: draft.webgl_renderer.trim(),
      webgl_vendor: draft.webgl_vendor.trim(),
    },
    geo: draft.geo,
    language: draft.language.split(",").map((item) => item.trim()).filter(Boolean),
    ...(draft.history_count.trim() ? { history_count: number(draft.history_count) } : {}),
    proxy: draft.proxy.trim(),
    timezone: draft.timezone.trim(),
  };
}

/* -------------------------------------------------------------------------- */
/* Guards                                                                      */
/* -------------------------------------------------------------------------- */

type SeedSource = BrowserIdentity | RedactedBrowserIdentity;

/**
 * A stable string for comparing two identities, skipping the three fields that
 * cannot be compared honestly: the proxy is never returned by the API, and the
 * browser version has its own directional rule below.
 */
function canonical(value: unknown): string {
  if (value === null || typeof value !== "object") return JSON.stringify(value) ?? "null";
  if (Array.isArray(value)) return `[${value.map(canonical).join(",")}]`;
  return `{${Object.entries(value as Record<string, unknown>)
    .filter(([key]) => key !== "proxy" && key !== "proxy_set" && key !== "browser_version")
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([key, entry]) => `${key}:${canonical(entry)}`)
    .join(",")}}`;
}

/**
 * Whether a save moves the identity at all.
 *
 * Every field of it feeds what a site sees — and the OS, release, screen, GPU,
 * cores, memory and platform feed the fingerprint seed itself, so the canvas,
 * audio and WebGL hashes move with them. A profile that keeps its cookies
 * through that presents one account from a computer replaced overnight.
 */
export function identityChanged(
  before: SeedSource | null | undefined,
  after: SeedSource | null | undefined,
): boolean {
  if (!before && !after) return false;
  if (!before || !after) return true;
  return canonical(before) !== canonical(after);
}

/**
 * How the browser version moved.
 *
 * Forward is what a real install does on its own — browsers update themselves,
 * and a version that never moves is its own weak signal. Backward is not: a
 * machine does not downgrade its browser, so a profile whose version regressed
 * is claiming something that does not happen.
 */
export function versionMove(
  before: SeedSource | null | undefined,
  after: SeedSource | null | undefined,
): "none" | "same" | "forward" | "backward" {
  const left = before?.browser_version;
  const right = after?.browser_version;
  if (!left?.length || !right?.length) return "none";

  for (let index = 0; index < Math.max(left.length, right.length); index += 1) {
    const difference = (right[index] ?? 0) - (left[index] ?? 0);
    if (difference > 0) return "forward";
    if (difference < 0) return "backward";
  }
  return "same";
}

function text(value: number | null | undefined): string {
  return value === undefined || value === null ? "" : String(value);
}

function number(value: string): number {
  const parsed = Number(value.trim());
  return Number.isFinite(parsed) ? parsed : 0;
}

/** `140.0.7339.80`, or the same digits separated by commas or spaces. */
function numbers(value: string): number[] {
  return value
    .split(/[.,\s]+/)
    .filter(Boolean)
    .map((part) => Math.trunc(Number(part)))
    .filter((part) => Number.isFinite(part));
}
