import type { ReactNode } from "react";
import { DocsHeader, DocsNav } from "@/components/docs/chrome";

export default function DocsLayout({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-dvh flex-col">
      <DocsHeader />
      <div className="mx-auto flex w-full max-w-[1440px] items-start">
        <DocsNav />
        <main id="main" className="min-w-0 flex-1">
          {children}
        </main>
      </div>
    </div>
  );
}
