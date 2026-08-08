"use client";

import { useEffect, useState } from "react";

import { useReducedMotion } from "@/lib/view/use-reduced-motion";

/**
 * The runtime, drawn as a field rather than a dashboard.
 *
 * Everything a scraper reaches for is already wired to the run — it installs
 * nothing — and the engine walks those connections, out and back. Each
 * leg picks its own service, stops somewhere along the way, and reports
 * something that line could actually produce. Nothing else moves.
 *
 * The walk has to be unpredictable, so it is driven here rather than in CSS.
 * The first leg is fixed so server and client agree on the first frame, and it
 * doubles as the settled frame under reduced motion.
 */
const CENTER = 150;
const ORBIT = 100;
const LABEL = 122;

const SERVICES = [
  {
    name: "browser",
    issues: ["tls fingerprint drift", "webgl vendor mismatch", "headless flag exposed"],
  },
  { name: "http", issues: ["429 · backing off", "keep-alive dropped", "redirect loop cut"] },
  { name: "parse", issues: ["selector missed", "encoding fell back", "html shape changed"] },
  { name: "storage", issues: ["write retried", "payload truncated", "checksum re-read"] },
  { name: "proxy", issues: ["exit node rotated", "pool exhausted", "geo off profile"] },
  { name: "deps", issues: ["lockfile drift", "wheel cache cold", "abi mismatch"] },
];

/** Out to the stop, hold there, on to the service, back to the run. */
const PHASES = ["out", "hold", "reach", "back"] as const;
type Phase = (typeof PHASES)[number];

const MS: Record<Phase, number> = { out: 760, hold: 1150, reach: 520, back: 820 };

interface Leg {
  index: number;
  /** How far along the spoke the engine stops. */
  stop: number;
  issue: string;
}

const FIRST: Leg = { index: 0, stop: 0.52, issue: SERVICES[0].issues[0] };

function pick<T>(items: T[]): T {
  return items[Math.floor(Math.random() * items.length)];
}

/** Anywhere but the line it just walked. */
function nextLeg(previous: number): Leg {
  const index = pick(SERVICES.map((_, at) => at).filter((at) => at !== previous));
  return { index, stop: 0.32 + Math.random() * 0.38, issue: pick(SERVICES[index].issues) };
}

function place(index: number, radius: number) {
  const angle = ((index / SERVICES.length) * 2 - 0.5) * Math.PI;
  return { x: CENTER + radius * Math.cos(angle), y: CENTER + radius * Math.sin(angle) };
}

export function RuntimeField() {
  const [leg, setLeg] = useState(FIRST);
  // Starting home means the first move is a randomly chosen one, and the
  // server-rendered frame is simply the engine at rest.
  const [phase, setPhase] = useState<Phase>("back");
  const reduced = useReducedMotion();

  useEffect(() => {
    if (reduced) return;
    const timer = setTimeout(() => {
      if (phase !== "back") {
        setPhase(PHASES[PHASES.indexOf(phase) + 1]);
        return;
      }
      setLeg((previous) => nextLeg(previous.index));
      setPhase("out");
    }, MS[phase]);
    return () => clearTimeout(timer);
  }, [phase, reduced]);

  const node = place(leg.index, ORBIT);
  const caught = place(leg.index, ORBIT * leg.stop);
  // Reduced motion parks the engine at the stop with its report showing.
  const holding = reduced || phase === "hold";
  const engine =
    reduced || phase === "out" || phase === "hold"
      ? caught
      : phase === "reach"
        ? node
        : { x: CENTER, y: CENTER };

  return (
    <svg viewBox="0 0 300 300" role="img" className="w-full max-w-105 self-center">
      <title>An execution inside the runtime</title>
      <desc>
        The services a scraper needs sit wired inside the runtime boundary. The engine walks those
        connections and stops to report what it finds — here, {leg.issue} on the {
          SERVICES[leg.index].name
        } line.
      </desc>

      {SERVICES.map((service, index) => {
        const spoke = place(index, ORBIT);
        const label = place(index, LABEL);
        const anchor = label.x > CENTER + 2 ? "start" : label.x < CENTER - 2 ? "end" : "middle";
        const dy = label.y > CENTER ? 10 : label.y < CENTER - 40 ? -8 : 3;

        return (
          <g key={service.name}>
            {/* Wired, permanently: the run installs none of this. */}
            <line
              x1={CENTER}
              y1={CENTER}
              x2={spoke.x}
              y2={spoke.y}
              className="stroke-fg-faint opacity-40"
              strokeWidth={1}
            />
            <circle cx={spoke.x} cy={spoke.y} r={3.5} className="fill-fg-muted" />
            <text
              x={label.x}
              y={label.y + dy}
              textAnchor={anchor}
              fontSize={9}
              className="fill-fg-muted font-mono"
            >
              {service.name}
            </text>
          </g>
        );
      })}

      {/* What the stop was for, named where it happened. */}
      <g className="transition-opacity duration-200" opacity={holding ? 1 : 0}>
        <circle
          cx={caught.x}
          cy={caught.y}
          r={8}
          className="fill-none stroke-(--nr-accent)"
          strokeWidth={1}
          strokeDasharray="3 3"
        />
        <text
          x={caught.x + (caught.x < CENTER ? -14 : 14)}
          y={caught.y - 6}
          textAnchor={caught.x < CENTER ? "end" : "start"}
          fontSize={9}
          className="fill-fg font-mono"
        >
          {leg.issue}
        </text>
      </g>

      {/* The engine. */}
      <circle
        cx={CENTER}
        cy={CENTER}
        r={3}
        className="fill-(--nr-accent)"
        style={{
          transformBox: "view-box",
          transform: `translate(${engine.x - CENTER}px, ${engine.y - CENTER}px)`,
          transition: `transform ${MS[phase]}ms linear`,
        }}
      />

      {/* The run itself. */}
      <circle cx={CENTER} cy={CENTER} r={4.5} className="fill-(--nr-accent)" />
    </svg>
  );
}
