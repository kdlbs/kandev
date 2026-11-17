import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { defaultSettingsState } from "@/lib/state/slices/settings/settings-slice";
import type { TasksListViewProps } from "./tasks-list-view";
import { TasksListFilters } from "./tasks-list-filters";
import { taskId, type Repository, type Task } from "@/lib/types/http";

const UNAVAILABLE_FILTER_LABEL = "Unavailable filter";

const controls = vi.hoisted(() => ({
  mobile: false,
  display: {
    activeWorkspaceId: "ws1",
    activeWorkflowId: "wf1",
    repositories: [],
    selectedRepositoryId: "repo1",
  },
  list: vi.fn(),
  toast: vi.fn(),
  setView: vi.fn(),
}));

vi.mock("@/lib/api", () => ({
  listTasksByWorkspace: controls.list,
  deleteTask: vi.fn(),
  unarchiveTask: vi.fn(),
  updateUserSettings: vi.fn().mockResolvedValue(undefined),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: controls.toast }) }));
vi.mock("@/hooks/use-kanban-display-settings", () => ({
  useKanbanDisplaySettings: () => ({
    ...controls.display,
    repositories: useAppStore(
      (state) =>
        state.repositories.itemsByWorkspaceId[controls.display.activeWorkspaceId] ??
        controls.display.repositories,
    ),
  }),
}));
vi.mock("@/hooks/use-user-display-settings", () => ({
  useUserDisplaySettings: () => ({
    settings: useAppStore((state) => state.userSettings),
    commitSettings: vi.fn(),
  }),
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: controls.mobile }),
}));
vi.mock("@/hooks/use-task-listing-view", () => ({
  useTaskListingView: () => ({ setView: controls.setView }),
}));
vi.mock("@/hooks/use-workflow-snapshot", () => ({ useWorkflowSnapshot: () => undefined }));
vi.mock("@/hooks/domains/github/use-task-pr", () => ({ useWorkspacePRs: () => undefined }));
vi.mock("@/hooks/domains/gitlab/use-task-mr", () => ({ useWorkspaceMRs: () => undefined }));
vi.mock("@/hooks/use-task-actions", () => ({
  useTaskActions: () => ({ archiveTaskById: vi.fn() }),
}));
vi.mock("@/hooks/use-task-list-workflow-steps", () => ({
  useTasksListStepRefresh: () => ({ workflows: [], previews: {}, refresh: vi.fn() }),
}));
vi.mock("./mobile-tasks-actions", () => ({ MobileTasksActions: () => null }));

// Keep List filters, URL state, debounce, and page effects real.
// Header chrome and rows observe the query without owning it.
vi.mock("@/components/kanban/kanban-header", () => ({
  KanbanHeader: () => null,
}));
vi.mock("./tasks-list-view", () => ({
  TasksListView: ({
    tasks,
    total,
    isLoading,
    pagination,
    setPagination,
    filters,
    archiveMode,
    setArchiveMode,
    unavailableFilter,
  }: TasksListViewProps) => (
    <div>
      <TasksListFilters
        {...filters!}
        archiveMode={archiveMode}
        onArchiveModeChange={setArchiveMode}
        displayOptions={null}
        total={total}
        isLoading={isLoading}
      />
      <output aria-label="Rows">{tasks.map((task) => task.title).join(",")}</output>
      <output aria-label="Total">{total}</output>
      <output aria-label="Loading">{String(isLoading)}</output>
      <output aria-label="Page">{pagination.pageIndex + 1}</output>
      <output aria-label={UNAVAILABLE_FILTER_LABEL}>{unavailableFilter ?? ""}</output>
      <button onClick={() => setPagination((prev) => ({ ...prev, pageIndex: 1 }))}>Page two</button>
      <button onClick={() => setPagination((prev) => ({ ...prev, pageSize: 10 }))}>Ten rows</button>
      <button onClick={() => setArchiveMode("all")}>Include archived</button>
    </div>
  ),
}));

import { TasksPageClient } from "./tasks-page-client";

let testStore: ReturnType<typeof useAppStoreApi>;
function CaptureStore() {
  testStore = useAppStoreApi();
  return null;
}

function page(fromBoot = true, metadataLoaded = fromBoot) {
  return (
    <StateProvider
      initialState={{
        workspaces: { items: [], activeId: "ws1" },
        workflows: {
          items: fromBoot ? [{ id: "wf1", workspaceId: "ws1", name: "Workflow" } as never] : [],
          activeId: "wf1",
        },
        userSettings: {
          ...defaultSettingsState.userSettings,
          workspaceId: "ws1",
          workflowId: "wf1",
          repositoryIds: ["repo1"],
          loaded: true,
        },
      }}
    >
      <CaptureStore />
      <TasksPageClient
        workspaces={[]}
        initialWorkspaceId="ws1"
        initialRepositories={[{ id: "repo1", name: "Repository" } as Repository]}
        initialTasks={[]}
        initialTotal={0}
        initialDataLoaded={fromBoot}
        initialMetadataLoaded={{ workflows: metadataLoaded, repositories: metadataLoaded }}
        initialSort="title_asc"
        initialGroup="none"
      />
    </StateProvider>
  );
}

async function mount() {
  const view = render(page());
  await act(async () => {});
  return view;
}

async function advance(ms: number) {
  await act(async () => {
    vi.advanceTimersByTime(ms);
  });
}

function input() {
  return screen.getByRole("textbox") as HTMLInputElement;
}
function type(query: string) {
  fireEvent.change(input(), { target: { value: query } });
}
function clear() {
  fireEvent.click(input().parentElement!.querySelector("button")!);
}
function result(title: string) {
  return { tasks: [{ id: taskId(title), title } as Task], total: 1 };
}
function deferred() {
  let resolve!: (value: ReturnType<typeof result>) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<ReturnType<typeof result>>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  vi.useFakeTimers();
  controls.mobile = false;
  controls.display.activeWorkspaceId = "ws1";
  controls.list.mockReset().mockResolvedValue({ tasks: [], total: 0 });
  controls.toast.mockReset();
  window.history.replaceState({}, "", "/tasks");
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  window.history.replaceState({}, "", "/");
});

describe("List search admission", () => {
  // @covers AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.13
  it.each([false, true])(
    "admits the last input after one debounce window (mobile=%s)",
    async (mobile) => {
      controls.mobile = mobile;
      await mount();
      expect(controls.list).not.toHaveBeenCalled();
      type("Alpha");
      expect(input().value).toBe("Alpha");
      await advance(299);
      expect(controls.list).not.toHaveBeenCalled();
      await advance(1);
      expect(controls.list).toHaveBeenCalledTimes(1);
      expect(controls.list).toHaveBeenCalledWith(
        "ws1",
        expect.objectContaining({ query: "Alpha", page: 1 }),
        { init: { signal: expect.any(AbortSignal) } },
      );
    },
  );

  it("replaces queued edits with the latest input", async () => {
    await mount();
    type("A");
    await advance(100);
    type("AB");
    await advance(100);
    type("ABC");
    await advance(299);
    expect(input().value).toBe("ABC");
    expect(controls.list).not.toHaveBeenCalled();
    await advance(1);
    expect(controls.list).toHaveBeenCalledTimes(1);
    expect(controls.list.mock.calls[0][1].query).toBe("ABC");
    await advance(600);
    expect(controls.list).toHaveBeenCalledTimes(1);
  });

  // @covers AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.14
  it("clear cancels queued input without issuing it later", async () => {
    await mount();
    type("queued");
    await advance(100);
    clear();
    expect(input().value).toBe("");
    await advance(900);
    expect(controls.list.mock.calls.map((call) => call[1].query)).toEqual([""]);
  });

  it.each(["button", "typing"])("clears an admitted query using %s", async (method) => {
    await mount();
    type("Alpha");
    await advance(300);
    controls.list.mockClear();
    if (method === "button") {
      clear();
      await act(async () => {});
    } else {
      type("");
      await advance(300);
    }
    expect(input().value).toBe("");
    expect(controls.list).toHaveBeenCalledWith("ws1", expect.objectContaining({ query: "" }), {
      init: { signal: expect.any(AbortSignal) },
    });
    await advance(600);
    expect(controls.list).toHaveBeenCalledTimes(1);
  });

  it("unmount cancels the pending callback", async () => {
    const view = await mount();
    type("queued");
    await advance(100);
    view.unmount();
    await advance(900);
    expect(controls.list).not.toHaveBeenCalled();
  });
});

// @covers AC-TASKS-LIST-ARCHIVE-FILTERS-004.5
it("waits for metadata before treating a saved workflow as unavailable", async () => {
  const view = render(page(false));
  await act(async () => {});
  expect(screen.getByLabelText(UNAVAILABLE_FILTER_LABEL).textContent).toBe("");
  expect(screen.getByLabelText("Loading").textContent).toBe("true");
  expect(controls.list).not.toHaveBeenCalled();

  await act(async () => {
    const state = testStore.getState();
    const generation = state.workspaceContextGeneration;
    state.setWorkspaceContextRead("workflows", "ws1", generation, "pending");
    state.hydrate({
      workflows: {
        items: [{ id: "wf1", workspaceId: "ws1", name: "Workflow" } as never],
        activeId: "wf1",
      },
    });
    state.setWorkspaceContextRead("workflows", "ws1", generation, "success");
    state.setRepositories("ws1", [{ id: "repo1", name: "Repository" } as Repository]);
  });
  view.rerender(page(false, true));
  await act(async () => {});
  expect(screen.getByLabelText(UNAVAILABLE_FILTER_LABEL).textContent).toBe("");
  expect(screen.getByLabelText("Loading").textContent).toBe("false");
  expect(controls.list).toHaveBeenCalledWith(
    "ws1",
    expect.objectContaining({ workflowId: "wf1", repositoryId: "repo1" }),
    expect.anything(),
  );
});

// @covers AC-TASKS-LIST-ARCHIVE-FILTERS-004.5
it("waits for a fresh repository read before rejecting an ID absent from cached metadata", async () => {
  window.history.replaceState({}, "", "/tasks?workflow=all&repository=repo2");
  const view = render(page(false));
  await act(async () => {
    testStore.getState().setRepositories("ws1", []);
  });
  expect(screen.getByLabelText(UNAVAILABLE_FILTER_LABEL).textContent).toBe("");
  expect(screen.getByLabelText("Loading").textContent).toBe("true");
  expect(controls.list).not.toHaveBeenCalled();
  await act(async () => {
    testStore
      .getState()
      .setRepositories("ws1", [{ id: "repo2", name: "New repository" } as Repository]);
  });
  view.rerender(page(false, true));
  await act(async () => {});
  expect(screen.getByLabelText(UNAVAILABLE_FILTER_LABEL).textContent).toBe("");
  expect(screen.getByLabelText("Loading").textContent).toBe("false");
  expect(controls.list).toHaveBeenCalledWith(
    "ws1",
    expect.objectContaining({ workflowId: null, repositoryId: "repo2" }),
    expect.anything(),
  );
});

describe("List search scope and reply ownership", () => {
  // @covers AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.15
  it("resets to page one while retaining active scope and page size", async () => {
    await mount();
    controls.list.mockResolvedValue({ tasks: [], total: 30 });
    fireEvent.click(screen.getByText("Include archived"));
    fireEvent.click(screen.getByText("Ten rows"));
    fireEvent.click(screen.getByText("Page two"));
    await act(async () => {});
    expect(screen.getByLabelText("Page").textContent).toBe("2");
    controls.list.mockClear();
    type("Alpha");
    await advance(300);
    expect(screen.getByLabelText("Page").textContent).toBe("1");
    expect(controls.list).toHaveBeenLastCalledWith(
      "ws1",
      {
        page: 1,
        pageSize: 10,
        query: "Alpha",
        includeArchived: true,
        workflowId: "wf1",
        repositoryId: "repo1",
        sort: "title_asc",
      },
      { init: { signal: expect.any(AbortSignal) } },
    );
  });

  it.each(["success", "error"])(
    "ignores a late older %s and its loading finalizer",
    async (outcome) => {
      const a = deferred();
      const b = deferred();
      controls.list.mockImplementation((_workspace, params) =>
        params.query === "Alpha" ? a.promise : b.promise,
      );
      await mount();
      type("Alpha");
      await advance(300);
      type("Beta");
      await advance(300);
      await act(async () => {
        if (outcome === "success") a.resolve(result("Alpha"));
        else a.reject(new Error("obsolete query"));
      });
      expect(screen.getByLabelText("Loading").textContent).toBe("true");
      expect(screen.getByLabelText("Rows").textContent).toBe("");
      expect(controls.toast).not.toHaveBeenCalled();
      await act(async () => {
        b.resolve(result("Beta"));
      });
      expect(screen.getByLabelText("Rows").textContent).toBe("Beta");
      expect(screen.getByLabelText("Total").textContent).toBe("1");
      expect(screen.getByLabelText("Loading").textContent).toBe("false");
    },
  );

  it("retains newer rows when the older reply arrives afterward", async () => {
    const a = deferred();
    const b = deferred();
    controls.list.mockImplementation((_workspace, params) =>
      params.query === "Alpha" ? a.promise : b.promise,
    );
    await mount();
    type("Alpha");
    await advance(300);
    type("Beta");
    await advance(300);
    await act(async () => {
      b.resolve(result("Beta"));
    });
    await act(async () => {
      a.resolve(result("Alpha"));
    });
    expect(screen.getByLabelText("Rows").textContent).toBe("Beta");
    expect(screen.getByLabelText("Total").textContent).toBe("1");
    expect(screen.getByLabelText("Loading").textContent).toBe("false");
  });

  it("rejects a response from a previous workspace", async () => {
    const old = deferred();
    controls.list.mockImplementation((workspace) =>
      workspace === "ws1" ? old.promise : Promise.resolve(result("New workspace")),
    );
    const view = await mount();
    type("Alpha");
    await advance(300);
    controls.display.activeWorkspaceId = "ws2";
    view.rerender(page());
    await act(async () => {});
    await act(async () => {
      old.resolve(result("Old workspace"));
    });
    expect(screen.getByLabelText("Rows").textContent).toBe("New workspace");
    expect(screen.getByLabelText("Loading").textContent).toBe("false");
  });
});
