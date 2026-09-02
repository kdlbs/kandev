import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Task } from "@/lib/types/http";

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
    targetTask,
    state,
    store,
    setActiveTask: vi.fn(),
    setActiveSession: vi.fn(),
    loadTaskSessionsForTask: vi.fn(async () => []),
    listWorkflows: vi.fn(async () => ({ workflows: [] })),
    fetchWorkflowSnapshot: vi.fn(),
    replaceTaskUrl: vi.fn(),
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
    removeTaskFromBoard: mocks.removeTaskFromBoard,
  }),
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

vi.mock("@/lib/links", () => ({
  replaceTaskUrl: (...args: unknown[]) => mocks.replaceTaskUrl(...args),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

import { useSheetActions } from "./session-task-switcher-sheet-hooks";
import { createTaskSheetSelectionController } from "./session-task-switcher-sheet-selection";

const WORKSPACE_ID = "workspace-1";
const CREATED_TASK_ID = "created-task";
const CREATED_SESSION_ID = "created-session";

function resetMocks() {
  mocks.state.tasks.activeTaskId = "current-task";
  mocks.state.kanban.tasks = [mocks.targetTask];
  mocks.setActiveTask.mockReset();
  mocks.setActiveSession.mockReset();
  mocks.loadTaskSessionsForTask.mockReset();
  mocks.loadTaskSessionsForTask.mockResolvedValue([]);
  mocks.listWorkflows.mockReset();
  mocks.listWorkflows.mockResolvedValue({ workflows: [] });
  mocks.replaceTaskUrl.mockReset();
  mocks.archiveAndSwitch.mockReset().mockResolvedValue(undefined);
  mocks.deleteTaskById.mockReset().mockResolvedValue(undefined);
  mocks.removeTaskFromBoard.mockReset().mockResolvedValue(undefined);
  mocks.store.setState.mockClear();
  mocks.navigationRequest.mockReset();
}

describe("useSheetActions dirty-navigation boundary", () => {
  beforeEach(resetMocks);

  it("defers task selection until the dirty-navigation boundary confirms it", () => {
    const onOpenChange = vi.fn();
    const { result } = renderHook(() =>
      useSheetActions(
        WORKSPACE_ID,
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
    expect(mocks.replaceTaskUrl).toHaveBeenCalledWith(mocks.targetTaskId);
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("defers workspace switching until the dirty-navigation boundary confirms it", async () => {
    const onOpenChange = vi.fn();
    const { result } = renderHook(() =>
      useSheetActions(
        WORKSPACE_ID,
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

describe("useSheetActions task mutation navigation", () => {
  beforeEach(resetMocks);

  it("keeps a created task upserted but defers activation until navigation confirms", () => {
    const onOpenChange = vi.fn();
    const { result } = renderHook(() =>
      useSheetActions(
        WORKSPACE_ID,
        onOpenChange,
        createTaskSheetSelectionController(),
        mocks.navigationRequest,
      ),
    );
    const createdTask = {
      ...mocks.targetTask,
      id: CREATED_TASK_ID,
      title: "Created task",
      primary_session_id: CREATED_SESSION_ID,
    } as unknown as Task;

    act(() =>
      result.current.handleTaskCreated(createdTask, "create", {
        taskSessionId: CREATED_SESSION_ID,
      }),
    );

    expect(mocks.store.setState).toHaveBeenCalledOnce();
    expect(mocks.navigationRequest).toHaveBeenCalledOnce();
    expect(mocks.setActiveTask).not.toHaveBeenCalled();
    expect(mocks.replaceTaskUrl).not.toHaveBeenCalled();

    // A cancelled discard dialog does not invoke the deferred activation.
    expect(onOpenChange).not.toHaveBeenCalled();

    const deferredAction = mocks.navigationRequest.mock.calls[0]?.[0] as () => void;
    act(() => deferredAction());

    expect(mocks.setActiveTask).toHaveBeenCalledWith(CREATED_TASK_ID);
    expect(mocks.setActiveSession).toHaveBeenCalledWith(CREATED_TASK_ID, CREATED_SESSION_ID);
    expect(mocks.replaceTaskUrl).toHaveBeenCalledWith(CREATED_TASK_ID);
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("defers archive of the active task until the dirty-navigation boundary confirms it", async () => {
    mocks.state.tasks.activeTaskId = mocks.targetTaskId;
    const { result } = renderHook(() =>
      useSheetActions(
        WORKSPACE_ID,
        vi.fn(),
        createTaskSheetSelectionController(),
        mocks.navigationRequest,
      ),
    );

    act(() => result.current.handleArchiveTask(mocks.targetTaskId, { cascade: false }));

    expect(mocks.navigationRequest).toHaveBeenCalledOnce();
    expect(mocks.archiveAndSwitch).not.toHaveBeenCalled();
    const deferredAction = mocks.navigationRequest.mock.calls[0]?.[0] as () => Promise<void>;

    await act(async () => deferredAction());

    expect(mocks.archiveAndSwitch).toHaveBeenCalledWith(mocks.targetTaskId, { cascade: false });
  });

  it("defers delete of the active task until the dirty-navigation boundary confirms it", async () => {
    mocks.state.tasks.activeTaskId = mocks.targetTaskId;
    const { result } = renderHook(() =>
      useSheetActions(
        WORKSPACE_ID,
        vi.fn(),
        createTaskSheetSelectionController(),
        mocks.navigationRequest,
      ),
    );

    act(() => result.current.handleDeleteTask(mocks.targetTaskId));
    await act(async () => result.current.handleDeleteConfirm({ cascade: false }));

    expect(mocks.navigationRequest).toHaveBeenCalledOnce();
    expect(mocks.deleteTaskById).not.toHaveBeenCalled();
    const deferredAction = mocks.navigationRequest.mock.calls[0]?.[0] as () => Promise<void>;

    await act(async () => deferredAction());

    expect(mocks.deleteTaskById).toHaveBeenCalledWith(mocks.targetTaskId, { cascade: false });
    expect(mocks.removeTaskFromBoard).toHaveBeenCalledWith(
      mocks.targetTaskId,
      expect.objectContaining({ wasActiveTaskId: mocks.targetTaskId }),
    );
  });
});
