import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TurnGroup } from "@/hooks/use-processed-messages";
import { ActivityChip } from "./activity-chip";
import { ActivityStatusLineView } from "./activity-status-line";

vi.mock("@/components/task/chat/messages/turn-group-message", () => ({
  TurnGroupContent: () => <div data-testid="chip-rows" />,
}));

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

const group: TurnGroup = { type: "turn_group", id: "activity-chip-t-0", turnId: "t", messages: [] };
const props = { group, sessionId: "s", permissionsByToolCallId: new Map() };

describe("ActivityChip", () => {
  it("is collapsed by default and expands to the rows", () => {
    render(<ActivityChip {...props} chip={{ count: 3, failed: 0, durationSeconds: 9 }} />);
    const button = screen.getByRole("button", { name: "Checked 3 sources · 9s" });
    expect(button.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByTestId("chip-rows")).toBeNull();
    fireEvent.click(button);
    expect(button.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByTestId("chip-rows")).toBeTruthy();
  });

  it("states the failed count as text and drops a missing duration", () => {
    render(<ActivityChip {...props} chip={{ count: 1, failed: 2, durationSeconds: null }} />);
    expect(screen.getByRole("button", { name: "Checked 1 source · 2 failed" })).toBeTruthy();
  });
});

describe("ActivityStatusLineView", () => {
  it("announces the verb only and counts the elapsed seconds", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-09-29T10:00:04Z"));
    render(
      <ActivityStatusLineView
        line={{
          turnId: "t",
          verbKey: "coordinator:activityVerbListTasks",
          startedAtMs: Date.parse("2026-09-29T10:00:00Z"),
        }}
      />,
    );
    const status = screen.getByRole("status");
    expect(status.getAttribute("aria-live")).toBe("polite");
    expect(status.textContent).toContain("Reading tasks");
    const seconds = status.querySelector("[aria-hidden='true']");
    expect(seconds?.textContent).toBe("4s");
    act(() => {
      vi.advanceTimersByTime(2000);
    });
    expect(status.querySelector("[aria-hidden='true']")?.textContent).toBe("6s");
  });
});
