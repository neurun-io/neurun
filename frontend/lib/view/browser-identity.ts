/**
 * The identity half of a browser profile, as a form holds it.
 *
 * Two jobs. Every field is text while it is being typed — a number input that is
 * mid-edit is an empty string, not a NaN, and the parse happens once on submit.
 * And the fields bind: the catalogue the server publishes says which releases,
 * brands and GPUs exist under an operating system, so choosing one narrows the
 * next rather than leaving twenty free-text boxes that can contradict each
 * other. A Direct3D renderer on a Mac is not a typo, it is a caught bot.
 */
import type {
  BrowserBrand,
  BrowserIdentity,
  BrowserProfile,
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
  brand: BrowserBrand;
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

export function brandsFor(catalog: IdentityCatalog, os: string): BrowserBrand[] {
  return osEntry(catalog, os)?.brands ?? [];
}

export function browserVersionsFor(catalog: IdentityCatalog, brand: string): string[] {
  return catalog.browsers.find((entry) => entry.brand === brand)?.versions ?? [];
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
 * offer.
 */
export function gpusFor(
  catalog: IdentityCatalog,
  os: string,
  brand: string,
  device?: string,
): CatalogGPU[] {
  if (isMobile(catalog, os)) {
    return device ? (deviceNamed(catalog, device)?.gpus ?? []) : [];
  }
  return catalog.gpus.filter((gpu) => gpu.os === os && gpu.brands.includes(brand as BrowserBrand));
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

/**
 * A brand fixes its own version list and, with the OS, which cards can report.
 * Safari on a Mac reports one Apple pair whatever silicon is underneath.
 */
export function withBrand(
  draft: IdentityDraft,
  catalog: IdentityCatalog,
  brand: BrowserBrand,
): IdentityDraft {
  const versions = browserVersionsFor(catalog, brand);
  const gpus = gpusFor(catalog, draft.os, brand);
  let next: IdentityDraft = {
    ...draft,
    brand,
    browser_version: versions.includes(draft.browser_version)
      ? draft.browser_version
      : (versions[0] ?? draft.browser_version),
  };
  if (gpus.length > 0 && !gpus.some((gpu) => gpu.webgl_renderer === next.webgl_renderer)) {
    next = withGPU(next, gpus[0]);
  }
  return next;
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
 * outright, and everything below it — release, brand, GPU — is re-chosen rather
 * than carried across, because carrying it across is how a Mac ends up claiming
 * Win32.
 */
export function withOS(
  draft: IdentityDraft,
  catalog: IdentityCatalog,
  os: string,
): IdentityDraft {
  const entry = osEntry(catalog, os);
  if (!entry) return draft;

  const brand = entry.brands.includes(draft.brand) ? draft.brand : entry.brands[0];

  if (entry.form_factor === "mobile") {
    const device = devicesFor(catalog, entry.os)[0];
    const phone: IdentityDraft = { ...draft, os: entry.os, brand };
    return device ? withDevice(phone, catalog, device.name) : phone;
  }

  const next: IdentityDraft = {
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
  return withBrand(
    withOSVersion(next, catalog, entry.versions[0]?.os_version ?? next.os_version),
    catalog,
    brand,
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
    brand: "chrome",
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
    brand: identity.brand ?? "chrome",
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
    brand: draft.brand,
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
/* Summary                                                                     */
/* -------------------------------------------------------------------------- */

export interface SummaryRow {
  label: string;
  value: string;
}

/**
 * The profile as a reader scans it, and the basis for what a save changed. Every
 * value is a string so two versions of a profile can be compared by reading the
 * same rows a person reads, rather than by walking the payload.
 */
export function profileSummary(profile: BrowserProfile): SummaryRow[] {
  const identity = profile.identity;
  const rows: SummaryRow[] = [
    { label: "Name", value: profile.name },
    { label: "Browser", value: profile.browser },
    {
      label: "Identity",
      value: identity
        ? `${identity.brand} on ${identity.os} ${identity.os_version ?? ""}`.trim()
        : "Plain browser",
    },
  ];

  if (identity) {
    rows.push(
      { label: "Device", value: identity.device_model?.trim() || "Desktop or laptop" },
      {
        label: "Platform",
        value: [identity.platform?.navigator_platform, identity.platform?.version]
          .filter(Boolean)
          .join(" "),
      },
      { label: "Browser version", value: (identity.browser_version ?? []).join(".") },
      {
        label: "Screen",
        value: `${text(identity.screen?.logical_width)}×${text(identity.screen?.logical_height)} @${text(identity.screen?.density_pixel_ratio)}`,
      },
      {
        label: "Hardware",
        value: `${text(identity.hardware_concurrency)} cores · ${text(identity.memory)} GiB`,
      },
      { label: "GPU", value: identity.gpu?.webgl_renderer ?? "" },
      { label: "Locale", value: `${identity.geo} · ${(identity.language ?? []).join(", ")}` },
      {
        label: "Timezone",
        value: identity.timezone?.trim() || "Resolved through the proxy",
      },
      { label: "Proxy", value: identity.proxy_set ? "Set" : "None" },
    );
  }

  rows.push(
    { label: "Cookies", value: String(profile.cookies.length) },
    { label: "Storage origins", value: String(profile.storage_origins.length) },
  );
  return rows;
}

/** Which summary rows a save actually moved. An empty list means nothing did. */
export function changedRows(before: BrowserProfile, after: BrowserProfile): string[] {
  const previous = new Map(profileSummary(before).map((row) => [row.label, row.value]));
  return profileSummary(after)
    .filter((row) => previous.get(row.label) !== row.value)
    .map((row) => row.label);
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
