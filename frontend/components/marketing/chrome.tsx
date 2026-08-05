"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { ArrowRight } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Logo } from "@/components/neurun/logo";
import { ThemeToggle } from "@/components/neurun/theme-toggle";

const NAV = [
  { href: "/#model", label: "Model" },
  { href: "/#executions", label: "Executions" },
  { href: "/#capability", label: "Capability" },
  { href: "/#pricing", label: "Pricing" },
  { href: "/docs", label: "Docs" },
  { href: "/overview", label: "Dashboard" },
];

/**
 * The one place in the system that blurs. 88% base wash over 6px of backdrop
 * blur, plus a hairline that doubles as a scroll-progress readout.
 */
export function SiteHeader() {
  const [progress, setProgress] = useState(0);

  useEffect(() => {
    const onScroll = () => {
      const el = document.documentElement;
      const max = el.scrollHeight - el.clientHeight;
      setProgress(max > 8 ? Math.min(100, Math.max(0, (el.scrollTop / max) * 100)) : 0);
    };
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  return (
    <header className="sticky top-0 z-40 border-b border-line bg-[color-mix(in_srgb,var(--nr-surface-base)_88%,transparent)] backdrop-blur-[6px]">
      <div className="mx-auto flex h-(--nr-nav-height) max-w-(--nr-container-max) items-center gap-7 px-6">
        <Link href="/" aria-label="Neurun home" className="flex items-center">
          <Logo />
        </Link>

        <nav aria-label="Site" className="hidden flex-1 items-center gap-5.5 lg:flex">
          {NAV.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              className="text-sm text-fg-secondary transition-colors duration-120 ease-mech hover:text-fg"
            >
              {item.label}
            </Link>
          ))}
        </nav>

        <div className="ml-auto flex items-center gap-3 lg:ml-0">
          <Link
            href="/login"
            className="hidden text-sm text-fg-secondary transition-colors duration-120 ease-mech hover:text-fg sm:inline"
          >
            Sign in
          </Link>
          <ThemeToggle />
          <Button asChild size="sm">
            <Link href="/register">
              Start free
              <ArrowRight aria-hidden strokeWidth={1.5} />
            </Link>
          </Button>
        </div>
      </div>
      <div
        aria-hidden
        className="h-px bg-(--nr-accent) transition-[width] duration-120 ease-linear"
        style={{ width: `${progress}%` }}
      />
    </header>
  );
}

const FOOTER_COLUMNS = [
  {
    label: "Product",
    links: [
      { href: "/#model", label: "The model" },
      { href: "/#executions", label: "Executions" },
      { href: "/#capability", label: "Capability matrix" },
      { href: "/#pricing", label: "Pricing" },
      { href: "/overview", label: "Dashboard" },
    ],
  },
  {
    label: "Contracts",
    links: [
      { href: "/docs/quickstart", label: "Quickstart" },
      { href: "/docs/executions", label: "Executions" },
      { href: "/docs/authentication", label: "Authentication" },
      { href: "/docs/errors", label: "Error model" },
    ],
  },
  {
    label: "Company",
    links: [
      { href: "/login", label: "Sign in" },
      { href: "/docs/runners", label: "Runners" },
      { href: "mailto:sales@neurun.dev", label: "Talk to engineering" },
      { href: "mailto:security@neurun.dev", label: "Security" },
    ],
  },
];

export function SiteFooter() {
  return (
    <footer className="border-t border-line bg-surface-sunken">
      <div className="mx-auto w-full max-w-(--nr-container-max) px-6 pt-14 pb-8">
        <div className="grid gap-10 sm:grid-cols-2 lg:grid-cols-[minmax(0,2fr)_repeat(3,minmax(0,1fr))]">
          <div className="flex flex-col gap-3.5">
            <Logo />
            <p className="max-w-[280px] text-caption leading-[1.6] text-fg-muted">
              The execution and evidence plane for reliable web automation. Contract first, priced
              per compute.
            </p>
          </div>
          {FOOTER_COLUMNS.map((column) => (
            <div key={column.label} className="flex flex-col gap-2.5">
              <p className="nr-label">{column.label}</p>
              {column.links.map((link) => (
                <Link
                  key={link.href + link.label}
                  href={link.href}
                  className="text-caption text-fg-secondary transition-colors duration-120 ease-mech hover:text-fg"
                >
                  {link.label}
                </Link>
              ))}
            </div>
          ))}
        </div>

        <div className="mt-11 flex flex-wrap items-center gap-5 border-t border-line pt-5 font-mono text-micro text-fg-muted">
          <span>© {new Date().getFullYear()} Neurun, Inc.</span>
          <span className="hidden flex-1 sm:block" />
          <span>All rights reserved</span>
          <span>api v1</span>
          <span>0.1.0</span>
        </div>
      </div>
    </footer>
  );
}
