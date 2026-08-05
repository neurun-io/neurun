"use client";

import { useTheme } from "next-themes";
import { Moon, Sun } from "lucide-react";

import { Button } from "@/components/ui/button";

export function ThemeToggle() {
  const { resolvedTheme, setTheme } = useTheme();

  return (
    <Button
      variant="ghost"
      size="icon-sm"
      onClick={() => setTheme(resolvedTheme === "light" ? "dark" : "light")}
      aria-label="Switch colour mode"
    >
      <Moon aria-hidden className="size-3.5 dark:hidden" strokeWidth={1.5} />
      <Sun aria-hidden className="size-3.5 light:hidden" strokeWidth={1.5} />
    </Button>
  );
}
