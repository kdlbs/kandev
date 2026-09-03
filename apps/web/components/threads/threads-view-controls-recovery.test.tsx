import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ThreadView, ThreadViewDraft } from "@/lib/state/slices/ui/thread-view-types";
import { ThreadsViewControls } from "./threads-view-controls";

const EMPTY_CANDIDATES: never[] = [];

const ALL_VIEW: ThreadView = {
  id: "view-all-threads",
  name: "All threads",
  taskScope: { mode: "all", taskIds: [] },
  filters: [],
  sort: { key: "attention", direction: "asc" },
  maxColumns: null,
};

const state = {
  threadViews: {
    views: [ALL_VIEW],
    activeViewId: ALL_VIEW.id,
    draft: null as ThreadViewDraft | null,
    syncError: null as string | null,
    orderResetGeneration: 0,
  },
  setThreadActiveView: vi.fn(),
  createThreadView: vi.fn(() => "view-new"),
  updateThreadViewDraft: vi.fn(),
  saveThreadViewDraftAs: vi.fn(),
  saveThreadViewDraftOverwrite: vi.fn(),
  discardThreadViewDraft: vi.fn(),
  deleteThreadView: vi.fn(),
  renameThreadView: vi.fn(),
  duplicateThreadView: vi.fn(),
  reapplyThreadViewSort: vi.fn(),
  retryThreadViewSync: vi.fn(),
  clearThreadViewSyncError: vi.fn(),
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: typeof state) => unknown) => selector(state),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ usesDesktopWorkbench: true }),
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  state.threadViews.draft = null;
  state.threadViews.syncError = null;
});

describe("ThreadsViewControls recovery", () => {
  it("shows retry and dismiss actions for a saved-view sync error", () => {
    state.threadViews.syncError = "write failed";
    render(
      <ThreadsViewControls
        candidates={EMPTY_CANDIDATES}
        admittedCount={1}
        matchingCount={1}
        hiddenCount={0}
      />,
    );

    expect(screen.getByTestId("threads-view-sync-error")).toBeTruthy();
    expect(screen.getByTestId("threads-view-sync-error").textContent).toContain("write failed");

    fireEvent.click(screen.getByTestId("threads-view-sync-retry"));
    expect(state.retryThreadViewSync).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByTestId("threads-view-sync-dismiss"));
    expect(state.clearThreadViewSyncError).toHaveBeenCalledTimes(1);
  });

  it("keeps invalid maximum columns edits out of the active query", () => {
    render(
      <ThreadsViewControls
        candidates={EMPTY_CANDIDATES}
        admittedCount={1}
        matchingCount={1}
        hiddenCount={0}
      />,
    );

    fireEvent.click(screen.getByTestId("threads-view-settings"));
    const input = screen.getByTestId("threads-max-columns");
    fireEvent.change(input, { target: { value: "2.5" } });

    expect(state.updateThreadViewDraft).not.toHaveBeenCalled();
    expect(input.getAttribute("aria-invalid")).toBe("true");
    expect(
      screen.getByText("Enter a whole number from 1 to 30, or leave this field empty"),
    ).toBeTruthy();

    fireEvent.change(input, { target: { value: "2" } });
    expect(state.updateThreadViewDraft).toHaveBeenCalledWith({ maxColumns: 2 });
  });
});
