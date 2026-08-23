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
});
