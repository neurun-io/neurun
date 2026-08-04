"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  Activity,
  Box,
  Fingerprint,
  Folder,
  Globe,
  Key,
  LayoutDashboard,
  ListChecks,
  Monitor,
  Route as RouteIcon,
  ScrollText,
  UserRound,
  Webhook,
  type LucideIcon,
} from "lucide-react";

import { cn } from "@/lib/utils";
import { NAV_SECTIONS, isActiveRoute } from "@/lib/navigation";

const ICONS: Record<string, LucideIcon> = {
  "layout-dashboard": LayoutDashboard,
  "list-checks": ListChecks,
  activity: Activity,
  box: Box,
  globe: Globe,
  monitor: Monitor,
  route: RouteIcon,
  folder: Folder,
  key: Key,
  fingerprint: Fingerprint,
  "user-round": UserRound,
  webhook: Webhook,
  "scroll-text": ScrollText,
};

export function SideNav({ className }: { className?: string }) {
  const pathname = usePathname();

  return (
    <nav
      aria-label="Dashboard sections"
      className={cn(
        "w-(--nr-rail-width) shrink-0 overflow-y-auto border-r border-line bg-surface-sunken py-3",
        className,
      )}
    >
      {NAV_SECTIONS.map((section) => (
        <div key={section.label} className="mb-4 last:mb-0">
          <p className="nr-label px-4 pb-1.5">{section.label}</p>
          <ul>
            {section.items.map((item) => {
              const Icon = ICONS[item.icon] ?? Box;
              const active = isActiveRoute(pathname, item.href);
              const unavailable = item.availability === "future";

              return (
                <li key={item.href}>
                  <Link
                    href={item.href}
                    aria-current={active ? "page" : undefined}
                    className={cn(
                      "relative flex h-(--nr-density-row) items-center gap-2.5 px-4 text-sm",
                      "transition-colors duration-120 ease-mech",
                      active
                        ? "bg-surface-inset text-fg"
                        : "text-fg-secondary hover:bg-surface-hover hover:text-fg",
                    )}
                  >
                    {/* Active nav item: inset fill plus a 1px leading bar. */}
                    {active ? (
                      <span aria-hidden className="absolute inset-y-0 left-0 w-px bg-line-inverse" />
                    ) : null}
                    <Icon aria-hidden className="size-4 shrink-0" strokeWidth={1.5} />
                    <span className="min-w-0 truncate">{item.label}</span>
                    {unavailable ? (
                      <span
                        aria-hidden
                        title="Not available in this release"
                        className="ml-auto size-1.5 shrink-0 rounded-full border border-dashed border-line-strong"
                      />
                    ) : null}
                    {unavailable ? (
                      <span className="sr-only">— not available in this release</span>
                    ) : null}
                  </Link>
                </li>
              );
            })}
          </ul>
        </div>
      ))}
    </nav>
  );
}
