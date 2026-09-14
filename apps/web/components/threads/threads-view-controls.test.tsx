import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ThreadView, ThreadViewDraft } from "@/lib/state/slices/ui/thread-view-types";
import { ThreadsViewControls } from "./threads-view-controls";

const responsive = vi.hoisted(() => ({
  usesDesktopWorkbench: true,
  isFinePointer: true,
  isMobile: false,
}));
const EMPTY_CANDIDATES: never[] = [];
const VIEW_PICKER_TEST_ID = "threads-view-picker";
const VIEW_SETTINGS_TEST_ID = "threads-view-settings";
const MOBILE_VIEW_TRIGGER_TEST_ID = "threads-mobile-view-trigger";
const DELETE_ACTION_TEST_ID = "threads-view-delete";
const MOBILE_DRAWER_TEST_ID = "threads-mobile-view-drawer";

const ALL_VIEW: ThreadView = {
  id: "view-all-threads",
  name: "All threads",
  taskScope: { mode: "all", taskIds: [] },
  filters: [],
  sort: { key: "attention", direction: "asc" },
  maxColumns: null,
  layout: "columns",
  autoHideComposer: false,
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
  responsive.isFinePointer = true;
  responsive.isMobile = false;
  state.threadViews.draft = null;
  state.threadViews.syncError = null;
});

describe("ThreadsViewControls", () => {
  it("deletes the captured desktop view only after named confirmation", async () => {
    render(
      <ThreadsViewControls
        candidates={EMPTY_CANDIDATES}
        admittedCount={0}
        matchingCount={0}
        hiddenCount={0}
      />,
    );

    fireEvent.click(screen.getByTestId(VIEW_SETTINGS_TEST_ID));
    fireEvent.click(screen.getByTestId(DELETE_ACTION_TEST_ID));

    expect(state.deleteThreadView).not.toHaveBeenCalled();
    const dialog = await screen.findByRole("dialog", { name: "Delete All threads?" });
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(state.deleteThreadView).not.toHaveBeenCalled();
    expect(screen.getByTestId("threads-view-settings-popover")).toBeTruthy();

    fireEvent.click(screen.getByTestId(DELETE_ACTION_TEST_ID));
    fireEvent.click(screen.getByRole("button", { name: "Delete All threads" }));

    await waitFor(() => expect(state.deleteThreadView).toHaveBeenCalledWith(ALL_VIEW.id));
    expect(state.deleteThreadView).toHaveBeenCalledOnce();
    await waitFor(() => expect(screen.queryByTestId("threads-view-settings-popover")).toBeNull());
  });

  it("hosts mobile deletion in the existing editor drawer until confirmed", async () => {
    responsive.isMobile = true;
    responsive.usesDesktopWorkbench = false;
    responsive.isFinePointer = false;
    render(
      <ThreadsViewControls
        candidates={EMPTY_CANDIDATES}
        admittedCount={0}
        matchingCount={0}
        hiddenCount={0}
      />,
    );

    fireEvent.click(screen.getByTestId(MOBILE_VIEW_TRIGGER_TEST_ID));
    fireEvent.click(await screen.findByTestId("threads-mobile-view-settings"));
    const trigger = await screen.findByTestId(DELETE_ACTION_TEST_ID);
    const drawerId = screen.getByTestId(MOBILE_DRAWER_TEST_ID).id;
    fireEvent.click(trigger);

    expect(state.deleteThreadView).not.toHaveBeenCalled();
    const confirmation = screen.getByRole("group", { name: "Delete All threads?" });
    expect(within(confirmation).getByRole("button", { name: "Cancel" }).className).toContain(
      "min-h-12",
    );
    expect(trigger.isConnected).toBe(true);
    expect(screen.getByRole("dialog", { name: "Delete All threads?" }).id).toBe(drawerId);
    expect(document.querySelectorAll('[data-slot="drawer-content"]')).toHaveLength(1);
    fireEvent.click(within(confirmation).getByRole("button", { name: "Cancel" }));
    expect(screen.getByTestId(MOBILE_DRAWER_TEST_ID).dataset.state).toBe("open");

    fireEvent.click(screen.getByTestId(DELETE_ACTION_TEST_ID));
    fireEvent.click(screen.getByRole("button", { name: "Delete All threads" }));

    await waitFor(() => expect(state.deleteThreadView).toHaveBeenCalledWith(ALL_VIEW.id));
    expect(state.deleteThreadView).toHaveBeenCalledOnce();
    await waitFor(() =>
      expect(screen.getByTestId(MOBILE_DRAWER_TEST_ID).dataset.state).toBe("closed"),
    );
  });

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
    expect(screen.getByText("2 of 5 chats")).toBeTruthy();
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

    fireEvent.click(screen.getByTestId(VIEW_SETTINGS_TEST_ID));
    expect(screen.getByTestId("threads-view-editor")).toBeTruthy();
    fireEvent.click(screen.getByTestId("threads-filter-add"));

    expect(state.updateThreadViewDraft).toHaveBeenCalledWith(
      expect.objectContaining({ filters: expect.arrayContaining([expect.any(Object)]) }),
    );
  });
});

describe("ThreadsViewControls mobile composition", () => {
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

    const trigger = screen.getByTestId(MOBILE_VIEW_TRIGGER_TEST_ID);
    expect(trigger).toBeTruthy();
    expect(screen.queryByTestId(VIEW_PICKER_TEST_ID)).toBeNull();

    fireEvent.click(trigger);
    expect(await screen.findByTestId(MOBILE_DRAWER_TEST_ID)).toBeTruthy();
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
      layout: "columns",
      autoHideComposer: false,
    };
    render(
      <ThreadsViewControls
        candidates={EMPTY_CANDIDATES}
        admittedCount={0}
        matchingCount={0}
        hiddenCount={0}
      />,
    );

    fireEvent.click(screen.getByTestId(MOBILE_VIEW_TRIGGER_TEST_ID));
    fireEvent.click(await screen.findByTestId("threads-mobile-view-settings"));
    expect(await screen.findByTestId("threads-view-editor")).toBeTruthy();
    fireEvent.click(screen.getByTestId("threads-open-task-picker"));
    expect(screen.getByTestId("threads-task-picker")).toBeTruthy();
    fireEvent.click(screen.getByTestId("threads-task-picker-back"));
    expect(screen.getByTestId("threads-view-editor")).toBeTruthy();
    expect(screen.getByTestId(MOBILE_DRAWER_TEST_ID)).toBeTruthy();
  });
});

describe("Threads saved-view draft protection", () => {
  it.each(["desktop", "phone", "tablet"])(
    "requires Save or Discard before switching on %s",
    async (mode) => {
      responsive.isMobile = mode === "phone";
      responsive.isFinePointer = mode === "desktop";
      responsive.usesDesktopWorkbench = mode !== "phone";
      state.threadViews.draft = { ...ALL_VIEW, baseViewId: ALL_VIEW.id, layout: "grid" };
      render(
        <ThreadsViewControls
          candidates={EMPTY_CANDIDATES}
          admittedCount={0}
          matchingCount={0}
          hiddenCount={0}
        />,
      );
      if (mode === "desktop") fireEvent.pointerDown(screen.getByTestId(VIEW_PICKER_TEST_ID));
      else fireEvent.click(screen.getByTestId(MOBILE_VIEW_TRIGGER_TEST_ID));
      const option = await screen.findByTestId(
        mode === "desktop"
          ? "threads-view-option-view-review"
          : "threads-mobile-view-option-view-review",
      );
      expect(option.matches('[disabled], [aria-disabled="true"]')).toBe(true);
      expect(
        screen.getByText("Save or discard your changes in View settings before switching views."),
      ).toBeTruthy();
      fireEvent.click(option);
      expect(state.setThreadActiveView).not.toHaveBeenCalled();
    },
  );
});

describe("Threads Display settings", () => {
  function controls() {
    return (
      <ThreadsViewControls
        candidates={EMPTY_CANDIDATES}
        admittedCount={2}
        matchingCount={5}
        hiddenCount={3}
      />
    );
  }

  it("keeps layout selection inside the configurator with the existing draft actions", async () => {
    state.updateThreadViewDraft.mockImplementationOnce((patch) => {
      state.threadViews.draft = { ...ALL_VIEW, baseViewId: ALL_VIEW.id, ...patch };
    });
    const view = render(controls());
    expect(screen.queryByTestId("threads-layout-shortcut")).toBeNull();
    expect(screen.queryByRole("combobox")).toBeNull();
    fireEvent.click(screen.getByTestId(VIEW_SETTINGS_TEST_ID));
    fireEvent.keyDown(screen.getByTestId("threads-layout-select"), { key: "ArrowDown" });
    fireEvent.keyDown(await screen.findByRole("option", { name: "Grid" }), { key: "Enter" });
    expect(state.updateThreadViewDraft).toHaveBeenCalledExactlyOnceWith({ layout: "grid" });
    view.rerender(controls());
    expect(await screen.findByTestId("threads-view-settings-popover")).toBeTruthy();
    expect(screen.getByTestId("threads-view-save")).toBeTruthy();
    expect(screen.getByTestId("threads-view-discard")).toBeTruthy();
    expect(state.saveThreadViewDraftOverwrite).not.toHaveBeenCalled();
  });

  it("shows draft presentation values and sends minimal editor patches", async () => {
    state.threadViews.draft = {
      ...ALL_VIEW,
      baseViewId: ALL_VIEW.id,
      layout: "grid",
      autoHideComposer: true,
    };
    render(controls());
    expect(screen.queryByTestId("threads-layout-shortcut")).toBeNull();
    fireEvent.click(screen.getByTestId(VIEW_SETTINGS_TEST_ID));
    expect(screen.getByTestId("threads-layout-select").textContent).toBe("Grid");
    const toggle = screen.getByRole("switch", { name: "Auto-hide composer" });
    expect(toggle.getAttribute("aria-checked")).toBe("true");
    fireEvent.click(toggle);
    expect(state.updateThreadViewDraft).toHaveBeenLastCalledWith({ autoHideComposer: false });
    fireEvent.keyDown(screen.getByTestId("threads-layout-select"), { key: "ArrowDown" });
    fireEvent.keyDown(await screen.findByRole("option", { name: "Columns" }), { key: "Enter" });
    expect(state.updateThreadViewDraft).toHaveBeenLastCalledWith({ layout: "columns" });
    expect(screen.getByLabelText("Maximum chats")).toBeTruthy();
    fireEvent.click(screen.getByTestId("threads-view-discard"));
    expect(state.discardThreadViewDraft).toHaveBeenCalledOnce();
  });

  it("uses the existing drawer on a wide coarse-pointer tablet", async () => {
    responsive.usesDesktopWorkbench = true;
    responsive.isFinePointer = false;
    render(controls());
    expect(screen.queryByTestId("threads-layout-shortcut")).toBeNull();
    fireEvent.click(screen.getByTestId(MOBILE_VIEW_TRIGGER_TEST_ID));
    fireEvent.click(await screen.findByTestId("threads-mobile-view-settings"));
    expect(screen.getAllByTestId(MOBILE_DRAWER_TEST_ID)).toHaveLength(1);
    expect(screen.getByRole("switch", { name: "Auto-hide composer" })).toBeTruthy();
    expect(screen.getByTestId("threads-touch-composer-hint")).toBeTruthy();
  });
});
