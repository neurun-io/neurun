"use client";

import { useMemo, useState } from "react";
import { ChevronRight } from "lucide-react";
import { cn } from "@/lib/utils";
import { CopyButton } from "./copy-id";
import { redactSecrets, toRedactedJson } from "@/lib/view/redaction";
import { formatBytes } from "@/lib/view/units";

/**
 * A JSON viewer that renders **text nodes only**.
 *
 * There is no `dangerouslySetInnerHTML` here and there must never be one: these
 * payloads carry data captured from the open web, and the dashboard's document
 * is trusted. Captured markup is downloaded or sandboxed, never injected.
 *
 * Values are redacted before display, and large payloads are capped rather than
 * pasted wholesale into the DOM.
 */

/** Above this, the viewer offers a download instead of rendering a tree. */
const PREVIEW_BYTE_CAP = 256 * 1024;

/** Nodes deeper than this start collapsed. */
const AUTO_COLLAPSE_DEPTH = 2;

export interface JsonViewProps {
  value: unknown;
  className?: string;
  /** Filename offered when the payload exceeds the preview cap. */
  downloadName?: string;
  /** Skip redaction where the server already returned a redacted projection. */
  preRedacted?: boolean;
}

export function JsonView({
  value,
  className,
  downloadName = "payload.json",
  preRedacted = false,
}: JsonViewProps) {
  const { redacted, serialized, bytes } = useMemo(() => {
    const safe = preRedacted ? value : redactSecrets(value);
    const text = JSON.stringify(safe, null, 2) ?? "null";
    return {
      redacted: safe,
      serialized: text,
      bytes: new Blob([text]).size,
    };
  }, [value, preRedacted]);

  if (value === undefined) {
    return <p className="font-mono text-caption text-fg-muted">No payload recorded.</p>;
  }

  if (bytes > PREVIEW_BYTE_CAP) {
    return (
      <div className={cn("rounded-md border border-line bg-surface-sunken p-3", className)}>
        <p className="text-sm text-fg-secondary">
          This payload is {formatBytes(bytes)} — too large to preview safely.
        </p>
        <DownloadLink text={serialized} name={downloadName} />
      </div>
    );
  }

  return (
    <div className={cn("relative min-w-0", className)}>
      <div className="absolute top-1 right-1 z-10">
        <CopyButton value={serialized} label="Copy JSON" />
      </div>
      <pre className="overflow-x-auto rounded-md border border-line bg-surface-sunken p-3 font-mono text-caption leading-normal">
        <code>
          <JsonNode value={redacted} depth={0} isLast />
        </code>
      </pre>
    </div>
  );
}

function DownloadLink({ text, name }: { text: string; name: string }) {
  const href = useMemo(
    () => `data:application/json;charset=utf-8,${encodeURIComponent(text)}`,
    [text],
  );
  return (
    <a
      href={href}
      download={name}
      className="mt-2 inline-block font-mono text-caption text-fg underline underline-offset-3"
    >
      Download {name} →
    </a>
  );
}

interface NodeProps {
  value: unknown;
  depth: number;
  name?: string;
  isLast: boolean;
}

function JsonNode({ value, depth, name, isLast }: NodeProps) {
  const isContainer = value !== null && typeof value === "object";
  const [open, setOpen] = useState(depth < AUTO_COLLAPSE_DEPTH);

  const key = name === undefined ? null : <span className="text-code-keyword">{`"${name}"`}: </span>;
  const comma = isLast ? null : <span className="text-code-comment">,</span>;

  if (!isContainer) {
    return (
      <div style={{ paddingLeft: depth * 12 }}>
        {key}
        <Scalar value={value} />
        {comma}
      </div>
    );
  }

  const isArray = Array.isArray(value);
  const entries: [string, unknown][] = isArray
    ? (value as unknown[]).map((item, index) => [String(index), item])
    : Object.entries(value as Record<string, unknown>);

  const open_ = isArray ? "[" : "{";
  const close = isArray ? "]" : "}";

  if (entries.length === 0) {
    return (
      <div style={{ paddingLeft: depth * 12 }}>
        {key}
        <span className="text-code-comment">{`${open_}${close}`}</span>
        {comma}
      </div>
    );
  }

  return (
    <div style={{ paddingLeft: depth * 12 }}>
      <button
        type="button"
        onClick={() => setOpen((previous) => !previous)}
        aria-expanded={open}
        className="inline-flex items-center gap-1 rounded-xs text-left hover:bg-surface-hover"
      >
        <ChevronRight
          aria-hidden
          className={cn("size-3 shrink-0 transition-transform duration-120", open && "rotate-90")}
          strokeWidth={1.5}
        />
        {key}
        <span className="text-code-comment">{open_}</span>
        {!open ? (
          <span className="text-code-comment">
            {` ${entries.length} ${entries.length === 1 ? "entry" : "entries"} ${close}`}
          </span>
        ) : null}
      </button>

      {open ? (
        <>
          {entries.map(([entryKey, entryValue], index) => (
            <JsonNode
              key={entryKey}
              name={isArray ? undefined : entryKey}
              value={entryValue}
              depth={depth + 1}
              isLast={index === entries.length - 1}
            />
          ))}
          <div style={{ paddingLeft: depth * 12 }}>
            <span className="text-code-comment">{close}</span>
            {comma}
          </div>
        </>
      ) : (
        comma
      )}
    </div>
  );
}

function Scalar({ value }: { value: unknown }) {
  if (value === null) return <span className="text-code-comment">null</span>;
  switch (typeof value) {
    case "string":
      // Rendered as a text node: no markup in this string can execute.
      return <span className="text-code-string break-all">{`"${value}"`}</span>;
    case "number":
      return <span className="text-code-number">{String(value)}</span>;
    case "boolean":
      return <span className="text-code-number">{String(value)}</span>;
    default:
      return <span className="text-code-comment">{String(value)}</span>;
  }
}

/** Copy an immutable request with client-side redaction already applied. */
export function CopyRedactedJson({ value, label }: { value: unknown; label: string }) {
  return <CopyButton value={toRedactedJson(value)} label={label} />;
}
