"use client";

import { useTheme } from "next-themes";
import { Moon, Sun } from "lucide-react";

import { Button } from "@/components/ui/button";

/**
 * Light/dark switch.
 *
 * Shared by the dashboard chrome and the sign-in screen so the theme can be set
 * before signing in, rather than only once the dashboard has loaded.
 *
 * The icon shows the theme the button switches *to*, matching the label.
 */
export function ThemeToggle() {
  const { resolvedTheme, setTheme } = useTheme();
  const next = resolvedTheme === "light" ? "dark" : "light";

  return (
    <Button
      variant="ghost"
      size="icon-sm"
      onClick={() => setTheme(next)}
      aria-label={`Switch to ${next} theme`}
    >
      {resolvedTheme === "light" ? (
        <Moon aria-hidden className="size-3.5" strokeWidth={1.5} />
      ) : (
        <Sun aria-hidden className="size-3.5" strokeWidth={1.5} />
      )}
    </Button>
  );
}
