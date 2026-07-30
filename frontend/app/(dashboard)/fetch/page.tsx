"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { Callout } from "@/components/neurun/feedback";
import { InlineError } from "@/components/neurun/error-panel";
import { InvocationResult } from "@/components/functions/invocation-result";
import { useFetchMutation } from "@/lib/api/queries";
import { useCapability } from "@/lib/connection/capability";
import type { FetchRequest } from "@/lib/api/types";

/**
 * The HTTP fetch instrument.
 *
 * `POST /v1/fetch` maps the request onto the release-owned `http.fetch`
 * function and supplies a server-owned ephemeral HTTP context — the context a
 * public client cannot assign itself through the generic invoke route.
 *
 * The current function accepts GET and HEAD. `mode=auto` resolves to HTTP;
 * browser mode is a future capability and is not offered.
 */
export default function FetchPage() {
  const router = useRouter();
  const fetchMutation = useFetchMutation();
  const { asyncAvailability } = useCapability();

  const [url, setUrl] = useState("");
  const [method, setMethod] = useState<"GET" | "HEAD">("GET");
  const [mode, setMode] = useState<"auto" | "http">("auto");
  const [execution, setExecution] = useState<"sync" | "async">("sync");
  const [headersText, setHeadersText] = useState("");
  const [maxAttempts, setMaxAttempts] = useState("3");
  const [formError, setFormError] = useState<string | null>(null);

  const asyncDisabled = asyncAvailability === "unavailable";
  const effectiveExecution = asyncDisabled ? "sync" : execution;

  function parseHeaders(): Record<string, string> | null {
    const text = headersText.trim();
    if (!text) return {};

    const headers: Record<string, string> = {};
    for (const line of text.split("\n")) {
      if (!line.trim()) continue;
      const separator = line.indexOf(":");
      if (separator === -1) {
        setFormError(`Header line is missing a colon: "${line.trim()}"`);
        return null;
      }
      headers[line.slice(0, separator).trim()] = line.slice(separator + 1).trim();
    }
    return headers;
  }

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setFormError(null);

    const headers = parseHeaders();
    if (!headers) return;

    const body: FetchRequest = {
      mode,
      // The contract's documented default. `http.fetch` is release-owned, so
      // there is no operator-chosen version to pin here.
      version: "stable",
      execution: effectiveExecution,
      request: {
        url: url.trim(),
        method,
        ...(Object.keys(headers).length > 0 ? { headers } : {}),
      },
    };

    if (effectiveExecution === "async") {
      const attempts = Number(maxAttempts);
      if (!Number.isInteger(attempts) || attempts < 1 || attempts > 10) {
        setFormError("Max attempts must be a whole number between 1 and 10.");
        return;
      }
      body.max_attempts = attempts;
    }

    try {
      const outcome = await fetchMutation.mutateAsync(body);
      if (outcome.kind === "accepted") {
        router.push(`/jobs/${outcome.result.data.job_id}`);
      }
    } catch {
      // Rendered inline below.
    }
  }

  return (
    <div className="flex min-h-full flex-col">
      <PageHeader
        title="HTTP fetch"
        description="Execute the release-owned http.fetch function against a URL, with a server-owned ephemeral HTTP context."
      />

      <div className="mx-auto grid w-full max-w-5xl gap-4 px-6 py-4 lg:grid-cols-[minmax(0,420px)_minmax(0,1fr)]">
        <div className="space-y-4">
          <Panel label="Request">
            <form onSubmit={submit} className="space-y-4">
              <div className="space-y-1.5">
                <Label htmlFor="fetch-url">URL</Label>
                <Input
                  id="fetch-url"
                  type="url"
                  required
                  value={url}
                  onChange={(event) => setUrl(event.target.value)}
                  placeholder="https://example.com"
                  className="font-mono text-caption"
                />
              </div>

              <div className="grid gap-3 sm:grid-cols-2">
                <div className="space-y-1.5">
                  <Label htmlFor="fetch-method">Method</Label>
                  <Select value={method} onValueChange={(next) => setMethod(next as "GET" | "HEAD")}>
                    <SelectTrigger id="fetch-method" className="w-full font-mono text-caption">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="GET">GET</SelectItem>
                      <SelectItem value="HEAD">HEAD</SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                <div className="space-y-1.5">
                  <Label htmlFor="fetch-mode">Mode</Label>
                  <Select value={mode} onValueChange={(next) => setMode(next as "auto" | "http")}>
                    <SelectTrigger id="fetch-mode" className="w-full font-mono text-caption">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="auto">auto</SelectItem>
                      <SelectItem value="http">http</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="fetch-headers">Headers</Label>
                <Textarea
                  id="fetch-headers"
                  value={headersText}
                  onChange={(event) => setHeadersText(event.target.value)}
                  rows={4}
                  spellCheck={false}
                  placeholder={"Accept: text/html\nUser-Agent: neurun"}
                  className="font-mono text-caption"
                />
                <p className="text-micro text-fg-muted">One `Name: value` pair per line.</p>
              </div>

              <fieldset className="space-y-2 border-t border-line pt-3">
                <legend className="nr-label mb-1.5">Execution</legend>
                <RadioGroup
                  value={effectiveExecution}
                  onValueChange={(next) => setExecution(next as "sync" | "async")}
                  className="gap-2"
                >
                  <div className="flex items-center gap-2">
                    <RadioGroupItem value="sync" id="fetch-sync" />
                    <Label htmlFor="fetch-sync" className="font-normal">
                      Synchronous
                    </Label>
                  </div>
                  <div className="flex items-start gap-2">
                    <RadioGroupItem
                      value="async"
                      id="fetch-async"
                      disabled={asyncDisabled}
                      className="mt-0.5"
                    />
                    <Label htmlFor="fetch-async" className="font-normal">
                      Asynchronous
                      {asyncDisabled ? (
                        <span className="mt-0.5 block text-micro text-fg-muted">
                          Unavailable: this control plane returned durable_backend_unavailable.
                          Synchronous fetch still works.
                        </span>
                      ) : null}
                    </Label>
                  </div>
                </RadioGroup>
              </fieldset>

              {effectiveExecution === "async" ? (
                <div className="space-y-1.5">
                  <Label htmlFor="fetch-attempts">Max attempts</Label>
                  <Input
                    id="fetch-attempts"
                    type="number"
                    min={1}
                    max={10}
                    value={maxAttempts}
                    onChange={(event) => setMaxAttempts(event.target.value)}
                    className="font-mono text-caption"
                  />
                </div>
              ) : null}

              {formError ? (
                <p role="alert" className="text-sm text-fg-secondary">
                  {formError}
                </p>
              ) : null}
              {fetchMutation.isError ? <InlineError error={fetchMutation.error} /> : null}

              <Button type="submit" disabled={fetchMutation.isPending}>
                {fetchMutation.isPending
                  ? "Fetching…"
                  : effectiveExecution === "async"
                    ? "Submit fetch job"
                    : "Fetch"}
              </Button>
            </form>
          </Panel>

          <Callout kind="roadmap" title="Browser mode">
            `mode=auto` currently resolves to HTTP. Browser-backed fetching arrives with the session
            and browser-runtime contracts.
          </Callout>
        </div>

        <div className="min-w-0">
          {fetchMutation.data?.kind === "invocation" ? (
            <InvocationResult invocation={fetchMutation.data.result.data} />
          ) : (
            <Panel label="Result">
              <p className="text-sm text-fg-muted">
                Run a fetch to see the invocation, its usage and its trace.
              </p>
            </Panel>
          )}
        </div>
      </div>
    </div>
  );
}
