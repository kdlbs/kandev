import { act, fireEvent, render, renderHook, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ToastProvider } from "@/components/toast-provider";
import {
  MobileTaskList,
  useTaskSheetSelectionController,
} from "@/components/task/mobile/session-task-switcher-sheet";
import { selectTaskFromSheet } from "@/components/task/mobile/session-task-switcher-sheet-selection";
import type { TaskSwitcherItem } from "@/components/task/task-switcher";
import type { TaskSession } from "@/lib/types/http";

const mocks = vi.hoisted(() => ({
  toggleSidebarGroupCollapsed: vi.fn(),
  appState: {
    workspaces: { activeId: "workspace-1" },
    repositories: { itemsByWorkspaceId: {} },
    taskPRs: { byTaskId: {} },
    taskMRs: { byWorkspaceId: {} },
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
  // TaskNestContextMenuItems (rendered inside the shared context menu) reads
  // the store api via useNestTask; provide a minimal stub so it renders.
  useAppStoreApi: () => ({
    getState: () => ({ kanbanMulti: { snapshots: {} } }),
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
          onDetachTask={vi.fn()}
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

  it("opens detach from the touch task actions button for a subtask", () => {
    const onDetachTask = vi.fn();
    render(
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
      </ToastProvider>,
    );

    const childRow = screen
      .getByText("Task child")
      .closest<HTMLElement>("[data-testid='sidebar-task-item']");
    expect(childRow).not.toBeNull();
    fireEvent.click(within(childRow!).getByRole("button", { name: "Task actions" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Detach from parent" }));

    expect(onDetachTask).toHaveBeenCalledWith("child");
  });

  it("opens edit from the touch task actions button", () => {
    const onEditTask = vi.fn();
    const editableTask = task("editable");
    render(
      <ToastProvider>
        <MobileTaskList
          tasks={[editableTask]}
          workflows={[]}
          stepsByWorkflowId={{}}
          activeTaskId={null}
          selectedTaskId={null}
          onSelectTask={vi.fn()}
          onEditTask={onEditTask}
          onArchiveTask={vi.fn()}
          onDeleteTask={vi.fn()}
          onDetachTask={vi.fn()}
          deletingTaskId={null}
        />
      </ToastProvider>,
    );

    const row = screen
      .getByText(editableTask.title)
      .closest<HTMLElement>("[data-testid='sidebar-task-item']");
    expect(row).not.toBeNull();
    fireEvent.click(within(row!).getByRole("button", { name: "Task actions" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Edit" }));

    expect(onEditTask).toHaveBeenCalledWith(editableTask);
  });
});

describe("SessionTaskSwitcherSheet lifecycle", () => {
  it("invalidates a deferred selection when responsive remount replaces the sheet", async () => {
    const phoneTaskId = "task-phone";
    const tabletTaskId = "task-tablet";
    let resolveOldLoad: (sessions: TaskSession[]) => void = () => undefined;
    let resolveNewLoad: (sessions: TaskSession[]) => void = () => undefined;
    const oldLoad = vi.fn(
      () =>
        new Promise<TaskSession[]>((resolve) => {
          resolveOldLoad = resolve;
        }),
    );
    const newLoad = vi.fn(
      () =>
        new Promise<TaskSession[]>((resolve) => {
          resolveNewLoad = resolve;
        }),
    );
    const setActiveSession = vi.fn();
    const navigate = vi.fn();
    const onOpenChange = vi.fn();
    const state = {
      lastSessionByTaskId: {},
      environmentIdBySessionId: {},
      taskSessionsById: {},
    };
    const oldSheet = renderHook(() => useTaskSheetSelectionController());

    selectTaskFromSheet({
      selectionController: oldSheet.result.current,
      taskId: phoneTaskId,
      task: { primarySessionId: "primary-phone", taskPendingAction: "clarification" },
      state,
      loadTaskSessionsForTask: oldLoad,
      setActiveSession,
      setActiveTask: vi.fn(),
      navigate,
      onOpenChange,
    });
    oldSheet.unmount();

    const tabletSheet = renderHook(() => useTaskSheetSelectionController());
    selectTaskFromSheet({
      selectionController: tabletSheet.result.current,
      taskId: tabletTaskId,
      task: { primarySessionId: "primary-tablet", taskPendingAction: "clarification" },
      state,
      loadTaskSessionsForTask: newLoad,
      setActiveSession,
      setActiveTask: vi.fn(),
      navigate,
      onOpenChange,
    });
    await act(async () => {
      resolveNewLoad([
        {
          id: "owner-tablet",
          task_id: tabletTaskId,
          state: "WAITING_FOR_INPUT",
          pending_action: "clarification",
        } as TaskSession,
      ]);
    });
    await act(async () => {
      resolveOldLoad([
        {
          id: "owner-phone",
          task_id: phoneTaskId,
          state: "WAITING_FOR_INPUT",
          pending_action: "clarification",
        } as TaskSession,
      ]);
    });

    expect(setActiveSession).toHaveBeenCalledTimes(1);
    expect(setActiveSession).toHaveBeenCalledWith(tabletTaskId, "owner-tablet");
    expect(navigate).toHaveBeenCalledTimes(1);
    expect(navigate).toHaveBeenCalledWith(tabletTaskId);
    expect(onOpenChange).toHaveBeenCalledTimes(1);
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});
