"use client";

import { useState, type ReactNode } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { ThemeProvider } from "next-themes";
import { TooltipProvider } from "@/components/ui/tooltip";
import { Toaster } from "@/components/ui/sonner";
import { createQueryClient } from "@/lib/api/query-client";
import { SessionProvider } from "@/lib/session/store";
import { PreferencesProvider } from "@/lib/preferences/store";

export function Providers({ children }: { children: ReactNode }) {
  // One client per browser session. Created in state so a re-render never
  // swaps the cache out from under an in-flight query.
  const [queryClient] = useState(createQueryClient);

  return (
    <ThemeProvider
      attribute="data-theme"
      defaultTheme="dark"
      themes={["dark", "light"]}
      // Dark is the brand's home, not a system-derived preference.
      enableSystem={false}
      storageKey="neurun-theme"
      disableTransitionOnChange
    >
      <QueryClientProvider client={queryClient}>
        <SessionProvider>
            <PreferencesProvider>
              <TooltipProvider delayDuration={200}>
                {children}
                <Toaster position="bottom-right" />
              </TooltipProvider>
            </PreferencesProvider>
        </SessionProvider>
      </QueryClientProvider>
    </ThemeProvider>
  );
}
