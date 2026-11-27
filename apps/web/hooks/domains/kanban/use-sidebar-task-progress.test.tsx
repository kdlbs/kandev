import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useStore } from "zustand";
import { createAppStore, type AppState } from "@/lib/state/store";
import { querySidebarTasks } from "@/lib/api/domains/kanban-api";
import { registerTasksHandlers } from "@/lib/ws/handlers/tasks";
import type { SidebarTaskPageResponse } from "@/lib/types/http";
import { sidebarTaskPageCache } from "@/lib/sidebar/sidebar-task-page-cache";
import { useSidebarTaskPage } from "./use-sidebar-task-page";

let store: ReturnType<typeof createAppStore>;
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: AppState) => unknown) => useStore(store, selector),
  useAppStoreApi: () => store,
}));
vi.mock("@/lib/api/domains/kanban-api", () => ({ querySidebarTasks: vi.fn() }));
vi.mock("@/hooks/use-foreground-refresh", () => ({ useForegroundRefresh: vi.fn() }));
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}
function response(): SidebarTaskPageResponse {
  return {
    query_key: "progress",
    page: 1,
    page_size: 100,
    total_entries: 4,
    total_tasks: 3,
    total_visible_tasks: 3,
    has_previous: false,
    has_next: false,
    entries: ["kept", "deleted", "archived"].map((id) => ({
      kind: "task",
      task_id: id,
      task: {
        id,
        workspace_id: "ws",
        workflow_id: "wf",
        workflow_step_id: "step",
        title: id,
        updated_at: "2026-09-29T00:00:00Z",
      },
    })),
  } as SidebarTaskPageResponse;
}
beforeEach(() => {
  vi.resetAllMocks();
  store = createAppStore();
  store.setState((state) => {
    state.workspaces.activeId = "ws";
    state.workspaceContextRead.workspaceId = "ws";
    state.workspaceContextRead.generation = state.workspaceContextGeneration;
    state.workflows.items = [{ id: "wf", workspaceId: "ws", name: "Workflow" }];
    state.kanbanMulti.snapshots.wf = {
      workflowId: "wf",
      workflowName: "Workflow",
      steps: [{ id: "step", title: "Step", color: "", position: 0 }],
      tasks: [],
    };
  });
});
afterEach(cleanup);

it("publishes safe first rows after three invalidations with at most one trailing read", async () => {
  const initial = deferred<SidebarTaskPageResponse>(),
    trailing = deferred<SidebarTaskPageResponse>();
  vi.mocked(querySidebarTasks)
    .mockReturnValueOnce(initial.promise)
    .mockReturnValueOnce(trailing.promise);
  const hook = renderHook(() => useSidebarTaskPage("ws"));
  await waitFor(() => expect(querySidebarTasks).toHaveBeenCalledTimes(1));
  const handlers = registerTasksHandlers(store);
  act(() => {
    for (const [id, patch] of [
      ["kept", { title: "Live title" }],
      ["deleted", null],
      ["archived", { archived_at: "2026-09-29T01:00:00Z" }],
    ] as const) {
      const payload = {
        task_id: id,
        workspace_id: "ws",
        workflow_id: "wf",
        workflow_step_id: "step",
        ...patch,
      };
      if (patch) {
        handlers["task.updated"]?.({
          type: "notification",
          action: "task.updated",
          payload,
        } as Parameters<NonNullable<(typeof handlers)["task.updated"]>>[0]);
      } else {
        handlers["task.deleted"]?.({
          type: "notification",
          action: "task.deleted",
          payload,
        } as Parameters<NonNullable<(typeof handlers)["task.deleted"]>>[0]);
      }
    }
  });
  expect(querySidebarTasks).toHaveBeenCalledTimes(1);
  await act(async () => initial.resolve(response()));
  expect(
    hook.result.current.response?.entries.flatMap((row) => (row.task_id ? [row.task_id] : [])),
  ).toEqual(["kept"]);
  expect(store.getState().taskOverview.byId.kept.title).toBe("Live title");
  expect(hook.result.current.isLoading).toBe(false);
  expect(sidebarTaskPageCache(store).get(hook.result.current.scopeKey)).toBeNull();
  await waitFor(() => expect(querySidebarTasks).toHaveBeenCalledTimes(2));
  await act(async () =>
    trailing.resolve({
      ...response(),
      entries: [
        { ...response().entries[0], task: { ...response().entries[0].task!, title: "Live title" } },
      ],
    }),
  );
  await waitFor(() => expect(hook.result.current.isRefreshing).toBe(false));
  expect(querySidebarTasks).toHaveBeenCalledTimes(2);
});

it.each(["workspace", "logout", "reconnect", "access"] as const)(
  "rejects a late first response across %s",
  async (barrier) => {
    const initial = deferred<SidebarTaskPageResponse>();
    vi.mocked(querySidebarTasks)
      .mockReturnValue(new Promise(() => {}))
      .mockReturnValueOnce(initial.promise);
    const hook = renderHook(() => useSidebarTaskPage("ws"));
    await waitFor(() => expect(querySidebarTasks).toHaveBeenCalledTimes(1));
    act(() => {
      if (barrier === "workspace") store.getState().setActiveWorkspace("other");
      if (barrier === "logout") store.getState().clearAuthenticated();
      if (barrier === "access") store.getState().denyTaskOverviewAccess();
      if (barrier === "reconnect") {
        store.setState((state) => {
          state.connection.status = "connected";
        });
        store.setState((state) => {
          state.connection.status = "disconnected";
        });
      }
    });
    await act(async () => initial.resolve(response()));
    expect(hook.result.current.response).toBeNull();
    expect(store.getState().taskOverview.byId.kept).toBeUndefined();
    if (barrier === "access") {
      expect(hook.result.current.hasError).toBe(true);
      expect(hook.result.current.canRetry).toBe(false);
    }
  },
);

it("shows recovery when live removals empty a replacement instead of retaining an authoritative old page", async () => {
  const replacement = deferred<SidebarTaskPageResponse>(),
    trailing = deferred<SidebarTaskPageResponse>();
  vi.mocked(querySidebarTasks)
    .mockResolvedValueOnce(response())
    .mockReturnValueOnce(replacement.promise)
    .mockReturnValueOnce(trailing.promise);
  const hook = renderHook(() => useSidebarTaskPage("ws"));
  await waitFor(() => expect(hook.result.current.response?.entries).toHaveLength(3));
  act(() => hook.result.current.refresh());
  await waitFor(() => expect(querySidebarTasks).toHaveBeenCalledTimes(2));
  const handlers = registerTasksHandlers(store);
  act(() => {
    for (const id of ["kept", "deleted", "archived"])
      handlers["task.deleted"]?.({
        type: "notification",
        action: "task.deleted",
        payload: { task_id: id, workspace_id: "ws", workflow_id: "wf" },
      } as Parameters<NonNullable<(typeof handlers)["task.deleted"]>>[0]);
  });
  await act(async () => replacement.resolve(response()));
  expect(hook.result.current.response?.entries).toEqual([]);
  expect(hook.result.current.response?.provisional).toBe(true);
  expect(hook.result.current.isLoading).toBe(true);
  await waitFor(() => expect(querySidebarTasks).toHaveBeenCalledTimes(3));
  await act(async () =>
    trailing.resolve({
      ...response(),
      entries: [],
      total_tasks: 0,
      total_visible_tasks: 0,
      total_entries: 0,
    }),
  );
  await waitFor(() => expect(hook.result.current.isLoading).toBe(false));
  expect(hook.result.current.response?.provisional).toBe(false);
});
