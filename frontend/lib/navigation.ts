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
      { href: "/deployments", label: "Deployments", icon: "box", availability: "current" },
      { href: "/builds", label: "Builds", icon: "list-checks", availability: "current" },
      { href: "/executions", label: "Executions", icon: "activity", availability: "current" },
    ],
  },
  {
    label: "Access",
    items: [
      { href: "/users", label: "Users", icon: "user-round", availability: "current" },
      { href: "/api-keys", label: "API keys", icon: "key", availability: "current" },
    ],
  },
  {
    label: "Roadmap",
    items: [
      { href: "/", label: "Overview", icon: "layout-dashboard", availability: "future" },
      { href: "/browsers", label: "Browsers", icon: "globe", availability: "future" },
      { href: "/sessions", label: "Sessions", icon: "monitor", availability: "future" },
      { href: "/proxies", label: "Proxies", icon: "route", availability: "future" },
    ],
  },
  {
    label: "Roadmap settings",
    items: [
      {
        href: "/settings/projects",
        label: "Project settings",
        icon: "folder",
        availability: "future",
      },
      {
        href: "/settings/api-keys",
        label: "API key settings",
        icon: "key",
        availability: "future",
      },
      {
        href: "/settings/identities",
        label: "Identities",
        icon: "fingerprint",
        availability: "future",
      },
      {
        href: "/settings/profiles",
        label: "Profiles",
        icon: "user-round",
        availability: "future",
      },
      {
        href: "/settings/webhooks",
        label: "Webhooks",
        icon: "webhook",
        availability: "future",
      },
      {
        href: "/settings/audit",
        label: "Audit",
        icon: "scroll-text",
        availability: "future",
      },
      {
        href: "/settings/activity",
        label: "Activity",
        icon: "activity",
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

/**
 * Route an operator-pasted identifier to its detail page.
 *
 * The prefixes are the ones the focused contract declares. Anything
 * else is not guessed at — a wrong guess sends the operator to a 404 and wastes
 * their time.
 */
export function routeForIdentifier(raw: string): string | null {
  const value = raw.trim();
  if (!value) return null;
  if (value.startsWith("prj_")) return `/projects/${value}`;
  if (value.startsWith("app_")) return `/apps/${value}`;
  if (value.startsWith("dep_")) return `/deployments/${value}`;
  if (value.startsWith("bld_")) return `/builds/${value}`;
  if (value.startsWith("exe_")) return `/executions/${value}`;
  return null;
}
