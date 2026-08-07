import { describe, expect, it, vi, beforeEach } from "vitest";
import { render } from "@testing-library/react";

const mocks = vi.hoisted(() => ({
  listWorkspacesAction: vi.fn(),
  listWorkflowsAction: vi.fn(),
  listRepositoriesAction: vi.fn(),
  listWorkspaceWorkflowStepsAction: vi.fn(),
  fetchUserSettings: vi.fn(),
  cookies: vi.fn(),
  capturedInitialState: null as unknown,
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

vi.mock("next/headers", () => ({
  cookies: mocks.cookies,
}));

vi.mock("@/components/state-hydrator", () => ({
  StateHydrator: ({ initialState }: { initialState: unknown }) => {
    mocks.capturedInitialState = initialState;
    return null;
  },
}));

vi.mock("./github-page-client", () => ({
  GitHubPageClient: () => null,
}));

import GitHubPage from "./page";

describe("GitHubPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.capturedInitialState = null;
    mocks.listWorkspacesAction.mockResolvedValue({
      workspaces: [
        { id: "ws-go", name: "GO" },
        { id: "ws-connection", name: "Connection" },
      ],
    });
    mocks.fetchUserSettings.mockResolvedValue({
      settings: { workspace_id: "ws-connection" },
    });
    mocks.cookies.mockResolvedValue({
      get: (name: string) =>
        name === "office-active-workspace" ? { value: "ws-go" } : undefined,
    });
    mocks.listWorkflowsAction.mockResolvedValue({ workflows: [] });
    mocks.listRepositoriesAction.mockResolvedValue({ repositories: [] });
    mocks.listWorkspaceWorkflowStepsAction.mockResolvedValue({ steps: [] });
  });

  it("prefers the active workspace cookie over stale saved user settings", async () => {
    const ui = await GitHubPage({});
    render(ui);

    expect(mocks.listWorkflowsAction).toHaveBeenCalledWith("ws-go");
    expect(mocks.listRepositoriesAction).toHaveBeenCalledWith("ws-go");
    expect(mocks.listWorkspaceWorkflowStepsAction).toHaveBeenCalledWith("ws-go");
    expect(mocks.capturedInitialState).toMatchObject({
      workspaces: { activeId: "ws-go" },
      userSettings: { workspaceId: "ws-go" },
    });
  });
});
