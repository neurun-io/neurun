import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { TooltipProvider } from "@/components/ui/tooltip";
import { ConnectionProvider } from "@/lib/connection/store";
import { CapabilityProvider } from "@/lib/connection/capability";
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
      <ConnectionProvider>
        <CapabilityProvider>
          <PreferencesProvider>
            <TooltipProvider>{children}</TooltipProvider>
          </PreferencesProvider>
        </CapabilityProvider>
      </ConnectionProvider>
    </QueryClientProvider>
  );
}
