import { cn } from "@/lib/utils";

/**
 * Loaded from /logo.svg (a filled black diagram) rather than drawn inline,
 * since it's a single traced path, not a small set of strokes.
 */
export function Logo({
  className,
  title = "Neurun",
}: {
  className?: string;
  title?: string;
}) {
  return (
    // eslint-disable-next-line @next/next/no-img-element -- next/image will not optimise an SVG without dangerouslyAllowSVG
    <img
      src="/logo.svg"
      alt={title}
      className={cn("size-10 shrink-0 object-contain", className)}
    />
  );
}

export function Wordmark({ className }: { className?: string }) {
  return (
    <span className={cn("inline-flex items-center gap-2", className)}>
      <Logo />
      <span className="text-lg font-medium tracking-title text-fg">neurun</span>
    </span>
  );
}
