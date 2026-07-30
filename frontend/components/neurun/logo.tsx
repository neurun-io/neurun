import { cn } from "@/lib/utils";

/**
 * The neuron mark: two stems plus the axon diagonal form an "N", with a filled
 * soma at the diagonal's midpoint and asymmetric dendrites ending in nodes.
 *
 * Drawn inline rather than loaded as a file so `currentColor` and the soma
 * pulse both work. The pulse is a live-connection signal, so it is opt-in and
 * respects reduced-motion.
 */
export function Logo({
  className,
  pulse = false,
  title = "Neurun",
}: {
  className?: string;
  pulse?: boolean;
  title?: string;
}) {
  return (
    <svg
      viewBox="0 0 24 24"
      role="img"
      aria-label={title}
      className={cn("size-5 shrink-0", className)}
      fill="none"
      stroke="currentColor"
      strokeWidth={1.5}
      strokeLinecap="round"
    >
      {/* stems */}
      <path d="M5 20V4" />
      <path d="M19 20V4" />
      {/* axon */}
      <path d="M5 4l14 16" />
      {/* dendrites */}
      <path d="M5 7.5L2.2 5.6" />
      <path d="M5 12.5L1.8 13.4" />
      <path d="M19 16.5l3 1.9" />
      {/* terminal nodes */}
      <circle cx="1.9" cy="5.3" r="0.9" fill="currentColor" stroke="none" />
      <circle cx="1.5" cy="13.5" r="0.9" fill="currentColor" stroke="none" />
      <circle cx="22.3" cy="18.6" r="0.9" fill="currentColor" stroke="none" />
      {/* soma */}
      <circle
        cx="12"
        cy="12"
        r="2.4"
        fill="currentColor"
        stroke="none"
        className={pulse ? "motion-safe:animate-pulse-node" : undefined}
        style={pulse ? { transformOrigin: "12px 12px" } : undefined}
      />
    </svg>
  );
}

export function Wordmark({ className, pulse }: { className?: string; pulse?: boolean }) {
  return (
    <span className={cn("inline-flex items-center gap-2", className)}>
      <Logo pulse={pulse} />
      <span className="text-base font-medium tracking-title text-fg">neurun</span>
    </span>
  );
}
