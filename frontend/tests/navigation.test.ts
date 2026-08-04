import { describe, expect, it } from "vitest";

import { NAV_SECTIONS, routeForIdentifier } from "@/lib/navigation";

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
      "Users",
      "API keys",
    ]);
  });

  it("keeps the restored frontend-only roadmap visible", () => {
    expect(
      NAV_SECTIONS.flatMap((section) => section.items)
        .filter((item) => item.availability === "future")
        .map((item) => item.href),
    ).toEqual([
      "/",
      "/browsers",
      "/sessions",
      "/proxies",
      "/settings/projects",
      "/settings/api-keys",
      "/settings/identities",
      "/settings/profiles",
      "/settings/webhooks",
      "/settings/audit",
      "/settings/activity",
    ]);
  });

  it("routes focused resource identifiers", () => {
    expect(routeForIdentifier("prj_123")).toBe("/projects/prj_123");
    expect(routeForIdentifier("app_123")).toBe("/apps/app_123");
    expect(routeForIdentifier("dep_123")).toBe("/deployments/dep_123");
    expect(routeForIdentifier("bld_123")).toBe("/builds/bld_123");
    expect(routeForIdentifier("exe_123")).toBe("/executions/exe_123");
    expect(routeForIdentifier("job_123")).toBeNull();
  });
});
