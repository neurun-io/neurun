/**
 * The dashboard's information architecture.
 *
 * `availability` is not decoration. Routes marked `future` have no backing
 * contract in the current OpenAPI; they render an honest unavailable page that
 * names the endpoints still required. Nothing in the navigation implies a
 * capability works before it does.
 */

export type RouteAvailability = "current" | "future";

export interface NavItem {
  href: string;
  label: string;
  /** Lucide icon name. */
  icon: string;
  availability: RouteAvailability;
}

export interface NavSection {
  label: string;
  items: NavItem[];
}

export const NAV_SECTIONS: NavSection[] = [
  {
    label: "Runtime",
    items: [
      { href: "/projects", label: "Projects", icon: "folder", availability: "current" },
      { href: "/apps", label: "Apps", icon: "server", availability: "current" },
      { href: "/deployments", label: "Deployments", icon: "rocket", availability: "current" },
      { href: "/builds", label: "Builds", icon: "list-checks", availability: "current" },
      { href: "/executions", label: "Executions", icon: "activity", availability: "current" },
      {
        href: "/browser-profiles",
        label: "Browser profiles",
        icon: "fingerprint",
        availability: "current",
      },
    ],
  },
  {
    label: "Access",
    items: [
      { href: "/users", label: "Users", icon: "user-round", availability: "current" },
      {
        href: "/users/activity",
        label: "Activity",
        icon: "scroll-text",
        availability: "future",
      },
      { href: "/api-keys", label: "API keys", icon: "key", availability: "current" },
      { href: "/organization", label: "Organization", icon: "building", availability: "current" },
    ],
  },
  {
    label: "Roadmap",
    items: [
      { href: "/overview", label: "Overview", icon: "layout-dashboard", availability: "future" },
      {
        href: "/stealth",
        label: "AI stealth coherence",
        icon: "radar",
        availability: "future",
      },
      {
        href: "/ai-builder",
        label: "AI automation builder",
        icon: "sparkles",
        availability: "future",
      },
      { href: "/servers", label: "Servers", icon: "server-cog", availability: "future" },
      { href: "/proxies", label: "Proxies", icon: "route", availability: "future" },
      {
        href: "/environment-variables",
        label: "Environment variables",
        icon: "sliders-horizontal",
        availability: "future",
      },
      {
        href: "/data-health",
        label: "Data health",
        icon: "git-compare",
        availability: "future",
      },
    ],
  },
];

/** Is `pathname` inside `href`? Exact match for `/`, prefix match otherwise. */
export function isActiveRoute(pathname: string, href: string): boolean {
  if (href === "/") return pathname === "/";
  return pathname === href || pathname.startsWith(`${href}/`);
}

