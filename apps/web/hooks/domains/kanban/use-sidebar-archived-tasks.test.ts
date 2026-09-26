import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { listTasksByWorkspace } from "@/lib/api/domains/kanban-api";
import { loadSidebarArchivedTasks, useSidebarArchivedTasks } from "./use-sidebar-archived-tasks";

const hookMocks = vi.hoisted(() => ({
  state: {
    workspaces: { activeId: "ws-1" },
    workspaceContextGeneration: 1,
    sidebarArchivedTasks: {
      itemsByWorkspaceId: {},
      loadedByWorkspaceId: {},
      loadingByWorkspaceId: {},
      errorByWorkspaceId: {},
      revisionByWorkspaceId: {},
    },
    setSidebarArchivedTasks: vi.fn(() => true),
    setSidebarArchivedTasksLoading: vi.fn(),
    setSidebarArchivedTasksError: vi.fn(),
  },
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  listTasksByWorkspace: vi.fn(),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof hookMocks.state) => unknown) => selector(hookMocks.state),
  useAppStoreApi: () => ({ getState: () => hookMocks.state }),
}));

vi.mock("@/hooks/use-foreground-refresh", () => ({
  useForegroundRefresh: vi.fn(),
}));

describe("loadSidebarArchivedTasks", () => {
  beforeEach(() => vi.mocked(listTasksByWorkspace).mockReset());

  it("loads every archived-only page and stops at the reported total", async () => {
    vi.mocked(listTasksByWorkspace)
      .mockResolvedValueOnce({
        tasks: [{ id: "archived-1" }],
        total: 2,
      } as never)
      .mockResolvedValueOnce({
        tasks: [{ id: "archived-2" }],
        total: 2,
      } as never);

    await expect(loadSidebarArchivedTasks("ws-1")).resolves.toEqual([
      { id: "archived-1" },
      { id: "archived-2" },
    ]);
    expect(listTasksByWorkspace).toHaveBeenNthCalledWith(1, "ws-1", {
      page: 1,
      pageSize: 100,
      onlyArchived: true,
    });
    expect(listTasksByWorkspace).toHaveBeenNthCalledWith(2, "ws-1", {
      page: 2,
      pageSize: 100,
      onlyArchived: true,
    });
  });
});

describe("useSidebarArchivedTasks", () => {
  beforeEach(() => {
    hookMocks.state.workspaces.activeId = "ws-1";
    hookMocks.state.workspaceContextGeneration = 1;
    hookMocks.state.sidebarArchivedTasks.loadingByWorkspaceId = {};
    vi.clearAllMocks();
  });

  it("clears loading when the request finishes after leaving the workspace", async () => {
    let resolveRequest!: (value: { tasks: never[]; total: number }) => void;
    const request = new Promise<{ tasks: never[]; total: number }>((resolve) => {
      resolveRequest = resolve;
    });
    vi.mocked(listTasksByWorkspace).mockReturnValue(request as never);

    renderHook(() => useSidebarArchivedTasks("ws-1", true));
    await waitFor(() =>
      expect(hookMocks.state.setSidebarArchivedTasksLoading).toHaveBeenCalledWith("ws-1", true),
    );

    hookMocks.state.workspaces.activeId = "ws-2";
    hookMocks.state.workspaceContextGeneration = 2;
    resolveRequest({ tasks: [], total: 0 });

    await waitFor(() =>
      expect(hookMocks.state.setSidebarArchivedTasksLoading).toHaveBeenCalledWith("ws-1", false),
    );
  });
});
