import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { removeRecentTask } from "@/lib/recent-tasks";
import type { AppState } from "@/lib/state/store";
import { registerTasksHandlers } from "./tasks";

vi.mock("@/lib/recent-tasks", () => ({
  removeRecentTask: vi.fn(),
}));

type Listener = (state: AppState) => void;
type KanbanTask = { id: string; title: string; workflowId: string; workflowStepId: string };
const TASK_ID = "t1";
const SESSION_ID = "sess-old";
const ARCHIVED_AT = "2026-06-30T12:00:00Z";

function makeStore(initial: Partial<AppState> = {}) {
  let state = {
    kanban: { workflowId: "wf1", steps: [], tasks: [] },
    kanbanMulti: { snapshots: {}, isLoading: false },
    tasks: {
      activeTaskId: null,
      activeSessionId: null,
      pinnedSessionId: null,
      lastSessionByTaskId: {},
    },
    taskSessionsByTask: { itemsByTaskId: {}, loadedByTaskId: {}, loadingByTaskId: {} },
    environmentIdBySessionId: {},
    setActiveSessionAuto: vi.fn(),
    removeTaskFromSidebarPrefs: vi.fn(),
    setTaskDeletedNotification: vi.fn(),
    setOfficeRefetchTrigger: vi.fn(),
    ...initial,
  } as unknown as AppState;

  const listeners = new Set<Listener>();
  return {
    getState: () => state,
    setState: (updater: AppState | ((s: AppState) => AppState)) => {
      const next =
        typeof updater === "function" ? (updater as (s: AppState) => AppState)(state) : updater;
      state = { ...state, ...next };
      for (const listener of listeners) listener(state);
    },
    subscribe: (listener: Listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    destroy: vi.fn(),
    getInitialState: vi.fn(),
  } as unknown as StoreApi<AppState> & { getState: () => AppState };
}

function makeUpdatedMessage(payload: Record<string, unknown>) {
  return {
    id: "msg-1",
    type: "notification" as const,
    action: "task.updated" as const,
    payload,
  } as Parameters<NonNullable<ReturnType<typeof registerTasksHandlers>["task.updated"]>>[0];
}

function makeDeletedMessage(payload: Record<string, unknown>) {
  return {
    id: "msg-1",
    type: "notification" as const,
    action: "task.deleted" as const,
    payload,
  } as Parameters<NonNullable<ReturnType<typeof registerTasksHandlers>["task.deleted"]>>[0];
}

function taskPayload(id: string, workflowId = "wf1") {
  return {
    task_id: id,
    workflow_id: workflowId,
    workflow_step_id: "step1",
    title: "Test",
    state: "IN_PROGRESS",
    is_ephemeral: false,
  };
}

function archiveTask(store: StoreApi<AppState>, taskId = TASK_ID) {
  const handlers = registerTasksHandlers(store);
  handlers["task.updated"]!(
    makeUpdatedMessage({
      ...taskPayload(taskId),
      archived_at: ARCHIVED_AT,
    }),
  );
}

function makeStoreWithTask(initial: Partial<AppState> = {}) {
  return makeStore({
    kanban: {
      workflowId: "wf1",
      steps: [],
      tasks: [{ id: TASK_ID, primarySessionId: SESSION_ID, workflowId: "wf1" }],
    } as unknown as AppState["kanban"],
    ...initial,
  });
}

describe("task.updated archive cleanup", () => {
  beforeEach(() => {
    vi.mocked(removeRecentTask).mockClear();
    window.history.replaceState({}, "", "/");
  });

  it("removes archived tasks from the active kanban cache even when workflow focus changed", () => {
    const staleTask: KanbanTask = {
      id: "t1",
      title: "Test",
      workflowId: "wf1",
      workflowStepId: "step1",
    };
    const store = makeStore({
      kanban: {
        workflowId: "wf-active",
        steps: [],
        tasks: [staleTask],
      } as unknown as AppState["kanban"],
      kanbanMulti: {
        isLoading: false,
        snapshots: {
          wf1: { workflowId: "wf1", workflowName: "WF1", steps: [], tasks: [staleTask] },
        },
      } as unknown as AppState["kanbanMulti"],
    });

    const handlers = registerTasksHandlers(store);
    handlers["task.updated"]!(
      makeUpdatedMessage({
        ...taskPayload(TASK_ID, "wf1"),
        archived_at: ARCHIVED_AT,
      }),
    );

    const state = store.getState();
    expect(state.kanban.tasks).toEqual([]);
    expect(state.kanbanMulti.snapshots.wf1.tasks).toEqual([]);
  });

  it("clears active task state, pin, recent history, and sidebar prefs for archived task events", () => {
    const store = makeStoreWithTask({
      tasks: {
        activeTaskId: TASK_ID,
        activeSessionId: SESSION_ID,
        pinnedSessionId: SESSION_ID,
        lastSessionByTaskId: { [TASK_ID]: SESSION_ID, t2: "sess-other" },
      },
      environmentIdBySessionId: {},
    } as unknown as Partial<AppState>);

    archiveTask(store);

    const state = store.getState();
    expect(state.tasks.activeTaskId).toBeNull();
    expect(state.tasks.activeSessionId).toBeNull();
    expect(state.tasks.pinnedSessionId).toBeNull();
    expect(state.tasks.lastSessionByTaskId).not.toHaveProperty(TASK_ID);
    expect(state.tasks.lastSessionByTaskId).toHaveProperty("t2", "sess-other");
    expect(removeRecentTask).toHaveBeenCalledWith(TASK_ID);
    expect(state.removeTaskFromSidebarPrefs).toHaveBeenCalledWith(TASK_ID);
    expect(state.setOfficeRefetchTrigger).toHaveBeenCalledWith("tasks");
  });

  it.each(["/t/t1", "/tasks/t1"])("redirects away when archived on %s", (path) => {
    window.history.replaceState({}, "", path);
    const store = makeStoreWithTask({
      tasks: {
        activeTaskId: TASK_ID,
        activeSessionId: SESSION_ID,
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
      environmentIdBySessionId: {},
    } as unknown as Partial<AppState>);

    archiveTask(store);

    expect(window.location.pathname).toBe("/");
  });

  it("does not redirect when a different task is archived", () => {
    window.history.replaceState({}, "", "/t/other");
    const store = makeStoreWithTask();

    archiveTask(store);

    expect(window.location.pathname).toBe("/t/other");
  });
});

describe("office task removal routes", () => {
  beforeEach(() => {
    vi.mocked(removeRecentTask).mockClear();
    window.history.replaceState({}, "", "/");
  });

  it.each(["/office/tasks/t1", "/office/tasks/t1/"])(
    "redirects office task detail route %s to the office task list when archived",
    (path) => {
      window.history.replaceState({}, "", path);
      const store = makeStoreWithTask();

      archiveTask(store);

      expect(window.location.pathname).toBe("/office/tasks");
    },
  );

  it("redirects office task detail routes to the office task list when auto-deleted", () => {
    window.history.replaceState({}, "", "/office/tasks/t1");
    const store = makeStoreWithTask();
    const handlers = registerTasksHandlers(store);

    handlers["task.deleted"]!(
      makeDeletedMessage({
        task_id: TASK_ID,
        workflow_id: "wf1",
        title: "Deleted task",
        reason: "pr_approved_by_user",
      }),
    );

    expect(window.location.pathname).toBe("/office/tasks");
    expect(store.getState().setTaskDeletedNotification).toHaveBeenCalledWith({
      taskId: TASK_ID,
      title: "Deleted task",
      reason: "pr_approved_by_user",
    });
  });
});
