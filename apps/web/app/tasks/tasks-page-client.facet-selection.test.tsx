import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import { defaultSettingsState } from "@/lib/state/slices/settings/settings-slice";
import { pluginRegistry } from "@/lib/plugins/registry";
import type { Task } from "@/lib/types/http";

const { replace, updateUserSettings, listTasksByWorkspace } = vi.hoisted(() => ({
  replace: vi.fn(),
  updateUserSettings: vi.fn().mockResolvedValue({}),
  listTasksByWorkspace: vi.fn(),
}));
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: vi.fn(), replace }),
  useSearchParams: () => new URLSearchParams(window.location.search),
}));
vi.mock("@/lib/api", () => ({
  archiveTask: vi.fn(),
  deleteTask: vi.fn(),
  unarchiveTask: vi.fn(),
  listTasksByWorkspace,
  updateUserSettings,
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));
vi.mock("@/hooks/use-kanban-display-settings", () => ({
  useKanbanDisplaySettings: () => ({
    activeWorkspaceId: "ws1",
    activeWorkflowId: null,
    repositories: [],
    selectedRepositoryId: null,
  }),
}));
vi.mock("@/hooks/use-debounce", () => ({ useDebounce: (value: unknown) => value }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
}));
vi.mock("@/hooks/use-task-listing-view", () => ({
  useTaskListingView: () => ({ effectiveView: "list", setView: vi.fn() }),
}));
vi.mock("@/hooks/use-foreground-refresh", () => ({ useForegroundRefresh: () => undefined }));
vi.mock("@/hooks/use-workflow-snapshot", () => ({ useWorkflowSnapshot: () => undefined }));
vi.mock("@/hooks/domains/gitlab/use-task-mr", () => ({ useWorkspaceMRs: () => undefined }));
vi.mock("@/hooks/domains/github/use-task-pr", () => ({ useWorkspacePRs: () => undefined }));
vi.mock("./tasks-page-content", () => ({
  TasksPageContent: ({
    tasks,
    sort,
    group,
    onSortChange,
    onGroupChange,
  }: {
    tasks: Task[];
    sort: string;
    group: string;
    onSortChange: (value: string) => void;
    onGroupChange: (value: string) => void;
  }) => (
    <div>
      <output data-testid="sort">{sort}</output>
      <output data-testid="group">{group}</output>
      <ol data-testid="rows">
        {tasks.map((task) => (
          <li key={task.id}>{task.id}</li>
        ))}
      </ol>
      <button onClick={() => onSortChange("facet:plugin:tags")}>Choose facet</button>
      <button onClick={() => onGroupChange("facet:plugin:tags")}>Choose facet group</button>
    </div>
  ),
}));
import { TasksPageClient } from "./tasks-page-client";
let capturedStore: StoreApi<AppState>;

function StoreCapture() {
  capturedStore = useAppStoreApi();
  return null;
}

const key = "facet:plugin:tags";
const tasks = [
  {
    id: "alpha",
    title: "Alpha",
    updated_at: "2026-01-01T00:00:00Z",
    created_at: "2026-01-01T00:00:00Z",
  },
  {
    id: "beta",
    title: "Beta",
    updated_at: "2026-02-01T00:00:00Z",
    created_at: "2026-02-01T00:00:00Z",
  },
] as Task[];

function renderPage(
  initialSort: "title_asc" | typeof key = "title_asc",
  initialGroup: "workflow_step" | typeof key = "workflow_step",
  savedWorkflowId: string | null = null,
) {
  return render(
    <StateProvider
      initialState={{
        workspaces: { items: [], activeId: "ws1" },
        userSettings: {
          ...defaultSettingsState.userSettings,
          workspaceId: "ws1",
          workflowId: savedWorkflowId,
          tasksListSort: initialSort,
          tasksListGroup: initialGroup,
          loaded: true,
        },
      }}
    >
      <StoreCapture />
      <TasksPageClient
        workspaces={[]}
        initialWorkspaceId="ws1"
        initialMetadataLoaded={{ workflows: true, repositories: true }}
        initialRepositories={[]}
        initialTasks={tasks}
        initialTotal={2}
        initialDataLoaded
        initialSort={initialSort as never}
        initialGroup={initialGroup as never}
      />
    </StateProvider>,
  );
}

beforeEach(() => {
  listTasksByWorkspace.mockReset().mockResolvedValue({ tasks, total: tasks.length });
  updateUserSettings.mockReset().mockResolvedValue({});
});

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin("plugin");
  replace.mockReset();
  updateUserSettings.mockClear();
  window.history.replaceState({}, "", "/");
});

// @covers AC-TASKS-LIST-ARCHIVE-FILTERS-002.5 AC-TASKS-LIST-ARCHIVE-FILTERS-004.3
it("uses the resolved workspace workflow without saving a stale portable default", async () => {
  window.history.replaceState({}, "", "/tasks?q=current&repository=all");
  renderPage("title_asc", "workflow_step", "previous-workspace-workflow");

  await waitFor(() =>
    expect(listTasksByWorkspace).toHaveBeenCalledWith(
      "ws1",
      expect.objectContaining({ query: "current", workflowId: null }),
      expect.anything(),
    ),
  );
  expect(capturedStore.getState().userSettings.workflowId).toBe("previous-workspace-workflow");
  expect(updateUserSettings).not.toHaveBeenCalled();
});

describe("TasksPageClient facet preference", () => {
  // AC-PLUGINS-TASKLIST-FACETS-002.6 and 003.1: preserve loaded ties on selection.
  it("persists a chosen facet without a built-in local re-sort", async () => {
    pluginRegistry.forPlugin("plugin").registerTaskListFacet({
      id: "tags",
      label: "Tag",
      getValues: () => [{ value: "same", label: "Same" }],
    });
    window.history.replaceState({}, "", "/tasks");
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Choose facet" }));
    await waitFor(() => expect(screen.getByTestId("rows").textContent).toBe("alphabeta"));
    expect(screen.getByTestId("sort").textContent).toBe(key);
    await waitFor(() =>
      expect(updateUserSettings).toHaveBeenCalledWith(
        expect.objectContaining({ tasks_list_sort: key }),
        { cache: "no-store" },
      ),
    );
    expect(replace).toHaveBeenCalledWith(expect.stringContaining("sort=facet%3Aplugin%3Atags"), {
      scroll: false,
    });
  });

  // AC-PLUGINS-TASKLIST-FACETS-003.3: a link must retain the selection while unavailable.
  it("does not save the fallback when a facet deep link is temporarily unavailable", () => {
    window.history.replaceState({}, "", `/tasks?sort=${key}&group=${key}`);
    renderPage(key, key);
    expect(screen.getByTestId("sort").textContent).toBe("updated_desc");
    expect(screen.getByTestId("group").textContent).toBe("workflow_step");
    expect(updateUserSettings).not.toHaveBeenCalledWith(
      expect.objectContaining({
        tasks_list_sort: "updated_desc",
        tasks_list_group: "workflow_step",
      }),
      expect.anything(),
    );
    expect(new URLSearchParams(window.location.search).get("sort")).toBe(key);
  });
});

describe("TasksPageClient facet selection lifecycle", () => {
  // AC-PLUGINS-TASKLIST-FACETS-003.1: selecting Group persists and deep-links the facet.
  it("persists a chosen facet group in settings and the route", async () => {
    pluginRegistry.forPlugin("plugin").registerTaskListFacet({
      id: "tags",
      label: "Tag",
      getValues: () => [{ value: "same", label: "Same" }],
    });
    window.history.replaceState({}, "", "/tasks");
    renderPage();
    updateUserSettings.mockClear();
    replace.mockClear();

    fireEvent.click(screen.getByRole("button", { name: "Choose facet group" }));

    expect(screen.getByTestId("group").textContent).toBe(key);
    await waitFor(() =>
      expect(updateUserSettings).toHaveBeenCalledWith(
        expect.objectContaining({ tasks_list_group: key }),
        { cache: "no-store" },
      ),
    );
    expect(replace).toHaveBeenCalledWith(
      expect.stringContaining(`group=${encodeURIComponent(key)}`),
      {
        scroll: false,
      },
    );
  });

  // AC-PLUGINS-TASKLIST-FACETS-003.3: mounted registry changes preserve and restore the request.
  it("falls back and restores when the facet unregisters and registers without reload", async () => {
    pluginRegistry.forPlugin("plugin").registerTaskListFacet({
      id: "tags",
      label: "Tag",
      getValues: () => [{ value: "alpha", label: "Alpha" }],
    });
    window.history.replaceState({}, "", `/tasks?sort=${key}&group=${key}`);
    renderPage(key, key);
    expect(screen.getByTestId("sort").textContent).toBe(key);
    expect(screen.getByTestId("group").textContent).toBe(key);
    updateUserSettings.mockClear();
    replace.mockClear();

    act(() => pluginRegistry.unregisterPlugin("plugin"));

    await waitFor(() => {
      expect(screen.getByTestId("sort").textContent).toBe("updated_desc");
      expect(screen.getByTestId("group").textContent).toBe("workflow_step");
    });
    expect(new URLSearchParams(window.location.search).get("sort")).toBe(key);
    expect(new URLSearchParams(window.location.search).get("group")).toBe(key);
    expect(capturedStore.getState().userSettings.tasksListSort).toBe(key);
    expect(capturedStore.getState().userSettings.tasksListGroup).toBe(key);
    expect(updateUserSettings).not.toHaveBeenCalled();
    expect(replace).not.toHaveBeenCalled();

    act(() => {
      pluginRegistry.forPlugin("plugin").registerTaskListFacet({
        id: "tags",
        label: "Tag",
        getValues: () => [{ value: "alpha", label: "Alpha" }],
      });
    });

    await waitFor(() => {
      expect(screen.getByTestId("sort").textContent).toBe(key);
      expect(screen.getByTestId("group").textContent).toBe(key);
    });
    expect(new URLSearchParams(window.location.search).get("sort")).toBe(key);
    expect(new URLSearchParams(window.location.search).get("group")).toBe(key);
    expect(capturedStore.getState().userSettings.tasksListSort).toBe(key);
    expect(capturedStore.getState().userSettings.tasksListGroup).toBe(key);
    expect(updateUserSettings).not.toHaveBeenCalled();
    expect(replace).not.toHaveBeenCalled();
  });
});

describe("TasksPageClient list preference persistence", () => {
  it("keeps rapid sort and group writes in selection order", async () => {
    pluginRegistry.forPlugin("plugin").registerTaskListFacet({
      id: "tags",
      label: "Tag",
      getValues: () => [{ value: "same", label: "Same" }],
    });
    window.history.replaceState({}, "", "/tasks");
    renderPage();
    updateUserSettings.mockClear();
    const releases: Array<() => void> = [];
    updateUserSettings.mockImplementation(
      () =>
        new Promise<Record<string, never>>((resolve) => {
          releases.push(() => resolve({}));
        }),
    );

    fireEvent.click(screen.getByRole("button", { name: "Choose facet" }));
    await waitFor(() => expect(updateUserSettings).toHaveBeenCalledTimes(1));
    fireEvent.click(screen.getByRole("button", { name: "Choose facet group" }));
    await act(async () => {});
    expect(updateUserSettings).toHaveBeenCalledTimes(1);

    await act(async () => releases[0]?.());
    await waitFor(() => expect(updateUserSettings).toHaveBeenCalledTimes(2));
    expect(updateUserSettings).toHaveBeenLastCalledWith(
      { tasks_list_sort: key, tasks_list_group: key },
      { cache: "no-store" },
    );
    await act(async () => releases[1]?.());
  });
});
