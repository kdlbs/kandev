import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import type { Repository } from "@/lib/types/http";
import { useKanbanData } from "./use-kanban-data";

vi.mock("@/hooks/use-workflow-snapshot", () => ({ useWorkflowSnapshot: vi.fn() }));
vi.mock("@/hooks/use-user-display-settings", async () => {
  const { useAppStore } = await import("@/components/state-provider");
  const { mapSelectedRepositoryIds } = await import("@/lib/kanban/filters");
  return {
    useUserDisplaySettings: () => {
      const settings = useAppStore((state) => state.userSettings);
      const byWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
      const repositories = byWorkspace.active ?? [];
      return {
        settings,
        repositories,
        repositoriesLoading: false,
        commitSettings: vi.fn(),
        allRepositoriesSelected: settings.repositoryIds.length === 0,
        selectedRepositoryIds: mapSelectedRepositoryIds(repositories, settings.repositoryIds),
      };
    },
  };
});

const repositories = [
  { id: "backend", name: "server", local_path: "/repos/server" },
  { id: "web", name: "client-ui", local_path: "/repos/browser-app" },
] as Repository[];
const links = [
  { id: "backend-link", repository_id: "backend", base_branch: "main", position: 0 },
  { id: "web-link", repository_id: "web", base_branch: "main", position: 1 },
];

function makeTask(overrides: Partial<KanbanState["tasks"][number]> = {}) {
  return {
    id: "multi",
    workspaceId: "active",
    title: "Fix rendering",
    description: "Handle resizing",
    workflowId: "workflow",
    workflowStepId: "ready",
    position: 0,
    repositoryId: "backend",
    repositories: links,
    ...overrides,
  };
}

function renderData(task = makeTask(), selected: string[] = [], query = "") {
  const view = renderHook(
    ({ searchQuery }) => ({
      store: useAppStoreApi(),
      data: useKanbanData({
        onWorkspaceChange: vi.fn(),
        onWorkflowChange: vi.fn(),
        searchQuery,
      }),
    }),
    {
      initialProps: { searchQuery: query },
      wrapper: ({ children }) => <StateProvider>{children}</StateProvider>,
    },
  );
  act(() => {
    view.result.current.store.setState((state) => ({
      kanban: { ...state.kanban, workflowId: "workflow", tasks: [task] },
      workspaces: { ...state.workspaces, activeId: "active" },
      workflows: { ...state.workflows, activeId: "workflow" },
      repositories: { ...state.repositories, itemsByWorkspaceId: { active: repositories } },
      userSettings: { ...state.userSettings, repositoryIds: selected },
    }));
  });
  return view;
}

// @covers AC-UI-BOARD-REPOSITORY-MATCHING-001.1, AC-UI-BOARD-REPOSITORY-MATCHING-001.4
describe("useKanbanData repository matching", () => {
  it.each(["CLIENT-UI", "BROWSER-APP"])("searches secondary repository metadata: %s", (query) => {
    const { result } = renderData(makeTask(), [], query);
    expect(result.current.data.filteredTasks.map((task) => task.id)).toEqual(["multi"]);
  });

  it.each([{ selected: [] }, { selected: ["web"] }, { selected: ["backend", "web"] }])(
    "supports repository selection $selected",
    ({ selected }) => {
      const { result } = renderData(makeTask(), selected);
      expect(result.current.data.filteredTasks.map((task) => task.id)).toEqual(["multi"]);
    },
  );

  it.each(["client-ui", "browser-app"])("composes search %s with secondary selection", (query) => {
    const { result } = renderData(makeTask(), ["web"], query);
    expect(result.current.data.filteredTasks.map((task) => task.id)).toEqual(["multi"]);
  });

  // @covers AC-UI-BOARD-REPOSITORY-MATCHING-001.2
  it.each([{ collection: [] }, { collection: [links[0]] }])(
    "does not search a stale scalar outside collection $collection",
    ({ collection }) => {
      const { result } = renderData(
        makeTask({ repositoryId: "web", repositories: collection }),
        [],
        "client-ui",
      );
      expect(result.current.data.filteredTasks).toEqual([]);
    },
  );

  it("does not admit an explicit empty collection under a repository selection", () => {
    const { result } = renderData(makeTask({ repositoryId: "web", repositories: [] }), ["web"]);
    expect(result.current.data.filteredTasks).toEqual([]);
  });

  it("searches a legacy scalar when the collection is absent", () => {
    const { result } = renderData(
      makeTask({ repositoryId: "web", repositories: undefined }),
      ["web"],
      "client-ui",
    );
    expect(result.current.data.filteredTasks.map((task) => task.id)).toEqual(["multi"]);
  });

  it("searches collection-only membership without a scalar", () => {
    const { result } = renderData(makeTask({ repositoryId: undefined }), [], "browser-app");
    expect(result.current.data.filteredTasks.map((task) => task.id)).toEqual(["multi"]);
  });

  it("keeps title and description search for tasks without repository membership", () => {
    const { result, rerender } = renderData(
      makeTask({ repositories: [], repositoryId: undefined }),
      [],
      "rendering",
    );
    expect(result.current.data.filteredTasks).toHaveLength(1);
    rerender({ searchQuery: "resizing" });
    expect(result.current.data.filteredTasks).toHaveLength(1);
    rerender({ searchQuery: "absent" });
    expect(result.current.data.filteredTasks).toEqual([]);
  });

  it("does not bypass membership with a matching title", () => {
    const { result } = renderData(makeTask({ repositories: [] }), ["web"], "rendering");
    expect(result.current.data.filteredTasks).toEqual([]);
  });

  it("ignores missing and other-workspace metadata, then updates when active metadata arrives", () => {
    const { result } = renderData(makeTask(), [], "client-ui");
    act(() => {
      result.current.store.setState((state) => ({
        repositories: {
          ...state.repositories,
          itemsByWorkspaceId: { active: [repositories[0]], other: [repositories[1]] },
        },
      }));
    });
    expect(result.current.data.filteredTasks).toEqual([]);
    act(() => {
      result.current.store.setState((state) => ({
        repositories: { ...state.repositories, itemsByWorkspaceId: { active: repositories } },
      }));
    });
    expect(result.current.data.filteredTasks.map((task) => task.id)).toEqual(["multi"]);
    act(() => {
      result.current.store.setState((state) => ({
        workspaces: { ...state.workspaces, activeId: null },
      }));
    });
    expect(result.current.data.filteredTasks).toEqual([]);
  });
});
