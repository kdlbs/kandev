import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  fetchAccessibleRepos: vi.fn(),
  listUserProjects: vi.fn(),
  listAzureDevOpsProjects: vi.fn(),
  listAzureDevOpsRepositories: vi.fn(),
}));

vi.mock("@/lib/api/domains/github-api", () => ({
  fetchAccessibleRepos: mocks.fetchAccessibleRepos,
}));
vi.mock("@/lib/api/domains/gitlab-api", () => ({
  listUserProjects: mocks.listUserProjects,
}));
vi.mock("@/lib/api/domains/azure-devops-api", () => ({
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
    mocks.listUserProjects.mockRejectedValue(new Error("GitLab not configured"));
    mocks.listAzureDevOpsProjects.mockRejectedValue(new Error("Azure DevOps not configured"));

    const { result } = renderHook(() => useRepositoryRuleCatalog(WORKSPACE_ID, true));
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
