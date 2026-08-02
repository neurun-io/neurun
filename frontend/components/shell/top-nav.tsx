"use client";

import { useState, type FormEvent } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { ChevronDown, LogOut, Search } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Wordmark } from "@/components/neurun/logo";
import { ThemeToggle } from "@/components/neurun/theme-toggle";
import { useVersionQuery } from "@/lib/api/queries";
import { useSession } from "@/lib/session/store";
import { usePreferences } from "@/lib/preferences/store";
import { routeForIdentifier } from "@/lib/navigation";

export function TopNav() {
  const { operator, logout } = useSession();
  const { timeZone, toggleTimeZone } = usePreferences();
  const version = useVersionQuery();

  return (
    <header className="flex h-(--nr-nav-height) shrink-0 items-center gap-4 border-b border-line bg-surface-base px-4">
      <Link href="/jobs" className="shrink-0 rounded-xs">
        <Wordmark pulse={version.isSuccess} />
      </Link>

      <IdentifierSearch />

      <div className="ml-auto flex shrink-0 items-center gap-1">
        {/* Control-plane health and version. Unknown is stated, never faked. */}
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="hidden items-center gap-1.5 rounded-xs px-2 py-1 font-mono text-micro text-fg-muted md:inline-flex">
              <span
                aria-hidden
                className={
                  version.isSuccess
                    ? "size-1.5 rounded-full bg-line-inverse"
                    : "size-1.5 rounded-full border border-dashed border-line-strong"
                }
              />
              {version.data ? `v${version.data.version}` : "version unknown"}
            </span>
          </TooltipTrigger>
          <TooltipContent>
            {version.data ? (
              <dl className="space-y-0.5 font-mono text-micro">
                <div>server {version.data.version}</div>
                <div>api {version.data.api_version}</div>
                <div>schema {version.data.schema_version}</div>
                <div>bundle {version.data.function_bundle}</div>
                <div className="text-fg-muted">{version.data.commit}</div>
              </dl>
            ) : (
              <span className="font-mono text-micro">Control-plane version unavailable</span>
            )}
          </TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              onClick={toggleTimeZone}
              className="font-mono text-micro"
            >
              {timeZone === "utc" ? "UTC" : "local"}
            </Button>
          </TooltipTrigger>
          <TooltipContent>Show exact times in {timeZone === "utc" ? "local time" : "UTC"}</TooltipContent>
        </Tooltip>

        <ThemeToggle />

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="sm" className="gap-1.5 font-mono text-micro">
              {operator?.username ?? "signed out"}
              <ChevronDown aria-hidden className="size-3" strokeWidth={1.5} />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-72">
            <DropdownMenuLabel className="font-normal">
              <span className="block font-mono text-caption text-fg">{operator?.username}</span>
              <span className="block font-mono text-micro text-fg-muted">
                {operator?.role}
              </span>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <div className="px-2 py-1.5 text-caption text-fg-muted">
              Scopes: <span className="font-mono">{operator?.scopes.join(" ")}</span>
            </div>
            <div className="px-2 pb-1.5 text-caption text-fg-muted">
              The session determines the project. Project switching requires the future project API.
            </div>
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={() => void logout()}>
              <LogOut aria-hidden className="size-3.5" strokeWidth={1.5} />
              Sign out
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  );
}

/**
 * Identifier navigation.
 *
 * Deliberately not a search box: there is no cross-resource search contract, so
 * this resolves the ID prefixes the API actually declares (`job_`, `fni_`) and
 * says so plainly rather than pretending to search.
 */
function IdentifierSearch() {
  const router = useRouter();
  const [value, setValue] = useState("");
  const [error, setError] = useState<string | null>(null);

  function onSubmit(event: FormEvent) {
    event.preventDefault();
    const href = routeForIdentifier(value);
    if (!href) {
      setError("Enter a job ID (job_…) or invocation ID (fni_…).");
      return;
    }
    setError(null);
    setValue("");
    router.push(href);
  }

  return (
    <form onSubmit={onSubmit} className="hidden min-w-0 max-w-sm flex-1 lg:block">
      <label htmlFor="identifier-nav" className="sr-only">
        Go to a job or invocation by ID
      </label>
      <div className="relative">
        <Search
          aria-hidden
          className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-fg-muted"
          strokeWidth={1.5}
        />
        <Input
          id="identifier-nav"
          value={value}
          onChange={(event) => {
            setValue(event.target.value);
            if (error) setError(null);
          }}
          placeholder="job_… or fni_…"
          aria-invalid={error ? true : undefined}
          aria-describedby={error ? "identifier-nav-error" : undefined}
          className="h-(--nr-control-h-sm) pl-8 font-mono text-caption"
        />
      </div>
      {error ? (
        <p id="identifier-nav-error" role="alert" className="mt-1 text-micro text-fg-muted">
          {error}
        </p>
      ) : null}
    </form>
  );
}
