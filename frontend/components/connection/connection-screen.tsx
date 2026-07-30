"use client";

import { useState, type FormEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Panel } from "@/components/neurun/panel";
import { Callout } from "@/components/neurun/feedback";
import { InlineError } from "@/components/neurun/error-panel";
import { Logo } from "@/components/neurun/logo";
import { listFunctions } from "@/lib/api/endpoints";
import { useConnection } from "@/lib/connection/store";

interface ConnectionInfo {
  default_base_url: string;
  allowed_base_urls: string[];
}

/**
 * The connection screen.
 *
 * The key is verified against a scoped endpoint (`GET /v1/functions`) rather
 * than `/version`, because `/version` is unauthenticated and would happily
 * accept a key the server will later reject.
 */
export function ConnectionScreen() {
  const { connect } = useConnection();
  // `null` means "the operator has not typed anything", so the field can fall
  // back to the deployment's configured default without an effect syncing the
  // fetched value into state.
  const [typedBaseUrl, setTypedBaseUrl] = useState<string | null>(null);
  const [apiKey, setApiKey] = useState("");
  const [remember, setRemember] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const info = useQuery<ConnectionInfo>({
    queryKey: ["connection-info"],
    queryFn: async () => {
      const response = await fetch("/api/connection-info", { cache: "no-store" });
      if (!response.ok) throw new Error("Could not read the dashboard's proxy configuration.");
      return (await response.json()) as ConnectionInfo;
    },
    staleTime: Number.POSITIVE_INFINITY,
  });

  const baseUrl = typedBaseUrl ?? info.data?.default_base_url ?? "";

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    setSubmitting(true);
    setError(null);

    const candidate = { baseUrl: baseUrl.trim().replace(/\/+$/, ""), apiKey: apiKey.trim() };

    try {
      // A scoped call: proves the key is valid for this project, not merely
      // that the server is up.
      await listFunctions(candidate, {});
      connect(candidate, remember);
    } catch (cause) {
      setError(cause);
    } finally {
      setSubmitting(false);
    }
  }

  const multipleTargets = (info.data?.allowed_base_urls.length ?? 0) > 1;

  return (
    <main id="main" className="flex min-h-dvh items-center justify-center px-6 py-12">
      <div className="w-full max-w-md">
        <div className="mb-6 flex items-center gap-2.5">
          <Logo className="size-6" />
          <div>
            <h1 className="text-xl">Connect to a control plane</h1>
            <p className="text-caption text-fg-muted">Run the web. Keep the evidence.</p>
          </div>
        </div>

        <Panel label="Connection">
          <form onSubmit={onSubmit} className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="base-url">Control-plane base URL</Label>
              <Input
                id="base-url"
                name="baseUrl"
                value={baseUrl}
                onChange={(event) => setTypedBaseUrl(event.target.value)}
                placeholder="http://localhost:8080"
                autoComplete="url"
                required
                readOnly={info.data && !multipleTargets ? true : undefined}
                className="font-mono text-caption"
                aria-describedby="base-url-hint"
              />
              <p id="base-url-hint" className="text-micro text-fg-muted">
                {multipleTargets
                  ? `This dashboard can reach: ${info.data?.allowed_base_urls.join(", ")}`
                  : "Set by this deployment's NEURUN_API_BASE_URL. Requests are proxied same-origin."}
              </p>
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="api-key">API key</Label>
              <Input
                id="api-key"
                name="apiKey"
                type="password"
                value={apiKey}
                onChange={(event) => setApiKey(event.target.value)}
                placeholder="neu_local_abc123.…"
                autoComplete="off"
                spellCheck={false}
                required
                className="font-mono text-caption"
              />
            </div>

            <div className="flex items-start gap-2">
              <Checkbox
                id="remember"
                checked={remember}
                onCheckedChange={(checked) => setRemember(checked === true)}
                className="mt-0.5"
              />
              <Label htmlFor="remember" className="text-caption leading-snug font-normal">
                Remember for this browser session
                <span className="mt-0.5 block text-micro text-fg-muted">
                  Stored in <code className="font-mono">sessionStorage</code> and cleared when the
                  tab closes. Otherwise the key is held in memory only.
                </span>
              </Label>
            </div>

            {error ? <InlineError error={error} /> : null}

            <Button type="submit" disabled={submitting} className="w-full">
              {submitting ? (
                <>
                  <Loader2 aria-hidden className="size-3.5 animate-spin" strokeWidth={1.5} />
                  Verifying key
                </>
              ) : (
                "Connect"
              )}
            </Button>
          </form>
        </Panel>

        <Callout kind="warning" title="Development authentication" className="mt-4">
          The key is sent from the browser on every request. Before a production browser dashboard
          ships, the backend must add an API-key exchange endpoint that issues a short-lived{" "}
          <code className="font-mono text-micro">HttpOnly</code> operator session, or hold the key
          server-side in a backend-for-frontend. Treat that as a release blocker.
        </Callout>
      </div>
    </main>
  );
}
