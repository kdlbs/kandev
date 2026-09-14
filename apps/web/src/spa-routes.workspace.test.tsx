import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { Repository } from "@/lib/types/http";
import type { StoreApi } from "zustand";
import { SpaRoutes } from "./spa-routes";

const mocks = vi.hoisted(() => ({
  listWorkspaces: vi.fn(),
  listRepositories: vi.fn(),
  listWorkflows: vi.fn(),
  fetchUserSettings: vi.fn(),
  fetchJson: vi.fn(),
}));

const DEFAULT_WORKSPACE_ID = "ws-default";
const SELECTED_WORKSPACE_ID = "ws-selected";
const EXISTING_WORKFLOW_ID = "wf-existing";
const EXISTING_REPOSITORY_ID = "repo-existing";
const TEST_TIMESTAMP = "2026-06-24T00:00:00Z";

let store: StoreApi<AppState>;

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function CaptureStore() {
  store = useAppStoreApi();
  return null;
}

vi.mock("@/app/github/github-page-client", () => ({
  GitHubPageClient: ({ workspaceId }: { workspaceId?: string }) => (
    <div data-testid="github-page" data-workspace-id={workspaceId ?? ""} />
  ),
}));

vi.mock("@/app/page-client", () => ({
  PageClient: ({ workspaceId }: { workspaceId?: string }) => (
    <div data-testid="kanban-page" data-workspace-id={workspaceId ?? ""} />
  ),
}));

vi.mock("@/lib/api/domains/workspace-api", () => ({
  listWorkspaces: mocks.listWorkspaces,
  listRepositories: mocks.listRepositories,
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  listWorkflows: mocks.listWorkflows,
}));

vi.mock("@/lib/api/domains/settings-api", () => ({
  fetchUserSettings: mocks.fetchUserSettings,
}));

vi.mock("@/lib/api/client", () => ({
  fetchJson: mocks.fetchJson,
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  document.cookie = "kandev-active-workspace=; path=/; max-age=0";
  document.cookie = "office-active-workspace=; path=/; max-age=0";
  window.history.replaceState({}, "", "/");
});

// eslint-disable-next-line max-lines-per-function -- route recovery cases share one boot fixture
describe("SpaRoutes data-backed workspace context", () => {
  it("passes the selected workspace to the kanban home client after bootstrap", async () => {
    mockGitHubWorkspaceBootstrap();
    window.history.replaceState({}, "", `/?workspaceId=${SELECTED_WORKSPACE_ID}`);

    render(
      <StateProvider>
        <SpaRoutes />
      </StateProvider>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("kanban-page").getAttribute("data-workspace-id")).toBe(
        SELECTED_WORKSPACE_ID,
      );
    });
  });

  it("keeps the currently active workspace when opening GitHub from another workspace", async () => {
    mockGitHubWorkspaceBootstrap();

    render(
      <StateProvider
        initialState={{
          workspaces: {
            items: [workspaceState(DEFAULT_WORKSPACE_ID), workspaceState(SELECTED_WORKSPACE_ID)],
            activeId: SELECTED_WORKSPACE_ID,
          },
        }}
      >
        <SpaRoutes />
      </StateProvider>,
    );

    await expectSelectedWorkspace();
  });

  it("uses the active workspace cookie when the store has no active workspace yet", async () => {
    document.cookie = `kandev-active-workspace=${SELECTED_WORKSPACE_ID}; path=/`;
    mockGitHubWorkspaceBootstrap();

    render(
      <StateProvider>
        <SpaRoutes />
      </StateProvider>,
    );

    await expectSelectedWorkspace();
  });

  it("keeps a known active workspace when the workspace list request fails", async () => {
    document.cookie = `kandev-active-workspace=${SELECTED_WORKSPACE_ID}; path=/`;
    mockGitHubWorkspaceBootstrap({ workspacesError: new Error("network down") });

    render(
      <StateProvider>
        <SpaRoutes />
      </StateProvider>,
    );

    await expectSelectedWorkspace();
  });

  it("retains workflows after a failed refresh", async () => {
    mockGitHubWorkspaceBootstrap();
    mocks.listWorkflows.mockRejectedValueOnce(new Error("workflow read failed"));
    mocks.listRepositories.mockRejectedValueOnce(new Error("repository read failed"));
    mocks.fetchJson.mockRejectedValueOnce(new Error("step read failed"));

    render(
      <StateProvider
        initialState={{
          workspaces: {
            items: [workspace(DEFAULT_WORKSPACE_ID), workspace(SELECTED_WORKSPACE_ID)],
            activeId: SELECTED_WORKSPACE_ID,
          },
          workflows: {
            activeId: EXISTING_WORKFLOW_ID,
            items: [
              {
                id: EXISTING_WORKFLOW_ID,
                workspaceId: SELECTED_WORKSPACE_ID,
                name: "Existing",
                description: null,
                sortOrder: 0,
              },
            ],
          },
          repositories: {
            itemsByWorkspaceId: {
              [SELECTED_WORKSPACE_ID]: [repository(EXISTING_REPOSITORY_ID)],
            },
            loadingByWorkspaceId: {},
            loadedByWorkspaceId: {},
          },
        }}
      >
        <CaptureStore />
        <SpaRoutes />
      </StateProvider>,
    );

    await waitFor(() => expect(screen.getByTestId("github-page")).toBeTruthy());
    expect(store.getState().workflows.items).toEqual([
      expect.objectContaining({ id: EXISTING_WORKFLOW_ID }),
    ]);
    expect(store.getState().repositories.itemsByWorkspaceId[SELECTED_WORKSPACE_ID]).toEqual([
      expect.objectContaining({ id: EXISTING_REPOSITORY_ID }),
    ]);
  });

  it("accepts a successful empty list", async () => {
    mockGitHubWorkspaceBootstrap();
    mocks.listWorkflows.mockResolvedValueOnce({ workflows: [] });

    render(
      <StateProvider
        initialState={{
          workspaces: {
            items: [workspace(DEFAULT_WORKSPACE_ID), workspace(SELECTED_WORKSPACE_ID)],
            activeId: SELECTED_WORKSPACE_ID,
          },
          workflows: {
            activeId: EXISTING_WORKFLOW_ID,
            items: [
              {
                id: EXISTING_WORKFLOW_ID,
                workspaceId: SELECTED_WORKSPACE_ID,
                name: "Existing",
                description: null,
                sortOrder: 0,
              },
            ],
          },
          repositories: {
            itemsByWorkspaceId: {
              [SELECTED_WORKSPACE_ID]: [repository(EXISTING_REPOSITORY_ID)],
            },
            loadingByWorkspaceId: {},
            loadedByWorkspaceId: {},
          },
        }}
      >
        <CaptureStore />
        <SpaRoutes />
      </StateProvider>,
    );

    await waitFor(() => expect(store.getState().workflows.items).toEqual([]));
    expect(store.getState().repositories.itemsByWorkspaceId[SELECTED_WORKSPACE_ID]).toEqual([]);
  });

  it("applies partial successes", async () => {
    mockGitHubWorkspaceBootstrap();
    mocks.listWorkflows.mockResolvedValueOnce({
      workflows: [workflow("wf-new", SELECTED_WORKSPACE_ID)],
    });
    mocks.listRepositories.mockRejectedValueOnce(new Error("repository read failed"));

    render(
      <StateProvider
        initialState={{
          workspaces: {
            items: [workspace(DEFAULT_WORKSPACE_ID), workspace(SELECTED_WORKSPACE_ID)],
            activeId: SELECTED_WORKSPACE_ID,
          },
          workflows: {
            activeId: EXISTING_WORKFLOW_ID,
            items: [
              {
                id: EXISTING_WORKFLOW_ID,
                workspaceId: SELECTED_WORKSPACE_ID,
                name: "Existing",
                description: null,
                sortOrder: 0,
              },
            ],
          },
          repositories: {
            itemsByWorkspaceId: {
              [SELECTED_WORKSPACE_ID]: [repository(EXISTING_REPOSITORY_ID)],
            },
            loadingByWorkspaceId: {},
            loadedByWorkspaceId: {},
          },
        }}
      >
        <CaptureStore />
        <SpaRoutes />
      </StateProvider>,
    );

    await waitFor(() =>
      expect(store.getState().workflows.items).toEqual([expect.objectContaining({ id: "wf-new" })]),
    );
    expect(store.getState().repositories.itemsByWorkspaceId[SELECTED_WORKSPACE_ID]).toEqual([
      expect.objectContaining({ id: EXISTING_REPOSITORY_ID }),
    ]);
    expect(store.getState().workspaceContextRead.errors.repositories).toBe("transient");
  });

  it("settles route-owned pending reads before recovering a later transient failure", async () => {
    mockGitHubWorkspaceBootstrap();
    const oldWorkflows = deferred<{ workflows: Array<Record<string, unknown>> }>();
    const oldRepositories = deferred<{ repositories: Repository[] }>();
    const oldSteps = deferred<{ steps: Array<Record<string, unknown>>; total: number }>();
    mocks.listWorkflows.mockReturnValueOnce(oldWorkflows.promise);
    mocks.listRepositories.mockReturnValueOnce(oldRepositories.promise);
    mocks.fetchJson.mockReturnValueOnce(oldSteps.promise);

    const first = render(
      <StateProvider>
        <CaptureStore />
        <SpaRoutes />
      </StateProvider>,
    );

    await waitFor(() =>
      expect(store.getState().workspaceContextRead.pending).toMatchObject({
        workflows: true,
        repositories: true,
        steps: true,
      }),
    );
    first.unmount();
    expect(store.getState().workspaceContextRead.pending).toEqual({
      workflows: false,
      repositories: false,
      steps: false,
    });

    await act(async () => {
      oldWorkflows.resolve({ workflows: [] });
      oldRepositories.resolve({ repositories: [] });
      oldSteps.resolve({ steps: [], total: 0 });
    });

    mocks.listWorkflows.mockRejectedValueOnce(new Error("temporary workflow failure"));
    mocks.listRepositories.mockResolvedValueOnce({ repositories: [] });
    mocks.fetchJson.mockResolvedValueOnce({ steps: [], total: 0 });
    const second = render(
      <StateProvider>
        <CaptureStore />
        <SpaRoutes />
      </StateProvider>,
    );
    await waitFor(() =>
      expect(store.getState().workspaceContextRead.errors.workflows).toBe("transient"),
    );

    mocks.listWorkflows.mockResolvedValueOnce({ workflows: [] });
    act(() => store.getState().requestWorkspaceContextRefresh());
    await waitFor(() => expect(store.getState().workspaceContextRead.errors.workflows).toBeNull());
    second.unmount();
  });
});

function mockGitHubWorkspaceBootstrap({ workspacesError }: { workspacesError?: Error } = {}) {
  window.history.replaceState({}, "", "/github");
  if (workspacesError) {
    mocks.listWorkspaces.mockRejectedValue(workspacesError);
  } else {
    mocks.listWorkspaces.mockResolvedValue({
      workspaces: [workspace(DEFAULT_WORKSPACE_ID), workspace(SELECTED_WORKSPACE_ID)],
    });
  }
  mocks.fetchUserSettings.mockResolvedValue({
    settings: {
      workspace_id: DEFAULT_WORKSPACE_ID,
      workflow_filter_id: "",
      repository_ids: [],
      updated_at: TEST_TIMESTAMP,
    },
  });
  mocks.listWorkflows.mockResolvedValue({
    workflows: [workflow("wf-selected", SELECTED_WORKSPACE_ID)],
  });
  mocks.listRepositories.mockResolvedValue({ repositories: [] });
  mocks.fetchJson.mockResolvedValue({ steps: [], total: 0 });
}

async function expectSelectedWorkspace() {
  await waitFor(() => {
    expect(screen.getByTestId("github-page").getAttribute("data-workspace-id")).toBe(
      SELECTED_WORKSPACE_ID,
    );
  });
  expect(mocks.listWorkflows).toHaveBeenCalledWith(SELECTED_WORKSPACE_ID, {
    cache: "no-store",
  });
}

function workspace(id: string) {
  return {
    id,
    name: id,
    description: null,
    owner_id: "owner-1",
    default_executor_id: null,
    default_environment_id: null,
    default_agent_profile_id: null,
    default_config_agent_profile_id: null,
    office_workflow_id: null,
    created_at: TEST_TIMESTAMP,
    updated_at: TEST_TIMESTAMP,
  };
}

function workspaceState(id: string) {
  return workspace(id);
}

function workflow(id: string, workspaceId: string) {
  return {
    id,
    workspace_id: workspaceId,
    name: id,
    description: null,
    sort_order: 0,
    created_at: TEST_TIMESTAMP,
    updated_at: TEST_TIMESTAMP,
  };
}

function repository(id: string): Repository {
  return {
    id,
    workspace_id: SELECTED_WORKSPACE_ID,
    name: id,
    source_type: "local",
    path: `/tmp/${id}`,
    local_path: `/tmp/${id}`,
    provider: "",
    provider_repo_id: "",
    provider_owner: "",
    provider_name: "",
    default_branch: "main",
    worktree_branch_prefix: "task/",
    pull_before_worktree: false,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
    copy_files: "",
    created_at: TEST_TIMESTAMP,
    updated_at: TEST_TIMESTAMP,
  } as unknown as Repository;
}
