import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  fetchAccessibleRepos: vi.fn(),
  listUserProjects: vi.fn(),
  listAzureDevOpsProjects: vi.fn(),
  listAzureDevOpsRepositories: vi.fn(),
  useGitHubStatus: vi.fn(),
  useGitLabStatus: vi.fn(),
  useAzureDevOpsConnection: vi.fn(),
}));

vi.mock("@/lib/api/domains/github-api", () => ({
  fetchAccessibleRepos: mocks.fetchAccessibleRepos,
}));
vi.mock("@/lib/api/domains/gitlab-api", () => ({ listUserProjects: mocks.listUserProjects }));
vi.mock("@/lib/api/domains/azure-devops-api", () => ({
  listAzureDevOpsProjects: mocks.listAzureDevOpsProjects,
  listAzureDevOpsRepositories: mocks.listAzureDevOpsRepositories,
}));
vi.mock("@/hooks/domains/github/use-github-status", () => ({
  useGitHubStatus: mocks.useGitHubStatus,
}));
vi.mock("@/hooks/domains/gitlab/use-gitlab-status", () => ({
  useGitLabStatus: mocks.useGitLabStatus,
}));
vi.mock("@/hooks/domains/azure-devops/use-azure-devops-browse", () => ({
  useAzureDevOpsConnection: mocks.useAzureDevOpsConnection,
}));

import { useRemoteRepositories } from "./use-remote-repositories";
import { pluginRegistry } from "@/lib/plugins/registry";

const WORKSPACE_ID = "workspace-1";
const PLUGIN_ID = "test-bitbucket-provider";

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN_ID);
  vi.resetAllMocks();
});

function setBuiltInAvailability({
  github = true,
  gitlab = true,
  azureDevOps = true,
}: {
  github?: boolean;
  gitlab?: boolean;
  azureDevOps?: boolean;
} = {}) {
  mocks.useGitHubStatus.mockReturnValue({
    status: github ? { authenticated: true, token_configured: true } : null,
    loaded: true,
    loading: false,
    refresh: vi.fn(),
  });
  mocks.useGitLabStatus.mockReturnValue({
    status: gitlab ? { authenticated: true, token_configured: true } : null,
    loading: false,
    refresh: vi.fn(),
  });
  mocks.useAzureDevOpsConnection.mockReturnValue({
    data: azureDevOps ? { hasSecret: true, lastOk: true } : null,
    loading: false,
    error: null,
    refresh: vi.fn(),
  });
}

beforeEach(() => setBuiltInAvailability());

function expectWorkspaceScopedGitHubRequest() {
  expect(mocks.fetchAccessibleRepos).toHaveBeenCalledWith({
    workspaceId: WORKSPACE_ID,
    limit: 100,
  });
}

const githubRepo = (name: string) => ({
  owner: "acme",
  name,
  full_name: `acme/${name}`,
  default_branch: "main",
  private: false,
});

describe("useRemoteRepositories provider restriction", () => {
  it("loads only the requested providers", async () => {
    const pluginList = vi.fn(async () => []);
    pluginRegistry.forPlugin(PLUGIN_ID).registerRepositoryProvider({
      id: "bitbucket",
      label: "Bitbucket",
      matchesURL: () => false,
      listBranches: async () => [],
      inspectURL: async () => null,
      listRepositories: pluginList,
    });
    mocks.fetchAccessibleRepos.mockResolvedValue([githubRepo("api")]);
    mocks.listUserProjects.mockResolvedValue({ projects: [] });
    mocks.listAzureDevOpsProjects.mockResolvedValue({ projects: [] });

    const { result } = renderHook(() =>
      useRemoteRepositories(WORKSPACE_ID, { providers: ["github"] }),
    );

    await waitFor(() => expect(result.current.loading).toBe(false));
    expectWorkspaceScopedGitHubRequest();
    expect(mocks.listUserProjects).not.toHaveBeenCalled();
    expect(mocks.listAzureDevOpsProjects).not.toHaveBeenCalled();
    expect(pluginList).not.toHaveBeenCalled();
    expect(result.current.availableProviders).toEqual(["github"]);
    expect(result.current.repos.map((repo) => repo.fullName)).toEqual(["acme/api"]);
  });
});

describe("useRemoteRepositories cache bypass", () => {
  it("asks GitHub to bypass its cache only for the load after a manual refresh", async () => {
    mocks.fetchAccessibleRepos.mockResolvedValue([githubRepo("api")]);
    const { result, rerender } = renderHook(() =>
      useRemoteRepositories(WORKSPACE_ID, { providers: ["github"] }),
    );
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(mocks.fetchAccessibleRepos).toHaveBeenLastCalledWith({
      workspaceId: WORKSPACE_ID,
      limit: 100,
    });

    act(() => result.current.refresh?.());
    await waitFor(() => expect(mocks.fetchAccessibleRepos).toHaveBeenCalledTimes(2));
    expect(mocks.fetchAccessibleRepos).toHaveBeenLastCalledWith({
      workspaceId: WORKSPACE_ID,
      limit: 100,
      refresh: true,
    });
    await waitFor(() => expect(result.current.loading).toBe(false));

    setBuiltInAvailability({ github: true });
    rerender();
    expect(mocks.fetchAccessibleRepos).toHaveBeenCalledTimes(2);
  });
});

describe("useRemoteRepositories GitHub host", () => {
  it("exposes the workspace's GitHub host for typed URL parsing", async () => {
    mocks.useGitHubStatus.mockReturnValue({
      status: {
        authenticated: true,
        token_configured: true,
        automation: { github_host: "git.company.example" },
      },
      loaded: true,
      loading: false,
      refresh: vi.fn(),
    });
    mocks.fetchAccessibleRepos.mockResolvedValue([]);
    const { result } = renderHook(() =>
      useRemoteRepositories(WORKSPACE_ID, { providers: ["github"] }),
    );
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.githubHost).toBe("git.company.example");
  });
});

describe("useRemoteRepositories GitHub-only status", () => {
  it("does not wait for or load other providers' status", async () => {
    mocks.useGitLabStatus.mockReturnValue({ status: null, loading: true, refresh: vi.fn() });
    mocks.useAzureDevOpsConnection.mockReturnValue({
      data: null,
      loading: true,
      error: null,
      refresh: vi.fn(),
    });
    mocks.fetchAccessibleRepos.mockResolvedValue([githubRepo("api")]);
    const { result } = renderHook(() =>
      useRemoteRepositories(WORKSPACE_ID, { providers: ["github"] }),
    );
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.repos.map((repo) => repo.fullName)).toEqual(["acme/api"]);
    expect(mocks.useGitLabStatus).toHaveBeenCalledWith("");
    expect(mocks.useGitLabStatus).not.toHaveBeenCalledWith(WORKSPACE_ID);
    expect(mocks.useAzureDevOpsConnection).not.toHaveBeenCalledWith(WORKSPACE_ID);
  });
});
