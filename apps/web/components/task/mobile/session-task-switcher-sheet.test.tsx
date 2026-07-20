import { fireEvent, render, screen, within } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ToastProvider } from "@/components/toast-provider";
import { MobileTaskList } from "@/components/task/mobile/session-task-switcher-sheet";
import type { TaskSwitcherItem } from "@/components/task/task-switcher";
import { makeQueryClient } from "@/lib/query/client";

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
    const queryClient = makeQueryClient();
    render(
      <QueryClientProvider client={queryClient}>
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
            onDetachTask={vi.fn()}
            deletingTaskId={null}
          />
        </ToastProvider>
      </QueryClientProvider>,
    );

    const header = screen.getByTestId("sidebar-group-header");
    expect(header.textContent).toContain("Build");
    expect(header.textContent).toContain("5");

    fireEvent.click(header);

    expect(mocks.toggleSidebarGroupCollapsed).toHaveBeenCalledWith("default", "step-1");
  });

  it("opens detach from the touch task actions button for a subtask", () => {
    const onDetachTask = vi.fn();
    const queryClient = makeQueryClient();
    render(
      <QueryClientProvider client={queryClient}>
        <ToastProvider>
          <MobileTaskList
            tasks={[task("parent"), { ...task("child"), parentTaskId: "parent" }]}
            workflows={[]}
            stepsByWorkflowId={{}}
            activeTaskId={null}
            selectedTaskId={null}
            onSelectTask={vi.fn()}
            onArchiveTask={vi.fn()}
            onDeleteTask={vi.fn()}
            onDetachTask={onDetachTask}
            deletingTaskId={null}
          />
        </ToastProvider>
      </QueryClientProvider>,
    );

    const childRow = screen
      .getByText("Task child")
      .closest<HTMLElement>("[data-testid='sidebar-task-item']");
    expect(childRow).not.toBeNull();
    fireEvent.click(within(childRow!).getByRole("button", { name: "Task actions" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Detach from parent" }));

    expect(onDetachTask).toHaveBeenCalledWith("child");
  });
});
