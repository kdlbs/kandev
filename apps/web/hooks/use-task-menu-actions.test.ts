import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { useTaskMenuActions } from "./use-task-menu-actions";

const api = vi.hoisted(() => ({
  archiveTask: vi.fn(),
  deleteTask: vi.fn(),
  toast: vi.fn(),
  softNavigate: vi.fn(),
}));
let store: ReturnType<typeof createAppStore>;
vi.mock("@/components/state-provider", () => ({ useAppStoreApi: () => store }));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: api.toast }) }));
vi.mock("@/lib/routing/client-router", () => ({ softNavigate: api.softNavigate }));
vi.mock("@/lib/api", () => ({
  archiveTask: api.archiveTask,
  deleteTask: api.deleteTask,
  moveTask: vi.fn(),
  updateTask: vi.fn(),
  fetchTask: vi.fn(),
  listTaskSessions: vi.fn(),
}));

const tasks = ["A", "B"].map((id) => ({
  id,
  title: id,
  workspaceId: "workspace",
  workflowId: "workflow",
  workflowStepId: "step",
  position: 0,
}));
beforeEach(() => {
  vi.clearAllMocks();
  store = createAppStore();
  store.setState((state) => ({
    kanban: { ...state.kanban, workflowId: "workflow", tasks },
    kanbanMulti: {
      ...state.kanbanMulti,
      snapshots: {
        workflow: { ...state.kanban, workflowId: "workflow", tasks },
      },
    },
  }));
});

describe("concurrent shared task menu removal", () => {
  it.each(["runArchive", "runDelete"] as const)(
    "%s accepts another task while retaining the first task's duplicate guard",
    async (method) => {
      let releaseA!: () => void;
      let releaseB!: () => void;
      const requestA = new Promise<void>((resolve) => {
        releaseA = resolve;
      });
      const requestB = new Promise<void>((resolve) => {
        releaseB = resolve;
      });
      const request = method === "runArchive" ? api.archiveTask : api.deleteTask;
      request.mockImplementation((id: string) => (id === "A" ? requestA : requestB));
      const { result } = renderHook(() => useTaskMenuActions({ stayOnListing: true }));
      let outcomeA!: Promise<boolean>;
      let outcomeB!: Promise<boolean>;
      await act(async () => {
        outcomeA = result.current[method]("A");
        outcomeB = result.current[method]("B");
      });
      expect(request.mock.calls.map(([id]) => id)).toEqual(["A", "B"]);
      expect(result.current.pendingTaskId).toBe("A");
      await act(async () => {
        expect(await result.current[method]("A")).toBe(false);
        expect(await result.current[method]("B")).toBe(false);
        releaseB();
        expect(await outcomeB).toBe(true);
      });
      expect(result.current.pendingTaskId).toBe("A");
      expect(store.getState().kanban.tasks.map(({ id }) => id)).toEqual(["A"]);
      await act(async () => {
        expect(await result.current[method]("A")).toBe(false);
        releaseA();
        expect(await outcomeA).toBe(true);
      });
      expect(request).toHaveBeenCalledTimes(2);
      expect(result.current.pendingTaskId).toBeNull();
      expect(store.getState().kanban.tasks).toEqual([]);
    },
  );

  it.each(["runArchive", "runDelete"] as const)(
    "%s failing A keeps B pending and permits retrying only A",
    async (method) => {
      let rejectA!: (error: Error) => void;
      let releaseB!: () => void;
      const requestA = new Promise<void>((_, reject) => {
        rejectA = reject;
      });
      const requestB = new Promise<void>((resolve) => {
        releaseB = resolve;
      });
      const request = method === "runArchive" ? api.archiveTask : api.deleteTask;
      request.mockImplementation((id: string) => (id === "A" ? requestA : requestB));
      const { result } = renderHook(() => useTaskMenuActions({ stayOnListing: true }));
      let outcomeA!: Promise<boolean>;
      let outcomeB!: Promise<boolean>;
      await act(async () => {
        outcomeA = result.current[method]("A");
        outcomeB = result.current[method]("B");
      });
      expect(request.mock.calls.map(([id]) => id)).toEqual(["A", "B"]);
      await act(async () => {
        rejectA(new Error("offline"));
        expect(await outcomeA).toBe(false);
      });
      expect(result.current.pendingTaskId).toBe("B");
      expect(api.toast).toHaveBeenCalledTimes(1);
      request.mockResolvedValueOnce(undefined);
      await act(async () => {
        expect(await result.current[method]("B")).toBe(false);
        expect(await result.current[method]("A")).toBe(true);
      });
      expect(result.current.pendingTaskId).toBe("B");
      await act(async () => {
        releaseB();
        expect(await outcomeB).toBe(true);
      });
      expect(request).toHaveBeenCalledTimes(3);
      expect(result.current.pendingTaskId).toBeNull();
    },
  );
});

describe("shared task menu removal", () => {
  // @covers AC-TASKS-THREADS-ACTIONS-002.4, AC-TASKS-REMOVAL-NAVIGATION-002.3
  it.each(["runArchive", "runDelete"] as const)(
    "%s coordinates a listing removal without departing the remembered task",
    async (method) => {
      store.getState().setActiveSession("A", "session-A");
      const selection = store.getState().tasks;
      let release!: () => void;
      const request = method === "runArchive" ? api.archiveTask : api.deleteTask;
      request.mockReturnValueOnce(new Promise<void>((resolve) => (release = resolve)));
      const { result } = renderHook(() => useTaskMenuActions({ stayOnListing: true }));
      let outcome!: Promise<boolean>;
      act(() => {
        outcome = result.current[method]("A");
      });
      try {
        const operations = Object.values(store.getState().taskRemoval.operationsByToken);
        expect(operations).toHaveLength(1);
        expect(operations[0]).toMatchObject({ taskIds: ["A"], departure: null });
        expect(store.getState().kanban.tasks).toEqual(tasks);
        expect(api.toast).not.toHaveBeenCalled();
      } finally {
        await act(async () => {
          release();
          expect(await outcome).toBe(true);
        });
      }
      expect(store.getState().tasks).toEqual(selection);
      expect(store.getState().kanban.tasks.map(({ id }) => id)).toEqual(["B"]);
      expect(store.getState().taskRemoval.operationsByToken).toEqual({});
      expect(api.softNavigate).not.toHaveBeenCalled();
      expect(api.toast).toHaveBeenCalledOnce();
      expect(api.toast).toHaveBeenCalledWith(expect.objectContaining({ variant: "success" }));
    },
  );

  // @covers AC-TASKS-THREADS-ACTIONS-002.5, AC-TASKS-THREADS-ACTIONS-003.4
  it.each(["runArchive", "runDelete"] as const)(
    "%s failure on a listing never navigates back to the remembered task",
    async (method) => {
      store.getState().setActiveSession("A", "session-A");
      const selection = store.getState().tasks;
      const request = method === "runArchive" ? api.archiveTask : api.deleteTask;
      request.mockRejectedValueOnce(new Error("offline"));
      const { result } = renderHook(() => useTaskMenuActions({ stayOnListing: true }));
      await act(async () => {
        expect(await result.current[method]("A")).toBe(false);
      });
      expect(store.getState().tasks).toEqual(selection);
      expect(store.getState().kanban.tasks).toEqual(tasks);
      expect(store.getState().taskRemoval.operationsByToken).toEqual({});
      expect(api.softNavigate).not.toHaveBeenCalled();
      expect(api.toast).toHaveBeenCalledOnce();
      expect(api.toast).toHaveBeenCalledWith(expect.objectContaining({ variant: "error" }));
    },
  );

  // @covers AC-TASKS-THREADS-ACTIONS-002.2, AC-TASKS-THREADS-ACTIONS-002.4, AC-TASKS-THREADS-ACTIONS-002.6
  it.each(["runArchive", "runDelete"] as const)(
    "%s retains A while selection changes to B and rejects duplicate submits",
    async (method) => {
      let release!: () => void;
      const pending = new Promise<void>((resolve) => {
        release = resolve;
      });
      const request = method === "runArchive" ? api.archiveTask : api.deleteTask;
      request.mockReturnValueOnce(pending);
      const { result } = renderHook(() => useTaskMenuActions({ stayOnListing: true }));
      let outcome!: Promise<boolean>;
      act(() => {
        outcome = result.current[method]("A", { cascade: true });
      });
      expect(result.current.pendingTaskId).toBe("A");
      store.getState().setActiveTask("B");
      await act(async () => {
        expect(await result.current[method]("A")).toBe(false);
        release();
        expect(await outcome).toBe(true);
      });
      expect(request).toHaveBeenCalledTimes(1);
      expect(request).toHaveBeenCalledWith("A", { cascade: true });
      expect(store.getState().kanban.tasks.map((task) => task.id)).toEqual(["B"]);
      expect(store.getState().tasks.activeTaskId).toBe("B");
      expect(result.current.pendingTaskId).toBeNull();
    },
  );

  // @covers AC-TASKS-THREADS-ACTIONS-002.5
  it.each(["runArchive", "runDelete"] as const)(
    "%s failure retains tasks, reports once and permits retry",
    async (method) => {
      const request = method === "runArchive" ? api.archiveTask : api.deleteTask;
      request.mockRejectedValueOnce(new Error("offline"));
      const { result } = renderHook(() => useTaskMenuActions({ stayOnListing: true }));
      await act(async () => {
        expect(await result.current[method]("A")).toBe(false);
      });
      expect(store.getState().kanban.tasks).toEqual(tasks);
      expect(api.toast).toHaveBeenCalledTimes(1);
      expect(result.current.pendingTaskId).toBeNull();
      request.mockResolvedValueOnce(undefined);
      await act(async () => {
        expect(await result.current[method]("A")).toBe(true);
      });
    },
  );
});
