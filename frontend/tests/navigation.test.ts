import { describe, expect, it } from "vitest";

import { NAV_SECTIONS } from "@/lib/navigation";

describe("focused navigation", () => {
  it("contains the project-owned app hierarchy", () => {
    expect(
      NAV_SECTIONS.flatMap((section) => section.items)
        .filter((item) => item.availability === "current")
        .map((item) => item.label),
    ).toEqual([
      "Projects",
      "Apps",
      "Deployments",
      "Builds",
      "Executions",
      "Browser profiles",
      "Users",
      "API keys",
      "Organization",
    ]);
  });

  it("keeps the restored frontend-only roadmap visible", () => {
    expect(
      NAV_SECTIONS.flatMap((section) => section.items)
        .filter((item) => item.availability === "future")
        .map((item) => item.href),
    ).toEqual([
      "/overview",
      "/stealth",
      "/ai-builder",
      "/runners",
      "/proxies",
      "/data-health",
      "/settings/webhooks",
      "/settings/activity",
    ]);
  });
});
