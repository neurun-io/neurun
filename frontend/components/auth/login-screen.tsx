"use client";

import Link from "next/link";
import { useState, type FormEvent, type ReactNode } from "react";
import {
  ArrowRight,
  AtSign,
  Building2,
  KeyRound,
  Loader2,
  type LucideIcon,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { InlineError } from "@/components/neurun/error-panel";
import { Wordmark } from "@/components/neurun/logo";
import { ThemeToggle } from "@/components/neurun/theme-toggle";
import { NeurunApiError } from "@/lib/api/errors";
import { useSession } from "@/lib/session/store";
import { cn } from "@/lib/utils";

type Mode = "register" | "login";

/**
 * The account surface: create one, or sign in to one.
 *
 * The password is posted once and never retained — the server answers with an
 * `HttpOnly` session cookie, so this component holds no credential after submit
 * and there is nothing for a later script to read. Registration is the only way
 * an account comes into being; there is no CLI to run on the host.
 */
export function LoginScreen({ initialMode }: { initialMode?: Mode }) {
  const [mode, setMode] = useState<Mode>(initialMode ?? "login");

  return (
    <div className="flex min-h-dvh flex-col px-6 pt-5 pb-12">
      <div className="flex items-center gap-3">
        <Link href="/" aria-label="Neurun home" className="flex items-center">
          <Wordmark className="text-4xl" logoClassName="size-20" />
        </Link>
        <span aria-hidden className="flex-1" />
        <Link
          href="/docs"
          className="text-caption text-fg-secondary transition-colors duration-120 ease-mech hover:text-fg"
        >
          Docs
        </Link>
        <Link
          href="/#pricing"
          className="text-caption text-fg-secondary transition-colors duration-120 ease-mech hover:text-fg"
        >
          Pricing
        </Link>
        <ThemeToggle />
      </div>

      <main
        id="main"
        className="mx-auto flex w-full max-w-109 flex-1 flex-col gap-5.5"
      >
        <div className="flex flex-col gap-2">
          <h1 className="text-3xl tracking-title">
            {mode === "register" ? "Create an account" : "Sign in"}
          </h1>
          {/* Two lines in both modes, so the switch never reflows what follows. */}
          <p className="min-h-11 text-sm leading-[1.55] text-fg-secondary">
            {mode === "register"
              ? "Free credit to start. Two minutes, no card, no sales call."
              : "Your role and scopes come from your membership. Nothing to pick, nothing to get wrong."}
          </p>
        </div>

        <Segmented mode={mode} onChange={setMode} />

        {/* Both forms occupy one grid cell, so the column is always as tall
              as the taller of the two and switching moves nothing. */}
        <div className="grid">
          <div
            className={cn(
              "col-start-1 row-start-1",
              mode !== "register" && "invisible",
            )}
            inert={mode !== "register"}
          >
            <RegisterForm />
          </div>
          <div
            className={cn(
              "col-start-1 row-start-1",
              mode !== "login" && "invisible",
            )}
            inert={mode !== "login"}
          >
            <LoginForm />
          </div>
        </div>

        <Federated />
      </main>
    </div>
  );
}

function Segmented({
  mode,
  onChange,
}: {
  mode: Mode;
  onChange: (mode: Mode) => void;
}) {
  return (
    <div
      role="tablist"
      aria-label="Account"
      className="inline-flex h-7.5 items-center gap-0.5 rounded-md border border-gray-500 p-0.5"
    >
      {(
        [
          { id: "register", label: "create account" },
          { id: "login", label: "sign in" },
        ] as const
      ).map((option) => (
        <button
          key={option.id}
          type="button"
          role="tab"
          aria-selected={mode === option.id}
          onClick={() => onChange(option.id)}
          className={cn(
            "h-6.5 flex-1 rounded-xs px-2.5 font-mono text-sm transition-colors duration-120 ease-mech",
            mode === option.id
              ? "bg-surface-inverse font-medium text-fg-inverse"
              : "text-fg-secondary hover:text-fg",
          )}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}

function RegisterForm() {
  const { register, isRegistering, registerError } = useSession();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [organizationName, setOrganizationName] = useState("");
  const [accepted, setAccepted] = useState(false);

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    try {
      await register({
        email: email.trim(),
        password,
        ...(organizationName.trim()
          ? { organization_name: organizationName.trim() }
          : {}),
      });
    } finally {
      setPassword("");
    }
  }

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4">
      <Field
        label="Work email"
        htmlFor="email"
        hint="Used for billing and incident notices. Nothing else."
      >
        <IconInput
          icon={AtSign}
          id="email"
          name="email"
          type="email"
          value={email}
          onChange={(event) => setEmail(event.target.value)}
          autoComplete="email"
          autoCapitalize="none"
          spellCheck={false}
          required
          placeholder="ada@example.com"
        />
      </Field>

      <Field label="Password" htmlFor="password" hint="6 characters minimum.">
        <IconInput
          icon={KeyRound}
          id="password"
          name="password"
          type="password"
          value={password}
          onChange={(event) => setPassword(event.target.value)}
          autoComplete="new-password"
          required
          minLength={6}
          placeholder="••••••••••••"
        />
      </Field>

      <Field
        label="Organization name"
        htmlFor="organization-name"
        optional
        hint="Leave it blank and you will be asked to start one, or to accept an invitation, straight after. You may own one and join any number."
      >
        <IconInput
          icon={Building2}
          id="organization-name"
          name="organization-name"
          value={organizationName}
          onChange={(event) => setOrganizationName(event.target.value)}
          placeholder="Acme Data"
          mono={false}
        />
      </Field>

      <label className="flex items-start gap-2.5">
        <Checkbox
          checked={accepted}
          onCheckedChange={(value) => setAccepted(value === true)}
          className="mt-0.5"
          required
        />
        <span className="flex flex-col gap-0.5">
          <span className="text-caption text-fg">
            I accept the terms of service
          </span>
          <span className="text-micro text-fg-muted">
            Commercial licence. Compute is metered per organization and billed
            per GB-hour.
          </span>
        </span>
      </label>

      {registerError ? <InlineError error={registerError} /> : null}

      <Button
        type="submit"
        disabled={isRegistering || !accepted}
        className="w-full"
      >
        {isRegistering ? (
          <>
            <Loader2
              aria-hidden
              className="size-3.5 animate-spin"
              strokeWidth={1.5}
            />
            Creating account
          </>
        ) : (
          <>
            Create account
            <ArrowRight aria-hidden strokeWidth={1.5} />
          </>
        )}
      </Button>
    </form>
  );
}

function LoginForm() {
  const { login, isLoggingIn, loginError } = useSession();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    try {
      await login(email.trim(), password);
    } finally {
      // Drop the password from component state whether or not sign-in
      // succeeded, so a failed attempt does not leave it sitting in memory.
      setPassword("");
    }
  }

  const retryAfter =
    loginError instanceof NeurunApiError && loginError.status === 429
      ? loginError.retryAfter
      : undefined;

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4">
      <Field label="Work email" htmlFor="login-email">
        <IconInput
          icon={AtSign}
          id="login-email"
          name="email"
          type="email"
          value={email}
          onChange={(event) => setEmail(event.target.value)}
          autoComplete="email"
          autoCapitalize="none"
          spellCheck={false}
          required
          placeholder="ada@example.com"
        />
      </Field>

      <Field
        label="Password"
        htmlFor="login-password"
        hint="Verified server-side. Nothing is stored in this browser."
      >
        <IconInput
          icon={KeyRound}
          id="login-password"
          name="password"
          type="password"
          value={password}
          onChange={(event) => setPassword(event.target.value)}
          autoComplete="current-password"
          required
          placeholder="••••••••••••"
        />
      </Field>

      {loginError ? <InlineError error={loginError} /> : null}
      {retryAfter ? (
        <p className="text-micro text-fg-muted">
          Too many attempts. Retry in {retryAfter} seconds.
        </p>
      ) : null}

      <Button type="submit" disabled={isLoggingIn} className="w-full">
        {isLoggingIn ? (
          <>
            <Loader2
              aria-hidden
              className="size-3.5 animate-spin"
              strokeWidth={1.5}
            />
            Signing in
          </>
        ) : (
          <>
            Sign in
            <ArrowRight aria-hidden strokeWidth={1.5} />
          </>
        )}
      </Button>
    </form>
  );
}

/** An input with a leading glyph, as the auth screens are drawn. */
function IconInput({
  icon: Icon,
  mono = true,
  className,
  ...props
}: React.ComponentProps<typeof Input> & { icon: LucideIcon; mono?: boolean }) {
  return (
    <div className="relative">
      <Icon
        aria-hidden
        className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-fg-muted"
        strokeWidth={1.5}
      />
      <Input
        className={cn("pl-8", mono && "font-mono text-caption", className)}
        {...props}
      />
    </div>
  );
}

function Field({
  label,
  htmlFor,
  hint,
  optional = false,
  children,
}: {
  label: string;
  htmlFor: string;
  hint?: string;
  optional?: boolean;
  children: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-baseline gap-2">
        <Label htmlFor={htmlFor} className="nr-label">
          {label}
        </Label>
        {optional ? (
          <span className="rounded-xs border border-dashed border-line-default px-1 font-mono text-micro text-fg-muted">
            optional
          </span>
        ) : null}
      </div>
      {children}
      {hint ? <p className="text-micro text-fg-muted">{hint}</p> : null}
    </div>
  );
}

function Federated() {
  return (
    <div className="flex flex-col gap-2.5">
      <div className="flex items-center gap-3">
        <span aria-hidden className="h-px flex-1 bg-line" />
        <span className="nr-label">not yet available</span>
        <span aria-hidden className="h-px flex-1 bg-line" />
      </div>
      <div className="grid grid-cols-2 gap-2">
        <Button variant="secondary" size="sm" aria-disabled className="w-full">
          GitHub
        </Button>
        <Button variant="secondary" size="sm" aria-disabled className="w-full">
          Google
        </Button>
      </div>
    </div>
  );
}
