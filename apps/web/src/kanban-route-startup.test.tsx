import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { AppState, HydrationState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import { defaultState } from "@/lib/state/default-state";
import type { StartupPage } from "@/lib/types/http-user-settings";
import { mapUserSettingsData } from "@/lib/ssr/user-settings";
import { TASK_LISTING_VIEW_STORAGE_KEY } from "@/lib/task-listing/view-preference";
import { KanbanRoute, type KanbanRouteSelection } from "./kanban-route";

const mocks = vi.hoisted(() => ({
  router: { replace: vi.fn() },
  listWorkspaces: vi.fn(),
  fetchUserSettings: vi.fn(),
  listWorkflows: vi.fn(),
  listRepositories: vi.fn(),
}));
vi.mock("@/lib/api/domains/workspace-api", () => ({
  listWorkspaces: mocks.listWorkspaces,
  listRepositories: mocks.listRepositories,
}));
vi.mock("@/lib/api/domains/settings-api", () => ({ fetchUserSettings: mocks.fetchUserSettings }));
vi.mock("@/lib/api/domains/kanban-api", () => ({ listWorkflows: mocks.listWorkflows }));
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => mocks.router,
  useSearchParams: () => new URLSearchParams(),
}));
vi.mock("@/components/kanban-with-preview", () => ({ KanbanWithPreview: () => null }));
vi.mock("@/components/onboarding-dialog", () => ({ OnboardingDialog: () => null }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
}));

function workspace(id: string, officeWorkflowId: string | null = null) {
  return {
    id,
    name: id,
    description: null,
    owner_id: "owner",
    default_executor_id: null,
    default_environment_id: null,
    default_agent_profile_id: null,
    default_config_agent_profile_id: null,
    office_workflow_id: officeWorkflowId,
    created_at: "2026-09-10",
    updated_at: "2026-09-10",
  };
}
function settings(startupPage: StartupPage = "threads") {
  return {
    settings: {
      startup_page: startupPage,
      workspace_id: "ws-1",
      workflow_filter_id: "",
      repository_ids: [],
    },
  };
}
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}
let store: StoreApi<AppState>;
function CaptureStore() {
  store = useAppStoreApi();
  return null;
}
function Subject({
  route = {},
  initialState,
}: {
  route?: KanbanRouteSelection;
  initialState?: HydrationState;
}) {
  return (
    <StateProvider initialState={initialState}>
      <CaptureStore />
      <KanbanRoute route={route} fallback={<p role="status">Loading</p>} />
    </StateProvider>
  );
}

beforeEach(() => {
  vi.resetAllMocks();
  window.localStorage.clear();
  mocks.listWorkspaces.mockResolvedValue({ workspaces: [workspace("ws-1")], total: 1 });
  mocks.fetchUserSettings.mockResolvedValue(settings());
  mocks.listWorkflows.mockResolvedValue({ workflows: [] });
  mocks.listRepositories.mockResolvedValue({ repositories: [] });
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  window.localStorage.clear();
});

// @covers AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.4, 003.6, 003.7
// eslint-disable-next-line max-lines-per-function -- startup cases share the route and store fixture
describe("Home startup bootstrap", () => {
  it("waits for authoritative settings before making one redirect, even without workflows", async () => {
    window.localStorage.setItem(TASK_LISTING_VIEW_STORAGE_KEY, '"list"');
    const pending = deferred<ReturnType<typeof settings>>();
    mocks.fetchUserSettings.mockReturnValue(pending.promise);
    render(<Subject />);
    expect(mocks.router.replace).not.toHaveBeenCalled();
    await act(async () => {
      pending.resolve(settings());
    });
    await waitFor(() =>
      expect(mocks.router.replace).toHaveBeenCalledExactlyOnceWith("/threads?workspace=ws-1"),
    );
  });

  it("honors boot settings without fetching when board state is already hydrated", async () => {
    render(
      <Subject
        initialState={{
          workspaces: { activeId: "ws-1", items: [workspace("ws-1")] },
          userSettings: mapUserSettingsData(settings().settings),
          workflows: {
            activeId: "wf-1",
            items: [
              { id: "wf-1", workspaceId: "ws-1", name: "Main", description: null, sortOrder: 0 },
            ],
          },
          kanban: { ...defaultState.kanban, workflowId: "wf-1" },
        }}
      />,
    );
    await waitFor(() =>
      expect(mocks.router.replace).toHaveBeenCalledExactlyOnceWith("/threads?workspace=ws-1"),
    );
    expect(mocks.fetchUserSettings).not.toHaveBeenCalled();
  });

  it("ignores a late response after the requested workspace changes", async () => {
    const old = deferred<ReturnType<typeof settings>>();
    mocks.fetchUserSettings.mockReturnValueOnce(old.promise).mockResolvedValue(settings());
    mocks.listWorkspaces.mockResolvedValue({
      workspaces: [workspace("ws-1"), workspace("ws-2")],
      total: 2,
    });
    const { rerender } = render(<Subject route={{ workspaceId: "ws-1" }} />);
    rerender(<Subject route={{ workspaceId: "ws-2" }} />);
    await waitFor(() =>
      expect(mocks.router.replace).toHaveBeenCalledExactlyOnceWith("/threads?workspace=ws-2"),
    );
    await act(async () => {
      old.resolve(settings("last_task"));
    });
    expect(store.getState().workspaces.activeId).toBe("ws-2");
    expect(store.getState().userSettings.startupPage).toBe("threads");
    expect(mocks.router.replace).toHaveBeenCalledTimes(1);
  });

  it("ignores a late response from another workspace", async () => {
    const oldWorkflows = deferred<{ workflows: Array<Record<string, unknown>> }>();
    mocks.listWorkspaces.mockResolvedValue({
      workspaces: [workspace("ws-1"), workspace("ws-2")],
      total: 2,
    });
    mocks.listWorkflows.mockImplementation((workspaceId: string) =>
      workspaceId === "ws-1"
        ? oldWorkflows.promise
        : Promise.resolve({
            workflows: [
              {
                id: "wf-2",
                workspace_id: "ws-2",
                name: "Workspace 2",
                description: null,
                sort_order: 0,
              },
            ],
          }),
    );

    const { rerender } = render(<Subject route={{ workspaceId: "ws-1" }} />);
    await waitFor(() =>
      expect(mocks.listWorkflows).toHaveBeenCalledWith("ws-1", expect.anything()),
    );

    rerender(<Subject route={{ workspaceId: "ws-2" }} />);
    await waitFor(() => {
      expect(store.getState().workspaces.activeId).toBe("ws-2");
      expect(store.getState().workflows.items).toEqual([
        expect.objectContaining({ id: "wf-2", workspaceId: "ws-2" }),
      ]);
    });

    await act(async () => {
      oldWorkflows.resolve({
        workflows: [
          {
            id: "wf-1",
            workspace_id: "ws-1",
            name: "Workspace 1",
            description: null,
            sort_order: 0,
          },
        ],
      });
    });

    expect(store.getState().workspaces.activeId).toBe("ws-2");
    expect(store.getState().workflows.items).not.toEqual(
      expect.arrayContaining([expect.objectContaining({ id: "wf-1" })]),
    );
  });

  it("cancels a pending bootstrap when the route unmounts", async () => {
    const pending = deferred<ReturnType<typeof settings>>();
    mocks.fetchUserSettings.mockReturnValue(pending.promise);
    const { unmount } = render(<Subject />);
    unmount();
    await act(async () => {
      pending.resolve(settings());
    });
    expect(store.getState().workspaces.activeId).toBeNull();
    expect(store.getState().userSettings.loaded).toBe(false);
    expect(mocks.router.replace).not.toHaveBeenCalled();
  });

  it("settles route-owned reads on unmount and recovers a later transient failure", async () => {
    const workflows = deferred<{ workflows: Array<Record<string, unknown>> }>();
    const repositories = deferred<{ repositories: Array<Record<string, unknown>> }>();
    mocks.listWorkflows.mockReturnValue(workflows.promise);
    mocks.listRepositories.mockReturnValue(repositories.promise);

    const { unmount } = render(<Subject route={{ workspaceId: "ws-1" }} />);
    await waitFor(() => {
      expect(store.getState().workspaceContextRead.pending).toMatchObject({
        workflows: true,
        repositories: true,
      });
    });

    unmount();
    expect(store.getState().workspaceContextRead.pending).toMatchObject({
      workflows: false,
      repositories: false,
    });

    await act(async () => {
      workflows.resolve({ workflows: [] });
      repositories.resolve({ repositories: [] });
    });
    expect(store.getState().workspaceContextRead.pending).toMatchObject({
      workflows: false,
      repositories: false,
    });

    mocks.listWorkflows.mockRejectedValueOnce(new Error("temporary workflow failure"));
    mocks.listRepositories.mockResolvedValueOnce({ repositories: [] });
    const second = render(<Subject route={{ workspaceId: "ws-1" }} />);
    await waitFor(() =>
      expect(store.getState().workspaceContextRead.errors.workflows).toBe("transient"),
    );

    mocks.listWorkflows.mockResolvedValueOnce({ workflows: [] });
    act(() => store.getState().requestWorkspaceContextRefresh());
    await waitFor(() => expect(store.getState().workspaceContextRead.errors.workflows).toBeNull());
    second.unmount();
  });
});

// @covers AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.5
describe("Home readiness after hydrated startup", () => {
  it.each([null, "wf-2"])(
    "stays ready for live workflow filter %s but gates a new route",
    async (id) => {
      const pending = deferred<ReturnType<typeof settings>>();
      mocks.fetchUserSettings.mockReturnValue(pending.promise);
      const { rerender } = render(
        <Subject
          initialState={{
            workspaces: { activeId: "ws-1", items: [workspace("ws-1")] },
            userSettings: mapUserSettingsData(settings("task_overview").settings),
            workflows: {
              activeId: "wf-1",
              items: ["wf-1", "wf-2"].map((workflowId) => ({
                id: workflowId,
                workspaceId: "ws-1",
                name: workflowId,
                description: null,
                sortOrder: 0,
              })),
            },
            kanban: { ...defaultState.kanban, workflowId: "wf-1" },
          }}
        />,
      );
      expect(screen.queryByRole("status")).toBeNull();
      act(() => store.getState().setActiveWorkflow(id));
      expect(screen.queryByRole("status")).toBeNull();
      expect(mocks.fetchUserSettings).not.toHaveBeenCalled();

      mocks.listWorkspaces.mockResolvedValue({
        workspaces: [workspace("ws-1"), workspace("ws-2")],
        total: 2,
      });
      rerender(<Subject route={{ workspaceId: "ws-2" }} />);
      expect(screen.getByRole("status")).toBeTruthy();
      await act(async () => pending.resolve(settings("task_overview")));
      await waitFor(() => expect(screen.queryByRole("status")).toBeNull());
      expect(store.getState().workspaces.activeId).toBe("ws-2");
      expect(mocks.router.replace).not.toHaveBeenCalled();
    },
  );
});

describe("Home startup fallback", () => {
  it.each(["blocked", "malformed"])("uses fixed Threads with %s listing storage", async (mode) => {
    if (mode === "blocked") {
      vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
        throw new Error("storage denied");
      });
    } else window.localStorage.setItem(TASK_LISTING_VIEW_STORAGE_KEY, "not-json");
    render(<Subject />);
    await waitFor(() =>
      expect(mocks.router.replace).toHaveBeenCalledExactlyOnceWith("/threads?workspace=ws-1"),
    );
  });

  it("keeps Office priority over fixed Threads", async () => {
    mocks.listWorkspaces.mockResolvedValue({
      workspaces: [workspace("ws-1", "office-wf")],
      total: 1,
    });
    render(<Subject initialState={{ features: { ...defaultState.features, office: true } }} />);
    await waitFor(() =>
      expect(mocks.router.replace).toHaveBeenCalledExactlyOnceWith("/office?workspaceId=ws-1"),
    );
  });

  it("settles an empty workspace list without sending onboarding to Threads", async () => {
    mocks.listWorkspaces.mockResolvedValue({ workspaces: [], total: 0 });
    render(<Subject />);
    await waitFor(() => expect(store.getState().userSettings.loaded).toBe(true));
    expect(screen.queryByRole("status")).toBeNull();
    expect(mocks.router.replace).not.toHaveBeenCalled();
  });

  it("settles failed bootstrap with the existing fallback", async () => {
    mocks.listWorkspaces.mockRejectedValue(new Error("offline"));
    mocks.fetchUserSettings.mockRejectedValue(new Error("offline"));
    render(<Subject />);
    expect(screen.getByRole("status")).toBeTruthy();
    await waitFor(() => expect(screen.queryByRole("status")).toBeNull());
    expect(store.getState().userSettings.startupPage).toBe("task_overview");
    expect(mocks.router.replace).not.toHaveBeenCalled();
  });

  it("falls back to remembered List when the settings fetch fails", async () => {
    window.localStorage.setItem(TASK_LISTING_VIEW_STORAGE_KEY, '"list"');
    mocks.fetchUserSettings.mockRejectedValue(new Error("offline"));
    render(<Subject />);
    await waitFor(() =>
      expect(mocks.router.replace).toHaveBeenCalledExactlyOnceWith("/tasks?workspace=ws-1"),
    );
  });
});
