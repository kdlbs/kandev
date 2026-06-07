import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import type { StoreApi } from "zustand";

const replaceTaskUrlMock = vi.fn();
const performLayoutSwitchMock = vi.fn();
const listTaskSessionsMock = vi.fn();

vi.mock("@/lib/links", () => ({
  replaceTaskUrl: (...args: unknown[]) => replaceTaskUrlMock(...args),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  performLayoutSwitch: (...args: unknown[]) => performLayoutSwitchMock(...args),
}));

vi.mock("@/lib/api", () => ({
  listTaskSessions: (...args: unknown[]) => listTaskSessionsMock(...args),
}));

import { useTaskRemoval } from "./use-task-removal";

type TaskRow = { id: string; primarySessionId: string | null };

type FakeState = {
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

function makeStore(init: {
  activeTaskId: string | null;
  activeSessionId?: string | null;
  remainingTasks: TaskRow[];
}): StoreApi<FakeState> & { getRecorded: () => FakeState } {
  const state: FakeState = {
    tasks: {
      activeTaskId: init.activeTaskId,
      activeSessionId: init.activeSessionId ?? null,
    },
    kanban: { tasks: [] },
    kanbanMulti: {
      snapshots: { "wf-1": { tasks: init.remainingTasks } },
    },
    environmentIdBySessionId: { "sess-next": "env-next", "sess-A": "env-A" },
    taskSessionsByTask: {
      itemsByTaskId: {},
      loadedByTaskId: {},
      loadingByTaskId: {},
    },
    setActiveTask: vi.fn() as ReturnType<typeof vi.fn>,
    setActiveSession: vi.fn() as ReturnType<typeof vi.fn>,
    setWorkflowSnapshot: vi.fn((wfId: string, snapshot: { tasks: TaskRow[] }) => {
      state.kanbanMulti.snapshots[wfId] = snapshot;
    }) as unknown as ReturnType<typeof vi.fn>,
    setTaskSessionsForTask: vi.fn() as ReturnType<typeof vi.fn>,
    setTaskSessionsLoading: vi.fn() as ReturnType<typeof vi.fn>,
  };

  const api: StoreApi<FakeState> = {
    getState: () => state,
    setState: (updater: unknown) => {
      const next =
        typeof updater === "function"
          ? (updater as (s: FakeState) => FakeState)(state)
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

const nextTask: TaskRow = { id: "task-next", primarySessionId: "sess-next" };

beforeEach(() => {
  vi.clearAllMocks();
});

describe("useTaskRemoval — switch guard (current store wins)", () => {
  it("switches to next task when activeTaskId === taskId (user still on removed task)", async () => {
    const store = makeStore({
      activeTaskId: "task-A",
      activeSessionId: "sess-A",
      remainingTasks: [{ id: "task-A", primarySessionId: "sess-A" }, nextTask],
    });
    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    await result.current.removeTaskFromBoard("task-A", {
      wasActiveTaskId: "task-A",
      wasActiveSessionId: "sess-A",
    });

    expect(store.getRecorded().setActiveSession).toHaveBeenCalledWith("task-next", "sess-next");
    expect(replaceTaskUrlMock).toHaveBeenCalledWith("task-next");
  });

  it("does NOT switch when user manually moved to a different task during in-flight archive", async () => {
    const store = makeStore({
      activeTaskId: "task-B",
      activeSessionId: "sess-B",
      remainingTasks: [{ id: "task-B", primarySessionId: "sess-B" }, nextTask],
    });
    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    await result.current.removeTaskFromBoard("task-A", {
      wasActiveTaskId: "task-A",
      wasActiveSessionId: "sess-A",
    });

    expect(store.getRecorded().setActiveSession).not.toHaveBeenCalled();
    expect(store.getRecorded().setActiveTask).not.toHaveBeenCalled();
    expect(replaceTaskUrlMock).not.toHaveBeenCalled();
  });
});

describe("useTaskRemoval — switch guard (WS-clear fallback)", () => {
  it("switches when WS cleared activeTaskId AND wasActiveTaskId matches removed task", async () => {
    const store = makeStore({
      activeTaskId: null,
      activeSessionId: null,
      remainingTasks: [nextTask],
    });
    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    await result.current.removeTaskFromBoard("task-A", {
      wasActiveTaskId: "task-A",
      wasActiveSessionId: "sess-A",
    });

    expect(store.getRecorded().setActiveSession).toHaveBeenCalledWith("task-next", "sess-next");
    expect(replaceTaskUrlMock).toHaveBeenCalledWith("task-next");
  });

  it("does NOT switch when activeTaskId is null AND wasActiveTaskId does not match removed task", async () => {
    const store = makeStore({
      activeTaskId: null,
      activeSessionId: null,
      remainingTasks: [nextTask],
    });
    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    await result.current.removeTaskFromBoard("task-A", {
      wasActiveTaskId: "task-B",
      wasActiveSessionId: "sess-B",
    });

    expect(store.getRecorded().setActiveSession).not.toHaveBeenCalled();
    expect(store.getRecorded().setActiveTask).not.toHaveBeenCalled();
    expect(replaceTaskUrlMock).not.toHaveBeenCalled();
  });

  it("redirects to / when no remaining tasks AND user still on removed task", async () => {
    const store = makeStore({
      activeTaskId: "task-A",
      activeSessionId: "sess-A",
      remainingTasks: [{ id: "task-A", primarySessionId: "sess-A" }],
    });
    const hrefSetter = vi.fn();
    const originalLocation = window.location;
    Object.defineProperty(window, "location", {
      configurable: true,
      value: {
        get href() {
          return "";
        },
        set href(value: string) {
          hrefSetter(value);
        },
      },
    });

    try {
      const { result } = renderHook(() =>
        useTaskRemoval({ store: store as unknown as StoreApi<never> }),
      );
      await result.current.removeTaskFromBoard("task-A", {
        wasActiveTaskId: "task-A",
        wasActiveSessionId: "sess-A",
      });
      expect(hrefSetter).toHaveBeenCalledWith("/");
    } finally {
      Object.defineProperty(window, "location", {
        configurable: true,
        value: originalLocation,
      });
    }
  });
});
