import * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"
import { Slot } from "radix-ui"

import { cn } from "@/lib/utils"

/**
 * Design-system alignment: badges are square-ish (2px). The pill radius is
 * reserved for true tags and filters, which is what the `tag` variant is for.
 * Nothing here carries hue — see `StatusBadge` for how state is signalled.
 */
const badgeVariants = cva(
  "inline-flex w-fit shrink-0 items-center justify-center gap-1 overflow-hidden rounded-xs border border-transparent px-2 py-0.5 font-mono text-micro font-medium whitespace-nowrap transition-[color,background-color,border-color] duration-120 ease-mech focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--nr-border-focus) [&>svg]:pointer-events-none [&>svg]:size-3",
  {
    variants: {
      variant: {
        default: "bg-primary text-primary-foreground [a&]:hover:bg-(--nr-accent-hover)",
        secondary:
          "bg-surface-inset text-fg [a&]:hover:bg-surface-active",
        outline:
          "border-line-default text-fg-secondary [a&]:hover:bg-surface-hover [a&]:hover:text-fg",
        /** Roadmap / not-yet-available. Dashed, never filled. */
        dotted:
          "border-dashed border-line-default text-fg-muted [a&]:hover:text-fg-secondary",
        ghost: "text-fg-muted [a&]:hover:bg-surface-hover [a&]:hover:text-fg",
        tag: "rounded-full border-line-default px-2.5 text-fg-secondary [a&]:hover:bg-surface-hover",
        link: "text-fg underline-offset-4 [a&]:hover:underline",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  }
)

function Badge({
  className,
  variant = "default",
  asChild = false,
  ...props
}: React.ComponentProps<"span"> &
  VariantProps<typeof badgeVariants> & { asChild?: boolean }) {
  const Comp = asChild ? Slot.Root : "span"

  return (
    <Comp
      data-slot="badge"
      data-variant={variant}
      className={cn(badgeVariants({ variant }), className)}
      {...props}
    />
  )
}

export { Badge, badgeVariants }
