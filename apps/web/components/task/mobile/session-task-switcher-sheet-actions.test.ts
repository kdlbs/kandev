import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => {
  const targetTaskId = "target-task";
  const targetSessionId = "target-session";
  const targetTask = {
    id: targetTaskId,
    title: "Target task",
    state: "TODO",
    workflow_id: "workflow-1",
    primarySessionId: targetSessionId,
  };
  const state = {
    tasks: {
      activeTaskId: "current-task",
      lastSessionByTaskId: {},
    },
    environmentIdBySessionId: { [targetSessionId]: "environment-1" },
    taskSessions: {
      items: {
        [targetSessionId]: { id: targetSessionId, task_id: targetTaskId },
      },
    },
    kanbanMulti: { snapshots: {} },
    kanban: { tasks: [targetTask], workflowId: "workflow-1", isLoading: false },
    workspaces: { activeId: "workspace-2" },
    workspaceContextGeneration: 0,
    setActiveWorkspace: vi.fn(),
    setWorkspaceContextRead: vi.fn(),
  };
  const store = {
    getState: () => state,
    setState: vi.fn((updater: unknown) => {
      const next =
        typeof updater === "function"
          ? (updater as (current: typeof state) => typeof state)(state)
          : updater;
      if (next && typeof next === "object") Object.assign(state, next);
    }),
  };

  return {
    targetTaskId,
    targetSessionId,
    state,
    store,
    setActiveTask: vi.fn(),
    setActiveSession: vi.fn(),
    loadTaskSessionsForTask: vi.fn(async () => []),
    listWorkflows: vi.fn(async () => ({ workflows: [] })),
    fetchWorkflowSnapshot: vi.fn(),
    routerReplace: vi.fn(),
    archiveAndSwitch: vi.fn(),
    deleteTaskById: vi.fn(),
    removeTaskFromBoard: vi.fn(),
    navigationRequest: vi.fn(),
  };
});

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      ...mocks.state,
      setActiveTask: mocks.setActiveTask,
      setActiveSession: mocks.setActiveSession,
    }),
  useAppStoreApi: () => mocks.store,
}));

vi.mock("@/hooks/use-task-actions", () => ({
  useArchiveAndSwitchTask: () => mocks.archiveAndSwitch,
  useTaskActions: () => ({ deleteTaskById: mocks.deleteTaskById }),
}));

vi.mock("@/hooks/use-task-removal", () => ({
  useTaskRemoval: () => ({
    loadTaskSessionsForTask: mocks.loadTaskSessionsForTask,
    runTaskRemoval: vi.fn(),
    removeTaskFromBoard: mocks.removeTaskFromBoard,
  }),
  useTaskRemovalSuccessNotifier: () => vi.fn(),
}));

vi.mock("@/hooks/use-detach-task", () => ({
  useTaskDetachDialog: () => ({
    detachingTask: null,
    detachingTaskId: null,
    setDetachingTask: vi.fn(),
    handleDetachTask: vi.fn(),
    handleDetachConfirm: vi.fn(),
  }),
}));

vi.mock("@/hooks/use-nest-task", () => ({
  useNestTaskByDrag: () => vi.fn(),
}));

vi.mock("@/lib/kanban/find-task", () => ({
  findTaskInSnapshots: (taskId: string, _snapshots: unknown, tasks: Array<{ id: string }>) =>
    tasks.find((task) => task.id === taskId),
}));

vi.mock("@/lib/api", () => ({
  listWorkflows: mocks.listWorkflows,
  fetchWorkflowSnapshot: mocks.fetchWorkflowSnapshot,
}));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ replace: (...args: unknown[]) => mocks.routerReplace(...args) }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

import { useSheetActions } from "./session-task-switcher-sheet-hooks";
import { createTaskSheetSelectionController } from "./session-task-switcher-sheet-selection";

describe("useSheetActions dirty-navigation boundary", () => {
  beforeEach(() => {
    mocks.setActiveTask.mockReset();
    mocks.setActiveSession.mockReset();
    mocks.loadTaskSessionsForTask.mockReset();
    mocks.loadTaskSessionsForTask.mockResolvedValue([]);
    mocks.listWorkflows.mockReset();
    mocks.listWorkflows.mockResolvedValue({ workflows: [] });
    mocks.routerReplace.mockReset();
    mocks.navigationRequest.mockReset();
  });

  it("defers task selection until the dirty-navigation boundary confirms it", () => {
    const onOpenChange = vi.fn();
    const { result } = renderHook(() =>
      useSheetActions(
        "workspace-1",
        onOpenChange,
        createTaskSheetSelectionController(),
        mocks.navigationRequest,
      ),
    );

    act(() => result.current.handleSelectTask(mocks.targetTaskId));

    expect(mocks.navigationRequest).toHaveBeenCalledOnce();
    expect(mocks.setActiveSession).not.toHaveBeenCalled();
    const deferredAction = mocks.navigationRequest.mock.calls[0]?.[0] as () => void;

    act(() => deferredAction());

    expect(mocks.setActiveSession).toHaveBeenCalledWith(mocks.targetTaskId, mocks.targetSessionId);
    expect(mocks.routerReplace).toHaveBeenCalledWith("/t/target-task");
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("defers workspace switching until the dirty-navigation boundary confirms it", async () => {
    const onOpenChange = vi.fn();
    const { result } = renderHook(() =>
      useSheetActions(
        "workspace-1",
        onOpenChange,
        createTaskSheetSelectionController(),
        mocks.navigationRequest,
      ),
    );

    await act(async () => {
      await result.current.handleWorkspaceChange("workspace-2");
    });

    expect(mocks.navigationRequest).toHaveBeenCalledOnce();
    expect(mocks.listWorkflows).not.toHaveBeenCalled();
    const deferredAction = mocks.navigationRequest.mock.calls[0]?.[0] as () => Promise<void>;

    await act(async () => deferredAction());

    expect(mocks.listWorkflows).toHaveBeenCalledWith("workspace-2", {
      cache: "no-store",
      includeHidden: true,
    });
  });
});
