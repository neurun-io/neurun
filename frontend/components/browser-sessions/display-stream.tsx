"use client";

import { useEffect, useRef, useState } from "react";
import type RFBType from "@novnc/novnc/lib/rfb";

import { API_BASE_URL } from "@/lib/api/client";

/**
 * A session's framebuffer, streamed.
 *
 * The socket carries RFB straight from the session's VNC server, proxied by the
 * control plane — which is the only thing that reaches that port, and the only
 * thing that checks who is watching. Nothing here knows where the display
 * actually is; it names a session and asks.
 *
 * View-only by default: watching is a diagnostic, and a stray click landing in
 * somebody's signed-in browser is not.
 */
export function DisplayStream({
  sessionId,
  interactive = false,
}: {
  sessionId: string;
  interactive?: boolean;
}) {
  const target = useRef<HTMLDivElement>(null);
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const mount = target.current;
    if (!mount) return;

    const url = `${API_BASE_URL.replace(/^http/, "ws")}/v1/browser-sessions/${encodeURIComponent(
      sessionId,
    )}/display`;

    let client: { disconnect: () => void } | null = null;
    let cancelled = false;

    (async () => {
      // The package's exports map only allows the bare specifier at runtime,
      // while the types are only declared on the subpath — so the import is
      // untyped and the type is taken from where it is actually declared.
      const { default: RFB } = (await import(
        /* webpackIgnore: false */ "@novnc/novnc"
      )) as unknown as { default: typeof RFBType };
      if (cancelled) return;
      const instance = new RFB(mount, url);
      instance.scaleViewport = true;
      instance.viewOnly = !interactive;
      instance.addEventListener("connect", () => setConnected(true));
      instance.addEventListener("disconnect", (event: { detail?: { clean?: boolean } }) => {
        setConnected(false);
        if (!event?.detail?.clean) setError("The display stopped responding");
      });
      instance.addEventListener("securityfailure", (event: { detail?: { reason?: string } }) => {
        setError(event?.detail?.reason ?? "The display refused the connection");
      });
      client = instance;
    })().catch((cause) => setError((cause as Error).message));

    return () => {
      cancelled = true;
      client?.disconnect();
    };
  }, [sessionId, interactive]);

  return (
    <div className="relative overflow-hidden rounded-lg border border-line bg-black">
      <div ref={target} className="aspect-video w-full" />
      {!connected || error ? (
        <p className="absolute inset-x-0 bottom-0 bg-black/70 px-3 py-1 font-mono text-micro text-fg-muted">
          {error ?? "Connecting…"}
        </p>
      ) : null}
    </div>
  );
}
