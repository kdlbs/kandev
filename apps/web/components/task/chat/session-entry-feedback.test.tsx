import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const breakpoint = vi.hoisted(() => ({ isFinePointer: true }));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => breakpoint,
}));

import { SessionHistoryFeedback } from "./session-entry-feedback";

afterEach(() => {
  cleanup();
  breakpoint.isFinePointer = true;
});

describe("SessionHistoryFeedback", () => {
  it("announces loading without showing the empty conversation invitation", () => {
    render(<SessionHistoryFeedback status="loading" error={null} onRetry={vi.fn()} />);

    expect(screen.getByTestId("session-history-loading").getAttribute("role")).toBe("status");
    expect(screen.getByText("Loading conversation...")).toBeTruthy();
    expect(screen.queryByText("No messages yet. Start the conversation!")).toBeNull();
  });

  it("shows a retry and collapsed technical detail after history fails", () => {
    const onRetry = vi.fn();
    render(
      <SessionHistoryFeedback
        status="unavailable"
        error={new Error("WebSocket request timed out: message.list")}
        onRetry={onRetry}
      />,
    );

    fireEvent.click(screen.getByTestId("session-history-retry"));
    expect(onRetry).toHaveBeenCalledTimes(1);
    const details = screen.getByTestId("session-history-details") as HTMLDetailsElement;
    expect(details.open).toBe(false);

    fireEvent.click(screen.getByTestId("session-history-details-summary"));
    expect(details.open).toBe(true);
    expect(screen.getByText("WebSocket request timed out: message.list")).toBeTruthy();
  });

  it("uses a touch-sized retry control for coarse pointers", () => {
    breakpoint.isFinePointer = false;
    render(<SessionHistoryFeedback status="unavailable" error={null} onRetry={vi.fn()} />);

    expect(screen.getByTestId("session-history-retry").className).toContain("min-h-11");
  });
});
