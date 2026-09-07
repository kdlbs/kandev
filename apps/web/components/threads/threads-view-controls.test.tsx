import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ThreadView, ThreadViewDraft } from "@/lib/state/slices/ui/thread-view-types";
import { ThreadsViewControls } from "./threads-view-controls";

const responsive = vi.hoisted(() => ({ usesDesktopWorkbench: true }));
const EMPTY_CANDIDATES: never[] = [];
const VIEW_PICKER_TEST_ID = "threads-view-picker";

const ALL_VIEW: ThreadView = {
  id: "view-all-threads",
  name: "All threads",
  taskScope: { mode: "all", taskIds: [] },
  filters: [],
  sort: { key: "attention", direction: "asc" },
  maxColumns: null,
};
const REVIEW_VIEW: ThreadView = {
  ...ALL_VIEW,
  id: "view-review",
  name: "Reviews",
  filters: [{ id: "filter-1", dimension: "taskState", op: "is", value: "REVIEW" }],
};

const state = {
  threadViews: {
    views: [ALL_VIEW, REVIEW_VIEW],
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
  useResponsiveBreakpoint: () => responsive,
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  responsive.usesDesktopWorkbench = true;
  state.threadViews.draft = null;
  state.threadViews.syncError = null;
});

describe("ThreadsViewControls", () => {
  it("switches saved views from the compact selector and shows bounded counts", async () => {
    render(
      <ThreadsViewControls
        candidates={EMPTY_CANDIDATES}
        admittedCount={2}
        matchingCount={5}
        hiddenCount={3}
      />,
    );

    expect(screen.getByTestId(VIEW_PICKER_TEST_ID)).toBeTruthy();
    expect(screen.getByText("2 of 5 columns")).toBeTruthy();
    expect(screen.getByText("3 hidden")).toBeTruthy();

    fireEvent.pointerDown(screen.getByTestId(VIEW_PICKER_TEST_ID));
    fireEvent.click(await screen.findByTestId("threads-view-option-view-review"));

    expect(state.setThreadActiveView).toHaveBeenCalledWith(REVIEW_VIEW.id);
  });

  it("opens the editor and adds a Threads filter to the independent draft", () => {
    render(
      <ThreadsViewControls
        candidates={EMPTY_CANDIDATES}
        admittedCount={0}
        matchingCount={0}
        hiddenCount={0}
      />,
    );

    fireEvent.click(screen.getByTestId("threads-view-settings"));
    expect(screen.getByTestId("threads-view-editor")).toBeTruthy();
    fireEvent.click(screen.getByTestId("threads-filter-add"));

    expect(state.updateThreadViewDraft).toHaveBeenCalledWith(
      expect.objectContaining({ filters: expect.arrayContaining([expect.any(Object)]) }),
    );
  });

  it("keeps the saved-view surface independent from sidebar view state", async () => {
    render(
      <ThreadsViewControls
        candidates={EMPTY_CANDIDATES}
        admittedCount={1}
        matchingCount={1}
        hiddenCount={0}
      />,
    );

    fireEvent.pointerDown(screen.getByTestId(VIEW_PICKER_TEST_ID));
    fireEvent.click(await screen.findByTestId("threads-view-option-view-review"));

    expect(state.setThreadActiveView).toHaveBeenCalledTimes(1);
    expect(state).not.toHaveProperty("setSidebarActiveView");
  });

  it("uses one touch drawer for saved views on tablet and phone layouts", async () => {
    responsive.usesDesktopWorkbench = false;
    render(
      <ThreadsViewControls
        candidates={EMPTY_CANDIDATES}
        admittedCount={1}
        matchingCount={1}
        hiddenCount={0}
      />,
    );

    const trigger = screen.getByTestId("threads-mobile-view-trigger");
    expect(trigger).toBeTruthy();
    expect(screen.queryByTestId(VIEW_PICKER_TEST_ID)).toBeNull();

    fireEvent.click(trigger);
    expect(await screen.findByTestId("threads-mobile-view-drawer")).toBeTruthy();
    fireEvent.click(await screen.findByTestId("threads-mobile-view-option-view-review"));

    expect(state.setThreadActiveView).toHaveBeenCalledWith(REVIEW_VIEW.id);
    expect(document.activeElement).toBe(trigger);
  });

  it("keeps the editor and task picker inside the same mobile drawer", async () => {
    responsive.usesDesktopWorkbench = false;
    state.threadViews.draft = {
      baseViewId: ALL_VIEW.id,
      taskScope: { mode: "selected", taskIds: [] },
      filters: [],
      sort: ALL_VIEW.sort,
      maxColumns: null,
    };
    render(
      <ThreadsViewControls
        candidates={EMPTY_CANDIDATES}
        admittedCount={0}
        matchingCount={0}
        hiddenCount={0}
      />,
    );

    fireEvent.click(screen.getByTestId("threads-mobile-view-trigger"));
    fireEvent.click(await screen.findByTestId("threads-mobile-view-settings"));
    expect(await screen.findByTestId("threads-view-editor")).toBeTruthy();
    fireEvent.click(screen.getByTestId("threads-open-task-picker"));
    expect(screen.getByTestId("threads-task-picker")).toBeTruthy();
    fireEvent.click(screen.getByTestId("threads-task-picker-back"));
    expect(screen.getByTestId("threads-view-editor")).toBeTruthy();
    expect(screen.getByTestId("threads-mobile-view-drawer")).toBeTruthy();
  });
});
