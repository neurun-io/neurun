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
    label: "Execution",
    items: [
      { href: "/", label: "Overview", icon: "layout-dashboard", availability: "future" },
      { href: "/jobs", label: "Jobs", icon: "list-checks", availability: "current" },
      { href: "/invocations", label: "Invocations", icon: "activity", availability: "current" },
      { href: "/functions", label: "Functions", icon: "box", availability: "current" },
      { href: "/fetch", label: "HTTP fetch", icon: "globe", availability: "current" },
    ],
  },
  {
    label: "Fleet",
    items: [
      { href: "/sessions", label: "Sessions", icon: "monitor", availability: "future" },
      { href: "/proxies", label: "Proxies", icon: "route", availability: "future" },
      { href: "/agents", label: "Agents", icon: "server", availability: "future" },
    ],
  },
  {
    label: "Settings",
    items: [
      { href: "/settings/projects", label: "Projects", icon: "folder", availability: "future" },
      { href: "/settings/api-keys", label: "API keys", icon: "key", availability: "future" },
      { href: "/settings/identities", label: "Identities", icon: "fingerprint", availability: "future" },
      { href: "/settings/profiles", label: "Profiles", icon: "user-round", availability: "future" },
      { href: "/settings/webhooks", label: "Webhooks", icon: "webhook", availability: "future" },
      { href: "/settings/audit", label: "Audit", icon: "scroll-text", availability: "future" },
    ],
  },
];

/** Is `pathname` inside `href`? Exact match for `/`, prefix match otherwise. */
export function isActiveRoute(pathname: string, href: string): boolean {
  if (href === "/") return pathname === "/";
  return pathname === href || pathname.startsWith(`${href}/`);
}

/**
 * Route an operator-pasted identifier to its detail page.
 *
 * The prefixes are the ones the contract declares: `job_`, `fni_`. Anything
 * else is not guessed at — a wrong guess sends the operator to a 404 and wastes
 * their time.
 */
export function routeForIdentifier(raw: string): string | null {
  const value = raw.trim();
  if (!value) return null;
  if (value.startsWith("job_")) return `/jobs/${value}`;
  if (value.startsWith("fni_")) return `/invocations/${value}`;
  return null;
}
