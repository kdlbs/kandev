import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ToastProvider } from "@/components/toast-provider";
import { MobileTaskList } from "@/components/task/mobile/session-task-switcher-sheet";
import type { TaskSwitcherItem } from "@/components/task/task-switcher";

const mocks = vi.hoisted(() => ({
  toggleSidebarGroupCollapsed: vi.fn(),
  appState: {
    taskPRs: { byTaskId: {} },
    comments: { byTaskId: {} },
  },
}));

type MockAppState = typeof mocks.appState & {
  toggleSidebarGroupCollapsed: typeof mocks.toggleSidebarGroupCollapsed;
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockAppState) => unknown) =>
    selector({
      ...mocks.appState,
      toggleSidebarGroupCollapsed: mocks.toggleSidebarGroupCollapsed,
    }),
}));

vi.mock("@/hooks/domains/sidebar/use-effective-sidebar-view", () => ({
  useEffectiveSidebarView: () => ({
    id: "default",
    name: "Default",
    filters: [],
    sort: { key: "updatedAt", direction: "desc" },
    group: "workflowStep",
    collapsedGroups: [],
  }),
}));

vi.mock("@/hooks/domains/sidebar/use-sidebar-task-prefs", () => ({
  useSidebarTaskPrefs: () => ({
    pinnedTaskIds: [],
    orderedTaskIds: [],
    subtaskOrderByParentId: {},
    togglePinnedTask: vi.fn(),
    handleReorderGroup: vi.fn(),
    handleReorderSubtasks: vi.fn(),
  }),
}));

function task(id: string): TaskSwitcherItem {
  return {
    id,
    title: `Task ${id}`,
    state: "TODO",
    workflowId: "workflow-1",
    workflowName: "Workflow",
    workflowStepId: "step-1",
    workflowStepTitle: "Build",
  };
}

describe("MobileTaskList", () => {
  beforeEach(() => {
    mocks.toggleSidebarGroupCollapsed.mockClear();
  });

  it("toggles workflow-step groups from the mobile sidebar", () => {
    render(
      <ToastProvider>
        <MobileTaskList
          tasks={["1", "2", "3", "4", "5"].map(task)}
          workflows={[]}
          stepsByWorkflowId={{}}
          activeTaskId={null}
          selectedTaskId={null}
          onSelectTask={vi.fn()}
          onArchiveTask={vi.fn()}
          onDeleteTask={vi.fn()}
          deletingTaskId={null}
        />
      </ToastProvider>,
    );

    const header = screen.getByTestId("sidebar-group-header");
    expect(header.textContent).toContain("Build");
    expect(header.textContent).toContain("5");

    fireEvent.click(header);

    expect(mocks.toggleSidebarGroupCollapsed).toHaveBeenCalledWith("default", "step-1");
  });
});
