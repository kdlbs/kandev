import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { getGitHubIntegrationStatus } from "./use-nav-availability";
import type { GitHubStatus } from "@/lib/types/github";

const mocks = vi.hoisted(() => {
  const workspaceId = "workspace-1";
  return {
    workspaceId,
    state: {
      workspaces: {
        activeId: workspaceId as string | null,
        items: [{ id: workspaceId }],
      },
    },
    azureDevOpsAvailable: vi.fn(() => false),
    gitlabAvailable: vi.fn(() => false),
    jiraAvailable: vi.fn(() => false),
    linearAvailable: vi.fn(() => false),
  };
});

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
}));
vi.mock("@/hooks/domains/azure-devops/use-azure-devops-availability", () => ({
  useAzureDevOpsAvailable: mocks.azureDevOpsAvailable,
}));
vi.mock("@/hooks/domains/github/use-github-status", () => ({
  useGitHubStatus: () => ({ status: null, loading: false }),
}));
vi.mock("@/hooks/domains/gitlab/use-task-mr", () => ({
  useGitLabAvailable: mocks.gitlabAvailable,
}));
vi.mock("@/hooks/domains/jira/use-jira-availability", () => ({
  useJiraAvailable: mocks.jiraAvailable,
}));
vi.mock("@/hooks/domains/linear/use-linear-availability", () => ({
  useLinearAvailable: mocks.linearAvailable,
}));

import { useNavAvailability } from "./use-nav-availability";

function status(overrides: Partial<GitHubStatus>): GitHubStatus {
  return {
    authenticated: false,
    username: "",
    auth_method: "none",
    token_configured: false,
    required_scopes: [],
    ...overrides,
  };
}

describe("getGitHubIntegrationStatus", () => {
  it("shows checking while GitHub status is loading and not configured", () => {
    expect(getGitHubIntegrationStatus(status({}), true)).toEqual({
      ready: false,
      label: "Checking",
    });
  });

  it("treats a configured token as ready even before live auth is green", () => {
    expect(getGitHubIntegrationStatus(status({ token_configured: true }), false)).toEqual({
      ready: true,
      label: "Configured",
    });
  });

  it("uses the Connected label for authenticated status", () => {
    expect(getGitHubIntegrationStatus(status({ authenticated: true }), false)).toEqual({
      ready: true,
      label: "Connected",
    });
  });

  it("shows setup only when no auth or token is configured", () => {
    expect(getGitHubIntegrationStatus(status({}), false)).toEqual({
      ready: false,
      label: "Setup",
    });
  });
});

describe("useNavAvailability", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.state.workspaces.activeId = mocks.workspaceId;
    mocks.state.workspaces.items = [{ id: mocks.workspaceId }];
  });

  it("scopes workspace integrations to the active workspace", () => {
    renderHook(() => useNavAvailability());

    expect(mocks.jiraAvailable).toHaveBeenCalledWith(mocks.workspaceId);
    expect(mocks.linearAvailable).toHaveBeenCalledWith(mocks.workspaceId);
    expect(mocks.azureDevOpsAvailable).toHaveBeenCalledWith(mocks.workspaceId);
  });

  it("falls back to default workspace resolution for a stale active id", () => {
    mocks.state.workspaces.items = [{ id: "workspace-2" }];

    renderHook(() => useNavAvailability());

    expect(mocks.jiraAvailable).toHaveBeenCalledWith(null);
    expect(mocks.linearAvailable).toHaveBeenCalledWith(null);
    expect(mocks.azureDevOpsAvailable).toHaveBeenCalledWith(null);
  });
});
