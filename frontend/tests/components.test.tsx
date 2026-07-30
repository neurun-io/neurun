import { describe, expect, it } from "vitest";
import { act, render, renderHook, screen, within } from "@testing-library/react";

import { StatusBadge } from "@/components/neurun/status-badge";
import { ErrorPanel } from "@/components/neurun/error-panel";
import { EventTimeline } from "@/components/neurun/event-timeline";
import { StateFlow } from "@/components/neurun/state-flow";
import { RoadmapRoute } from "@/components/neurun/feedback";
import { NeurunApiError } from "@/lib/api/errors";
import { useCapability } from "@/lib/connection/capability";
import { useCursorPages } from "@/lib/view/use-cursor-pages";
import { ROADMAP } from "@/lib/roadmap";
import { Providers } from "./utils";
import { jobEvents } from "./msw/fixtures";

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
  it("always exposes a copyable request ID", () => {
    render(
      <Providers>
        <ErrorPanel
          error={
            new NeurunApiError({
              status: 400,
              code: "invalid_request",
              message: "$.input.message: must be a string",
              requestId: "req_01HXQ8F2INVALID",
              traceId: "trc_01HXQ8F2",
            })
          }
        />
      </Providers>,
    );

    expect(screen.getByText("400 invalid_request")).toBeInTheDocument();
    // The server's human-readable path survives verbatim.
    expect(screen.getByText("$.input.message: must be a string")).toBeInTheDocument();
    expect(screen.getByText("req_01HXQ8F2INVALID")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Copy request ID" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Copy trace ID" })).toBeInTheDocument();
  });
});

describe("EventTimeline", () => {
  it("preserves the server's order, including the accepted step", () => {
    render(
      <Providers>
        <EventTimeline events={jobEvents} />
      </Providers>,
    );

    const items = screen.getAllByRole("listitem");
    expect(items).toHaveLength(3);
    expect(within(items[0]).getByText("job.accepted")).toBeInTheDocument();
    expect(within(items[1]).getByText("job.queued")).toBeInTheDocument();
    expect(within(items[2]).getByText("attempt.leased")).toBeInTheDocument();
  });

  it("says so plainly when nothing has been recorded", () => {
    render(
      <Providers>
        <EventTimeline events={[]} />
      </Providers>,
    );
    expect(screen.getByText("No events recorded yet")).toBeInTheDocument();
  });
});

describe("StateFlow", () => {
  it("marks the current stage for assistive technology", () => {
    render(<StateFlow state="running" />);
    expect(screen.getByText("running")).toHaveAttribute("aria-current", "step");
  });

  it("appends an unrecognised state as its own terminal stage", () => {
    render(<StateFlow state="quarantined" />);
    expect(screen.getByText("quarantined")).toBeInTheDocument();
  });
});

describe("RoadmapRoute", () => {
  it("states that the capability is unavailable and names what is missing", () => {
    render(<RoadmapRoute {...ROADMAP.sessions} />);

    expect(screen.getByRole("heading", { name: "Sessions" })).toBeInTheDocument();
    expect(screen.getByText("not in this release")).toBeInTheDocument();
    expect(
      screen.getByText(/session create \/ list \/ detail \/ keepalive/),
    ).toBeInTheDocument();
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

describe("capability tracking", () => {
  it("records process_local durability without ever calling it durable", () => {
    const { result } = renderHook(() => useCapability(), { wrapper: Providers });

    expect(result.current.isProcessLocal).toBe(false);
    act(() => result.current.recordDurability("process_local"));

    expect(result.current.durability).toBe("process_local");
    expect(result.current.isProcessLocal).toBe(true);
    expect(result.current.isDurable).toBe(false);
    // An accepted async mutation also proves async is enabled here.
    expect(result.current.asyncAvailability).toBe("available");
  });

  it("gates async after durable_backend_unavailable", () => {
    const { result } = renderHook(() => useCapability(), { wrapper: Providers });

    expect(result.current.asyncAvailability).toBe("unknown");
    act(() => result.current.recordAsyncUnavailable());
    expect(result.current.asyncAvailability).toBe("unavailable");
  });

  it("treats an unknown durability value as not durable", () => {
    const { result } = renderHook(() => useCapability(), { wrapper: Providers });

    act(() => result.current.recordDurability("replicated"));
    expect(result.current.durability).toBe("replicated");
    expect(result.current.isDurable).toBe(false);
    expect(result.current.isProcessLocal).toBe(false);
  });
});
