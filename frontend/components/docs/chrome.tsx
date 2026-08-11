"use client";

import Link from "next/link";
import { useRouter, usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import {
  Activity,
  Box,
  ExternalLink,
  KeyRound,
  ListChecks,
  Play,
  Search,
  ServerCog,
  TriangleAlert,
  type LucideIcon,
} from "lucide-react";

import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { Button } from "@/components/ui/button";
import { Logo } from "@/components/neurun/logo";
import { ThemeToggle } from "@/components/neurun/theme-toggle";
import { DOCS, DOCS_NAV, DOCS_ORDER } from "@/lib/docs/content";
import { cn } from "@/lib/utils";

const ICONS: Record<string, LucideIcon> = {
  play: Play,
  box: Box,
  "list-checks": ListChecks,
  activity: Activity,
  "server-cog": ServerCog,
  "key-round": KeyRound,
  "triangle-alert": TriangleAlert,
};

export function DocsHeader() {
  const [open, setOpen] = useState(false);
  const router = useRouter();

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setOpen((value) => !value);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  return (
    <header className="sticky top-0 z-40 flex h-(--nr-nav-height) items-center gap-3.5 border-b border-line bg-[color-mix(in_srgb,var(--nr-surface-base)_92%,transparent)] px-5 backdrop-blur-[6px]">
      <Link href="/" aria-label="Neurun home" className="flex items-center">
        <Logo />
      </Link>
      <span className="nr-label border-l border-line-default pl-2.5">docs</span>
      <span className="nr-label ml-3 hidden border border-line-default px-1.5 py-0.5 sm:inline">
        api v1 · current
      </span>

      <button
        type="button"
        onClick={() => setOpen(true)}
        className="ml-auto flex h-7 w-56 items-center gap-2 rounded-md border border-line-default bg-surface-sunken pr-2 pl-2.5 text-meta text-fg-muted transition-colors duration-120 ease-mech hover:border-line-strong hover:text-fg-secondary max-md:w-auto"
      >
        <Search aria-hidden className="size-3.5" strokeWidth={1.5} />
        <span className="flex-1 text-left max-md:hidden">Search documentation</span>
        <kbd className="rounded-xs border border-line-default px-1 font-mono text-micro max-md:hidden">
          ⌘K
        </kbd>
      </button>

      <ThemeToggle />
      <Button asChild variant="ghost" size="sm" className="max-sm:hidden">
        <Link href="/auth">
          Sign in
          <ExternalLink aria-hidden strokeWidth={1.5} />
        </Link>
      </Button>

      <CommandDialog
        open={open}
        onOpenChange={setOpen}
        title="Search documentation"
        description="Jump to a page"
      >
        <CommandInput placeholder="Search pages and endpoints" />
        <CommandList>
          <CommandEmpty>No page matches that.</CommandEmpty>
          <CommandGroup heading="Pages">
            {DOCS_ORDER.map((slug) => {
              const page = DOCS[slug];
              return (
                <CommandItem
                  key={slug}
                  value={`${page.label} ${page.group}`}
                  onSelect={() => {
                    setOpen(false);
                    router.push(`/docs/${slug}`);
                  }}
                >
                  <span>{page.label}</span>
                  <span className="ml-auto font-mono text-micro text-fg-muted">{page.group}</span>
                </CommandItem>
              );
            })}
          </CommandGroup>
        </CommandList>
      </CommandDialog>
    </header>
  );
}

export function DocsNav() {
  const pathname = usePathname();

  return (
    <nav
      aria-label="Documentation"
      className="sticky top-(--nr-nav-height) hidden h-[calc(100dvh-var(--nr-nav-height))] w-(--nr-rail-width) shrink-0 overflow-y-auto border-r border-line py-5 lg:block"
    >
      {DOCS_NAV.map((section) => (
        <div key={section.label} className="mb-4 last:mb-0">
          <p className="nr-label px-4 pb-1.5">{section.label}</p>
          <ul>
            {section.items.map((item) => {
              const Icon = ICONS[item.icon] ?? Box;
              const active = pathname === `/docs/${item.slug}`;
              return (
                <li key={item.slug}>
                  <Link
                    href={`/docs/${item.slug}`}
                    aria-current={active ? "page" : undefined}
                    className={cn(
                      "relative flex h-(--nr-density-row) items-center gap-2.5 px-4 text-sm transition-colors duration-120 ease-mech",
                      active
                        ? "bg-surface-inset text-fg"
                        : "text-fg-secondary hover:bg-surface-hover hover:text-fg",
                    )}
                  >
                    {active ? (
                      <span aria-hidden className="absolute inset-y-0 left-0 w-px bg-line-inverse" />
                    ) : null}
                    <Icon aria-hidden className="size-4 shrink-0" strokeWidth={1.5} />
                    <span className="min-w-0 truncate">{item.label}</span>
                  </Link>
                </li>
              );
            })}
          </ul>
        </div>
      ))}
      <div className="mt-4 flex gap-1.5 px-4">
        <span className="nr-label rounded-xs border border-line-default px-1.5 py-0.5">
          openapi 3.1
        </span>
      </div>
    </nav>
  );
}

/**
 * On this page.
 *
 * The active heading is whichever `h2[id]` is highest in the viewport, so the
 * marker tracks reading position rather than the last anchor clicked.
 *
 * Mounted with a `key` of the page slug, so a route change remounts it rather
 * than needing the first heading reset back in on every `items` change.
 */
export function DocsToc({ items }: { items: { id: string; label: string }[] }) {
  const [active, setActive] = useState(items[0]?.id);

  useEffect(() => {
    const headings = items
      .map((item) => document.getElementById(item.id))
      .filter((element): element is HTMLElement => element !== null);
    if (!headings.length) return;

    const observer = new IntersectionObserver(
      (entries) => {
        const visible = entries
          .filter((entry) => entry.isIntersecting)
          .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top);
        if (visible[0]) setActive(visible[0].target.id);
      },
      { rootMargin: "-88px 0px -62% 0px" },
    );
    headings.forEach((heading) => observer.observe(heading));
    return () => observer.disconnect();
  }, [items]);

  return (
    <nav
      aria-label="On this page"
      className="sticky top-20 hidden w-52 shrink-0 flex-col gap-2 xl:flex"
    >
      <p className="nr-label">On this page</p>
      {items.map((item) => (
        <a
          key={item.id}
          href={`#${item.id}`}
          aria-current={active === item.id ? "location" : undefined}
          className={cn(
            "border-l py-1.5 pl-3 text-meta leading-[1.4] transition-colors duration-120 ease-mech",
            active === item.id
              ? "border-(--nr-accent) text-fg"
              : "border-line text-fg-muted hover:text-fg-secondary",
          )}
        >
          {item.label}
        </a>
      ))}
    </nav>
  );
}
