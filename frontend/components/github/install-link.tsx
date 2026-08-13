"use client";

import { ExternalLink, Plus } from "lucide-react";

import { Button } from "@/components/ui/button";

/**
 * The App's slug, which its install URL is keyed by — it is not derivable from
 * the numeric App ID the server holds, so the browser is told separately.
 */
const APP_SLUG = process.env.NEXT_PUBLIC_NEURUN_GITHUB_APP_SLUG ?? "";

/**
 * Sends the operator to GitHub to install the App or change which repositories
 * it grants. GitHub routes an existing installation to its configure page, so
 * one URL covers both.
 */
export function InstallLink({
  label,
  variant,
}: {
  label: string;
  variant?: "secondary";
}) {
  if (!APP_SLUG) {
    return (
      <p className="text-sm text-fg-muted">
        Set <span className="font-mono text-caption">NEXT_PUBLIC_NEURUN_GITHUB_APP_SLUG</span> to
        the app&apos;s slug to install from here.
      </p>
    );
  }
  return (
    <Button asChild size="sm" variant={variant}>
      <a
        href={`https://github.com/apps/${APP_SLUG}/installations/new`}
        target="_blank"
        rel="noreferrer noopener"
      >
        {variant ? <ExternalLink aria-hidden /> : <Plus aria-hidden />}
        {label}
      </a>
    </Button>
  );
}
