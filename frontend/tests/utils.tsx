import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { TooltipProvider } from "@/components/ui/tooltip";
import { SessionProvider } from "@/lib/session/store";
import { CapabilityProvider } from "@/lib/session/capability";
import { PreferencesProvider } from "@/lib/preferences/store";

export function createTestQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0, staleTime: 0 },
      mutations: { retry: false },
    },
  });
}

/** The provider stack a component sees inside the dashboard. */
export function Providers({
  children,
  queryClient = createTestQueryClient(),
}: {
  children: ReactNode;
  queryClient?: QueryClient;
}) {
  return (
    <QueryClientProvider client={queryClient}>
      <SessionProvider>
        <CapabilityProvider>
          <PreferencesProvider>
            <TooltipProvider>{children}</TooltipProvider>
          </PreferencesProvider>
        </CapabilityProvider>
      </SessionProvider>
    </QueryClientProvider>
  );
}
