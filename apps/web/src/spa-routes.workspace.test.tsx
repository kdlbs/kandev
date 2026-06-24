import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { StateProvider } from "@/components/state-provider";
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
const TEST_TIMESTAMP = "2026-06-24T00:00:00Z";

vi.mock("@/app/github/github-page-client", () => ({
  GitHubPageClient: ({ workspaceId }: { workspaceId?: string }) => (
    <div data-testid="github-page" data-workspace-id={workspaceId ?? ""} />
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

describe("SpaRoutes data-backed workspace context", () => {
  it("keeps the currently active workspace when opening GitHub from another workspace", async () => {
    window.history.replaceState({}, "", "/github");
    mocks.listWorkspaces.mockResolvedValue({
      workspaces: [workspace(DEFAULT_WORKSPACE_ID), workspace(SELECTED_WORKSPACE_ID)],
    });
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

    await waitFor(() => {
      expect(screen.getByTestId("github-page").getAttribute("data-workspace-id")).toBe(
        SELECTED_WORKSPACE_ID,
      );
    });
    expect(mocks.listWorkflows).toHaveBeenCalledWith(SELECTED_WORKSPACE_ID, {
      cache: "no-store",
    });
  });

  it("uses the active workspace cookie when the store has no active workspace yet", async () => {
    window.history.replaceState({}, "", "/github");
    document.cookie = `kandev-active-workspace=${SELECTED_WORKSPACE_ID}; path=/`;
    mocks.listWorkspaces.mockResolvedValue({
      workspaces: [workspace(DEFAULT_WORKSPACE_ID), workspace(SELECTED_WORKSPACE_ID)],
    });
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

    render(
      <StateProvider>
        <SpaRoutes />
      </StateProvider>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("github-page").getAttribute("data-workspace-id")).toBe(
        SELECTED_WORKSPACE_ID,
      );
    });
    expect(mocks.listWorkflows).toHaveBeenCalledWith(SELECTED_WORKSPACE_ID, {
      cache: "no-store",
    });
  });
});

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
