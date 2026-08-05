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
