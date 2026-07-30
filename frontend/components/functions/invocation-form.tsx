"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Switch } from "@/components/ui/switch";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Panel } from "@/components/neurun/panel";
import { Callout } from "@/components/neurun/feedback";
import { InlineError } from "@/components/neurun/error-panel";
import { InvocationResult } from "./invocation-result";
import { useInvokeFunctionMutation } from "@/lib/api/queries";
import { useCapability } from "@/lib/session/capability";
import {
  acceptsAsyncExecution,
  requiresFetchEndpointForSync,
  type FunctionManifest,
  type InvokeFunctionRequest,
} from "@/lib/api/types";
import { coerceValues, deriveFields, emptyValues } from "@/lib/view/schema-form";

type Execution = "sync" | "async";

export function InvocationForm({ manifest }: { manifest: FunctionManifest }) {
  const router = useRouter();
  const invoke = useInvokeFunctionMutation();
  const { asyncAvailability } = useCapability();

  const fields = useMemo(() => deriveFields(manifest.input_schema), [manifest.input_schema]);
  const [mode, setMode] = useState<"form" | "raw">(fields ? "form" : "raw");
  const [values, setValues] = useState<Record<string, string>>(() =>
    fields ? emptyValues(fields) : {},
  );
  const [rawInput, setRawInput] = useState("{}");
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState<string | null>(null);

  const [execution, setExecution] = useState<Execution>("sync");
  const [timeoutMs, setTimeoutMs] = useState(String(manifest.timeout.default_ms));
  const [maxAttempts, setMaxAttempts] = useState("3");

  /* ---------------------------------------------------------------------- */
  /* Capability gating                                                       */
  /* ---------------------------------------------------------------------- */

  // Async is accepted only for `none` and `http_attempt` execution contexts.
  const asyncSupportedByManifest = acceptsAsyncExecution(manifest);
  // …and only while the server has a durable (or explicitly volatile) backend.
  const asyncDisabledByServer = asyncAvailability === "unavailable";
  const asyncAvailable = asyncSupportedByManifest && !asyncDisabledByServer;

  // `http.fetch` cannot be invoked synchronously through the generic route: a
  // public client cannot assign the trusted HTTP context it requires.
  const syncBlocked = requiresFetchEndpointForSync(manifest.name);

  const effectiveExecution: Execution = !asyncAvailable
    ? "sync"
    : syncBlocked
      ? "async"
      : execution;

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setFormError(null);
    setFieldErrors({});

    let input: unknown;
    if (mode === "raw") {
      try {
        input = JSON.parse(rawInput);
      } catch {
        setFormError("Input must be valid JSON.");
        return;
      }
    } else if (fields) {
      const { input: coerced, errors } = coerceValues(fields, values);
      if (Object.keys(errors).length > 0) {
        setFieldErrors(errors);
        return;
      }
      input = coerced;
    } else {
      input = {};
    }

    // Version is pinned to the exact resolved version, never an alias: an
    // invocation an operator repeats must run the same code.
    const body: InvokeFunctionRequest = {
      version: manifest.version,
      digest: manifest.digest,
      execution: effectiveExecution,
      input,
    };

    if (effectiveExecution === "async") {
      // Async requests must omit `context` and `timeout_ms` in this release.
      const attempts = Number(maxAttempts);
      if (!Number.isInteger(attempts) || attempts < 1 || attempts > 10) {
        setFormError("Max attempts must be a whole number between 1 and 10.");
        return;
      }
      body.max_attempts = attempts;
    } else {
      const timeout = Number(timeoutMs);
      if (!Number.isInteger(timeout) || timeout < 1 || timeout > manifest.timeout.maximum_ms) {
        setFormError(`Timeout must be between 1 and ${manifest.timeout.maximum_ms} ms.`);
        return;
      }
      body.timeout_ms = timeout;
    }

    try {
      const outcome = await invoke.mutateAsync({ functionName: manifest.name, body });
      if (outcome.kind === "accepted") {
        router.push(`/jobs/${outcome.result.data.job_id}`);
      }
    } catch {
      // Rendered inline below.
    }
  }

  if (syncBlocked && !asyncAvailable) {
    return (
      <Panel label="Invoke">
        <Callout kind="note" title="Use the HTTP fetch instrument">
          <code className="font-mono text-micro">http.fetch</code> requires a server-owned HTTP
          execution context that a public client cannot assign, so synchronous HTTP work goes
          through <code className="font-mono text-micro">POST /v1/fetch</code> instead of the
          generic invoke route.{" "}
          <Link href="/fetch" className="underline underline-offset-3">
            Open the fetch instrument →
          </Link>
        </Callout>
      </Panel>
    );
  }

  return (
    <div className="space-y-4">
      <Panel label="Invoke">
        <form onSubmit={submit} className="space-y-4">
          {/* ---------------- input ---------------- */}
          <div className="flex items-center justify-between gap-3">
            <p className="nr-label">Input</p>
            {fields ? (
              <div className="flex items-center gap-2">
                <Label htmlFor="raw-toggle" className="text-micro font-normal text-fg-muted">
                  Raw JSON
                </Label>
                <Switch
                  id="raw-toggle"
                  checked={mode === "raw"}
                  onCheckedChange={(checked) => setMode(checked ? "raw" : "form")}
                />
              </div>
            ) : null}
          </div>

          {mode === "raw" || !fields ? (
            <div className="space-y-1.5">
              <Label htmlFor="raw-input" className="sr-only">
                Raw JSON input
              </Label>
              <Textarea
                id="raw-input"
                value={rawInput}
                onChange={(event) => setRawInput(event.target.value)}
                rows={10}
                spellCheck={false}
                className="font-mono text-caption"
              />
              {!fields ? (
                <p className="text-micro text-fg-muted">
                  This function&apos;s input schema is not a plain object, so the raw editor is the
                  only faithful control.
                </p>
              ) : null}
            </div>
          ) : (
            <div className="space-y-3">
              {fields.map((field) => (
                <div key={field.name} className="space-y-1.5">
                  <Label htmlFor={`field-${field.name}`}>
                    <span className="font-mono">{field.name}</span>
                    {field.required ? (
                      <span aria-hidden className="ml-1 text-fg-muted">
                        *
                      </span>
                    ) : null}
                    {field.required ? <span className="sr-only"> (required)</span> : null}
                  </Label>

                  {field.kind === "enum" ? (
                    <Select
                      value={values[field.name] ?? ""}
                      onValueChange={(next) =>
                        setValues((previous) => ({ ...previous, [field.name]: next }))
                      }
                    >
                      <SelectTrigger id={`field-${field.name}`} className="w-full font-mono text-caption">
                        <SelectValue placeholder="choose…" />
                      </SelectTrigger>
                      <SelectContent>
                        {field.options?.map((option) => (
                          <SelectItem key={option} value={option} className="font-mono text-caption">
                            {option}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  ) : field.kind === "boolean" ? (
                    <div className="flex items-center gap-2">
                      <Switch
                        id={`field-${field.name}`}
                        checked={values[field.name] === "true"}
                        onCheckedChange={(checked) =>
                          setValues((previous) => ({
                            ...previous,
                            [field.name]: checked ? "true" : "false",
                          }))
                        }
                      />
                      <span className="font-mono text-caption text-fg-muted">
                        {values[field.name] === "true" ? "true" : "false"}
                      </span>
                    </div>
                  ) : field.kind === "json" ? (
                    <Textarea
                      id={`field-${field.name}`}
                      value={values[field.name] ?? ""}
                      onChange={(event) =>
                        setValues((previous) => ({ ...previous, [field.name]: event.target.value }))
                      }
                      rows={4}
                      spellCheck={false}
                      placeholder="JSON value"
                      className="font-mono text-caption"
                      aria-invalid={fieldErrors[field.name] ? true : undefined}
                    />
                  ) : (
                    <Input
                      id={`field-${field.name}`}
                      // Secret-looking fields are masked and excluded from
                      // autofill; the value still travels to the server as
                      // typed.
                      type={field.secret ? "password" : field.kind === "string" ? "text" : "number"}
                      autoComplete={field.secret ? "off" : undefined}
                      value={values[field.name] ?? ""}
                      onChange={(event) =>
                        setValues((previous) => ({ ...previous, [field.name]: event.target.value }))
                      }
                      className="font-mono text-caption"
                      aria-invalid={fieldErrors[field.name] ? true : undefined}
                      aria-describedby={
                        fieldErrors[field.name] ? `field-${field.name}-error` : undefined
                      }
                    />
                  )}

                  {field.description ? (
                    <p className="text-micro text-fg-muted">{field.description}</p>
                  ) : null}
                  {fieldErrors[field.name] ? (
                    <p
                      id={`field-${field.name}-error`}
                      role="alert"
                      className="text-micro text-fg-secondary"
                    >
                      {fieldErrors[field.name]}
                    </p>
                  ) : null}
                </div>
              ))}
            </div>
          )}

          {/* ---------------- execution ---------------- */}
          <fieldset className="space-y-2 border-t border-line pt-3">
            <legend className="nr-label mb-1.5">Execution</legend>
            <RadioGroup
              value={effectiveExecution}
              onValueChange={(next) => setExecution(next as Execution)}
              className="gap-2"
            >
              <div className="flex items-start gap-2">
                <RadioGroupItem value="sync" id="execution-sync" disabled={syncBlocked} className="mt-0.5" />
                <Label htmlFor="execution-sync" className="font-normal">
                  Synchronous
                  <span className="mt-0.5 block text-micro text-fg-muted">
                    Runs in the serving process and returns the completed invocation.
                  </span>
                </Label>
              </div>
              <div className="flex items-start gap-2">
                <RadioGroupItem
                  value="async"
                  id="execution-async"
                  disabled={!asyncAvailable}
                  className="mt-0.5"
                />
                <Label htmlFor="execution-async" className="font-normal">
                  Asynchronous
                  <span className="mt-0.5 block text-micro text-fg-muted">
                    {!asyncSupportedByManifest
                      ? `Unavailable: async accepts only "none" and "http_attempt" execution contexts, and this function needs "${manifest.execution_context}".`
                      : asyncDisabledByServer
                        ? "Unavailable: this control plane returned durable_backend_unavailable. Synchronous invocation still works."
                        : "Accepted as a digest-pinned job. Requires jobs:write in addition to functions:invoke."}
                  </span>
                </Label>
              </div>
            </RadioGroup>
          </fieldset>

          {effectiveExecution === "sync" ? (
            <div className="space-y-1.5">
              <Label htmlFor="timeout-ms">Timeout (ms)</Label>
              <Input
                id="timeout-ms"
                type="number"
                min={1}
                max={manifest.timeout.maximum_ms}
                value={timeoutMs}
                onChange={(event) => setTimeoutMs(event.target.value)}
                className="font-mono text-caption"
              />
              <p className="text-micro text-fg-muted">
                Default {manifest.timeout.default_ms} ms · maximum {manifest.timeout.maximum_ms} ms,
                bounded by the manifest.
              </p>
            </div>
          ) : (
            <div className="space-y-1.5">
              <Label htmlFor="max-attempts">Max attempts</Label>
              <Input
                id="max-attempts"
                type="number"
                min={1}
                max={10}
                value={maxAttempts}
                onChange={(event) => setMaxAttempts(event.target.value)}
                className="font-mono text-caption"
              />
              <p className="text-micro text-fg-muted">
                Between 1 and 10; defaults to 3. Async requests omit context and timeout — the
                server owns both.
              </p>
            </div>
          )}

          {formError ? (
            <p role="alert" className="text-sm text-fg-secondary">
              {formError}
            </p>
          ) : null}
          {invoke.isError ? <InlineError error={invoke.error} /> : null}

          <Button type="submit" disabled={invoke.isPending}>
            {invoke.isPending
              ? "Invoking…"
              : effectiveExecution === "async"
                ? "Submit job"
                : "Invoke"}
          </Button>
        </form>
      </Panel>

      {invoke.data?.kind === "invocation" ? (
        <InvocationResult invocation={invoke.data.result.data} />
      ) : null}
    </div>
  );
}
