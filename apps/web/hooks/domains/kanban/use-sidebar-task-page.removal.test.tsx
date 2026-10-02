import type { ReactNode } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { querySidebarTasks } from "@/lib/api/domains/kanban-api";
import { coordinateTaskRemovalBatch } from "@/hooks/task-removal-coordinator";
import type { SidebarTaskPageResponse, Task } from "@/lib/types/http";
import { taskId, workflowId, workspaceId } from "@/lib/types/ids";
import { registerTasksHandlers } from "@/lib/ws/handlers/tasks";
import { makeDeletedMessage } from "@/lib/ws/handlers/tasks.test-helpers";
import { useWorkspaceSidebarTasks } from "./use-workspace-sidebar-tasks";

vi.mock("@/lib/api/domains/kanban-api", () => ({ querySidebarTasks: vi.fn() }));
vi.mock("@/hooks/domains/sidebar/use-effective-sidebar-view", () => ({
  useEffectiveSidebarView: () => ({
    id: "view",
    filters: [],
    sort: { key: "updatedAt", direction: "desc" },
    group: "none",
    collapsedGroups: [],
  }),
}));
vi.mock("@/hooks/domains/sidebar/use-sidebar-task-prefs", () => ({
  useSidebarTaskPrefs: () => ({
    pinnedTaskIds: [],
    orderedTaskIds: [],
    subtaskOrderByParentId: {},
  }),
}));
vi.mock("@/hooks/use-foreground-refresh", () => ({ useForegroundRefresh: vi.fn() }));

afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

function Wrapper({ children }: { children: ReactNode }) {
  return (
    <StateProvider initialState={{ workspaces: { items: [], activeId: "ws" } }}>
      {children}
    </StateProvider>
  );
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}
function response(ids: string[]): SidebarTaskPageResponse {
  return {
    query_key: "query",
    page: 1,
    page_size: 100,
    total_entries: ids.length,
    total_tasks: ids.length,
    total_visible_tasks: ids.length,
    has_previous: false,
    has_next: false,
    entries: ids.map((id) => ({
      kind: "task",
      task: {
        id: taskId(id),
        workspace_id: workspaceId("ws"),
        workflow_id: workflowId("wf"),
        workflow_step_id: "step",
        title: id,
        description: "",
        state: "TODO",
        priority: "medium",
        position: 0,
        created_at: "2026-09-30T00:00:00Z",
        updated_at: "2026-09-30T00:00:00Z",
      } as Task,
    })),
  };
}

describe("confirmed sidebar deletion", () => {
  // @covers AC-TASKS-REMOVAL-NAVIGATION-005.2, AC-TASKS-REMOVAL-NAVIGATION-005.3
  it("removes successful cascade targets before token release and rejects an older refresh", async () => {
    const staleRefresh = deferred<SidebarTaskPageResponse>();
    const freshRefresh = deferred<SidebarTaskPageResponse>();
    vi.mocked(querySidebarTasks)
      .mockResolvedValueOnce(response(["parent", "child", "failed", "keep"]))
      .mockReturnValueOnce(staleRefresh.promise)
      .mockReturnValue(freshRefresh.promise);
    const { result } = renderHook(
      () => ({
        store: useAppStoreApi(),
        sidebar: useWorkspaceSidebarTasks("ws"),
      }),
      { wrapper: Wrapper },
    );
    await waitFor(() => expect(result.current.sidebar.allTasks).toHaveLength(4));
    act(() => result.current.sidebar.page.refresh());
    await waitFor(() => expect(querySidebarTasks).toHaveBeenCalledTimes(2));
    const mutation = deferred<void>();
    let removal!: ReturnType<typeof coordinateTaskRemovalBatch>;
    act(() => {
      removal = coordinateTaskRemovalBatch(
        {
          store: result.current.store,
          removeTaskFromBoard: async () => ({ switchedTaskId: null }),
          getRemovalIds: () =>
            new Map([
              ["parent", new Set(["parent", "child"])],
              ["failed", new Set(["failed"])],
            ]),
        },
        "delete",
        [
          { taskId: "parent", mutate: () => mutation.promise },
          {
            taskId: "failed",
            mutate: async () => {
              throw new Error("refused");
            },
          },
        ],
        { workspaceId: "ws", cascade: true },
      );
    });
    expect(result.current.sidebar.pendingRemovalTaskIds).toEqual(
      new Set(["parent", "child", "failed"]),
    );
    await act(async () => {
      mutation.resolve();
      await removal;
    });
    expect(result.current.sidebar.allTasks.map((task) => task.id)).toEqual(["failed", "keep"]);
    expect(result.current.sidebar.pendingRemovalTaskIds.size).toBe(0);
    await act(async () => {
      staleRefresh.resolve(response(["parent", "child", "failed", "keep"]));
    });
    expect(result.current.sidebar.allTasks.map((task) => task.id)).toEqual(["failed", "keep"]);
    await waitFor(() => expect(querySidebarTasks).toHaveBeenCalledTimes(3));
    await act(async () => {
      freshRefresh.resolve(response(["failed", "keep"]));
    });
    expect(result.current.sidebar.allTasks.map((task) => task.id)).toEqual(["failed", "keep"]);
  });
  it("removes consecutive deletions from both mounted pickers before a replacement arrives", async () => {
    const refresh = deferred<SidebarTaskPageResponse>();
    vi.mocked(querySidebarTasks)
      .mockResolvedValueOnce(response(["target", "second", "keep"]))
      .mockReturnValue(refresh.promise);
    const { result } = renderHook(
      () => ({
        store: useAppStoreApi(),
        desktop: useWorkspaceSidebarTasks("ws"),
        phone: useWorkspaceSidebarTasks("ws"),
      }),
      { wrapper: Wrapper },
    );
    await waitFor(() => expect(result.current.phone.allTasks).toHaveLength(3));
    act(() => {
      for (const id of ["target", "second"]) {
        registerTasksHandlers(result.current.store)["task.deleted"]!(
          makeDeletedMessage({ task_id: id, workspace_id: "ws", workflow_id: "wf" }),
        );
      }
    });
    expect(result.current.desktop.allTasks.map((task) => task.id)).toEqual(["keep"]);
    expect(result.current.phone.allTasks.map((task) => task.id)).toEqual(["keep"]);
    await act(async () => {
      refresh.resolve(response(["keep"]));
    });
  });
});
