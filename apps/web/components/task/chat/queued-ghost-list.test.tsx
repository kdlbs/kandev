import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { QueuedMessage } from "@/lib/state/slices/session/types";

const useQueueMock = vi.fn();

vi.mock("@/hooks/domains/session/use-queue", () => ({
  useQueue: (sessionId: string | null) => useQueueMock(sessionId),
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

// Mock Radix Collapsible because the real primitive pulls in a React global the
// jsdom test environment doesn't provide. We just need open/closed behavior to
// drive the assertions — the close-animation path is exercised in E2E.
vi.mock("@kandev/ui/collapsible", () => {
  const Collapsible = ({ open, children }: { open: boolean; children: ReactNode }) =>
    open ? <div data-collapsible-open="true">{children}</div> : null;
  const CollapsibleContent = ({ children }: { children: ReactNode }) => <>{children}</>;
  return { Collapsible, CollapsibleContent };
});

vi.mock("sonner", () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}));

import { QueueAffordance } from "./queued-ghost-list";

const SESSION_ID = "sess-1";
const CHIP_ID = "queue-chip";
const PANEL_ID = "queued-ghost-list";

function entry(overrides: Partial<QueuedMessage> = {}): QueuedMessage {
  return {
    id: "q-1",
    session_id: "sess-1",
    task_id: "task-1",
    content: "hello world",
    plan_mode: false,
    queued_at: "2026-05-18T00:00:00Z",
    queued_by: "user-1",
    ...overrides,
  };
}

function queueState(entries: QueuedMessage[], extra: Partial<ReturnType<typeof baseState>> = {}) {
  return { ...baseState(entries), ...extra };
}

function baseState(entries: QueuedMessage[]) {
  return {
    entries,
    count: entries.length,
    max: 10,
    isFull: false,
    isLoading: false,
    queue: vi.fn(async () => {}),
    clearAll: vi.fn(async () => {}),
    editEntry: vi.fn(async () => {}),
    removeEntry: vi.fn(async () => {}),
    refetch: vi.fn(async () => {}),
  };
}

const CHILD = <div data-testid="child-marker">input</div>;

beforeEach(() => {
  useQueueMock.mockReset();
});

afterEach(() => {
  cleanup();
});

describe("QueueAffordance", () => {
  it("renders only children when there are no queued entries", () => {
    useQueueMock.mockReturnValue(queueState([]));
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    expect(screen.getByTestId("child-marker")).toBeTruthy();
    expect(screen.queryByTestId(CHIP_ID)).toBeNull();
    expect(screen.queryByTestId(PANEL_ID)).toBeNull();
  });

  it("renders only children when sessionId is null", () => {
    useQueueMock.mockReturnValue(queueState([entry()]));
    render(<QueueAffordance sessionId={null}>{CHILD}</QueueAffordance>);
    expect(screen.getByTestId("child-marker")).toBeTruthy();
    expect(screen.queryByTestId(CHIP_ID)).toBeNull();
  });

  it("shows a collapsed chip with the queue count when entries exist", () => {
    useQueueMock.mockReturnValue(queueState([entry(), entry({ id: "q-2", content: "second" })]));
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    const chip = screen.getByTestId(CHIP_ID);
    expect(chip.textContent).toContain("2 queued");
    expect(chip.getAttribute("aria-label")).toContain("click to expand");
    expect(screen.queryByTestId(PANEL_ID)).toBeNull();
  });

  it("marks the chip as full when isFull is true", () => {
    useQueueMock.mockReturnValue(queueState([entry()], { isFull: true, max: 1 }));
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    const chip = screen.getByTestId(CHIP_ID);
    expect(chip.getAttribute("data-full")).toBe("true");
    expect(chip.textContent).toContain("full");
  });

  it("clicking the chip swaps it for the expanded panel", () => {
    useQueueMock.mockReturnValue(queueState([entry()]));
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    fireEvent.click(screen.getByTestId(CHIP_ID));
    expect(screen.getByTestId(PANEL_ID)).toBeTruthy();
    // While the panel is open the chip is hidden — its info is duplicated in
    // the panel header, so we collapse via the X close button or Escape.
    expect(screen.queryByTestId(CHIP_ID)).toBeNull();
  });

  it("clicking the X close button in the panel collapses the panel", () => {
    useQueueMock.mockReturnValue(queueState([entry()]));
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    fireEvent.click(screen.getByTestId(CHIP_ID));
    fireEvent.click(screen.getByTestId("queue-close"));
    expect(screen.queryByTestId(PANEL_ID)).toBeNull();
  });

  it("Escape collapses an open panel", () => {
    useQueueMock.mockReturnValue(queueState([entry()]));
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    fireEvent.click(screen.getByTestId(CHIP_ID));
    fireEvent.keyDown(window, { key: "Escape" });
    expect(screen.queryByTestId(PANEL_ID)).toBeNull();
  });

  it("auto-collapses the panel when the queue drains to zero", () => {
    useQueueMock.mockReturnValue(queueState([entry()]));
    const { rerender } = render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    fireEvent.click(screen.getByTestId(CHIP_ID));
    expect(screen.getByTestId(PANEL_ID)).toBeTruthy();
    useQueueMock.mockReturnValue(queueState([]));
    rerender(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    expect(screen.queryByTestId(PANEL_ID)).toBeNull();
    expect(screen.queryByTestId(CHIP_ID)).toBeNull();
  });

  it("auto-collapses the panel when sessionId changes", () => {
    useQueueMock.mockReturnValue(queueState([entry()]));
    const { rerender } = render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    fireEvent.click(screen.getByTestId(CHIP_ID));
    expect(screen.getByTestId(PANEL_ID)).toBeTruthy();
    rerender(<QueueAffordance sessionId="sess-2">{CHILD}</QueueAffordance>);
    expect(screen.queryByTestId(PANEL_ID)).toBeNull();
  });

  it("clear-all button invokes clearAll from the queue hook", () => {
    const state = queueState([entry()]);
    useQueueMock.mockReturnValue(state);
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    fireEvent.click(screen.getByTestId(CHIP_ID));
    fireEvent.click(screen.getByTestId("queue-clear-all"));
    expect(state.clearAll).toHaveBeenCalledTimes(1);
  });

  it("workflow queued entries are read-only", () => {
    useQueueMock.mockReturnValue(
      queueState([
        entry({
          queued_by: "workflow",
          metadata: {
            workflow_message: true,
            workflow_step_name: "Review",
          },
        }),
      ]),
    );
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);

    fireEvent.click(screen.getByTestId(CHIP_ID));

    expect(screen.getByTestId("workflow-message-badge").textContent).toContain("Review");
    expect(screen.queryByTitle("Edit queued message")).toBeNull();
    expect(screen.queryByTitle("Remove queued message")).toBeNull();
  });
});
