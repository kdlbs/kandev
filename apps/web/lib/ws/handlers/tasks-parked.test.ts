import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerTasksHandlers } from "./tasks";

// MUST-FIX #2, Review round 3: D1's (parked_epoch, revision) ordered-pair
// discard rule was previously unimplemented at the task level —
// preserveParkedFields only branched on hasPayloadField (field presence),
// never compared parked_revision values, so a stale-but-present task.updated
// frame could regress a fresher parked reading. Split into its own file
// (rather than tasks.test.ts) to keep that file under its 600-line limit.

type Listener = (state: AppState) => void;

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

function makeTask(id: string) {
  return {
    task_id: id,
    workflow_id: "wf1",
    workflow_step_id: "step1",
    title: "Test",
    description: "",
    state: "IN_PROGRESS",
    primary_session_id: null,
    is_ephemeral: false,
  } as Record<string, unknown>;
}

function makeMessage(payload: Record<string, unknown>) {
  return {
    id: "msg-1",
    type: "notification" as const,
    action: "task.updated" as const,
    payload,
  } as Parameters<NonNullable<ReturnType<typeof registerTasksHandlers>["task.updated"]>>[0];
}

function makeParkedTaskStore(
  parkedOnBackgroundWork: boolean,
  parkedEpoch: number,
  parkedRevision: number,
) {
  return makeStore({
    kanban: {
      workflowId: "wf1",
      steps: [],
      tasks: [
        {
          id: "t1",
          workflowId: "wf1",
          parkedOnBackgroundWork,
          parkedEpoch,
          parkedRevision,
        },
      ],
    } as unknown as AppState["kanban"],
  });
}

describe("task.updated parked projection ordering (AC-49, D1)", () => {
  it("discards a stale parked_revision arriving after a fresher one within the same epoch", () => {
    const store = makeParkedTaskStore(true, 100, 7);

    registerTasksHandlers(store)["task.updated"]!(
      makeMessage({
        ...makeTask("t1"),
        parked_on_background_work: false,
        parked_epoch: 100,
        parked_revision: 6,
      }),
    );

    const task = store.getState().kanban.tasks[0];
    expect(task).toMatchObject({
      parkedOnBackgroundWork: true,
      parkedEpoch: 100,
      parkedRevision: 7,
    });
  });

  it("accepts a strictly higher epoch even carrying a lower revision (AC-77 restart reset)", () => {
    const store = makeParkedTaskStore(true, 100, 7);

    registerTasksHandlers(store)["task.updated"]!(
      makeMessage({
        ...makeTask("t1"),
        parked_on_background_work: false,
        parked_epoch: 200,
        parked_revision: 0,
      }),
    );

    const task = store.getState().kanban.tasks[0];
    expect(task).toMatchObject({
      parkedOnBackgroundWork: false,
      parkedEpoch: 200,
      parkedRevision: 0,
    });
  });

  it("accepts a higher revision within the same epoch", () => {
    const store = makeParkedTaskStore(false, 100, 1);

    registerTasksHandlers(store)["task.updated"]!(
      makeMessage({
        ...makeTask("t1"),
        parked_on_background_work: true,
        parked_epoch: 100,
        parked_revision: 2,
      }),
    );

    const task = store.getState().kanban.tasks[0];
    expect(task).toMatchObject({ parkedOnBackgroundWork: true, parkedRevision: 2 });
  });

  it("leaves the parked triple untouched when the frame omits parked_on_background_work and parked_revision", () => {
    const store = makeParkedTaskStore(true, 100, 7);

    registerTasksHandlers(store)["task.updated"]!(makeMessage({ ...makeTask("t1") }));

    const task = store.getState().kanban.tasks[0];
    expect(task).toMatchObject({
      parkedOnBackgroundWork: true,
      parkedEpoch: 100,
      parkedRevision: 7,
    });
  });
});
