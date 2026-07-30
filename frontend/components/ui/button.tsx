import * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"
import { Slot } from "radix-ui"

import { cn } from "@/lib/utils"

/**
 * Aligned to the Neurun design system:
 * - Control heights are 28 / 34 / 44px; radius is 4px (`rounded-md`).
 * - Hover and press change fill only. There are no scale transforms, ever.
 * - Focus is a 2px outline at 2px offset, always visible, never removed.
 * - Disabled is 42% opacity, and callers pass `aria-disabled` for reasons.
 * - `destructive` carries no hue — the system has none. It sits outlined until
 *   hover, then inverts completely; the inversion is the commitment signal.
 */
const buttonVariants = cva(
  [
    "inline-flex shrink-0 items-center justify-center gap-2 whitespace-nowrap",
    "rounded-md font-medium tracking-tight",
    "transition-[background-color,color,border-color] duration-120 ease-mech",
    "outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--nr-border-focus)",
    "disabled:pointer-events-none disabled:opacity-42",
    "aria-disabled:pointer-events-none aria-disabled:opacity-42",
    "[&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
  ].join(" "),
  {
    variants: {
      variant: {
        default:
          "bg-primary text-primary-foreground hover:bg-(--nr-accent-hover) active:bg-(--nr-accent-active)",
        secondary:
          "border border-line-default bg-surface-inset text-fg hover:border-line-inverse active:bg-surface-active",
        outline:
          "border border-line-default bg-transparent text-fg hover:bg-surface-hover active:bg-surface-active",
        ghost:
          "bg-transparent text-fg hover:bg-surface-hover active:bg-surface-active",
        destructive:
          "border border-line-strong bg-transparent text-fg hover:border-(--nr-accent) hover:bg-(--nr-accent) hover:text-(--nr-accent-contrast) active:bg-(--nr-accent-active)",
        link: "bg-transparent text-fg underline-offset-3 hover:underline",
      },
      size: {
        default: "h-8.5 px-3 text-sm",
        xs: "h-6 gap-1 px-2 text-micro [&_svg:not([class*='size-'])]:size-3",
        sm: "h-7 gap-1.5 px-2.5 text-caption",
        lg: "h-11 px-5 text-base",
        icon: "size-8.5",
        "icon-xs": "size-6 [&_svg:not([class*='size-'])]:size-3",
        "icon-sm": "size-7",
        "icon-lg": "size-11",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
)

function Button({
  className,
  variant = "default",
  size = "default",
  asChild = false,
  ...props
}: React.ComponentProps<"button"> &
  VariantProps<typeof buttonVariants> & {
    asChild?: boolean
  }) {
  const Comp = asChild ? Slot.Root : "button"

  return (
    <Comp
      data-slot="button"
      data-variant={variant}
      data-size={size}
      className={cn(buttonVariants({ variant, size, className }))}
      {...props}
    />
  )
}

export { Button, buttonVariants }
