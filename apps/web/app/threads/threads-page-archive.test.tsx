import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useEffect } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { createAppStore, type HydrationState } from "@/lib/state/store";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import { sessionId as toSessionId, taskId as toTaskId, type TaskSession } from "@/lib/types/http";
import { useTaskMenuActions } from "@/hooks/use-task-menu-actions";
import { registerTasksHandlers } from "@/lib/ws/handlers/tasks";
import { DEFAULT_THREAD_VIEW } from "@/lib/state/slices/ui/thread-view-builtins";
import { ThreadsPageClient } from "./threads-page-client";

const transport = vi.hoisted(() => ({ archive: vi.fn(), remove: vi.fn(), toast: vi.fn() }));
const route = vi.hoisted(() => ({ search: "", push: vi.fn(), replace: vi.fn() }));
const setView = vi.hoisted(() => vi.fn());
const activeChats = new Set<string>();

vi.mock("@/lib/api", async (original) => ({
  ...(await original<typeof import("@/lib/api")>()),
  archiveTask: transport.archive,
  deleteTask: transport.remove,
}));
vi.mock("@/lib/routing/client-router", async (original) => ({
  ...(await original<typeof import("@/lib/routing/client-router")>()),
  useRouter: () => route,
  useSearchParams: () => new URLSearchParams(route.search),
}));
vi.mock("@/src/kanban-route", () => ({ useKanbanRouteBootstrap: () => {} }));
vi.mock("@/hooks/domains/kanban/use-all-workflow-snapshots", () => ({
  useAllWorkflowSnapshots: () => {},
}));
vi.mock("@/hooks/use-task-listing-view", () => ({
  useTaskListingView: () => ({ setView }),
}));
vi.mock("@/hooks/use-kanban-display-settings", () => ({
  useKanbanDisplaySettings: () => ({
    activeWorkspaceId: useAppStore((state) => state.workspaces.activeId),
    workspaces: useAppStore((state) => state.workspaces.items),
    workflows: useAppStore((state) => state.workflows.items),
    repositories: [],
  }),
}));
vi.mock("@/components/kanban/kanban-header", () => ({ KanbanHeader: () => null }));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: transport.toast }) }));
vi.mock("@/components/threads/thread-conversation", () => ({
  ThreadConversation: ({ taskId }: { taskId: string }) => {
    useEffect(() => {
      activeChats.add(taskId);
      return () => {
        activeChats.delete(taskId);
      };
    }, [taskId]);
    return null;
  },
}));

let store: ReturnType<typeof useAppStoreApi>;
let actions: ReturnType<typeof useTaskMenuActions>;

function Harness() {
  store = useAppStoreApi();
  actions = useTaskMenuActions({ stayOnListing: true });
  return <ThreadsPageClient />;
}

function task(id: string, parentTaskId?: string): KanbanState["tasks"][number] {
  return {
    id,
    title: `Task ${id}`,
    workspaceId: "workspace",
    workflowId: "workflow",
    workflowStepId: "step",
    position: 0,
    state: "IN_PROGRESS",
    parentTaskId,
    primarySessionId: `session-${id}`,
    primarySessionState: "RUNNING",
  };
}

function renderThreads(tasks = [task("a"), task("b"), task("c")]) {
  const base = createAppStore().getState();
  const sessions = tasks.map(
    ({ id }): TaskSession => ({
      id: toSessionId(`session-${id}`),
      task_id: toTaskId(id),
      state: "RUNNING",
      is_primary: true,
      started_at: "2026-09-12T08:00:00Z",
      updated_at: "2026-09-12T08:00:00Z",
    }),
  );
  const initialState: HydrationState = {
    workspaces: { ...base.workspaces, activeId: "workspace" },
    workflows: {
      activeId: null,
      items: [{ id: "workflow", workspaceId: "workspace", name: "Delivery" }],
    },
    kanban: { ...base.kanban, workflowId: "workflow", tasks },
    kanbanMulti: {
      ...base.kanbanMulti,
      snapshots: {
        workflow: { workflowId: "workflow", workflowName: "Delivery", steps: [], tasks },
      },
    },
    taskSessions: {
      ...base.taskSessions,
      items: Object.fromEntries(sessions.map((s) => [s.id, s])),
    },
    taskSessionsByTask: {
      ...base.taskSessionsByTask,
      itemsByTaskId: Object.fromEntries(sessions.map((s) => [s.task_id, [s]])),
      loadedByTaskId: Object.fromEntries(tasks.map(({ id }) => [id, true])),
    },
  };
  return render(
    <StateProvider initialState={initialState}>
      <Harness />
    </StateProvider>,
  );
}

function deferred() {
  let resolve!: () => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<void>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

function visibleIds() {
  return screen.queryAllByTestId(/^thread-column-/).map((column) => column.dataset.threadColumnId);
}

function publishArchive(id: string) {
  registerTasksHandlers(store)["task.updated"]!({
    id: `archived-${id}`,
    type: "notification",
    action: "task.updated",
    payload: {
      task_id: id,
      workspace_id: "workspace",
      workflow_id: "workflow",
      workflow_step_id: "step",
      title: `Task ${id}`,
      state: "IN_PROGRESS",
      is_ephemeral: false,
      archived_at: "2026-09-12T09:00:00Z",
    },
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  route.search = "";
  window.history.replaceState({}, "", "/threads");
});
afterEach(() => {
  cleanup();
  activeChats.clear();
});

// @covers AC-TASKS-THREADS-ACTIONS-003.7, AC-TASKS-THREADS-ACTIONS-003.8
describe("Threads pending archive lifecycle", () => {
  it("removes an archived column before the request settles", async () => {
    const pending = deferred();
    transport.archive.mockReturnValueOnce(pending.promise);
    renderThreads();
    expect(activeChats.has("a")).toBe(true);
    let result!: Promise<boolean>;
    act(() => {
      result = actions.runArchive("a");
    });
    try {
      expect(visibleIds()).toEqual(["b", "c"]);
      expect(activeChats.has("a")).toBe(false);
      expect(store.getState().kanbanMulti.snapshots.workflow.tasks.map(({ id }) => id)).toEqual([
        "a",
        "b",
        "c",
      ]);
      expect(transport.toast).not.toHaveBeenCalled();
    } finally {
      await act(async () => {
        pending.resolve();
        await result;
      });
    }
    expect(visibleIds()).toEqual(["b", "c"]);
    expect(transport.toast).toHaveBeenCalledWith(expect.objectContaining({ variant: "success" }));
  });

  // @covers AC-TASKS-THREADS-ACTIONS-003.4, AC-TASKS-THREADS-ACTIONS-004.6
  it("readmits a rejected archive without reviving its consumed deep link", async () => {
    route.search = "taskId=a&sessionId=session-a";
    const pending = deferred();
    transport.archive.mockReturnValueOnce(pending.promise);
    renderThreads();
    fireEvent.pointerDown(screen.getByTestId("thread-column-a"));
    let result!: Promise<boolean>;
    act(() => {
      result = actions.runArchive("a");
    });
    try {
      expect(visibleIds()).toEqual(["b", "c"]);
      fireEvent.pointerDown(screen.getByTestId("thread-column-c"));
    } finally {
      await act(async () => {
        pending.reject(new Error("archive rejected"));
        await result;
      });
    }
    expect(visibleIds()).toEqual(["b", "c", "a"]);
    expect(screen.getByTestId("thread-column-a").getAttribute("data-focused")).toBeNull();
    expect(route.replace).not.toHaveBeenCalled();
    expect(route.push).not.toHaveBeenCalled();
    expect(transport.toast).toHaveBeenCalledWith(expect.objectContaining({ variant: "error" }));
  });

  it.each(["event-first", "response-first"])(
    "does not reopen successful archives with %s delivery",
    async (order) => {
      const pending = deferred();
      transport.archive.mockReturnValueOnce(pending.promise);
      renderThreads([task("a")]);
      let result!: Promise<boolean>;
      act(() => {
        result = actions.runArchive("a");
      });
      try {
        expect(screen.queryByTestId("threads-empty-state")).not.toBeNull();
        if (order === "event-first") act(() => publishArchive("a"));
      } finally {
        await act(async () => {
          pending.resolve();
          await result;
        });
      }
      if (order === "response-first") act(() => publishArchive("a"));
      expect(visibleIds()).toEqual([]);
      expect(activeChats.has("a")).toBe(false);
      expect(screen.queryByTestId("threads-empty-state")).not.toBeNull();
      expect(store.getState().taskRemoval.operationsByToken).toEqual({});
    },
  );
});

describe("Threads independent removal intent", () => {
  it("releases only the failed archive when two different tasks are pending", async () => {
    const first = deferred();
    const second = deferred();
    transport.archive.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    renderThreads();
    let firstResult!: Promise<boolean>;
    let secondResult!: Promise<boolean>;
    act(() => {
      firstResult = actions.runArchive("a");
      secondResult = actions.runArchive("b");
    });
    try {
      expect(visibleIds()).toEqual(["c"]);
      await act(async () => {
        first.reject(new Error("archive rejected"));
        await firstResult;
      });
      expect(visibleIds()).toEqual(["c", "a"]);
      expect(Object.values(store.getState().taskRemoval.operationsByToken)).toHaveLength(1);
    } finally {
      await act(async () => {
        first.resolve();
        second.resolve();
        await Promise.all([firstResult, secondResult]);
      });
    }
    expect(visibleIds()).toEqual(["c", "a"]);
  });

  it("excludes captured descendants while leaving a pending delete visible", async () => {
    const archive = deferred();
    const remove = deferred();
    transport.archive.mockReturnValueOnce(archive.promise);
    transport.remove.mockReturnValueOnce(remove.promise);
    route.search = "taskId=child";
    renderThreads([task("parent"), task("child", "parent"), task("b")]);
    let archiveResult!: Promise<boolean>;
    let deleteResult!: Promise<boolean>;
    act(() => {
      archiveResult = actions.runArchive("parent", { cascade: true });
      deleteResult = actions.runDelete("b");
    });
    try {
      expect(visibleIds()).toEqual(["b"]);
      expect(transport.archive).toHaveBeenCalledWith("parent", { cascade: true });
    } finally {
      await act(async () => {
        archive.resolve();
        remove.resolve();
        await Promise.all([archiveResult, deleteResult]);
      });
    }
    expect(visibleIds()).toEqual([]);
  });
});

describe("Threads archive rejection reconciliation", () => {
  it.each(["workspace", "view"])("does not undo a newer %s choice on rejection", async (choice) => {
    const pending = deferred();
    transport.archive.mockReturnValueOnce(pending.promise);
    renderThreads();
    let result!: Promise<boolean>;
    act(() => {
      result = actions.runArchive("a");
    });
    try {
      act(() =>
        store.setState((state) =>
          choice === "workspace"
            ? {
                workspaces: { ...state.workspaces, activeId: "another-workspace" },
              }
            : {
                threadViews: {
                  ...state.threadViews,
                  draft: {
                    ...DEFAULT_THREAD_VIEW,
                    baseViewId: DEFAULT_THREAD_VIEW.id,
                    taskScope: { mode: "selected", taskIds: ["b"] },
                  },
                },
              },
        ),
      );
    } finally {
      await act(async () => {
        pending.reject(new Error("archive rejected"));
        await result;
      });
    }
    expect(visibleIds()).toEqual(choice === "workspace" ? [] : ["b"]);
    expect(route.push).not.toHaveBeenCalled();
    expect(route.replace).not.toHaveBeenCalled();
  });

  it.each(["archive", "delete"])(
    "keeps an authoritative %s removed after the request rejects",
    async (event) => {
      const pending = deferred();
      transport.archive.mockReturnValueOnce(pending.promise);
      renderThreads([task("a")]);
      let result!: Promise<boolean>;
      act(() => {
        result = actions.runArchive("a");
      });
      try {
        act(() => {
          if (event === "archive") publishArchive("a");
          else
            registerTasksHandlers(store)["task.deleted"]!({
              id: "deleted-a",
              type: "notification",
              action: "task.deleted",
              payload: {
                task_id: "a",
                workspace_id: "workspace",
                workflow_id: "workflow",
                workflow_step_id: "step",
                title: "Task a",
                is_ephemeral: false,
              },
            });
        });
      } finally {
        await act(async () => {
          pending.reject(new Error("response lost"));
          await result;
        });
      }
      expect(visibleIds()).toEqual([]);
      expect(screen.queryByTestId("threads-empty-state")).not.toBeNull();
    },
  );
});
