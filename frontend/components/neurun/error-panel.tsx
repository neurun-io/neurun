"use client";

import { AlertTriangle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { CopyId } from "./copy-id";
import { Panel } from "./panel";
import { cn } from "@/lib/utils";
import {
  NeurunApiError,
  NeurunContractError,
  NeurunTransportError,
  errorMessageFor,
} from "@/lib/api/errors";

/**
 * Renders the standard error envelope.
 *
 * The request ID is always exposed and always copyable — it is the one thing
 * that lets an operator and a backend engineer talk about the same request.
 *
 * Validation messages are printed as the server wrote them. The current
 * contract puts human-readable paths like `$.field` inside the message rather
 * than emitting structured violations, so parsing them into field-level state
 * would be guesswork. Field mapping waits for structured violations.
 */
export function ErrorPanel({
  error,
  onRetry,
  className,
  title = "Request failed",
}: {
  error: unknown;
  onRetry?: () => void;
  className?: string;
  title?: string;
}) {
  const apiError = error instanceof NeurunApiError ? error : null;
  const contractError = error instanceof NeurunContractError ? error : null;
  const transportError = error instanceof NeurunTransportError ? error : null;

  const label = apiError
    ? `${apiError.status} ${apiError.code}`
    : contractError
      ? "contract mismatch"
      : transportError
        ? "unreachable"
        : "error";

  return (
    <Panel
      label={
        <span className="inline-flex items-center gap-1.5">
          <AlertTriangle aria-hidden className="size-3" strokeWidth={1.5} />
          {title}
        </span>
      }
      className={cn("border-line-strong", className)}
      actions={
        onRetry ? (
          <Button size="sm" variant="ghost" onClick={onRetry}>
            Try again
          </Button>
        ) : null
      }
    >
      <div className="space-y-3">
        <p className="font-mono text-micro tracking-wide text-fg-muted uppercase">{label}</p>
        <p className="text-sm text-fg-secondary">{errorMessageFor(error)}</p>

        {contractError ? (
          <ul className="space-y-1 border-l border-line-default pl-3">
            {contractError.issues.map((issue) => (
              <li key={issue} className="font-mono text-micro text-fg-muted">
                {issue}
              </li>
            ))}
          </ul>
        ) : null}

        {apiError?.requestId || apiError?.traceId ? (
          <dl className="space-y-1 border-t border-line pt-2">
            {apiError.requestId ? (
              <div className="flex items-center justify-between gap-3">
                <dt className="nr-label">Request ID</dt>
                <dd>
                  <CopyId value={apiError.requestId} label="request ID" />
                </dd>
              </div>
            ) : null}
            {apiError.traceId ? (
              <div className="flex items-center justify-between gap-3">
                <dt className="nr-label">Trace ID</dt>
                <dd>
                  <CopyId value={apiError.traceId} label="trace ID" />
                </dd>
              </div>
            ) : null}
          </dl>
        ) : null}

        {apiError?.details && Object.keys(apiError.details).length > 0 ? (
          <details className="border-t border-line pt-2">
            <summary className="nr-label cursor-pointer select-none">Details</summary>
            <pre className="mt-2 overflow-x-auto rounded-md border border-line bg-surface-sunken p-2 font-mono text-micro text-fg-secondary">
              <code>{JSON.stringify(apiError.details, null, 2)}</code>
            </pre>
          </details>
        ) : null}
      </div>
    </Panel>
  );
}

/** Compact inline variant for forms and dialogs. */
export function InlineError({ error, className }: { error: unknown; className?: string }) {
  const apiError = error instanceof NeurunApiError ? error : null;

  return (
    <div
      role="alert"
      className={cn("rounded-md border border-line-strong bg-surface-panel p-2.5", className)}
    >
      <p className="text-sm text-fg-secondary">{errorMessageFor(error)}</p>
      {apiError?.requestId ? (
        <div className="mt-1.5 flex items-center gap-1.5">
          <span className="nr-label">Request ID</span>
          <CopyId value={apiError.requestId} label="request ID" truncate />
        </div>
      ) : null}
    </div>
  );
}
