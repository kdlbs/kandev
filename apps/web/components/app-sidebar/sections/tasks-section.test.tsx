import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const state = {
  appSidebar: {
    sectionExpanded: {
      tasks: true,
    },
  },
  workspaces: { activeId: "ws-1" as string | null },
  workflows: { activeId: "wf-1" as string | null },
  sidebarViews: {
    views: [{ id: "all", name: "All tasks" }],
    activeViewId: "all",
    draft: null,
  },
  toggleAppSidebarSection: vi.fn(),
  setAppSidebarCollapsed: vi.fn(),
  setSidebarActiveView: vi.fn(),
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));

vi.mock("@/components/task/task-session-sidebar", () => ({
  TaskSessionSidebar: ({ workflowId }: { workflowId: string | null }) => (
    <div data-testid="task-sidebar" data-workflow-id={workflowId ?? ""} />
  ),
}));

vi.mock("@/components/task/sidebar-filter/sidebar-filter-popover", () => ({
  SidebarFilterPopover: ({ trigger }: { trigger: ReactNode }) => trigger,
}));

import { TasksSection } from "./tasks-section";

function renderSection() {
  return render(
    <TooltipProvider>
      <TasksSection collapsed={false} />
    </TooltipProvider>,
  );
}

describe("TasksSection", () => {
  beforeEach(() => {
    state.appSidebar.sectionExpanded.tasks = true;
    state.workspaces.activeId = "ws-1";
    state.workflows.activeId = "wf-1";
    state.sidebarViews.views = [{ id: "all", name: "All tasks" }];
    state.sidebarViews.activeViewId = "all";
    state.sidebarViews.draft = null;
    state.toggleAppSidebarSection.mockClear();
    state.setAppSidebarCollapsed.mockClear();
    state.setSidebarActiveView.mockClear();
  });

  afterEach(() => cleanup());

  it("keeps the tasks view picker inline in the section header", () => {
    renderSection();

    const picker = screen.getByTestId("tasks-view-picker");
    const header = screen.getByRole("button", { name: "Tasks" }).parentElement;

    expect(header?.contains(picker)).toBe(true);
  });

  it("passes the active workflow from the workflow UI state", () => {
    state.workflows.activeId = "wf-query";

    renderSection();

    expect(screen.getByTestId("task-sidebar").dataset.workflowId).toBe("wf-query");
  });
});
