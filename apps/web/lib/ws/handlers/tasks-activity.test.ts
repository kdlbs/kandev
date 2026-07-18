import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerTasksHandlers } from "./tasks";

vi.mock("@/lib/recent-tasks", () => ({
  removeRecentTask: vi.fn(),
}));

type Listener = (state: AppState) => void;

// Self-contained harness (kept separate from tasks.test.ts, which already sits at
// the file-length limit) covering the task-level MOST-ACTIVE-WINS activity
// aggregate carried by task.updated — the live-propagation + safe-fallback path
// the board card and task-list rows read (§spec:live-propagation-fallback).
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
    setActiveSession: vi.fn(),
    setActiveSessionAuto: vi.fn(),
    removeTaskFromSidebarPrefs: vi.fn(),
    setTaskDeletedNotification: vi.fn(),
    ...initial,
  } as unknown as AppState;

  const listeners = new Set<Listener>();
  return {
    getState: () => state,
    setState: (updater: AppState | ((s: AppState) => AppState)) => {
      const next =
        typeof updater === "function" ? (updater as (s: AppState) => AppState)(state) : updater;
      state = { ...state, ...next };
      for (const l of listeners) l(state);
    },
    subscribe: (l: Listener) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    destroy: vi.fn(),
    getInitialState: vi.fn(),
  } as unknown as StoreApi<AppState> & { getState: () => AppState };
}

function makeTask(id: string): Record<string, unknown> {
  return {
    task_id: id,
    workflow_id: "wf1",
    workflow_step_id: "step1",
    title: "Test",
    description: "",
    state: "IN_PROGRESS",
    primary_session_id: null,
    is_ephemeral: false,
  };
}

function makeMessage(payload: Record<string, unknown>) {
  return {
    id: "msg-1",
    type: "notification" as const,
    action: "task.updated" as const,
    payload,
  } as Parameters<NonNullable<ReturnType<typeof registerTasksHandlers>["task.updated"]>>[0];
}

describe("task.updated task-level activity aggregate (live propagation + safe fallback)", () => {
  type ExistingTask = { id: string; foregroundActivity?: "generating" | "background" };

  function storeWithTask(existing: ExistingTask) {
    const task = { workflowStepId: "step1", title: "T", position: 0, ...existing };
    return makeStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [task],
      } as unknown as AppState["kanban"],
      kanbanMulti: {
        isLoading: false,
        snapshots: {
          wf1: { workflowId: "wf1", workflowName: "WF1", steps: [], tasks: [task] },
        },
      } as unknown as AppState["kanbanMulti"],
    });
  }

  function activityFor(store: ReturnType<typeof makeStore>, id: string) {
    const kanban = store.getState().kanban.tasks.find((t) => t.id === id)?.foregroundActivity;
    const multi = store
      .getState()
      .kanbanMulti.snapshots.wf1.tasks.find((t) => t.id === id)?.foregroundActivity;
    return { kanban, multi };
  }

  it("applies a generating→background flip without any coarse state change", () => {
    // A bare activity flip (coarse state stays IN_PROGRESS) must refresh the
    // task-level surfaces so the board card and list read background-running
    // promptly, not on the next coarse transition.
    const store = storeWithTask({ id: "t1", foregroundActivity: "generating" });
    const handlers = registerTasksHandlers(store);

    handlers["task.updated"]!(
      makeMessage({ ...makeTask("t1"), state: "IN_PROGRESS", foreground_activity: "background" }),
    );

    const { kanban, multi } = activityFor(store, "t1");
    expect(kanban).toBe("background");
    expect(multi).toBe("background");
    expect(store.getState().kanban.tasks.find((t) => t.id === "t1")?.state).toBe("IN_PROGRESS");
  });

  it("clears a stale background reading when the aggregate goes empty (explicit null wins)", () => {
    // The done transition: the backend recomputes an empty aggregate and emits an
    // explicit foreground_activity: null. That null must clear the prior
    // background so the card can settle to its coarse (done) reading — never a
    // spinner stuck on forever.
    const store = storeWithTask({ id: "t1", foregroundActivity: "background" });
    const handlers = registerTasksHandlers(store);

    handlers["task.updated"]!(makeMessage({ ...makeTask("t1"), foreground_activity: null }));

    const { kanban, multi } = activityFor(store, "t1");
    expect(kanban ?? undefined).toBeUndefined();
    expect(multi ?? undefined).toBeUndefined();
  });

  it("preserves the aggregate when a partial update omits foreground_activity", () => {
    // Clobber guard: a lightweight task.updated that carries no activity field
    // (e.g. a rename) must NOT wipe a live background-running reading, or the
    // surface would falsely flip toward done between real activity events.
    const store = storeWithTask({ id: "t1", foregroundActivity: "background" });
    const handlers = registerTasksHandlers(store);

    handlers["task.updated"]!(makeMessage({ ...makeTask("t1"), title: "Renamed" }));

    const { kanban, multi } = activityFor(store, "t1");
    expect(kanban).toBe("background");
    expect(multi).toBe("background");
  });
});
