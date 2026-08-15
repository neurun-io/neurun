import { cn } from "@/lib/utils";

/**
 * Loaded from /logo.svg rather than drawn inline, since it's a single traced
 * path, not a small set of strokes. One fill per theme, both rendered and one
 * hidden in CSS: an img cannot inherit currentColor, and swapping the src on
 * the client would show the wrong ink until the theme resolved.
 */
export function Logo({
  className,
  title = "Neurun",
}: {
  className?: string;
  title?: string;
}) {
  const shared = cn("size-10 shrink-0 object-contain", className);
  return (
    <>
      {/* eslint-disable-next-line @next/next/no-img-element -- next/image will not optimise an SVG without dangerouslyAllowSVG */}
      <img src="/logo.svg" alt={title} className={cn(shared, "dark:hidden")} />
      {/* eslint-disable-next-line @next/next/no-img-element -- as above */}
      <img src="/logo-dark.svg" alt="" aria-hidden className={cn(shared, "hidden dark:block")} />
    </>
  );
}

export function Wordmark({
  className,
  logoClassName = "size-10",
}: {
  className?: string;
  logoClassName?: string;
}) {
  return (
    <span className={cn("inline-flex items-center gap-2 text-lg", className)}>
      <Logo className={logoClassName} />
      <span className="font-medium tracking-title text-fg">neurun</span>
    </span>
  );
}
