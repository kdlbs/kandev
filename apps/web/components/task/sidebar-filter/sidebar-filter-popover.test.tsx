import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";
import { SidebarFilterPopover } from "./sidebar-filter-popover";

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
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: typeof state) => unknown) => selector(state),
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("SidebarFilterPopover task-row editor", () => {
  it("keeps the editor collapsed until the user opens it", () => {
    render(
      <SidebarFilterPopover
        trigger={<button type="button">Open</button>}
        open
        onOpenChange={vi.fn()}
      />,
    );

    expect(screen.getByTestId("task-row-settings-toggle")).toBeTruthy();
    expect(screen.queryByTestId("task-row-details-toggle")).toBeNull();
    fireEvent.click(screen.getByTestId("task-row-settings-toggle"));
    expect(screen.getByTestId("task-row-details-toggle")).toBeTruthy();
    expect(state.updateSidebarDraft).not.toHaveBeenCalled();
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
