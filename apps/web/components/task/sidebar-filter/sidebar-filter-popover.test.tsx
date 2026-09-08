import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";
import { SidebarFilterPopover } from "./sidebar-filter-popover";

const responsive = vi.hoisted(() => ({
  usesDesktopWorkbench: true,
  isFinePointer: true,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsive,
}));

const VIEW: SidebarView = {
  id: "view-all",
  name: "All tasks",
  filters: [],
  sort: { key: "state", direction: "asc" },
  group: "repository",
  collapsedGroups: [],
  taskRow: {
    detailsEnabled: true,
    detailOrder: ["relative_time", "repository", "pull_request_number"],
    visibleDetails: ["relative_time", "repository", "pull_request_number"],
    trailing: "git_changes",
  },
};

const SECOND_VIEW: SidebarView = {
  ...VIEW,
  id: "view-review",
  name: "Needs review",
};

const state = {
  sidebarViews: {
    views: [VIEW],
    activeViewId: VIEW.id,
    draft: null,
  },
  updateSidebarDraft: vi.fn(),
  saveSidebarDraftAs: vi.fn(),
  saveSidebarDraftOverwrite: vi.fn(),
  discardSidebarDraft: vi.fn(),
  deleteSidebarView: vi.fn(),
  renameSidebarView: vi.fn(),
  workspaces: { activeId: null },
  kanbanMulti: { snapshots: {} },
  workflows: { items: [] },
  agentProfiles: { items: [] },
  executors: { items: [] },
  userSettings: { sidebarTaskColorAutomation: { enabled: false, rules: [] } },
  setUserSettings: vi.fn(),
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: typeof state) => unknown) => selector(state),
  useAppStoreApi: () => ({ getState: () => state }),
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  state.sidebarViews.views = [VIEW];
  state.sidebarViews.activeViewId = VIEW.id;
  responsive.usesDesktopWorkbench = true;
  responsive.isFinePointer = true;
});

describe("SidebarFilterPopover task-row editor", () => {
  it("waits for named confirmation before deleting the active view", async () => {
    state.sidebarViews.views = [VIEW, SECOND_VIEW];
    render(
      <SidebarFilterPopover
        trigger={<button type="button">Open</button>}
        open
        onOpenChange={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByTestId("view-delete-button"));

    expect(state.deleteSidebarView).not.toHaveBeenCalled();
    const confirmation = await screen.findByRole("dialog", { name: "Delete All tasks?" });
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(confirmation.isConnected).toBe(false);
    expect(state.deleteSidebarView).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTestId("view-delete-button"));
    fireEvent.click(screen.getByRole("button", { name: "Delete All tasks" }));

    await waitFor(() => expect(state.deleteSidebarView).toHaveBeenCalledWith(VIEW.id));
    expect(state.deleteSidebarView).toHaveBeenCalledOnce();
  });

  it("keeps deletion touch-reachable in a phone drawer with a fine pointer", async () => {
    responsive.usesDesktopWorkbench = false;
    responsive.isFinePointer = true;
    state.sidebarViews.views = [VIEW, SECOND_VIEW];
    render(
      <SidebarFilterPopover
        trigger={<button type="button">Open</button>}
        open
        onOpenChange={vi.fn()}
      />,
    );

    const deleteButton = screen.getByTestId("view-delete-button");
    expect(deleteButton.className).toContain("min-h-11");
    fireEvent.click(deleteButton);

    expect(state.deleteSidebarView).not.toHaveBeenCalled();
    expect(await screen.findByRole("group", { name: "Delete All tasks?" })).toBeTruthy();
  });

  it("keeps view settings collapsed until the user opens them", () => {
    render(
      <SidebarFilterPopover
        trigger={<button type="button">Open</button>}
        open
        onOpenChange={vi.fn()}
      />,
    );

    expect(screen.getByTestId("task-row-settings-toggle")).toBeTruthy();
    expect(screen.queryByTestId("task-row-details-toggle")).toBeNull();
    expect(screen.queryByTestId("sort-key-select")).toBeNull();
    expect(screen.queryByTestId("group-key-select")).toBeNull();
    expect(screen.getByText("Status, Sort direction asc", { exact: true })).toBeTruthy();

    fireEvent.click(screen.getByTestId("sidebar-sort-settings-toggle"));
    expect(screen.getByTestId("sort-key-select")).toBeTruthy();
    fireEvent.click(screen.getByTestId("sidebar-group-settings-toggle"));
    expect(screen.getByTestId("group-key-select")).toBeTruthy();

    fireEvent.click(screen.getByTestId("task-row-settings-toggle"));
    expect(screen.getByTestId("task-row-details-toggle")).toBeTruthy();
    expect(state.updateSidebarDraft).not.toHaveBeenCalled();
  });

  it("gives each collapsed view setting the same bottom separator", () => {
    render(
      <SidebarFilterPopover
        trigger={<button type="button">Open</button>}
        open
        onOpenChange={vi.fn()}
      />,
    );

    for (const testId of ["sidebar-sort-settings", "sidebar-group-settings", "task-row-settings"]) {
      const classes = screen.getByTestId(testId).className.split(" ");
      expect(classes).toContain("border-b");
      expect(classes).toContain("pb-1");
      expect(classes).toContain("pt-1");
    }
    expect(screen.getByTestId("automatic-color-settings").className.split(" ")).not.toContain(
      "border-t",
    );
  });
});

describe("SidebarFilterPopover option details", () => {
  it("removes the popover primitive's default section gap", () => {
    render(
      <SidebarFilterPopover
        trigger={<button type="button">Open</button>}
        open
        onOpenChange={vi.fn()}
      />,
    );

    expect(screen.getByTestId("sidebar-filter-popover").className.split(" ")).toContain("gap-0");
  });

  it("describes every group-by and right-side option", () => {
    const renderEditor = () =>
      render(
        <SidebarFilterPopover
          trigger={<button type="button">Open</button>}
          open
          onOpenChange={vi.fn()}
        />,
      );

    renderEditor();
    fireEvent.click(screen.getByTestId("sidebar-group-settings-toggle"));
    fireEvent.click(screen.getByTestId("group-key-select"));
    for (const { label, description } of [
      { label: "None", description: "Keep all tasks in one list." },
      { label: "Repository", description: "Separate tasks by repository." },
      { label: "Workflow", description: "Separate tasks by workflow." },
      { label: "Workflow step", description: "Separate tasks by workflow step." },
      { label: "Executor type", description: "Separate tasks by executor type." },
      { label: "State", description: "Separate tasks by state." },
    ]) {
      const option = screen.getByRole("option", { name: label });
      const descriptionId = option.getAttribute("aria-describedby");
      expect(descriptionId).toBeTruthy();
      expect(screen.getByText(description, { exact: true })).toBeTruthy();
    }

    cleanup();
    renderEditor();
    fireEvent.click(screen.getByTestId("task-row-settings-toggle"));
    fireEvent.click(screen.getByTestId("task-row-trailing-select"));
    for (const { label, description } of [
      { label: "Git changes", description: "Show added and removed lines." },
      { label: "Relative time", description: "Show when the task was last updated." },
      {
        label: "Change request status",
        description: "Show the pull request or merge request status.",
      },
      { label: "Nothing", description: "Leave the right side empty." },
    ]) {
      const option = screen.getByRole("option", { name: label });
      const descriptionId = option.getAttribute("aria-describedby");
      expect(descriptionId).toBeTruthy();
      expect(screen.getByText(description, { exact: true })).toBeTruthy();
    }
  });
});
