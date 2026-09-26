import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { QueuedMessage } from "@/lib/state/slices/session/types";

const useQueueMock = vi.fn();

vi.mock("@/hooks/domains/session/use-queue", () => ({
  useQueue: (sessionId: string | null) => useQueueMock(sessionId),
}));

const useQueuePinnedMock = vi.fn();

vi.mock("@/hooks/use-queue-pinned", () => ({
  useQueuePinned: (sessionId: string | null) => useQueuePinnedMock(sessionId),
}));

const breakpointMocks: { isMobile: boolean } = { isMobile: false };

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => breakpointMocks,
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

import { QueueAffordance } from "./queued-ghost-list";

const SESSION_ID = "sess-1";
const CHIP_ID = "queue-chip";
const PANEL_ID = "queued-ghost-list";

function entry(): QueuedMessage {
  return {
    id: "q-1",
    session_id: "sess-1",
    task_id: "task-1",
    content: "hello world",
    plan_mode: false,
    queued_at: "2026-05-18T00:00:00Z",
    queued_by: "user",
  };
}

function queueState(entries: QueuedMessage[]) {
  return {
    entries,
    count: entries.length,
    max: 10,
    isFull: false,
    mergeEnabled: true,
    autoRun: true,
    isLoading: false,
    queue: vi.fn(async () => {}),
    clearAll: vi.fn(async () => {}),
    setAutoRun: vi.fn(async () => {}),
    editEntry: vi.fn(async () => {}),
    removeEntry: vi.fn(async () => {}),
    mergeEntry: vi.fn(async () => {}),
    reorderEntries: vi.fn(async () => {}),
    sendEntryNow: vi.fn(async () => {}),
    cancellationPending: false,
    refetch: vi.fn(async () => {}),
  };
}

const CHILD = <div data-testid="child-marker">input</div>;

beforeEach(() => {
  useQueueMock.mockReset();
  useQueuePinnedMock.mockReset();
  useQueuePinnedMock.mockReturnValue({ value: false, setValue: vi.fn(), toggle: vi.fn() });
  breakpointMocks.isMobile = false;
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("QueueAffordance pin", () => {
  it("renders the pin toggle between Clear all and the close button", () => {
    useQueueMock.mockReturnValue(queueState([entry()]));
    const toggle = vi.fn();
    useQueuePinnedMock.mockReturnValue({ value: false, setValue: vi.fn(), toggle });
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    fireEvent.click(screen.getByTestId(CHIP_ID));

    const pin = screen.getByTestId("queue-pin") as HTMLButtonElement;
    expect(pin.getAttribute("aria-pressed")).toBe("false");
    // DOM order: Clear all < pin < close (relative positions in the shared
    // header control row).
    const controlOrder = Array.from(pin.parentElement!.children).map((el) =>
      el.getAttribute("data-testid"),
    );
    expect(controlOrder.indexOf("queue-pin")).toBeGreaterThan(
      controlOrder.indexOf("queue-clear-all"),
    );
    expect(controlOrder.indexOf("queue-pin")).toBeLessThan(controlOrder.indexOf("queue-close"));

    fireEvent.click(pin);
    expect(toggle).toHaveBeenCalledTimes(1);
  });

  it("starts expanded and hides the chip when the session is pinned", () => {
    useQueueMock.mockReturnValue(queueState([entry()]));
    useQueuePinnedMock.mockReturnValue({ value: true, setValue: vi.fn(), toggle: vi.fn() });
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    expect(screen.getByTestId(PANEL_ID)).toBeTruthy();
    expect(screen.queryByTestId(CHIP_ID)).toBeNull();
  });

  it("reopens expanded after a pinned remount cycle", () => {
    useQueueMock.mockReturnValue(queueState([entry()]));
    useQueuePinnedMock.mockReturnValue({ value: true, setValue: vi.fn(), toggle: vi.fn() });
    const { unmount } = render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    expect(screen.getByTestId(PANEL_ID)).toBeTruthy();
    // Simulate navigating away and back: the affordance unmounts, then a
    // fresh instance mounts with the same pinned session and must come back
    // expanded without a chip click.
    unmount();
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    expect(screen.getByTestId(PANEL_ID)).toBeTruthy();
    expect(screen.queryByTestId(CHIP_ID)).toBeNull();
  });

  it("does not reopen on remount when the session is unpinned", () => {
    useQueueMock.mockReturnValue(queueState([entry()]));
    useQueuePinnedMock.mockReturnValue({ value: false, setValue: vi.fn(), toggle: vi.fn() });
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    expect(screen.queryByTestId(PANEL_ID)).toBeNull();
    expect(screen.getByTestId(CHIP_ID)).toBeTruthy();
  });

  it("hides both chip and panel when a pinned session has no entries", () => {
    useQueueMock.mockReturnValue(queueState([]));
    useQueuePinnedMock.mockReturnValue({ value: true, setValue: vi.fn(), toggle: vi.fn() });
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    expect(screen.queryByTestId(PANEL_ID)).toBeNull();
    expect(screen.queryByTestId(CHIP_ID)).toBeNull();
  });

  it("opens the panel when queued entries arrive after mount while pinned", () => {
    useQueueMock.mockReturnValue(queueState([]));
    useQueuePinnedMock.mockReturnValue({ value: true, setValue: vi.fn(), toggle: vi.fn() });
    const { rerender } = render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    expect(screen.queryByTestId(PANEL_ID)).toBeNull();

    // Queue entries land asynchronously after the affordance mounted (e.g.
    // returning to the task route): the pinned panel must appear without a
    // chip click.
    useQueueMock.mockReturnValue(queueState([entry()]));
    rerender(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    expect(screen.getByTestId(PANEL_ID)).toBeTruthy();
    expect(screen.queryByTestId(CHIP_ID)).toBeNull();
  });
});

describe("QueueAffordance pin desktop-only", () => {
  it("hides the pin toggle on mobile viewports", () => {
    useQueueMock.mockReturnValue(queueState([entry()]));
    useQueuePinnedMock.mockReturnValue({ value: false, setValue: vi.fn(), toggle: vi.fn() });
    breakpointMocks.isMobile = true;
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    fireEvent.click(screen.getByTestId(CHIP_ID));

    expect(screen.getByTestId(PANEL_ID)).toBeTruthy();
    expect(screen.queryByTestId("queue-pin")).toBeNull();
    // The rest of the header controls remain available on mobile.
    expect(screen.getByTestId("queue-clear-all")).toBeTruthy();
    expect(screen.getByTestId("queue-close")).toBeTruthy();
  });

  it("renders the pin toggle on desktop viewports", () => {
    useQueueMock.mockReturnValue(queueState([entry()]));
    useQueuePinnedMock.mockReturnValue({ value: false, setValue: vi.fn(), toggle: vi.fn() });
    breakpointMocks.isMobile = false;
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    fireEvent.click(screen.getByTestId(CHIP_ID));
    expect(screen.getByTestId("queue-pin")).toBeTruthy();
  });

  it("does not auto-open from a stored desktop pin on mobile", () => {
    useQueueMock.mockReturnValue(queueState([entry()]));
    useQueuePinnedMock.mockReturnValue({ value: true, setValue: vi.fn(), toggle: vi.fn() });
    breakpointMocks.isMobile = true;
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);

    // The stored pin stays off on phone viewports: the panel starts
    // collapsed with the chip, and there is no pin control to unpin.
    expect(screen.queryByTestId(PANEL_ID)).toBeNull();
    expect(screen.getByTestId(CHIP_ID)).toBeTruthy();
    expect(screen.queryByTestId("queue-pin")).toBeNull();
  });
});

describe("QueueAffordance pin sync", () => {
  it("collapses the panel when navigating to an unpinned session", () => {
    useQueueMock.mockReturnValue(queueState([entry()]));
    useQueuePinnedMock.mockImplementation((sessionId: string | null) => ({
      value: sessionId === "sess-1",
      setValue: vi.fn(),
      toggle: vi.fn(),
    }));
    const { rerender } = render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    expect(screen.getByTestId(PANEL_ID)).toBeTruthy();

    // Switch to an unpinned session: panel collapses, chip shows.
    rerender(<QueueAffordance sessionId="sess-2">{CHILD}</QueueAffordance>);
    expect(screen.queryByTestId(PANEL_ID)).toBeNull();
    expect(screen.getByTestId(CHIP_ID)).toBeTruthy();
  });

  it("opens the panel when a storage-synced pin turns on for the current session", () => {
    useQueueMock.mockReturnValue(queueState([entry()]));
    useQueuePinnedMock.mockReturnValue({ value: false, setValue: vi.fn(), toggle: vi.fn() });
    const { rerender } = render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    expect(screen.queryByTestId(PANEL_ID)).toBeNull();
    expect(screen.getByTestId(CHIP_ID)).toBeTruthy();

    // Another tab pins the same session; the storage event flips `pinned`.
    useQueuePinnedMock.mockReturnValue({ value: true, setValue: vi.fn(), toggle: vi.fn() });
    rerender(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    expect(screen.getByTestId(PANEL_ID)).toBeTruthy();
    expect(screen.queryByTestId(CHIP_ID)).toBeNull();
  });

  it("does not close an open panel when the pin turns off", () => {
    useQueueMock.mockReturnValue(queueState([entry()]));
    useQueuePinnedMock.mockReturnValue({ value: true, setValue: vi.fn(), toggle: vi.fn() });
    const { rerender } = render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    expect(screen.getByTestId(PANEL_ID)).toBeTruthy();

    // Unpinning (here or in another tab) never collapses the current view.
    useQueuePinnedMock.mockReturnValue({ value: false, setValue: vi.fn(), toggle: vi.fn() });
    rerender(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    expect(screen.getByTestId(PANEL_ID)).toBeTruthy();
    expect(screen.queryByTestId(CHIP_ID)).toBeNull();
  });

  it("stays collapsed when a pinned empty queue is unpinned before entries arrive", () => {
    useQueueMock.mockReturnValue(queueState([]));
    useQueuePinnedMock.mockReturnValue({ value: true, setValue: vi.fn(), toggle: vi.fn() });
    const { rerender } = render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    expect(screen.queryByTestId(PANEL_ID)).toBeNull();

    // Another tab unpins the session before its queue has loaded.
    useQueuePinnedMock.mockReturnValue({ value: false, setValue: vi.fn(), toggle: vi.fn() });
    rerender(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    expect(screen.queryByTestId(PANEL_ID)).toBeNull();

    // Entries arrive: the unpinned queue must stay collapsed with the chip,
    // not leak an open panel from the earlier pinned-empty state.
    useQueueMock.mockReturnValue(queueState([entry()]));
    rerender(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    expect(screen.queryByTestId(PANEL_ID)).toBeNull();
    expect(screen.getByTestId(CHIP_ID)).toBeTruthy();
  });

  it("opens when entries arrive after a storage-synced pin on an empty queue", () => {
    useQueueMock.mockReturnValue(queueState([]));
    useQueuePinnedMock.mockReturnValue({ value: false, setValue: vi.fn(), toggle: vi.fn() });
    const { rerender } = render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    expect(screen.queryByTestId(PANEL_ID)).toBeNull();

    // Another tab pins the session while the queue is still empty.
    useQueuePinnedMock.mockReturnValue({ value: true, setValue: vi.fn(), toggle: vi.fn() });
    rerender(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    expect(screen.queryByTestId(PANEL_ID)).toBeNull();

    // Entries arrive: the pinned panel must open without a chip click.
    useQueueMock.mockReturnValue(queueState([entry()]));
    rerender(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    expect(screen.getByTestId(PANEL_ID)).toBeTruthy();
    expect(screen.queryByTestId(CHIP_ID)).toBeNull();
  });
});

describe("QueueAffordance pin transitions", () => {
  it("opens when the session switches to a pinned queue whose entries load later", () => {
    useQueuePinnedMock.mockImplementation((sessionId: string | null) => ({
      value: sessionId === "sess-2",
      setValue: vi.fn(),
      toggle: vi.fn(),
    }));
    useQueueMock.mockReturnValue(queueState([]));
    const { rerender } = render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);

    // Switch to the pinned session before its queue has loaded.
    rerender(<QueueAffordance sessionId="sess-2">{CHILD}</QueueAffordance>);
    expect(screen.queryByTestId(PANEL_ID)).toBeNull();

    useQueueMock.mockReturnValue(queueState([entry()]));
    rerender(<QueueAffordance sessionId="sess-2">{CHILD}</QueueAffordance>);
    expect(screen.getByTestId(PANEL_ID)).toBeTruthy();
    expect(screen.queryByTestId(CHIP_ID)).toBeNull();
  });

  it("reopens a pinned queue after a full drain when a new entry arrives", () => {
    useQueueMock.mockReturnValue(queueState([entry()]));
    useQueuePinnedMock.mockReturnValue({ value: true, setValue: vi.fn(), toggle: vi.fn() });
    const { rerender } = render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    expect(screen.getByTestId(PANEL_ID)).toBeTruthy();

    // Full drain: panel closes, nothing rendered.
    useQueueMock.mockReturnValue(queueState([]));
    rerender(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    expect(screen.queryByTestId(PANEL_ID)).toBeNull();

    // A later queue for the same session reopens the pinned panel.
    useQueueMock.mockReturnValue(queueState([entry()]));
    rerender(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    expect(screen.getByTestId(PANEL_ID)).toBeTruthy();
    expect(screen.queryByTestId(CHIP_ID)).toBeNull();
  });

  it("does not reopen an X-collapsed pinned queue on ordinary count changes", () => {
    useQueueMock.mockReturnValue(
      queueState([entry(), { ...entry(), id: "q-2", content: "second" }]),
    );
    useQueuePinnedMock.mockReturnValue({ value: true, setValue: vi.fn(), toggle: vi.fn() });
    const { rerender } = render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    expect(screen.getByTestId(PANEL_ID)).toBeTruthy();

    fireEvent.click(screen.getByTestId("queue-close"));
    expect(screen.queryByTestId(PANEL_ID)).toBeNull();
    expect(screen.getByTestId(CHIP_ID)).toBeTruthy();

    // One entry removed while the queue stays nonempty: stays collapsed.
    useQueueMock.mockReturnValue(queueState([entry()]));
    rerender(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    expect(screen.queryByTestId(PANEL_ID)).toBeNull();
    expect(screen.getByTestId(CHIP_ID)).toBeTruthy();
  });

  it("keeps the collapse button touch-sized alongside the pin", () => {
    useQueueMock.mockReturnValue(queueState([entry()]));
    useQueuePinnedMock.mockReturnValue({ value: true, setValue: vi.fn(), toggle: vi.fn() });
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    const pin = screen.getByTestId("queue-pin");
    expect(pin.className).toContain("[@media(pointer:coarse)]:h-11");
    expect(pin.className).toContain("[@media(pointer:coarse)]:w-11");
  });
});
