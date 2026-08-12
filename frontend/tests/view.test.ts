import { describe, expect, it } from "vitest";

import { describeStatus, isKnownStatus } from "@/lib/view/status";
import {
  formatBytes,
  formatBytesExact,
  formatDurationMs,
  formatNanoseconds,
  nanosecondsToMs,
  NO_VALUE,
} from "@/lib/view/units";
import { formatAbsolute, formatRelative, parseInstant } from "@/lib/view/time";
import { isSecretKey, redactSecrets, redactString, toRedactedJson } from "@/lib/view/redaction";
import {
  catalogDraft,
  identityChanged,
  fromDraft,
  gpusFor,
  withBrand,
  withGeo,
  withOS,
  withOSVersion,
  withRatio,
  versionMove,
} from "@/lib/view/browser-identity";
import type { BrowserProfile, IdentityCatalog } from "@/lib/api/resource-types";

describe("status legend", () => {
  it("classifies known states without relying on colour", () => {
    expect(describeStatus("succeeded")).toMatchObject({ treatment: "solid", tone: "success" });
    expect(describeStatus("running")).toMatchObject({ treatment: "pulse", tone: "active" });
    expect(describeStatus("queued")).toMatchObject({ treatment: "dashed" });
    expect(describeStatus("retry_wait")).toMatchObject({ treatment: "hatch", tone: "warning" });
    expect(describeStatus("failed")).toMatchObject({ treatment: "inverted", tone: "danger" });
    expect(describeStatus("canceled")).toMatchObject({ treatment: "strike", tone: "neutral" });
  });

  it("keeps a validation rejection distinct from a transport failure", () => {
    expect(describeStatus("rejected").treatment).toBe("rejected");
    expect(describeStatus("failed").treatment).toBe("inverted");
    expect(describeStatus("rejected").treatment).not.toBe(describeStatus("failed").treatment);
  });

  it("renders an unknown status neutrally, carrying its raw value", () => {
    const descriptor = describeStatus("quarantined");

    expect(descriptor.value).toBe("quarantined");
    expect(descriptor.known).toBe(false);
    expect(descriptor.treatment).toBe("neutral");
    // The failure mode that matters: never silently mapped onto success.
    expect(descriptor.tone).not.toBe("success");
  });

  it("survives a missing status", () => {
    expect(describeStatus(undefined).known).toBe(false);
    expect(describeStatus(null).treatment).toBe("neutral");
    expect(describeStatus("").treatment).toBe("neutral");
  });

  it("knows which values it knows", () => {
    expect(isKnownStatus("succeeded")).toBe(true);
    expect(isKnownStatus("quarantined")).toBe(false);
  });
});

describe("units", () => {
  it("converts Go nanosecond durations explicitly", () => {
    expect(nanosecondsToMs(1_000_000_000)).toBe(1000);
    expect(formatNanoseconds(1_000_000_000)).toBe("1.00s");
    expect(formatNanoseconds(30_000_000_000)).toBe("30.0s");
    // The retry policy in the job fixture, as it would render.
    expect(formatNanoseconds(500_000_000)).toBe("500ms");
  });

  it("formats millisecond fields as milliseconds", () => {
    expect(formatDurationMs(412)).toBe("412ms");
    expect(formatDurationMs(1_500)).toBe("1.50s");
    expect(formatDurationMs(65_000)).toBe("1m 5s");
    expect(formatDurationMs(3_900_000)).toBe("1h 5m");
  });

  it("uses binary byte units and preserves the exact count", () => {
    expect(formatBytes(0)).toBe("0 B");
    expect(formatBytes(1024)).toBe("1.00 KiB");
    expect(formatBytes(1_048_576)).toBe("1.00 MiB");
    expect(formatBytesExact(2048)).toBe("2.00 KiB (2,048 bytes)");
  });

  it("distinguishes a missing metric from a measured zero", () => {
    expect(NO_VALUE).toBe("—");
    expect(formatBytes(0)).not.toBe(NO_VALUE);
    expect(formatDurationMs(Number.NaN)).toBe(NO_VALUE);
  });
});

describe("time", () => {
  it("parses RFC 3339 and rejects nonsense without throwing", () => {
    expect(parseInstant("2026-07-29T10:00:00Z")?.toISOString()).toBe("2026-07-29T10:00:00.000Z");
    expect(parseInstant("not a date")).toBeNull();
    expect(parseInstant(undefined)).toBeNull();
  });

  it("formats relative time against a supplied now", () => {
    const now = new Date("2026-07-29T10:00:00Z");
    expect(formatRelative(new Date("2026-07-29T09:59:00Z"), now)).toBe("1 minute ago");
    expect(formatRelative(new Date("2026-07-29T08:00:00Z"), now)).toBe("2 hours ago");
    expect(formatRelative(new Date("2026-07-29T10:00:02Z"), now)).toBe("just now");
  });

  it("names the zone on the exact value", () => {
    const instant = new Date("2026-07-29T10:00:00Z");
    expect(formatAbsolute(instant, "utc")).toBe("29/07/2026 10:00:00 UTC");
    expect(formatAbsolute(instant, "local")).not.toBe(formatAbsolute(instant, "utc").slice(0, -4));
  });
});

describe("secret redaction", () => {
  it("recognises secret-looking keys", () => {
    expect(isSecretKey("api_key")).toBe(true);
    expect(isSecretKey("password")).toBe(true);
    expect(isSecretKey("authorization")).toBe(true);
    expect(isSecretKey("message")).toBe(false);
    expect(isSecretKey("keyboard")).toBe(false);
  });

  it("redacts values under secret keys while preserving structure", () => {
    const redacted = redactSecrets({
      message: "hello",
      api_key: "neu_local_abc.supersecret",
      nested: { password: "hunter2", keep: 1 },
      list: [{ token: "abc" }],
    });

    expect(redacted).toEqual({
      message: "hello",
      api_key: "[redacted]",
      nested: { password: "[redacted]", keep: 1 },
      list: [{ token: "[redacted]" }],
    });
  });

  it("redacts an API key's secret half wherever it appears in a string", () => {
    expect(redactString("Authorization: Bearer neu_local_abc123.supersecret")).toBe(
      "Authorization: Bearer [redacted]",
    );
    expect(redactString("key is neu_prod_xyz789.deadbeefcafe here")).toBe(
      "key is neu_prod_xyz789.[redacted] here",
    );
  });

  it("tolerates cycles", () => {
    const cyclic: Record<string, unknown> = { name: "job" };
    cyclic.self = cyclic;
    expect(() => redactSecrets(cyclic)).not.toThrow();
  });

  it("produces redacted JSON for the copy action", () => {
    const copied = toRedactedJson({ input: { message: "hi" }, api_key: "neu_a_b.c" });
    expect(copied).toContain('"message": "hi"');
    expect(copied).not.toContain("neu_a_b.c");
  });
});

/** A catalogue shaped like the server's, small enough to reason about. */
const CATALOG: IdentityCatalog = {
  operating_systems: [
    {
      os: "Windows",
      form_factor: "desktop",
      navigator_platform: "Win32",
      bitness: "64",
      architecture: "x86",
      brands: ["chrome", "edge"],
      versions: [
        { os_version: "11", platform_versions: ["15.0.0", "14.0.0"] },
        { os_version: "7", platform_versions: ["0.0.0"] },
      ],
    },
    {
      os: "Macintosh",
      form_factor: "desktop",
      navigator_platform: "MacIntel",
      bitness: "64",
      architecture: "x86",
      brands: ["chrome", "safari"],
      versions: [{ os_version: "14", platform_versions: ["14.6"] }],
    },
    {
      os: "Android",
      form_factor: "mobile",
      navigator_platform: "",
      bitness: "",
      architecture: "",
      brands: ["chrome"],
      versions: [],
    },
  ],
  devices: [
    {
      name: "Samsung Galaxy S23",
      os: "Android",
      brands: ["chrome"],
      models: ["SM-S911B", "SM-S911U"],
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
          os: "Android",
          brands: ["chrome"],
          vendor: "Qualcomm",
          webgl_renderer: "Adreno 740",
          webgl_vendor: "Qualcomm",
        },
      ],
    },
  ],
  browsers: [
    { brand: "chrome", versions: ["139.0.6889.109", "138.0.0.0"] },
    { brand: "safari", versions: ["18"] },
    { brand: "edge", versions: ["138.0.3402.56"] },
  ],
  screens: [
    { width: 1920, height: 1080, share: 28.58 },
    { width: 1512, height: 982, share: 1.2 },
  ],
  density_pixel_ratios: [1, 2],
  gpus: [
    {
      os: "Windows",
      brands: ["chrome", "edge"],
      vendor: "Intel",
      webgl_renderer: "ANGLE (Intel(R) HD Graphics 620 Direct3D11 vs_5_0 ps_5_0)",
      webgl_vendor: "Google Inc. (Intel)",
    },
    {
      os: "Macintosh",
      brands: ["chrome"],
      vendor: "Intel",
      webgl_renderer: "Intel Iris OpenGL Engine",
      webgl_vendor: "Intel Inc.",
    },
    {
      os: "Macintosh",
      brands: ["safari"],
      vendor: "Apple",
      webgl_renderer: "Apple GPU",
      webgl_vendor: "Apple Inc.",
    },
  ],
  hardware_concurrency: [2, 4, 6, 8],
  memory: [2, 4, 8],
  geos: [
    { code: "US", languages: ["en-US", "en"], timezone: "America/New_York" },
    { code: "DE", languages: ["de-DE", "de", "en"], timezone: "Europe/Berlin" },
  ],
};

describe("identity bindings", () => {
  it("starts from a machine the catalogue actually describes", () => {
    const draft = catalogDraft(CATALOG);

    expect(draft).toMatchObject({
      os: "Windows",
      navigator_platform: "Win32",
      os_version: "11",
      platform_version: "15.0.0",
      brand: "chrome",
      browser_version: "139.0.6889.109",
      webgl_renderer: "ANGLE (Intel(R) HD Graphics 620 Direct3D11 vs_5_0 ps_5_0)",
      geo: "US",
      language: "en-US, en",
      timezone: "America/New_York",
    });
    // Physical pixels are derived, never typed.
    expect(draft.original_width).toBe("1920");
  });

  it("re-chooses everything the operating system owns", () => {
    const windows = withBrand(catalogDraft(CATALOG), CATALOG, "edge");
    expect(windows.brand).toBe("edge");

    const mac = withOS(windows, CATALOG, "Macintosh");

    expect(mac.navigator_platform).toBe("MacIntel");
    expect(mac.os_version).toBe("14");
    expect(mac.platform_version).toBe("14.6");
    // Edge does not run on the Mac catalogue, and a Direct3D card cannot.
    expect(mac.brand).toBe("chrome");
    expect(mac.webgl_renderer).toBe("Intel Iris OpenGL Engine");
    expect(mac.webgl_vendor).toBe("Intel Inc.");
  });

  it("keeps the release and what it reports as different strings", () => {
    const draft = withOSVersion(catalogDraft(CATALOG), CATALOG, "7");

    expect(draft.os_version).toBe("7");
    expect(draft.platform_version).toBe("0.0.0");
  });

  it("gives Safari the one WebGL pair it ever reports", () => {
    const mac = withOS(catalogDraft(CATALOG), CATALOG, "Macintosh");
    const safari = withBrand(mac, CATALOG, "safari");

    expect(safari.webgl_renderer).toBe("Apple GPU");
    expect(safari.webgl_vendor).toBe("Apple Inc.");
    expect(gpusFor(CATALOG, "Macintosh", "safari")).toHaveLength(1);
  });

  it("lets a country carry its language list and clock", () => {
    const draft = withGeo(catalogDraft(CATALOG), CATALOG, "DE");

    expect(draft.language).toBe("de-DE, de, en");
    expect(draft.timezone).toBe("Europe/Berlin");
  });

  it("scales the physical screen by the ratio", () => {
    const draft = withRatio(catalogDraft(CATALOG), "2");

    expect(draft.original_width).toBe("3840");
    expect(draft.original_height).toBe("2160");
  });

  it("lets the handset settle the phone in one move", () => {
    const phone = withOS(catalogDraft(CATALOG), CATALOG, "Android");

    expect(phone.device).toBe("Samsung Galaxy S23");
    expect(phone.device_model).toBe("SM-S911B");
    expect(phone.navigator_platform).toBe("Linux armv8l");
    // The release lives on the device, and reports itself padded.
    expect(phone.os_version).toBe("13");
    expect(phone.platform_version).toBe("13.0.0");
    // Screen, card, cores and memory shipped in one box.
    expect(phone.logical_width).toBe("360");
    expect(phone.original_width).toBe("1080");
    expect(phone.webgl_renderer).toBe("Adreno 740");
    expect(phone.memory).toBe("8");
    // A phone is battery-powered and touched, not moused.
    expect(phone).toMatchObject({ has_touch: true, has_mouse: false, has_battery: true });
  });

  it("offers only the cards the chosen handset carries", () => {
    expect(gpusFor(CATALOG, "Android", "chrome", "Samsung Galaxy S23")).toHaveLength(1);
    // Without a handset there is nothing coherent to offer.
    expect(gpusFor(CATALOG, "Android", "chrome")).toHaveLength(0);
  });

  it("drops the handset when the profile goes back to a desktop", () => {
    const phone = withOS(catalogDraft(CATALOG), CATALOG, "Android");
    const desktop = withOS(phone, CATALOG, "Windows");

    expect(desktop.device).toBe("");
    expect(desktop.device_model).toBe("");
    expect(desktop.navigator_platform).toBe("Win32");
    expect(desktop.webgl_renderer).toContain("Direct3D11");
  });

  it("parses the text back into the record the API takes", () => {
    const identity = fromDraft(withGeo(catalogDraft(CATALOG), CATALOG, "DE"));

    expect(identity.browser_version).toEqual([139, 0, 6889, 109]);
    expect(identity.language).toEqual(["de-DE", "de", "en"]);
    expect(identity.screen.logical_width).toBe(1920);
    expect(identity.platform.navigator_platform).toBe("Win32");
  });
});

describe("fingerprint and version guards", () => {
  const identity: NonNullable<BrowserProfile["identity"]> = {
    os: "Windows",
    os_version: "11",
    platform: { navigator_platform: "Win32", version: "15.0.0" },
    brand: "chrome",
    browser_version: [139, 0, 6889, 109],
    screen: {
      logical_width: 1920,
      logical_height: 1080,
      original_width: 1920,
      original_height: 1080,
      density_pixel_ratio: 1,
    },
    hardware_concurrency: 8,
    memory: 8,
    gpu: {
      vendor: "Intel Inc.",
      webgl_renderer: "ANGLE (Intel)",
      webgl_vendor: "Google Inc. (Intel)",
    },
    geo: "US",
    language: ["en-US", "en"],
    has_battery: false,
    has_mouse: true,
    has_touch: false,
    proxy_set: false,
  };

  it("sees a dirty identity, whatever moved", () => {
    expect(identityChanged(identity, identity)).toBe(false);
    expect(identityChanged(identity, { ...identity, memory: 4 })).toBe(true);
    expect(identityChanged(identity, { ...identity, geo: "DE" })).toBe(true);
    expect(
      identityChanged(identity, { ...identity, gpu: { ...identity.gpu, vendor: "Apple" } }),
    ).toBe(true);
  });

  it("ignores the two fields that cannot be compared honestly", () => {
    // The API never returns the proxy, and the version has its own rule.
    expect(identityChanged(identity, { ...identity, proxy_set: true })).toBe(false);
    expect(
      identityChanged(identity, { ...identity, browser_version: [140, 0, 0, 0] }),
    ).toBe(false);
  });

  it("treats gaining or losing an identity as a change", () => {
    expect(identityChanged(null, identity)).toBe(true);
    expect(identityChanged(identity, null)).toBe(true);
    expect(identityChanged(null, null)).toBe(false);
  });

  it("tells a browser update apart from a downgrade", () => {
    const at = (version: number[]) => ({ ...identity, browser_version: version });

    expect(versionMove(identity, at([140, 0, 7339, 80]))).toBe("forward");
    expect(versionMove(identity, at([139, 0, 6889, 200]))).toBe("forward");
    expect(versionMove(identity, at([139, 0, 6889, 109]))).toBe("same");
    // A machine does not downgrade its own browser.
    expect(versionMove(identity, at([138, 0, 3402, 56]))).toBe("backward");
    expect(versionMove(identity, at([139, 0, 6889, 1]))).toBe("backward");
    expect(versionMove(identity, null)).toBe("none");
  });
});
