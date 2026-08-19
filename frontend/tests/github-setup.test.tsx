import { StrictMode } from "react";
import { describe, expect, it, vi } from "vitest";
import { act, render, screen, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { toast } from "sonner";

import SetupPage from "@/app/(dashboard)/organization/github/setup/page";
import { apiUrl, server } from "./msw/server";
import { Providers } from "./utils";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

// The query string the page is meant to read. It arrives through the router,
// not as a prop: handing the component a resolved promise was what let this
// pass while production could not find the id at all.
const navigation = vi.hoisted(() => ({ params: new URLSearchParams() }));

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: vi.fn(),
    replace: vi.fn(),
    refresh: vi.fn(),
    back: vi.fn(),
    forward: vi.fn(),
    prefetch: vi.fn(),
  }),
  usePathname: () => "/",
  useSearchParams: () => navigation.params,
}));

const INSTALLATION = {
  id: "ghi_01HXQ8F2ACME",
  organization_id: "org_01HXQ8F2ACME",
  installation_id: 153767247,
  account_login: "acme",
  created_at: "2026-08-14T09:00:00Z",
  updated_at: "2026-08-14T09:00:00Z",
};

async function renderSetup(params: Record<string, string>) {
  navigation.params = new URLSearchParams(params);
  await act(async () => {
    render(
      // Strict mode, because the dashboard runs in it: React remounts every
      // effect once, and anything fired from mount has to survive that.
      //
      // No Suspense here on purpose. The page brings its own, and supplying one
      // is what hid the missing boundary that broke the built route.
      <StrictMode>
        <Providers>
          <SetupPage />
        </Providers>
      </StrictMode>,
    );
  });
}

describe("github setup", () => {
  it("reads the installation id GitHub put in the URL", async () => {
    server.use(
      http.post(apiUrl("/v1/github/installation"), () =>
        HttpResponse.json(INSTALLATION, { status: 201 }),
      ),
    );

    await renderSetup({ installation_id: "153767247", setup_action: "update" });

    expect(
      screen.queryByText(/did not send an installation id/i),
    ).not.toBeInTheDocument();
  });

  it("says the installation was refused rather than claiming to record it", async () => {
    server.use(
      http.post(apiUrl("/v1/github/installation"), () =>
        HttpResponse.json(
          { error: { code: "invalid_request", message: "github does not know this installation" } },
          { status: 422 },
        ),
      ),
    );

    await renderSetup({ installation_id: "153767247" });

    expect(
      await screen.findByText(/github does not know this installation/i),
    ).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "GitHub not connected" })).toBeInTheDocument();
    expect(screen.queryByText(/Recording installation/)).not.toBeInTheDocument();
  });

  it("announces the account once the server has recorded it", async () => {
    server.use(
      http.post(apiUrl("/v1/github/installation"), () =>
        HttpResponse.json(INSTALLATION, { status: 201 }),
      ),
    );

    await renderSetup({ installation_id: "153767247" });

    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Connected acme"));
  });

  it("only says GitHub sent nothing when it really sent nothing", async () => {
    await renderSetup({});

    expect(
      await screen.findByText(/did not send an installation id/i),
    ).toBeInTheDocument();
  });
});
