import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { useTaskMenuActions } from "./use-task-menu-actions";

const api = vi.hoisted(() => ({ archiveTask: vi.fn(), deleteTask: vi.fn(), toast: vi.fn() }));
let store: ReturnType<typeof createAppStore>;
vi.mock("@/components/state-provider", () => ({ useAppStoreApi: () => store }));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: api.toast }) }));
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

describe("shared task menu removal", () => {
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
