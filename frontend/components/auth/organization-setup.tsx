"use client";

import { useState, type FormEvent } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useMutation } from "@tanstack/react-query";
import { ArrowRight, Building2, Loader2, Mail } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Callout } from "@/components/neurun/feedback";
import { InlineError } from "@/components/neurun/error-panel";
import { Logo } from "@/components/neurun/logo";
import { ThemeToggle } from "@/components/neurun/theme-toggle";
import * as api from "@/lib/api/endpoints";
import { SESSION_QUERY_KEY, useSession } from "@/lib/session/store";
import { cn } from "@/lib/utils";

type Route = "create" | "join";

/**
 * Where an account with no organization lands.
 *
 * Signing up without one is allowed, but nothing below an organization exists
 * until there is one — so this is the whole surface until the account either
 * starts one or accepts an invitation. The server re-issues the session on
 * either path, so there is no second sign-in.
 */
export function OrganizationSetup() {
  const { session, logout } = useSession();
  const queryClient = useQueryClient();
  const [route, setRoute] = useState<Route>("create");
  const [name, setName] = useState("");
  const [token, setToken] = useState("");

  const settle = () => queryClient.invalidateQueries({ queryKey: SESSION_QUERY_KEY });

  const create = useMutation({
    mutationFn: (value: string) => api.createOrganization(value),
    onSuccess: settle,
  });
  const join = useMutation({
    mutationFn: (value: string) => api.acceptInvite(value),
    onSuccess: settle,
  });

  const pending = create.isPending || join.isPending;
  const error = create.error ?? join.error;

  function onSubmit(event: FormEvent) {
    event.preventDefault();
    if (route === "create") create.mutate(name.trim());
    else join.mutate(token.trim());
  }

  return (
    <main id="main" className="flex min-h-dvh flex-col px-6 py-6">
      <div className="flex items-center gap-3">
        <Logo />
        <span className="nr-label border-l border-line-default pl-3">one step left</span>
        <span className="ml-auto" />
        <ThemeToggle />
        <Button variant="ghost" size="sm" onClick={() => void logout()}>
          Sign out
        </Button>
      </div>

      <div className="mx-auto flex w-full max-w-115 flex-1 flex-col justify-center gap-6">
        <div className="flex flex-col gap-2">
          <h1 className="text-3xl tracking-title">Pick an organization</h1>
          <p className="text-sm leading-[1.55] text-fg-secondary">
            Your account exists, but projects, apps and executions all hang from an organization.
            Start one and you own it, or accept an invitation to join someone else&apos;s.
          </p>
        </div>

        <div role="radiogroup" aria-label="How to continue" className="grid grid-cols-2 gap-2">
          {(
            [
              { id: "create", label: "Start one", hint: "You own it. Only one.", icon: Building2 },
              { id: "join", label: "Join with an invite", hint: "As many as you like", icon: Mail },
            ] as const
          ).map((option) => (
            <button
              key={option.id}
              type="button"
              role="radio"
              aria-checked={route === option.id}
              onClick={() => setRoute(option.id)}
              className={cn(
                "flex flex-col gap-1 rounded-lg border p-3.5 text-left transition-colors duration-120 ease-mech",
                route === option.id
                  ? "border-line-inverse bg-surface-panel"
                  : "border-line hover:border-line-default",
              )}
            >
              <option.icon aria-hidden className="size-4 text-fg-muted" strokeWidth={1.5} />
              <span className="text-caption text-fg">{option.label}</span>
              <span className="text-micro text-fg-muted">{option.hint}</span>
            </button>
          ))}
        </div>

        <form onSubmit={onSubmit} className="flex flex-col gap-4">
          {route === "create" ? (
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="organization-name" className="nr-label">
                Organization name
              </Label>
              <Input
                id="organization-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder="Acme Data"
                required
              />
              <p className="text-micro text-fg-muted">
                Owns every project beneath it. You may own one organization, and join any number.
              </p>
            </div>
          ) : (
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="invite-token" className="nr-label">
                Invitation token
              </Label>
              <Input
                id="invite-token"
                value={token}
                onChange={(event) => setToken(event.target.value)}
                autoCapitalize="none"
                spellCheck={false}
                required
                className="font-mono text-caption"
              />
              <p className="text-micro text-fg-muted">
                It must have been issued to{" "}
                <span className="font-mono text-fg-secondary">{session?.email}</span>, and it
                expires seven days after it was sent.
              </p>
            </div>
          )}

          {error ? <InlineError error={error} /> : null}

          <Button type="submit" disabled={pending} className="w-full">
            {pending ? (
              <>
                <Loader2 aria-hidden className="size-3.5 animate-spin" strokeWidth={1.5} />
                {route === "create" ? "Creating" : "Joining"}
              </>
            ) : (
              <>
                {route === "create" ? "Create organization" : "Join organization"}
                <ArrowRight aria-hidden strokeWidth={1.5} />
              </>
            )}
          </Button>
        </form>
      </div>
    </main>
  );
}
