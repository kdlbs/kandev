import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useStore } from "zustand";
import { createAppStore, type AppState } from "@/lib/state/store";
import { useTaskRemoval } from "@/hooks/use-task-removal";
import { useWorkspaceSidebarTasks } from "./use-workspace-sidebar-tasks";

let store: ReturnType<typeof createAppStore>;
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: AppState) => unknown) => useStore(store, selector),
  useAppStoreApi: () => store,
}));
vi.mock("./use-all-workflow-snapshots", () => ({ useAllWorkflowSnapshots: () => {} }));
vi.mock("@/lib/api", () => ({ fetchTask: vi.fn(), listTaskSessions: vi.fn() }));
vi.mock("@/lib/routing/client-router", () => ({ softNavigate: vi.fn() }));
vi.mock("@/lib/state/dockview-store", () => ({ performLayoutSwitch: vi.fn() }));

function task(id: string, parentTaskId?: string) {
  return {
    id,
    parentTaskId,
    workspaceId: "ws",
    workflowId: "wf",
    workflowStepId: "step",
    title: id,
    position: 0,
  };
}
function setTasks(tasks = [task("a"), task("b"), task("child", "a")]) {
  store.setState((state) => ({
    kanban: { ...state.kanban, workflowId: "wf", tasks },
    kanbanMulti: {
      ...state.kanbanMulti,
      snapshots: { wf: { workflowId: "wf", workflowName: "Workflow", steps: [], tasks } },
    },
  }));
}
function deferred() {
  let resolve!: () => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<void>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}
function setup() {
  return renderHook(() => ({
    sidebar: useWorkspaceSidebarTasks("ws"),
    removal: useTaskRemoval({ store, stayOnListing: true }),
  }));
}

beforeEach(() => {
  store = createAppStore();
  store.setState((state) => ({
    workflows: { ...state.workflows, items: [{ id: "wf", workspaceId: "ws", name: "Workflow" }] },
    workspaces: { ...state.workspaces, activeId: "ws" },
  }));
  setTasks();
});

describe("sidebar pending archive projection", () => {
  // @covers AC-TASKS-REMOVAL-NAVIGATION-003.1, AC-TASKS-REMOVAL-NAVIGATION-003.2, AC-TASKS-REMOVAL-NAVIGATION-003.3
  it("hides before mutation settles and restores latest data after rejection", async () => {
    const view = setup();
    const pending = deferred();
    const sibling = view.result.current.sidebar.allTasks[1];
    let operation!: Promise<unknown>;
    act(() => {
      operation = view.result.current.removal.runTaskRemovalBatch("archive", [
        { taskId: "a", mutate: () => pending.promise },
      ]);
    });
    expect(view.result.current.sidebar.allTasks.map((row) => row.id)).toEqual(["b", "child"]);
    expect(view.result.current.sidebar.allTasks[0]).toBe(sibling);
    expect(store.getState().kanban.tasks).toHaveLength(3);
    act(() => setTasks([{ ...task("a"), title: "New title" }, task("b"), task("child", "a")]));
    expect(view.result.current.sidebar.allTasks.map((row) => row.id)).toEqual(["b", "child"]);
    await act(async () => {
      pending.reject(new Error("refused"));
      await operation;
    });
    expect(view.result.current.sidebar.allTasks.map((row) => row.title)).toEqual([
      "New title",
      "b",
      "child",
    ]);
  });

  it("hides a bulk cascade and only restores failed targets", async () => {
    const view = setup();
    const first = deferred();
    const second = deferred();
    let operation!: Promise<unknown>;
    act(() => {
      operation = view.result.current.removal.runTaskRemovalBatch(
        "archive",
        [
          { taskId: "a", mutate: () => first.promise },
          { taskId: "b", mutate: () => second.promise },
        ],
        { cascade: true },
      );
    });
    expect(view.result.current.sidebar.allTasks).toEqual([]);
    await act(async () => {
      first.resolve();
      second.reject(new Error("refused"));
      await operation;
    });
    expect(view.result.current.sidebar.allTasks.map((row) => row.id)).toEqual(["b"]);
  });
});

describe("sidebar archive compatibility", () => {
  it("keeps confirmed archived rows visible and respects later navigation", () => {
    const view = setup();
    act(() => {
      store.getState().beginTaskRemoval({
        action: "archive",
        workspaceId: "ws",
        taskIds: ["a"],
        requestIds: ["a"],
        departure: null,
      });
      store.getState().setActiveTask("b");
    });
    expect(view.result.current.sidebar.allTasks.map((row) => row.id)).toEqual(["b", "child"]);
    act(() => {
      const state = store.getState();
      const tasks = state.kanban.tasks.map((row) =>
        row.id === "a" ? { ...row, isArchived: true } : row,
      );
      store.setState({
        kanban: { ...state.kanban, tasks },
        kanbanMulti: {
          ...state.kanbanMulti,
          snapshots: { wf: { ...state.kanbanMulti.snapshots.wf, tasks } },
        },
      });
    });
    expect(view.result.current.sidebar.allTasks.find((row) => row.id === "a")?.isArchived).toBe(
      true,
    );
    expect(store.getState().tasks.activeTaskId).toBe("b");
  });

  it("keeps the last row absent when success releases pending intent", async () => {
    setTasks([task("a")]);
    const view = setup();
    const pending = deferred();
    let operation!: Promise<unknown>;
    act(() => {
      operation = view.result.current.removal.runTaskRemovalBatch("archive", [
        { taskId: "a", mutate: () => pending.promise },
      ]);
    });
    expect(view.result.current.sidebar.allTasks).toEqual([]);
    await act(async () => {
      pending.resolve();
      await operation;
    });
    expect(view.result.current.sidebar.allTasks).toEqual([]);
    expect(store.getState().taskRemoval.operationsByToken).toEqual({});
  });

  it("does not hide delete targets or another workspace's operation", () => {
    const view = setup();
    act(() => {
      store.getState().beginTaskRemoval({
        action: "delete",
        workspaceId: "ws",
        taskIds: ["a"],
        requestIds: ["a"],
        departure: null,
      });
      store.getState().beginTaskRemoval({
        action: "archive",
        workspaceId: "other",
        taskIds: ["b"],
        requestIds: ["b"],
        departure: null,
      });
    });
    expect(view.result.current.sidebar.allTasks.map((row) => row.id)).toEqual(["a", "b", "child"]);
  });
});
