"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Expand, Maximize2, Minimize2, MousePointerClick, Eye } from "lucide-react";
import type RFBType from "@novnc/novnc/lib/rfb";

import { Button } from "@/components/ui/button";
import { API_BASE_URL } from "@/lib/api/client";

/**
 * A session's framebuffer, streamed.
 *
 * The socket carries RFB straight from the session's VNC server, proxied by the
 * control plane — which is the only thing that reaches that port, and the only
 * thing that checks who is watching. Nothing here knows where the display
 * actually is; it names a session and asks.
 *
 * It opens view-only, and taking control is a deliberate click: watching is a
 * diagnostic, and a stray click landing in somebody's signed-in browser is not.
 * Control is handed over on the live connection rather than by reconnecting,
 * so the session is never interrupted to change your mind.
 */
export function DisplayStream({
  sessionId,
  interactive = false,
}: {
  sessionId: string;
  interactive?: boolean;
}) {
  const target = useRef<HTMLDivElement>(null);
  const frame = useRef<HTMLDivElement>(null);
  const client = useRef<RFBType | null>(null);
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [controlling, setControlling] = useState(interactive);
  const [expanded, setExpanded] = useState(false);
  const [fullscreen, setFullscreen] = useState(false);

  useEffect(() => {
    const mount = target.current;
    if (!mount) return;

    const url = `${API_BASE_URL.replace(/^http/, "ws")}/v1/browser-sessions/${encodeURIComponent(
      sessionId,
    )}/display`;

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
      instance.viewOnly = true;
      instance.addEventListener("connect", () => setConnected(true));
      instance.addEventListener("disconnect", (event: { detail?: { clean?: boolean } }) => {
        setConnected(false);
        if (!event?.detail?.clean) setError("The display stopped responding");
      });
      instance.addEventListener("securityfailure", (event: { detail?: { reason?: string } }) => {
        setError(event?.detail?.reason ?? "The display refused the connection");
      });
      client.current = instance;
    })().catch((cause) => setError((cause as Error).message));

    return () => {
      cancelled = true;
      client.current?.disconnect();
      client.current = null;
    };
  }, [sessionId]);

  // Set on the live connection, so control can be taken and given back without
  // dropping the session.
  useEffect(() => {
    if (client.current) client.current.viewOnly = !controlling;
  }, [controlling, connected]);

  // The element fills the screen on its own; what does not is the canvas
  // inside it, which keeps whatever box it was given and merely stretches.
  useEffect(() => {
    const sync = () => setFullscreen(document.fullscreenElement === frame.current);
    document.addEventListener("fullscreenchange", sync);
    return () => document.removeEventListener("fullscreenchange", sync);
  }, []);

  const toggleFullscreen = useCallback(() => {
    const element = frame.current;
    if (!element) return;
    if (document.fullscreenElement) {
      void document.exitFullscreen();
      return;
    }
    void element.requestFullscreen?.();
  }, []);

  return (
    <div
      ref={frame}
      className={
        fullscreen
          ? "relative flex h-screen w-screen items-center justify-center bg-black"
          : "relative overflow-hidden rounded-lg border border-line bg-black"
      }
    >
      <div
        ref={target}
        className={
          fullscreen
            ? "h-full w-full"
            : expanded
              ? "h-[80vh] w-full"
              : "aspect-video w-full"
        }
      />
      <div className="absolute right-2 top-2 flex gap-1">
        <Button
          size="sm"
          variant={controlling ? "default" : "secondary"}
          onClick={() => setControlling((taken) => !taken)}
          title={controlling ? "Stop sending input" : "Send keyboard and mouse to this browser"}
        >
          {controlling ? <MousePointerClick className="size-3" /> : <Eye className="size-3" />}
          {controlling ? "Controlling" : "View only"}
        </Button>
        <Button
          size="sm"
          variant="secondary"
          onClick={() => setExpanded((open) => !open)}
          title={expanded ? "Shrink" : "Enlarge"}
        >
          {expanded ? <Minimize2 className="size-3" /> : <Maximize2 className="size-3" />}
        </Button>
        <Button size="sm" variant="secondary" onClick={toggleFullscreen} title="Fullscreen">
          <Expand className="size-3" />
        </Button>
      </div>
      {!connected || error ? (
        <p className="absolute inset-x-0 bottom-0 bg-black/70 px-3 py-1 font-mono text-micro text-fg-muted">
          {error ?? "Connecting…"}
        </p>
      ) : null}
    </div>
  );
}
