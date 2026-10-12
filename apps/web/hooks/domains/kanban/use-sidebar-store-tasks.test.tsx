import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useStore } from "zustand";
import { createAppStore, type AppState } from "@/lib/state/store";
import { querySidebarTasks } from "@/lib/api/domains/kanban-api";
import { selectSidebarStoreTasks } from "./use-sidebar-store-tasks";
import { useWorkspaceSidebarTasks } from "./use-workspace-sidebar-tasks";
import { DEFAULT_VIEW } from "@/lib/state/slices/ui/sidebar-view-builtins";

let store: ReturnType<typeof createAppStore>;
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: AppState) => unknown) => useStore(store, selector),
  useAppStoreApi: () => store,
}));
vi.mock("@/lib/api/domains/kanban-api", () => ({ querySidebarTasks: vi.fn() }));
vi.mock("@/hooks/use-foreground-refresh", () => ({ useForegroundRefresh: vi.fn() }));

function task(id: string): AppState["kanban"]["tasks"][number] {
  return {
    id,
    workspaceId: "ws-1",
    workflowId: "wf-1",
    workflowStepId: "step-1",
    title: id,
    position: 0,
    parentTaskId: undefined,
    metadata: {},
    origin: "",
    state: "TODO",
    statusSummary: undefined,
    createdAt: "2026-09-29T00:00:00Z",
    updatedAt: "2026-09-29T00:00:00Z",
    repositories: [],
    primaryExecutorType: undefined,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  store = createAppStore();
  store.setState((state) => {
    state.workspaces.activeId = "ws-1";
    state.workspaceContextRead.workspaceId = "ws-1";
    state.workspaceContextRead.generation = state.workspaceContextGeneration;
    state.repositories.itemsByWorkspaceId["ws-1"] = [];
    state.workflows.taskWorkflowCoverage = {
      workspace_id: "ws-1",
      workflow_ids: ["wf-1"],
      complete: true,
    };
    state.workflows.items = [{ id: "wf-1", workspaceId: "ws-1", name: "Workflow" }];
    state.kanbanMulti.snapshots["wf-1"] = {
      workflowId: "wf-1",
      workflowName: "Workflow",
      steps: [],
      tasks: [task("a"), task("b")],
      taskCoverage: {
        workspace_id: "ws-1",
        workflow_id: "wf-1",
        membership: "active",
        complete: true,
        total: 2,
        ordering_profile: "sqlite_nocase_v1",
      },
    };
  });
  vi.mocked(querySidebarTasks).mockResolvedValue({
    query_key: "large",
    page: 1,
    page_size: 100,
    total_entries: 101,
    total_tasks: 101,
    total_visible_tasks: 101,
    has_next: true,
    has_previous: false,
    entries: [],
  });
});
afterEach(cleanup);

describe("complete sidebar store inventory", () => {
  it("reuses current snapshots but rejects a mixed complete and missing workflow inventory", () => {
    expect(selectSidebarStoreTasks(store.getState(), "ws-1")?.map((row) => row.id)).toEqual([
      "a",
      "b",
    ]);
    store.setState((state) => {
      state.workflows.items.push({ id: "wf-2", workspaceId: "ws-1", name: "Missing" });
    });
    expect(selectSidebarStoreTasks(store.getState(), "ws-1")).toBeNull();
  });

  it.each(["isPlaceholder", "fetchFailed"] as const)("rejects %s snapshots", (flag) => {
    store.setState((state) => {
      state.kanbanMulti.snapshots["wf-1"][flag] = true;
    });
    expect(selectSidebarStoreTasks(store.getState(), "ws-1")).toBeNull();
  });

  it("does not reuse another workspace, generation or denied context", () => {
    expect(selectSidebarStoreTasks(store.getState(), "ws-2")).toBeNull();
    store.setState((state) => {
      state.workspaceContextGeneration += 1;
    });
    expect(selectSidebarStoreTasks(store.getState(), "ws-1")).toBeNull();
    store.setState((state) => {
      state.workspaceContextRead.generation = state.workspaceContextGeneration;
      state.workspaceContextRead.errors.workflows = "access_denied";
    });
    expect(selectSidebarStoreTasks(store.getState(), "ws-1")).toBeNull();
  });

  it("does not treat a foreign task as a complete empty inventory", () => {
    store.setState((state) => {
      state.kanbanMulti.snapshots["wf-1"].tasks[0].workspaceId = "ws-2";
    });
    expect(selectSidebarStoreTasks(store.getState(), "ws-1")).toBeNull();
  });

  it("rejects cached tasks after authenticated access is lost", () => {
    store.setState((state) => {
      state.auth = { mode: "enabled", authenticated: false, user: null };
    });
    expect(selectSidebarStoreTasks(store.getState(), "ws-1")).toBeNull();
  });
});

describe("shared sidebar task state", () => {
  it("keeps archived views on the bounded server path", async () => {
    store.setState((state) => {
      state.sidebarViewsByWorkspace["ws-1"] = {
        views: [
          {
            ...DEFAULT_VIEW,
            filters: [{ id: "archive", dimension: "archived", op: "is", value: true }],
          },
        ],
        activeViewId: DEFAULT_VIEW.id,
        draft: null,
        syncError: null,
      };
    });
    const hook = renderHook(() => useWorkspaceSidebarTasks("ws-1"));
    await waitFor(() => expect(hook.result.current.page.response?.query_key).toBe("large"));
    expect(querySidebarTasks).toHaveBeenCalledTimes(1);
    expect(hook.result.current.pageEntries).toEqual([]);
    expect(hook.result.current.allTasks).toEqual([]);
  });

  it("renders immediately, tracks live changes and never queries or accumulates page responses", () => {
    const hook = renderHook(() => useWorkspaceSidebarTasks("ws-1"));
    expect(hook.result.current.allTasks.map((row) => row.id)).toEqual(["a", "b"]);
    expect(hook.result.current.pageEntries?.filter((entry) => entry.kind === "task")).toHaveLength(
      2,
    );
    expect(hook.result.current.page.response?.total_visible_tasks).toBe(2);
    expect(hook.result.current.isLoading).toBe(false);
    act(() =>
      store.setState((state) => {
        state.kanbanMulti.snapshots["wf-1"].tasks = [{ ...task("b"), title: "Updated live" }];
        state.sidebarArchivedTasks.revisionByWorkspaceId["ws-1"] = 1;
      }),
    );
    expect(hook.result.current.allTasks.map((row) => row.title)).toEqual(["Updated live"]);
    expect(querySidebarTasks).not.toHaveBeenCalled();
  });

  it("pages a complete resident set of 101 tasks without any network traversal", () => {
    store.setState((state) => {
      state.kanbanMulti.snapshots["wf-1"].tasks = Array.from({ length: 101 }, (_, i) =>
        task(String(i).padStart(3, "0")),
      );
    });
    const hook = renderHook(() => useWorkspaceSidebarTasks("ws-1"));
    expect(hook.result.current.allTasks).toHaveLength(100);
    expect(hook.result.current.page.response?.has_next).toBe(true);
    act(() => hook.result.current.page.goToPage(2));
    expect(hook.result.current.allTasks.map((task) => task.id)).toEqual(["100"]);
    expect(hook.result.current.page.isRefreshing).toBe(false);
    expect(querySidebarTasks).not.toHaveBeenCalled();
    act(() => hook.result.current.page.goToPage(1));
    expect(hook.result.current.allTasks).toHaveLength(100);
  });
});
