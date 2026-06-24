import { describe, expect, it, vi, beforeEach } from "vitest";

const mocks = vi.hoisted(() => ({
  listWorkspacesAction: vi.fn(),
  listWorkflowsAction: vi.fn(),
  listRepositoriesAction: vi.fn(),
  listWorkspaceWorkflowStepsAction: vi.fn(),
  fetchUserSettings: vi.fn(),
  readCookies: vi.fn(),
  stateHydrator: vi.fn(),
  pageClient: vi.fn(),
}));

vi.mock("@/app/actions/workspaces", () => ({
  listWorkspacesAction: mocks.listWorkspacesAction,
  listWorkflowsAction: mocks.listWorkflowsAction,
  listRepositoriesAction: mocks.listRepositoriesAction,
  listWorkspaceWorkflowStepsAction: mocks.listWorkspaceWorkflowStepsAction,
}));

vi.mock("@/lib/api", () => ({
  fetchUserSettings: mocks.fetchUserSettings,
}));

vi.mock("@/lib/server/cookies", () => ({
  readCookies: mocks.readCookies,
}));

vi.mock("@/components/state-hydrator", () => ({
  StateHydrator: (props: { initialState: unknown }) => {
    mocks.stateHydrator(props);
    return <div data-testid="state-hydrator" />;
  },
}));

vi.mock("./github-page-client", () => ({
  GitHubPageClient: (props: { workspaceId?: string }) => {
    mocks.pageClient(props);
    return <div data-testid="github-page-client" />;
  },
}));

import GitHubPage from "./page";

const ACTIVE_WORKSPACE_COOKIE = "kandev-active-workspace";
const WORKSPACE_TIMESTAMP = "2026-01-01T00:00:00Z";

const defaultWorkspace = {
  id: "ws-default",
  name: "Default",
  created_at: WORKSPACE_TIMESTAMP,
  updated_at: WORKSPACE_TIMESTAMP,
};

const activeWorkspace = {
  id: "ws-active",
  name: "Active",
  created_at: WORKSPACE_TIMESTAMP,
  updated_at: WORKSPACE_TIMESTAMP,
};

describe("GitHubPage", () => {
  beforeEach(() => {
    Object.values(mocks).forEach((mock) => mock.mockReset());
    mocks.listWorkspacesAction.mockResolvedValue({
      workspaces: [defaultWorkspace, activeWorkspace],
    });
    mocks.fetchUserSettings.mockResolvedValue({
      settings: { workspace_id: defaultWorkspace.id },
    });
    mocks.readCookies.mockResolvedValue({
      get: (name: string) =>
        name === ACTIVE_WORKSPACE_COOKIE ? { value: activeWorkspace.id } : undefined,
    });
    mocks.listWorkflowsAction.mockResolvedValue({ workflows: [] });
    mocks.listRepositoriesAction.mockResolvedValue({ repositories: [] });
    mocks.listWorkspaceWorkflowStepsAction.mockResolvedValue({ steps: [] });
  });

  it("loads local GitHub context for the active workspace cookie", async () => {
    const result = (await GitHubPage()) as {
      props: { children: { props: { workspaceId?: string } }[] };
    };

    expect(mocks.listWorkflowsAction).toHaveBeenCalledWith(activeWorkspace.id);
    expect(mocks.listRepositoriesAction).toHaveBeenCalledWith(activeWorkspace.id);
    expect(mocks.listWorkspaceWorkflowStepsAction).toHaveBeenCalledWith(activeWorkspace.id);
    expect(result.props.children[1]?.props.workspaceId).toBe(activeWorkspace.id);
  });
});
