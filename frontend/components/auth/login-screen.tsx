"use client";

import { useState, type FormEvent } from "react";
import { Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Panel } from "@/components/neurun/panel";
import { Callout } from "@/components/neurun/feedback";
import { InlineError } from "@/components/neurun/error-panel";
import { Logo } from "@/components/neurun/logo";
import { NeurunApiError } from "@/lib/api/errors";
import { useSession } from "@/lib/session/store";

/**
 * Operator sign-in.
 *
 * The password is posted once and never retained: the server answers with an
 * `HttpOnly` session cookie, so this component holds no credential after submit
 * and there is nothing for a later script to read.
 */
export function LoginScreen() {
  const { login, isLoggingIn, loginError, status } = useSession();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    try {
      await login(username.trim(), password);
    } finally {
      // Drop the password from component state whether or not sign-in
      // succeeded, so a failed attempt does not leave it sitting in memory.
      setPassword("");
    }
  }

  if (status === "unavailable") {
    return (
      <main id="main" className="flex min-h-dvh items-center justify-center px-6 py-12">
        <div className="w-full max-w-md">
          <Header />
          <Callout kind="warning" title="No operator accounts are configured">
            <p>
              This control plane has no operator accounts, so username and password sign-in is
              unavailable. Generate a hash and set{" "}
              <code className="font-mono text-micro">NEURUN_OPERATOR_ACCOUNTS</code>:
            </p>
            <pre className="mt-2 overflow-x-auto rounded-md border border-line bg-surface-sunken p-2 font-mono text-micro">
              <code>{`printf '%s' 'your-password' | neurun hash-password\n\nNEURUN_OPERATOR_ACCOUNTS='alice:admin:<hash>'`}</code>
            </pre>
            <p className="mt-2">Roles are admin, operator, or viewer.</p>
          </Callout>
        </div>
      </main>
    );
  }

  const retryAfter =
    loginError instanceof NeurunApiError && loginError.status === 429
      ? loginError.retryAfter
      : undefined;

  return (
    <main id="main" className="flex min-h-dvh items-center justify-center px-6 py-12">
      <div className="w-full max-w-md">
        <Header />

        <Panel label="Sign in">
          <form onSubmit={onSubmit} className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="username">Username</Label>
              <Input
                id="username"
                name="username"
                value={username}
                onChange={(event) => setUsername(event.target.value)}
                autoComplete="username"
                autoCapitalize="none"
                spellCheck={false}
                required
                className="font-mono text-caption"
              />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="password">Password</Label>
              <Input
                id="password"
                name="password"
                type="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                autoComplete="current-password"
                required
                className="font-mono text-caption"
              />
            </div>

            {loginError ? <InlineError error={loginError} /> : null}
            {retryAfter ? (
              <p className="text-micro text-fg-muted">
                Too many attempts. Retry in {retryAfter} seconds.
              </p>
            ) : null}

            <Button type="submit" disabled={isLoggingIn} className="w-full">
              {isLoggingIn ? (
                <>
                  <Loader2 aria-hidden className="size-3.5 animate-spin" strokeWidth={1.5} />
                  Signing in
                </>
              ) : (
                "Sign in"
              )}
            </Button>
          </form>
        </Panel>

        <Callout kind="note" title="Session handling" className="mt-4">
          Sign-in issues an <code className="font-mono text-micro">HttpOnly</code>,{" "}
          <code className="font-mono text-micro">Secure</code>,{" "}
          <code className="font-mono text-micro">SameSite=Strict</code> cookie. No credential is
          stored in the browser where a script could read it. Sessions live in the server process,
          so a restart signs everyone out.
        </Callout>
      </div>
    </main>
  );
}

function Header() {
  return (
    <div className="mb-6 flex items-center gap-2.5">
      <Logo className="size-6" />
      <div>
        <h1 className="text-xl">Neurun operator dashboard</h1>
        <p className="text-caption text-fg-muted">Run the web. Keep the evidence.</p>
      </div>
    </div>
  );
}
