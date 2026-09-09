import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import type { QueuedMessage } from "@/lib/state/slices/session/types";
import type { EntityReference } from "@/lib/types/entity-reference";
import { entityReferenceMarkdown } from "@/lib/entity-references/message-references";
import { QueueEntryNotFoundError } from "@/lib/api/domains/queue-api";
import { toast } from "sonner";
import { simulateReorderDrag } from "./queued-ghost-list.test-helpers";

const useQueueMock = vi.fn();

vi.mock("@/hooks/domains/session/use-queue", () => ({
  useQueue: (sessionId: string | null) => useQueueMock(sessionId),
}));

// The queue pin is desktop-only; these tests exercise the desktop path.
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
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
const QUEUED_BY_USER = "user";
const MERGE_BUTTON_ID = "queue-entry-merge";
const EDIT_BUTTON_ID = "queue-entry-edit";
const REMOVE_BUTTON_ID = "queue-entry-remove";
const SEND_NOW_BUTTON_ID = "queue-entry-send-now";
const AUTO_RUN_BUTTON_ID = "queue-auto-run";

function entry(overrides: Partial<QueuedMessage> = {}): QueuedMessage {
  return {
    id: "q-1",
    session_id: "sess-1",
    task_id: "task-1",
    content: "hello world",
    plan_mode: false,
    queued_at: "2026-05-18T00:00:00Z",
    queued_by: QUEUED_BY_USER,
    ...overrides,
  };
}

function taskReference(): EntityReference {
  return {
    version: 1,
    ref: "mention:v1:kandev:task:workspace-1:task-1",
    provider: "kandev",
    kind: "task",
    id: "task-1",
    key: "KAN-1",
    title: "Fix authentication",
    url: "/t/task-1",
    scope: "workspace-1",
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

function renderQueue(node: ReactNode) {
  return render(<StateProvider>{node}</StateProvider>);
}

function pressQueueEscape(): ReturnType<typeof vi.fn> {
  const outerEscapeHandler = vi.fn();
  document.addEventListener("keydown", outerEscapeHandler);
  try {
    fireEvent.keyDown(screen.getByTestId("queue-close"), { key: "Escape" });
  } finally {
    document.removeEventListener("keydown", outerEscapeHandler);
  }
  return outerEscapeHandler;
}

beforeEach(() => {
  useQueueMock.mockReset();
  vi.mocked(toast.error).mockClear();
  vi.mocked(toast.success).mockClear();
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
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

  it("shows Auto-run and removes legacy header dispatch actions", () => {
    const state = queueState([entry()]);
    useQueueMock.mockReturnValue(state);
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    fireEvent.click(screen.getByTestId(CHIP_ID));

    const autoRun = screen.getByTestId(AUTO_RUN_BUTTON_ID);
    expect(autoRun.getAttribute("data-state")).toBe("checked");
    const autoRunHelpId = autoRun.getAttribute("aria-describedby");
    expect(autoRunHelpId).toBeTruthy();
    expect(document.getElementById(autoRunHelpId!)).toBeTruthy();
    expect(screen.getByText("Runs queued messages one at a time.")).toBeTruthy();
    expect(autoRun.className).toContain("[@media(pointer:coarse)]:after:-inset-y-3.5");
    expect(autoRun.parentElement?.className).toContain("[@media(pointer:coarse)]:min-h-11");
    expect(screen.queryByTestId("queue-drain-next")).toBeNull();
    expect(screen.queryByTestId("queue-send-now")).toBeNull();
  });
});

describe("QueueAffordance Send Now", () => {
  it("offers Send Now on the head and every later row", () => {
    const state = queueState([
      entry({ id: "q-1", content: "first" }),
      entry({ id: "q-2", content: "second" }),
    ]);
    useQueueMock.mockReturnValue(state);
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    fireEvent.click(screen.getByTestId(CHIP_ID));
    const sendNowButtons = screen.getAllByTestId(SEND_NOW_BUTTON_ID);
    expect(sendNowButtons).toHaveLength(2);
    fireEvent.click(sendNowButtons[0]);
    expect(state.sendEntryNow).toHaveBeenCalledWith("q-1");
    fireEvent.click(sendNowButtons[1]);
    expect(state.sendEntryNow).toHaveBeenCalledWith("q-2");
  });

  it.each([
    ["queue mutation", { isLoading: true }],
    ["cancellation", { cancellationPending: true }],
  ])("disables row Send Now and Auto-run during %s", (_name, extra) => {
    useQueueMock.mockReturnValue(queueState([entry()], extra));
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    fireEvent.click(screen.getByTestId(CHIP_ID));
    expect((screen.getByTestId(AUTO_RUN_BUTTON_ID) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByTestId(SEND_NOW_BUTTON_ID) as HTMLButtonElement).disabled).toBe(true);
  });

  it("changes the backend-owned Auto-run policy", () => {
    const state = queueState([entry()], { autoRun: false });
    useQueueMock.mockReturnValue(state);
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    fireEvent.click(screen.getByTestId(CHIP_ID));
    const autoRun = screen.getByTestId(AUTO_RUN_BUTTON_ID);
    expect(autoRun.getAttribute("data-state")).toBe("unchecked");
    expect(
      screen.getByText("Finishes the current response, then queued messages wait."),
    ).toBeTruthy();
    fireEvent.click(autoRun);
    expect(state.setAutoRun).toHaveBeenCalledWith(true);
  });

  it("reports an Auto-run update failure", async () => {
    const state = queueState([entry()]);
    state.setAutoRun = vi.fn(async () => {
      throw new Error("policy failed");
    });
    useQueueMock.mockReturnValue(state);
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    fireEvent.click(screen.getByTestId(CHIP_ID));

    fireEvent.click(screen.getByTestId(AUTO_RUN_BUTTON_ID));

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith("Failed to update queue Auto-run."),
    );
  });
});

describe("QueueAffordance positions", () => {
  it("compacts displayed positions when persisted queue positions contain gaps", () => {
    const initialEntries = [
      entry({ id: "q-1", position: 1 }),
      entry({ id: "q-2", position: 2 }),
      entry({ id: "q-3", position: 3 }),
    ];
    useQueueMock.mockReturnValue(queueState(initialEntries));
    const { rerender } = render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    fireEvent.click(screen.getByTestId(CHIP_ID));

    useQueueMock.mockReturnValue(queueState([initialEntries[0], initialEntries[2]]));
    rerender(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);

    expect(screen.getByLabelText("Position #1")).toBeTruthy();
    expect(screen.getByLabelText("Position #2")).toBeTruthy();
    expect(screen.queryByLabelText("Position #3")).toBeNull();
  });
});

describe("QueueAffordance Escape handling", () => {
  it("collapses an open panel without reaching an outer dialog", () => {
    useQueueMock.mockReturnValue(queueState([entry()]));
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    fireEvent.click(screen.getByTestId(CHIP_ID));
    const outerEscapeHandler = pressQueueEscape();
    expect(screen.queryByTestId(PANEL_ID)).toBeNull();
    expect(screen.getByTestId(CHIP_ID)).toBeTruthy();
    expect(outerEscapeHandler).not.toHaveBeenCalled();
  });
});

describe("QueueAffordance — renderStatusBar prop", () => {
  it("calls renderStatusBar with null when there are no queued entries", () => {
    useQueueMock.mockReturnValue(queueState([]));
    const renderStatusBar = vi.fn(() => <div data-testid="status-bar-slot" />);
    render(
      <QueueAffordance sessionId={SESSION_ID} renderStatusBar={renderStatusBar}>
        {CHILD}
      </QueueAffordance>,
    );
    expect(renderStatusBar).toHaveBeenCalledWith(null);
    expect(screen.getByTestId("status-bar-slot")).toBeTruthy();
  });

  it("calls renderStatusBar with a chip node when entries exist and panel is closed", () => {
    useQueueMock.mockReturnValue(queueState([entry()]));
    const renderStatusBar = vi.fn((chip) => <div data-testid="status-bar-slot">{chip}</div>);
    render(
      <QueueAffordance sessionId={SESSION_ID} renderStatusBar={renderStatusBar}>
        {CHILD}
      </QueueAffordance>,
    );
    const callArg = renderStatusBar.mock.calls[0][0];
    expect(callArg).not.toBeNull();
    // The chip lives inside the caller-supplied slot, not as a separate
    // inline element above the input.
    const slot = screen.getByTestId("status-bar-slot");
    expect(slot.contains(screen.getByTestId(CHIP_ID))).toBe(true);
  });

  it("calls renderStatusBar with null when entries exist and the panel is open", () => {
    useQueueMock.mockReturnValue(queueState([entry()]));
    const renderStatusBar = vi.fn((chip) => <div data-testid="status-bar-slot">{chip}</div>);
    render(
      <QueueAffordance sessionId={SESSION_ID} renderStatusBar={renderStatusBar}>
        {CHILD}
      </QueueAffordance>,
    );
    // Open the panel — the chip should drop out of the status bar.
    fireEvent.click(screen.getByTestId(CHIP_ID));
    expect(screen.getByTestId(PANEL_ID)).toBeTruthy();
    const lastCall = renderStatusBar.mock.calls.at(-1);
    expect(lastCall?.[0]).toBeNull();
  });
});

describe("QueueAffordance — provenance actions", () => {
  it("offers Remove for every visible origin and Edit only for user rows", () => {
    const state = queueState([
      entry({ id: "q-user", queued_by: "user" }),
      entry({ id: "q-agent", queued_by: "agent" }),
      entry({
        id: "q-workflow",
        queued_by: "workflow",
        metadata: { workflow_message: true, workflow_step_name: "Review" },
      }),
      entry({ id: "q-server", queued_by: "server" }),
    ]);
    useQueueMock.mockReturnValue(state);
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);

    fireEvent.click(screen.getByTestId(CHIP_ID));

    expect(screen.getByTestId("workflow-message-badge").textContent).toContain("Review");
    expect(screen.getAllByTestId(REMOVE_BUTTON_ID)).toHaveLength(4);
    expect(screen.getAllByTestId(EDIT_BUTTON_ID)).toHaveLength(1);

    fireEvent.click(screen.getAllByTestId(REMOVE_BUTTON_ID)[1]);
    expect(state.removeEntry).toHaveBeenCalledWith("q-agent");
  });

  it("keeps one queue scroll owner and touch-sizes clear and close controls", () => {
    useQueueMock.mockReturnValue(queueState([entry()]));
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);

    fireEvent.click(screen.getByTestId(CHIP_ID));

    const panel = screen.getByTestId(PANEL_ID);
    expect(panel.querySelectorAll(".overflow-y-auto")).toHaveLength(1);
    for (const testId of ["queue-clear-all", "queue-close"]) {
      expect(screen.getByTestId(testId).className).toContain("[@media(pointer:coarse)]:h-11");
    }
  });
});

describe("QueueAffordance entity-reference edits", () => {
  it("forwards an explicit empty replacement through the queue edit callback", async () => {
    const reference = taskReference();
    const state = queueState([
      entry({
        content: entityReferenceMarkdown(reference),
        metadata: { entity_references: [reference] },
      }),
    ]);
    useQueueMock.mockReturnValue(state);
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);

    fireEvent.click(screen.getByTestId(CHIP_ID));
    fireEvent.click(screen.getByTitle("Edit queued message"));
    fireEvent.change(screen.getByTestId("queue-edit-textarea"), {
      target: { value: "reference removed" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(state.editEntry).toHaveBeenCalledWith("q-1", "reference removed", undefined, []),
    );
  });
});

describe("QueueAffordance merge wiring — eligibility", () => {
  it("shows a merge control on the second row and calls mergeEntry with its id", () => {
    const state = queueState([
      entry({ id: "q-1", content: "first", queued_by: QUEUED_BY_USER }),
      entry({ id: "q-2", content: "second", queued_by: QUEUED_BY_USER }),
    ]);
    useQueueMock.mockReturnValue(state);
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);

    fireEvent.click(screen.getByTestId(CHIP_ID));

    const mergeButtons = screen.getAllByTestId(MERGE_BUTTON_ID);
    // Only the second row merges (the head row has nothing above it).
    expect(mergeButtons).toHaveLength(1);
    fireEvent.click(mergeButtons[0]);
    expect(state.mergeEntry).toHaveBeenCalledWith("q-2");
  });

  it("hides the merge control when the rows have mismatched sender kinds", () => {
    const state = queueState([
      entry({ id: "q-1", content: "agent", queued_by: "agent" }),
      entry({ id: "q-2", content: "user", queued_by: QUEUED_BY_USER }),
    ]);
    useQueueMock.mockReturnValue(state);
    renderQueue(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);

    fireEvent.click(screen.getByTestId(CHIP_ID));
    expect(screen.queryByTestId(MERGE_BUTTON_ID)).toBeNull();
  });

  it("hides the merge control for agent rows from different sender tasks", () => {
    const state = queueState([
      entry({
        id: "q-1",
        content: "agent a",
        queued_by: "agent",
        metadata: { sender_task_id: "task-7" },
      }),
      entry({
        id: "q-2",
        content: "agent b",
        queued_by: "agent",
        metadata: { sender_task_id: "task-8" },
      }),
    ]);
    useQueueMock.mockReturnValue(state);
    renderQueue(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);

    fireEvent.click(screen.getByTestId(CHIP_ID));
    expect(screen.queryByTestId(MERGE_BUTTON_ID)).toBeNull();
  });
});

describe("QueueAffordance merge wiring — dispatch", () => {
  it("toasts an error when the merge fails", async () => {
    const { toast } = await import("sonner");
    const state = queueState([
      entry({ id: "q-1", content: "first", queued_by: QUEUED_BY_USER }),
      entry({ id: "q-2", content: "second", queued_by: QUEUED_BY_USER }),
    ]);
    state.mergeEntry = vi.fn(async () => {
      throw new Error("boom");
    });
    useQueueMock.mockReturnValue(state);
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);

    fireEvent.click(screen.getByTestId(CHIP_ID));
    fireEvent.click(screen.getAllByTestId(MERGE_BUTTON_ID)[0]);
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith("Failed to merge queued messages."),
    );
  });

  it("does not toast on a drain race (QueueEntryNotFoundError)", async () => {
    const { toast } = await import("sonner");
    const state = queueState([
      entry({ id: "q-1", content: "first", queued_by: QUEUED_BY_USER }),
      entry({ id: "q-2", content: "second", queued_by: QUEUED_BY_USER }),
    ]);
    state.mergeEntry = vi.fn(async () => {
      throw new QueueEntryNotFoundError();
    });
    useQueueMock.mockReturnValue(state);
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);

    fireEvent.click(screen.getByTestId(CHIP_ID));
    fireEvent.click(screen.getAllByTestId(MERGE_BUTTON_ID)[0]);
    await waitFor(() => expect(state.mergeEntry).toHaveBeenCalledTimes(1));
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("serializes rapid double-clicks on the same merge row", async () => {
    const state = queueState([
      entry({ id: "q-1", content: "first", queued_by: QUEUED_BY_USER }),
      entry({ id: "q-2", content: "second", queued_by: QUEUED_BY_USER }),
    ]);
    state.mergeEntry = vi.fn(async () => {});
    useQueueMock.mockReturnValue(state);
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);

    fireEvent.click(screen.getByTestId(CHIP_ID));
    const button = screen.getAllByTestId(MERGE_BUTTON_ID)[0];
    fireEvent.click(button);
    fireEvent.click(button);
    await waitFor(() => expect(state.mergeEntry).toHaveBeenCalledTimes(1));
  });

  it("hides the merge control entirely when merging is disabled", () => {
    const state = queueState(
      [
        entry({ id: "q-1", content: "first", queued_by: QUEUED_BY_USER }),
        entry({ id: "q-2", content: "second", queued_by: QUEUED_BY_USER }),
      ],
      { mergeEnabled: false },
    );
    useQueueMock.mockReturnValue(state);
    render(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);

    fireEvent.click(screen.getByTestId(CHIP_ID));
    expect(screen.queryAllByTestId(MERGE_BUTTON_ID)).toHaveLength(0);
  });
});

describe("QueueAffordance reorder", () => {
  const GRAB_HANDLE_ID = "queue-grab-handle";

  it("renders a localized grab handle on every row when multiple entries are queued", () => {
    const state = queueState([
      entry({ id: "q-1", content: "first" }),
      entry({ id: "q-2", content: "second" }),
    ]);
    useQueueMock.mockReturnValue(state);
    renderQueue(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    fireEvent.click(screen.getByTestId(CHIP_ID));

    const handles = screen.getAllByTestId(GRAB_HANDLE_ID);
    expect(handles).toHaveLength(2);
    for (const handle of handles) {
      expect(handle.getAttribute("aria-label")).toBe("Reorder queued message");
      expect(handle.getAttribute("aria-roledescription")).toBe("sortable");
      expect((handle as HTMLButtonElement).disabled).toBe(false);
    }
    const rowShell = screen.getAllByTestId("queue-entry")[0].parentElement!;
    expect(rowShell.className).toContain("pl-5");
    expect(rowShell.className).toContain("[@media(pointer:coarse)]:pl-11");
  });

  it("does not render a handle when only one entry is queued", () => {
    useQueueMock.mockReturnValue(queueState([entry()]));
    renderQueue(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    fireEvent.click(screen.getByTestId(CHIP_ID));

    expect(screen.queryByTestId(GRAB_HANDLE_ID)).toBeNull();
  });

  it("keeps compact row padding when no drag handle is shown", () => {
    useQueueMock.mockReturnValue(queueState([entry()]));
    renderQueue(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    fireEvent.click(screen.getByTestId(CHIP_ID));

    const rowShell = screen.getAllByTestId("queue-entry")[0].parentElement!;
    expect(rowShell.className).toContain("pl-2");
    expect(rowShell.className).not.toContain("pl-5");
    expect(rowShell.className).not.toContain("[@media(pointer:coarse)]:pl-11");
  });

  it("disables every handle while a queue mutation or cancellation is pending", () => {
    const state = queueState([entry({ id: "q-1" }), entry({ id: "q-2", content: "second" })], {
      isLoading: true,
      cancellationPending: true,
    });
    useQueueMock.mockReturnValue(state);
    renderQueue(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    fireEvent.click(screen.getByTestId(CHIP_ID));

    const handles = screen.getAllByTestId(GRAB_HANDLE_ID) as HTMLButtonElement[];
    expect(handles).toHaveLength(2);
    for (const handle of handles) expect(handle.disabled).toBe(true);
  });

  it("removes the handle while the row is being edited", () => {
    const state = queueState([
      entry({ id: "q-1", queued_by: QUEUED_BY_USER }),
      entry({ id: "q-2", content: "second" }),
    ]);
    useQueueMock.mockReturnValue(state);
    renderQueue(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    fireEvent.click(screen.getByTestId(CHIP_ID));

    fireEvent.click(screen.getAllByTestId(EDIT_BUTTON_ID)[0]);
    expect(screen.getByTestId("queue-edit-textarea")).toBeTruthy();
    expect(screen.getAllByTestId(GRAB_HANDLE_ID)).toHaveLength(1);
  });

  it("drags the last row onto the first and calls reorderEntries with the new order", async () => {
    const state = queueState([
      entry({ id: "q-1", content: "first" }),
      entry({ id: "q-2", content: "second" }),
      entry({ id: "q-3", content: "third" }),
    ]);
    state.reorderEntries = vi.fn(async () => {});
    useQueueMock.mockReturnValue(state);
    renderQueue(<QueueAffordance sessionId={SESSION_ID}>{CHILD}</QueueAffordance>);
    fireEvent.click(screen.getByTestId(CHIP_ID));

    const handles = screen.getAllByTestId(GRAB_HANDLE_ID);
    simulateReorderDrag(handles[2], handles.length, 10);

    await waitFor(() => expect(state.reorderEntries).toHaveBeenCalledWith(["q-3", "q-1", "q-2"]));
  });
});
