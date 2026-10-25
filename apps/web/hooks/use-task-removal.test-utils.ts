import type { StoreApi } from "zustand";
import { vi } from "vitest";

export type TaskRow = {
  id: string;
  primarySessionId: string | null;
  parentTaskId?: string | null;
  workspaceId?: string | null;
};

export type FakeState = {
  tasks: { activeTaskId: string | null; activeSessionId: string | null };
  kanban: { tasks: TaskRow[] };
  kanbanMulti: { snapshots: Record<string, { tasks: TaskRow[] }> };
  environmentIdBySessionId: Record<string, string>;
  taskSessionsByTask: {
    itemsByTaskId: Record<string, never[]>;
    loadedByTaskId: Record<string, boolean>;
    loadingByTaskId: Record<string, boolean>;
  };
  setActiveTask: ReturnType<typeof vi.fn>;
  setActiveSession: ReturnType<typeof vi.fn>;
  setWorkflowSnapshot: ReturnType<typeof vi.fn>;
  setTaskSessionsForTask: ReturnType<typeof vi.fn>;
  setTaskSessionsLoading: ReturnType<typeof vi.fn>;
};

export function makeTaskRemovalStore(init: {
  activeTaskId: string | null;
  activeSessionId?: string | null;
  remainingTasks: TaskRow[];
  canonicalTasks?: TaskRow[];
}): StoreApi<FakeState> & { getRecorded: () => FakeState } {
  const state: FakeState = {
    tasks: {
      activeTaskId: init.activeTaskId,
      activeSessionId: init.activeSessionId ?? null,
    },
    kanban: { tasks: init.canonicalTasks ?? [] },
    kanbanMulti: { snapshots: { "wf-1": { tasks: init.remainingTasks } } },
    environmentIdBySessionId: { "sess-next": "env-next", "sess-A": "env-A" },
    taskSessionsByTask: {
      itemsByTaskId: {},
      loadedByTaskId: {},
      loadingByTaskId: {},
    },
    setActiveTask: vi.fn() as ReturnType<typeof vi.fn>,
    setActiveSession: vi.fn() as ReturnType<typeof vi.fn>,
    setWorkflowSnapshot: vi.fn((workflowId: string, snapshot: { tasks: TaskRow[] }) => {
      state.kanbanMulti.snapshots[workflowId] = snapshot;
    }) as unknown as ReturnType<typeof vi.fn>,
    setTaskSessionsForTask: vi.fn() as ReturnType<typeof vi.fn>,
    setTaskSessionsLoading: vi.fn() as ReturnType<typeof vi.fn>,
  };
  const api: StoreApi<FakeState> = {
    getState: () => state,
    setState: (updater: unknown) => {
      const next =
        typeof updater === "function"
          ? (updater as (value: FakeState) => FakeState)(state)
          : (updater as FakeState);
      Object.assign(state, next);
    },
    subscribe: () => () => {},
    getInitialState: () => state,
  } as unknown as StoreApi<FakeState>;
  return Object.assign(api, { getRecorded: () => state }) as StoreApi<FakeState> & {
    getRecorded: () => FakeState;
  };
}
