import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";

const mocks = vi.hoisted(() => ({
  fetchAccessibleRepos: vi.fn(),
  fetchGitHubStatus: vi.fn(),
  fetchGitLabStatus: vi.fn(),
  getAzureDevOpsConfig: vi.fn(),
  listUserProjects: vi.fn(),
  listAzureDevOpsProjects: vi.fn(),
  listAzureDevOpsRepositories: vi.fn(),
}));

vi.mock("@/lib/api/domains/github-api", () => ({
  fetchAccessibleRepos: mocks.fetchAccessibleRepos,
  fetchGitHubStatus: mocks.fetchGitHubStatus,
}));
vi.mock("@/lib/api/domains/gitlab-api", () => ({
  fetchGitLabStatus: mocks.fetchGitLabStatus,
  listUserProjects: mocks.listUserProjects,
}));
vi.mock("@/lib/api/domains/azure-devops-api", () => ({
  getAzureDevOpsConfig: mocks.getAzureDevOpsConfig,
  listAzureDevOpsProjects: mocks.listAzureDevOpsProjects,
  listAzureDevOpsRepositories: mocks.listAzureDevOpsRepositories,
}));
vi.mock("@/components/task/add-workspace-sources/use-workspace-repository-options", () => ({
  useWorkspaceRepositoryOptions: () => ({
    repositories: [],
    discoveredRepositories: [],
    repositoriesRefreshing: false,
    error: null,
    refreshRepositoryOptions: vi.fn(),
  }),
}));

import { pluginRegistry } from "@/lib/plugins/registry";
import { useRepositoryRuleCatalog } from "./use-repository-rule-catalog";

const WORKSPACE_ID = "workspace-1";
const PLUGIN_ID = "catalog-bitbucket-provider";

const wrapper = ({ children }: { children: ReactNode }) => (
  <StateProvider>{children}</StateProvider>
);

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN_ID);
  vi.resetAllMocks();
});

describe("useRepositoryRuleCatalog", () => {
  it("keeps provider hosts searchable through the remote hook", async () => {
    pluginRegistry.forPlugin(PLUGIN_ID).registerRepositoryProvider({
      id: "bitbucket",
      label: "Bitbucket",
      matchesURL: () => false,
      listBranches: async () => [],
      inspectURL: async () => null,
      listRepositories: async () => [
        {
          providerId: "bitbucket",
          providerHost: "https://bitbucket.org",
          ownerOrProject: "platform",
          repositoryId: "api",
          repositoryName: "api",
          cloneUrl: "https://bitbucket.org/platform/api.git",
        },
      ],
    });
    mocks.fetchAccessibleRepos.mockRejectedValue(new Error("GitHub not configured"));
    mocks.fetchGitHubStatus.mockResolvedValue({
      authenticated: false,
      token_configured: false,
    });
    mocks.fetchGitLabStatus.mockResolvedValue(null);
    mocks.getAzureDevOpsConfig.mockResolvedValue(null);
    mocks.listUserProjects.mockRejectedValue(new Error("GitLab not configured"));
    mocks.listAzureDevOpsProjects.mockRejectedValue(new Error("Azure DevOps not configured"));

    const { result } = renderHook(() => useRepositoryRuleCatalog(WORKSPACE_ID, true), { wrapper });
    await waitFor(() => expect(result.current.loading).toBe(false));

    act(() => result.current.setQuery("bitbucket.org"));
    await waitFor(() => expect(result.current.options).toHaveLength(1));
    expect(result.current.options[0]).toMatchObject({
      label: "platform/api",
      secondaryLabel: "https://bitbucket.org",
      group: "plugin",
    });
  });
});
