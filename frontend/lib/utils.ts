import { clsx, type ClassValue } from "clsx";
import { extendTailwindMerge } from "tailwind-merge";

/**
 * tailwind-merge has to be told about the type scale this system adds.
 *
 * It classifies an unrecognised `text-*` as a colour, so `text-micro`,
 * `text-meta` and `text-caption` were read as colours and silently displaced
 * the real one: every `<Button size="sm">` lost its variant's `text-*` and fell
 * back to inheriting the page colour — invisible on a filled button.
 */
const twMerge = extendTailwindMerge({
  extend: {
    classGroups: {
      "font-size": [{ text: ["micro", "meta", "caption"] }],
    },
  },
});

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
