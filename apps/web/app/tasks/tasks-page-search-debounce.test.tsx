import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ComponentProps } from "react";
import { StateProvider, useAppStore } from "@/components/state-provider";
import { defaultSettingsState } from "@/lib/state/slices/settings/settings-slice";
import { TaskSearchInput } from "@/components/kanban/task-search-input";
import type { KanbanHeader } from "@/components/kanban/kanban-header";
import type { TasksListViewProps } from "./tasks-list-view";
import { taskId, type Task } from "@/lib/types/http";

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
  router: { push: vi.fn(), replace: vi.fn() },
  setView: vi.fn(),
}));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => controls.router,
  useSearchParams: () => new URLSearchParams(),
}));
vi.mock("@/lib/api", () => ({
  listTasksByWorkspace: controls.list,
  deleteTask: vi.fn(),
  unarchiveTask: vi.fn(),
  updateUserSettings: vi.fn().mockResolvedValue(undefined),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: controls.toast }) }));
vi.mock("@/hooks/use-kanban-display-settings", () => ({
  useKanbanDisplaySettings: () => controls.display,
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

// Keep TasksPageContent, MobileSearchBar, TaskSearchInput and page effects real.
// Header chrome and row presentation are boundary observers, not query owners.
vi.mock("@/components/kanban/kanban-header", () => ({
  KanbanHeader: ({ searchQuery, onSearchChange }: ComponentProps<typeof KanbanHeader>) => {
    const setOpen = useAppStore((state) => state.setMobileKanbanSearchOpen);
    return controls.mobile ? (
      <button onClick={() => setOpen(true)}>Reveal search</button>
    ) : (
      <TaskSearchInput value={searchQuery ?? ""} onChange={onSearchChange!} />
    );
  },
}));
vi.mock("./tasks-list-view", () => ({
  TasksListView: ({
    tasks,
    total,
    isLoading,
    pagination,
    setPagination,
    setShowArchived,
  }: TasksListViewProps) => (
    <div>
      <output aria-label="Rows">{tasks.map((task) => task.title).join(",")}</output>
      <output aria-label="Total">{total}</output>
      <output aria-label="Loading">{String(isLoading)}</output>
      <output aria-label="Page">{pagination.pageIndex + 1}</output>
      <button onClick={() => setPagination({ pageIndex: 1, pageSize: 10 })}>Page two</button>
      <button onClick={() => setShowArchived(true)}>Include archived</button>
    </div>
  ),
}));

import { TasksPageClient } from "./tasks-page-client";

function page() {
  return (
    <StateProvider
      initialState={{
        workspaces: { items: [], activeId: "ws1" },
        userSettings: { ...defaultSettingsState.userSettings, loaded: true },
      }}
    >
      <TasksPageClient
        workspaces={[]}
        initialRepositories={[]}
        initialTasks={[]}
        initialTotal={0}
        initialDataLoaded
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
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("List search admission", () => {
  // @covers AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.13
  it.each([false, true])(
    "admits the last input after one debounce window (mobile=%s)",
    async (mobile) => {
      controls.mobile = mobile;
      await mount();
      if (mobile) fireEvent.click(screen.getByText("Reveal search"));
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
    expect(controls.list).not.toHaveBeenCalled();
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
    expect(controls.list).toHaveBeenCalledWith("ws1", expect.objectContaining({ query: "" }));
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

describe("List search scope and reply ownership", () => {
  // @covers AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.15
  it("resets to page one while retaining active scope and page size", async () => {
    await mount();
    fireEvent.click(screen.getByText("Include archived"));
    fireEvent.click(screen.getByText("Page two"));
    await act(async () => {});
    expect(screen.getByLabelText("Page").textContent).toBe("2");
    controls.list.mockClear();
    type("Alpha");
    await advance(300);
    expect(screen.getByLabelText("Page").textContent).toBe("1");
    expect(controls.list).toHaveBeenLastCalledWith("ws1", {
      page: 1,
      pageSize: 10,
      query: "Alpha",
      includeArchived: true,
      workflowId: "wf1",
      repositoryId: "repo1",
      sort: "title_asc",
    });
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
