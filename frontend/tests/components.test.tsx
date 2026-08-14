import { describe, expect, it } from "vitest";
import { act, fireEvent, render, renderHook, screen, waitFor } from "@testing-library/react";
import { ThemeProvider } from "next-themes";

import { ProfileForm, type ProfileValues } from "@/components/browser-profiles/profile-form";

import { ThemeToggle } from "@/components/neurun/theme-toggle";
import { LoginScreen } from "@/components/auth/login-screen";

import { StatusBadge } from "@/components/neurun/status-badge";
import { ErrorPanel } from "@/components/neurun/error-panel";
import { UnbuiltRoute } from "@/components/neurun/feedback";
import { NeurunApiError } from "@/lib/api/errors";
import { useCursorPages } from "@/lib/view/use-cursor-pages";
import { ROADMAP } from "@/lib/roadmap";
import { Providers } from "./utils";

describe("StatusBadge", () => {
  it("renders a known status with its treatment and a text description", () => {
    render(<StatusBadge status="succeeded" />);

    const badge = screen.getByText("succeeded").closest("[data-status]")!;
    expect(badge).toHaveAttribute("data-treatment", "solid");
    expect(badge).toHaveAttribute("data-known", "true");
    // Status is conveyed as text as well as treatment.
    expect(badge).toHaveTextContent("Completed successfully");
  });

  it("renders an unknown status neutrally, carrying its raw value", () => {
    render(<StatusBadge status="quarantined" />);

    const badge = screen.getByText("quarantined").closest("[data-status]")!;
    expect(badge).toHaveAttribute("data-treatment", "neutral");
    expect(badge).toHaveAttribute("data-known", "false");
    expect(badge).toHaveTextContent("Unrecognised status");
  });

  it("does not throw on a missing status", () => {
    expect(() => render(<StatusBadge status={undefined} />)).not.toThrow();
  });

  it("gives a validation rejection a different treatment from a failure", () => {
    const { unmount } = render(<StatusBadge status="rejected" />);
    const rejected = screen.getByText("rejected").closest("[data-status]")!;
    expect(rejected).toHaveAttribute("data-treatment", "rejected");
    unmount();

    render(<StatusBadge status="failed" />);
    expect(screen.getByText("failed").closest("[data-status]")).toHaveAttribute(
      "data-treatment",
      "inverted",
    );
  });
});

describe("ErrorPanel", () => {
  it("names the refusal and prints the server's message verbatim", () => {
    render(
      <Providers>
        <ErrorPanel
          error={
            new NeurunApiError({
              status: 400,
              code: "invalid_request",
              message: "$.input.message: must be a string",
            })
          }
        />
      </Providers>,
    );

    expect(screen.getByText("400 invalid_request")).toBeInTheDocument();
    // The server's human-readable path survives verbatim.
    expect(screen.getByText("$.input.message: must be a string")).toBeInTheDocument();
  });
});

describe("UnbuiltRoute", () => {
  it("renders the route's empty state rather than any record", () => {
    render(<UnbuiltRoute {...ROADMAP.servers} />);

    expect(screen.getByRole("heading", { name: "Servers" })).toBeInTheDocument();
    expect(screen.getByText("No servers")).toBeInTheDocument();
    expect(screen.getByText(/An app is executed, not hosted/)).toBeInTheDocument();
  });
});

describe("ProfileForm", () => {
  it("hides the identity until asked, then fills it from the catalogue", async () => {
    const submitted: ProfileValues[] = [];
    render(
      <Providers>
        <ProfileForm
          submitLabel="Create profile"
          pending={false}
          onSubmit={(values) => submitted.push(values)}
        />
      </Providers>,
    );

    // Every profile wears one; this form just does not open with it on screen.
    expect(screen.getByText("Hidden")).toBeInTheDocument();
    expect(screen.queryByLabelText("WebGL vendor")).not.toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "shopper" } });

    // The switch waits for the catalogue: there is nothing coherent to fill in
    // until the server has said what exists.
    const identity = screen.getByRole("switch");
    await waitFor(() => expect(identity).toBeEnabled());
    await act(async () => {
      identity.click();
    });

    // Fixed by the operating system rather than typed, and derived rather than
    // asked for: Win32 comes with Windows, and physical pixels with the ratio.
    expect(screen.getByLabelText("navigator.platform")).toHaveValue("Win32");
    expect(screen.getByLabelText("Physical pixels")).toHaveValue("1920×1080");
    expect(screen.getByLabelText("WebGL vendor")).toHaveValue("Google Inc. (Intel)");
    expect(screen.getByText(/Reported to UA-CH as 15\.0\.0/)).toBeInTheDocument();

    await act(async () => {
      screen.getByRole("button", { name: "Create profile" }).click();
    });

    expect(submitted).toHaveLength(1);
    expect(submitted[0].name).toBe("shopper");
    // The fields are text while they are typed; the parse happens once, here.
    expect(submitted[0].identity).toMatchObject({
      os: "Windows",
      os_version: "11",
      platform: { navigator_platform: "Win32", version: "15.0.0" },
      browser: "chrome",
      browser_version: [139, 0, 6889, 109],
      language: ["en-US", "en"],
      timezone: "America/New_York",
      screen: { logical_width: 1920, original_width: 1920, density_pixel_ratio: 1 },
      gpu: { webgl_renderer: "ANGLE (Intel(R) HD Graphics 620 Direct3D11 vs_5_0 ps_5_0)" },
    });
  });
});

describe("useCursorPages", () => {
  it("walks forward with the server's cursors and back through history", () => {
    const { result } = renderHook(() => useCursorPages());

    expect(result.current.cursor).toBeUndefined();
    expect(result.current.canGoBack).toBe(false);

    act(() => result.current.next("cursor-page-2"));
    expect(result.current.cursor).toBe("cursor-page-2");
    expect(result.current.pageIndex).toBe(1);

    act(() => result.current.back());
    expect(result.current.cursor).toBeUndefined();
    expect(result.current.canGoBack).toBe(false);
  });

  it("refuses to paginate past an empty cursor", () => {
    const { result } = renderHook(() => useCursorPages());

    act(() => result.current.next(""));
    expect(result.current.pageIndex).toBe(0);
  });
});

describe("ThemeToggle", () => {
  function renderToggle() {
    return render(
      <ThemeProvider
        attribute="data-theme"
        defaultTheme="dark"
        themes={["dark", "light"]}
        enableSystem={false}
        storageKey="neurun-theme-test"
      >
        <ThemeToggle />
      </ThemeProvider>,
    );
  }

  it("offers the theme it would switch to, and switches on click", async () => {
    renderToggle();

    // The label names the destination theme, not the current one.
    const toggle = await screen.findByRole("button", { name: "Switch to light theme" });

    await act(async () => {
      toggle.click();
    });

    expect(document.documentElement).toHaveAttribute("data-theme", "light");
    expect(screen.getByRole("button", { name: "Switch to dark theme" })).toBeInTheDocument();
  });
});

describe("LoginScreen", () => {
  it("carries a theme toggle, so the theme is settable before signing in", async () => {
    render(
      <ThemeProvider
        attribute="data-theme"
        defaultTheme="dark"
        themes={["dark", "light"]}
        enableSystem={false}
        storageKey="neurun-theme-test"
      >
        <Providers>
          <LoginScreen />
        </Providers>
      </ThemeProvider>,
    );

    expect(await screen.findByRole("button", { name: /Switch to .* theme/ })).toBeInTheDocument();
  });
});
